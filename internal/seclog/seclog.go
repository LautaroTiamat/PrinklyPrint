// Package seclog emite eventos de SEGURIDAD a dos destinos a la vez:
//
//   - el slog de archivo del agente (complementario, local), y
//   - el Windows Event Log (canal "Application", source "PrinklyPrint"), que es
//     lo que un SIEM corporativo recolecta de forma centralizada.
//
// Los eventos tienen IDs estables para correlación en el SIEM. NUNCA se loguea
// el valor del token ni el contenido de los PDFs.
//
// El destino nativo (Event Log) está detrás de la interfaz [Sink], inyectable
// para tests. La implementación real (Windows) está en eventlog_windows.go;
// fuera de Windows es no-op.
package seclog

import (
	"fmt"
	"log/slog"
	"strings"
)

// maxFieldLen acota cada valor que va a la línea del Event Log.
const maxFieldLen = 256

// Level mapea a las severidades del Event Log (Info/Warning/Error).
type Level int

const (
	LevelInfo Level = iota
	LevelWarning
	LevelError
)

// IDs estables de eventos de seguridad (rango 1000+). No reordenar/reusar:
// el SIEM correlaciona por estos números.
const (
	IDAuthFailure     uint32 = 1001 // rechazo de acceso a endpoint sensible (401)
	IDPairingApproved uint32 = 1002 // el operador aprobó un origen
	IDPairingDenied   uint32 = 1003 // pareo rechazado
	IDPrintEnqueued   uint32 = 1004 // se encoló un job de impresión
	IDSettingsChanged uint32 = 1005 // cambió la configuración del agente
	IDTokenRotated    uint32 = 1006 // se rotó el token de la instalación
	IDInsecureMode    uint32 = 1007 // el agente arrancó con un modo inseguro activo
)

// Sink es el destino nativo de eventos (Windows Event Log). Inyectable para tests.
type Sink interface {
	Emit(level Level, eventID uint32, message string) error
	Close() error
}

// noopSink descarta los eventos (se usa fuera de Windows o si no se pudo abrir
// el Event Log).
type noopSink struct{}

func (noopSink) Emit(Level, uint32, string) error { return nil }
func (noopSink) Close() error                     { return nil }

// Logger emite eventos de seguridad a slog + Sink.
type Logger struct {
	log  *slog.Logger
	sink Sink
}

// New crea un Logger. Si base es nil usa slog.Default(); si sink es nil usa un
// destino no-op (solo archivo).
func New(base *slog.Logger, sink Sink) *Logger {
	if base == nil {
		base = slog.Default()
	}
	if sink == nil {
		sink = noopSink{}
	}
	return &Logger{log: base.With("category", "security"), sink: sink}
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	return l.sink.Close()
}

// emit escribe el evento a ambos destinos. attrs son pares clave/valor para slog
// y se serializan como "k=v" en la línea del Event Log.
func (l *Logger) emit(level Level, id uint32, event, msg string, attrs ...any) {
	if l == nil {
		return
	}
	all := append([]any{"event", event, "event_id", id}, attrs...)
	switch level {
	case LevelError:
		l.log.Error(msg, all...)
	case LevelWarning:
		l.log.Warn(msg, all...)
	default:
		l.log.Info(msg, all...)
	}
	_ = l.sink.Emit(level, id, formatLine(event, msg, attrs...))
}

func formatLine(event, msg string, attrs ...any) string {
	line := "[" + event + "] " + msg
	for i := 0; i+1 < len(attrs); i += 2 {
		line += " " + toStr(attrs[i]) + "=" + toStr(attrs[i+1])
	}
	return line
}

func toStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return sanitizeField(x)
	default:
		return sanitizeField(fmt.Sprintf("%v", x))
	}
}

// sanitizeField evita log-forging en el Event Log: los valores controlados por
// el cliente (filename, header Origin) pueden traer saltos de línea o pares
// "k=v" falsos. Reemplazamos los caracteres de control por espacio y acotamos
// la longitud, así no se pueden forjar/partir líneas que el SIEM recolecta.
func sanitizeField(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if len(s) > maxFieldLen {
		s = strings.ToValidUTF8(s[:maxFieldLen], "") + "…"
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────
// Eventos. NUNCA incluir el token ni el contenido del PDF.
// ─────────────────────────────────────────────────────────────────────

// AuthFailure: un request a un endpoint sensible fue rechazado (401). reason
// describe el motivo (token faltante/inválido, origen no aprobado). hadOrigin
// indica si el request traía header Origin (navegador) o no (proceso local).
func (l *Logger) AuthFailure(path, origin, reason string, hadOrigin bool) {
	l.emit(LevelWarning, IDAuthFailure, "auth_failure", "acceso rechazado a endpoint sensible",
		"path", path, "origin", origin, "had_origin", boolStr(hadOrigin), "reason", reason)
}

// PairingApproved: el operador (o una pre-aprobación) habilitó un origen.
func (l *Logger) PairingApproved(origin, label string) {
	l.emit(LevelInfo, IDPairingApproved, "pairing_approved", "origen autorizado para imprimir",
		"origin", origin, "label", label)
}

// PairingDenied: el handshake de pairing fue rechazado.
func (l *Logger) PairingDenied(origin, reason string) {
	l.emit(LevelWarning, IDPairingDenied, "pairing_denied", "pareo rechazado",
		"origin", origin, "reason", reason)
}

// PrintEnqueued: se aceptó y encoló un job de impresión.
func (l *Logger) PrintEnqueued(jobID, filename, origin string) {
	l.emit(LevelInfo, IDPrintEnqueued, "print_enqueued", "job de impresión encolado",
		"job_id", jobID, "filename", filename, "origin", origin)
}

// SettingsChanged: cambió la configuración del agente (detail describe qué).
func (l *Logger) SettingsChanged(detail string) {
	l.emit(LevelInfo, IDSettingsChanged, "settings_changed", "configuración modificada", "detail", detail)
}

// TokenRotated: se regeneró el token de la instalación (invalida los cacheados).
func (l *Logger) TokenRotated() {
	l.emit(LevelWarning, IDTokenRotated, "token_rotated", "token de la instalación rotado")
}

// InsecureMode: el agente arrancó con un modo inseguro habilitado por el
// instalador (p. ej. "permitir cualquier origen"). detail describe cuál. Queda
// como WARNING en el SIEM para que sea visible que el equipo corre así.
func (l *Logger) InsecureMode(detail string) {
	l.emit(LevelWarning, IDInsecureMode, "insecure_mode_enabled", "modo inseguro habilitado", "detail", detail)
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
