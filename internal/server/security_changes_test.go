package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/config"
)

// ── /ping no expone machine_id ────────────────────────────────────────────────

func TestHandlePing_OmitsMachineID(t *testing.T) {
	s := testPrintServer(t)
	s.cfg.Version = "9.9.9"
	s.cfg.MachineID = "machine-id-no-deberia-aparecer"

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	s.handlePing(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, quiero 200", rec.Code)
	}
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("parsear body: %v", err)
	}
	if _, ok := m["machine_id"]; ok {
		t.Error("/ping NO debería incluir machine_id (endpoint sin token)")
	}
	if m["ok"] != true {
		t.Errorf("falta ok=true, body=%v", m)
	}
	if m["version"] != "9.9.9" {
		t.Errorf("version=%v, quiero 9.9.9", m["version"])
	}
	if _, ok := m["paused"]; !ok {
		t.Error("falta paused")
	}
}

func TestHandleGetSettings_ExposesMachineID(t *testing.T) {
	// machine_id se sigue pudiendo obtener, pero en /settings (que exige token).
	s := testPrintServer(t)
	s.cfg.MachineID = "abc123"
	req := httptest.NewRequest(http.MethodGet, "/settings", nil)
	rec := httptest.NewRecorder()
	s.handleGetSettings(rec, req)
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("parsear: %v", err)
	}
	if m["machine_id"] != "abc123" {
		t.Errorf("/settings debería exponer machine_id, body=%v", m)
	}
}

// ── token bucket (unidad) ─────────────────────────────────────────────────────

func TestTokenBucket_BurstThenLimit(t *testing.T) {
	now := time.Unix(1000, 0)
	b := newTokenBucket(60, 3, func() time.Time { return now }) // 60/min = 1/s, burst 3

	for i := 0; i < 3; i++ {
		if !b.allow() {
			t.Fatalf("la llamada %d (dentro del burst) debería pasar", i+1)
		}
	}
	if b.allow() {
		t.Fatal("la 4ª llamada inmediata debería limitarse (bucket vacío)")
	}
	// Avanzo 1s → 1 token nuevo.
	now = now.Add(1 * time.Second)
	if !b.allow() {
		t.Fatal("tras 1s debería permitir 1 más")
	}
	if b.allow() {
		t.Fatal("y volver a limitarse")
	}
}

func TestTokenBucket_ReconfigureClampsDown(t *testing.T) {
	now := time.Unix(0, 0)
	b := newTokenBucket(60, 10, func() time.Time { return now }) // arranca con 10 tokens
	b.reconfigure(60, 3)                                         // baja el burst a 3 → recorta a 3

	got := 0
	for b.allow() { // reloj congelado: no hay recarga
		got++
		if got > 50 {
			break
		}
	}
	if got != 3 {
		t.Fatalf("tras bajar el burst a 3 deberían quedar 3 tokens, hubo %d", got)
	}
}

// ── rate limit en /pair ───────────────────────────────────────────────────────

func TestHandlePair_RateLimited(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	if err := cm.Update(func(c *config.Config) {
		c.PairRateLimitEnabled = true
		c.PairRateLimitPerMinute = 60 // 1/s
		c.PairRateLimitBurst = 2
		c.AllowedOrigins = []string{"https://ok.example"} // pre-aprobado: 200 sin diálogo
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := newPairServer(store, cm, &fakePrompter{interactive: true})
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now } // reloj congelado: sin recarga

	for i := 0; i < 2; i++ {
		if rec := doPair(s, "https://ok.example"); rec.Code != http.StatusOK {
			t.Fatalf("llamada %d: code=%d, quiero 200 (dentro del burst)", i+1, rec.Code)
		}
	}
	rec := doPair(s, "https://ok.example")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3ª llamada: code=%d, quiero 429", rec.Code)
	}
	if got := bodyMap(t, rec)["error"]; got != "rate_limited" {
		t.Errorf("error=%q, quiero rate_limited", got)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("falta el header Retry-After en el 429")
	}

	// Avanzo 1s (60/min = 1/s) → 1 token → vuelve a permitir.
	now = now.Add(1 * time.Second)
	if rec := doPair(s, "https://ok.example"); rec.Code != http.StatusOK {
		t.Fatalf("tras 1s: code=%d, quiero 200", rec.Code)
	}
}

func TestHandlePair_RateLimitDisabledNeverLimits(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t) // enabled = false por default
	if err := cm.Update(func(c *config.Config) { c.AllowedOrigins = []string{"https://ok.example"} }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := newPairServer(store, cm, &fakePrompter{interactive: true})
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }

	for i := 0; i < 50; i++ {
		if rec := doPair(s, "https://ok.example"); rec.Code != http.StatusOK {
			t.Fatalf("rate limit desactivado: llamada %d code=%d, quiero 200 (nunca 429)", i+1, rec.Code)
		}
	}
}

// TestHandlePrint_NotRateLimited confirma que el rate limit de pairing NUNCA
// aplica a /print, aunque esté activado (con burst mínimo).
func TestHandlePrint_NotRateLimited(t *testing.T) {
	s := testPrintServer(t)
	if err := s.cfg.Config.Update(func(c *config.Config) {
		c.PairRateLimitEnabled = true
		c.PairRateLimitPerMinute = 1
		c.PairRateLimitBurst = 1
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	now := time.Unix(0, 0)
	s.now = func() time.Time { return now }

	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodPost, "/print", strings.NewReader(`{}`))
		rec := httptest.NewRecorder()
		s.handlePrint(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			t.Fatalf("/print NO debe estar sujeto al rate limit de pairing (llamada %d dio 429)", i+1)
		}
		// Cae en 400 bad_request (falta pdf_base64); nunca 429 por rate limit.
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("/print llamada %d: code=%d, esperaba 400", i+1, rec.Code)
		}
	}
}
