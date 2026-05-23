// Package server expone la API HTTP local que consume la aplicación web del
// cliente. Escucha exclusivamente en 127.0.0.1:17777 (binding explícito a
// loopback IPv4) — no es accesible desde la red.
//
// Endpoints (todos devuelven JSON):
//
//	GET    /ping                    healthcheck (usado por el "circulito" de la lib JS)
//	GET    /printers                lista impresoras del sistema + estado
//	GET    /settings                config default actual del agente
//	POST   /print                   encola un PDF (base64 o URL)
//	GET    /jobs                    lista jobs (filtro por estado, paginado)
//	GET    /jobs/{id}               detalle de un job
//	POST   /jobs/{id}/retry         reencola un job failed
//	DELETE /jobs/{id}               cancela un job queued
//
// Seguridad: el modelo de amenazas es "página web maliciosa en el navegador
// del operador". Por eso aplicamos CORS estricto: solo aceptamos requests
// cuyo Origin esté en la whitelist [config.Config.AllowedOrigins]. La
// whitelist se configura desde la UI del agente; arranca vacía y rechaza
// todo hasta que el operador agrega el dominio explícitamente.
//
// No usamos TLS porque es loopback: el tráfico nunca sale de la máquina.
// No usamos pairing token porque CORS estricto cubre el caso de uso real.
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
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

type Config struct {
	Addr      string
	Logger    *slog.Logger
	Store     *store.Store
	Printer   *printer.Service
	Queue     *queue.Worker
	Config    *config.Manager
	Version   string
	MachineID string
}

type Server struct {
	cfg Config
	srv *http.Server
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg}
	mux := http.NewServeMux()
	s.routes(mux)
	s.srv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           cors(cfg.Config, cfg.Logger, mux),
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
