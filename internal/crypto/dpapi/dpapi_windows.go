//go:build windows

// Package dpapi cifra/descifra datos en reposo con la DPAPI de Windows
// (CryptProtectData / CryptUnprotectData), scope de USUARIO.
//
// Scope de usuario (sin CRYPTPROTECT_LOCAL_MACHINE): la clave deriva del perfil
// del usuario que corre el agente. Un blob exfiltrado a otro usuario o a otro
// equipo NO se puede descifrar — justo lo que queremos para los PDFs en reposo.
//
// Usamos los wrappers de golang.org/x/sys/windows (ya es dependencia), sin
// agregar libs nuevas ni lazyproc manual.
package dpapi

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func toBlob(b []byte) windows.DataBlob {
	if len(b) == 0 {
		return windows.DataBlob{}
	}
	return windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

// copyAndFree copia el buffer de salida de DPAPI a un slice de Go y libera el
// buffer asignado por Windows con LocalFree.
func copyAndFree(out windows.DataBlob) []byte {
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data))) //nolint:errcheck
	if out.Size == 0 || out.Data == nil {
		return nil
	}
	src := unsafe.Slice(out.Data, out.Size)
	dst := make([]byte, out.Size)
	copy(dst, src)
	return dst
}

// Protect cifra plain con DPAPI (scope de usuario). CRYPTPROTECT_UI_FORBIDDEN
// evita cualquier prompt de UI (corremos sin interacción).
func Protect(plain []byte) ([]byte, error) {
	in := toBlob(plain)
	var out windows.DataBlob
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("CryptProtectData: %w", err)
	}
	return copyAndFree(out), nil
}

// Unprotect descifra un blob producido por Protect en el MISMO usuario/equipo.
// Falla (esperado) si el blob es de otro usuario/equipo.
func Unprotect(enc []byte) ([]byte, error) {
	in := toBlob(enc)
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	return copyAndFree(out), nil
}
