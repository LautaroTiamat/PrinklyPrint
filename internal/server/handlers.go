package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ping", s.handlePing)
	mux.HandleFunc("GET /printers", s.handlePrinters)
	mux.HandleFunc("GET /settings", s.handleGetSettings)
	mux.HandleFunc("POST /print", s.handlePrint)
	mux.HandleFunc("GET /jobs", s.handleListJobs)
	mux.HandleFunc("GET /jobs/{id}", s.handleGetJob)
	mux.HandleFunc("POST /jobs/{id}/retry", s.handleRetryJob)
	mux.HandleFunc("DELETE /jobs/{id}", s.handleCancelJob)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": code, "message": msg})
}

func (s *Server) handlePing(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"version":    s.cfg.Version,
		"machine_id": s.cfg.MachineID,
		"paused":     s.cfg.Queue.IsPaused(),
	})
}

func (s *Server) handlePrinters(w http.ResponseWriter, r *http.Request) {
	list, err := s.cfg.Printer.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_printers_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	c := s.cfg.Config.Get()
	writeJSON(w, http.StatusOK, map[string]any{
		"default_printer":  c.DefaultPrinter,
		"paper_size":       c.PaperSize,
		"custom_width_mm":  c.CustomWidthMM,
		"custom_height_mm": c.CustomHeightMM,
		"orientation":      c.Orientation,
		"color":            c.Color,
		"duplex":           c.Duplex,
		"scale":            c.Scale,
		"paused":           c.Paused,
	})
}

type printRequest struct {
	PDFBase64 string              `json:"pdf_base64"`
	PDFURL    string              `json:"pdf_url"`
	Filename  string              `json:"filename"`
	Options   printRequestOptions `json:"options"`
	Metadata  map[string]any      `json:"metadata"`
}

type printRequestOptions struct {
	Printer        string   `json:"printer"`
	PaperSize      string   `json:"paper_size"`
	CustomWidthMM  *float64 `json:"custom_width_mm,omitempty"`
	CustomHeightMM *float64 `json:"custom_height_mm,omitempty"`
	Orientation    string   `json:"orientation"`
	Copies         int      `json:"copies"`
	Duplex         string   `json:"duplex"`
	Color          *bool    `json:"color,omitempty"`
	Scale          string   `json:"scale"`
	PageRange      string   `json:"page_range"`
}

func resolveOptions(req printRequestOptions, cfg config.Config) printer.Options {
	out := printer.Options{
		Printer:     firstNonEmpty(req.Printer, cfg.DefaultPrinter),
		PaperSize:   firstNonEmpty(req.PaperSize, cfg.PaperSize),
		Orientation: firstNonEmpty(req.Orientation, cfg.Orientation),
		Duplex:      firstNonEmpty(req.Duplex, cfg.Duplex),
		Scale:       firstNonEmpty(req.Scale, cfg.Scale),
		PageRange:   req.PageRange,
		Copies:      req.Copies,
	}
	if out.Copies <= 0 {
		out.Copies = 1
	}
	if req.Color != nil {
		out.Color = *req.Color
	} else {
		out.Color = cfg.Color
	}
	if req.CustomWidthMM != nil {
		out.CustomWidthMM = *req.CustomWidthMM
	} else {
		out.CustomWidthMM = cfg.CustomWidthMM
	}
	if req.CustomHeightMM != nil {
		out.CustomHeightMM = *req.CustomHeightMM
	} else {
		out.CustomHeightMM = cfg.CustomHeightMM
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 50<<20)
	var req printRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_body", err.Error())
		return
	}
	cfg := s.cfg.Config.Get()
	resolved := resolveOptions(req.Options, cfg)
	id, err := s.cfg.Queue.Enqueue(r.Context(), queue.EnqueueParams{
		PDFBase64: req.PDFBase64, PDFURL: req.PDFURL, Filename: req.Filename,
		Options: resolved, Metadata: req.Metadata,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, "enqueue_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": id, "status": "queued"})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	f := store.ListJobsFilter{
		Status: store.Status(strings.TrimSpace(q.Get("status"))),
		Limit:  limit, Offset: offset,
	}
	jobs, total, err := s.cfg.Store.ListJobs(r.Context(), f)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total": total, "limit": f.Limit, "offset": f.Offset, "jobs": jobsToDTO(jobs),
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, err := s.cfg.Store.GetJob(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "not_found", "job no existe")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "get_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, jobToDTO(*job))
}

func (s *Server) handleRetryJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.cfg.Store.RetryJob(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found", "no se puede reintentar")
			return
		}
		writeErr(w, http.StatusInternalServerError, "retry_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "queued"})
}

func (s *Server) handleCancelJob(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.cfg.Store.CancelJob(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusNotFound, "not_found", "no se puede cancelar")
			return
		}
		writeErr(w, http.StatusInternalServerError, "cancel_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "cancelled"})
}

func jobToDTO(j store.Job) map[string]any {
	var opts, meta any
	_ = json.Unmarshal([]byte(j.OptionsJSON), &opts)
	_ = json.Unmarshal([]byte(j.MetadataJSON), &meta)
	dto := map[string]any{
		"id": j.ID, "filename": j.Filename, "printer": j.Printer,
		"options": opts, "metadata": meta, "status": j.Status,
		"attempts": j.Attempts, "last_error": j.LastError, "sumatra_log": j.SumatraLog,
		"created_at": j.CreatedAt, "updated_at": j.UpdatedAt,
	}
	if j.CompletedAt != nil {
		dto["completed_at"] = j.CompletedAt
	}
	if j.NextAttemptAt != nil {
		dto["next_attempt_at"] = j.NextAttemptAt
	}
	return dto
}

func jobsToDTO(in []store.Job) []map[string]any {
	out := make([]map[string]any, 0, len(in))
	for _, j := range in {
		out = append(out, jobToDTO(j))
	}
	return out
}
