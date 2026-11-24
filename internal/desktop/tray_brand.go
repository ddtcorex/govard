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

// brandTray names the tray on every platform. On Linux the two calls write two
// different StatusNotifierItem properties, both measured on Wails v3.1.0:
// SetLabel sets Title (without it the Title reads "Wails"), and SetTooltip sets
// the descriptive line a host shows under that title. Dropping SetLabel loses
// the name; dropping SetTooltip only loses the line.
func brandTray(t trayBrander) {
	t.SetLabel(trayName)
	t.SetTooltip(trayName)
}
