package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/engine"
	"govard/internal/try"
)

// closedSignal returns a signal channel that is already closed, so the run
// finishes the "storefront is up, wait for the user" step immediately.
func closedSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	close(ch)
	return ch
}

func hugeDisk(string) (uint64, error) { return 100 << 30, nil }

func noDelete(context.Context, string) error { return nil }

func TestTryRunsBootstrapInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()

	var gotDir string
	var gotArgs []string
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf: func(_ context.Context, dir string, args ...string) error {
			if len(args) > 0 && args[0] == "bootstrap" {
				gotDir, gotArgs = dir, args
			}
			return nil
		},
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	if err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}

	if gotDir != workspace {
		t.Fatalf("bootstrap ran in %q, want %q", gotDir, workspace)
	}
	want := []string{"bootstrap", "--fresh", "--framework", "mageos", "--yes", "--include-sample"}
	if strings.Join(gotArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("bootstrap args = %v, want %v", gotArgs, want)
	}
}

func TestTryRunsEnvUpAfterBootstrap(t *testing.T) {
	workspace := t.TempDir()

	var calls []string
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	if err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}

	if len(calls) != 2 || !strings.HasPrefix(calls[0], "bootstrap ") || calls[1] != "env up" {
		t.Fatalf("calls = %v, want bootstrap then env up", calls)
	}
}

func TestTrySkipsSampleDataWithFlag(t *testing.T) {
	workspace := t.TempDir()

	var gotArgs []string
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			if len(args) > 0 && args[0] == "bootstrap" {
				gotArgs = args
			}
			return nil
		},
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	if err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: false, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}

	for _, arg := range gotArgs {
		if arg == "--include-sample" {
			t.Fatalf("bootstrap args = %v, want no --include-sample", gotArgs)
		}
	}
}

func TestTryRemovesWorkspaceByDefaultAndKeepsWithFlag(t *testing.T) {
	for _, keep := range []bool{false, true} {
		workspace := t.TempDir()

		var deleted string
		restore := SetTryDependenciesForTest(TryDependenciesForTest{
			FreeDisk:      hugeDisk,
			RunSelf:       func(context.Context, string, ...string) error { return nil },
			Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
			DeleteProject: func(_ context.Context, dir string) error { deleted = dir; return nil },
			Signal:        closedSignal(),
			Stdin:         strings.NewReader(""),
		})

		err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Keep: keep, Timeout: time.Second})
		restore()
		if err != nil {
			t.Fatalf("keep=%v runTry: %v", keep, err)
		}

		if keep && deleted != "" {
			t.Fatalf("keep=true removed %q", deleted)
		}
		if !keep && deleted != workspace {
			t.Fatalf("keep=false removed %q, want %q", deleted, workspace)
		}
	}
}

func TestTryInterruptedDuringBootstrapPrintsCleanup(t *testing.T) {
	workspace := t.TempDir()

	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf: func(context.Context, string, ...string) error {
			return errors.New("interrupted")
		},
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second})
	if err == nil {
		t.Fatalf("runTry returned nil for a failed bootstrap")
	}
	if !strings.Contains(err.Error(), workspace) || !strings.Contains(err.Error(), "govard project delete") {
		t.Fatalf("error = %v, want the workspace path and the cleanup command", err)
	}
}

func TestTryRefusesWhenFreeDiskIsLow(t *testing.T) {
	workspace := t.TempDir()

	ran := false
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk:      func(string) (uint64, error) { return 1 << 30, nil },
		RunSelf:       func(context.Context, string, ...string) error { ran = true; return nil },
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second})
	if err == nil {
		t.Fatalf("runTry ran with 1 GiB free")
	}
	if !strings.Contains(err.Error(), "GiB") {
		t.Fatalf("error = %v, want it to state the free space", err)
	}
	if ran {
		t.Fatalf("bootstrap ran despite the disk preflight failing")
	}
}

