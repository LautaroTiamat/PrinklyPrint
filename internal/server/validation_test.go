package server

import (
	"strings"
	"testing"
)

func TestValidatePrintRequest(t *testing.T) {
	bigMeta := map[string]any{"x": strings.Repeat("a", maxMetadataBytes+1)}

	cases := []struct {
		name    string
		req     printRequest
		wantErr bool
	}{
		{"vacío/defaults", printRequest{}, false},
		{"copies ok", printRequest{Options: printRequestOptions{Copies: 5}}, false},
		{"copies en el tope", printRequest{Options: printRequestOptions{Copies: maxCopies}}, false},
		{"copies excede", printRequest{Options: printRequestOptions{Copies: maxCopies + 1}}, true},
		{"copies enorme", printRequest{Options: printRequestOptions{Copies: 100000}}, true},
		{"orientation válida", printRequest{Options: printRequestOptions{Orientation: "landscape"}}, false},
		{"orientation inválida", printRequest{Options: printRequestOptions{Orientation: "diagonal"}}, true},
		{"duplex válido", printRequest{Options: printRequestOptions{Duplex: "long_edge"}}, false},
		{"duplex inválido", printRequest{Options: printRequestOptions{Duplex: "triple"}}, true},
		{"scale válido", printRequest{Options: printRequestOptions{Scale: "shrink"}}, false},
		{"scale inválido", printRequest{Options: printRequestOptions{Scale: "huge"}}, true},
		{"paper conocido", printRequest{Options: printRequestOptions{PaperSize: "A4"}}, false},
		{"paper custom flexible", printRequest{Options: printRequestOptions{PaperSize: "Rollo_80mm"}}, false},
		{"paper con inyección", printRequest{Options: printRequestOptions{PaperSize: "'; rm -rf"}}, true},
		{"page_range válido", printRequest{Options: printRequestOptions{PageRange: "1,3-5,10"}}, false},
		{"page_range con espacios", printRequest{Options: printRequestOptions{PageRange: "1, 3 - 5"}}, false},
		{"page_range con inyección", printRequest{Options: printRequestOptions{PageRange: "1;-print-settings x"}}, true},
		{"page_range con letras", printRequest{Options: printRequestOptions{PageRange: "1-a"}}, true},
		{"metadata gigante", printRequest{Metadata: bigMeta}, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validatePrintRequest(c.req)
			if (err != nil) != c.wantErr {
				t.Errorf("validatePrintRequest()=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestPaperSizeBounds(t *testing.T) {
	if validPaperSize(strings.Repeat("A", maxPaperSizeLen+1)) {
		t.Error("paper_size demasiado largo debería ser inválido")
	}
	if !validPaperSize("") {
		t.Error("paper_size vacío debería ser válido (default)")
	}
}
