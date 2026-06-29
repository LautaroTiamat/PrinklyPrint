// Package config gestiona la configuración persistente del agente.
//
// La config se serializa como YAML en %LOCALAPPDATA%\PrinklyPrint\config.yaml
// y se carga al arrancar. Es threadsafe: [Manager.Get] devuelve una copia
// inmutable del snapshot actual; [Manager.Update] aplica un mutator dentro
// de un mutex y persiste a disco antes de soltar el lock.
//
// El idioma de la UI se autodetecta del SO en la primera ejecución (cae a
// "en" si el idioma del SO no está en [SupportedLanguages]). El operador
// puede cambiarlo después desde la pestaña General.
//
// Validación: [Update] rechaza valores fuera de rango (puerto, retries,
// retención) o enum inválidos (orientation, duplex, scale, language) sin
// persistir el cambio.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/lautarotiamat/prinklyprint/internal/locale"
	"github.com/lautarotiamat/prinklyprint/internal/winfs"
	"gopkg.in/yaml.v3"
)

var SupportedLanguages = []string{"es", "en", "pt"}

type Config struct {
	Language       string   `yaml:"language" json:"language"`
	Port           int      `yaml:"port" json:"port"`
	AllowedOrigins []string `yaml:"allowed_origins" json:"allowed_origins"`
	// NOTA: el modo "permitir cualquier origen" (allow_any_origin) YA NO vive en
	// el config.yaml ni se activa desde la UI. Es una marca controlada solo por el
	// instalador (HKLM, ver internal/insecure) que el server lee al arrancar. Así
	// un operador no puede activar por error un modo inseguro editando el yaml.
	MaxRetries     int     `yaml:"max_retries" json:"max_retries"`
	RetentionDays  int     `yaml:"retention_days" json:"retention_days"`
	Paused         bool    `yaml:"paused" json:"paused"`
	AutoStart      bool    `yaml:"auto_start" json:"auto_start"`
	DefaultPrinter string  `yaml:"default_printer" json:"default_printer"`
	PaperSize      string  `yaml:"paper_size" json:"paper_size"`
	CustomWidthMM  float64 `yaml:"custom_width_mm" json:"custom_width_mm"`
	CustomHeightMM float64 `yaml:"custom_height_mm" json:"custom_height_mm"`
	Orientation    string  `yaml:"orientation" json:"orientation"`
	Color          bool    `yaml:"color" json:"color"`
	Duplex         string  `yaml:"duplex" json:"duplex"`
	Scale          string  `yaml:"scale" json:"scale"`

	// Rate limit de POST /pair (abuse-protection del pareo). Apagado por default.
	// NO afecta la impresión: /print tiene sus propios límites (tamaño de body y
	// profundidad de cola) y nunca pasa por este limitador.
	PairRateLimitEnabled   bool `yaml:"pair_rate_limit_enabled" json:"pair_rate_limit_enabled"`
	PairRateLimitPerMinute int  `yaml:"pair_rate_limit_per_minute" json:"pair_rate_limit_per_minute"`
	PairRateLimitBurst     int  `yaml:"pair_rate_limit_burst" json:"pair_rate_limit_burst"`
}

func Defaults() Config {
	return Config{
		Language:       locale.Detect(SupportedLanguages, "en"),
		Port:           17777,
		AllowedOrigins: []string{},
		MaxRetries:     1,
		RetentionDays:  7,
		Paused:         false,
		AutoStart:      true,
		PaperSize:      "A4",
		Orientation:    "portrait",
		Color:          true,
		Duplex:         "none",
		Scale:          "fit",
		// Rate limit de /pair: apagado por default; valores usados solo si se activa.
		PairRateLimitEnabled:   false,
		PairRateLimitPerMinute: 30,
		PairRateLimitBurst:     10,
	}
}

type Manager struct {
	path string
	mu   sync.RWMutex
	cfg  Config
}

