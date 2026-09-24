package desktop

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

const (
	DesktopBackgroundFlag       = "--background"
	DesktopBackgroundEnvVar     = "GOVARD_DESKTOP_BACKGROUND"
	desktopSingleInstanceLockID = "govard.desktop.app"
)

type LaunchOptions struct {
	Background bool
}

func ResolveLaunchOptions(args []string, envBackground string) LaunchOptions {
	options := LaunchOptions{}
	for _, arg := range args {
		trimmed := strings.ToLower(strings.TrimSpace(arg))
		if trimmed == "--version" || trimmed == "-v" {
			fmt.Printf("Govard Desktop v%s\n", Version)
			os.Exit(0)
		}
		if trimmed == DesktopBackgroundFlag {
			options.Background = true
		}
	}
	if parseTruthyBool(envBackground) {
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
			app.Settings,
			app.Onboarding,
			app.Environment,
			app.Remote,
			app.System,
			app.Logs,
			app.Global,
			app.Update,
		},
	}

	wailsOptions.SingleInstanceLock = &options.SingleInstanceLock{
		UniqueId: desktopSingleInstanceLockID,
		OnSecondInstanceLaunch: func(options.SecondInstanceData) {
			app.showWindow()
		},
	}

	if !launch.Background {
		return wailsOptions
	}

	wailsOptions.StartHidden = true
	wailsOptions.HideWindowOnClose = true
	wailsOptions.OnBeforeClose = func(ctx context.Context) bool {
		app.hideWindow(ctx)
		return true // prevent close; hide to tray instead
	}
	return wailsOptions
}

func parseTruthyBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
