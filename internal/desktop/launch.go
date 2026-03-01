package desktop

import (
	"context"
	"io/fs"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const (
	DesktopBackgroundFlag       = "--background"
	DesktopBackgroundEnvVar     = "GOVARD_DESKTOP_BACKGROUND"
	desktopSingleInstanceLockID = "govard.desktop"
)

type LaunchOptions struct {
	Background bool
}

func ResolveLaunchOptions(args []string, envBackground string) LaunchOptions {
	options := LaunchOptions{}
	if hasBackgroundFlag(args) || parseTruthyBool(envBackground) {
		options.Background = true
	}
	return options
}

func BuildWailsOptions(app *App, assets fs.FS, launch LaunchOptions) *options.App {
	wailsOptions := &options.App{
		Title:       "Govard Desktop",
		Width:       1200,
		Height:      800,
		AssetServer: &assetserver.Options{Assets: assets},
		OnStartup:   app.Startup,
		OnShutdown:  app.Shutdown,
		Bind: []interface{}{
			app,
			app.Settings,
			app.Onboarding,
			app.Environment,
			app.Remote,
			app.System,
			app.Logs,
		},
	}

	if !launch.Background {
		return wailsOptions
	}

	wailsOptions.StartHidden = true
	wailsOptions.HideWindowOnClose = true
	wailsOptions.OnBeforeClose = func(ctx context.Context) bool {
		app.hideWindow(ctx)
		return true
	}
	wailsOptions.SingleInstanceLock = &options.SingleInstanceLock{
		UniqueId: desktopSingleInstanceLockID,
		OnSecondInstanceLaunch: func(options.SecondInstanceData) {
			app.showWindow()
		},
	}
	return wailsOptions
}

func hasBackgroundFlag(args []string) bool {
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), DesktopBackgroundFlag) {
			return true
		}
	}
	return false
}

func parseTruthyBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
