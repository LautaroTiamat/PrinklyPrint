//go:build windows

package app

import (
	"syscall"
	"unsafe"
)

const (
	mbYesNo      = 0x00000004
	mbIconWarn   = 0x00000030
	mbDefButton2 = 0x00000100
	idYes        = 6
)

type localizedString struct{ Title, Body string }

var quitConfirm = map[string]localizedString{
	"es": {Title: "Cerrar PrinklyPrint", Body: "¿Estás seguro de que querés cerrar PrinklyPrint?\n\nEl agente se detendrá y la web no podrá imprimir hasta que lo vuelvas a iniciar."},
	"en": {Title: "Shut down PrinklyPrint", Body: "Are you sure you want to shut down PrinklyPrint?\n\nThe agent will stop and your web won't be able to print until you launch it again."},
	"pt": {Title: "Encerrar PrinklyPrint", Body: "Tem certeza que deseja encerrar o PrinklyPrint?\n\nO agente será interrompido e a web não poderá imprimir até reiniciá-lo."},
}

func confirmQuit(lang string) bool {
	s, ok := quitConfirm[lang]
	if !ok {
		s = quitConfirm["en"]
	}
	title, _ := syscall.UTF16PtrFromString(s.Title)
	body, _ := syscall.UTF16PtrFromString(s.Body)
	r, _, _ := procMessageBoxW.Call(
		0, uintptr(unsafe.Pointer(body)), uintptr(unsafe.Pointer(title)),
		uintptr(mbYesNo|mbIconWarn|mbDefButton2),
	)
	return int(r) == idYes
}
