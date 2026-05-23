package server

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/lautarotiamat/prinklyprint/internal/config"
)

func cors(cm *config.Manager, logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		cfg := cm.Get()
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		allowed := cfg.AllowAnyOrigin || isAllowedOrigin(origin, cfg.AllowedOrigins)
		if !allowed {
			logger.Warn("CORS rechazado", "origin", origin, "path", r.URL.Path)
			http.Error(w, `{"error":"forbidden_origin","origin":"`+origin+`","hint":"Agregalo en la configuración de PrinklyPrint"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedOrigin(origin string, allowed []string) bool {
	host := stripScheme(origin)
	for _, a := range allowed {
		if a == "" {
			continue
		}
		if a == origin {
			return true
		}
		if strings.HasPrefix(a, "*.") {
			suffix := a[1:]
			if strings.HasSuffix(host, suffix) && len(host) > len(suffix) {
				return true
			}
		}
		if a == host {
			return true
		}
	}
	return false
}

func stripScheme(origin string) string {
	if i := strings.Index(origin, "://"); i >= 0 {
		return origin[i+3:]
	}
	return origin
}
