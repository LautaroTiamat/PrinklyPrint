//go:build windows

// print_settings_tab.go construye la pestaña "Impresión" — defaults de
// impresión que se aplican a jobs que no especifican opciones explícitas.
//
// Estos defaults los usa [server.resolveOptions] al recibir un POST /print:
// los campos que el cliente no envía toman el valor configurado acá. Los
// que sí envía prevalecen.
//
// La lista inferior muestra todas las impresoras del sistema con su estado
// actual (✓ ok / ⚠ warning / ✕ error) y se refresca cada 5 segundos para
// que el operador vea en tiempo real cuando arregla un papel atascado,
// cambia un cartucho, etc.
//
// El botón "Imprimir página de prueba" encola un PDF mínimo embebido en el
// código ([minimalTestPDF]) con la config actual. Útil para verificar que
// la impresora default está bien configurada sin pedirle a la web nada.

package ui

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type PrintSettingsTab struct {
	cfg           *config.Manager
	pr            *printer.Service
	q             *queue.Worker
	lang          i18n.Lang
	printerCombo  *walk.ComboBox
	paperCombo    *walk.ComboBox
	orientCombo   *walk.ComboBox
	colorCheck    *walk.CheckBox
	duplexCombo   *walk.ComboBox
	scaleCombo    *walk.ComboBox
	printerList   *walk.ListBox
	printersModel *printersModel
	printers      []printer.Printer
}

func NewPrintSettingsTab(cfg *config.Manager, pr *printer.Service, q *queue.Worker, lang i18n.Lang) *PrintSettingsTab {
	return &PrintSettingsTab{cfg: cfg, pr: pr, q: q, lang: lang, printersModel: &printersModel{lang: lang}}
}

func (p *PrintSettingsTab) Page() TabPage {
	// Labels visibles en el combo: nombres técnicos universales (A4/Letter/Legal/A5)
	// no se traducen; solo "Custom" tiene una traducción local. Los valores que se
	// persisten en config siguen siendo las keys fijas ("A4", "Letter", ...).
	paperOptions := []string{"A4", "Letter", "Legal", "A5", i18n.T(p.lang, "ps.paper.custom")}
	orientOptions := []string{i18n.T(p.lang, "ps.portrait"), i18n.T(p.lang, "ps.landscape")}
	duplexOptions := []string{
		i18n.T(p.lang, "ps.duplex.none"),
		i18n.T(p.lang, "ps.duplex.long"),
		i18n.T(p.lang, "ps.duplex.short"),
	}
	scaleOptions := []string{
		i18n.T(p.lang, "ps.scale.fit"),
		i18n.T(p.lang, "ps.scale.shrink"),
		i18n.T(p.lang, "ps.scale.noscale"),
	}

	c := p.cfg.Get()

	return TabPage{
		Title:  "🖨️  " + i18n.T(p.lang, "tab.print_settings"),
		Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 12}, Spacing: 12},
		Children: []Widget{
			GroupBox{
				Title:  i18n.T(p.lang, "tab.print_settings"),
				Layout: Grid{Columns: 2, Spacing: 10, Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
				Children: []Widget{
					Label{Text: i18n.T(p.lang, "ps.default_printer")},
					ComboBox{AssignTo: &p.printerCombo, MinSize: Size{Width: 320}, OnCurrentIndexChanged: func() { p.persist() }},

					Label{Text: i18n.T(p.lang, "ps.paper_size")},
					ComboBox{AssignTo: &p.paperCombo, Model: paperOptions, MinSize: Size{Width: 140}, OnCurrentIndexChanged: func() { p.persist() }},

					Label{Text: i18n.T(p.lang, "ps.orientation")},
					ComboBox{AssignTo: &p.orientCombo, Model: orientOptions, MinSize: Size{Width: 140}, OnCurrentIndexChanged: func() { p.persist() }},

					Label{Text: i18n.T(p.lang, "ps.color")},
					CheckBox{AssignTo: &p.colorCheck, Checked: c.Color, OnCheckedChanged: func() { p.persist() }},

					Label{Text: i18n.T(p.lang, "ps.duplex")},
					ComboBox{AssignTo: &p.duplexCombo, Model: duplexOptions, MinSize: Size{Width: 140}, OnCurrentIndexChanged: func() { p.persist() }},

					Label{Text: i18n.T(p.lang, "ps.scale")},
					ComboBox{AssignTo: &p.scaleCombo, Model: scaleOptions, MinSize: Size{Width: 180}, OnCurrentIndexChanged: func() { p.persist() }},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					PushButton{
						Text: "🖨️  " + i18n.T(p.lang, "ps.print_test"),
						OnClicked: func() {
							if err := p.printTest(); err != nil {
								walk.MsgBox(nil, i18n.T(p.lang, "app.error"), err.Error(), walk.MsgBoxIconError)
								return
							}
							walk.MsgBox(nil, i18n.T(p.lang, "app.title"), i18n.T(p.lang, "ps.test_queued"), walk.MsgBoxIconInformation)
						},
					},
					HSpacer{},
				},
			},
			GroupBox{
				Title:  i18n.T(p.lang, "ps.printers_title"),
				Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}},
				Children: []Widget{
					ListBox{
						AssignTo: &p.printerList,
						Model:    p.printersModel,
						MinSize:  Size{Height: 140},
					},
				},
			},
		},
	}
}

