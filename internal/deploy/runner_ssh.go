package deploy

import (
	"bytes"
	"context"
	"os/exec"

	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// SSHRunner runs commands on a remote host over SSH. It reuses the existing
// remote layer (key resolution, BatchMode, timeouts, host key policy) rather
// than growing a second SSH implementation.
type SSHRunner struct {
	RemoteName string
	Config     engine.RemoteConfig
}

// Args builds the ssh argv for one command. Exported so tests can assert the
// non-interactive contract without a server.
func (r SSHRunner) Args(command string) []string {
	args := remote.BuildSSHArgs(r.RemoteName, r.Config, false, false)
	args = append(args, remote.RemoteTarget(r.Config))
	return append(args, command)
}

// Run implements Runner over SSH.
func (r SSHRunner) Run(ctx context.Context, command string, opts RunOptions) (Result, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "ssh", r.Args(command)...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	// A killed process does not necessarily close the pipes its children
	// inherited, and Wait would then block until those children exit — a
	// 500ms timeout observed as 30s. WaitDelay bounds that wait; this is the
	// same failure the desktop doctor probe hit.
	cmd.WaitDelay = waitDelayAfterKill

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	if err == nil {
		return result, nil
	}
	return result, commandError(ctx, command, result.Stderr, err)
}
