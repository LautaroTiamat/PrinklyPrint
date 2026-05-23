//go:build windows

package app

import (
	"context"

	"github.com/lautarotiamat/prinklyprint/internal/tray"
	"github.com/lautarotiamat/prinklyprint/internal/ui"
)

// runUI arranca el tray + ventana nativa Win32.
func (a *App) runUI(ctx context.Context) error {
	win := &ui.Window{
		Store:      a.store,
		Config:     a.cfg,
		Printer:    a.printer,
		Queue:      a.queue,
		Logger:     a.logger.With("module", "ui"),
		DataDir:    a.dataDir,
		Version:    a.opts.Version,
		MachineID:  a.machineID,
		OnShutdown: a.RequestShutdown,
	}

	hooks := tray.Hooks{
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

	tr := tray.New(a.store, a.queue, a.cfg, a.logger.With("module", "tray"), hooks)
	go tr.Run(ctx)

	return win.Run(ctx)
}
