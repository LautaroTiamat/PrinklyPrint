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

FROM golang:1.22-alpine

RUN apk add --no-cache git ca-certificates curl imagemagick

# rsrc: embebe el manifest XML (Common Controls v6 + DPI awareness) y el ícono
# del .exe (lo que se ve en taskbar, Alt+Tab, escritorio).
RUN go install github.com/akavel/rsrc@latest

ENV CGO_ENABLED=0 \
    GOOS=windows \
    GOARCH=amd64 \
    GOFLAGS="-mod=mod"

WORKDIR /work

ENTRYPOINT ["/bin/sh", "-c", "\
set -e; \
VERSION=\"${VERSION:-dev}\"; \
mkdir -p dist; \
mkdir -p internal/embedded; \
SUMATRA=internal/embedded/SumatraPDF.exe; \
if [ ! -s \"$SUMATRA\" ]; then \
  echo 'Bajando SumatraPDF...'; \
  curl -fsSL -o \"$SUMATRA\" https://www.sumatrapdfreader.org/dl/rel/3.5.2/SumatraPDF-3.5.2-64.exe; \
fi; \
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
  -ldflags=\"-s -w -H=windowsgui -X main.version=$VERSION\" \
  -o dist/prinklyprint.exe .; \
echo \"OK -> dist/prinklyprint.exe v$VERSION\""]
