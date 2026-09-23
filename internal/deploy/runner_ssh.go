package deploy

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"govard/internal/conventions"
	"govard/internal/engine"
	"govard/internal/engine/remote"
)

// remoteInterruptTimeout bounds the connection that tears a remote step down. It
// is opened after the run it is stopping has already been cancelled, so it must
// never be able to hold the CLI: a bounded connect and a bounded teardown mean
// Ctrl-C stays the operator's escape hatch, not a request that opens a second
// wait.
const remoteInterruptTimeout = 20 * time.Second

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

	// Killing the local ssh client does not stop the deploy: the work runs on the
	// other machine, and the connection carrying it is all that dies. sshd gives
	// the command a session and a process group of its own (measured on a real
	// target: pid == pgid == sid), so recording that pid is what lets the cancel
	// path signal the group over a second connection — the only way to reach work
	// whose connection is gone.
	pidFile := remoteInterruptPIDFile()
	report := remoteExitReport()
	started := recordRemoteProcessGroup(command, pidFile, report)

	cmd := exec.CommandContext(ctx, "ssh", r.Args(started)...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	if opts.Stdin != "" {
		// Over the SSH channel, which needs no server-side configuration —
		// unlike SendEnv, which requires AcceptEnv the project may not control.
		cmd.Stdin = strings.NewReader(opts.Stdin)
	}
	// The local client is killed with the rest of its group, and the remote group
	// is signalled by the second connection this installs.
	prepareProcessGroup(cmd, func() { r.terminateRemote(pidFile) })
	// A killed process does not necessarily close the pipes its children
	// inherited, and Wait would then block until those children exit — a
	// 500ms timeout observed as 30s. WaitDelay bounds that wait; this is the
	// same failure the desktop doctor probe hit.
	cmd.WaitDelay = waitDelayAfterKill

	stdout, stderr := newBoundedBuffer(captureLimit), newBoundedBuffer(captureLimit)
	cmd.Stdout = streamTo(stdout, opts.Out)
	cmd.Stderr = streamTo(stderr, opts.Out)

	err := cmd.Run()
	result := Result{Stdout: stdout.String(), Stderr: stderr.String()}
	// The report belongs to the wrapper, not to the step, and is removed before
	// anything reads the buffer so it cannot reach the operator's message.
	stripped, reported := stripExitReport(result.Stderr, report)
	result.Stderr = stripped
	if err == nil {
		return result, nil
	}
	// ssh's own exit code for a failed transport, and the one case where the
	// step's own status is unknown. A step is free to exit 255 itself — PHP does
	// it for every fatal error, so a compile or static-content step that runs out
	// of memory ends exactly that way — so the number alone cannot tell the two
	// apart. The wrapper prints its report only when the step itself
	// exited 255, so a report is the step saying it ended and a missing one is the
	// connection dying first. Nothing signalled the remote group when the link
	// died (with BatchMode and no pty, sshd sends no SIGHUP), so a 255 with no
	// report means the step may still be running, or may have died partway
	// through. Only a run that was not cancelled has this ambiguity: a cancelled
	// run already ran the teardown through the process-group hook.
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 255 && !reported && ctx.Err() == nil {
		r.terminateRemote(pidFile)
		return result, fmt.Errorf("%w: %v", ErrConnectionMayHaveDropped, commandError(ctx, command, result.Stderr, err))
	}
	// The operator is told about the step they wrote, not about the two
	// statements govard wrapped around it to make it interruptible.
	return result, commandError(ctx, command, result.Stderr, err)
}

