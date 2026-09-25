package cmd

import (
	"io"
	"os/exec"
)

// DevProcessForTest is a started dev child process and its stdout.
type DevProcessForTest struct {
	Cmd    *exec.Cmd
	Stdout io.Reader
}

// StartDevProcessForTest starts a command the way runDesktopDev starts Vite.
func StartDevProcessForTest(name string, args ...string) (*DevProcessForTest, error) {
	c := exec.Command(name, args...)
	out, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	configureDevProcess(c)
	if err := c.Start(); err != nil {
		return nil, err
	}
	return &DevProcessForTest{Cmd: c, Stdout: out}, nil
}

// StopDevProcessForTest stops a command the way runDesktopDev stops Vite.
func StopDevProcessForTest(c *exec.Cmd) { stopDevProcess(c) }
