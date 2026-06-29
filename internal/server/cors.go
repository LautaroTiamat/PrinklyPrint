package server

import (
	"net/http"
	"strings"
)

// cors maneja CORS para que la app web pueda hablar con el agente desde el
// navegador.
//
// IMPORTANTE: el control de acceso real NO lo hace CORS, sino el token Bearer
// (ver requireToken) más el diálogo de pairing. Un origen sin token recibe 401,
// y para obtener un token vía POST /pair el operador tiene que aprobar el
// diálogo nativo (que muestra el origen). Por eso CORS es PERMISIVO: refleja el
// Origin del request y responde el preflight para CUALQUIER origen.
//
// ¿Por qué permisivo y no una whitelist? El navegador manda un preflight
// (OPTIONS) antes de un POST con Content-Type JSON o con header Authorization.
// Si rechazáramos orígenes no aprobados en el preflight, el navegador bloquearía
// el request ANTES de llegar al 401, y el flujo de pairing (401 → /pair →
// diálogo) nunca podría arrancar para un origen nuevo: la app web ni siquiera
// podría descubrir que el agente está corriendo. Dejar pasar el request y gatear
// con el token + diálogo es lo que hace que el pairing funcione.
//
// Requests sin Origin (curl, procesos locales no-navegador) no necesitan
// headers CORS; el token igual los gatea en requireToken.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		setCORSHeaders(w, origin)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// setCORSHeaders refleja el Origin y habilita los métodos/headers que usa la
// librería. Allow-Headers incluye Authorization para que el navegador pueda
// mandar el Bearer token en los endpoints sensibles.
func setCORSHeaders(w http.ResponseWriter, origin string) {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", origin)
	h.Set("Vary", "Origin")
	h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	h.Set("Access-Control-Max-Age", "600")
}

// isAllowedOrigin indica si el origen coincide con alguno de la lista (match
// exacto, por host, o wildcard de subdominio "*.dominio"). Lo usa
// [Server.originApproved] para decidir si /pair entrega el token sin diálogo.
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
