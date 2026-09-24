//go:build linux

package desktop

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

const statusNotifierWatcher = "org.kde.StatusNotifierWatcher"

// TrayHostAvailable reports whether a StatusNotifierItem host is on the
// session bus. Wails v3 exports no such query, and vanilla GNOME has none, so
// the close decision needs its own probe.
func TrayHostAvailable() bool {
	conn, err := dbus.SessionBusPrivate()
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	if err := conn.Auth(nil); err != nil {
		return false
	}
	if err := conn.Hello(); err != nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var hasOwner bool
	call := conn.BusObject().CallWithContext(ctx, "org.freedesktop.DBus.NameHasOwner", 0, statusNotifierWatcher)
	if call.Err != nil || call.Store(&hasOwner) != nil {
		return false
	}
	return hasOwner
}
