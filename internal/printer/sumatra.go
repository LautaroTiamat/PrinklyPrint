package printer

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

func BuildSumatraArgs(pdfPath string, opts Options) []string {
	args := []string{}
	if opts.Printer != "" {
		args = append(args, "-print-to", opts.Printer)
	} else {
		args = append(args, "-print-to-default")
	}
	var settings []string
	if ps := paperFlag(opts.PaperSize); ps != "" {
		settings = append(settings, ps)
	}
	if strings.EqualFold(opts.Orientation, "landscape") {
		settings = append(settings, "landscape")
	}
	switch opts.Duplex {
	case "long_edge":
		settings = append(settings, "duplexlong")
	case "short_edge":
		settings = append(settings, "duplexshort")
	}
	if !opts.Color {
		settings = append(settings, "monochrome")
	} else {
		settings = append(settings, "color")
	}
	if opts.Copies > 1 {
		settings = append(settings, strconv.Itoa(opts.Copies)+"x")
	}
	switch opts.Scale {
	case "shrink":
		settings = append(settings, "shrink")
	case "noscale":
		settings = append(settings, "noscale")
	case "fit", "":
		settings = append(settings, "fit")
	}
	if r := strings.TrimSpace(opts.PageRange); r != "" {
		settings = append(settings, r)
	}
	if len(settings) > 0 {
		args = append(args, "-print-settings", strings.Join(settings, ","))
	}
	args = append(args, "-silent", pdfPath)
	return args
}

func paperFlag(size string) string {
	switch strings.ToLower(size) {
	case "a4":
		return "paper=A4"
	case "letter":
		return "paper=letter"
	case "legal":
		return "paper=legal"
	case "a5":
		return "paper=A5"
	default:
		return ""
	}
}

func runSumatra(ctx context.Context, sumatraPath, pdfPath string, opts Options) (*PrintResult, error) {
	if runtime.GOOS != "windows" {
		return nil, fmt.Errorf("impresión solo soportada en Windows (OS actual: %s)", runtime.GOOS)
	}
	args := BuildSumatraArgs(pdfPath, opts)
	cmd := exec.CommandContext(ctx, sumatraPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := &PrintResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return res, fmt.Errorf("SumatraPDF salió con error: %w (stderr=%q)", err, res.Stderr)
	}
	return res, nil
}
