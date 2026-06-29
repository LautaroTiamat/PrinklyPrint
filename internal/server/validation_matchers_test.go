package server

import "testing"

// TestInputMatchers fija el contrato de los validadores de enums y charset que
// usa validatePrintRequest. Complementa el test de alto nivel TestValidatePrintRequest.
func TestInputMatchers(t *testing.T) {
	check := func(name string, fn func(string) bool, valid, invalid []string) {
		for _, s := range valid {
			if !fn(s) {
				t.Errorf("%s(%q) = false; debería ser válido", name, s)
			}
		}
		for _, s := range invalid {
			if fn(s) {
				t.Errorf("%s(%q) = true; debería ser inválido", name, s)
			}
		}
	}

	check("validOrientation", validOrientation,
		[]string{"", "portrait", "landscape"},
		[]string{"diagonal", "PORTRAIT", "0", " landscape"})

	check("validDuplex", validDuplex,
		[]string{"", "none", "long_edge", "short_edge"},
		[]string{"both", "long", "1", "duplexlong"})

	check("validScale", validScale,
		[]string{"", "fit", "shrink", "noscale"},
		[]string{"huge", "100%", "zoom", "FIT"})

	check("validPaperSize", validPaperSize,
		[]string{"", "A4", "Letter", "Rollo_80mm", "custom-1"},
		[]string{"'; rm -rf", "A4\nlandscape", "a/b", "x=y", "a,b", "paper=A4"})
}

// TestRePageRangeContract es una regresión de seguridad: page_range se concatena
// dentro de -print-settings de SumatraPDF (lista separada por comas), así que la
// whitelist tiene que dejar pasar SOLO selección de páginas y rechazar cualquier
// carácter que permita inyectar una directiva.
func TestRePageRangeContract(t *testing.T) {
	ok := []string{"", "1", "1,3-5,10", "1, 3 - 5", "10-20"}
	for _, s := range ok {
		if !rePageRange.MatchString(s) {
			t.Errorf("page_range %q debería pasar la whitelist", s)
		}
	}
	// Intentos de inyección: cualquier letra o símbolo fuera de [0-9,\- ].
	bad := []string{"1;-print-settings x", "1,landscape", "1-a", "color", "1,monochrome", "1\n2", "1&2", "paper=A4"}
	for _, s := range bad {
		if rePageRange.MatchString(s) {
			t.Errorf("page_range %q NO debería pasar (riesgo de inyección a -print-settings)", s)
		}
	}
}
