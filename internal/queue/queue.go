// Package queue procesa los jobs persistidos en el [store] en orden FIFO.
//
// Diseño:
//
//   - Worker único: una sola goroutine principal poll-ea la DB cada PollInterval
//     y procesa de a un job. Mantener un único worker es intencional — la
//     mayoría de impresoras serializa internamente igual, y simplifica el
//     orden de impresión.
//
//   - Reintentos con backoff exponencial: cuando un job falla, se reencola
//     con un next_attempt_at futuro (default: 5s, 30s, 2min). Después de
//     MaxRetries fallidos, queda en estado "failed" y el operador puede
//     reintentar manualmente desde la UI.
//
//   - Pausa cooperativa: [Worker.Pause]/[Worker.Resume] permiten que el
//     operador detenga temporalmente el procesamiento sin matar el agente.
//
//   - Drain: [Worker.Drain] bloquea hasta que termine el job en curso o
//     expire el deadline del context. Se usa durante el apagado ordenado.
//
//   - Limpieza periódica: jobs done/failed/cancelled más viejos que
//     RetentionDays (default 7) se borran junto con sus PDFs en disco.
//
//   - Pre-flight check: antes de invocar SumatraPDF, consulta el estado
//     de la impresora ([printer.Service.CheckReady]). Si está sin tinta,
//     sin papel, offline, etc., falla rápido con mensaje claro en lugar de
//     gastar reintentos.
//
//   - Timeout de impresión: SumatraPDF se mata con context.WithTimeout
//     (default 60s). Evita que un Sumatra colgado bloquee toda la cola.
package queue

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

type Config struct {
	Store           *store.Store
	Printer         *printer.Service
	Logger          *slog.Logger
	PDFDir          string
	MaxRetries      int
	Backoffs        []time.Duration
	RetentionDays   int
	PrintTimeout    time.Duration
	PollInterval    time.Duration
	CleanupInterval time.Duration
	HTTPClient      *http.Client
}

type Worker struct {
	cfg    Config
	paused atomic.Bool
	busy   atomic.Bool
	mu     sync.Mutex
	done   chan struct{}
}

func New(cfg Config) *Worker {
	if cfg.PollInterval == 0 {
		cfg.PollInterval = 1 * time.Second
	}
	if cfg.CleanupInterval == 0 {
		cfg.CleanupInterval = 1 * time.Hour
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 1
	}
	if len(cfg.Backoffs) == 0 {
		cfg.Backoffs = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute}
	}
	if cfg.RetentionDays == 0 {
		cfg.RetentionDays = 7
	}
	if cfg.PrintTimeout == 0 {
		cfg.PrintTimeout = 60 * time.Second
	}
	return &Worker{cfg: cfg, done: make(chan struct{})}
}

func (w *Worker) Pause()         { w.paused.Store(true) }
func (w *Worker) Resume()        { w.paused.Store(false) }
func (w *Worker) IsPaused() bool { return w.paused.Load() }
func (w *Worker) IsBusy() bool   { return w.busy.Load() }

func (w *Worker) Run(ctx context.Context) error {
	defer close(w.done)
	pollTicker := time.NewTicker(w.cfg.PollInterval)
	defer pollTicker.Stop()
	cleanupTicker := time.NewTicker(w.cfg.CleanupInterval)
	defer cleanupTicker.Stop()

	if n, err := w.cfg.Store.RecoverStaleJobs(ctx); err != nil {
		w.cfg.Logger.Error("RecoverStaleJobs", "err", err)
	} else if n > 0 {
		w.cfg.Logger.Info("jobs recuperados tras reinicio", "n", n)
	}

	w.cleanup(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-cleanupTicker.C:
			w.cleanup(ctx)
		case <-pollTicker.C:
			if w.paused.Load() {
				continue
			}
			w.tickOnce(ctx)
		}
	}
}

