//go:build windows

// queue_tab.go construye la pestaña "Cola" de la ventana principal.
//
// Layout: arriba una toolbar con filtro por estado + botones de Refrescar y
// Limpiar histórico. Abajo, la tabla de jobs con botonera vertical a la
// derecha. Los botones contextuales (Detalle, Reintentar, Cancelar) se
// habilitan/deshabilitan según el estado del job seleccionado:
//
//   - Detalle: cualquier job seleccionado.
//   - Reintentar: solo si el job está failed.
//   - Cancelar job: solo si el job está queued.
//
// Auto-refresh: cada 2 segundos (desde [Window.autoRefreshLoop]) se recarga
// la tabla. La selección se preserva por ID — si el job seleccionado sigue
// existiendo después del refresh, se vuelve a seleccionar; si desapareció
// (purga, retención), la selección se pierde.

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/i18n"
	"github.com/lautarotiamat/prinklyprint/internal/store"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type QueueTab struct {
	st        *store.Store
	lang      i18n.Lang
	model     *jobsModel
	tv        *walk.TableView
	statusBox *walk.ComboBox

	btnDetail *walk.PushButton
	btnRetry  *walk.PushButton
	btnCancel *walk.PushButton
}

func NewQueueTab(st *store.Store, lang i18n.Lang) *QueueTab {
	return &QueueTab{st: st, lang: lang, model: &jobsModel{lang: lang}}
}

func (q *QueueTab) Page() TabPage {
	statusOptions := []string{
		i18n.T(q.lang, "status.all"),
		i18n.T(q.lang, "status.queued"),
		i18n.T(q.lang, "status.printing"),
		i18n.T(q.lang, "status.done"),
		i18n.T(q.lang, "status.failed"),
		i18n.T(q.lang, "status.cancelled"),
	}

	return TabPage{
		Title:  "📋  " + i18n.T(q.lang, "tab.queue"),
		Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 12}, Spacing: 10},
		Children: []Widget{
			Composite{
				Layout: HBox{Spacing: 10, MarginsZero: true},
				Children: []Widget{
					Label{Text: i18n.T(q.lang, "queue.filter")},
					ComboBox{
						AssignTo:     &q.statusBox,
						Model:        statusOptions,
						CurrentIndex: 0,
						MinSize:      Size{Width: 160},
						OnCurrentIndexChanged: func() { q.Refresh() },
					},
					PushButton{
						Text:      "↻  " + i18n.T(q.lang, "app.refresh"),
						OnClicked: func() { q.Refresh() },
					},
					HSpacer{},
					PushButton{
						Text: "🗑  " + i18n.T(q.lang, "queue.purge"),
						OnClicked: func() {
							if walk.MsgBox(nil, i18n.T(q.lang, "app.title"),
								i18n.T(q.lang, "queue.purge_confirm"),
								walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
								return
							}
							paths, err := q.st.PurgeAll(context.Background())
							if err != nil {
								walk.MsgBox(nil, i18n.T(q.lang, "app.error"), err.Error(), walk.MsgBoxIconError)
								return
							}
							for _, p := range paths {
								_ = remove(p)
							}
							q.Refresh()
						},
					},
				},
			},
			Composite{
				Layout: HBox{MarginsZero: true, Spacing: 10},
				Children: []Widget{
					TableView{
						AssignTo:         &q.tv,
						Model:            q.model,
						AlternatingRowBG: true,
						MultiSelection:   false,
						Columns: []TableViewColumn{
							{Title: i18n.T(q.lang, "queue.col.date"), Width: 140},
							{Title: i18n.T(q.lang, "queue.col.file"), Width: 200},
							{Title: i18n.T(q.lang, "queue.col.printer"), Width: 160},
							{Title: i18n.T(q.lang, "queue.col.status"), Width: 110},
							{Title: i18n.T(q.lang, "queue.col.attempts"), Width: 60, Alignment: AlignCenter},
						},
						OnItemActivated:       func() { q.openDetail() },
						OnCurrentIndexChanged: func() { q.updateButtonsForSelection() },
						MinSize:               Size{Height: 320},
					},
					Composite{
						Layout:  VBox{Spacing: 6, MarginsZero: true},
						MinSize: Size{Width: 150},
						MaxSize: Size{Width: 150},
						Children: []Widget{
							PushButton{
								AssignTo:  &q.btnDetail,
								Text:      "ⓘ  " + i18n.T(q.lang, "queue.detail"),
								OnClicked: func() { q.openDetail() },
								Enabled:   false,
							},
							PushButton{
								AssignTo: &q.btnRetry,
								Text:     "↻  " + i18n.T(q.lang, "queue.retry"),
								Enabled:  false,
								OnClicked: func() {
									j := q.selected()
									if j == nil {
										return
									}
									_ = q.st.RetryJob(context.Background(), j.ID)
									q.Refresh()
								},
							},
							PushButton{
								AssignTo: &q.btnCancel,
								Text:     "✕  " + i18n.T(q.lang, "queue.cancel_job"),
								Enabled:  false,
								OnClicked: func() {
									j := q.selected()
									if j == nil {
										return
									}
									_ = q.st.CancelJob(context.Background(), j.ID)
									q.Refresh()
								},
							},
							VSpacer{},
						},
					},
				},
			},
		},
	}
}

