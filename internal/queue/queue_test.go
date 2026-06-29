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
