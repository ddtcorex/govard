package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"govard/internal/engine"
)

const (
	// tunnelStopGrace is how long the recorded process gets to end on SIGTERM
	// before govard escalates to SIGKILL.
	tunnelStopGrace = 5 * time.Second
	// tunnelStopPollInterval is how often liveness is re-checked while waiting.
	tunnelStopPollInterval = 100 * time.Millisecond
	// tunnelStopMaxPolls bounds the wait even if Now is frozen.
	tunnelStopMaxPolls = 100
)

// TunnelPIDRecord is the process govard started for one project: the PID, and
// the argv it was started with, so a later stop can prove the PID is still that
// process and not a recycled one.
type TunnelPIDRecord struct {
	PID  int    `json:"pid"`
	Argv string `json:"argv"`
}

// tunnelPIDFilePath resolves where one project's record lives. The project name
// is user input from .govard.yml, so it goes through the one sanitizer the repo
// already owns (NormalizeProjectName keeps [a-zA-Z0-9_-] and nothing else)
// instead of being pasted into a path raw.
func tunnelPIDFilePath(projectName string) string {
	name := engine.NormalizeProjectName(projectName)
	if name == "" {
		// A name that normalizes to nothing would otherwise write a hidden
		// ".pid"; ComposeFilePath falls back the same way for the same input.
		name = "project"
	}
	return filepath.Join(engine.GovardHomeDir(), "tunnels", name+".pid")
}

// recordTunnelPID writes the PID and argv of the process govard just started.
func recordTunnelPID(projectName string, pid int, argv []string) error {
	path := tunnelPIDFilePath(projectName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create tunnel record directory: %w", err)
	}
	payload, err := json.Marshal(TunnelPIDRecord{PID: pid, Argv: strings.Join(argv, " ")})
	if err != nil {
		return fmt.Errorf("encode tunnel record: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o600); err != nil {
		return fmt.Errorf("write tunnel record: %w", err)
	}
	return nil
}

// clearTunnelPID drops the record. A record that is already gone is the state
// this function is trying to reach, so a failed remove is not reported.
func clearTunnelPID(projectName string) {
	_ = os.Remove(tunnelPIDFilePath(projectName))
}

// readTunnelPIDRecord returns the record, whether one exists, and an error only
// when a record exists but cannot be read: a record nobody can interpret is not
// the same as no record, and silently treating it as "nothing to stop" would
// strand a running tunnel.
func readTunnelPIDRecord(projectName string) (TunnelPIDRecord, bool, error) {
	path := tunnelPIDFilePath(projectName)
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return TunnelPIDRecord{}, false, nil
		}
		return TunnelPIDRecord{}, false, fmt.Errorf("read tunnel record %s: %w", path, err)
	}
	var record TunnelPIDRecord
	if err := json.Unmarshal(payload, &record); err != nil {
		return TunnelPIDRecord{}, false, fmt.Errorf(
			"tunnel record %s is unreadable (%w): remove it once you have checked the tunnel by hand", path, err)
	}
	if record.PID <= 0 {
		return TunnelPIDRecord{}, false, fmt.Errorf(
			"tunnel record %s names no process (pid %d): remove it once you have checked the tunnel by hand", path, record.PID)
	}
	return record, true, nil
}

// readProcessArgv returns the argv the host reports for a pid. It answers false
// whenever the answer is missing or empty, because "cannot tell" has to stay
// distinguishable from "is another process" at the call site.
func readProcessArgv(pid int) (string, bool) {
	if pid <= 0 {
		return "", false
	}
	output, err := exec.Command("ps", "-o", "args=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return "", false
	}
	argv := strings.TrimSpace(string(output))
	if argv == "" {
		return "", false
	}
	return argv, true
}

