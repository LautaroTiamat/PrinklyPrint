//go:build !windows

package printer

import "context"

func listPrinters(_ context.Context) ([]Printer, error) {
	return []Printer{{
		Name: "Microsoft Print to PDF", IsDefault: true, IsNetwork: false,
		Status: StatusReady, Statuses: []string{StatusReady}, Severity: string(SeverityOK),
	}}, nil
}
