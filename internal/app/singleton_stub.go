//go:build !windows

package app

func acquireSingletonLock() (bool, error)         { return true, nil }
func notifyAlreadyRunning(_, _ string)            {}
