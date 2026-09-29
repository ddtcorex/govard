package tests

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pterm/pterm"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/engine/tunnel"
	"govard/internal/runtime"
)

type fakeTunnelProvider struct {
	name       string
	buildErr   error
	captured   []tunnel.StartOptions
	returnPlan tunnel.StartPlan
}

func (provider *fakeTunnelProvider) Name() string {
	if strings.TrimSpace(provider.name) == "" {
		return "fake"
	}
	return provider.name
}

func (provider *fakeTunnelProvider) BuildStartPlan(options tunnel.StartOptions) (tunnel.StartPlan, error) {
	provider.captured = append(provider.captured, options)
	if provider.buildErr != nil {
		return tunnel.StartPlan{}, provider.buildErr
	}
	return provider.returnPlan, nil
}

func TestTunnelCommandExists(t *testing.T) {
	root := cmd.RootCommandForTest()

	tunnelCommand, _, err := root.Find([]string{"tunnel"})
	if err != nil {
		t.Fatalf("find tunnel command: %v", err)
	}
	if tunnelCommand == nil || tunnelCommand.Use != "tunnel" {
		t.Fatalf("unexpected tunnel command: %#v", tunnelCommand)
	}

	startCommand, _, err := root.Find([]string{"tunnel", "start"})
	if err != nil {
		t.Fatalf("find tunnel start command: %v", err)
	}
	if startCommand == nil || startCommand.Use != "start [url]" {
		t.Fatalf("unexpected tunnel start command: %#v", startCommand)
	}

	// #469: stop owns one PID, so the help text must not describe a host-wide
	// kill any more.
	stopCommand, _, err := root.Find([]string{"tunnel", "stop"})
	if err != nil {
		t.Fatalf("find tunnel stop command: %v", err)
	}
	if stopCommand == nil || stopCommand.Use != "stop" {
		t.Fatalf("unexpected tunnel stop command: %#v", stopCommand)
	}
	stopLong := strings.ToLower(stopCommand.Long)
	for _, overreach := range []string{"every cloudflared", "not just the tunnel govard started", "all cloudflared"} {
		if strings.Contains(stopLong, overreach) {
			t.Fatalf("tunnel stop help still claims a host-wide kill (%q): %s", overreach, stopCommand.Long)
		}
	}
	if !strings.Contains(stopLong, "govard started") {
		t.Fatalf("tunnel stop help must say it only stops the tunnel govard started: %s", stopCommand.Long)
	}
}

func TestCloudflareTunnelProviderBuildStartPlan(t *testing.T) {
	provider, err := tunnel.NewProvider(engine.ProviderRef{Kind: engine.ProviderKindTunnel, Name: "cloudflare"})
	if err != nil {
		t.Fatalf("new cloudflare provider: %v", err)
	}

	plan, err := provider.BuildStartPlan(tunnel.StartOptions{
		TargetURL:   "https://demo.test",
		NoTLSVerify: true,
	})
	if err != nil {
		t.Fatalf("build start plan: %v", err)
	}
	if plan.Binary != "cloudflared" {
		t.Fatalf("expected cloudflared binary, got %s", plan.Binary)
	}
	joined := strings.Join(plan.Args, " ")
	if !strings.Contains(joined, "tunnel --url https://demo.test") {
		t.Fatalf("expected url args in plan, got: %s", joined)
	}
	if !strings.Contains(joined, "--no-tls-verify") {
		t.Fatalf("expected --no-tls-verify flag in plan, got: %s", joined)
	}
}

func TestTunnelStartPlanUsesConfigDomainByDefault(t *testing.T) {
	restoreCapabilities := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapCloudflared)
	defer restoreCapabilities()

	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: laravel
`), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeProvider := &fakeTunnelProvider{
		name: "fake-tunnel",
		returnPlan: tunnel.StartPlan{
			Binary: "fake-tunnel",
			Args:   []string{"start", "--target", "https://demo.test"},
		},
	}
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: func(ref engine.ProviderRef) (tunnel.Provider, error) {
			if ref.Kind != engine.ProviderKindTunnel {
				t.Fatalf("expected tunnel provider kind, got %s", ref.Kind)
			}
			if ref.Name != "cloudflare" {
				t.Fatalf("expected default provider name cloudflare, got %s", ref.Name)
			}
			return fakeProvider, nil
		},
		RunCommand: func(_ *exec.Cmd) error {
			t.Fatal("did not expect command execution in --plan mode")
			return nil
		},
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	buf := &strings.Builder{}
	root := cmd.RootCommandForTest()
	// --plan left set on the process-wide tree would turn a later `tunnel start`
	// into a dry run that never spawns, so its record would never appear.
	defer snapshotCommandTreeFlags(t, root)()
	root.SetOut(buf)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"tunnel", "start", "--plan"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute tunnel start --plan: %v", err)
	}
	if len(fakeProvider.captured) != 1 {
		t.Fatalf("expected one BuildStartPlan call, got %d", len(fakeProvider.captured))
	}
	options := fakeProvider.captured[0]
	if options.TargetURL != "https://demo.test" {
		t.Fatalf("expected default target URL from config domain, got %s", options.TargetURL)
	}
	if !options.NoTLSVerify {
		t.Fatal("expected no-tls-verify default true")
	}

	out := buf.String()
	if !strings.Contains(out, "Tunnel Plan") {
		t.Fatalf("expected tunnel plan header, got: %s", out)
	}
	if !strings.Contains(out, "fake-tunnel start --target https://demo.test") {
		t.Fatalf("expected planned command output, got: %s", out)
	}
}

func TestTunnelStartRejectsConflictingURLInputs(t *testing.T) {
	restoreCapabilities := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapCloudflared)
	defer restoreCapabilities()

	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: laravel
`), 0o644); err != nil {
		t.Fatal(err)
	}

	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: func(ref engine.ProviderRef) (tunnel.Provider, error) {
			_ = ref
			return &fakeTunnelProvider{
				name: "fake",
				returnPlan: tunnel.StartPlan{
					Binary: "fake",
				},
			}, nil
		},
		RunCommand: func(_ *exec.Cmd) error {
			return errors.New("unexpected command run")
		},
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	// --url parsed here would stay set on the process-wide tree and make a
	// later `tunnel start` report a bogus "not both" conflict.
	defer snapshotCommandTreeFlags(t, root)()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"tunnel", "start", "https://arg.test", "--url", "https://flag.test"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected conflicting URL input error")
	}
	if !strings.Contains(err.Error(), "either positional [url] or --url") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}

// #469: `tunnel stop`/`tunnel status` act on the PID govard recorded, never on a
// name pattern. Every test below runs the real command; the only input a shim
// supplies is the argv, which a child process cannot rewrite for itself.

func initTunnelHome(t *testing.T) {
	t.Helper()
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	// The tunnel group declares cloudflared and a test host has none. This commit
	// deliberately leaves the requirement alone, so satisfy the gate explicitly.
	restore := runtime.StubSatisfiedCapabilitiesForTest(runtime.CapCloudflared)
	t.Cleanup(restore)
}

func writeTunnelRecord(t *testing.T, project string, pid int, argv string) {
	t.Helper()
	if err := cmd.RecordTunnelPIDForTest(project, pid, argv); err != nil {
		t.Fatalf("record pid: %v", err)
	}
}

// The record is keyed by the config's project_name, and tunnel stop/status load
// the project config, so every stop/status case runs from a temp project.
func tunnelProjectForTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".govard.yml"),
		[]byte("project_name: demo\ndomain: demo.test\nframework: laravel\n"), 0o644); err != nil {
		t.Fatalf("write project: %v", err)
	}
	chdirForTest(t, dir)
}

