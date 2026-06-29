//go:build !windows

// Stub no-Windows (solo dev/CI; el agente productivo corre solo en Windows).
// Aplica permisos Unix: 0o700 para directorios, 0o600 para archivos.
package winfs

import "os"

// Restrict aplica permisos restrictivos al path (debe existir). En Unix solo
// hay bit de permisos; en Windows real (winfs_windows.go) se aplica una DACL.
func Restrict(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return os.Chmod(path, 0o700)
	}
	return os.Chmod(path, 0o600)
}
