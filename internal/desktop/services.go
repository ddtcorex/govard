package desktop

import (
	"context"
	"sync"
)

// SettingsService handles user preferences and desktop settings.
type SettingsService struct {
	platform Platform
	ctx      context.Context
}

func NewSettingsService() *SettingsService {
	return &SettingsService{platform: newDefaultPlatform()}
}

func (s *SettingsService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// OnboardingService handles project discovery and onboarding.
type OnboardingService struct {
	platform Platform
	ctx      context.Context
}

func NewOnboardingService() *OnboardingService {
	return &OnboardingService{platform: newDefaultPlatform()}
}

func (s *OnboardingService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// EnvironmentService handles dashboard data and project lifecycle.
type EnvironmentService struct {
	platform Platform
	ctx      context.Context
}

func NewEnvironmentService() *EnvironmentService {
	return &EnvironmentService{platform: newDefaultPlatform()}
}

func (s *EnvironmentService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// RemoteService handles remote project management and synchronization.
type RemoteService struct {
	platform Platform
	ctx      context.Context
}

func NewRemoteService() *RemoteService {
	return &RemoteService{platform: newDefaultPlatform()}
}

func (s *RemoteService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// SystemService handles system metrics and user information.
type SystemService struct {
	platform Platform
	ctx      context.Context
}

func NewSystemService() *SystemService {
	return &SystemService{platform: newDefaultPlatform()}
}

func (s *SystemService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// LogService handles log streaming and terminal sessions.
type LogService struct {
	platform           Platform
	ctx                context.Context
	streamMu           sync.Mutex
	streamCancel       context.CancelFunc
	globalStreamMu     sync.Mutex
	globalStreamCancel context.CancelFunc
}

func NewLogService() *LogService {
	return &LogService{platform: newDefaultPlatform()}
}

func (s *LogService) Setup(ctx context.Context) {
	s.ctx = ctx
}

// GlobalServiceService handles global Govard service management.
type GlobalServiceService struct {
	platform Platform
	ctx      context.Context
}

func NewGlobalServiceService() *GlobalServiceService {
	return &GlobalServiceService{platform: newDefaultPlatform()}
}

func (s *GlobalServiceService) Setup(ctx context.Context) {
	s.ctx = ctx
}
