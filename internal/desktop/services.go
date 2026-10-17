package desktop

import (
	"context"
	"sync"
)

// SettingsService handles user preferences and desktop settings.
type SettingsService struct {
	serviceBase
}

func NewSettingsService() *SettingsService {
	return &SettingsService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// OnboardingService handles project discovery and onboarding.
type OnboardingService struct {
	serviceBase
}

func NewOnboardingService() *OnboardingService {
	return &OnboardingService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// EnvironmentService handles dashboard data and project lifecycle.
type EnvironmentService struct {
	serviceBase
}

func NewEnvironmentService() *EnvironmentService {
	return &EnvironmentService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// RemoteService handles remote project management and synchronization.
type RemoteService struct {
	serviceBase
}

func NewRemoteService() *RemoteService {
	return &RemoteService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// SystemService handles system metrics and user information.
type SystemService struct {
	serviceBase
}

func NewSystemService() *SystemService {
	return &SystemService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// HealthService exposes the doctor + audit health UX: runs, per-check
// fixes (safe 1-click, risky preview-gated), integrity audit results, and
// the persisted banner index. See health_service.go.
type HealthService struct {
	serviceBase
}

func NewHealthService() *HealthService {
	return &HealthService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// ToolService exposes the preset tool runner: the allowlisted presets,
// validated runs with streamed output, cancellation, and scrubbed history.
// The palette NEVER sends argv: runs take a preset ID plus an args map and
// the backend owns the exec argv.
type ToolService struct {
	serviceBase
}

func NewToolService() *ToolService {
	return &ToolService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// LogService handles log streaming and terminal sessions.
type LogService struct {
	serviceBase

	streamMu           sync.Mutex
	streamCancel       context.CancelFunc
	globalStreamMu     sync.Mutex
	globalStreamCancel context.CancelFunc
}

func NewLogService() *LogService {
	return &LogService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}

// GlobalServiceService handles global Govard service management.
type GlobalServiceService struct {
	serviceBase
}

func NewGlobalServiceService() *GlobalServiceService {
	return &GlobalServiceService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}
