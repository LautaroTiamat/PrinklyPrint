//go:build !windows

// Stub no-Windows (solo dev/CI; el agente productivo corre solo en Windows).
// El modo inseguro nunca está activo fuera de Windows: la marca real vive en
// HKLM, que solo existe en Windows.
package insecure

// AllowAnyOrigin siempre devuelve false fuera de Windows (modo seguro).
func AllowAnyOrigin() bool { return false }
