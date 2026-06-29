package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lautarotiamat/prinklyprint/internal/auth"
	"github.com/lautarotiamat/prinklyprint/internal/config"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func testAuth(t *testing.T) *auth.Store {
	t.Helper()
	s, err := auth.Load(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("auth.Load: %v", err)
	}
	return s
}

func testConfig(t *testing.T) *config.Manager {
	t.Helper()
	cm, err := config.Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cm
}

// okHandler responde 200 — sirve para detectar si un request pasó los middlewares.
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

type fakePrompter struct {
	approve     bool
	interactive bool
	calls       int
	lastOrigin  string
	lastLabel   string
}

func (f *fakePrompter) Confirm(origin, label string) bool {
	f.calls++
	f.lastOrigin = origin
	f.lastLabel = label
	return f.approve
}

func (f *fakePrompter) Interactive() bool { return f.interactive }

func containsOrigin(origins []string, want string) bool {
	for _, o := range origins {
		if o == want {
			return true
		}
	}
	return false
}

func TestBearerToken(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc", "abc", true},
		{"bearer abc", "abc", true},
		{"Bearer   abc  ", "abc", true},
		{"Bearer ", "", false},
		{"", "", false},
		{"Basic abc", "", false},
		{"abc", "", false},
	}
	for _, c := range cases {
		got, ok := bearerToken(c.header)
		if got != c.want || ok != c.ok {
			t.Errorf("bearerToken(%q) = (%q,%v), quiero (%q,%v)", c.header, got, ok, c.want, c.ok)
		}
	}
}

func TestRequireToken_SensitiveEndpointsNeedToken(t *testing.T) {
	store := testAuth(t)
	s := newPairServer(store, testConfig(t), nil)
	h := s.requireToken(okHandler())

	sensitive := []struct{ method, path string }{
		{http.MethodGet, "/printers"},
		{http.MethodGet, "/settings"},
		{http.MethodPost, "/print"},
		{http.MethodGet, "/jobs"},
		{http.MethodGet, "/jobs/abc"},
		{http.MethodPost, "/jobs/abc/retry"},
		{http.MethodDelete, "/jobs/abc"},
	}

	for _, tc := range sensitive {
		// Sin Authorization → 401, tenga o no header Origin.
		for _, origin := range []string{"", "https://evil.example"} {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s (origin=%q) sin token: code=%d, quiero 401", tc.method, tc.path, origin, rec.Code)
			}
		}
		// Token inválido → 401.
		req := httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer token-falso")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s token inválido: code=%d, quiero 401", tc.method, tc.path, rec.Code)
		}
		// Token válido → 200.
		req = httptest.NewRequest(tc.method, tc.path, nil)
		req.Header.Set("Authorization", "Bearer "+store.GetToken())
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s token válido: code=%d, quiero 200", tc.method, tc.path, rec.Code)
		}
	}
}

func TestRequireToken_ExemptEndpoints(t *testing.T) {
	s := newPairServer(testAuth(t), testConfig(t), nil)
	h := s.requireToken(okHandler())
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/ping"},
		{http.MethodPost, "/pair"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("%s %s sin token: code=%d, quiero 200 (exento)", tc.method, tc.path, rec.Code)
		}
	}
}