// tunnelArgvMatches compares the argv govard recorded against the one the host
// reports for a pid, token by token from the first argument on. The executable
// is compared by base name rather than by path because the two always differ
// for govard's own process: the record carries the path exec.LookPath resolved,
// while the kernel reports argv[0] exactly as exec.Command passed it.
//
// Everything past the executable has to match too. Two tokens are not enough to
// claim ownership: another `cloudflared tunnel` on this host — a systemd unit
// running `cloudflared tunnel --no-autoupdate --config /etc/cloudflared/other.yml
// tunnel run` — shares the binary and the verb, and a comparison that stopped
// there would let a recycled pid reach a tunnel belonging to somebody else. The
// observed argv therefore has to begin with everything govard recorded; an argv
// shorter than the record is refused, which is the direction that leaves a
// running tunnel alone rather than the one that kills a stranger's.
func tunnelArgvMatches(recorded, observed string) bool {
	recordedTokens, observedTokens := strings.Fields(recorded), strings.Fields(observed)
	if len(recordedTokens) == 0 || len(observedTokens) < len(recordedTokens) {
		return false
	}
	if filepath.Base(recordedTokens[0]) != filepath.Base(observedTokens[0]) {
		return false
	}
	for i := 1; i < len(recordedTokens); i++ {
		if recordedTokens[i] != observedTokens[i] {
			return false
		}
	}
	return true
}

// recordIsLiveAndOwned answers whether the recorded pid is still the process
// govard started. A pid that is gone — or alive with a different argv because
// the pid was recycled — is not govard's process. It never signals anything, so
// it is safe to call before deciding what to do with the record.
func recordIsLiveAndOwned(record TunnelPIDRecord, alive func(int) bool, readArgv func(int) (string, bool)) bool {
	if !alive(record.PID) {
		return false
	}
	observed, ok := readArgv(record.PID)
	if !ok {
		return false
	}
	return tunnelArgvMatches(record.Argv, observed)
}

// signalRecordedTunnel signals the process govard started for this project, and
// nothing else. It is the whole safety argument of `tunnel stop`: without a
// record nothing happens, and a record govard cannot prove it owns is refused
// rather than replaced by a name pattern.
func signalRecordedTunnel(
	projectName string,
	readArgv func(int) (string, bool),
	alive func(int) bool,
	now func() time.Time,
) error {
	record, found, err := readTunnelPIDRecord(projectName)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if !alive(record.PID) {
		// The tunnel is already gone; the record is the only stale thing left.
		clearTunnelPID(projectName)
		return nil
	}
	observed, ok := readArgv(record.PID)
	if !ok {
		return fmt.Errorf(
			"refusing to signal pid %d: cannot read its argv to confirm govard started it; stop it by hand or remove %s",
			record.PID, tunnelPIDFilePath(projectName))
	}
	if !tunnelArgvMatches(record.Argv, observed) {
		return fmt.Errorf(
			"refusing to signal pid %d: it is not the tunnel govard started for %q (it now runs %q)",
			record.PID, projectName, observed)
	}
	if err := tunnelDeps.SignalProcess(record.PID, tunnelSignalTerminate); err != nil {
		// The tunnel can end on its own between the liveness check above and
		// this signal. ESRCH says the process this record names is already gone,
		// which is the outcome a stop exists to reach — not a refusal, and not a
		// reason to fail. Handing the error back would take the caller's
		// "nothing was signalled" branch, which skips the base-URL revert
		// because the tunnel is still up, and it would leave the record behind:
		// a base URL pointing at a tunnel that is not running, for a command
		// that did the right thing.
		if errors.Is(err, os.ErrProcessDone) {
			clearTunnelPID(projectName)
			return nil
		}
		return fmt.Errorf("stopping tunnel pid %d: %w", record.PID, err)
	}
	// Give cloudflared the chance to shut its own connections down before the
	// signal is escalated; the poll cap keeps the loop finite even if a test
	// hands in a frozen clock.
	deadline := now().Add(tunnelStopGrace)
	for polls := 0; polls < tunnelStopMaxPolls && alive(record.PID); polls++ {
		if now().After(deadline) {
			break
		}
		tunnelDeps.Sleep(tunnelStopPollInterval)
	}
	if alive(record.PID) {
		// The same race as the SIGTERM arm exists between the poll's last
		// liveness check and this signal, and the reasoning is identical: a
		// process that is already gone is the outcome a stop exists to reach.
		// Handling ESRCH on one arm and not the other fails the command that
		// runs when a tunnel ignored SIGTERM long enough to be escalated.
		if err := tunnelDeps.SignalProcess(record.PID, tunnelSignalKill); err != nil &&
			!errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("stopping tunnel pid %d: %w", record.PID, err)
		}
	}
	clearTunnelPID(projectName)
	return nil
}

