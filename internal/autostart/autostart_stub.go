//go:build !windows

package autostart

// Implementación stub para que el package compile en Linux (tests).
// PrinklyPrint solo corre en Windows, así que estas funciones nunca se llaman
// en producción; existen únicamente para que `go test ./internal/...` pase.

func IsEnabled() (bool, error)  { return false, nil }
func Enable() error             { return nil }
func Disable() error            { return nil }
func Sync(desired bool) error   { return nil }
