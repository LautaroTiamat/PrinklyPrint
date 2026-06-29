package auth

import (
	"path/filepath"
	"testing"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Load(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return s
}

func TestTokenGeneratedAndNonEmpty(t *testing.T) {
	s := newStore(t)
	if s.GetToken() == "" {
		t.Fatal("el token no debería estar vacío al crear el store")
	}
}

func TestTokenPersistedAndReused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s1, err := Load(path)
	if err != nil {
		t.Fatalf("Load 1: %v", err)
	}
	tok := s1.GetToken()

	s2, err := Load(path)
	if err != nil {
		t.Fatalf("Load 2: %v", err)
	}
	if s2.GetToken() != tok {
		t.Fatalf("el token debería sobrevivir al reinicio: %q != %q", s2.GetToken(), tok)
	}
}

func TestTwoInstallationsDistinctTokens(t *testing.T) {
	a, err := Load(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("Load a: %v", err)
	}
	b, err := Load(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("Load b: %v", err)
	}
	if a.GetToken() == b.GetToken() {
		t.Fatal("dos instalaciones (data dir distinto) deberían tener tokens distintos")
	}
}

func TestValidateToken(t *testing.T) {
	s := newStore(t)
	tok := s.GetToken()
	if !s.ValidateToken(tok) {
		t.Fatal("el token correcto debería validar")
	}
	if s.ValidateToken("token-incorrecto") {
		t.Fatal("un token incorrecto no debería validar")
	}
	if s.ValidateToken("") {
		t.Fatal("un token vacío no debería validar")
	}
}

func TestRotateTokenChanges(t *testing.T) {
	s := newStore(t)
	old := s.GetToken()

	if err := s.RotateToken(); err != nil {
		t.Fatalf("RotateToken: %v", err)
	}
	if s.GetToken() == old {
		t.Fatal("RotateToken debería cambiar el token")
	}
	if s.ValidateToken(old) {
		t.Fatal("el token viejo no debería validar tras rotar")
	}
	if !s.ValidateToken(s.GetToken()) {
		t.Fatal("el token nuevo debería validar")
	}
}
