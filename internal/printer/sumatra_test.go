package printer

import (
	"strings"
	"testing"
)

// settingsValue extrae el valor del flag -print-settings de los args generados
// para SumatraPDF. Devuelve ("", false) si el flag no está presente.
func settingsValue(args []string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-print-settings" {
			return args[i+1], true
		}
	}
	return "", false
}

// printToOf devuelve la porción "-print-to <impresora>" o "-print-to-default".
func printToOf(args []string) string {
	if len(args) >= 2 && args[0] == "-print-to" {
		return args[0] + " " + args[1]
	}
	if len(args) >= 1 {
		return args[0]
	}
	return ""
}

// TestBuildSumatraArgs_Mapping fija el contrato de traducción de Options a los
// flags de SumatraPDF. Es el punto donde las opciones del cliente se vuelven el
// comando real, así que cualquier cambio acá debe ser intencional.
func TestBuildSumatraArgs_Mapping(t *testing.T) {
	const pdf = "C:\\tmp\\doc.pdf"
	cases := []struct {
		name         string
		opts         Options
		wantPrintTo  string
		wantSettings string
	}{
		{
			// Sin printer, Color=false → monochrome, Scale="" → fit (siempre uno de
			// cada). Sin paper/orientation/duplex/copies/page_range no aportan tokens.
			name:         "defaults",
			opts:         Options{},
			wantPrintTo:  "-print-to-default",
			wantSettings: "monochrome,fit",
		},
		{
			name:         "A4 landscape duplex-largo color 2 copias shrink",
			opts:         Options{Printer: "HP LaserJet", PaperSize: "A4", Orientation: "landscape", Duplex: "long_edge", Color: true, Copies: 2, Scale: "shrink"},
			wantPrintTo:  "-print-to HP LaserJet",
			wantSettings: "paper=A4,landscape,duplexlong,color,2x,shrink",
		},
		{
			name:         "letter duplex-corto noscale 1 copia (copies=1 no agrega Nx)",
			opts:         Options{PaperSize: "letter", Duplex: "short_edge", Color: true, Copies: 1, Scale: "noscale"},
			wantPrintTo:  "-print-to-default",
			wantSettings: "paper=letter,duplexshort,color,noscale",
		},
		{
			// paper_size desconocido (custom) → paperFlag vacío, no se agrega token.
			name:         "paper custom desconocido se omite",
			opts:         Options{PaperSize: "Rollo_80mm", Color: false},
			wantPrintTo:  "-print-to-default",
			wantSettings: "monochrome,fit",
		},
		{
			name:         "page_range válido aporta solo selección de páginas",
			opts:         Options{PageRange: "1,3-5,10"},
			wantPrintTo:  "-print-to-default",
			wantSettings: "monochrome,fit,1,3-5,10",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := BuildSumatraArgs(pdf, c.opts)
			if got := printToOf(args); got != c.wantPrintTo {
				t.Errorf("print-to = %q, quiero %q", got, c.wantPrintTo)
			}
			got, ok := settingsValue(args)
			if !ok {
				t.Fatalf("esperaba flag -print-settings en %v", args)
			}
			if got != c.wantSettings {
				t.Errorf("-print-settings = %q, quiero %q", got, c.wantSettings)
			}
			// Siempre debe terminar en "-silent <pdf>".
			if len(args) < 2 || args[len(args)-2] != "-silent" || args[len(args)-1] != pdf {
				t.Errorf("los args deberían terminar en -silent %q, son %v", pdf, args)
			}
		})
	}
}

// TestBuildSumatraArgs_CopiesFormat confirma el formato "Nx" de copias.
func TestBuildSumatraArgs_CopiesFormat(t *testing.T) {
	for _, copies := range []int{2, 5, 99} {
		args := BuildSumatraArgs("x.pdf", Options{Copies: copies})
		settings, _ := settingsValue(args)
		want := strconvN(copies) + "x"
		if !hasToken(settings, want) {
			t.Errorf("copies=%d: -print-settings %q debería contener %q", copies, settings, want)
		}
	}
	// copies<=1 NO debe agregar token de copias.
	for _, copies := range []int{0, 1} {
		args := BuildSumatraArgs("x.pdf", Options{Copies: copies})
		settings, _ := settingsValue(args)
		for _, tok := range strings.Split(settings, ",") {
			if strings.HasSuffix(tok, "x") && tok != "" {
				t.Errorf("copies=%d no debería producir token de copias, vi %q", copies, tok)
			}
		}
	}
}

// TestBuildSumatraArgs_PageRangeNoInjection es una REGRESIÓN DE SEGURIDAD.
//
// page_range se concatena verbatim dentro del string de -print-settings (que es
// una lista separada por comas que SumatraPDF parsea como tokens). El contrato de
// seguridad es: page_range solo puede aportar selección de páginas, NUNCA una
// directiva de impresión (paper=, landscape, color, monochrome, duplex*, Nx,
// fit/shrink/noscale). Eso se garantiza upstream con la whitelist rePageRange
// (^[0-9,\- ]*$) en internal/server/handlers.go: ningún carácter alfabético ni de
// puntuación puede llegar acá. Este test documenta y fija ese contrato.
func TestBuildSumatraArgs_PageRangeNoInjection(t *testing.T) {
	// Entrada que ya pasó la whitelist: solo dígitos, comas, guiones y espacios.
	args := BuildSumatraArgs("x.pdf", Options{PageRange: "1,3-5,10"})
	settings, ok := settingsValue(args)
	if !ok {
		t.Fatal("esperaba -print-settings")
	}

	// Los únicos tokens permitidos son los defaults que pedimos implícitamente
	// (monochrome, fit) más los tokens numéricos de páginas. Ninguna directiva.
	forbidden := []string{"paper=", "landscape", "duplexlong", "duplexshort", "color", "noscale", "shrink"}
	for _, f := range forbidden {
		if strings.Contains(settings, f) {
			t.Errorf("page_range no debería poder introducir la directiva %q (settings=%q)", f, settings)
		}
	}

	gotTokens := strings.Split(settings, ",")
	wantTokens := []string{"monochrome", "fit", "1", "3-5", "10"}
	if len(gotTokens) != len(wantTokens) {
		t.Fatalf("tokens=%v, quiero %v", gotTokens, wantTokens)
	}
	for i := range wantTokens {
		if gotTokens[i] != wantTokens[i] {
			t.Errorf("token[%d]=%q, quiero %q", i, gotTokens[i], wantTokens[i])
		}
	}
}

func hasToken(settings, token string) bool {
	for _, t := range strings.Split(settings, ",") {
		if t == token {
			return true
		}
	}
	return false
}

// strconvN evita importar strconv solo para esto en el test.
func strconvN(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
