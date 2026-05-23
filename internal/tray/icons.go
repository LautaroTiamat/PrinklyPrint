//go:build windows

package tray

import _ "embed"

// Los 3 íconos del tray se pre-generan durante el build con ImageMagick
// (ver docker-compose.yml → pinklyprint-build → step 3).
//
// Por qué pre-generar y no generar en runtime:
//   - Windows es muy estricto con el formato de bytes que acepta en
//     Shell_NotifyIcon. Nuestros encoders Go puro (PNG, ICO con PNG embedido,
//     ICO con BMP DIB) fallaban silenciosamente: el bitmap se rechazaba sin
//     error y el slot del tray quedaba vacío.
//   - ImageMagick produce ICOs multi-frame (16/24/32/48/64) que Windows
//     consume sin problemas. Es lo que usan todas las apps Win32 serias.
//
// Trade-off: agrega ~50 MB de imagemagick a la imagen Docker de build, pero
// el .exe final no crece — los 3 ICOs juntos pesan ~80 KB.
//
//go:embed assets/tray-green.ico
var iconGreen []byte

//go:embed assets/tray-yellow.ico
var iconYellow []byte

//go:embed assets/tray-red.ico
var iconRed []byte
