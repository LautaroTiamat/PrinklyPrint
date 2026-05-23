//go:build windows

// Package tray — icono y menú de bandeja, traducido en vivo según config.Language.
package tray

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

type Hooks struct {
	OnOpenQueue    func()
	OnOpenSettings func()
	OnOpenMain     func()
	OnQuit         func() bool
}

type Color int

const (
	// ColorNone es el valor inicial del campo Tray.current. Sirve para que la
	// primera llamada a setColor(ColorGreen) NO se confunda con "ya está en verde,
	// no hagas nada" — si no, systray.SetIcon nunca se invoca al arrancar y el
	// ícono queda invisible hasta que el estado cambie a otro color.
	ColorNone Color = iota
	ColorGreen
	ColorYellow
	ColorRed
)

type Tray struct {
	store       *store.Store
	queue       *queue.Worker
	cfg         *config.Manager
	logger      *slog.Logger
	hooks       Hooks
	mu          sync.Mutex
	current     Color
	currentLang i18n.Lang
}

func New(st *store.Store, q *queue.Worker, cm *config.Manager, logger *slog.Logger, hooks Hooks) *Tray {
	return &Tray{store: st, queue: q, cfg: cm, logger: logger, hooks: hooks}
}

func (t *Tray) Run(ctx context.Context) {
	systray.Run(func() { t.onReady(ctx) }, func() {})
}

type menuItems struct {
	open, queue, settings, pause, quit *systray.MenuItem
}

func (t *Tray) onReady(ctx context.Context) {
	lang := i18n.Lang(t.cfg.Get().Language)
	t.currentLang = lang

	// IMPORTANTE: en fyne.io/systray sobre Windows, hay que llamar SetIcon ANTES
	// que cualquier otra cosa, sino el icono no se registra correctamente.
	// Setea directamente sin pasar por setColor (que tiene la guarda de cambio
	// y al inicio podría no disparar).
	systray.SetIcon(iconGreen)
	t.current = ColorGreen

	// En Windows, SetTitle pone texto AL LADO del ícono en la bandeja, lo cual
	// no queremos. Solo seteamos Tooltip (lo que aparece al hover). En otros OS
	// (linux/mac) SetTitle sí tiene su rol pero no nos afecta acá.
	systray.SetTooltip(i18n.T(lang, "tray.tooltip_ready"))

	items := &menuItems{
		open:     systray.AddMenuItem(i18n.T(lang, "tray.open"), ""),
		queue:    systray.AddMenuItem(i18n.T(lang, "tray.queue"), ""),
		settings: systray.AddMenuItem(i18n.T(lang, "tray.settings"), ""),
	}
	systray.AddSeparator()
	items.pause = systray.AddMenuItem(i18n.T(lang, "tray.pause"), "")
	systray.AddSeparator()
	items.quit = systray.AddMenuItem(i18n.T(lang, "tray.quit"), "")

	go t.refreshLoop(ctx, items)

	for {
		select {
		case <-ctx.Done():
			systray.Quit()
			return
		case <-items.open.ClickedCh:
			if t.hooks.OnOpenMain != nil {
				t.hooks.OnOpenMain()
			}
		case <-items.queue.ClickedCh:
			if t.hooks.OnOpenQueue != nil {
				t.hooks.OnOpenQueue()
			}
		case <-items.settings.ClickedCh:
			if t.hooks.OnOpenSettings != nil {
				t.hooks.OnOpenSettings()
			}
		case <-items.pause.ClickedCh:
			l := i18n.Lang(t.cfg.Get().Language)
			if t.queue.IsPaused() {
				t.queue.Resume()
				items.pause.SetTitle(i18n.T(l, "tray.pause"))
			} else {
				t.queue.Pause()
				items.pause.SetTitle(i18n.T(l, "tray.resume"))
			}
		case <-items.quit.ClickedCh:
			if t.hooks.OnQuit != nil {
				if !t.hooks.OnQuit() {
					continue
				}
			}
			systray.Quit()
			return
		}
	}
}

func (t *Tray) refreshLoop(ctx context.Context, items *menuItems) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			t.refresh(ctx, items)
		}
	}
}

func (t *Tray) refresh(ctx context.Context, items *menuItems) {
	lang := i18n.Lang(t.cfg.Get().Language)
	if lang != t.currentLang {
		t.applyLanguage(items, lang)
	}

	failed, _ := t.store.CountFailedSince(ctx, time.Now().Add(-24*time.Hour))
	queued, _ := t.store.CountByStatus(ctx, store.StatusQueued)
	printing, _ := t.store.CountByStatus(ctx, store.StatusPrinting)

	var color Color
	var tooltipKey string
	switch {
	case failed > 0:
		color = ColorRed
		tooltipKey = "tray.tooltip_failed"
	case queued > 0 || printing > 0:
		color = ColorYellow
		tooltipKey = "tray.tooltip_busy"
	default:
		color = ColorGreen
		tooltipKey = "tray.tooltip_ready"
	}
	t.setColor(color)
	systray.SetTooltip(i18n.T(lang, tooltipKey))

	if t.queue.IsPaused() {
		items.pause.SetTitle(i18n.T(lang, "tray.resume"))
	} else {
		items.pause.SetTitle(i18n.T(lang, "tray.pause"))
	}
}

func (t *Tray) applyLanguage(items *menuItems, lang i18n.Lang) {
	items.open.SetTitle(i18n.T(lang, "tray.open"))
	items.queue.SetTitle(i18n.T(lang, "tray.queue"))
	items.settings.SetTitle(i18n.T(lang, "tray.settings"))
	if t.queue.IsPaused() {
		items.pause.SetTitle(i18n.T(lang, "tray.resume"))
	} else {
		items.pause.SetTitle(i18n.T(lang, "tray.pause"))
	}
	items.quit.SetTitle(i18n.T(lang, "tray.quit"))
	t.currentLang = lang
}

func (t *Tray) setColor(c Color) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.current == c {
		return
	}
	t.current = c
	switch c {
	case ColorGreen:
		systray.SetIcon(iconGreen)
	case ColorYellow:
		systray.SetIcon(iconYellow)
	case ColorRed:
		systray.SetIcon(iconRed)
	}
}
