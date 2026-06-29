//go:build windows

package seclog

import "golang.org/x/sys/windows/svc/eventlog"

// winSink escribe al Windows Event Log (canal Application).
type winSink struct{ l *eventlog.Log }

// OpenEventLog abre el source (que debe estar registrado — ver RegisterEventSource,
// que corre el instalador con permisos de admin). Si falla (no registrado / sin
// permisos), el caller cae al sink no-op + archivo, sin abortar.
func OpenEventLog(source string) (Sink, error) {
	l, err := eventlog.Open(source)
	if err != nil {
		return nil, err
	}
	return &winSink{l: l}, nil
}

func (s *winSink) Emit(level Level, eid uint32, msg string) error {
	switch level {
	case LevelError:
		return s.l.Error(eid, msg)
	case LevelWarning:
		return s.l.Warning(eid, msg)
	default:
		return s.l.Info(eid, msg)
	}
}

func (s *winSink) Close() error { return s.l.Close() }

// RegisterEventSource registra el source en el registro de Windows (requiere
// admin). Lo corre el instalador (o `prinklyprint --register-eventlog` elevado).
// Usa el message file genérico de EventCreate, suficiente para mensajes de texto.
func RegisterEventSource(source string) error {
	return eventlog.InstallAsEventCreate(source, eventlog.Info|eventlog.Warning|eventlog.Error)
}

// UnregisterEventSource quita el registro del source (lo corre el desinstalador).
func UnregisterEventSource(source string) error {
	return eventlog.Remove(source)
}
