package desktop

import (
	"context"
	"sync"
)

// SettingsService handles user preferences and desktop settings.
type SettingsService struct {
	ctx context.Context
}

func NewSettingsService() *SettingsService {
	return &SettingsService{}
}

func (s *SettingsService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// OnboardingService handles project discovery and onboarding.
type OnboardingService struct {
	ctx context.Context
}

func NewOnboardingService() *OnboardingService {
	return &OnboardingService{}
}

func (s *OnboardingService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// EnvironmentService handles dashboard data and project lifecycle.
type EnvironmentService struct {
	ctx context.Context
}

func NewEnvironmentService() *EnvironmentService {
	return &EnvironmentService{}
}

func (s *EnvironmentService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// RemoteService handles remote project management and synchronization.
type RemoteService struct {
	ctx context.Context
}

func NewRemoteService() *RemoteService {
	return &RemoteService{}
}

func (s *RemoteService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// SystemService handles system metrics and user information.
type SystemService struct {
	ctx context.Context
}

func NewSystemService() *SystemService {
	return &SystemService{}
}

func (s *SystemService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// LogService handles log streaming and terminal sessions.
type LogService struct {
	ctx          context.Context
	streamMu     sync.Mutex
	streamCancel context.CancelFunc
}

func NewLogService() *LogService {
	return &LogService{}
}

func (s *LogService) Setup(ctx context.Context) {
	s.ctx = ctx
}
