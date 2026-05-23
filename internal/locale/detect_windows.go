//go:build windows

package locale

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const localeNameMaxLength = 85

var (
	kernel32                     = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultLocaleName = kernel32.NewProc("GetUserDefaultLocaleName")
)

func detectSystem() string {
	var buf [localeNameMaxLength]uint16
	r, _, _ := syscall.SyscallN(
		procGetUserDefaultLocaleName.Addr(),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r == 0 {
		return ""
	}
	return windows.UTF16ToString(buf[:r])
}
