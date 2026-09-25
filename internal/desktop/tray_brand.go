package desktop

const trayName = "Govard Desktop"

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
