package desktop

// UpdateService exposes update checks, the update channel and self-restart
// to the frontend.
type UpdateService struct {
	serviceBase
}

func NewUpdateService() *UpdateService {
	return &UpdateService{serviceBase: serviceBase{platform: newDefaultPlatform()}}
}
