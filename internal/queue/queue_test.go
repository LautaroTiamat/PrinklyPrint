package queue

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/lautarotiamat/prinklyprint/internal/crypto/dpapi"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

const samplePDF = "%PDF-1.4\n1 0 obj<<>>endobj\n%%EOF\n"

func testWorker(t *testing.T) (*Worker, *store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := os.MkdirAll(filepath.Join(dir, "pdfs"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := New(Config{
		Store:   st,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		PDFDir:  filepath.Join(dir, "pdfs"),
		TempDir: filepath.Join(dir, "tmp"),
	})
	return w, st, dir
}

func TestEnqueueRejectsNonPDF(t *testing.T) {
	w, _, _ := testWorker(t)
	b64 := base64.StdEncoding.EncodeToString([]byte("esto no es un PDF"))
	_, err := w.Enqueue(context.Background(), EnqueueParams{PDFBase64: b64})
	if !errors.Is(err, ErrNotPDF) {
		t.Fatalf("esperaba ErrNotPDF, obtuve %v", err)
	}
}

func TestEnqueueRequiresBase64(t *testing.T) {
	// pdf_base64 es el único camino: sin él, error claro (pdf_url se eliminó).
	w, _, _ := testWorker(t)
	if _, err := w.Enqueue(context.Background(), EnqueueParams{}); err == nil {
		t.Fatal("esperaba error cuando falta pdf_base64")
	}
}

func TestEnqueueWritesEncryptedFile(t *testing.T) {
	w, st, _ := testWorker(t)
	b64 := base64.StdEncoding.EncodeToString([]byte(samplePDF))
	id, err := w.Enqueue(context.Background(), EnqueueParams{PDFBase64: b64, Filename: "x.pdf"})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	j, err := st.GetJob(context.Background(), id)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if filepath.Ext(j.PDFPath) != ".enc" {
		t.Errorf("PDFPath debería terminar en .enc, es %q", j.PDFPath)
	}
	if _, err := os.Stat(j.PDFPath); err != nil {
		t.Errorf("el archivo .enc debería existir en disco: %v", err)
	}
}

func TestResolvePrintFileDecryptsAndDeletesTemp(t *testing.T) {
	w, _, dir := testWorker(t)
	// Escribimos un .enc como lo haría Enqueue (en el stub de Linux, Protect es
	// passthrough; en Windows real sería DPAPI).
	enc, err := dpapi.Protect([]byte(samplePDF))
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	encPath := filepath.Join(dir, "pdfs", "job1.pdf.enc")
	if err := os.WriteFile(encPath, enc, 0o600); err != nil {
		t.Fatal(err)
	}

	j := &store.Job{ID: "job1", PDFPath: encPath}
	path, cleanup, err := w.resolvePrintFile(j)
	if err != nil {
		t.Fatalf("resolvePrintFile: %v", err)
	}
	if filepath.Dir(path) != w.cfg.TempDir {
		t.Errorf("el temporal debería estar en TempDir (%q), está en %q", w.cfg.TempDir, filepath.Dir(path))
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("leer temporal: %v", err)
	}
	if string(got) != samplePDF {
		t.Errorf("el contenido descifrado no coincide con el original")
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("el temporal debería estar borrado tras cleanup (stat err=%v)", err)
	}
}

func TestResolvePrintFilePlainPassthrough(t *testing.T) {
	w, _, dir := testWorker(t)
	plain := filepath.Join(dir, "pdfs", "job2.pdf")
	if err := os.WriteFile(plain, []byte(samplePDF), 0o600); err != nil {
		t.Fatal(err)
	}
	j := &store.Job{ID: "job2", PDFPath: plain}
	path, cleanup, err := w.resolvePrintFile(j)
	if err != nil {
		t.Fatalf("resolvePrintFile: %v", err)
	}
	if path != plain {
		t.Errorf("un .pdf plano debería devolver el mismo path, got %q", path)
	}
	cleanup() // no-op para el plano
	if _, err := os.Stat(plain); err != nil {
		t.Errorf("el .pdf plano NO debería borrarse en cleanup: %v", err)
	}
}

func TestSweepTempDir(t *testing.T) {
	w, _, _ := testWorker(t)
	if err := os.MkdirAll(w.cfg.TempDir, 0o700); err != nil {
		t.Fatal(err)
	}
	// Simula PDFs en claro huérfanos de una corrida anterior interrumpida.
	for _, n := range []string{"a.pdf", "b.pdf"} {
		if err := os.WriteFile(filepath.Join(w.cfg.TempDir, n), []byte(samplePDF), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	w.sweepTempDir()
	entries, err := os.ReadDir(w.cfg.TempDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("el barrido debería dejar TempDir vacío, quedan %d archivos", len(entries))
	}
}

func TestSweepTempDirNoDir(t *testing.T) {
	w, _, _ := testWorker(t)
	// TempDir todavía no existe: no debe paniquear ni fallar.
	w.sweepTempDir()
}

func TestFailRetriesBeforeFailing(t *testing.T) {
	// max_retries cuenta reintentos DESPUÉS del intento inicial: con el default
	// (1), el primer fallo reencola con backoff y el segundo deja el job failed.
	w, st, _ := testWorker(t)
	ctx := context.Background()
	b64 := base64.StdEncoding.EncodeToString([]byte(samplePDF))
	id, err := w.Enqueue(ctx, EnqueueParams{PDFBase64: b64})
	if err != nil {
		t.Fatalf("Enqueue: %v", err)
	}

	j, err := st.GetJob(ctx, id)
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	w.fail(ctx, j, errors.New("boom"), "")
	j, _ = st.GetJob(ctx, id)
	if j.Status != store.StatusQueued {
		t.Fatalf("tras el primer fallo el job debería reencolarse, está %q", j.Status)
	}
	if j.Attempts != 1 {
		t.Fatalf("attempts tras el primer fallo = %d, quiero 1", j.Attempts)
	}

	w.fail(ctx, j, errors.New("boom"), "")
	j, _ = st.GetJob(ctx, id)
	if j.Status != store.StatusFailed {
		t.Fatalf("tras agotar el reintento el job debería quedar failed, está %q", j.Status)
	}
}

func TestSettingsHookOverridesStatics(t *testing.T) {
	// El hook Settings entrega valores en vivo; los campos estáticos son fallback.
	maxR, ret := 5, 30
	w := New(Config{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Settings: func() (int, int) { return maxR, ret },
	})
	if got := w.maxRetries(); got != 5 {
		t.Errorf("maxRetries() = %d, quiero 5", got)
	}
	if got := w.retentionDays(); got != 30 {
		t.Errorf("retentionDays() = %d, quiero 30", got)
	}
	maxR, ret = 2, 10 // simula un cambio desde la pestaña General, sin reiniciar
	if got := w.maxRetries(); got != 2 {
		t.Errorf("maxRetries() tras cambio = %d, quiero 2", got)
	}
	if got := w.retentionDays(); got != 10 {
		t.Errorf("retentionDays() tras cambio = %d, quiero 10", got)
	}
	// Valores fuera de rango del hook → fallback a los estáticos (defaults 1 y 7).
	maxR, ret = -1, 0
	if got := w.maxRetries(); got != 1 {
		t.Errorf("maxRetries() con hook inválido = %d, quiero fallback 1", got)
	}
	if got := w.retentionDays(); got != 7 {
		t.Errorf("retentionDays() con hook inválido = %d, quiero fallback 7", got)
	}
}

func TestLooksLikePDF(t *testing.T) {
	if !looksLikePDF([]byte("%PDF-1.7 ...")) {
		t.Error("un PDF válido debería pasar")
	}
	if looksLikePDF([]byte("PK\x03\x04")) {
		t.Error("un zip no debería pasar como PDF")
	}
	if looksLikePDF([]byte("%PDF")) {
		t.Error("prefijo incompleto no debería pasar")
	}
}