// TestRequireToken_BrowserOriginMustBeApproved verifica que, además del token,
// un request del navegador exige que su Origin siga aprobado: quitar el origen
// de la lista revoca el acceso aunque la app conserve el token. Los callers sin
// Origin (curl/Node) se gatean solo por token.
func TestRequireToken_BrowserOriginMustBeApproved(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	if err := cm.Update(func(c *config.Config) { c.AllowedOrigins = []string{"https://ok.example"} }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	s := newPairServer(store, cm, nil)
	h := s.requireToken(okHandler())
	tok := "Bearer " + store.GetToken()

	do := func(origin string) int {
		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		req.Header.Set("Authorization", tok)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := do("https://ok.example"); code != http.StatusOK {
		t.Errorf("origen aprobado + token: code=%d, quiero 200", code)
	}
	if code := do("https://revocada.example"); code != http.StatusUnauthorized {
		t.Errorf("origen NO aprobado + token: code=%d, quiero 401", code)
	}
	if code := do(""); code != http.StatusOK {
		t.Errorf("sin Origin (curl/Node) + token: code=%d, quiero 200", code)
	}
}

func newPairServer(store *auth.Store, cm *config.Manager, p PairingPrompter) *Server {
	return &Server{cfg: Config{Auth: store, Config: cm, Prompter: p, Logger: testLogger()}}
}

func doPair(s *Server, origin string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"label":"Test App"}`))
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	s.handlePair(rec, req)
	return rec
}

func bodyMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("parsear body: %v (body=%s)", err, rec.Body.String())
	}
	return m
}

func TestPair_PreApprovedOriginReturnsTokenWithoutDialog(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	if err := cm.Update(func(c *config.Config) { c.AllowedOrigins = []string{"https://approved.example"} }); err != nil {
		t.Fatalf("Update: %v", err)
	}
	p := &fakePrompter{interactive: true}
	s := newPairServer(store, cm, p)

	rec := doPair(s, "https://approved.example")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, quiero 200", rec.Code)
	}
	if p.calls != 0 {
		t.Errorf("no debería mostrarse diálogo para un origen pre-aprobado (calls=%d)", p.calls)
	}
	if got := bodyMap(t, rec)["token"]; got != store.GetToken() {
		t.Errorf("token=%q, quiero %q", got, store.GetToken())
	}
}

func TestPair_InteractiveApproveAddsAllowedOriginAndReturnsToken(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	p := &fakePrompter{approve: true, interactive: true}
	s := newPairServer(store, cm, p)

	const origin = "https://nueva.example"
	rec := doPair(s, origin)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, quiero 200", rec.Code)
	}
	if p.calls != 1 {
		t.Errorf("debería mostrarse el diálogo una vez (calls=%d)", p.calls)
	}
	if p.lastLabel != "Test App" {
		t.Errorf("label=%q, quiero \"Test App\"", p.lastLabel)
	}
	if !containsOrigin(cm.Get().AllowedOrigins, origin) {
		t.Error("el origen aprobado debería quedar en allowed_origins (lista visible en la UI)")
	}
	if got := bodyMap(t, rec)["token"]; got != store.GetToken() {
		t.Errorf("token=%q, quiero %q", got, store.GetToken())
	}
}

func TestPair_InteractiveDenyReturns403(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	p := &fakePrompter{approve: false, interactive: true}
	s := newPairServer(store, cm, p)

	const origin = "https://rechazada.example"
	rec := doPair(s, origin)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, quiero 403", rec.Code)
	}
	if got := bodyMap(t, rec)["error"]; got != "pairing_denied" {
		t.Errorf("error=%q, quiero pairing_denied", got)
	}
	if containsOrigin(cm.Get().AllowedOrigins, origin) {
		t.Error("un origen rechazado no debería quedar en allowed_origins")
	}
}

func TestPair_HeadlessNonInteractiveReturns403(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	p := &fakePrompter{interactive: false} // headless: sin UI
	s := newPairServer(store, cm, p)

	rec := doPair(s, "https://nueva.example")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, quiero 403", rec.Code)
	}
	if p.calls != 0 {
		t.Errorf("en headless no debería intentarse el diálogo (calls=%d)", p.calls)
	}
	if got := bodyMap(t, rec)["error"]; got != "pairing_denied" {
		t.Errorf("error=%q, quiero pairing_denied", got)
	}
}

func TestPair_MissingOriginReturns403(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	p := &fakePrompter{interactive: true}
	s := newPairServer(store, cm, p)

	rec := doPair(s, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, quiero 403", rec.Code)
	}
	if got := bodyMap(t, rec)["error"]; got != "pairing_denied" {
		t.Errorf("error=%q, quiero pairing_denied", got)
	}
}

func TestCORS_ReflectsAnyOrigin(t *testing.T) {
	// CORS es permisivo: cualquier origen pasa y recibe ACAO. El control real es
	// el token (ver TestStack_TokenIsTheGate), no CORS.
	h := cors(okHandler())
	const origin = "https://cualquiera.example"

	req := httptest.NewRequest(http.MethodGet, "/printers", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cualquier origen debería pasar CORS: code=%d, quiero 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO=%q, quiero %q", got, origin)
	}
}

func TestCORS_EmptyOriginPassesThrough(t *testing.T) {
	h := cors(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/ping", nil) // sin header Origin
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sin Origin debería pasar: code=%d, quiero 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("sin Origin no debería setear ACAO, got %q", got)
	}
}

func TestCORS_PairReachableFromAnyOrigin(t *testing.T) {
	h := cors(okHandler())
	const origin = "https://cualquiera.example"

	// POST /pair desde un origen NO autorizado igual debe pasar a next.
	req := httptest.NewRequest(http.MethodPost, "/pair", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/pair desde cualquier origen: code=%d, quiero 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("ACAO=%q, quiero %q", got, origin)
	}

	// Preflight OPTIONS /pair → 204.
	req = httptest.NewRequest(http.MethodOptions, "/pair", nil)
	req.Header.Set("Origin", origin)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight /pair: code=%d, quiero 204", rec.Code)
	}
}

func TestCORS_AllowHeadersIncludeAuthorization(t *testing.T) {
	h := cors(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/print", nil)
	req.Header.Set("Origin", "https://app.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight /print: code=%d, quiero 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Authorization") {
		t.Errorf("Allow-Headers=%q, debería incluir Authorization", got)
	}
}

// TestStack_TokenIsTheGate fija el comportamiento OBSERVABLE del stack real
// (cors por fuera, requireToken por dentro — mismo orden que server.New). Con
// CORS permisivo, el token es la única puerta: un endpoint sensible sin token
// devuelve 401 SIEMPRE, tenga o no Origin (CORS no rechaza orígenes).
func TestStack_TokenIsTheGate(t *testing.T) {
	store := testAuth(t)
	s := newPairServer(store, testConfig(t), nil)
	h := cors(s.requireToken(okHandler()))

	cases := []struct {
		name, origin string
		want         int
	}{
		{"sin Origin + sin token", "", http.StatusUnauthorized},
		{"Origin cualquiera + sin token", "https://evil.example", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodPost, "/print", nil)
		if c.origin != "" {
			req.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s: code=%d, quiero %d", c.name, rec.Code, c.want)
		}
	}
}

func TestPair_NilPrompterReturns403(t *testing.T) {
	s := newPairServer(testAuth(t), testConfig(t), nil)
	rec := doPair(s, "https://nueva.example")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d, quiero 403", rec.Code)
	}
	if got := bodyMap(t, rec)["error"]; got != "pairing_denied" {
		t.Errorf("error=%q, quiero pairing_denied", got)
	}
}

func TestPair_AllowAnyOriginReturnsTokenWithoutDialog(t *testing.T) {
	store := testAuth(t)
	p := &fakePrompter{interactive: true}
	s := newPairServer(store, testConfig(t), p)
	// El modo "permitir cualquier origen" YA NO viene del config.yaml: es el valor
	// EFECTIVO que el server recibe de la marca del instalador. Lo simulamos acá.
	s.cfg.AllowAnyOrigin = true

	rec := doPair(s, "https://cualquiera.example")
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, quiero 200", rec.Code)
	}
	if p.calls != 0 {
		t.Errorf("con allow_any_origin no debería mostrarse diálogo (calls=%d)", p.calls)
	}
	if got := bodyMap(t, rec)["token"]; got != store.GetToken() {
		t.Errorf("token=%q, quiero %q", got, store.GetToken())
	}
}

