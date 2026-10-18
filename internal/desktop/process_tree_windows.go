//go:build windows

package desktop

import (
	"os/exec"
)

// setDesktopProcessGroup is a no-op on Windows: process groups are a Unix
// concept and exec.CommandContext already kills the direct child there.
func setDesktopProcessGroup(cmd *exec.Cmd) {
}

// killDesktopProcessTree falls back to killing the direct child on Windows.
func killDesktopProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
