package desktop

import "sync/atomic"

// CloseAction is what closing the main window does.
type CloseAction int

const (
	CloseQuit CloseAction = iota
	CloseHide
)

// decideClose hides only when the user asked to keep running and a tray exists
// to bring the window back; otherwise hiding would trap the app with no way to
// reach it again.
func decideClose(runInBackground, trayAvailable bool) CloseAction {
	if runInBackground && trayAvailable {
		return CloseHide
	}
	return CloseQuit
}

// closeGate lets explicit quits (tray Quit, the app's Quit action, restart)
// through the WindowClosing hook, which app.Quit() also triggers: without it a
// cancelled close would make Quit a no-op and the app could never exit.
type closeGate struct {
	quitting atomic.Bool
}

func (g *closeGate) MarkQuitting() { g.quitting.Store(true) }

func (g *closeGate) ShouldCancelClose(runInBackground, trayAvailable bool) bool {
	if g.quitting.Load() {
		return false
	}
	return decideClose(runInBackground, trayAvailable) == CloseHide
}
