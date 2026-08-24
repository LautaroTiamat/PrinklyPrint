//go:build windows

package tray

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
	"github.com/lxn/walk"
)

// doubleClickWindow es la ventana para detectar el doble click izquierdo sobre
// el ícono. walk.NotifyIcon no publica WM_LBUTTONDBLCLK, así que lo detectamos
// midiendo el tiempo entre dos MouseUp izquierdos. 500 ms es el default de
// Windows (GetDoubleClickTime), que lxn/win no expone.
const doubleClickWindow = 500 * time.Millisecond

type Tray struct {
	ni     *walk.NotifyIcon
	form   walk.Form
	store  *store.Store
	queue  *queue.Worker
	cfg    *config.Manager
	logger *slog.Logger
	hooks  Hooks

	icons map[Color]*walk.Icon
	// current y currentLang solo se tocan en el hilo de UI (Attach y los
	// Synchronize de refresh), así que no necesitan mutex.
	current     Color
	currentLang i18n.Lang
	lastLeftUp  time.Time

	openAct, queueAct, settingsAct, pauseAct, quitAct *walk.Action
}

// Attach crea el ícono de bandeja colgado del form (la MainWindow). DEBE
// llamarse en el hilo de UI, después de crear la ventana. iconDir es donde se
// extraen los .ico embebidos (los archivos se reescriben en cada arranque).
func Attach(form walk.Form, iconDir string, st *store.Store, q *queue.Worker, cm *config.Manager, logger *slog.Logger, hooks Hooks) (*Tray, error) {
	t := &Tray{form: form, store: st, queue: q, cfg: cm, logger: logger, hooks: hooks}
	t.currentLang = i18n.Lang(cm.Get().Language)

	icons, err := extractIcons(iconDir)
	if err != nil {
		return nil, err
	}
	t.icons = icons

	ni, err := walk.NewNotifyIcon(form)
	if err != nil {
		return nil, fmt.Errorf("crear NotifyIcon: %w", err)
	}
	t.ni = ni

	if err := ni.SetIcon(icons[ColorGreen]); err != nil {
		_ = ni.Dispose()
		return nil, fmt.Errorf("setear ícono del tray: %w", err)
	}
	t.current = ColorGreen
	_ = ni.SetToolTip(i18n.T(t.currentLang, "tray.tooltip_ready"))

	t.buildMenu()

	// Doble click izquierdo → abrir la ventana. El click izquierdo simple no
	// hace nada y el menú queda SOLO en el click derecho (walk.NotifyIcon abre
	// el menú contextual únicamente con el botón derecho).
	ni.MouseUp().Attach(func(x, y int, button walk.MouseButton) {
		if button != walk.LeftButton {
			return
		}
		now := time.Now()
		if now.Sub(t.lastLeftUp) <= doubleClickWindow {
			t.lastLeftUp = time.Time{}
			if t.hooks.OnOpenMain != nil {
				t.hooks.OnOpenMain()
			}
			return
		}
		t.lastLeftUp = now
	})

	if err := ni.SetVisible(true); err != nil {
		_ = ni.Dispose()
		return nil, fmt.Errorf("mostrar ícono del tray: %w", err)
	}
	return t, nil
}

// buildMenu arma el menú contextual (click derecho). Corre en el hilo de UI.
func (t *Tray) buildMenu() {
	lang := t.currentLang
	add := func(key string, onTriggered func()) *walk.Action {
		a := walk.NewAction()
		_ = a.SetText(i18n.T(lang, key))
		a.Triggered().Attach(onTriggered)
		_ = t.ni.ContextMenu().Actions().Add(a)
		return a
	}
	sep := func() { _ = t.ni.ContextMenu().Actions().Add(walk.NewSeparatorAction()) }

	t.openAct = add("tray.open", func() {
		if t.hooks.OnOpenMain != nil {
			t.hooks.OnOpenMain()
		}
	})
	t.queueAct = add("tray.queue", func() {
		if t.hooks.OnOpenQueue != nil {
			t.hooks.OnOpenQueue()
		}
	})
	t.settingsAct = add("tray.settings", func() {
		if t.hooks.OnOpenSettings != nil {
			t.hooks.OnOpenSettings()
		}
	})
	sep()
	t.pauseAct = add("tray.pause", func() { t.togglePause() })
	sep()
	t.quitAct = add("tray.quit", func() {
		if t.hooks.OnQuit != nil {
			_ = t.hooks.OnQuit()
		}
	})

	if t.queue.IsPaused() {
		_ = t.pauseAct.SetText(i18n.T(lang, "tray.resume"))
	}
}

