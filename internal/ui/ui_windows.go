//go:build windows

// Package ui — ventana principal nativa Win32 vía lxn/walk.
//
// Decisión de tema: NO intervenimos. La app usa controles Win32 standard y
// hereda el theme del SO. Si el operador quiere dark mode, lo cambia desde
// Windows Settings → Personalization → Colors. Es lo más coherente porque
// todos los controles built-in respetan el system theme.
//
// Diferencias clave con la versión Wails:
//   - 100% Win32: usa Common Controls v6 (look "moderno" Windows).
//   - Sin WebView: no carga Chromium, ~15-25 MB de RAM en lugar de ~100 MB.
//   - Sin frontend separado: todo el render es código Go.
package ui

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type Window struct {
	Store     *store.Store
	Config    *config.Manager
	Printer   *printer.Service
	Queue     *queue.Worker
	Logger    *slog.Logger
	DataDir   string
	Version   string
	MachineID string

	OnShutdown func()

	mw            *walk.MainWindow
	tabWidget     *walk.TabWidget
	queueTab      *QueueTab
	printTab      *PrintSettingsTab
	generalTab    *GeneralTab
	currentLang   i18n.Lang
	openRequestCh chan int

	// shuttingDown distingue "el usuario apretó la X" (queremos minimizar al tray)
	// de "shutdown programático real" (queremos cerrar la ventana de verdad y dejar
	// que mw.Run() retorne). Sin esto, el handler de Closing() siempre cancela el
	// cierre y el proceso nunca termina.
	shuttingDown atomic.Bool
}

func (w *Window) Run(ctx context.Context) error {
	w.openRequestCh = make(chan int, 4)
	w.currentLang = i18n.Lang(w.Config.Get().Language)

	w.queueTab = NewQueueTab(w.Store, w.currentLang)
	w.printTab = NewPrintSettingsTab(w.Config, w.Printer, w.Queue, w.currentLang)
	w.generalTab = NewGeneralTab(generalDeps{
		cfg:          w.Config,
		lang:         w.currentLang,
		version:      w.Version,
		machineID:    w.MachineID,
		dataDir:      w.DataDir,
		onShutdown:   w.OnShutdown,
		onLangChange: func(l i18n.Lang) { w.applyLanguage(l) },
	})

	// El ícono aparece en barra de título, taskbar y Alt+Tab. Si la decodificación
	// del PNG falla por algún motivo, queda nil y walk simplemente no muestra ícono
	// (la app sigue funcionando).
	appIcon := loadAppIcon()

	if err := (MainWindow{
		AssignTo: &w.mw,
		Title:    i18n.T(w.currentLang, "app.title"),
		Icon:     appIcon,
		MinSize:  Size{Width: 860, Height: 600},
		Size:     Size{Width: 960, Height: 680},
		Layout:   VBox{MarginsZero: true},
		Visible:  false,
		Children: []Widget{
			TabWidget{
				AssignTo: &w.tabWidget,
				Pages: []TabPage{
					w.queueTab.Page(),
					w.printTab.Page(),
					w.generalTab.Page(),
				},
			},
		},
	}.Create()); err != nil {
		return err
	}

	// Goroutine: requests del tray + shutdown del ctx.
	go func() {
		for {
			select {
			case <-ctx.Done():
				// Apagado real: marcamos el flag para que el handler de Closing()
				// no intercepte el cierre, y disparamos el cierre desde el thread UI.
				w.shuttingDown.Store(true)
				w.mw.Synchronize(func() { w.mw.Close() })
				return
			case idx := <-w.openRequestCh:
				w.mw.Synchronize(func() {
					if idx >= 0 && idx <= 2 {
						w.tabWidget.SetCurrentIndex(idx)
					}
					w.mw.Show()
					_ = w.mw.SetFocus()
					w.queueTab.Refresh()
					w.printTab.RefreshPrinters()
				})
			}
		}
	}()

	go w.autoRefreshLoop(ctx)

	// Handler del cierre:
	//   - Si el usuario apretó la X de la ventana: cancelamos el cierre y la escondemos
	//     en el tray (la app sigue viva).
	//   - Si el shutdown vino programáticamente (botón "Cerrar PrinklyPrint" o "Salir" del
	//     tray): dejamos que la ventana cierre de verdad para que mw.Run() retorne
	//     y la goroutine de runUI() termine.
	w.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		if w.shuttingDown.Load() {
			return // permite el cierre
		}
		*canceled = true
		w.mw.Hide()
	})

	w.mw.Run()
	return nil
}

func (w *Window) OpenTab(idx int) {
	if w.openRequestCh != nil {
		select {
		case w.openRequestCh <- idx:
		default:
		}
	}
}

func (w *Window) applyLanguage(newLang i18n.Lang) {
	if newLang == w.currentLang {
		return
	}
	w.currentLang = newLang
	_ = w.Config.Update(func(c *config.Config) { c.Language = string(newLang) })
	walk.MsgBox(w.mw,
		i18n.T(newLang, "app.title"),
		map[i18n.Lang]string{
			i18n.ES: "Idioma cambiado. Reabrí la ventana para ver todos los textos traducidos.",
			i18n.EN: "Language changed. Reopen the window to see everything translated.",
			i18n.PT: "Idioma alterado. Reabra a janela para ver todos os textos traduzidos.",
		}[newLang],
		walk.MsgBoxIconInformation,
	)
}

func (w *Window) autoRefreshLoop(ctx context.Context) {
	go w.tickEvery(ctx, 2*time.Second, func() {
		if w.mw.Visible() {
			w.queueTab.Refresh()
		}
	})
	go w.tickEvery(ctx, 5*time.Second, func() {
		if w.mw.Visible() {
			w.printTab.RefreshPrinters()
		}
	})
}

func (w *Window) tickEvery(ctx context.Context, d time.Duration, fn func()) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn()
		}
	}
}
