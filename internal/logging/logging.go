// Package logging — slog JSON con rotación diaria.
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lautarotiamat/prinklyprint/internal/winfs"
)

const retentionDays = 14

type FileRotator struct {
	dir         string
	prefix      string
	mu          sync.Mutex
	currentDay  string
	currentFile *os.File
}

func NewFileRotator(dir, prefix string) (*FileRotator, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("crear dir de logs: %w", err)
	}
	// Datos en reposo: los logs pueden tener metadata de jobs (filenames, errores).
	// Restringimos el dir owner-only (best-effort). Ver internal/winfs.
	_ = winfs.Restrict(dir)
	r := &FileRotator{dir: dir, prefix: prefix}
	if err := r.rotate(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *FileRotator) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if today := time.Now().Format("2006-01-02"); today != r.currentDay {
		if err := r.rotateLocked(); err != nil {
			return 0, err
		}
	}
	return r.currentFile.Write(p)
}

func (r *FileRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.currentFile != nil {
		return r.currentFile.Close()
	}
	return nil
}

func (r *FileRotator) rotate() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rotateLocked()
}

func (r *FileRotator) rotateLocked() error {
	if r.currentFile != nil {
		_ = r.currentFile.Close()
	}
	day := time.Now().Format("2006-01-02")
	name := filepath.Join(r.dir, fmt.Sprintf("%s-%s.log", r.prefix, day))
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600) // #nosec G304 -- name = dir interno + prefijo + fecha; no proviene de input externo.
	if err != nil {
		return fmt.Errorf("abrir log: %w", err)
	}
	_ = winfs.Restrict(name) // owner-only, best-effort
	r.currentFile = f
	r.currentDay = day
	go r.cleanup()
	return nil
}

func (r *FileRotator) cleanup() {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), r.prefix+"-") || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(r.dir, e.Name()))
		}
	}
}

func New(dir string) (*slog.Logger, io.Closer, error) {
	// Importante: NO usamos MultiWriter(stderr, rot) porque cuando el .exe
	// se compila con -H=windowsgui (sin consola), un write a stderr puede
	// fallar y MultiWriter aborta toda la escritura — el archivo de log
	// queda vacío. Solo escribimos a archivo (que es lo que el operador
	// puede revisar) cuando hay dir.
	if dir != "" {
		rot, err := NewFileRotator(dir, "agent")
		if err != nil {
			return nil, nil, err
		}
		h := slog.NewJSONHandler(rot, &slog.HandlerOptions{Level: slog.LevelInfo})
		return slog.New(h), rot, nil
	}
	h := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(h), io.NopCloser(os.Stderr), nil
}
