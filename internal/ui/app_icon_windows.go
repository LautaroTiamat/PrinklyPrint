//go:build windows

package ui

import (
	"bytes"
	_ "embed"
	"image/png"

	"github.com/lxn/walk"
)

// app_icon_windows.go — ícono de la app que walk usa para la ventana principal.
//
// El ícono del .exe (taskbar, Alt+Tab, escritorio) se embebe vía rsrc en build
// time. Pero walk.MainWindow no lee ese ícono automáticamente: hay que pasarle
// un *walk.Icon explícito a su campo Icon, sino la ventana arranca sin ícono
// y se ve el placeholder vacío en la barra de título.
//
// Cargamos el PNG embebido y lo convertimos a Icon. walk.NewIconFromImageForDPI
// genera internamente la representación que necesita (HICON via CreateIconFromResourceEx).

//go:embed assets/app.png
var appIconPNG []byte

// loadAppIcon decodifica el PNG embebido y lo convierte en *walk.Icon.
// Si algo falla, devuelve nil — walk tolera Icon=nil (queda sin ícono pero
// la app sigue funcionando).
func loadAppIcon() *walk.Icon {
	img, err := png.Decode(bytes.NewReader(appIconPNG))
	if err != nil {
		return nil
	}
	// 96 DPI es el valor estándar. Windows escala automáticamente para DPI más altos.
	icon, err := walk.NewIconFromImageForDPI(img, 96)
	if err != nil {
		return nil
	}
	return icon
}
