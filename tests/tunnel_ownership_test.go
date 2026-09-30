package tests

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/tunnel"
)

// tunnelStartEvent runs `tunnel start` against a fake provider and returns the
// one tunnel.start operation event it wrote. The category in that event is what
// the operation log and the metrics read, so it is asserted there rather than on
// the classifier alone: deleting the call-site wrapping must fail this test.
func tunnelStartEvent(t *testing.T, provider func(engine.ProviderRef) (tunnel.Provider, error), args ...string) (engine.OperationEvent, error) {
	t.Helper()
	initTunnelHome(t)
	tunnelProjectForTest(t)
	t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))
	run := newTunnelRunner(t)
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{NewProvider: provider})
	t.Cleanup(restore)
	t.Cleanup(func() { cmd.ClearTunnelPIDForTest("demo") })

	_, runErr := run(append([]string{"tunnel", "start"}, args...)...)
	events, err := engine.ReadOperationEvents(10)
	if err != nil {
		t.Fatalf("read operation events: %v", err)
	}
	var found []engine.OperationEvent
	for _, event := range events {
		if event.Operation == "tunnel.start" {
			found = append(found, event)
		}
	}
	if len(found) != 1 {
		t.Fatalf("want exactly one tunnel.start event, got %d: %+v", len(found), events)
	}
	return found[0], runErr
}

func fakePlanProvider(plan tunnel.StartPlan) func(engine.ProviderRef) (tunnel.Provider, error) {
	return func(engine.ProviderRef) (tunnel.Provider, error) {
		return &fakeTunnelProvider{name: "fake", returnPlan: plan}, nil
	}
}

// #518: the messages of real runtime failures contain the word "provider"
// ("tunnel provider %s failed", "failed to start tunnel provider %s"), so a
// string match filed a cloudflared crash under validation. The category is now
// set where the error is produced.
func TestTunnelRuntimeFailureIsCategorisedRuntime(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider func(engine.ProviderRef) (tunnel.Provider, error)
		args     []string
		want     string
	}{
		{
			// The provider process runs and exits non-zero: a crash.
			name:     "the provider process exits non-zero",
			provider: fakePlanProvider(tunnel.StartPlan{Binary: "/bin/sh", Args: []string{"-c", "exit 7"}}),
			args:     []string{"https://demo.trycloudflare.com"},
			want:     "runtime",
		},
		{
			// The provider binary cannot be executed at all.
			name:     "the provider process cannot start",
			provider: fakePlanProvider(tunnel.StartPlan{Binary: filepath.Join(os.TempDir(), "govard-no-such-tunnel-binary")}),
			args:     []string{"https://demo.trycloudflare.com"},
			want:     "runtime",
		},
		{
			// An unknown provider name is operator input, not a runtime fault.
			name: "an unknown provider name",
			provider: func(engine.ProviderRef) (tunnel.Provider, error) {
				return nil, errors.New("unsupported tunnel provider \"nope\"")
			},
			args: []string{"https://demo.trycloudflare.com"},
			want: "validation",
		},
		{
			// A target the provider refuses to plan is operator input as well.
			name: "a target the provider cannot plan",
			provider: func(engine.ProviderRef) (tunnel.Provider, error) {
				return &fakeTunnelProvider{name: "fake", buildErr: errors.New("target URL \"ftp://x\" must use http or https")}, nil
			},
			args: []string{"https://demo.trycloudflare.com"},
			want: "validation",
		},
		{
			name:     "a positional url and --url together",
			provider: fakePlanProvider(tunnel.StartPlan{Binary: "/bin/true"}),
			args:     []string{"https://arg.test", "--url", "https://flag.test"},
			want:     "validation",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event, err := tunnelStartEvent(t, tc.provider, tc.args...)
			if err == nil {
				t.Fatal("tunnel start must fail in this case")
			}
			if event.Status != engine.OperationStatusFailure {
				t.Fatalf("event status = %q, want failure", event.Status)
			}
			if event.Category != tc.want {
				t.Fatalf("event category = %q, want %q (error: %v)", event.Category, tc.want, err)
			}
		})
	}
}

