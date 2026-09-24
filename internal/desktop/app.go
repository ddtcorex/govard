package desktop

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"sync"

	"govard/internal/engine"
	"govard/internal/frameworks"
)

func (app *App) GetUserInfo() (res UserInfo, err error) {
	defer RecoverPanic(&err, "GetUserInfo")
	res = UserInfo{
		Username: "unknown",
		Name:     "Unknown User",
	}
	u, errCurrent := user.Current()
	if errCurrent != nil {
		return res, errCurrent
	}
	res.Username = u.Username
	res.Name = u.Name
	if res.Name == "" {
		res.Name = u.Username
	}
	return res, nil
}

var Version = "dev"

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
	return app
}

func (app *App) GetVersion() (v string, err error) {
	defer RecoverPanic(&err, "GetVersion")
	return Version, nil
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

func (app *App) Status() string {
	return "Govard Desktop ready."
}

func (app *App) OpenDocs(path string) (res string, err error) {
	defer RecoverPanic(&err, "OpenDocs")
	if path == "" {
		return "", fmt.Errorf("no docs path provided")
	}
	if errOpen := openDocs(app.platform, path); errOpen != nil {
		return "", fmt.Errorf("failed to open docs: %w", errOpen)
	}
	return "Opening docs...", nil
}

func (app *App) QuickAction(action string) (res string, err error) {
	defer RecoverPanic(&err, "QuickAction")
	return quickAction(app.platform, action, "")
}

func (app *App) QuickActionForProject(action string, project string) (res string, err error) {
	defer RecoverPanic(&err, "QuickActionForProject")
	return quickAction(app.platform, action, project)
}

func (app *App) DeleteProject(projectQuery string) (res string, err error) {
	defer RecoverPanic(&err, "DeleteProject")
	if projectQuery == "" {
		return "", fmt.Errorf("project name or path is required")
	}

	root, score, err := resolveProjectRootForRemotes(projectQuery)
	if err != nil {
		return "", err
	}

	// Safety check for weak matches in the desktop app
	if score >= engine.ScoreAmbiguousThreshold {
		return "", fmt.Errorf("match for %q is too weak for deletion (confidence score: %d): use the full name or path", projectQuery, score)
	}

	// Check if it's an orphan (root is the name, not an absolute path)
	if !filepath.IsAbs(root) && !strings.Contains(root, string(filepath.Separator)) {
		if err := engine.DeleteOrphanProject(app.ctx, root, os.Stdout, os.Stderr); err != nil {
			return "", err
		}
		return "Orphaned project resources removed", nil
	}

	// We use the application context for the deletion process
	if err := engine.DeleteProject(app.ctx, root, os.Stdout, os.Stderr); err != nil {
		return "", err
	}
	return "Project deleted successfully", nil
}

// ListFrameworks returns every framework registered in internal/frameworks,
// for the onboarding UI's framework picker. This keeps the frontend's
// dropdown, alias resolution, and display-name formatting in sync with the
// Go-side registry instead of duplicating framework metadata in JS.
func (app *App) ListFrameworks() (res []FrameworkOption, err error) {
	defer RecoverPanic(&err, "ListFrameworks")
	defs := frameworks.All()
	res = make([]FrameworkOption, 0, len(defs))
	for _, def := range defs {
		res = append(res, FrameworkOption{
			Name:        def.Name,
			DisplayName: def.DisplayName,
			Aliases:     append([]string(nil), def.Aliases...), // defensive copy - registry.go's All()/Get() warn callers not to mutate shared slice fields
		})
	}
	return res, nil
}
