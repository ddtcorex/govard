//go:build !linux

package desktop

// TrayHostAvailable is always true on macOS and Windows, which always have a
// menu bar / notification area.
func TrayHostAvailable() bool { return true }
