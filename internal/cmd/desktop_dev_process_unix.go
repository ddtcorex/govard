//go:build !windows

package cmd

import (
	"os/exec"
	"syscall"
)

// configureDevProcess puts the child in its own process group. `pnpm dev`
// runs `sh -c vite`, which runs node; killing pnpm alone orphans Vite on
// port 5173 and the next --dev run fails on strictPort.
func configureDevProcess(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateDevProcess signals the child's whole process group, without reaping
// it: the caller may own Wait.
func terminateDevProcess(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	if err := syscall.Kill(-c.Process.Pid, syscall.SIGTERM); err != nil {
		_ = c.Process.Kill()
	}
}

// stopDevProcess terminates the child's whole process group, then reaps it.
func stopDevProcess(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	terminateDevProcess(c)
	_ = c.Wait()
}