// newTunnelRunner executes the real command. tunnel's messages go through pterm,
// whose writer is initialised to os.Stdout, so capturing needs both writers; the
// runner is built before any goroutine so the restore is registered in time.
func newTunnelRunner(t *testing.T) func(args ...string) (string, error) {
	t.Helper()
	out := &strings.Builder{}
	pterm.SetDefaultOutput(out)
	t.Cleanup(func() { pterm.SetDefaultOutput(os.Stdout) })
	root := cmd.RootCommandForTest()
	// The tree is process-wide and cobra parses onto the live command, so a
	// --plan or --url left set by this test would change what the next one
	// executes. Restored before any goroutine so the cleanup is registered in
	// time.
	t.Cleanup(snapshotCommandTreeFlags(t, root))
	root.SetOut(out)
	root.SetErr(io.Discard)
	return func(args ...string) (string, error) {
		root.SetArgs(args)
		err := root.Execute()
		return out.String(), err
	}
}

func runTunnel(t *testing.T, args ...string) (string, error) {
	t.Helper()
	return newTunnelRunner(t)(args...)
}

// assertStillAlive proves a process was left running, without signalling it.
func assertStillAlive(t *testing.T, pid int) {
	t.Helper()
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatalf("find pid %d: %v", pid, err)
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("pid %d is no longer alive: %v", pid, err)
	}
}

// startSleeper starts a real long-running child that cannot leak: the cleanup
// kills it whatever the test did, and the reaping goroutine stands in for the
// process.Wait() `tunnel start` runs itself, so the liveness probe sees a freed
// pid instead of a zombie. It is a direct `sleep`, never a shell wrapper, so it
// leaves no grandchild behind when it dies.
func startSleeper(t *testing.T) (int, <-chan error) {
	t.Helper()
	proc := exec.Command("sleep", "30")
	if err := proc.Start(); err != nil {
		t.Fatalf("start sleeper: %v", err)
	}
	t.Cleanup(func() {
		if proc.Process != nil {
			_ = proc.Process.Kill()
		}
	})
	waitErr := make(chan error, 1)
	go func() { waitErr <- proc.Wait() }()
	return proc.Process.Pid, waitErr
}

// installTunnelPatternShim logs every pkill/pgrep a command runs, so "nothing
// was matched by name" is observable rather than assumed.
func installTunnelPatternShim(t *testing.T) string {
	t.Helper()
	shimDir := t.TempDir()
	logPath := filepath.Join(shimDir, "pattern.log")
	script := "#!/bin/sh\nprintf '%s|%s\\n' \"$(basename \"$0\")\" \"$*\" >> \"${TUNNEL_PATTERN_LOG:-}\"\nexit 0\n"
	for _, name := range []string{"pkill", "pgrep"} {
		if err := os.WriteFile(filepath.Join(shimDir, name), []byte(script), 0o755); err != nil {
			t.Fatalf("write %s shim: %v", name, err)
		}
	}
	t.Setenv("TUNNEL_PATTERN_LOG", logPath)
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

// installTunnelPsShim replaces `ps`, the one input a child cannot rewrite: the
// observed argv. `body` also decides whether the host can be read at all.
func installTunnelPsShim(t *testing.T, body string) {
	t.Helper()
	shimDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(shimDir, "ps"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write ps shim: %v", err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestTunnelStopRefusesAForeignPID(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	// The recorded argv is not this process's argv, so govard does not own the PID.
	writeTunnelRecord(t, "demo", os.Getpid(), "cloudflared tunnel --url http://localhost:80")

	out, err := runTunnel(t, "tunnel", "stop")
	if err == nil {
		t.Fatalf("stop must refuse a PID whose argv it did not start, got: %q", out)
	}
	assertStillAlive(t, os.Getpid())
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); err != nil {
		t.Fatalf("a refused stop must keep the record for inspection: %v", err)
	}
}

// A recycled pid is not the only way a stop could reach the wrong process. A
// sibling `cloudflared tunnel` belonging to another tool shares the binary and
// the `tunnel` verb with the argv govard recorded — a systemd unit running
// `cloudflared tunnel --no-autoupdate --config /etc/cloudflared/other.yml
// tunnel run` is exactly that shape — so a comparison that stops at two tokens
// accepts it. The record here is govard's own and the host reports the sibling's;
// the stop must refuse rather than signal it.
func TestTunnelStopRefusesASiblingCloudflaredTunnel(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	pid, _ := startSleeper(t)
	installTunnelPsShim(t, "echo '/usr/bin/cloudflared tunnel --no-autoupdate --config /etc/cloudflared/other.yml tunnel run'")
	writeTunnelRecord(t, "demo", pid, "/usr/bin/cloudflared tunnel --url http://localhost:80")

	out, err := runTunnel(t, "tunnel", "stop")
	if err == nil {
		t.Fatalf("stop must refuse a sibling cloudflared tunnel, got: %q", out)
	}
	assertStillAlive(t, pid)
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); err != nil {
		t.Fatalf("a refused stop must keep the record for inspection: %v", err)
	}
}

