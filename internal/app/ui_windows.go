//go:build windows

package app

import (
	"context"
	"path/filepath"

	"github.com/lautarotiamat/prinklyprint/internal/tray"
	"github.com/lautarotiamat/prinklyprint/internal/ui"
)

// runUI arranca la ventana nativa Win32; el tray (walk.NotifyIcon) lo crea la
// propia ventana en su hilo de mensajes (ver ui.Window.Run).
func (a *App) runUI(ctx context.Context) error {
	win := &ui.Window{
		Store:          a.store,
		Config:         a.cfg,
		Printer:        a.printer,
		Queue:          a.queue,
		Logger:         a.logger.With("module", "ui"),
		DataDir:        a.dataDir,
		Version:        a.opts.Version,
		MachineID:      a.machineID,
		AllowAnyOrigin: a.allowAny,
		OnShutdown:     a.RequestShutdown,
		OnSettingsChanged: func(detail string) {
			a.secLog.SettingsChanged(detail)
		},
		OnRotateToken: func() error {
			if err := a.auth.RotateToken(); err != nil {
				return err
			}
			a.secLog.TokenRotated()
			return nil
		},
		TrayIconDir: filepath.Join(a.dataDir, "bin"),
	}

	win.TrayHooks = tray.Hooks{
		OnOpenMain:     func() { win.OpenTab(0) },
		OnOpenQueue:    func() { win.OpenTab(0) },
		OnOpenSettings: func() { win.OpenTab(2) },
		OnQuit: func() bool {
			// Confirmación nativa antes de matar el proceso.
			lang := a.cfg.Get().Language
			if !confirmQuit(lang) {
				return false
			}
			a.RequestShutdown()
			return true
		},
	}

	return win.Run(ctx)
}