func (p *PrintSettingsTab) persist() {
	if p.printerCombo == nil {
		return
	}
	prn := ""
	if p.printerCombo.CurrentIndex() > 0 && p.printerCombo.CurrentIndex()-1 < len(p.printers) {
		prn = p.printers[p.printerCombo.CurrentIndex()-1].Name
	}
	paper := "A4"
	if i := p.paperCombo.CurrentIndex(); i >= 0 {
		paper = []string{"A4", "Letter", "Legal", "A5", "Custom"}[i]
	}
	orientation := "portrait"
	if p.orientCombo.CurrentIndex() == 1 {
		orientation = "landscape"
	}
	duplex := []string{"none", "long_edge", "short_edge"}[max0(p.duplexCombo.CurrentIndex())]
	scale := []string{"fit", "shrink", "noscale"}[max0(p.scaleCombo.CurrentIndex())]

	_ = p.cfg.Update(func(c *config.Config) {
		c.DefaultPrinter = prn
		c.PaperSize = paper
		c.Orientation = orientation
		c.Color = p.colorCheck.Checked()
		c.Duplex = duplex
		c.Scale = scale
	})
}

func max0(i int) int {
	if i < 0 {
		return 0
	}
	return i
}

func (p *PrintSettingsTab) RefreshPrinters() {
	list, err := p.pr.List(context.Background())
	if err != nil {
		return
	}
	p.printers = list
	p.printersModel.set(list)

	if p.printerCombo == nil {
		return
	}
	items := []string{i18n.T(p.lang, "ps.default_system")}
	for _, pr := range list {
		items = append(items, pr.Name)
	}
	_ = p.printerCombo.SetModel(items)
	cur := p.cfg.Get().DefaultPrinter
	if cur == "" {
		p.printerCombo.SetCurrentIndex(0)
	} else {
		for i, pr := range list {
			if pr.Name == cur {
				p.printerCombo.SetCurrentIndex(i + 1)
				break
			}
		}
	}
	p.applyConfigToControls()
}

func (p *PrintSettingsTab) applyConfigToControls() {
	c := p.cfg.Get()
	if p.paperCombo != nil {
		for i, v := range []string{"A4", "Letter", "Legal", "A5", "Custom"} {
			if v == c.PaperSize {
				p.paperCombo.SetCurrentIndex(i)
			}
		}
	}
	if p.orientCombo != nil {
		if c.Orientation == "landscape" {
			p.orientCombo.SetCurrentIndex(1)
		} else {
			p.orientCombo.SetCurrentIndex(0)
		}
	}
	if p.duplexCombo != nil {
		for i, v := range []string{"none", "long_edge", "short_edge"} {
			if v == c.Duplex {
				p.duplexCombo.SetCurrentIndex(i)
			}
		}
	}
	if p.scaleCombo != nil {
		for i, v := range []string{"fit", "shrink", "noscale"} {
			if v == c.Scale {
				p.scaleCombo.SetCurrentIndex(i)
			}
		}
	}
	if p.colorCheck != nil {
		p.colorCheck.SetChecked(c.Color)
	}
}

func (p *PrintSettingsTab) printTest() error {
	pdf := minimalTestPDF()
	b64 := base64.StdEncoding.EncodeToString(pdf)
	c := p.cfg.Get()
	_, err := p.q.Enqueue(context.Background(), queue.EnqueueParams{
		PDFBase64: b64,
		Filename:  fmt.Sprintf("test-%s.pdf", time.Now().Format("20060102-150405")),
		Options: printer.Options{
			Printer: c.DefaultPrinter, PaperSize: c.PaperSize, Orientation: c.Orientation,
			Color: c.Color, Duplex: c.Duplex, Scale: c.Scale, Copies: 1,
		},
		Metadata: map[string]any{"source": "ui_test_page"},
	})
	return err
}

func minimalTestPDF() []byte {
	return []byte(`%PDF-1.4
1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj
2 0 obj<</Type/Pages/Count 1/Kids[3 0 R]>>endobj
3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 595 842]/Contents 4 0 R/Resources<</Font<</F1 5 0 R>>>>>>endobj
4 0 obj<</Length 80>>stream
BT /F1 24 Tf 100 700 Td (PrinklyPrint - test page) Tj ET
BT /F1 12 Tf 100 670 Td (Si lees esto, todo funciona.) Tj ET
endstream
endobj
5 0 obj<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>endobj
xref
0 6
0000000000 65535 f
0000000009 00000 n
0000000052 00000 n
0000000095 00000 n
0000000183 00000 n
0000000310 00000 n
trailer<</Size 6/Root 1 0 R>>
startxref
365
%%EOF`)
}

type printersModel struct {
	walk.ListModelBase
	items []string
	lang  i18n.Lang
}

func (m *printersModel) set(list []printer.Printer) {
	m.items = make([]string, len(list))
	for i, p := range list {
		mark := "✓"
		if p.Severity == "warning" {
			mark = "⚠"
		} else if p.Severity == "error" {
			mark = "✕"
		}
		statusKey := "pstatus." + p.Status
		statusText := i18n.T(m.lang, statusKey)
		if statusText == statusKey {
			statusText = p.Status
		}
		def := ""
		if p.IsDefault {
			def = " (default)"
		}
		m.items[i] = fmt.Sprintf("%s   %s%s — %s", mark, p.Name, def, statusText)
	}
	m.PublishItemsReset()
}

func (m *printersModel) ItemCount() int  { return len(m.items) }
func (m *printersModel) Value(i int) any { return m.items[i] }
