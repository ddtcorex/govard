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
	EventSyncProgress           = "sync:progress"
	EventSyncFailed             = "sync:failed"
	EventSyncCompleted          = "sync:completed"
	EventOnboardingProgress     = "onboarding:progress"
	EventOperationsNotification = "operations:notification"
	EventToolOutput             = "tool:output"
	EventToolCompleted          = "tool:completed"
	EventToolFailed             = "tool:failed"
	EventDBProgress             = "db:progress"
	EventDBCompleted            = "db:completed"
	EventDBFailed               = "db:failed"
	EventSnapshotCompleted      = "snapshot:completed"
	EventSnapshotFailed         = "snapshot:failed"
)

// ToolResultPayload is the tool:completed / tool:failed event payload: which
// op finished and how it ended. Gen is the admitted-run generation for the
// project lock: OpIDs are stable per project across reruns, so consumers
// apply a terminal event only when its Gen is still current (see
// IsCurrentToolGeneration); stale dead-run receipts are dropped, never
// allowed to mark the current run failed.
type ToolResultPayload struct {
	OpID    string `json:"opId"`
	Project string `json:"project"`
	Preset  string `json:"preset"`
	Message string `json:"message"`
	Gen     uint64 `json:"gen"`
}

// ToolOutputPayload tags one streamed tool:output batch with its op so the
// palette can attribute lines when runs overlap, and the publish stage can
// drop batches from an op that is no longer current for the project.
type ToolOutputPayload struct {
	OpID string `json:"opId"`
	Text string `json:"text"`
}

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
