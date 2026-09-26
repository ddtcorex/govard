//go:build desktop

package desktop

import (
	"io/fs"
	"log"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Run is the single launch path for the desktop binary. It lives in its own
// desktop-tagged file because it imports the Wails runtime, which untagged
// builds must not link.
func Run(assets fs.FS) error {
	if err := CheckAssetRoot(assets); err != nil {
		log.Printf("warning: %v", err)
	}
	launch := ResolveLaunchOptions(os.Args[1:], os.Getenv(DesktopBackgroundEnvVar))
	platform := &wailsPlatform{}
	desk := NewApp(WithPlatform(platform))
	registerEvents()

	app := application.New(application.Options{
		Name: "Govard",
		// The Wayland surface app_id is the GtkApplication id, and GNOME matches
		// it against the installed "<id>.desktop": see DesktopApplicationID.
		Linux:    application.LinuxOptions{ApplicationID: DesktopApplicationID},
		Services: desk.boundServices(),
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: desktopSingleInstanceLockID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				platform.ShowWindow()
			},
		},
	})
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "Govard",
		Width:  1200,
		Height: 800,
		URL:    "/index.html",
		Hidden: launch.Background,
	})
	platform.attach(app, window)

	trayAvailable := TrayHostAvailable()
	installCloseHook(window, &platform.gate, desk.Settings, trayAvailable)
	if trayAvailable {
		// Set before app.Run(), which starts the watcher service and would
		// otherwise miss the first notification.
		desk.watcher.onEvent = installTray(app, platform, desk)
	}
	if launch.Background && !trayAvailable {
		// Nothing can bring a hidden window back, so a --background launch
		// without a tray host would be unreachable.
		log.Printf("warning: no system tray host found; starting with a visible window")
		window.Show()
	}
	return app.Run()
}

// boundServices is the complete frontend-callable surface: the eight domain
// services plus the watcher, which is bound for its lifecycle alone.
func (app *App) boundServices() []application.Service {
	return []application.Service{
		application.NewService(app.Settings),
		application.NewService(app.Onboarding),
		application.NewService(app.Environment),
		application.NewService(app.Remote),
		application.NewService(app.System),
		application.NewService(app.Logs),
		application.NewService(app.Global),
		application.NewService(app.Update),
		application.NewService(app.watcher),
	}
}
