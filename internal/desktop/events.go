package desktop

// Backend event names. The payload JSON is part of the frontend contract, and
// each name is also registered with its payload type in events_wails.go so the
// bindings generator can emit typed declarations.
const (
	EventLogsLine               = "logs:line"
	EventLogsStatus             = "logs:status"
	EventLogsError              = "logs:error"
	EventGlobalLogsLine         = "global-logs:line"
	EventGlobalLogsStatus       = "global-logs:status"
	EventGlobalLogsError        = "global-logs:error"
	EventSyncStarted            = "sync:started"
	EventSyncOutput             = "sync:output"
	EventSyncFailed             = "sync:failed"
	EventSyncCompleted          = "sync:completed"
	EventOnboardingProgress     = "onboarding:progress"
	EventOperationsNotification = "operations:notification"
)

// SyncStartedPayload replaces the map[string]string sync:started payload with
// byte-identical JSON.
type SyncStartedPayload struct {
	Project string `json:"project"`
	Remote  string `json:"remote"`
	Preset  string `json:"preset"`
}

// OnboardingProgressPayload replaces the map[string]string
// onboarding:progress payload with byte-identical JSON.
type OnboardingProgressPayload struct {
	Step    string `json:"step"`
	Message string `json:"message"`
}
