//go:build windows

// Package winfs aplica permisos restrictivos (owner-only) a archivos y
// directorios sensibles, como parte del control de datos en reposo.
//
// En Windows usa una DACL protegida (sin herencia del padre) que concede Full
// Control solo a: el USUARIO que corre el agente (su SID explícito), Local
// System (SY) y el grupo Administradores locales (BA) — estos dos últimos por
// serviceability (backup, soporte). Cualquier otro usuario queda sin acceso.
//
// Dos decisiones importantes (aprendidas de un bug real):
//
//   - Se concede el SID EXPLÍCITO del usuario, no "OWNER RIGHTS" (S-1-3-4). El
//     SID del usuario es el mismo en un token elevado y uno normal, así que el
//     agente puede reabrir sus archivos lo haya creado como sea. OWNER RIGHTS
//     tenía semántica ambigua que terminaba denegando el acceso al propio dueño.
//
//   - En DIRECTORIOS las ACEs son HEREDABLES (OICI = object+container inherit).
//     Sin esto, los archivos creados adentro (logs rotados, PDFs, DB) no heredan
//     ninguna ACE y quedan con una DACL VACÍA = acceso denegado a todos,
//     incluido el agente, que no podía reabrir su propio log en el segundo
//     arranque del día.
package winfs

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// currentUserSID devuelve el SID (string SDDL) del usuario que corre el proceso.
func currentUserSID() (string, error) {
	tu, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("token user: %w", err)
	}
	return tu.User.Sid.String(), nil
}

// Restrict aplica una DACL protegida owner-only al archivo o directorio en path
// (debe existir). En directorios las ACEs son heredables para que los hijos
// hereden el acceso. Best-effort desde el punto de vista del caller: devuelve
// error para que decida (normalmente un warning, sin abortar).
func Restrict(path string) error {
	sid, err := currentUserSID()
	if err != nil {
		return err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	// D = DACL, P = protegida (no hereda del padre), AI = auto-inherited.
	// FA = Full Access. OICI (solo en dirs) = OBJECT_INHERIT + CONTAINER_INHERIT
	// para que los archivos/subdirs creados adentro hereden estas ACEs.
	var sddl string
	if fi.IsDir() {
		sddl = fmt.Sprintf("D:PAI(A;OICI;FA;;;%s)(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)", sid)
	} else {
		sddl = fmt.Sprintf("D:PAI(A;;FA;;;%s)(A;;FA;;;SY)(A;;FA;;;BA)", sid)
	}

	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	// PROTECTED_DACL_SECURITY_INFORMATION corta la herencia desde el padre: el
	// objeto queda exactamente con la DACL que seteamos.
	return windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
}
