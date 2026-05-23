//go:build windows

package printer

import (
	"context"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
	printerAttrNetwork     = 0x00000010

	psPaused            = 0x00000001
	psError             = 0x00000002
	psPendingDeletion   = 0x00000004
	psPaperJam          = 0x00000008
	psPaperOut          = 0x00000010
	psManualFeed        = 0x00000020
	psPaperProblem      = 0x00000040
	psOffline           = 0x00000080
	psIOActive          = 0x00000100
	psBusy              = 0x00000200
	psPrinting          = 0x00000400
	psOutputBinFull     = 0x00000800
	psNotAvailable      = 0x00001000
	psWaiting           = 0x00002000
	psProcessing        = 0x00004000
	psInitializing      = 0x00008000
	psWarmingUp         = 0x00010000
	psTonerLow          = 0x00020000
	psNoToner           = 0x00040000
	psPagePunt          = 0x00080000
	psUserIntervention  = 0x00100000
	psOutOfMemory       = 0x00200000
	psDoorOpen          = 0x00400000
	psServerUnknown     = 0x00800000
	psPowerSave         = 0x01000000
)

type printerInfo2 struct {
	ServerName         *uint16
	PrinterName        *uint16
	ShareName          *uint16
	PortName           *uint16
	DriverName         *uint16
	Comment            *uint16
	Location           *uint16
	DevMode            uintptr
	SepFile            *uint16
	PrintProcessor     *uint16
	Datatype           *uint16
	Parameters         *uint16
	SecurityDescriptor uintptr
	Attributes         uint32
	Priority           uint32
	DefaultPriority    uint32
	StartTime          uint32
	UntilTime          uint32
	Status             uint32
	Jobs               uint32
	AveragePPM         uint32
}

var (
	winspool               = windows.NewLazySystemDLL("winspool.drv")
	procEnumPrintersW      = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinterW = winspool.NewProc("GetDefaultPrinterW")
)

func listPrinters(_ context.Context) ([]Printer, error) {
	flags := uint32(printerEnumLocal | printerEnumConnections)
	level := uint32(2)
	var needed, returned uint32
	r1, _, _ := procEnumPrintersW.Call(
		uintptr(flags), 0, uintptr(level), 0, 0,
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r1 == 0 && needed == 0 {
		return nil, fmt.Errorf("EnumPrinters: no se pudo dimensionar buffer")
	}
	buf := make([]byte, needed)
	r1, _, err := procEnumPrintersW.Call(
		uintptr(flags), 0, uintptr(level),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(needed),
		uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&returned)))
	if r1 == 0 {
		return nil, fmt.Errorf("EnumPrinters: %w", err)
	}
	defaultName, _ := getDefaultPrinter()
	infos := unsafe.Slice((*printerInfo2)(unsafe.Pointer(&buf[0])), returned)
	out := make([]Printer, 0, len(infos))
	for _, info := range infos {
		name := windows.UTF16PtrToString(info.PrinterName)
		statuses := decodeStatus(info.Status)
		out = append(out, Printer{
			Name:       name,
			IsDefault:  name == defaultName,
			IsNetwork:  info.Attributes&printerAttrNetwork != 0,
			Status:     pickWorst(statuses),
			Statuses:   statuses,
			Severity:   string(ClassifySeverity(statuses)),
			PortName:   windows.UTF16PtrToString(info.PortName),
			DriverName: windows.UTF16PtrToString(info.DriverName),
			Location:   windows.UTF16PtrToString(info.Location),
			Comment:    windows.UTF16PtrToString(info.Comment),
			JobCount:   info.Jobs,
		})
	}
	return out, nil
}

func getDefaultPrinter() (string, error) {
	var size uint32 = 256
	for {
		buf := make([]uint16, size)
		r1, _, _ := procGetDefaultPrinterW.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)))
		if r1 != 0 {
			return windows.UTF16ToString(buf), nil
		}
		if size > 4096 {
			return "", fmt.Errorf("GetDefaultPrinter: buffer demasiado grande")
		}
		size *= 2
	}
}

func decodeStatus(status uint32) []string {
	if status == 0 {
		return []string{StatusReady}
	}
	checks := []struct {
		flag uint32
		name string
	}{
		{psOffline, StatusOffline}, {psPaperJam, StatusPaperJam}, {psPaperOut, StatusPaperOut},
		{psPaperProblem, StatusPaperProblem}, {psNoToner, StatusNoToner}, {psDoorOpen, StatusDoorOpen},
		{psOutOfMemory, StatusOutOfMemory}, {psUserIntervention, StatusUserIntervention},
		{psNotAvailable, StatusNotAvailable}, {psServerUnknown, StatusServerUnknown},
		{psManualFeed, StatusManualFeed}, {psPaused, StatusPaused},
		{psPendingDeletion, StatusPendingDeletion}, {psError, StatusError},
		{psPagePunt, StatusPagePunt}, {psTonerLow, StatusTonerLow},
		{psOutputBinFull, StatusOutputBinFull},
		{psPrinting, StatusPrinting}, {psProcessing, StatusProcessing}, {psWaiting, StatusWaiting},
		{psWarmingUp, StatusWarmingUp}, {psInitializing, StatusInitializing},
		{psIOActive, StatusIOActive}, {psBusy, StatusBusy}, {psPowerSave, StatusPowerSave},
	}
	var out []string
	for _, c := range checks {
		if status&c.flag != 0 {
			out = append(out, c.name)
		}
	}
	if len(out) == 0 {
		return []string{StatusError}
	}
	return out
}

func pickWorst(statuses []string) string {
	if len(statuses) == 0 {
		return StatusReady
	}
	return statuses[0]
}
