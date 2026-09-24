package desktop

import "context"

// UpdateService exposes update checks, the update channel and self-restart
// to the frontend.
type UpdateService struct {
	platform Platform
	ctx      context.Context
}

func NewUpdateService() *UpdateService {
	return &UpdateService{platform: newDefaultPlatform()}
}

// Setup records the lifecycle context. RestartDesktopApp reads it as the
// "startup has run" signal before arming its delayed quit, so the service keeps
// the context for that guard alone.
func (s *UpdateService) Setup(ctx context.Context) {
	s.ctx = ctx
}
