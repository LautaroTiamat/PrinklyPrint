package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/config"
)

// pairDialogBudget es cuánto extendemos el deadline de escritura del response
// de /pair mientras el diálogo de aprobación está abierto, para que el
// WriteTimeout del server no corte la respuesta si el operador tarda en decidir.
const pairDialogBudget = 5 * time.Minute

// PairingPrompter abstrae el diálogo de confirmación de pairing. La
// implementación interactiva (Windows) muestra un MessageBox nativo; en modo
// --headless o no-Windows se inyecta una implementación que siempre deniega.
//
// La definimos acá (en el package que la consume) para que internal/app pueda
// satisfacerla de forma implícita sin crear un ciclo de imports.
type PairingPrompter interface {
	// Confirm muestra un diálogo y devuelve true si el operador aprueba el
	// pareo del origen. label es un nombre amigable opcional de la app.
	Confirm(origin, label string) bool
	// Interactive indica si hay UI disponible para mostrar el diálogo. En
	// modo headless devuelve false y el pairing solo funciona con orígenes
	// pre-aprobados en la config.
	Interactive() bool
}

// pathsExentosDeToken son los endpoints que NO exigen Bearer token: /ping
// (liveness, debe responder antes del pairing) y /pair (es quien emite el
// token).
func exentoDeToken(path string) bool {
	return path == "/ping" || path == "/pair"
}

