package desktop

import (
	"context"
	"fmt"
	"os/user"
	"sync"
)

func (app *App) GetUserInfo() (UserInfo, error) {
	res := UserInfo{
		Username: "unknown",
		Name:     "Unknown User",
	}
	u, err := user.Current()
	if err != nil {
		return res, err
	}
	res.Username = u.Username
	res.Name = u.Name
	if res.Name == "" {
		res.Name = u.Username
	}
	return res, nil
}

var Version = "1.19.0"

type App struct {
	ctx context.Context

	Settings    *SettingsService
	Onboarding  *OnboardingService
	Environment *EnvironmentService
	Remote      *RemoteService
	System      *SystemService
	Logs        *LogService
	Global      *GlobalServiceService

	notifyMu     sync.Mutex
	notifyCancel context.CancelFunc
}

func NewApp() *App {
	return &App{
		Settings:    NewSettingsService(),
		Onboarding:  NewOnboardingService(),
		Environment: NewEnvironmentService(),
		Remote:      NewRemoteService(),
		System:      NewSystemService(),
		Logs:        NewLogService(),
		Global:      NewGlobalServiceService(),
	}
}

func (app *App) GetVersion() (string, error) {
	return Version, nil
}

func (app *App) Startup(ctx context.Context) {
	app.ctx = ctx
	app.Settings.Setup(ctx)
	app.Onboarding.Setup(ctx)
	app.Environment.Setup(ctx)
	app.Remote.Setup(ctx)
	app.System.Setup(ctx)
	app.Logs.Setup(ctx)
	app.Global.Setup(ctx)

	app.startOperationNotificationWatcher()
}

func (app *App) showWindow() {
	if app == nil || app.ctx == nil {
		return
	}
	showApplication(app.ctx)
}

func (app *App) hideWindow(ctx context.Context) {
	if app == nil {
		return
	}
	targetCtx := ctx
	if targetCtx == nil {
		targetCtx = app.ctx
	}
	if targetCtx == nil {
		return
	}
	hideApplication(targetCtx)
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

func (app *App) Status() string {
	return "Govard Desktop ready."
}

func (app *App) OpenDocs(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("no docs path provided")
	}
	if err := openDocs(app.ctx, path); err != nil {
		return "", fmt.Errorf("failed to open docs: %w", err)
	}
	return "Opening docs...", nil
}

func (app *App) QuickAction(action string) (string, error) {
	return quickAction(app.ctx, action, "")
}

func (app *App) QuickActionForProject(action string, project string) (string, error) {
	return quickAction(app.ctx, action, project)
}

func (app *App) GetShellUser(project string) (string, error) {
	return getShellUser(project)
}

func (app *App) SetShellUser(project string, user string) (string, error) {
	if err := setShellUser(project, user); err != nil {
		return "", err
	}
	if user == "" {
		return "Cleared shell user for " + project, nil
	}
	return "Saved shell user for " + project, nil
}

func (app *App) ResetShellUsers() (string, error) {
	if err := resetShellUsers(); err != nil {
		return "", err
	}
	return "Shell user preferences reset", nil
}