// The argv comparison is the whole ownership argument, so the shapes it has to
// separate are pinned here rather than only through a command: the recorded
// argv must be reproduced token for token, or something the same binary and the
// same verb happens to share is mistaken for govard's tunnel.
func TestTunnelArgvMatchesSeparatesAnotherToolsCloudflared(t *testing.T) {
	const (
		govardQuickTunnel = "/usr/bin/cloudflared tunnel --url http://localhost:80"
		siblingUnit       = "/usr/bin/cloudflared tunnel --no-autoupdate --config /etc/cloudflared/other.yml tunnel run"
		otherVendorPath   = "/opt/vendor/bin/cloudflared tunnel --url http://localhost:80"
		unrelatedBinary   = "/usr/bin/sleep 30"
	)
	for _, tc := range []struct {
		name               string
		recorded, observed string
		want               bool
	}{
		{name: "the recorded argv itself", recorded: govardQuickTunnel, observed: govardQuickTunnel, want: true},
		// The recorded argv[0] is the path exec.LookPath resolved, while the
		// kernel reports the name exec.Command passed, so the two differ for
		// govard's own process on every run. Only the base name is comparable —
		// and the same rule is what accepts a process whose recorded name is
		// the bare one.
		{name: "recorded by an unresolved name", recorded: "cloudflared tunnel --url http://localhost:80", observed: govardQuickTunnel, want: true},
		{name: "a sibling cloudflared tunnel unit", recorded: govardQuickTunnel, observed: siblingUnit, want: false},
		// The cost of comparing the executable by base name, stated rather than
		// left implicit: a different vendor path running govard's own command
		// line byte for byte cannot be told apart from govard's tunnel. Refusing
		// it instead would refuse govard's own process on every real run, so
		// this shape is accepted.
		{name: "another vendor path, identical arguments", recorded: govardQuickTunnel, observed: otherVendorPath, want: true},
		{name: "an unrelated binary", recorded: govardQuickTunnel, observed: unrelatedBinary, want: false},
		{name: "a shorter argv than the record", recorded: govardQuickTunnel, observed: "/usr/bin/cloudflared tunnel", want: false},
		{name: "an unreadable argv", recorded: govardQuickTunnel, observed: "", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cmd.TunnelArgvMatchesForTest(tc.recorded, tc.observed); got != tc.want {
				t.Fatalf("tunnelArgvMatches(%q, %q) = %v, want %v", tc.recorded, tc.observed, got, tc.want)
			}
		})
	}
}

func TestTunnelStatusIsInactiveWithoutARecord(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)

	out, err := runTunnel(t, "tunnel", "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "INACTIVE") {
		t.Fatalf("no record must read as no tunnel, got: %q", out)
	}
}

func TestTunnelStopKillsNothingWhenThereIsNoRecord(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	logPath := installTunnelPatternShim(t)

	out, err := runTunnel(t, "tunnel", "stop")
	if err != nil {
		t.Fatalf("stop with no record must be a no-op: %v", err)
	}
	if logged, err := os.ReadFile(logPath); err == nil && strings.TrimSpace(string(logged)) != "" {
		t.Fatalf("stop reached a broad pattern match: %s", logged)
	}
	if !strings.Contains(out, "No tunnel started by govard") {
		t.Fatalf("stop must say there was nothing to stop, got: %q", out)
	}
}