// #518: two starts for one project used to both write the record, the later
// write replacing the earlier, and the first to exit then removed the record of
// the one still running. The interleaving is reproduced deterministically: the
// StartProcess seam is the step between the pre-start check and the record
// write, and it lands the competing start's record exactly there.
func TestConcurrentTunnelStartsDoNotOverwriteEachOthersRecord(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	competitorPID, _ := startSleeper(t)
	const competitorArgv = "sleep 30"
	// The host reports the competitor's argv for every pid, which is what makes
	// the competitor's record provably live and owned.
	installTunnelPsShim(t, "echo '"+competitorArgv+"'")
	run := newTunnelRunner(t)

	var ours *exec.Cmd
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		// A provider that ignored the kill would hold Wait for five seconds.
		NewProvider: fakePlanProvider(tunnel.StartPlan{Binary: "/bin/sleep", Args: []string{"5"}}),
		StartProcess: func(process *exec.Cmd) error {
			if err := process.Start(); err != nil {
				return err
			}
			ours = process
			// The other start wins the race to the record here.
			if err := cmd.RecordTunnelPIDForTest("demo", competitorPID, competitorArgv); err != nil {
				t.Errorf("write competing record: %v", err)
			}
			return nil
		},
	})
	defer restore()
	t.Cleanup(func() {
		if ours != nil && ours.Process != nil {
			_ = ours.Process.Kill()
		}
	})

	out, err := run("tunnel", "start", "https://demo.trycloudflare.com")
	if err == nil {
		t.Fatalf("a start that lost the race to the record must refuse, got: %q", out)
	}
	if !strings.Contains(err.Error(), "tunnel stop") {
		t.Fatalf("the refusal must point at the command that clears the record: %v", err)
	}
	record, ok := cmd.TunnelPIDRecordForTest("demo")
	if !ok {
		t.Fatal("the losing start removed the winning start's record")
	}
	if record.PID != competitorPID {
		t.Fatalf("record names pid %d, want the winning start's pid %d", record.PID, competitorPID)
	}
	assertStillAlive(t, competitorPID)
	// The losing start must not leave its own provider running unrecorded:
	// `tunnel stop` could never reach it.
	if ours == nil || ours.ProcessState == nil {
		t.Fatal("the losing start must stop and reap the provider it started")
	}
}

// startTunnelUntilRecorded runs `tunnel start` with a /bin/sleep provider in the
// background and waits until the record names a pid other than notPID, which is
// this start's own process. The cleanup ends that process whatever happened.
func startTunnelUntilRecorded(t *testing.T, notPID int, startProcess func(*exec.Cmd) error) (cmd.TunnelPIDRecord, <-chan error) {
	t.Helper()
	run := newTunnelRunner(t)
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider:  fakePlanProvider(tunnel.StartPlan{Binary: "/bin/sleep", Args: []string{"30"}}),
		StartProcess: startProcess,
	})
	t.Cleanup(restore)
	t.Cleanup(func() {
		if record, ok := cmd.TunnelPIDRecordForTest("demo"); ok && record.PID > 0 && record.PID != notPID {
			_ = cmd.SignalProcessForTest(record.PID, os.Kill)
		}
		cmd.ClearTunnelPIDForTest("demo")
	})

	done := make(chan error, 1)
	go func() {
		_, err := run("tunnel", "start", "https://demo.trycloudflare.com")
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if record, ok := cmd.TunnelPIDRecordForTest("demo"); ok && record.PID != notPID {
			return record, done
		}
		select {
		case err := <-done:
			t.Fatalf("tunnel start ended before recording its own pid: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("tunnel start never recorded its own pid over the stale record")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// endRecordedTunnel stops the start's own process the way `tunnel stop` does and
// checks the session ends cleanly and takes its record with it.
func endRecordedTunnel(t *testing.T, record cmd.TunnelPIDRecord, done <-chan error) {
	t.Helper()
	if !strings.Contains(record.Argv, "sleep") {
		t.Fatalf("the record must name this start's provider, got %+v", record)
	}
	if err := cmd.SignalProcessForTest(record.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("terminate the recorded process: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("a SIGTERMed tunnel session is not a failure: %v", err)
	}
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); !os.IsNotExist(err) {
		t.Fatalf("the start must clear its own record when it ends, stat err = %v", err)
	}
}

// deadPIDForTest returns a pid whose process has already ended and been reaped.
func deadPIDForTest(t *testing.T) int {
	t.Helper()
	proc := exec.Command("sleep", "0.01")
	if err := proc.Start(); err != nil {
		t.Fatalf("start short sleeper: %v", err)
	}
	_ = proc.Wait()
	return proc.Process.Pid
}

// A record whose process is gone is stale: start must replace it with its own.
func TestTunnelStartReplacesARecordWhoseProcessIsGone(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	stale := deadPIDForTest(t)
	writeTunnelRecord(t, "demo", stale, "cloudflared tunnel --url http://localhost:80")

	record, done := startTunnelUntilRecorded(t, stale, nil)
	endRecordedTunnel(t, record, done)
}

// A live pid running something govard did not start (a recycled pid) is stale
// too: start must replace the record, and must leave that process alone.
func TestTunnelStartReplacesARecordWhosePIDRunsSomethingElse(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	// This test process is alive and its argv is not the recorded one.
	writeTunnelRecord(t, "demo", os.Getpid(), "cloudflared tunnel --url http://localhost:80")

	record, done := startTunnelUntilRecorded(t, os.Getpid(), nil)
	assertStillAlive(t, os.Getpid())
	endRecordedTunnel(t, record, done)
}

// The pre-check saw no record, and a stale one lands before the exclusive write:
// the claim's retry branch has to remove it and record this start's pid.
func TestTunnelStartClaimsOverAStaleRecordThatAppearsBeforeItsWrite(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	stale := deadPIDForTest(t)

	record, done := startTunnelUntilRecorded(t, stale, func(process *exec.Cmd) error {
		if err := process.Start(); err != nil {
			return err
		}
		if err := cmd.RecordTunnelPIDForTest("demo", stale, "cloudflared tunnel --url http://localhost:80"); err != nil {
			t.Errorf("write stale record: %v", err)
		}
		return nil
	})
	endRecordedTunnel(t, record, done)
}
