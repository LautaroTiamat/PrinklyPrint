//go:build windows

// general_tab.go construye la pestaña "General" — configuración del agente.
//
// Secciones:
//
//   - Apariencia: selector de idioma (ES/EN/PT). El cambio requiere reabrir
//     la ventana para que todos los controles se reconstruyan en el idioma
//     nuevo (walk no permite re-titular widgets ya creados).
//
//   - Servidor HTTP: puerto donde escucha (default 17777). Cambios requieren
//     reiniciar el agente.
//
//   - Orígenes CORS: whitelist de dominios autorizados a llamar al server
//     local. El operador agrega/quita orígenes con LineEdit + dos botones.
//     El checkbox "Permitir cualquier origen" se usa solo para debugging.
//
//   - Cola: max retries (default 1) y días de retención (default 7).
//
//   - Información: versión del agente, machine ID, ruta del data dir.
//
//   - Botones de acción al final: "Abrir carpeta de logs" abre Explorer
//     en %LOCALAPPDATA%\PrinklyPrint\logs\, y "Cerrar PrinklyPrint" dispara
//     el apagado ordenado con confirmación.

package ui

import (
	"os/exec"
	"path/filepath"

	"github.com/lautarotiamat/prinklyprint/internal/autostart"
	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type generalDeps struct {
	cfg          *config.Manager
	lang         i18n.Lang
	version      string
	machineID    string
	dataDir      string
	onShutdown   func()
	onLangChange func(i18n.Lang)
}

type GeneralTab struct {
	d generalDeps

	langCombo      *walk.ComboBox
	portEdit       *walk.NumberEdit
	originsList    *walk.ListBox
	originsModel   *originsModel
	newOriginEdit  *walk.LineEdit
	allowAnyCheck  *walk.CheckBox
	autostartCheck *walk.CheckBox
	maxRetriesEdit *walk.NumberEdit
	retentionEdit  *walk.NumberEdit
}

func NewGeneralTab(d generalDeps) *GeneralTab {
	return &GeneralTab{d: d, originsModel: &originsModel{}}
}

func (g *GeneralTab) Page() TabPage {
	c := g.d.cfg.Get()
	g.originsModel.set(c.AllowedOrigins)

	langOptions := []string{"Español", "English", "Português"}
	langCurrent := 0
	switch i18n.Lang(c.Language) {
	case i18n.EN:
		langCurrent = 1
	case i18n.PT:
		langCurrent = 2
	}

	return TabPage{
		Title:  "⚙️  " + i18n.T(g.d.lang, "tab.general"),
		Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 12}, Spacing: 12},
		Children: []Widget{
			GroupBox{
				Title:  "🌐  " + i18n.T(g.d.lang, "gen.appearance"),
				Layout: Grid{Columns: 2, Spacing: 10, Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
				Children: []Widget{
					Label{Text: i18n.T(g.d.lang, "gen.language")},
					ComboBox{
						AssignTo:     &g.langCombo,
						Model:        langOptions,
						CurrentIndex: langCurrent,
						MinSize:      Size{Width: 200},
						OnCurrentIndexChanged: func() {
							var l i18n.Lang
							switch g.langCombo.CurrentIndex() {
							case 0:
								l = i18n.ES
							case 1:
								l = i18n.EN
							case 2:
								l = i18n.PT
							}
							if g.d.onLangChange != nil {
								g.d.onLangChange(l)
							}
						},
					},
				},
			},
			GroupBox{
				Title:  "🚀  " + i18n.T(g.d.lang, "gen.startup_title"),
				Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 4},
				Children: []Widget{
					CheckBox{
						AssignTo: &g.autostartCheck,
						Text:     i18n.T(g.d.lang, "gen.autostart"),
						Checked:  c.AutoStart,
						OnCheckedChanged: func() {
							v := g.autostartCheck.Checked()
							if err := g.d.cfg.Update(func(c *config.Config) { c.AutoStart = v }); err != nil {
								return
							}
							_ = autostart.Sync(v)
						},
					},
					Label{Text: i18n.T(g.d.lang, "gen.autostart_help"), TextColor: walk.RGB(110, 110, 110)},
				},
			},
			GroupBox{
				Title:  "🌐  " + i18n.T(g.d.lang, "gen.http_server"),
				Layout: HBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
				Children: []Widget{
					Label{Text: i18n.T(g.d.lang, "gen.port")},
					NumberEdit{
						AssignTo: &g.portEdit,
						Value:    float64(c.Port),
						MinValue: 1024, MaxValue: 65535,
						OnValueChanged: func() { g.persistMisc() },
						MinSize:        Size{Width: 120},
					},
					HSpacer{},
				},
			},
			GroupBox{
				Title:  "🔒  " + i18n.T(g.d.lang, "gen.cors_title"),
				Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
				Children: []Widget{
					Composite{
						Layout: HBox{MarginsZero: true, Spacing: 8},
						Children: []Widget{
							LineEdit{AssignTo: &g.newOriginEdit, CueBanner: "https://app.empresa.com"},
							PushButton{
								Text: "＋  " + i18n.T(g.d.lang, "gen.cors_add"),
								OnClicked: func() {
									v := g.newOriginEdit.Text()
									if v == "" {
										return
									}
									_ = g.d.cfg.Update(func(c *config.Config) {
										for _, x := range c.AllowedOrigins {
											if x == v {
												return
											}
										}
										c.AllowedOrigins = append(c.AllowedOrigins, v)
									})
									g.newOriginEdit.SetText("")
									g.originsModel.set(g.d.cfg.Get().AllowedOrigins)
								},
							},
							PushButton{
								Text: "✕  " + i18n.T(g.d.lang, "gen.cors_remove"),
								OnClicked: func() {
									if g.originsList == nil {
										return
									}
									idx := g.originsList.CurrentIndex()
									if idx < 0 || idx >= len(g.originsModel.items) {
										return
									}
									target := g.originsModel.items[idx]
									_ = g.d.cfg.Update(func(c *config.Config) {
										out := c.AllowedOrigins[:0]
										for _, x := range c.AllowedOrigins {
											if x != target {
												out = append(out, x)
											}
										}
										c.AllowedOrigins = out
									})
									g.originsModel.set(g.d.cfg.Get().AllowedOrigins)
								},
							},
						},
					},
					ListBox{AssignTo: &g.originsList, Model: g.originsModel, MinSize: Size{Height: 100}},
					CheckBox{
						AssignTo: &g.allowAnyCheck,
						Text:     i18n.T(g.d.lang, "gen.cors_allow_any"),
						Checked:  c.AllowAnyOrigin,
						OnCheckedChanged: func() {
							_ = g.d.cfg.Update(func(c *config.Config) { c.AllowAnyOrigin = g.allowAnyCheck.Checked() })
						},
					},
				},
			},
			GroupBox{
				Title:  "📋  " + i18n.T(g.d.lang, "gen.queue_title"),
				Layout: Grid{Columns: 2, Spacing: 10, Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
				Children: []Widget{
					Label{Text: i18n.T(g.d.lang, "gen.max_retries")},
					NumberEdit{
						AssignTo: &g.maxRetriesEdit,
						Value:    float64(c.MaxRetries),
						MinValue: 1, MaxValue: 20,
						OnValueChanged: func() { g.persistMisc() },
						MinSize:        Size{Width: 100},
					},
					Label{Text: i18n.T(g.d.lang, "gen.retention_days")},
					NumberEdit{
						AssignTo: &g.retentionEdit,
						Value:    float64(c.RetentionDays),
						MinValue: 1, MaxValue: 365,
						OnValueChanged: func() { g.persistMisc() },
						MinSize:        Size{Width: 100},
					},
				},
			},
			GroupBox{
				Title:  "ⓘ  " + i18n.T(g.d.lang, "gen.info_title"),
				Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
				Children: []Widget{
					Label{Text: i18n.T(g.d.lang, "gen.version")},
					Label{Text: g.d.version},
					Label{Text: i18n.T(g.d.lang, "gen.machine_id")},
					Label{Text: g.d.machineID},
					Label{Text: i18n.T(g.d.lang, "gen.data_dir")},
					Label{Text: g.d.dataDir},
					Label{Text: i18n.T(g.d.lang, "gen.author")},
					Label{Text: "LautaroTiamat"},
					Label{Text: i18n.T(g.d.lang, "gen.github")},
					LinkLabel{
						Text: `<a href="https://github.com/LautaroTiamat">github.com/LautaroTiamat</a>`,
						OnLinkActivated: func(link *walk.LinkLabelLink) {
							_ = exec.Command("cmd", "/c", "start", "", link.URL()).Start()
						},
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					PushButton{
						Text: "📂  " + i18n.T(g.d.lang, "gen.open_logs"),
						OnClicked: func() {
							_ = exec.Command("explorer", filepath.Join(g.d.dataDir, "logs")).Start()
						},
					},
					HSpacer{},
					PushButton{
						Text: "⏻  " + i18n.T(g.d.lang, "gen.shutdown"),
						OnClicked: func() {
							if walk.MsgBox(nil, i18n.T(g.d.lang, "app.title"),
								i18n.T(g.d.lang, "gen.shutdown_confirm"),
								walk.MsgBoxYesNo|walk.MsgBoxIconWarning|walk.MsgBoxDefButton2) != walk.DlgCmdYes {
								return
							}
							if g.d.onShutdown != nil {
								g.d.onShutdown()
							}
						},
					},
				},
			},
		},
	}
}

// Refresh re-lee los orígenes permitidos desde la config y actualiza la lista
// si cambió (por ejemplo, cuando un pareo aprobado agregó un origen nuevo).
// Lo llama el loop de auto-refresh mientras la ventana está visible, así el
// origen recién conectado aparece en la lista sin reabrir la ventana.
func (g *GeneralTab) Refresh() {
	if g.originsModel == nil {
		return
	}
	origins := g.d.cfg.Get().AllowedOrigins
	if originsEqual(g.originsModel.items, origins) {
		return // sin cambios: no tocamos la lista (evita resetear la selección)
	}
	g.originsModel.set(origins)
}

func originsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (g *GeneralTab) persistMisc() {
	if g.portEdit == nil {
		return
	}
	port := int(g.portEdit.Value())
	maxR := int(g.maxRetriesEdit.Value())
	ret := int(g.retentionEdit.Value())
	_ = g.d.cfg.Update(func(c *config.Config) {
		c.Port = port
		c.MaxRetries = maxR
		c.RetentionDays = ret
	})
}

type originsModel struct {
	walk.ListModelBase
	items []string
}

func (m *originsModel) set(items []string) {
	m.items = items
	m.PublishItemsReset()
}

func (m *originsModel) ItemCount() int  { return len(m.items) }
func (m *originsModel) Value(i int) any { return m.items[i] }
