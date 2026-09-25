//go:build windows

package cmd

import "os/exec"

func configureDevProcess(*exec.Cmd) {}

// terminateDevProcess kills the child without reaping it: the caller may own Wait.
func terminateDevProcess(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	_ = c.Process.Kill()
}

func stopDevProcess(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	terminateDevProcess(c)
	_ = c.Wait()
}
