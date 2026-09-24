//go:build desktop

package desktop

import (
	_ "embed"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed assets/tray.png
var trayIcon []byte

// installCloseHook applies decideClose to the main window's close button. The
// hook reads the setting on every close, so toggling "run in background" takes
// effect without a restart.
func installCloseHook(window *application.WebviewWindow, gate *closeGate, settings *SettingsService, trayAvailable bool) {
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		prefs, err := settings.GetSettings()
		runInBackground := err == nil && prefs.RunInBackground
		if gate.ShouldCancelClose(runInBackground, trayAvailable) {
			e.Cancel()
			window.Hide()
		}
	})
}

// installTray creates the tray icon and menu. It returns a refresh function
// that rebuilds the project list, or nil when no tray host exists. The refresh
// is serialised because the operation watcher and the menu items both call it.
func installTray(app *application.App, platform *wailsPlatform, desk *App) func() {
	tray := app.SystemTray.New()
	tray.SetIcon(trayIcon)
	tray.SetTooltip("Govard Desktop")
	menu := app.Menu.New()

	var refresh func()
	var refreshMu sync.Mutex
	refresh = func() {
		refreshMu.Lock()
		defer refreshMu.Unlock()

		menu.Clear()
		menu.Add("Show / Hide Govard").OnClick(func(*application.Context) {
			if _, w := platform.handles(); w != nil && w.IsVisible() {
				platform.HideWindow()
			} else {
				platform.ShowWindow()
			}
		})
		menu.AddSeparator()
		if dashboard, err := desk.Environment.GetDashboard(); err == nil {
			for _, p := range buildTrayProjects(dashboard) {
				p := p
				label := p.Project
				if p.Running {
					label = "● " + label // filled circle marks running projects
				}
				sub := menu.AddSubmenu(label)
				if p.Running {
					sub.Add("Stop").OnClick(func(*application.Context) {
						go func() {
							_, _ = desk.Environment.StopEnvironment(p.Project)
							refresh()
						}()
					})
					if p.URL != "" {
						sub.Add("Open in browser").OnClick(func(*application.Context) {
							_ = openURLWithPreferences(platform, p.URL)
						})
					}
				} else {
					sub.Add("Start").OnClick(func(*application.Context) {
						go func() {
							_, _ = desk.Environment.StartEnvironment(p.Project)
							refresh()
						}()
					})
				}
			}
			menu.AddSeparator()
		}
		menu.Add("Quit Govard").OnClick(func(*application.Context) {
			platform.Quit()
		})
		menu.Update()
	}
	refresh()
	tray.SetMenu(menu)
	return refresh
}
