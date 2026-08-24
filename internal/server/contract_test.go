package server

// Contract test del API HTTP: fija la FORMA (claves + tipos JSON) de cada
// respuesta del agente en contract/api-shapes.json, generándola desde los
// handlers reales. La librería cliente (PrinklyPrint.js) tiene una copia de
// ese archivo y un test espejo que valida sus tipos TypeScript contra él —
// así el drift Go↔TS (como el traslado de machine_id de /ping a /settings)
// rompe un test en vez de descubrirse en producción.
//
// Si cambiaste el API a propósito:
//  1. CONTRACT_UPDATE=1 go test ./internal/server -run TestAPIContract
//  2. Copiá contract/api-shapes.json a PrinklyPrint.js/test/contract/
//  3. Ajustá los tipos/samples del lado JS hasta que su test pase.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
)

const contractPath = "../../contract/api-shapes.json"

// shapeOf reduce un valor JSON decodificado a su forma canónica: los objetos
// conservan sus claves (con la forma de cada valor), los arrays la forma de su
// primer elemento, y los escalares se reemplazan por el nombre de su tipo.
func shapeOf(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = shapeOf(val)
		}
		return out
	case []any:
		if len(x) == 0 {
			return []any{}
		}
		return []any{shapeOf(x[0])}
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func decodeBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) any {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status=%d, quiero %d (body=%s)", rec.Code, wantStatus, rec.Body.String())
	}
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("body no es JSON: %v (%s)", err, rec.Body.String())
	}
	return v
}

func TestAPIContract(t *testing.T) {
	s := testPrintServer(t)
	s.cfg.Auth = testAuth(t)
	s.cfg.Version = "0.0.0-contract"
	s.cfg.MachineID = "contract00000000"

	shapes := map[string]any{}

	// GET /ping
	{
		rec := httptest.NewRecorder()
		s.handlePing(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
		shapes["GET /ping"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// GET /settings
	{
		rec := httptest.NewRecorder()
		s.handleGetSettings(rec, httptest.NewRequest(http.MethodGet, "/settings", nil))
		shapes["GET /settings"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// GET /printers — elemento. En Linux/CI no hay spooler, así que la forma
	// sale de serializar el struct printer.Printer con TODOS los campos seteados
	// (los omitempty incluidos): es el mismo struct que el handler serializa.
	{
		b, err := json.Marshal(printer.Printer{
			Name: "x", IsDefault: true, IsNetwork: true, Status: "ready",
			Statuses: []string{"ready"}, Severity: "ok", PortName: "p",
			DriverName: "d", Location: "l", Comment: "c", JobCount: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		var v any
		_ = json.Unmarshal(b, &v)
		shapes["GET /printers[]"] = shapeOf(v)
	}

	// POST /print (y de paso obtenemos un job para /jobs y /jobs/{id})
	pdfB64 := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4\n1 0 obj<<>>endobj\n%%EOF\n"))
	var jobID string
	{
		body := fmt.Sprintf(`{"pdf_base64":%q,"filename":"contract.pdf","options":{"copies":1},"metadata":{"origen":"contract"}}`, pdfB64)
		req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.handlePrint(rec, req)
		v := decodeBody(t, rec, http.StatusAccepted)
		shapes["POST /print"] = shapeOf(v)
		jobID = v.(map[string]any)["job_id"].(string)
	}

	// GET /jobs
	{
		rec := httptest.NewRecorder()
		s.handleListJobs(rec, httptest.NewRequest(http.MethodGet, "/jobs", nil))
		shapes["GET /jobs"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// GET /jobs/{id} — job recién encolado (sin completed_at/next_attempt_at,
	// que solo aparecen cuando el job terminó o espera un reintento).
	{
		req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID, nil)
		req.SetPathValue("id", jobID)
		rec := httptest.NewRecorder()
		s.handleGetJob(rec, req)
		shapes["GET /jobs/{id}"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// DELETE /jobs/{id} (cancelar el job queued)
	{
		req := httptest.NewRequest(http.MethodDelete, "/jobs/"+jobID, nil)
		req.SetPathValue("id", jobID)
		rec := httptest.NewRecorder()
		s.handleCancelJob(rec, req)
		shapes["DELETE /jobs/{id}"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// POST /jobs/{id}/retry (necesita un job failed)
	{
		if err := s.cfg.Store.MarkFailed(t.Context(), jobID, 1, "contract", ""); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/jobs/"+jobID+"/retry", nil)
		req.SetPathValue("id", jobID)
		rec := httptest.NewRecorder()
		s.handleRetryJob(rec, req)
		shapes["POST /jobs/{id}/retry"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// POST /pair — con el origen pre-aprobado (camino sin diálogo).
	{
		if err := s.cfg.Config.Update(func(c *config.Config) {
			c.AllowedOrigins = append(c.AllowedOrigins, "http://localhost:9")
		}); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"label":"contract"}`))
		req.Header.Set("Origin", "http://localhost:9")
		rec := httptest.NewRecorder()
		s.handlePair(rec, req)
		shapes["POST /pair"] = shapeOf(decodeBody(t, rec, http.StatusOK))
	}

	// Cuerpo de error (writeErr): cualquier 4xx/5xx tiene esta forma.
	{
		req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		s.handlePrint(rec, req)
		shapes["error"] = shapeOf(decodeBody(t, rec, http.StatusBadRequest))
	}

	got, err := json.MarshalIndent(shapes, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')

	if os.Getenv("CONTRACT_UPDATE") == "1" {
		if err := os.MkdirAll(filepath.Dir(contractPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(contractPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("contrato regenerado en %s — copialo a PrinklyPrint.js/test/contract/", contractPath)
		return
	}

	want, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("no pude leer %s (¿primera vez? corré con CONTRACT_UPDATE=1): %v", contractPath, err)
	}
	if string(want) != string(got) {
		t.Errorf("el API cambió de forma respecto de contract/api-shapes.json.\n"+
			"Si el cambio es intencional: CONTRACT_UPDATE=1 go test ./internal/server -run TestAPIContract, "+
			"copiá el archivo a PrinklyPrint.js/test/contract/ y actualizá los tipos TS.\n--- contrato ---\n%s\n--- actual ---\n%s", want, got)
	}
}
