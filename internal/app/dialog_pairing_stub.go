//go:build !windows

package app

// confirmPairing no tiene diálogo nativo fuera de Windows: deniega siempre.
func confirmPairing(_, _, _ string) bool { return false }
