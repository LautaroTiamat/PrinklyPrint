// Package server expone la API HTTP local que consume la aplicación web del
// cliente. Escucha exclusivamente en 127.0.0.1:17777 (binding explícito a
// loopback IPv4) — no es accesible desde la red.
//
// Endpoints (todos devuelven JSON):
//
//	GET    /ping                    healthcheck (usado por el "circulito" de la lib JS)
//	GET    /printers                lista impresoras del sistema + estado
//	GET    /settings                config default actual del agente
//	POST   /print                   encola un PDF (base64 inline)
//	GET    /jobs                    lista jobs (filtro por estado, paginado)
//	GET    /jobs/{id}               detalle de un job
//	POST   /jobs/{id}/retry         reencola un job failed
//	DELETE /jobs/{id}               cancela un job queued
//
// Seguridad: el modelo de amenazas es (a) procesos locales no-navegador que
// llaman al loopback y (b) páginas web maliciosas en el navegador del operador.
// La puerta de acceso es el TOKEN + el DIÁLOGO de pairing:
//
//   - Token por instalación (Bearer): todos los endpoints sensibles exigen
//     "Authorization: Bearer <token>" (ver [requireToken]). Faltar o ser
//     inválido el token devuelve 401 sin importar el Origin. Exentos de token:
//     GET /ping y POST /pair.
//
//   - Pairing con consentimiento: el token se emite por POST /pair. Para un
//     origen nuevo, el agente muestra un diálogo nativo y el operador decide;
//     al aprobar, el origen se agrega a [config.Config.AllowedOrigins] (visible
//     en la UI) y los pareos siguientes son silenciosos. Ver [handlePair].
//
//   - CORS permisivo (NO es la puerta): refleja cualquier Origin y responde el
//     preflight. Es a propósito: con CORS estricto el navegador bloquearía el
//     preflight antes del 401 y el pairing nunca podría arrancar. Ver [cors].
//
// No usamos TLS porque es loopback: el tráfico nunca sale de la máquina.
//
// Merge de defaults: el handler de POST /print fusiona las opciones del
// cliente con los defaults del agente. Para los campos booleanos y float64
// se usan punteros (*bool, *float64) en el wire format para distinguir
// "campo omitido" de "valor zero". Ver [resolveOptions].
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/auth"
	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/seclog"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

type Config struct {
	Addr    string
	Logger  *slog.Logger
	Store   *store.Store
	Printer *printer.Service
	Queue   *queue.Worker
	Config  *config.Manager
	Auth    *auth.Store
	Prompter PairingPrompter
	SecLog   *seclog.Logger // eventos de seguridad → slog + Event Log
	Version  string
	MachineID string
	// AllowAnyOrigin es el valor EFECTIVO del modo "permitir cualquier origen",
	// calculado al arrancar desde la marca del instalador (internal/insecure), NO
	// desde config.yaml ni la UI. Es la única fuente que consultan originApproved
	// y el pairing. Default false (modo seguro).
	AllowAnyOrigin bool
}

type Server struct {
	cfg    Config
	srv    *http.Server
	pairMu sync.Mutex // serializa el handshake de pairing (un diálogo a la vez)

	// Rate limit de /pair (opcional, off por default). El bucket se crea perezoso
	// en el primer /pair con el rate limit activo y se re-deriva si cambian los
	// valores de config en vivo. NUNCA se aplica a /print.
	limiterMu   sync.Mutex
	pairLimiter *tokenBucket
	now         func() time.Time // reloj inyectable (tests); nil ⇒ time.Now
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, now: time.Now}
	mux := http.NewServeMux()
	s.routes(mux)
	// Orden de middlewares: cors por fuera, requireToken por dentro, mux al
	// fondo. Así el preflight OPTIONS lo resuelve CORS antes del chequeo de token.
	handler := cors(s.requireToken(mux))
	s.srv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	addr := s.srv.Addr
	if addr == "" || addr[0] == ':' {
		addr = "127.0.0.1" + addr
		s.srv.Addr = addr
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.cfg.Logger.Info("HTTP server listo", "addr", addr)

	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.Serve(ln) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
