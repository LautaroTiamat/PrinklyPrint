// Package app cablea todos los componentes del agente y maneja su ciclo de vida.
//
// Es el "main" lógico: [main.main] solo parsea flags y delega en [App.Run].
// Aquí se construye el grafo de dependencias (config → store → printer →
// queue → server → tray → ui) y se coordina:
//
//   - Single-instance lock: usa un mutex nombrado del kernel de Windows
//     (Global\PrinklyPrintSingletonMutex_v1) para que no haya dos agentes
//     corriendo en la misma PC a la vez.
//
//   - Apagado ordenado: el [context.Context] recibido se propaga a todas las
//     goroutines (server, queue, ui). [App.RequestShutdown] permite disparar
//     el apagado desde la UI (botón "Cerrar PrinklyPrint" o tray "Salir"). Al
//     terminar, drena la cola con timeout y cierra el store SQLite y los logs.
//
//   - Bootstrap: crea el data dir (%LOCALAPPDATA%\PrinklyPrint), extrae
//     SumatraPDF embebido al subdir bin/, carga config.yaml, abre agent.db
//     (SQLite) y deriva un machine_id estable de hostname+dataDir.
package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/autostart"
	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lautarotiamat/prinklyprint/internal/logging"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/server"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

type Options struct {
	Version  string
	Headless bool
	DataDir  string
}

type App struct {
	opts      Options
	dataDir   string
	logger    *slog.Logger
	logCloser interface{ Close() error }
	cfg       *config.Manager
	store     *store.Store
	printer   *printer.Service
	queue     *queue.Worker
	server    *server.Server
	machineID string
	shutdown  chan struct{}
}

func (a *App) RequestShutdown() {
	select {
	case <-a.shutdown:
	default:
		close(a.shutdown)
	}
}

func New(opts Options) (*App, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		var err error
		dataDir, err = defaultDataDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("crear data dir: %w", err)
	}
	logsDir := filepath.Join(dataDir, "logs")
	logger, closer, err := logging.New(logsDir)
	if err != nil {
		return nil, err
	}
	logger = logger.With("version", opts.Version, "build", "native")

	cfg, err := config.Load(filepath.Join(dataDir, "config.yaml"))
	if err != nil {
		return nil, err
	}

	st, err := store.Open(filepath.Join(dataDir, "agent.db"))
	if err != nil {
		return nil, err
	}

	sumatraPath, err := printer.EnsureSumatra(filepath.Join(dataDir, "bin"))
	if err != nil {
		return nil, fmt.Errorf("extraer SumatraPDF: %w", err)
	}
	logger.Info("sumatra listo", "path", sumatraPath)

	pdfDir := filepath.Join(dataDir, "pdfs")
	if err := os.MkdirAll(pdfDir, 0o755); err != nil {
		return nil, err
	}

	printerSvc := printer.NewService(sumatraPath, logger.With("module", "printer"))
	q := queue.New(queue.Config{
		Store:         st,
		Printer:       printerSvc,
		Logger:        logger.With("module", "queue"),
		PDFDir:        pdfDir,
		MaxRetries:    cfg.Get().MaxRetries,
		RetentionDays: cfg.Get().RetentionDays,
	})

	mid := machineID(dataDir)
	srv := server.New(server.Config{
		Addr:      fmt.Sprintf("127.0.0.1:%d", cfg.Get().Port),
		Logger:    logger.With("module", "server"),
		Store:     st,
		Printer:   printerSvc,
		Queue:     q,
		Config:    cfg,
		Version:   opts.Version,
		MachineID: mid,
	})

	return &App{
		opts: opts, dataDir: dataDir, logger: logger, logCloser: closer,
		cfg: cfg, store: st, printer: printerSvc, queue: q, server: srv,
		machineID: mid, shutdown: make(chan struct{}),
	}, nil
}

func (a *App) Run(ctx context.Context) error {
	ok, err := acquireSingletonLock()
	if err != nil {
		a.logger.Warn("no se pudo verificar instancia única", "err", err)
	} else if !ok {
		a.logger.Info("ya hay otra instancia corriendo, salgo")
		if !a.opts.Headless {
			lang := i18n.Lang(a.cfg.Get().Language)
			notifyAlreadyRunning(i18n.T(lang, "running.title"), i18n.T(lang, "running.body"))
		}
		return nil
	}

	a.logger.Info("agente arrancando",
		"data_dir", a.dataDir, "machine_id", a.machineID, "headless", a.opts.Headless)

	// Alinear el registro de Windows con la config (Iniciar con Windows).
	// Se hace acá (y no al cambiar el toggle) porque si el usuario movió el .exe
	// a otra carpeta, queremos reescribir la entrada apuntando al path actual.
	if err := autostart.Sync(a.cfg.Get().AutoStart); err != nil {
		a.logger.Warn("autostart sync falló", "err", err)
	}

	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	go func() {
		select {
		case <-a.shutdown:
			a.logger.Info("apagado solicitado desde la UI")
			cancelRun()
		case <-runCtx.Done():
		}
	}()
	ctx = runCtx

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.queue.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("queue parada con error", "err", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := a.server.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.logger.Error("server parado con error", "err", err)
		}
	}()

	if !a.opts.Headless {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.runUI(ctx); err != nil {
				a.logger.Error("ui parada con error", "err", err)
			}
		}()
	}

	<-ctx.Done()
	a.logger.Info("apagado solicitado, drenando…")
	drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a.queue.Drain(drainCtx)
	wg.Wait()
	_ = a.store.Close()
	_ = a.logCloser.Close()
	return nil
}

func machineID(dataDir string) string {
	h := sha256.New()
	host, _ := os.Hostname()
	_, _ = h.Write([]byte(host))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(dataDir))
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func defaultDataDir() (string, error) {
	if runtime.GOOS == "windows" {
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "PrinklyPrint"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".prinkly"), nil
}
