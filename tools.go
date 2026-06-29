//go:build tools

// Este archivo NO se compila en el binario: vive detrás del build tag `tools`,
// que el build normal (`go build -tags with_sumatra`) no activa.
//
// Existe para FIJAR la versión de las herramientas de build en go.mod/go.sum
// (patrón "tools" de Go). Así:
//   - `go run github.com/akavel/rsrc ...` usa la versión lockeada con integridad
//     verificada por go.sum (en vez de `@latest`, que es mutable).
//   - Dependabot (ecosistema gomod) mantiene la versión al día de forma controlada.
//
// rsrc embebe el manifest (Common Controls v6 + DPI awareness) y el ícono en el
// .exe. Ver .github/workflows/release.yml y Dockerfile.
package main

import (
	_ "github.com/akavel/rsrc"
)
