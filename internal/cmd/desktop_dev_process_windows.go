//go:build windows

package cmd

import "os/exec"

func configureDevProcess(*exec.Cmd) {}

func stopDevProcess(c *exec.Cmd) {
	if c.Process == nil {
		return
	}
	_ = c.Process.Kill()
	_ = c.Wait()
}
