package desktop

var Version = "dev"

// App is the Go-side composition root: it owns the services and the platform
// seam. It is deliberately no longer bound to the frontend, which reaches the
// services directly.
type App struct {
	platform Platform

	Settings    *SettingsService
	Onboarding  *OnboardingService
	Environment *EnvironmentService
	Remote      *RemoteService
	System      *SystemService
	Logs        *LogService
	Global      *GlobalServiceService
	Update      *UpdateService

	// watcher is bound as a service for its lifecycle only: it emits
	// operations:notification and has no frontend-callable methods.
	watcher *operationWatcher
}

// AppOption configures NewApp.
type AppOption func(*App)

// WithPlatform injects the GUI runtime seam (tests pass a FakePlatform).
func WithPlatform(p Platform) AppOption {
	return func(app *App) { app.platform = p }
}

func NewApp(opts ...AppOption) *App {
	app := &App{
		platform:    newDefaultPlatform(),
		Settings:    NewSettingsService(),
		Onboarding:  NewOnboardingService(),
		Environment: NewEnvironmentService(),
		Remote:      NewRemoteService(),
		System:      NewSystemService(),
		Logs:        NewLogService(),
		Global:      NewGlobalServiceService(),
		Update:      NewUpdateService(),
		watcher:     &operationWatcher{},
	}
	for _, opt := range opts {
		opt(app)
	}
	app.Settings.platform = app.platform
	app.Onboarding.platform = app.platform
	app.Environment.platform = app.platform
	app.Remote.platform = app.platform
	app.System.platform = app.platform
	app.Logs.platform = app.platform
	app.Global.platform = app.platform
	app.Update.platform = app.platform
	app.watcher.platform = app.platform
	return app
}
