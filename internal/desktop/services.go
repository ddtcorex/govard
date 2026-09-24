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
