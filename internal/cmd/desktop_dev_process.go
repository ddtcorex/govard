package cmd

import (
	"context"
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

// StartDevAppForTest starts the desktop-app command the way runDesktopDev does:
// in its own process group, with the whole group signalled when ctx is
// cancelled. The caller owns Wait, exactly as runDesktopDev does.
func StartDevAppForTest(ctx context.Context, name string, args ...string) (*DevProcessForTest, error) {
	c := exec.Command(name, args...)
	out, err := c.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if _, err := startDevGrouped(ctx, c); err != nil {
		return nil, err
	}
	return &DevProcessForTest{Cmd: c, Stdout: out}, nil
}

// startDevGrouped starts c in its own process group and signals that whole group
// when ctx is cancelled, so a signal delivered to govard alone tears the child
// tree down instead of orphaning it (issue #423). The caller still owns Wait and
// must call the returned function once Wait has returned, to release the watcher.
func startDevGrouped(ctx context.Context, c *exec.Cmd) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	configureDevProcess(c)
	if err := c.Start(); err != nil {
		return nil, err
	}
	stopped := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			terminateDevProcess(c)
		case <-stopped:
		}
	}()
	return func() { close(stopped) }, nil
}
