package printer

const (
	StatusReady             = "ready"
	StatusBusy              = "busy"
	StatusPrinting          = "printing"
	StatusInitializing      = "initializing"
	StatusWarmingUp         = "warming_up"
	StatusWaiting           = "waiting"
	StatusProcessing        = "processing"
	StatusPowerSave         = "power_save"
	StatusIOActive          = "io_active"
	StatusTonerLow          = "toner_low"
	StatusOutputBinFull     = "output_bin_full"
	StatusOffline           = "offline"
	StatusError             = "error"
	StatusPaperJam          = "paper_jam"
	StatusPaperOut          = "paper_out"
	StatusPaperProblem      = "paper_problem"
	StatusNoToner           = "no_toner"
	StatusDoorOpen          = "door_open"
	StatusOutOfMemory       = "out_of_memory"
	StatusUserIntervention  = "user_intervention"
	StatusNotAvailable      = "not_available"
	StatusServerUnknown     = "server_unknown"
	StatusManualFeed        = "manual_feed"
	StatusPaused            = "paused"
	StatusPendingDeletion   = "pending_deletion"
	StatusPagePunt          = "page_punt"
)

type Severity string

const (
	SeverityOK      Severity = "ok"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

var blockingStatuses = map[string]struct{}{
	StatusOffline: {}, StatusError: {}, StatusPaperJam: {}, StatusPaperOut: {},
	StatusPaperProblem: {}, StatusNoToner: {}, StatusDoorOpen: {}, StatusOutOfMemory: {},
	StatusUserIntervention: {}, StatusNotAvailable: {}, StatusServerUnknown: {},
	StatusManualFeed: {}, StatusPaused: {}, StatusPendingDeletion: {},
}

var warningStatuses = map[string]struct{}{
	StatusTonerLow: {}, StatusOutputBinFull: {},
}

func IsBlocking(statuses []string) bool {
	for _, s := range statuses {
		if _, ok := blockingStatuses[s]; ok {
			return true
		}
	}
	return false
}

func ClassifySeverity(statuses []string) Severity {
	if IsBlocking(statuses) {
		return SeverityError
	}
	for _, s := range statuses {
		if _, ok := warningStatuses[s]; ok {
			return SeverityWarning
		}
	}
	return SeverityOK
}

func BlockingReason(statuses []string) string {
	for _, s := range statuses {
		if _, ok := blockingStatuses[s]; ok {
			return s
		}
	}
	return ""
}