// terminateRemote signals the process group of the step that was interrupted.
//
// SIGTERM is the whole teardown for anything that honours it, which is every
// tool a deploy runs. The SIGKILL after it exists for work that does not, and it
// is taken only while the group is demonstrably still there: a group id belongs
// to whoever owns it, and signalling one that had emptied and been handed to an
// unrelated process is worse than leaving an obstinate child running.
//
// The record is read with a short retry because a cancel can land in the window
// between the connection being established and the remote shell executing its
// first statement. Giving up there would leave a step running that nothing can
// reach any more — the one outcome this whole mechanism exists to prevent.
func (r SSHRunner) terminateRemote(pidFile string) {
	ctx, cancel := context.WithTimeout(context.Background(), remoteInterruptTimeout)
	defer cancel()

	script := fmt.Sprintf(`p=$(cat %s 2>/dev/null)
n=0
while [ -z "$p" ] && [ $n -lt 2 ]; do
  sleep 1
  p=$(cat %s 2>/dev/null)
  n=$((n+1))
done
if [ -n "$p" ]; then
  kill -TERM -$p 2>/dev/null
  n=0
  while [ $n -lt 2 ] && kill -0 -$p 2>/dev/null; do
    sleep 1
    n=$((n+1))
  done
  kill -0 -$p 2>/dev/null && kill -KILL -$p 2>/dev/null
fi
rm -f %s`, conventions.ShellQuote(pidFile), conventions.ShellQuote(pidFile), pidFile)

	cmd := exec.CommandContext(ctx, "ssh", r.Args(script)...)
	cmd.WaitDelay = waitDelayAfterKill
	_ = cmd.Run()
}

// recordRemoteProcessGroup prefixes a step with the two things the cancel path
// needs: the pid of the shell sshd started, which is also the session and process
// group leader, and a cleanup that runs when the step ends by itself.
//
// The step runs in a subshell rather than under an EXIT trap, because a trap is
// not the step's to keep: a command that ends in `exec` replaces the shell that
// would have run it, and a command that installs its own EXIT trap replaces
// govard's. Both are things a project's deploy hook can legitimately do, and both
// left the record on the target after a run that succeeded. A subshell survives
// either, so the file is removed by the step's own end rather than by a promise
// about how it ends. The exit status is carried out by hand for the same reason:
// the last statement is now the cleanup, not the step.
//
// The exit report is what makes the exit code honest: only the wrapper's own
// word distinguishes a step that exited 255 from a connection that died before
// the step could. It is printed only for that one status, so an ordinary step —
// and a successful one above all — carries nothing extra, and its absence on a
// 255 is the signal that the remote side never finished.
func recordRemoteProcessGroup(command, pidFile, report string) string {
	// The newline before the closing parenthesis is load-bearing: a step whose
	// last line is a comment would otherwise comment it out.
	return fmt.Sprintf(
		"printf '%%s\\n' $$ > %s; ( %s\n); rc=$?; rm -f %s; if [ \"$rc\" = 255 ]; then printf '%%s\\n' %s >&2; fi; exit $rc",
		conventions.ShellQuote(pidFile), command, pidFile, conventions.ShellQuote(report))
}

// remoteInterruptPIDFile names the per-run record. The path is built here from a
// pid and a nanosecond clock, so every character in it is one this package
// chose — which is why the trap can name it without quoting.
func remoteInterruptPIDFile() string {
	return fmt.Sprintf("/tmp/.govard-deploy-interrupt-%d-%d.pid", os.Getpid(), time.Now().UnixNano())
}

// remoteExitReport is the token the wrapper prints on stderr when the step itself
// exited 255. Like the pid file, every character comes from this package plus a
// pid and a nanosecond clock, so no step can print it by accident and be taken
// for the wrapper's word.
func remoteExitReport() string {
	return fmt.Sprintf("__govard-exit-255-%d-%d", os.Getpid(), time.Now().UnixNano())
}

// stripExitReport reports whether the wrapper said the step exited 255, and
// returns the captured stderr without its report. The report is the last thing
// the wrapper writes, so everything from it onwards is the wrapper's and not the
// step's.
func stripExitReport(stderr, report string) (string, bool) {
	index := strings.Index(stderr, report)
	if index < 0 {
		return stderr, false
	}
	return stderr[:index], true
}
