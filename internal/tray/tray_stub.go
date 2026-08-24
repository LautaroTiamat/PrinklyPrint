//go:build !windows

package tray

// El tray real (walk.NotifyIcon) es Windows-only y lo crea la ventana (ver
// internal/ui). Este stub solo mantiene el paquete compilable en Linux/CI;
// Hooks y Color viven en tray_common.go.
