//go:build windows

// Package winfs aplica permisos restrictivos (owner-only) a archivos y
// directorios sensibles, como parte del control de datos en reposo (C-06/C-08).
//
// En Windows usa una DACL protegida (sin herencia del padre) que concede Full
// Control solo a: el dueño del objeto (OW), Local System (SY) y el grupo
// Administradores locales (BA) — estos dos últimos por serviceability (backup,
// soporte, el propio servicio si corre como SYSTEM). Cualquier otro usuario
// queda sin acceso.
package winfs

import "golang.org/x/sys/windows"

// ownerOnlySDDL: D = DACL, P = protegida (no hereda del padre), AI = auto-inherited.
// (A;;FA;;;OW) Allow Full Access al Owner; SY = System; BA = Builtin Administrators.
const ownerOnlySDDL = "D:PAI(A;;FA;;;OW)(A;;FA;;;SY)(A;;FA;;;BA)"

// Restrict aplica la DACL owner-only al archivo o directorio en path. El objeto
// debe existir. Best-effort desde el punto de vista del caller: devuelve error
// para que el caller decida (normalmente log de warning, sin abortar).
func Restrict(path string) error {
	sd, err := windows.SecurityDescriptorFromString(ownerOnlySDDL)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// PROTECTED_DACL_SECURITY_INFORMATION corta la herencia desde el contenedor
	// padre: el objeto queda exactamente con la DACL que seteamos.
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}