// processAlive reports whether the pid is still a process. Signal 0 asks the
// kernel without delivering anything; a process owned by another user answers
// EPERM rather than ESRCH, and that is still alive.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, os.ErrPermission) || errors.Is(err, syscall.EPERM)
}

// tunnelSignal names the signals tunnel's own process control uses, so the
// dependency seam can carry an os.Signal and the conversion to a real signal
// value happens in exactly one place.
type tunnelSignal int

const (
	tunnelSignalInterrupt tunnelSignal = iota
	tunnelSignalTerminate
	tunnelSignalKill
)

func (signal tunnelSignal) String() string {
	switch signal {
	case tunnelSignalInterrupt:
		return "interrupt"
	case tunnelSignalTerminate:
		return "terminated"
	case tunnelSignalKill:
		return "killed"
	default:
		return "signal"
	}
}

// Signal marks tunnelSignal as an os.Signal so it can travel through the
// dependency seam; the delivered value comes from systemSignal.
func (signal tunnelSignal) Signal() {}

func (signal tunnelSignal) systemSignal() os.Signal {
	switch signal {
	case tunnelSignalInterrupt:
		return os.Interrupt
	case tunnelSignalTerminate:
		return syscall.SIGTERM
	default:
		return os.Kill
	}
}

// signalProcess delivers a signal to a pid that may not be a child of this
// process, which is the whole point: `tunnel stop` runs in its own invocation
// and still has to reach the process `tunnel start` recorded.
func signalProcess(pid int, sig os.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if custom, ok := sig.(tunnelSignal); ok {
		return process.Signal(custom.systemSignal())
	}
	return process.Signal(sig)
}

// isTerminationExit reports whether the child ended because something sent it
// SIGTERM — which is how `govard tunnel stop` ends a session. It is a deliberate
// end, so the session is not reported as a provider failure. isInterruptExit
// (utils.go) is the SIGINT half and is reused unchanged.
func isTerminationExit(err error) bool {
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return false
	}
	status, ok := exitErr.Sys().(syscall.WaitStatus)
	if !ok {
		return false
	}
	return status.Signaled() && status.Signal() == syscall.SIGTERM
}

// TunnelPIDFileForTest exposes tunnelPIDFilePath to the tests/ package.
func TunnelPIDFileForTest(projectName string) string {
	return tunnelPIDFilePath(projectName)
}

// RecordTunnelPIDForTest exposes recordTunnelPID to the tests/ package.
func RecordTunnelPIDForTest(projectName string, pid int, argv string) error {
	return recordTunnelPID(projectName, pid, strings.Fields(argv))
}

// ClearTunnelPIDForTest exposes clearTunnelPID to the tests/ package.
func ClearTunnelPIDForTest(projectName string) {
	clearTunnelPID(projectName)
}

// TunnelPIDRecordForTest exposes readTunnelPIDRecord to the tests/ package.
func TunnelPIDRecordForTest(projectName string) (TunnelPIDRecord, bool) {
	record, found, err := readTunnelPIDRecord(projectName)
	if err != nil {
		return TunnelPIDRecord{}, false
	}
	return record, found
}

// SignalProcessForTest exposes signalProcess to the tests/ package.
func SignalProcessForTest(pid int, sig os.Signal) error {
	return signalProcess(pid, sig)
}

// TunnelArgvMatchesForTest exposes tunnelArgvMatches to the tests/ package.
func TunnelArgvMatchesForTest(recorded, observed string) bool {
	return tunnelArgvMatches(recorded, observed)
}