func (q *QueueTab) updateButtonsForSelection() {
	j := q.selected()
	hasSel := j != nil
	if q.btnDetail != nil {
		q.btnDetail.SetEnabled(hasSel)
	}
	if q.btnRetry != nil {
		q.btnRetry.SetEnabled(hasSel && j.Status == store.StatusFailed)
	}
	if q.btnCancel != nil {
		q.btnCancel.SetEnabled(hasSel && j.Status == store.StatusQueued)
	}
}

// Refresh recarga los jobs preservando la selección actual por ID.
func (q *QueueTab) Refresh() {
	if q.tv == nil {
		return
	}
	filter := store.Status("")
	if q.statusBox != nil {
		switch q.statusBox.CurrentIndex() {
		case 1:
			filter = store.StatusQueued
		case 2:
			filter = store.StatusPrinting
		case 3:
			filter = store.StatusDone
		case 4:
			filter = store.StatusFailed
		case 5:
			filter = store.StatusCancelled
		}
	}
	jobs, _, err := q.st.ListJobs(context.Background(), store.ListJobsFilter{Status: filter, Limit: 200})
	if err != nil {
		return
	}
	sort.Slice(jobs, func(i, j int) bool { return jobs[i].CreatedAt.After(jobs[j].CreatedAt) })

	prevID := ""
	if curIdx := q.tv.CurrentIndex(); curIdx >= 0 && curIdx < len(q.model.jobs) {
		prevID = q.model.jobs[curIdx].ID
	}

	q.model.set(jobs)

	if prevID != "" {
		for i, j := range jobs {
			if j.ID == prevID {
				_ = q.tv.SetCurrentIndex(i)
				break
			}
		}
	}
	q.updateButtonsForSelection()
}

func (q *QueueTab) selected() *store.Job {
	if q.tv == nil || q.model == nil {
		return nil
	}
	idx := q.tv.CurrentIndex()
	if idx < 0 || idx >= len(q.model.jobs) {
		return nil
	}
	return &q.model.jobs[idx]
}

func (q *QueueTab) openDetail() {
	j := q.selected()
	if j == nil {
		return
	}
	body := buildDetailText(*j, q.lang)
	walk.MsgBox(nil, i18n.T(q.lang, "detail.title")+" — "+j.Filename, body, walk.MsgBoxIconInformation)
}

func buildDetailText(j store.Job, lang i18n.Lang) string {
	stKey := "status." + string(j.Status)
	statusLabel := i18n.T(lang, stKey)
	if statusLabel == stKey {
		statusLabel = string(j.Status)
	}

	var optsPretty string
	var opts map[string]any
	if err := json.Unmarshal([]byte(j.OptionsJSON), &opts); err == nil {
		b, _ := json.MarshalIndent(opts, "", "  ")
		optsPretty = string(b)
	}

	attemptsLine := ""
	if j.Attempts == 0 && j.Status == store.StatusDone {
		attemptsLine = " · " + i18n.T(lang, "detail.first_try")
	} else if j.Attempts > 0 {
		attemptsLine = fmt.Sprintf(" · %d %s", j.Attempts, i18n.T(lang, "queue.col.attempts"))
	}

	s := fmt.Sprintf("%s %s\n%s\n\n%s %s%s\n%s %s\n%s %s\n",
		i18n.T(lang, "detail.id"), j.ID,
		j.Filename,
		i18n.T(lang, "detail.state"), statusLabel, attemptsLine,
		i18n.T(lang, "detail.created"), j.CreatedAt.Local().Format(time.RFC822),
		i18n.T(lang, "detail.printer"), nonEmpty(j.Printer, "(default)"),
	)
	if j.CompletedAt != nil {
		s += fmt.Sprintf("%s %s\n", i18n.T(lang, "detail.completed"), j.CompletedAt.Local().Format(time.RFC822))
	}
	if optsPretty != "" {
		s += "\nOptions:\n" + optsPretty + "\n"
	}
	if j.LastError != "" {
		s += "\n" + i18n.T(lang, "detail.last_error") + ":\n" + j.LastError + "\n"
	}
	if j.SumatraLog != "" {
		s += "\n" + i18n.T(lang, "detail.sumatra") + ":\n" + j.SumatraLog
	}
	return s
}

func nonEmpty(a, fallback string) string {
	if a == "" {
		return fallback
	}
	return a
}

type jobsModel struct {
	walk.TableModelBase
	jobs []store.Job
	lang i18n.Lang
}

func (m *jobsModel) set(jobs []store.Job) {
	m.jobs = jobs
	m.PublishRowsReset()
}

func (m *jobsModel) RowCount() int { return len(m.jobs) }

func (m *jobsModel) Value(row, col int) any {
	if row < 0 || row >= len(m.jobs) {
		return ""
	}
	j := m.jobs[row]
	switch col {
	case 0:
		return j.CreatedAt.Local().Format("2006-01-02 15:04:05")
	case 1:
		return j.Filename
	case 2:
		if j.Printer == "" {
			return "(default)"
		}
		return j.Printer
	case 3:
		prefix := ""
		switch j.Status {
		case store.StatusDone:
			prefix = "✓ "
		case store.StatusFailed:
			prefix = "✕ "
		case store.StatusPrinting:
			prefix = "⟳ "
		case store.StatusQueued:
			prefix = "○ "
		case store.StatusCancelled:
			prefix = "− "
		}
		k := "status." + string(j.Status)
		v := i18n.T(m.lang, k)
		if v == k {
			v = string(j.Status)
		}
		return prefix + v
	case 4:
		return j.Attempts
	}
	return ""
}
