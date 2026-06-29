package app

import (
	"log/slog"

	"github.com/lautarotiamat/prinklyprint/internal/config"
)

// denyPrompter rechaza todo pareo que requiera diálogo. Se inyecta en modo
// --headless (sin UI): ahí solo se puede parear con orígenes pre-aprobados en
// la config. Satisface server.PairingPrompter de forma implícita.
type denyPrompter struct{}

func (denyPrompter) Confirm(_, _ string) bool { return false }
func (denyPrompter) Interactive() bool        { return false }

// nativePairingPrompter muestra un diálogo nativo de Windows para que el
// operador apruebe el pareo de un origen web. En no-Windows el diálogo no
// existe (confirmPairing stub deniega). Satisface server.PairingPrompter.
type nativePairingPrompter struct {
	cfg    *config.Manager
	logger *slog.Logger
}

func (p *nativePairingPrompter) Confirm(origin, label string) bool {
	lang := p.cfg.Get().Language
	if p.logger != nil {
		p.logger.Info("solicitud de pareo, mostrando diálogo", "origin", origin, "label", label)
	}
	return confirmPairing(lang, origin, label)
}

func (p *nativePairingPrompter) Interactive() bool { return true }