func TestTunnelNeverReachesABroadPattern(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	logPath := installTunnelPatternShim(t)
	writeTunnelRecord(t, "demo", os.Getpid(), "cloudflared tunnel --url http://localhost:80")

	if out, err := runTunnel(t, "tunnel", "stop"); err == nil {
		t.Fatalf("stop must refuse a foreign PID, got: %q", out)
	}
	if out, err := runTunnel(t, "tunnel", "status"); err != nil {
		t.Fatalf("status: %v", err)
	} else if !strings.Contains(out, "INACTIVE") {
		t.Fatalf("a live PID govard did not start is not govard's tunnel, got: %q", out)
	}
	if logged, err := os.ReadFile(logPath); err == nil && strings.TrimSpace(string(logged)) != "" {
		t.Fatalf("a recorded PID must never fall back to a broad pattern: %s", logged)
	}
	assertStillAlive(t, os.Getpid())
}

func TestTunnelStatusDoesNotClaimAForeignPID(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	writeTunnelRecord(t, "demo", os.Getpid(), "cloudflared tunnel --url http://localhost:80")

	out, err := runTunnel(t, "tunnel", "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "INACTIVE") {
		t.Fatalf("a live PID govard did not start is not govard's tunnel, got: %q", out)
	}
	assertStillAlive(t, os.Getpid())
}

// An argv the host cannot report is not permission to fall back to a pattern:
// the command must refuse and leave the process running.
func TestTunnelStopRefusesWhenTheArgvCannotBeRead(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	pid, _ := startSleeper(t)
	installTunnelPsShim(t, "exit 1")
	writeTunnelRecord(t, "demo", pid, "sleep 30")

	out, err := runTunnel(t, "tunnel", "stop")
	if err == nil {
		t.Fatalf("stop must refuse when the argv cannot be verified, got: %q", out)
	}
	if !strings.Contains(err.Error(), "argv") {
		t.Fatalf("the refusal must name the unverifiable argv: %v", err)
	}
	assertStillAlive(t, pid)
}

func TestTunnelStopClearsAStaleRecordWithoutSignalling(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	// A pid whose process is genuinely gone: started, then waited for.
	proc := exec.Command("sleep", "0.01")
	if err := proc.Start(); err != nil {
		t.Fatalf("start short sleeper: %v", err)
	}
	_ = proc.Wait()
	writeTunnelRecord(t, "demo", proc.Process.Pid, "sleep 30")

	if _, err := runTunnel(t, "tunnel", "stop"); err != nil {
		t.Fatalf("stop on a stale record must be a no-op: %v", err)
	}
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); !os.IsNotExist(err) {
		t.Fatalf("a stale record must be removed, stat err = %v", err)
	}
}

