package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lautarotiamat/prinklyprint/internal/seclog"
)

type fakeSecSink struct{ ids []uint32 }

func (f *fakeSecSink) Emit(_ seclog.Level, id uint32, _ string) error {
	f.ids = append(f.ids, id)
	return nil
}
func (f *fakeSecSink) Close() error { return nil }

func (f *fakeSecSink) has(id uint32) bool {
	for _, x := range f.ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestRequireToken_LogsAuthFailure(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	sink := &fakeSecSink{}
	s := &Server{cfg: Config{Auth: store, Config: cm, Logger: testLogger(), SecLog: seclog.New(testLogger(), sink)}}
	h := s.requireToken(okHandler())

	// Sin token → 401 + evento auth_failure (el gap principal: 401 invisibles).
	req := httptest.NewRequest(http.MethodGet, "/printers", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("code=%d, quiero 401", rec.Code)
	}
	if !sink.has(seclog.IDAuthFailure) {
		t.Errorf("debería haberse emitido un evento auth_failure (ids=%v)", sink.ids)
	}
}

func TestRequireToken_NoLogOnSuccess(t *testing.T) {
	store := testAuth(t)
	cm := testConfig(t)
	sink := &fakeSecSink{}
	s := &Server{cfg: Config{Auth: store, Config: cm, Logger: testLogger(), SecLog: seclog.New(testLogger(), sink)}}
	h := s.requireToken(okHandler())

	// Con token válido y sin Origin → 200, sin auth_failure.
	req := httptest.NewRequest(http.MethodGet, "/printers", nil)
	req.Header.Set("Authorization", "Bearer "+store.GetToken())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d, quiero 200", rec.Code)
	}
	if sink.has(seclog.IDAuthFailure) {
		t.Errorf("no debería emitir auth_failure en éxito (ids=%v)", sink.ids)
	}
}