func TestTryWorksFromAnotherProjectDirectory(t *testing.T) {
	caller := t.TempDir()
	callerConfig := filepath.Join(caller, ".govard.yml")
	if err := os.WriteFile(callerConfig, []byte("project_name: host-project\n"), 0o600); err != nil {
		t.Fatalf("seed caller config: %v", err)
	}
	before, err := os.ReadFile(callerConfig)
	if err != nil {
		t.Fatalf("read caller config: %v", err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(caller); err != nil {
		t.Fatalf("chdir caller: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	workspace := t.TempDir()
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk:      hugeDisk,
		RunSelf:       func(context.Context, string, ...string) error { return nil },
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	if err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}

	after, err := os.ReadFile(callerConfig)
	if err != nil {
		t.Fatalf("reread caller config: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("caller config changed: %q -> %q", before, after)
	}
}

func TestTryReportsUnreachableProxyWhenProbeFails(t *testing.T) {
	workspace := t.TempDir()

	var probed try.Origin
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf:  func(context.Context, string, ...string) error { return nil },
		Probe: func(_ context.Context, origin try.Origin, _ time.Duration) error {
			probed = origin
			return try.ErrNotReady
		},
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: 5 * time.Second})
	if err == nil {
		t.Fatalf("runTry returned nil when the probe failed")
	}
	if !strings.HasPrefix(probed.URL, "https://") {
		t.Fatalf("probe origin = %q, want the served https origin", probed.URL)
	}
	msg := err.Error()
	for _, want := range []string{"5s", "govard svc up"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error = %v, want it to contain %q", msg, want)
		}
	}
}

func TestTryWithoutSetupRunsNoPrivilegedStep(t *testing.T) {
	workspace := t.TempDir()

	var calls []string
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	if err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}

	for _, call := range calls {
		if strings.HasPrefix(call, "svc ") {
			t.Fatalf("calls = %v, want no privileged step without --setup", calls)
		}
	}
}

func TestTryWithSetupRunsServiceSetupBeforeBootstrap(t *testing.T) {
	workspace := t.TempDir()

	var calls []string
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Setup: true, Timeout: time.Second})
	if err != nil {
		t.Fatalf("runTry: %v", err)
	}

	if len(calls) < 2 || calls[0] != "svc up" || !strings.HasPrefix(calls[1], "bootstrap ") {
		t.Fatalf("calls = %v, want svc up then bootstrap", calls)
	}
}

func TestTryTeardownRunsWithLiveContextAfterInterrupt(t *testing.T) {
	workspace := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sawCanceled := true
	deleted := false
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk: hugeDisk,
		RunSelf:  func(context.Context, string, ...string) error { return nil },
		Probe: func(context.Context, try.Origin, time.Duration) error {
			cancel() // the user hits Ctrl-C while the trial is up
			return nil
		},
		DeleteProject: func(cleanupCtx context.Context, _ string) error {
			deleted = true
			sawCanceled = cleanupCtx.Err() != nil
			return nil
		},
		Signal: closedSignal(),
		Stdin:  strings.NewReader(""),
	})
	defer restore()

	if err := runTry(ctx, tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}
	if !deleted {
		t.Fatalf("teardown never ran")
	}
	if sawCanceled {
		t.Fatalf("teardown ran on a cancelled context; docker compose down would be a no-op")
	}
}

func TestTryRemovesTheWorkspaceDirectory(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "trial")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk:      hugeDisk,
		RunSelf:       func(context.Context, string, ...string) error { return nil },
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	if err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}); err != nil {
		t.Fatalf("runTry: %v", err)
	}
	if _, err := os.Stat(workspace); !os.IsNotExist(err) {
		t.Fatalf("workspace %s survived teardown", workspace)
	}
}

func TestTryRefusesWorkspaceThatIsARegisteredProject(t *testing.T) {
	workspace := t.TempDir()
	t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        workspace,
		ProjectName: "existing",
		Framework:   "magento2",
	}); err != nil {
		t.Fatalf("seed registry: %v", err)
	}

	ran := false
	restore := SetTryDependenciesForTest(TryDependenciesForTest{
		FreeDisk:      hugeDisk,
		RunSelf:       func(context.Context, string, ...string) error { ran = true; return nil },
		Probe:         func(context.Context, try.Origin, time.Duration) error { return nil },
		DeleteProject: noDelete,
		Signal:        closedSignal(),
		Stdin:         strings.NewReader(""),
	})
	defer restore()

	err := runTry(context.Background(), tryOptions{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second})
	if err == nil || !strings.Contains(err.Error(), "already a registered Govard project") {
		t.Fatalf("error = %v, want a refusal naming the registered project", err)
	}
	if ran {
		t.Fatalf("bootstrap ran on an already registered project")
	}
}