// The project name is user input from .govard.yml, so the path component it
// produces must not be able to leave $GOVARD_HOME_DIR/tunnels.
func TestTunnelPIDRecordCannotEscapeTheTunnelsDirectory(t *testing.T) {
	initTunnelHome(t)
	tunnelsDir := filepath.Join(os.Getenv("GOVARD_HOME_DIR"), "tunnels")

	for _, hostile := range []string{"../../escape", "..", "/etc/passwd", "a/b/../..", "....//....//escape", "../.."} {
		path := cmd.TunnelPIDFileForTest(hostile)
		if filepath.Dir(path) != tunnelsDir {
			t.Fatalf("project name %q left the tunnels directory: %s", hostile, path)
		}
		if err := cmd.RecordTunnelPIDForTest(hostile, 4242, "cloudflared tunnel"); err != nil {
			t.Fatalf("record with project name %q: %v", hostile, err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("record for %q did not land inside the tunnels directory: %v", hostile, err)
		}
		cmd.ClearTunnelPIDForTest(hostile)
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("project name %q left %s behind: %v", hostile, path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("GOVARD_HOME_DIR"), "..", "escape.pid")); err == nil {
		t.Fatal("a hostile project name wrote outside the govard home directory")
	}
}

// tunnel start is what creates the record, so the lifecycle is asserted on the
// real command: the fake plan is `/bin/sleep 30`, which stands in for cloudflared
// so the test needs no cloudflared installed.
func TestTunnelStartRecordsAndClearsThePIDItStarted(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	run := newTunnelRunner(t)
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: func(ref engine.ProviderRef) (tunnel.Provider, error) {
			_ = ref
			return &fakeTunnelProvider{name: "fake", returnPlan: tunnel.StartPlan{
				Binary: "/bin/sleep", Args: []string{"30"},
			}}, nil
		},
		RunCommand: func(command *exec.Cmd) error { return command.Run() },
	})
	defer restore()

	// Whatever happens below, the sleeper this test causes to be started must not
	// survive it: read the recorded pid back at cleanup time and kill it.
	t.Cleanup(func() {
		if record, ok := cmd.TunnelPIDRecordForTest("demo"); ok && record.PID > 0 {
			_ = cmd.SignalProcessForTest(record.PID, os.Kill)
		}
		cmd.ClearTunnelPIDForTest("demo")
	})

	done := make(chan error, 1)
	go func() {
		_, err := run("tunnel", "start", "https://demo.trycloudflare.com")
		done <- err
	}()

	// The record appears while the process is still running, so stop can find it.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("tunnel start never recorded a PID")
		}
		time.Sleep(20 * time.Millisecond)
	}
	record, ok := cmd.TunnelPIDRecordForTest("demo")
	if !ok || record.PID <= 0 || !strings.Contains(record.Argv, "sleep") {
		t.Fatalf("record = %+v, ok = %v", record, ok)
	}
	// A `tunnel stop` ends the session with SIGTERM; that is a deliberate end
	// of the tunnel, not a provider failure.
	if err := cmd.SignalProcessForTest(record.PID, syscall.SIGTERM); err != nil {
		t.Fatalf("terminate the recorded process: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("a SIGTERMed tunnel session is not a failure: %v", err)
	}
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); !os.IsNotExist(err) {
		t.Fatalf("tunnel start must clear its record when the process ends, stat err = %v", err)
	}
}

func TestTunnelStartRefusesWhileItsOwnTunnelIsRunning(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	pid, _ := startSleeper(t)
	installTunnelPsShim(t, "echo 'sleep 30'")
	writeTunnelRecord(t, "demo", pid, "sleep 30")

	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		NewProvider: func(ref engine.ProviderRef) (tunnel.Provider, error) {
			_ = ref
			return &fakeTunnelProvider{name: "fake", returnPlan: tunnel.StartPlan{
				Binary: "/bin/sleep", Args: []string{"30"},
			}}, nil
		},
		RunCommand: func(command *exec.Cmd) error { return command.Run() },
	})
	defer restore()

	out, err := runTunnel(t, "tunnel", "start", "https://demo.trycloudflare.com")
	if err == nil {
		t.Fatalf("start must refuse while its own tunnel is still running, got: %q", out)
	}
	if !strings.Contains(err.Error(), "tunnel stop") {
		t.Fatalf("the refusal must point at the command that clears the record: %v", err)
	}
	assertStillAlive(t, pid)
}

func TestTunnelStopSignalsExactlyTheRecordedPID(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	// A real process is started so the signal, the liveness check and the record
	// removal are all exercised; only the argv the host reports is supplied,
	// because a child process cannot rewrite its own argv. The string below is
	// the sleeper's real argv, so the shim stands in for the host instead of
	// replacing what it would have said.
	pid, waitErr := startSleeper(t)
	installTunnelPsShim(t, "echo 'sleep 30'")
	writeTunnelRecord(t, "demo", pid, "sleep 30")

	if _, err := runTunnel(t, "tunnel", "stop"); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if err := <-waitErr; err == nil {
		t.Fatal("the recorded process must have been signalled")
	}
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); !os.IsNotExist(err) {
		t.Fatalf("stop must remove the record, stat err = %v", err)
	}
}

