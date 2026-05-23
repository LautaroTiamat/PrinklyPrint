// Package printer expone dos capacidades:
//
//   - Enumeración de impresoras del sistema con su estado actual
//     (driver, puerto, ubicación, jobs en cola, flags PRINTER_STATUS_*).
//     Implementado con Win32 EnumPrinters via golang.org/x/sys/windows.
//     Decodifica 25 flags individuales (sin tóner, papel atascado, puerta
//     abierta, etc.) y los clasifica en severidades ok / warning / error.
//
//   - Impresión de PDFs vía SumatraPDF embebido. El binario de SumatraPDF
//     se extrae del .exe (go:embed) al primer arranque a
//     %LOCALAPPDATA%\PrinklyPrint\bin\ y se invoca como subproceso por cada
//     job. La traducción de [Options] a flags CLI de SumatraPDF está en
//     [BuildSumatraArgs] (test-friendly: función pura sin I/O).
//
// La parte Win32 está en printer_windows.go; en otros OS hay un stub para
// que el package compile y se puedan correr tests en Linux desde CI.
//
// Pre-flight check: [Service.CheckReady] consulta el estado de la impresora
// destino y rechaza el job (con mensaje claro) si tiene un flag bloqueante
// (sin tinta, sin papel, offline, puerta abierta, etc.). Esto evita gastar
// reintentos contra una impresora que claramente no puede imprimir.
package printer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lautarotiamat/prinklyprint/internal/embedded"
)

type Printer struct {
	Name       string   `json:"name"`
	IsDefault  bool     `json:"is_default"`
	IsNetwork  bool     `json:"is_network"`
	Status     string   `json:"status"`
	Statuses   []string `json:"statuses"`
	Severity   string   `json:"severity"`
	PortName   string   `json:"port_name,omitempty"`
	DriverName string   `json:"driver_name,omitempty"`
	Location   string   `json:"location,omitempty"`
	Comment    string   `json:"comment,omitempty"`
	JobCount   uint32   `json:"job_count"`
}

type Options struct {
	Printer        string  `json:"printer"`
	PaperSize      string  `json:"paper_size"`
	CustomWidthMM  float64 `json:"custom_width_mm"`
	CustomHeightMM float64 `json:"custom_height_mm"`
	Orientation    string  `json:"orientation"`
	Copies         int     `json:"copies"`
	Duplex         string  `json:"duplex"`
	Color          bool    `json:"color"`
	Scale          string  `json:"scale"`
	PageRange      string  `json:"page_range"`
}

type Service struct {
	sumatraPath string
	logger      *slog.Logger
}

func NewService(sumatraPath string, logger *slog.Logger) *Service {
	return &Service{sumatraPath: sumatraPath, logger: logger}
}

func (s *Service) SumatraPath() string { return s.sumatraPath }

func (s *Service) List(ctx context.Context) ([]Printer, error) {
	return listPrinters(ctx)
}

func (s *Service) Find(ctx context.Context, name string) (*Printer, error) {
	list, err := listPrinters(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if name == "" && list[i].IsDefault {
			return &list[i], nil
		}
		if name != "" && list[i].Name == name {
			return &list[i], nil
		}
	}
	return nil, nil
}

func (s *Service) CheckReady(ctx context.Context, name string) error {
	p, err := s.Find(ctx, name)
	if err != nil {
		return fmt.Errorf("consultar impresoras: %w", err)
	}
	if p == nil {
		if name == "" {
			return fmt.Errorf("no hay impresora default configurada en el sistema")
		}
		return fmt.Errorf("la impresora %q no existe en este sistema", name)
	}
	if IsBlocking(p.Statuses) {
		return fmt.Errorf("impresora %q no lista: %s", p.Name, BlockingReason(p.Statuses))
	}
	return nil
}

type PrintResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

func (s *Service) Print(ctx context.Context, pdfPath string, opts Options) (*PrintResult, error) {
	if s.sumatraPath == "" {
		return nil, fmt.Errorf("SumatraPDF no inicializado")
	}
	return runSumatra(ctx, s.sumatraPath, pdfPath, opts)
}

func EnsureSumatra(binDir string) (string, error) {
	if len(embedded.SumatraPDF) == 0 {
		return "", fmt.Errorf("binario de SumatraPDF no embebido — recompilá con -tags with_sumatra")
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("crear bin dir: %w", err)
	}
	dst := filepath.Join(binDir, "SumatraPDF.exe")
	if info, err := os.Stat(dst); err == nil && info.Size() == int64(len(embedded.SumatraPDF)) {
		return dst, nil
	}
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, embedded.SumatraPDF, 0o755); err != nil {
		return "", fmt.Errorf("escribir SumatraPDF: %w", err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		return "", fmt.Errorf("rename SumatraPDF: %w", err)
	}
	return dst, nil
}
