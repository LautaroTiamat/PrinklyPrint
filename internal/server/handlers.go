package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/printer"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

// Límites de validación de entrada para /print.
const (
	maxCopies        = 999
	maxMetadataBytes = 16 << 10 // 16 KB
	maxQueueDepth    = 1000     // profundidad máxima de la cola (queued)
	maxPaperSizeLen  = 64
)

var (
	// page_range entra a -print-settings de SumatraPDF: whitelist estricta para
	// no inyectar directivas extra. Solo dígitos, comas, guiones y espacios.
	rePageRange = regexp.MustCompile(`^[0-9,\- ]*$`)
	// paper_size: la lib JS permite tamaños custom como string libre; en vez de
	// lista cerrada exigimos un charset seguro (orientation/duplex/scale SÍ son
	// enums cerrados, ver abajo).
	rePaperSize = regexp.MustCompile(`^[A-Za-z0-9 _-]+$`)
)

func validOrientation(s string) bool {
	switch s {
	case "", "portrait", "landscape":
		return true
	}
	return false
}

func validDuplex(s string) bool {
	switch s {
	case "", "none", "long_edge", "short_edge":
		return true
	}
	return false
}

func validScale(s string) bool {
	switch s {
	case "", "fit", "shrink", "noscale":
		return true
	}
	return false
}

func validPaperSize(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > maxPaperSizeLen {
		return false
	}
	return rePaperSize.MatchString(s)
}

// validatePrintRequest valida options + metadata + tamaños ANTES de encolar.
// Los magic bytes del PDF se validan en queue.Enqueue (sobre los bytes
// decodificados del base64).
func validatePrintRequest(req printRequest) error {
	o := req.Options
	if o.Copies > maxCopies {
		return fmt.Errorf("copies excede el máximo permitido (%d)", maxCopies)
	}
	if !validOrientation(o.Orientation) {
		return fmt.Errorf("orientation inválido: %q (use portrait|landscape)", o.Orientation)
	}
	if !validDuplex(o.Duplex) {
		return fmt.Errorf("duplex inválido: %q (use none|long_edge|short_edge)", o.Duplex)
	}
	if !validScale(o.Scale) {
		return fmt.Errorf("scale inválido: %q (use fit|shrink|noscale)", o.Scale)
	}
	if !validPaperSize(o.PaperSize) {
		return fmt.Errorf("paper_size inválido: solo letras, números, espacio, guion y guion bajo (máx %d)", maxPaperSizeLen)
	}
	if !rePageRange.MatchString(o.PageRange) {
		return fmt.Errorf(`page_range inválido: solo dígitos, comas, guiones y espacios (ej. "1,3-5,10")`)
	}
	if req.Metadata != nil {
		b, err := json.Marshal(req.Metadata)
		if err != nil {
			return fmt.Errorf("metadata no serializable: %w", err)
		}
		if len(b) > maxMetadataBytes {
			return fmt.Errorf("metadata excede el máximo permitido (%d bytes)", maxMetadataBytes)
		}
	}
	return nil
}

func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /ping", s.handlePing)
	mux.HandleFunc("POST /pair", s.handlePair)
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
	// /ping NO exige token: por eso devolvemos lo mínimo (liveness). machine_id NO
	// va acá — se expone solo en /settings, que sí exige token (ver handleGetSettings).
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": s.cfg.Version,
		"paused":  s.cfg.Queue.IsPaused(),
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
		// machine_id se expone solo acá (endpoint autenticado), ya no en /ping.
		"machine_id": s.cfg.MachineID,
	})
}

type printRequest struct {
	PDFBase64 string              `json:"pdf_base64"`
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
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validatePrintRequest(req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// Profundidad máxima de cola: evita acumulación/DoS de jobs pendientes.
	if n, err := s.cfg.Store.CountByStatus(r.Context(), store.StatusQueued); err == nil && n >= maxQueueDepth {
		writeErr(w, http.StatusTooManyRequests, "queue_full", "la cola está llena; reintentá más tarde")
		return
	}
	cfg := s.cfg.Config.Get()
	resolved := resolveOptions(req.Options, cfg)
	id, err := s.cfg.Queue.Enqueue(r.Context(), queue.EnqueueParams{
		PDFBase64: req.PDFBase64, Filename: req.Filename,
		Options: resolved, Metadata: req.Metadata,
	})
	if err != nil {
		// Errores de Enqueue son de input del cliente (pdf_base64 faltante,
		// base64 inválido, contenido que no es un PDF) → 400.
		writeErr(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	// Evento de seguridad: qué se encoló y desde qué origen.
	s.cfg.SecLog.PrintEnqueued(id, req.Filename, r.Header.Get("Origin"))
	writeJSON(w, http.StatusAccepted, map[string]string{"job_id": id, "status": "queued"})
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	// Bounds de paginación: limit en 1..500 (default 100), offset >= 0.
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
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
