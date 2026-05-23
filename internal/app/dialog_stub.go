//go:build !windows

package app

func confirmQuit(_ string) bool { return true }
