//go:build !windows

package app

import "context"

func (a *App) runUI(ctx context.Context) error {
	a.logger.Info("UI desactivada en no-Windows; corriendo headless")
	<-ctx.Done()
	return ctx.Err()
}
