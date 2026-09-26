package desktop

// trayName is the name the tray shows, and deliberately the same string the
// launcher (Name= in the desktop entry) and the window title use: a tray that
// disagrees with the dock reads as a second application.
const trayName = "Govard"

// trayBrander is the part of *application.SystemTray that names the tray.
type trayBrander interface {
	SetLabel(label string)
	SetTooltip(tooltip string)
}

// brandTray names the tray on every platform. On Linux, Wails v3's SetTooltip
// is a no-op and the StatusNotifierItem Title defaults to "Wails"; SetLabel is
// what sets that Title.
func brandTray(t trayBrander) {
	t.SetLabel(trayName)
	t.SetTooltip(trayName)
}