// requireToken envuelve al handler exigiendo "Authorization: Bearer <token>"
// en todos los paths sensibles. Dos chequeos:
//
//   - Token: faltante o inválido → 401, sin importar el Origin (cierra el
//     bypass de "Origin ausente" para procesos locales no-navegador).
//
//   - Origen aprobado (solo para requests del navegador, con Origin): aunque el
//     token sea válido, el origen tiene que seguir en la lista de aprobados
//     (ver originApproved). Así, quitar un origen de Orígenes CORS revoca su
//     acceso de inmediato, aunque la app todavía tenga el token cacheado.
//     Devolvemos 401 (no 403) a propósito: la librería lo interpreta como
//     "re-pareá" y el agente vuelve a pedir aprobación. Los callers SIN Origin
//     (curl, Node, procesos locales) se gatean solo por el token.
//
// Debe ir POR DENTRO del middleware de CORS: el preflight OPTIONS lo resuelve
// CORS antes de llegar acá.
func (s *Server) requireToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if exentoDeToken(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		tok, ok := bearerToken(r.Header.Get("Authorization"))
		if !ok || s.cfg.Auth == nil || !s.cfg.Auth.ValidateToken(tok) {
			reason := "token inválido"
			if !ok {
				reason = "token faltante o mal formado"
			}
			// Evento de seguridad: el gap principal eran los 401
			// invisibles. NUNCA logueamos el valor del token.
			s.cfg.SecLog.AuthFailure(r.URL.Path, origin, reason, origin != "")
			writeErr(w, http.StatusUnauthorized, "unauthorized",
				"falta o es inválido el token; obtené uno con POST /pair")
			return
		}
		if origin != "" && !s.originApproved(origin) {
			s.cfg.SecLog.AuthFailure(r.URL.Path, origin, "origen no aprobado", true)
			writeErr(w, http.StatusUnauthorized, "unauthorized",
				"el origen no está autorizado para imprimir; volvé a parear con POST /pair")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// bearerToken extrae el token de un header "Bearer <token>" (case-insensitive
// en el prefijo). Devuelve ("", false) si no tiene el formato esperado.
func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	tok := strings.TrimSpace(header[len(prefix):])
	if tok == "" {
		return "", false
	}
	return tok, true
}

// originApproved indica si un origen ya está autorizado en la config
// (allowed_origins, con soporte de wildcards, o allow_any_origin). Es la única
// fuente de verdad de orígenes aprobados — visible y editable en la UI
// (General → Orígenes CORS). Para estos orígenes /pair devuelve el token sin
// diálogo. Cuando el operador aprueba un pareo nuevo, el origen se agrega acá.
func (s *Server) originApproved(origin string) bool {
	if origin == "" {
		return false
	}
	cfg := s.cfg.Config.Get()
	// allow-any-origin viene de la marca del instalador (s.cfg.AllowAnyOrigin), NO
	// del config.yaml: un operador no puede reactivarlo editando el yaml ni desde
	// la UI. Ver internal/insecure y server.Config.AllowAnyOrigin.
	return s.cfg.AllowAnyOrigin || isAllowedOrigin(origin, cfg.AllowedOrigins)
}

// approveOrigin agrega el origen a la lista de orígenes permitidos (config),
// que es la lista visible en la UI. Idempotente.
func (s *Server) approveOrigin(origin string) error {
	return s.cfg.Config.Update(func(c *config.Config) {
		for _, o := range c.AllowedOrigins {
			if o == origin {
				return
			}
		}
		c.AllowedOrigins = append(c.AllowedOrigins, origin)
	})
}

// pairRateLimited aplica el rate limit de /pair SI está activo en la config.
// Devuelve true (y ya respondió 429 + Retry-After) cuando el request debe
// rechazarse. El bucket es GLOBAL (en loopback todo el tráfico viene de
// 127.0.0.1, así que per-origin no aporta) y se lee/re-deriva en vivo desde la
// config. Apagado por default ⇒ siempre devuelve false (comportamiento intacto).
// NUNCA se aplica a /print.
func (s *Server) pairRateLimited(w http.ResponseWriter, r *http.Request) bool {
	cfg := s.cfg.Config.Get()
	if !cfg.PairRateLimitEnabled {
		return false
	}
	s.limiterMu.Lock()
	if s.pairLimiter == nil {
		s.pairLimiter = newTokenBucket(cfg.PairRateLimitPerMinute, cfg.PairRateLimitBurst, s.now)
	} else {
		s.pairLimiter.reconfigure(cfg.PairRateLimitPerMinute, cfg.PairRateLimitBurst)
	}
	limiter := s.pairLimiter
	s.limiterMu.Unlock()

	if limiter.allow() {
		return false
	}
	// Limitado: evento de seguridad para trazar intentos de fuerza bruta de
	// pairing en el SIEM. NUNCA se loguea el token.
	s.cfg.SecLog.PairingDenied(r.Header.Get("Origin"), "rate_limited")
	w.Header().Set("Retry-After", "5")
	writeErr(w, http.StatusTooManyRequests, "rate_limited",
		"demasiados intentos de pareo; reintentá en unos segundos")
	return true
}

// handlePair implementa el handshake de pairing. Ver el contrato de cable en
// el README / CHANGELOG. Devuelve 200 {"token":...} o 403 pairing_denied.
func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	// Rate limit opcional, SOLO de /pair (off por default). Va al principio: antes
	// de leer el body, tomar s.pairMu o mostrar cualquier diálogo.
	if s.pairRateLimited(w, r) {
		return
	}
	if s.cfg.Auth == nil {
		writeErr(w, http.StatusServiceUnavailable, "pairing_unavailable",
			"el almacén de autenticación no está inicializado")
		return
	}

	origin := r.Header.Get("Origin")
	if origin == "" {
		s.cfg.SecLog.PairingDenied("", "sin header Origin")
		writeErr(w, http.StatusForbidden, "pairing_denied",
			"falta el header Origin; el pareo solo puede iniciarse desde un navegador")
		return
	}

	// Camino rápido: ya autorizado → token sin diálogo.
	if s.originApproved(origin) {
		writeJSON(w, http.StatusOK, map[string]string{"token": s.cfg.Auth.GetToken()})
		return
	}

	// Leemos el body (label opcional) ANTES de tomar el lock: hacer I/O de red
	// bajo s.pairMu permitiría que un cliente lento (slow-loris) retenga el
	// mutex y bloquee el pairing de los demás. Body acotado porque /pair está
	// exento de token. El label solo se usa para el diálogo/log.
	var body struct {
		Label string `json:"label"`
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		_ = json.NewDecoder(r.Body).Decode(&body) // body opcional: ignoramos errores
	}

	// Serializamos el diálogo para no abrir dos a la vez y para que un segundo
	// request del mismo origen vea el resultado del primero.
	s.pairMu.Lock()
	defer s.pairMu.Unlock()

	// Re-chequeo dentro del lock: otro request pudo haber aprobado el origen
	// mientras esperábamos.
	if s.originApproved(origin) {
		writeJSON(w, http.StatusOK, map[string]string{"token": s.cfg.Auth.GetToken()})
		return
	}

	if s.cfg.Prompter == nil || !s.cfg.Prompter.Interactive() {
		s.cfg.SecLog.PairingDenied(origin, "modo headless sin pre-aprobación")
		writeErr(w, http.StatusForbidden, "pairing_denied",
			"no hay UI para aprobar el pareo en este equipo; IT debe pre-aprobar el origen en allowed_origins, o ejecutar el agente en modo interactivo una vez para aprobarlo")
		return
	}

	// La aprobación del operador puede tardar (el diálogo queda abierto).
	// Extendemos el deadline de escritura para que el WriteTimeout del server
	// (60s) no corte la respuesta antes de que el operador decida.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(pairDialogBudget))

	if !s.cfg.Prompter.Confirm(origin, body.Label) {
		s.cfg.SecLog.PairingDenied(origin, "el operador rechazó el pareo")
		writeErr(w, http.StatusForbidden, "pairing_denied", "el operador rechazó el pareo para "+origin)
		return
	}

	// Aprobado: agregamos el origen a la lista de orígenes permitidos (config),
	// visible y editable en la UI. Es la fuente de verdad de orígenes aprobados.
	if err := s.approveOrigin(origin); err != nil {
		writeErr(w, http.StatusInternalServerError, "pairing_failed", err.Error())
		return
	}
	s.cfg.SecLog.PairingApproved(origin, body.Label)
	writeJSON(w, http.StatusOK, map[string]string{"token": s.cfg.Auth.GetToken()})
}
