//go:build windows

package ui

import "os"

func remove(p string) error { return os.Remove(p) }