func Load(path string) (*Manager, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("crear dir de config: %w", err)
	}
	m := &Manager{path: path, cfg: Defaults()}
	b, err := os.ReadFile(path) // #nosec G304 -- path interno de config; no proviene de input externo.
	switch {
	case os.IsNotExist(err):
		if err := m.persistLocked(); err != nil {
			return nil, err
		}
		return m, nil
	case err != nil:
		return nil, fmt.Errorf("leer config: %w", err)
	}
	var loaded Config
	if err := yaml.Unmarshal(b, &loaded); err != nil {
		return nil, fmt.Errorf("parsear config yaml: %w", err)
	}
	m.cfg = mergeDefaults(loaded)
	return m, nil
}

func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

func (m *Manager) Update(mutate func(*Config)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	next := m.cfg
	mutate(&next)
	if err := validate(next); err != nil {
		return err
	}
	m.cfg = next
	return m.persistLocked()
}

func (m *Manager) persistLocked() error {
	b, err := yaml.Marshal(m.cfg)
	if err != nil {
		return fmt.Errorf("serializar yaml: %w", err)
	}
	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("escribir config: %w", err)
	}
	if err := os.Rename(tmp, m.path); err != nil {
		return err
	}
	// Datos en reposo: el config puede tener orígenes aprobados y defaults;
	// lo restringimos owner-only (best-effort). Ver internal/winfs.
	_ = winfs.Restrict(m.path)
	return nil
}

func mergeDefaults(c Config) Config {
	d := Defaults()
	if c.Language == "" {
		c.Language = d.Language
	}
	{
		ok := false
		for _, s := range SupportedLanguages {
			if c.Language == s {
				ok = true
				break
			}
		}
		if !ok {
			c.Language = d.Language
		}
	}
	if c.Port == 0 {
		c.Port = d.Port
	}
	if c.MaxRetries == 0 {
		c.MaxRetries = d.MaxRetries
	}
	if c.RetentionDays == 0 {
		c.RetentionDays = d.RetentionDays
	}
	if c.PaperSize == "" {
		c.PaperSize = d.PaperSize
	}
	if c.Orientation == "" {
		c.Orientation = d.Orientation
	}
	if c.Duplex == "" {
		c.Duplex = d.Duplex
	}
	if c.Scale == "" {
		c.Scale = d.Scale
	}
	if c.AllowedOrigins == nil {
		c.AllowedOrigins = []string{}
	}
	// Defaults del rate limit de /pair para configs viejos (sin estos campos).
	if c.PairRateLimitPerMinute == 0 {
		c.PairRateLimitPerMinute = d.PairRateLimitPerMinute
	}
	if c.PairRateLimitBurst == 0 {
		c.PairRateLimitBurst = d.PairRateLimitBurst
	}
	return c
}

func validate(c Config) error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port fuera de rango: %d", c.Port)
	}
	if c.MaxRetries < 0 || c.MaxRetries > 20 {
		return fmt.Errorf("max_retries fuera de rango: %d", c.MaxRetries)
	}
	if c.RetentionDays < 1 || c.RetentionDays > 365 {
		return fmt.Errorf("retention_days fuera de rango: %d", c.RetentionDays)
	}
	switch c.Orientation {
	case "portrait", "landscape":
	default:
		return fmt.Errorf("orientation inválido: %s", c.Orientation)
	}
	switch c.Duplex {
	case "none", "long_edge", "short_edge":
	default:
		return fmt.Errorf("duplex inválido: %s", c.Duplex)
	}
	switch c.Scale {
	case "fit", "shrink", "noscale":
	default:
		return fmt.Errorf("scale inválido: %s", c.Scale)
	}
	switch c.Language {
	case "es", "en", "pt":
	default:
		return fmt.Errorf("language inválido: %s", c.Language)
	}
	// Si el rate limit de /pair está activo, tasa y burst deben ser positivos.
	if c.PairRateLimitEnabled {
		if c.PairRateLimitPerMinute <= 0 {
			return fmt.Errorf("pair_rate_limit_per_minute debe ser > 0 cuando el rate limit está activo: %d", c.PairRateLimitPerMinute)
		}
		if c.PairRateLimitBurst <= 0 {
			return fmt.Errorf("pair_rate_limit_burst debe ser > 0 cuando el rate limit está activo: %d", c.PairRateLimitBurst)
		}
	}
	return nil
}