func (w *Worker) Drain(ctx context.Context) {
	for w.busy.Load() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (w *Worker) tickOnce(ctx context.Context) {
	j, err := w.cfg.Store.NextDueJob(ctx, time.Now().UTC())
	if err != nil {
		w.cfg.Logger.Error("NextDueJob", "err", err)
		return
	}
	if j == nil {
		return
	}
	w.busy.Store(true)
	defer w.busy.Store(false)
	w.processJob(ctx, j)
}

func (w *Worker) processJob(ctx context.Context, j *store.Job) {
	log := w.cfg.Logger.With("job_id", j.ID, "filename", j.Filename, "attempt", j.Attempts+1)
	log.Info("procesando job")

	var opts printer.Options
	if err := json.Unmarshal([]byte(j.OptionsJSON), &opts); err != nil {
		w.fail(ctx, j, fmt.Errorf("opciones inválidas: %w", err), "")
		return
	}
	if opts.Printer == "" {
		opts.Printer = j.Printer
	}

	if _, err := os.Stat(j.PDFPath); errors.Is(err, os.ErrNotExist) {
		w.fail(ctx, j, fmt.Errorf("pdf no encontrado en disco: %s", j.PDFPath), "")
		return
	}

	if err := w.cfg.Printer.CheckReady(ctx, opts.Printer); err != nil {
		log.Warn("pre-flight check rechazó la impresora", "err", err)
		w.fail(ctx, j, err, "")
		return
	}

	printCtx, cancel := context.WithTimeout(ctx, w.cfg.PrintTimeout)
	defer cancel()
	res, err := w.cfg.Printer.Print(printCtx, j.PDFPath, opts)
	sumatraLog := ""
	if res != nil {
		sumatraLog = fmt.Sprintf("exit=%d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr)
	}
	if errors.Is(printCtx.Err(), context.DeadlineExceeded) {
		err = fmt.Errorf("SumatraPDF excedió timeout de %s y fue terminado", w.cfg.PrintTimeout)
	}
	if err == nil {
		if err := w.cfg.Store.MarkDone(ctx, j.ID, sumatraLog); err != nil {
			log.Error("MarkDone", "err", err)
		}
		log.Info("job ok")
		return
	}
	log.Warn("SumatraPDF falló", "err", err)
	w.fail(ctx, j, err, sumatraLog)
}

func (w *Worker) fail(ctx context.Context, j *store.Job, lastErr error, sumatraLog string) {
	attempts := j.Attempts + 1
	if attempts >= w.cfg.MaxRetries {
		if err := w.cfg.Store.MarkFailed(ctx, j.ID, attempts, lastErr.Error(), sumatraLog); err != nil {
			w.cfg.Logger.Error("MarkFailed", "err", err)
		}
		return
	}
	idx := attempts - 1
	if idx >= len(w.cfg.Backoffs) {
		idx = len(w.cfg.Backoffs) - 1
	}
	next := time.Now().UTC().Add(w.cfg.Backoffs[idx])
	if err := w.cfg.Store.RequeueForRetry(ctx, j.ID, attempts, lastErr.Error(), sumatraLog, next); err != nil {
		w.cfg.Logger.Error("RequeueForRetry", "err", err)
	}
}

func (w *Worker) cleanup(ctx context.Context) {
	cutoff := time.Now().UTC().AddDate(0, 0, -w.cfg.RetentionDays)
	paths, err := w.cfg.Store.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		w.cfg.Logger.Error("cleanup", "err", err)
		return
	}
	for _, p := range paths {
		_ = os.Remove(p)
	}
	if len(paths) > 0 {
		w.cfg.Logger.Info("limpieza", "borrados", len(paths))
	}
}

type EnqueueParams struct {
	PDFBase64 string
	PDFURL    string
	Filename  string
	Options   printer.Options
	Metadata  map[string]any
}

func (w *Worker) Enqueue(ctx context.Context, p EnqueueParams) (string, error) {
	if p.PDFBase64 == "" && p.PDFURL == "" {
		return "", fmt.Errorf("pdf_base64 o pdf_url requerido")
	}
	if p.PDFBase64 != "" && p.PDFURL != "" {
		return "", fmt.Errorf("pdf_base64 y pdf_url son mutuamente excluyentes")
	}

	id := uuid.NewString()
	pdfPath := filepath.Join(w.cfg.PDFDir, id+".pdf")

	if p.PDFBase64 != "" {
		raw, err := base64.StdEncoding.DecodeString(p.PDFBase64)
		if err != nil {
			return "", fmt.Errorf("base64 inválido: %w", err)
		}
		if err := os.WriteFile(pdfPath, raw, 0o644); err != nil {
			return "", fmt.Errorf("escribir pdf: %w", err)
		}
	} else {
		if err := w.downloadPDF(ctx, p.PDFURL, pdfPath); err != nil {
			return "", err
		}
	}

	optsJSON, _ := json.Marshal(p.Options)
	metaJSON, _ := json.Marshal(p.Metadata)
	filename := p.Filename
	if filename == "" {
		filename = id + ".pdf"
	}
	job := store.Job{
		ID: id, Filename: filename, Printer: p.Options.Printer,
		OptionsJSON: string(optsJSON), MetadataJSON: string(metaJSON),
		PDFPath: pdfPath, Status: store.StatusQueued,
	}
	if err := w.cfg.Store.CreateJob(ctx, job); err != nil {
		_ = os.Remove(pdfPath)
		return "", err
	}
	w.cfg.Logger.Info("job encolado", "job_id", id, "filename", filename)
	return id, nil
}

func (w *Worker) downloadPDF(ctx context.Context, url, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("new request: %w", err)
	}
	resp, err := w.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("descargar pdf: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("descargar pdf: status %d", resp.StatusCode)
	}
	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("crear pdf: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("copiar pdf: %w", err)
	}
	return nil
}
