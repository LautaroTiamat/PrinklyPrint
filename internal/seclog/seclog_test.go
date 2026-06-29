package seclog

import "testing"

type capturedEvent struct {
	level Level
	id    uint32
	msg   string
}

type fakeSink struct{ events []capturedEvent }

func (f *fakeSink) Emit(l Level, id uint32, msg string) error {
	f.events = append(f.events, capturedEvent{l, id, msg})
	return nil
}
func (f *fakeSink) Close() error { return nil }

func TestEmitsToSink(t *testing.T) {
	fs := &fakeSink{}
	lg := New(nil, fs)

	lg.AuthFailure("/print", "https://app.example", "token inválido", true)
	lg.PairingApproved("https://app.example", "Mi App")
	lg.PairingDenied("https://evil.example", "el operador rechazó el pareo")
	lg.PrintEnqueued("job-1", "factura.pdf", "https://app.example")

	if len(fs.events) != 4 {
		t.Fatalf("esperaba 4 eventos, obtuve %d", len(fs.events))
	}
	want := []struct {
		id    uint32
		level Level
	}{
		{IDAuthFailure, LevelWarning},
		{IDPairingApproved, LevelInfo},
		{IDPairingDenied, LevelWarning},
		{IDPrintEnqueued, LevelInfo},
	}
	for i, w := range want {
		if fs.events[i].id != w.id {
			t.Errorf("evento %d: id=%d, quiero %d", i, fs.events[i].id, w.id)
		}
		if fs.events[i].level != w.level {
			t.Errorf("evento %d: level=%d, quiero %d", i, fs.events[i].level, w.level)
		}
	}
}

func TestNilLoggerIsSafe(t *testing.T) {
	var lg *Logger // nil: el server lo llama así si no se configuró SecLog
	lg.AuthFailure("/print", "o", "r", false)
	lg.PairingApproved("o", "l")
	lg.PrintEnqueued("j", "f", "o")
	if err := lg.Close(); err != nil {
		t.Errorf("Close en nil: %v", err)
	}
}

func TestFormatLineIncludesFields(t *testing.T) {
	line := formatLine("auth_failure", "rechazo", "path", "/print", "reason", "token inválido")
	if line == "" {
		t.Fatal("línea vacía")
	}
	for _, want := range []string{"auth_failure", "path=/print", "reason=token inválido"} {
		if !contains(line, want) {
			t.Errorf("la línea %q debería contener %q", line, want)
		}
	}
}

func TestSanitizeFieldBlocksLogForging(t *testing.T) {
	// Un filename con CR/LF no debe poder partir/forjar líneas del Event Log.
	line := formatLine("print_enqueued", "encolado", "filename", "x.pdf\nfake_event=1", "origin", "https://app")
	if contains(line, "\n") || contains(line, "\r") {
		t.Errorf("la línea no debería contener saltos de línea: %q", line)
	}
	if !contains(line, "x.pdf") {
		t.Errorf("debería conservar el contenido legible: %q", line)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
