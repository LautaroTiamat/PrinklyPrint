// Package auth gestiona el secreto de autenticación del agente: un token por
// instalación.
//
// Por qué un archivo aparte:
//
//	El token NO vive en config.yaml. config.yaml es editable por el usuario y
//	lo manipula la UI; mezclar el secreto ahí lo expondría a edición casual y
//	dificultaría restringirle permisos. Por eso el token se persiste en
//	auth.json (mismo data dir), threadsafe.
//
// Los orígenes APROBADOS para imprimir NO viven acá: viven en
// config.AllowedOrigins (visibles y editables en la UI, pestaña General →
// Orígenes CORS). Cuando el operador aprueba un pareo desde el diálogo, el
// handler de POST /pair agrega el origen a esa lista. Así hay una única fuente
// de verdad de orígenes aprobados, visible para el operador.
//
// Modelo: un token por instalación. Cada PC genera el suyo en el primer
// arranque y lo reusa entre reinicios; comprometer un equipo no expone a los
// demás. El token se compara en tiempo constante ([crypto/subtle]).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/lautarotiamat/prinklyprint/internal/winfs"
)

// tokenBytes es la entropía del token: 32 bytes de crypto/rand → 256 bits.
const tokenBytes = 32

// persisted es la forma en disco de auth.json.
type persisted struct {
	Token string `json:"token"`
}

// Store guarda el token de la instalación. Es threadsafe.
type Store struct {
	path  string
	mu    sync.RWMutex
	token string
}

// Load abre (o crea) el archivo de auth. Si no existe, genera un token nuevo y
// lo persiste. Si existe, reusa el token guardado. Si el archivo existe pero
// quedó sin token (corrupto o editado a mano), genera uno nuevo para no dejar
// el agente sin secreto.
func Load(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("crear dir de auth: %w", err)
	}
	s := &Store{path: path}

	b, err := os.ReadFile(path)
	switch {
	case os.IsNotExist(err):
		tok, gerr := generateToken()
		if gerr != nil {
			return nil, gerr
		}
		s.token = tok
		if perr := s.persistLocked(); perr != nil {
			return nil, perr
		}
		return s, nil
	case err != nil:
		return nil, fmt.Errorf("leer auth: %w", err)
	}

	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, fmt.Errorf("parsear auth json: %w", err)
	}
	s.token = p.Token
	if s.token == "" {
		tok, gerr := generateToken()
		if gerr != nil {
			return nil, gerr
		}
		s.token = tok
		if perr := s.persistLocked(); perr != nil {
			return nil, perr
		}
	}
	return s, nil
}

// generateToken devuelve 32 bytes de crypto/rand en base64url sin padding.
func generateToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generar token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// GetToken devuelve el token actual.
func (s *Store) GetToken() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.token
}

// ValidateToken compara en tiempo constante el token recibido con el guardado.
// Devuelve false si alguno está vacío.
func (s *Store) ValidateToken(token string) bool {
	s.mu.RLock()
	expected := s.token
	s.mu.RUnlock()
	if token == "" || expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(expected)) == 1
}

// RotateToken genera un token nuevo, invalidando todos los tokens cacheados por
// las apps (que tendrán que re-parear). Pensado para una acción "Regenerar
// token" en la UI o ante una sospecha de filtración del secreto. No toca la
// lista de orígenes permitidos (eso se maneja en la config).
func (s *Store) RotateToken() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tok, err := generateToken()
	if err != nil {
		return err
	}
	s.token = tok
	return s.persistLocked()
}

// persistLocked serializa el estado a disco de forma atómica (tmp + rename).
// Debe llamarse con s.mu tomado en modo escritura, O sobre un *Store recién
// creado que todavía no se publicó a otras goroutines (caso de Load).
func (s *Store) persistLocked() error {
	b, err := json.MarshalIndent(persisted{Token: s.token}, "", "  ")
	if err != nil {
		return fmt.Errorf("serializar auth json: %w", err)
	}

	// Escritura atómica con fsync: escribimos a un tmp, lo sincronizamos a disco
	// y recién ahí renombramos. Ante cualquier fallo limpiamos el tmp.
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("escribir auth: %w", err)
	}
	if _, err := f.Write(b); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("escribir auth: %w", err)
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("fsync auth: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("cerrar auth: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp) // no dejar el .tmp huérfano
		return fmt.Errorf("rename auth: %w", err)
	}
	restrictPermissions(s.path) // best-effort, no bloquea
	return nil
}

// restrictPermissions restringe el acceso al archivo del secreto (owner-only).
// En Windows aplica una DACL protegida; en Unix (dev/CI) un chmod 0o600. Ver
// internal/winfs. Best-effort: el caller no aborta si falla.
func restrictPermissions(path string) {
	_ = winfs.Restrict(path)
}
