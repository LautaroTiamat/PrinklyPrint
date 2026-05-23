// Package locale detecta el idioma preferido del SO.
package locale

import "strings"

func Detect(supported []string, fallback string) string {
	raw := detectSystem()
	if raw == "" {
		return fallback
	}
	short := strings.ToLower(strings.SplitN(raw, "-", 2)[0])
	for _, s := range supported {
		if strings.EqualFold(s, short) {
			return s
		}
	}
	return fallback
}
