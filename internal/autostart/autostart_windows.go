//go:build windows

// Package autostart gestiona la entrada de inicio automático de PrinklyPrint
// en el registro de Windows.
//
// Usamos HKCU\Software\Microsoft\Windows\CurrentVersion\Run — la ubicación
// estándar para "apps que se inician con el usuario". Ventajas frente a Task
// Scheduler:
//
//   - No requiere admin (HKCU = current user).
//   - Es visible/editable desde Task Manager → Startup, así el usuario tiene
//     control desde herramientas del SO sin tocar nuestra UI.
//   - Una sola entrada de registro vs varias scheduled tasks que hay que
//     limpiar al desinstalar.
//
// El path al .exe se determina con os.Executable() al momento de habilitar,
// así si el usuario movió el binario después del instalador, queda apuntando
// al lugar correcto.
package autostart

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName  = "PrinklyPrint"
)

// IsEnabled devuelve true si existe la entrada en HKCU\...\Run.
func IsEnabled() (bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return false, nil
		}
		return false, fmt.Errorf("abrir Run key: %w", err)
	}
	defer k.Close()
	_, _, err = k.GetStringValue(valueName)
	if err == registry.ErrNotExist {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("leer valor: %w", err)
	}
	return true, nil
}

// Enable crea la entrada apuntando al .exe actual.
// La envolvemos en comillas para tolerar paths con espacios
// (ej. "C:\Program Files\PrinklyPrint\prinklyprint.exe").
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("os.Executable: %w", err)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("crear Run key: %w", err)
	}
	defer k.Close()
	cmd := `"` + exe + `"`
	if err := k.SetStringValue(valueName, cmd); err != nil {
		return fmt.Errorf("escribir valor: %w", err)
	}
	return nil
}

// Disable elimina la entrada. Si no existía, no es error.
func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil
		}
		return fmt.Errorf("abrir Run key: %w", err)
	}
	defer k.Close()
	if err := k.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
		return fmt.Errorf("borrar valor: %w", err)
	}
	return nil
}

// Sync alinea el estado del registro con lo que dice la config.
// Idempotente — si ya está en el estado deseado, no hace nada.
func Sync(desired bool) error {
	current, err := IsEnabled()
	if err != nil {
		return err
	}
	if current == desired {
		return nil
	}
	if desired {
		return Enable()
	}
	return Disable()
}
