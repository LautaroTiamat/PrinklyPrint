//go:build !windows

package tray

import (
	"context"
	"log/slog"

	"github.com/lautarotiamat/prinklyprint/internal/config"
	"github.com/lautarotiamat/prinklyprint/internal/queue"
	"github.com/lautarotiamat/prinklyprint/internal/store"
)

type Hooks struct {
	OnOpenQueue    func()
	OnOpenSettings func()
	OnOpenMain     func()
	OnQuit         func() bool
}

type Tray struct{}

func New(_ *store.Store, _ *queue.Worker, _ *config.Manager, _ *slog.Logger, _ Hooks) *Tray {
	return &Tray{}
}

func (t *Tray) Run(ctx context.Context) {
	<-ctx.Done()
}
