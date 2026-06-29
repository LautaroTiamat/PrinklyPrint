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
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/lautarotiamat/prinklyprint/internal/crypto/dpapi"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/store"
	"github.com/lautarotiamat/prinklyprint/internal/winfs"
)

// ErrNotPDF se devuelve cuando el contenido decodificado no parece un PDF
// (magic bytes "%PDF-" ausentes). Es un error de input del cliente → 400.
var ErrNotPDF = errors.New("el contenido no parece un PDF (faltan los magic bytes %PDF-)")

// maxSumatraLogBytes acota lo que persistimos del stdout/stderr de SumatraPDF
// en la DB: es data de debug que puede filtrar rutas/contenido.
const maxSumatraLogBytes = 4 << 10

// looksLikePDF chequea el magic prefix de un PDF: es la primera línea de defensa
// de /print, sobre los bytes decodificados del base64 antes de cifrar y encolar.
func looksLikePDF(b []byte) bool {
	return len(b) >= 5 && string(b[:5]) == "%PDF-"
}

func truncateForLog(s string) string {
	if len(s) <= maxSumatraLogBytes {
		return s
	}
	// ToValidUTF8 descarta una secuencia multibyte cortada al medio por el slice,
	// para no persistir bytes inválidos en SQLite/JSON.
	return strings.ToValidUTF8(s[:maxSumatraLogBytes], "") + "…[truncado]"
}

