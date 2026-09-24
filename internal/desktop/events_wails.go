//go:build desktop

package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

// registerEvents declares every backend event with its payload type so the
// bindings generator emits typed declarations. Called once before
// application.New.
func registerEvents() {
	application.RegisterEvent[string](EventLogsLine)
	application.RegisterEvent[string](EventLogsStatus)
	application.RegisterEvent[string](EventLogsError)
	application.RegisterEvent[string](EventGlobalLogsLine)
	application.RegisterEvent[string](EventGlobalLogsStatus)
	application.RegisterEvent[string](EventGlobalLogsError)
	application.RegisterEvent[SyncStartedPayload](EventSyncStarted)
	application.RegisterEvent[string](EventSyncOutput)
	application.RegisterEvent[string](EventSyncFailed)
	application.RegisterEvent[string](EventSyncCompleted)
	application.RegisterEvent[OnboardingProgressPayload](EventOnboardingProgress)
	application.RegisterEvent[OperationNotification](EventOperationsNotification)
}