// A tunnel can end on its own between the liveness check and the SIGTERM — the
// check said alive, then the process was gone. That arrives as ESRCH and means
// the process this record names has already ended, which is the outcome a stop
// exists to reach. Reported as an error it takes the caller's "refused" branch,
// which deliberately skips the base-URL revert: the operator is left with a base
// URL pointing at a tunnel that is not running, and a stale record, for a
// command that did exactly the right thing.
func TestTunnelStopSucceedsWhenTheTunnelExitsBeforeTheSignal(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	pid, _ := startSleeper(t)
	installTunnelPsShim(t, "echo 'sleep 30'")
	writeTunnelRecord(t, "demo", pid, "sleep 30")

	// Only the signal is replaced. Liveness and the observed argv keep their real
	// implementations, so the race this covers — alive, matching, then gone — is
	// the one the host can actually produce.
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		SignalProcess: func(int, os.Signal) error { return os.ErrProcessDone },
	})
	defer restore()

	out, err := runTunnel(t, "tunnel", "stop")
	if err != nil {
		t.Fatalf("a tunnel that ended before the signal is a stopped tunnel, not a refusal: %v (out %q)", err, out)
	}
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); !os.IsNotExist(err) {
		t.Fatalf("stop must remove the record, stat err = %v", err)
	}
	if !strings.Contains(out, "Tunnel stopped") {
		t.Fatalf("stop must report the success it took, got: %q", out)
	}
}

// The same race, one branch down: the tunnel ignores SIGTERM long enough to be
// escalated, then ends before the SIGKILL lands. The escalation arm carries the
// identical ESRCH meaning, so it must not fail the command — and a reader
// finding ESRCH handled on one arm and not the other is exactly how the
// escalation path regresses.
func TestTunnelStopSucceedsWhenTheTunnelEndsBeforeTheEscalation(t *testing.T) {
	initTunnelHome(t)
	tunnelProjectForTest(t)
	pid, _ := startSleeper(t)
	installTunnelPsShim(t, "echo 'sleep 30'")
	writeTunnelRecord(t, "demo", pid, "sleep 30")

	signals := 0
	// The first signal is delivered and the process survives it, so the poll
	// loop runs out its grace and escalates; the escalation finds it gone.
	restore := cmd.SetTunnelDependenciesForTest(cmd.TunnelDependenciesForTest{
		SignalProcess: func(int, os.Signal) error {
			signals++
			if signals == 1 {
				return nil
			}
			return os.ErrProcessDone
		},
		Now: func() time.Time {
			// Jump past the grace on the first poll, so the escalation is
			// reached without the test sleeping through it.
			return time.Now().Add(time.Hour)
		},
		Sleep: func(time.Duration) {
			// Moving the clock is not the same as skipping the wait. Without
			// this, every poll still slept for real and the test paid the whole
			// five-second grace it had just told the loop to skip. The poll cap
			// still bounds the loop, so dropping the wait cannot hang it.
		},
	})
	defer restore()

	out, err := runTunnel(t, "tunnel", "stop")
	if err != nil {
		t.Fatalf("a tunnel that ended before the escalation is a stopped tunnel, not a refusal: %v (out %q)", err, out)
	}
	if signals < 2 {
		t.Fatalf("this test must reach the escalation signal, made %d", signals)
	}
	if _, err := os.Stat(cmd.TunnelPIDFileForTest("demo")); !os.IsNotExist(err) {
		t.Fatalf("stop must remove the record, stat err = %v", err)
	}
	if !strings.Contains(out, "Reverting base URL") {
		t.Fatalf("a refused branch skips the base-URL revert, which is the harm this guards: %q", out)
	}
}
