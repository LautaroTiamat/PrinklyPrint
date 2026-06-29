//go:build !windows

// Stub no-Windows: passthrough SIN cifrado real, SOLO para que el código
// compile y se pueda testear el flujo en dev/CI (Linux). El agente productivo
// corre solo en Windows, donde Protect/Unprotect usan DPAPI de verdad. NO usar
// este passthrough en producción.
package dpapi

// Protect devuelve los bytes tal cual (NO cifra). Solo dev/CI.
func Protect(plain []byte) ([]byte, error) {
	return append([]byte(nil), plain...), nil
}

// Unprotect devuelve los bytes tal cual (NO descifra). Solo dev/CI.
func Unprotect(enc []byte) ([]byte, error) {
	return append([]byte(nil), enc...), nil
}
