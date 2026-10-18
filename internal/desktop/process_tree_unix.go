//go:build !windows

package desktop

import (
	"os/exec"
	"syscall"
)

// setDesktopProcessGroup puts the child in its own process group so a
// timeout can kill the whole tree: killing only the direct child leaves
// grandchildren (ssh, sleep) holding the output pipes open and
// CombinedOutput blocks until they exit on their own.
func setDesktopProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killDesktopProcessTree SIGKILLs the child's process group. Callers use it
// alongside exec.CommandContext (which only kills the direct child) when a
// ceiling fires while CombinedOutput is still blocked.
func killDesktopProcessTree(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