type Config struct {
	Store           *store.Store
	Printer         *printer.Service
	Logger          *slog.Logger
	PDFDir          string
	TempDir         string // dir para los PDFs descifrados temporales de impresión
	MaxRetries      int
	Backoffs        []time.Duration
	RetentionDays   int
	PrintTimeout    time.Duration
	PollInterval    time.Duration
	CleanupInterval time.Duration
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
	if cfg.TempDir == "" {
		cfg.TempDir = filepath.Join(os.TempDir(), "prinklyprint-print")
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

	// Barrido de temporales huérfanos: los PDFs descifrados en TempDir nunca
	// deben sobrevivir entre ejecuciones (worker único, FIFO). Si una corrida
	// anterior murió a mitad de impresión (crash/kill/corte de luz), el defer
	// cleanup no corrió y quedó un PDF EN CLARO; lo scrubbeamos y borramos acá.
	w.sweepTempDir()

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

	if err := w.cfg.Printer.CheckReady(ctx, opts.Printer); err != nil {
		log.Warn("pre-flight check rechazó la impresora", "err", err)
		w.fail(ctx, j, err, "")
		return
	}

	// Descifra el PDF (si está cifrado) a un temporal plano con ACL restrictiva,
	// solo para imprimir; se borra apenas SumatraPDF termina.
	printPath, cleanup, err := w.resolvePrintFile(j)
	if err != nil {
		w.fail(ctx, j, err, "")
		return
	}
	defer cleanup()

	printCtx, cancel := context.WithTimeout(ctx, w.cfg.PrintTimeout)
	defer cancel()
	res, err := w.cfg.Printer.Print(printCtx, printPath, opts)
	sumatraLog := ""
	if res != nil {
		// Truncamos el log de SumatraPDF: es data de debug que puede filtrar
		// rutas/contenido (ver maxSumatraLogBytes).
		sumatraLog = truncateForLog(fmt.Sprintf("exit=%d\nstdout:\n%s\nstderr:\n%s", res.ExitCode, res.Stdout, res.Stderr))
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

// resolvePrintFile devuelve la ruta del archivo PLANO a pasarle a SumatraPDF y
// una función de limpieza.
//
//   - Job cifrado ("<id>.pdf.enc", el único camino): lo lee, lo descifra con
//     DPAPI a memoria y escribe un temporal plano en TempDir con ACL restrictiva.
//     El cleanup lo sobreescribe (best-effort) y lo borra. Existe una ventana de
//     plano inevitable mientras SumatraPDF lee el archivo, mitigada por la ACL
//     owner-only + el borrado inmediato.
//   - Fallback defensivo: un "<id>.pdf" plano (ya no hay ninguna rama que lo
//     escriba, pero por las dudas) se imprime directo. cleanup es no-op.
//
// Si el descifrado falla (blob de otro usuario/equipo), devuelve error: el job
// falla con mensaje claro. Es comportamiento esperado y deseable.
func (w *Worker) resolvePrintFile(j *store.Job) (path string, cleanup func(), err error) {
	noop := func() {}

	if !strings.HasSuffix(j.PDFPath, ".enc") {
		if _, statErr := os.Stat(j.PDFPath); statErr != nil {
			return "", noop, fmt.Errorf("pdf no encontrado en disco: %s", j.PDFPath)
		}
		return j.PDFPath, noop, nil
	}

	enc, err := os.ReadFile(j.PDFPath)
	if err != nil {
		return "", noop, fmt.Errorf("leer pdf cifrado: %w", err)
	}
	plain, err := dpapi.Unprotect(enc)
	if err != nil {
		return "", noop, fmt.Errorf("no se pudo descifrar el PDF en reposo (¿blob de otro usuario/equipo?): %w", err)
	}

	if err := os.MkdirAll(w.cfg.TempDir, 0o700); err != nil {
		return "", noop, fmt.Errorf("crear temp dir de impresión: %w", err)
	}
	_ = winfs.Restrict(w.cfg.TempDir)

	tmp := filepath.Join(w.cfg.TempDir, uuid.NewString()+".pdf")
	if err := os.WriteFile(tmp, plain, 0o600); err != nil { // #nosec G703 -- el nombre es un UUID generado por el servidor (uuid.NewString()), sin componente controlable por el usuario; filepath.Join lo confina a TempDir (ruta interna). No hay traversal alcanzable.
		return "", noop, fmt.Errorf("escribir pdf temporal: %w", err)
	}
	_ = winfs.Restrict(tmp)
	// Zeroizamos la copia en memoria del PDF en claro: ya está en el temporal y
	// no la necesitamos más (higiene; reduce la exposición en un dump de memoria).
	for i := range plain {
		plain[i] = 0
	}

	cleanup = func() {
		// Sobreescritura best-effort para reducir el residuo en disco antes de borrar.
		if fi, e := os.Stat(tmp); e == nil && fi.Size() > 0 {
			_ = os.WriteFile(tmp, make([]byte, fi.Size()), 0o600)
		}
		_ = os.Remove(tmp)
	}
	return tmp, cleanup, nil
}

// sweepTempDir borra (con scrub best-effort) cualquier PDF temporal huérfano
// que haya quedado de una corrida anterior interrumpida. Seguro de purgar todo:
// el worker es único y los temporales no deben persistir entre ejecuciones.
func (w *Worker) sweepTempDir() {
	entries, err := os.ReadDir(w.cfg.TempDir)
	if err != nil {
		return // el dir aún no existe: nada que barrer
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		p := filepath.Join(w.cfg.TempDir, e.Name())
		if fi, serr := os.Stat(p); serr == nil && fi.Size() > 0 {
			_ = os.WriteFile(p, make([]byte, fi.Size()), 0o600) // scrub best-effort
		}
		_ = os.Remove(p)
	}
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
	Filename  string
	Options   printer.Options
	Metadata  map[string]any
}

func (w *Worker) Enqueue(ctx context.Context, p EnqueueParams) (string, error) {
	// pdf_base64 es el ÚNICO camino: los PDF se generan en el cliente y se mandan
	// inline. No hay descarga remota — pdf_url se eliminó por completo (era
	// superficie SSRF sin caso de uso legítimo). El agente no hace ninguna
	// conexión saliente de red.
	if p.PDFBase64 == "" {
		return "", fmt.Errorf("pdf_base64 es requerido (pdf_url ya no está soportado)")
	}

	id := uuid.NewString()

	raw, err := base64.StdEncoding.DecodeString(p.PDFBase64)
	if err != nil {
		return "", fmt.Errorf("base64 inválido: %w", err)
	}
	// Validación de entrada: tiene que parecer un PDF (magic bytes). Evita
	// encolar basura y es la primera línea de defensa de /print.
	if !looksLikePDF(raw) {
		return "", ErrNotPDF
	}
	// Datos en reposo: ciframos el contenido con DPAPI (scope de usuario) y
	// lo guardamos como "<id>.pdf.enc". En disco es un blob cifrado, no un
	// PDF legible. Se descifra a un temporal solo al imprimir (ver processJob).
	enc, err := dpapi.Protect(raw)
	if err != nil {
		return "", fmt.Errorf("cifrar pdf en reposo: %w", err)
	}
	pdfPath := filepath.Join(w.cfg.PDFDir, id+".pdf.enc")
	if err := os.WriteFile(pdfPath, enc, 0o600); err != nil {
		return "", fmt.Errorf("escribir pdf: %w", err)
	}
	_ = winfs.Restrict(pdfPath) // owner-only, best-effort

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
