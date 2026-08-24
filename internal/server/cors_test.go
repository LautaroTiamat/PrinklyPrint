package server

import "testing"

// TestNormalizeOrigin cubre la limpieza de entradas escritas por el operador.
func TestNormalizeOrigin(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://app.empresa.com", "https://app.empresa.com"},
		{"https://app.empresa.com/", "https://app.empresa.com"},
		{"https://app.empresa.com//", "https://app.empresa.com"},
		{"  https://app.empresa.com/ ", "https://app.empresa.com"},
		{"miapp.com/", "miapp.com"},
		{"*.empresa.com/", "*.empresa.com"},
		{"https://", "https://"}, // no comerse el "//" del esquema
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeOrigin(c.in); got != c.want {
			t.Errorf("NormalizeOrigin(%q) = %q, quiero %q", c.in, got, c.want)
		}
	}
}

// TestIsAllowedOrigin cubre el match con normalización: barra final, case,
// wildcard de subdominio y entradas sin esquema (matchean http y https).
func TestIsAllowedOrigin(t *testing.T) {
	cases := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{"exacto", "https://app.empresa.com", []string{"https://app.empresa.com"}, true},
		{"entrada con barra final", "https://app.empresa.com", []string{"https://app.empresa.com/"}, true},
		{"entrada con espacios", "https://app.empresa.com", []string{"  https://app.empresa.com  "}, true},
		{"case-insensitive", "https://App.Empresa.com", []string{"https://app.empresa.com"}, true},
		{"solo host matchea http", "http://miapp.com", []string{"miapp.com"}, true},
		{"solo host matchea https", "https://miapp.com", []string{"miapp.com"}, true},
		{"solo host con barra final", "https://miapp.com", []string{"miapp.com/"}, true},
		{"wildcard subdominio", "https://pos.empresa.com", []string{"*.empresa.com"}, true},
		{"wildcard subdominio http", "http://pos.empresa.com", []string{"*.empresa.com"}, true},
		{"wildcard con barra final", "https://pos.empresa.com", []string{"*.empresa.com/"}, true},
		{"wildcard case-insensitive", "https://POS.Empresa.com", []string{"*.empresa.com"}, true},
		{"wildcard no matchea el dominio pelado", "https://empresa.com", []string{"*.empresa.com"}, false},
		{"no matchea otro dominio", "https://atacante.com", []string{"https://app.empresa.com", "*.empresa.com"}, false},
		{"sufijo parecido no matchea", "https://malempresa.com", []string{"*.empresa.com"}, false},
		{"lista vacía", "https://app.empresa.com", nil, false},
		{"entrada vacía se ignora", "https://app.empresa.com", []string{""}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isAllowedOrigin(c.origin, c.allowed); got != c.want {
				t.Errorf("isAllowedOrigin(%q, %v) = %v, quiero %v", c.origin, c.allowed, got, c.want)
			}
		})
	}
}
