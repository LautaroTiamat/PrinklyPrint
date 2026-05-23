//go:build windows

package app

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	mutexName              = `Global\PrinklyPrintSingletonMutex_v1`
	errorAlreadyExistsCode = 183
)

var (
	kernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procCreateMutex = kernel32.NewProc("CreateMutexW")
	user32          = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW = user32.NewProc("MessageBoxW")

	mutexHandle windows.Handle
)

func acquireSingletonLock() (bool, error) {
	name, err := syscall.UTF16PtrFromString(mutexName)
	if err != nil {
		return false, fmt.Errorf("UTF16PtrFromString: %w", err)
	}
	r0, _, e1 := syscall.SyscallN(procCreateMutex.Addr(),
		0, 0, uintptr(unsafe.Pointer(name)))
	if r0 == 0 {
		return false, fmt.Errorf("CreateMutex falló: %w", e1)
	}
	h := windows.Handle(r0)
	if e1 == errorAlreadyExistsCode {
		_ = windows.CloseHandle(h)
		return false, nil
	}
	mutexHandle = h
	return true, nil
}

func notifyAlreadyRunning(title, body string) {
	t, _ := syscall.UTF16PtrFromString(title)
	b, _ := syscall.UTF16PtrFromString(body)
	const (
		mbOK          = 0x00000000
		mbIconInfo    = 0x00000040
		mbSetForeFG   = 0x00010000
		mbSystemModal = 0x00001000
	)
	_, _, _ = procMessageBoxW.Call(
		0, uintptr(unsafe.Pointer(b)), uintptr(unsafe.Pointer(t)),
		uintptr(mbOK|mbIconInfo|mbSetForeFG|mbSystemModal),
	)
}
