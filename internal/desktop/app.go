package desktop

import (
	"context"
	"sync"
)

var Version = "dev"

// App is the Go-side composition root: it owns the services and the platform
// seam. It is deliberately no longer bound to the frontend, which reaches the
// services directly.
type App struct {
	ctx      context.Context
	platform Platform

	Settings    *SettingsService
	Onboarding  *OnboardingService
	Environment *EnvironmentService
	Remote      *RemoteService
	System      *SystemService
	Logs        *LogService
	Global      *GlobalServiceService
	Update      *UpdateService

	notifyMu     sync.Mutex
	notifyCancel context.CancelFunc
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
	return app
}

func (app *App) Startup(ctx context.Context) {
	app.ctx = ctx
	if attacher, ok := app.platform.(contextAttacher); ok {
		attacher.attachContext(ctx)
	}
	app.Settings.Setup(ctx)
	app.Onboarding.Setup(ctx)
	app.Environment.Setup(ctx)
	app.Remote.Setup(ctx)
	app.System.Setup(ctx)
	app.Logs.Setup(ctx)
	app.Global.Setup(ctx)
	app.Update.Setup(ctx)

	app.startOperationNotificationWatcher()
}

func (app *App) showWindow() {
	if app == nil {
		return
	}
	app.platform.ShowWindow()
}

// hideWindow keeps its context parameter because Wails v2 passes the close
// event's context to App.BeforeClose; the platform does not need it.
func (app *App) hideWindow(_ context.Context) {
	if app == nil {
		return
	}
	app.platform.HideWindow()
}

func (app *App) BeforeClose(ctx context.Context) bool {
	settings, err := app.Settings.GetSettings()
	if err != nil {
		return false
	}
	if settings.RunInBackground {
		app.hideWindow(ctx)
		return true // prevent close
	}
	return false // allow close
}

func (app *App) Shutdown(ctx context.Context) {
	_ = ctx
	app.stopOperationNotificationWatcher()
}
