//go:build windows

package deploy

import "os/exec"

// Windows has no process group to signal: a job object would be needed to take a
// command's children down with it, and govard does not create one. Cancellation
// therefore kills the shell govard started, and a child of that shell may
// outlive the run — the behaviour every platform had before the process group
// was introduced. Stated here rather than implied, because the difference is
// only observable after someone presses Ctrl-C.
//
// alsoCancel still runs: the remote half of a teardown is not a platform
// feature, and a Windows operator interrupting a deploy must be able to stop the
// work on the server too.
func prepareProcessGroup(cmd *exec.Cmd, alsoCancel func()) {
	cmd.Cancel = func() error {
		if alsoCancel != nil {
			go alsoCancel()
		}
		return nil
	}
}

// sweepProcessGroup is a no-op on Windows for the same reason: there is no group
// to sweep, and killing a pid whose process already exited could reach an
// unrelated process that reused it.
func sweepProcessGroup(*exec.Cmd) {}
