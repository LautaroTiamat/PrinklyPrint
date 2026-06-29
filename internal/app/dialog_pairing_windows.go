//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Constantes de MessageBoxW que no están ya declaradas en dialog_windows.go
// (de ahí reusamos mbYesNo, mbDefButton2 e idYes).
const (
	mbIconQuestion  = 0x00000020
	mbSetForeground = 0x00010000
	mbTopMost       = 0x00040000
)

// confirmPairing muestra un diálogo modal nativo pidiéndole al operador que
// autorice (o rechace) que un origen web imprima por PrinklyPrint. El botón
// por defecto es "No" (mbDefButton2) para que un Enter accidental NO apruebe.
// Se puede llamar desde cualquier goroutine: MessageBoxW con hwnd=0 crea su
// propio loop de mensajes en el thread que llama.
func confirmPairing(lang, origin, label string) bool {
	title, body := pairingText(lang, origin, label)
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(body)
	r, _, _ := procMessageBoxW.Call(
		0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)),
		uintptr(mbYesNo|mbIconQuestion|mbDefButton2|mbSetForeground|mbTopMost),
	)
	return int(r) == idYes
}

func pairingText(lang, origin, label string) (title, body string) {
	app := origin
	if label != "" {
		app = fmt.Sprintf("%s (%s)", label, origin)
	}
	switch lang {
	case "es":
		return "PrinklyPrint — Autorizar impresión",
			fmt.Sprintf("¿Permitir que %s imprima en esta PC mediante PrinklyPrint?\n\nAprobá solo si reconocés esta aplicación.", app)
	case "pt":
		return "PrinklyPrint — Autorizar impressão",
			fmt.Sprintf("Permitir que %s imprima neste PC através do PrinklyPrint?\n\nAprove apenas se você reconhece este aplicativo.", app)
	default:
		return "PrinklyPrint — Authorize printing",
			fmt.Sprintf("Allow %s to print on this PC through PrinklyPrint?\n\nApprove only if you recognize this application.", app)
	}
}