func (t *Tray) togglePause() {
	l := i18n.Lang(t.cfg.Get().Language)
	if t.queue.IsPaused() {
		t.queue.Resume()
		_ = t.pauseAct.SetText(i18n.T(l, "tray.pause"))
	} else {
		t.queue.Pause()
		_ = t.pauseAct.SetText(i18n.T(l, "tray.resume"))
	}
	// Persistimos la pausa para que sobreviva reinicios y para que GET /settings
	// (que lee la config) coincida con GET /ping (que lee el worker).
	paused := t.queue.IsPaused()
	if err := t.cfg.Update(func(c *config.Config) { c.Paused = paused }); err != nil {
		t.logger.Error("persistir paused en config", "err", err)
	}
}

// Run mantiene el semáforo (verde/amarillo/rojo), el tooltip y los textos del
// menú al día. Al cancelarse el contexto, quita el ícono de la bandeja.
func (t *Tray) Run(ctx context.Context) {
	tick := time.NewTicker(3 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			t.form.Synchronize(func() { _ = t.ni.Dispose() })
			return
		case <-tick.C:
			t.refresh(ctx)
		}
	}
}

func (t *Tray) refresh(ctx context.Context) {
	// Las consultas al store se hacen acá (fuera del hilo de UI); solo la
	// aplicación de los cambios visuales pasa por Synchronize.
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
	lang := i18n.Lang(t.cfg.Get().Language)
	paused := t.queue.IsPaused()

	t.form.Synchronize(func() {
		if lang != t.currentLang {
			t.applyLanguage(lang)
		}
		t.setColor(color)
		_ = t.ni.SetToolTip(i18n.T(lang, tooltipKey))
		if paused {
			_ = t.pauseAct.SetText(i18n.T(lang, "tray.resume"))
		} else {
			_ = t.pauseAct.SetText(i18n.T(lang, "tray.pause"))
		}
	})
}

func (t *Tray) applyLanguage(lang i18n.Lang) {
	_ = t.openAct.SetText(i18n.T(lang, "tray.open"))
	_ = t.queueAct.SetText(i18n.T(lang, "tray.queue"))
	_ = t.settingsAct.SetText(i18n.T(lang, "tray.settings"))
	_ = t.quitAct.SetText(i18n.T(lang, "tray.quit"))
	t.currentLang = lang
}

func (t *Tray) setColor(c Color) {
	if t.current == c {
		return
	}
	if icon, ok := t.icons[c]; ok {
		if err := t.ni.SetIcon(icon); err != nil {
			return // reintenta en el próximo tick (current no cambia)
		}
		t.current = c
	}
}

// extractIcons escribe los .ico embebidos a disco y los carga como walk.Icon.
// walk no decodifica ICO desde memoria, así que pasan por archivo.
func extractIcons(dir string) (map[Color]*walk.Icon, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("crear dir de íconos del tray: %w", err)
	}
	out := make(map[Color]*walk.Icon, 3)
	for _, e := range []struct {
		color Color
		name  string
		data  []byte
	}{
		{ColorGreen, "tray-green.ico", iconGreen},
		{ColorYellow, "tray-yellow.ico", iconYellow},
		{ColorRed, "tray-red.ico", iconRed},
	} {
		p := filepath.Join(dir, e.name)
		if err := os.WriteFile(p, e.data, 0o600); err != nil {
			return nil, fmt.Errorf("extraer %s: %w", e.name, err)
		}
		icon, err := walk.NewIconFromFile(p)
		if err != nil {
			return nil, fmt.Errorf("cargar %s: %w", e.name, err)
		}
		out[e.color] = icon
	}
	return out, nil
}
