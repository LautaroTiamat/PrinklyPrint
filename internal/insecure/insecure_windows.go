//go:build windows

// Package insecure expone marcas de "modos inseguros" que NO deben poder
// activarse en runtime ni desde la UI ni desde el config.yaml editable. La única
// fuente de verdad es el registro HKLM, escribible solo por un proceso elevado
// (el instalador). Un usuario estándar no puede modificar HKLM.
package insecure

import "golang.org/x/sys/windows/registry"

// regPath es la clave donde el instalador deja las marcas.
const regPath = `Software\PrinklyPrint`

// AllowAnyOrigin devuelve true solo si el instalador activó el modo "permitir
// cualquier origen" (HKLM\Software\PrinklyPrint\AllowAnyOrigin = 1, DWORD).
// Ausente, no numérico o != 1 → false (modo seguro). Es la ÚNICA forma de
// habilitar ese modo: ni la UI ni el config.yaml pueden hacerlo.
func AllowAnyOrigin() bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, regPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AllowAnyOrigin")
	if err != nil {
		return false
	}
	return v == 1
}
