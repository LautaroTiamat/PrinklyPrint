package config

import (
	"path/filepath"
	"testing"
)

// TestConfigValidate cubre los rangos y enums que rechaza la validación de
// config (puerto, reintentos, retención, orientation/duplex/scale/language).
func TestConfigValidate(t *testing.T) {
	base := Defaults() // arranca de un snapshot válido y se tuerce un campo por caso
	cases := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"defaults", func(c *Config) {}, false},
		{"port mínimo", func(c *Config) { c.Port = 1 }, false},
		{"port máximo", func(c *Config) { c.Port = 65535 }, false},
		{"port 0", func(c *Config) { c.Port = 0 }, true},
		{"port fuera de rango", func(c *Config) { c.Port = 65536 }, true},
		{"retries 0", func(c *Config) { c.MaxRetries = 0 }, false},
		{"retries 20", func(c *Config) { c.MaxRetries = 20 }, false},
		{"retries negativo", func(c *Config) { c.MaxRetries = -1 }, true},
		{"retries excede", func(c *Config) { c.MaxRetries = 21 }, true},
		{"retención 1", func(c *Config) { c.RetentionDays = 1 }, false},
		{"retención 365", func(c *Config) { c.RetentionDays = 365 }, false},
		{"retención 0", func(c *Config) { c.RetentionDays = 0 }, true},
		{"retención excede", func(c *Config) { c.RetentionDays = 366 }, true},
		{"orientation inválida", func(c *Config) { c.Orientation = "diagonal" }, true},
		{"duplex inválido", func(c *Config) { c.Duplex = "triple" }, true},
		{"scale inválido", func(c *Config) { c.Scale = "huge" }, true},
		{"language inválido", func(c *Config) { c.Language = "fr" }, true},
		{"rate limit off con ceros (no valida)", func(c *Config) {
			c.PairRateLimitEnabled = false
			c.PairRateLimitPerMinute = 0
			c.PairRateLimitBurst = 0
		}, false},
		{"rate limit on válido", func(c *Config) {
			c.PairRateLimitEnabled = true
			c.PairRateLimitPerMinute = 30
			c.PairRateLimitBurst = 10
		}, false},
		{"rate limit on con per_min 0", func(c *Config) {
			c.PairRateLimitEnabled = true
			c.PairRateLimitPerMinute = 0
			c.PairRateLimitBurst = 10
		}, true},
		{"rate limit on con burst 0", func(c *Config) {
			c.PairRateLimitEnabled = true
			c.PairRateLimitPerMinute = 30
			c.PairRateLimitBurst = 0
		}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := base
			c.mutate(&cfg)
			if err := validate(cfg); (err != nil) != c.wantErr {
				t.Errorf("validate()=%v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

// TestConfigRoundTrip verifica que Load crea el archivo con defaults, que Update
// persiste a disco, y que una recarga ve los cambios.
func TestConfigRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.yaml")

	m, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := m.Get().Port; got != 17777 {
		t.Errorf("puerto default = %d, quiero 17777", got)
	}

	if err := m.Update(func(c *Config) {
		c.Port = 9090
		c.Paused = true
		c.AllowedOrigins = []string{"https://app.example"}
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}

	m2, err := Load(p) // recarga desde disco
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	g := m2.Get()
	if g.Port != 9090 {
		t.Errorf("puerto tras reload = %d, quiero 9090", g.Port)
	}
	if !g.Paused {
		t.Error("paused no persistió")
	}
	if len(g.AllowedOrigins) != 1 || g.AllowedOrigins[0] != "https://app.example" {
		t.Errorf("allowed_origins no persistió: %v", g.AllowedOrigins)
	}
}

// TestConfigUpdateRejectsInvalid confirma que un Update con un valor fuera de
// rango falla y NO persiste (el snapshot en memoria queda intacto).
func TestConfigUpdateRejectsInvalid(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := m.Update(func(c *Config) { c.Port = 70000 }); err == nil {
		t.Fatal("Update debería rechazar un puerto fuera de rango")
	}
	if got := m.Get().Port; got != 17777 {
		t.Errorf("un Update inválido no debería persistir; puerto=%d, quiero 17777", got)
	}
}
