// Package tray — ícono y menú de bandeja, traducido en vivo según
// config.Language.
//
// Implementación Windows: walk.NotifyIcon, colgado del HWND de la ventana
// principal (mismo hilo de mensajes que la UI). Esto reemplaza a
// fyne.io/systray, que abría el menú con CUALQUIER click; el comportamiento
// ahora es el estándar de Windows:
//
//   - Click derecho     → menú contextual.
//   - Doble click izq.  → abre la ventana principal (al frente).
//   - Click izq. simple → nada.
package tray

type Hooks struct {
	OnOpenQueue    func()
	OnOpenSettings func()
	OnOpenMain     func()
	OnQuit         func() bool
}

type Color int

const (
	// ColorNone es el valor inicial del campo current: garantiza que la primera
	// llamada a setColor aplique el ícono aunque el estado ya sea "verde".
	ColorNone Color = iota
	ColorGreen
	ColorYellow
	ColorRed
)
