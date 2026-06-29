//go:build !windows

// Fuera de Windows no hay Event Log: todo es no-op (solo dev/CI). Los eventos
// igual quedan en el slog de archivo vía Logger.
package seclog

// OpenEventLog devuelve un sink no-op fuera de Windows.
func OpenEventLog(source string) (Sink, error) { return noopSink{}, nil }

// RegisterEventSource es no-op fuera de Windows.
func RegisterEventSource(source string) error { return nil }

// UnregisterEventSource es no-op fuera de Windows.
func UnregisterEventSource(source string) error { return nil }
