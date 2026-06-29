package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

// testPrintServer arma un Server con Store + Queue reales (los únicos deps que
// toca handlePrint), suficiente para ejercitar /print a nivel HTTP. SecLog queda
// nil: sus métodos son nil-safe.
func testPrintServer(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	pdfDir := filepath.Join(dir, "pdfs")
	if err := os.MkdirAll(pdfDir, 0o755); err != nil {
		t.Fatal(err)
	}
	q := queue.New(queue.Config{
		Store:   st,
		Logger:  testLogger(),
		PDFDir:  pdfDir,
		TempDir: filepath.Join(dir, "tmp"),
	})
	return &Server{cfg: Config{
		Store:  st,
		Queue:  q,
		Config: testConfig(t),
		Logger: testLogger(),
	}}
}

// TestHandlePrint_RequiresPDFBase64 fija que /print exige pdf_base64 y que
// pdf_url ya no es un camino: mandarlo no descarga nada — al faltar pdf_base64
// el request termina en 400. (pdf_url se eliminó: superficie SSRF sin uso.)
func TestHandlePrint_RequiresPDFBase64(t *testing.T) {
	s := testPrintServer(t)
	cases := []struct {
		name string
		body string
	}{
		{"body vacío", `{}`},
		{"solo filename", `{"filename":"x.pdf"}`},
		{"pdf_url ignorado (ya no soportado)", `{"pdf_url":"http://169.254.169.254/latest/meta-data/"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(c.body))
			rec := httptest.NewRecorder()
			s.handlePrint(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("code=%d, quiero 400 (body=%s)", rec.Code, c.body)
			}
			if got := bodyMap(t, rec)["error"]; got != "bad_request" {
				t.Errorf("error=%q, quiero bad_request", got)
			}
		})
	}
}
