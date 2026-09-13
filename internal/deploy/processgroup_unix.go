//go:build unix

package deploy

import (
	"os/exec"
	"syscall"
)

// prepareProcessGroup makes a cancelled deploy step take its whole process group
// with it.
//
// A step is a chain — `cd {{release_path}} && composer install …` — so the
// process govard starts is a shell and the work is that shell's child. Go's
// default cancellation kills the shell alone, which leaves the work running:
// measured on a faithful stand-in for LocalRunner, a cancelled
// `touch marker && sleep 37 && touch finished` left `sleep 37` reparented and
// running for its full 37 seconds, and the rest of the chain never ran. Giving
// the command its own group means one signal reaches everything it started, and
// it also means the signal is aimed at a group govard created rather than at a
// group id it guessed from a pid.
//
// alsoCancel, when non-nil, is the rest of the same teardown: work that is not on
// this machine cannot be reached by a signal from here, so it runs in its own
// goroutine while this one returns promptly.
func prepareProcessGroup(cmd *exec.Cmd, alsoCancel func()) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if alsoCancel != nil {
			go alsoCancel()
		}
		// SIGTERM first: a compiler, a package manager or rsync gets the chance
		// to clean up after itself. waitDelayAfterKill bounds how long Wait then
		// waits, and sweepProcessGroup is what handles work that ignored it.
		signalProcessGroup(cmd, syscall.SIGTERM)
		return nil
	}
}

// sweepProcessGroup finishes what prepareProcessGroup started: after a command
// that was stopped rather than finished has returned, anything still in its
// group is killed outright.
//
// The escalation happens here, immediately, rather than on a timer. A timer
// would fire after the group's leader had already been reaped, and a process
// group id is only meaningful while something owns it — a group that empties and
// whose id is then handed to an unrelated process must not be signalled. On this
// path the leader was reaped microseconds ago.
func sweepProcessGroup(cmd *exec.Cmd) {
	signalProcessGroup(cmd, syscall.SIGKILL)
}

// signalProcessGroup signals every process in the command's group. The negative
// pid is the group id, which is the leader's pid because prepareProcessGroup
// made the command a group leader.
func signalProcessGroup(cmd *exec.Cmd, signal syscall.Signal) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, signal)
}
