# PrinklyPrint — Dockerfile self-contained para compilar el .exe localmente.
#
# Uso (Linux / macOS):
#   docker build -t prinklyprint-build .
#   docker run --rm -v "$PWD:/work" -e VERSION=1.0.0 prinklyprint-build
#
# Uso (Windows PowerShell):
#   docker build -t prinklyprint-build .
#   docker run --rm -v "${PWD}:/work" -e VERSION=1.0.0 prinklyprint-build
#
# Produce: dist/prinklyprint.exe (~30 MB, cross-compilado a windows/amd64).
#
# NO compila el instalador (Setup.exe) — eso lo hace GitHub Actions en un runner
# Windows porque Wine en Docker Desktop / WSL2 tiene un bug irresoluble con
# sockets (sock_check_pollhup ENOSYS). Para releases: git tag vX.Y.Z && push.
#
# Supply chain (ver también .github/workflows/release.yml):
#   - Imagen base pineada por DIGEST (inmutable), no por tag mutable.
#   - rsrc pineado a una versión exacta (verificada contra la checksum DB de Go).
#   - SumatraPDF se verifica por SHA256 contra checksums.txt; el build FALLA si
#     no coincide.
#   - Build reproducible: -trimpath y -buildid= vacío, CGO_ENABLED explícito.

# golang:1.25-alpine pineado por digest. Para actualizar (lo hace Dependabot):
#   docker buildx imagetools inspect golang:1.25-alpine --format '{{.Manifest.Digest}}'
# Go 1.25 (no 1.22): la 1.22 quedó fuera de la ventana de parches y arrastra
# vulnerabilidades de la stdlib que govulncheck marca (request smuggling, etc.).
FROM golang:1.26-alpine@sha256:3ad57304ad93bbec8548a0437ad9e06a455660655d9af011d58b993f6f615648

RUN apk add --no-cache git ca-certificates curl imagemagick

# rsrc: embebe el manifest XML (Common Controls v6 + DPI awareness) y el ícono
# del .exe. Pineado a una versión exacta (no @latest): `go install pkg@vX.Y.Z`
# verifica el módulo contra la checksum DB de Go (sum.golang.org). La versión va
# en sync con la fijada en go.mod/go.sum vía tools.go.
RUN go install github.com/akavel/rsrc@v0.10.2

ENV CGO_ENABLED=0 \
    GOOS=windows \
    GOARCH=amd64 \
    GOFLAGS="-mod=mod"

WORKDIR /work

ENTRYPOINT ["/bin/sh", "-c", "\
set -e; \
VERSION=\"${VERSION:-dev}\"; \
mkdir -p dist internal/embedded; \
SUMATRA=internal/embedded/SumatraPDF.exe; \
SUMATRA_URL=https://www.sumatrapdfreader.org/dl/rel/3.5.2/SumatraPDF-3.5.2-64.exe; \
if [ ! -s \"$SUMATRA\" ]; then echo 'Bajando SumatraPDF...'; curl -fsSL -o \"$SUMATRA\" \"$SUMATRA_URL\"; fi; \
EXPECTED=$(awk '/SumatraPDF-3.5.2-64.exe/ && $1 ~ /^[0-9a-f]{64}$/ {print $1}' checksums.txt); \
ACTUAL=$(sha256sum \"$SUMATRA\" | awk '{print $1}'); \
if [ -z \"$EXPECTED\" ]; then echo 'ERROR: no encontré el SHA256 esperado de SumatraPDF en checksums.txt'; exit 1; fi; \
if [ \"$EXPECTED\" != \"$ACTUAL\" ]; then echo \"ERROR: SHA256 de SumatraPDF no coincide. esperado=$EXPECTED obtenido=$ACTUAL\"; exit 1; fi; \
echo \"SumatraPDF verificado OK ($ACTUAL)\"; \
rsrc -manifest app.manifest -ico icons/PrinklyPrint.ico -o rsrc.syso; \
mkdir -p internal/tray/assets; \
for spec in green:#2ea044 yellow:#d9a307 red:#dc2626; do \
  name=\"${spec%:*}\"; color=\"${spec#*:}\"; \
  magick icons/icon_256x256.png \
    -fill white -draw \"circle 200,200 200,160\" \
    -fill \"$color\" -draw \"circle 200,200 200,165\" \
    \\( -clone 0 -resize 16x16 \\) \
    \\( -clone 0 -resize 24x24 \\) \
    \\( -clone 0 -resize 32x32 \\) \
    \\( -clone 0 -resize 48x48 \\) \
    \\( -clone 0 -resize 64x64 \\) \
    -delete 0 \
    \"internal/tray/assets/tray-$name.ico\"; \
done; \
go mod download; \
go build -tags with_sumatra -trimpath \
  -ldflags=\"-s -w -H=windowsgui -buildid= -X main.version=$VERSION\" \
  -o dist/prinklyprint.exe .; \
echo \"OK -> dist/prinklyprint.exe v$VERSION\""]
