//go:build !windows

package locale

import "os"

func detectSystem() string {
	for _, v := range []string{"LANG", "LC_ALL", "LC_MESSAGES"} {
		if s := os.Getenv(v); s != "" {
			return s
		}
	}
	return ""
}
