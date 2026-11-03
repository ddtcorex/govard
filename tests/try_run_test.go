package tests

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

// closedSignal returns a signal channel that is already closed, so a trial
// finishes the "storefront is up, wait for the user" step immediately.
func closedSignal() <-chan os.Signal {
	ch := make(chan os.Signal, 1)
	close(ch)
	return ch
}

// trialDeps fills every dependency a unit test must never perform for real (a
// proxy dial, a docker listing) with an inert stand-in, so a test overrides only
// the one it is about.
func trialDeps(d try.Dependencies) try.Dependencies {
	if d.RunSelf == nil {
		d.RunSelf = func(context.Context, string, ...string) error { return nil }
	}
	if d.Probe == nil {
		d.Probe = func(context.Context, try.Origin, time.Duration) error { return nil }
	}
	if d.DeleteProject == nil {
		d.DeleteProject = func(context.Context, string) error { return nil }
	}
	if d.CollectArtifacts == nil {
		d.CollectArtifacts = func(string, string, []string) engine.ProjectArtifacts {
			return engine.ProjectArtifacts{}
		}
	}
	if d.ProxyUp == nil {
		d.ProxyUp = func() error { return nil }
	}
	if d.FreeDisk == nil {
		d.FreeDisk = func(string) (uint64, error) { return 100 << 30, nil }
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Signal == nil {
		d.Signal = closedSignal()
	}
	if d.Stdin == nil {
		d.Stdin = strings.NewReader("")
	}

	return d
}

func runTrial(ctx context.Context, o try.Options, d try.Dependencies) error {
	return try.Run(ctx, o, trialDeps(d))
}

func TestNearestExistingDirWalksUpToAParent(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "a", "b", "c")

	got, err := try.NearestExistingDirForTest(missing)
	if err != nil {
		t.Fatalf("NearestExistingDirForTest: %v", err)
	}
	if got != root {
		t.Fatalf("nearest existing dir = %q, want %q", got, root)
	}
}

func TestTryRefusesWhenTheProxyIsNotRunning(t *testing.T) {
	workspace := t.TempDir()

	ran := false
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Timeout: time.Second}, try.Dependencies{
		ProxyUp: func() error { return errors.New("connection refused") },
		RunSelf: func(context.Context, string, ...string) error { ran = true; return nil },
	})
	if err == nil {
		t.Fatalf("trial ran without a proxy")
	}
	if !strings.Contains(err.Error(), "--setup") {
		t.Fatalf("error = %v, want the --setup remedy", err)
	}
	if ran {
		t.Fatalf("bootstrap ran despite the missing proxy")
	}
}

func TestTryRunsBootstrapInsideWorkspace(t *testing.T) {
	workspace := t.TempDir()

	var gotDir string
	var gotArgs []string
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(_ context.Context, dir string, args ...string) error {
			if len(args) > 0 && args[0] == "bootstrap" {
				gotDir, gotArgs = dir, args
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
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
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(calls) != 2 || !strings.HasPrefix(calls[0], "bootstrap ") || calls[1] != "env up" {
		t.Fatalf("calls = %v, want bootstrap then env up", calls)
	}
}

func TestTrySkipsSampleDataWhenSampleIsFalse(t *testing.T) {
	workspace := t.TempDir()

	var gotArgs []string
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: false, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			if len(args) > 0 && args[0] == "bootstrap" {
				gotArgs = args
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, arg := range gotArgs {
		if arg == "--include-sample" {
			t.Fatalf("bootstrap args = %v, want no --include-sample", gotArgs)
		}
	}
}

func TestTryKeepsAnExplicitDirAndRemovesAGeneratedWorkspace(t *testing.T) {
	explicit := t.TempDir()
	if err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: explicit, Sample: true, Timeout: time.Second}, try.Dependencies{}); err != nil {
		t.Fatalf("trial with --dir: %v", err)
	}
	if _, err := os.Stat(explicit); err != nil {
		t.Fatalf("--dir workspace %s was removed: %v", explicit, err)
	}

	var generated string
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Sample: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(_ context.Context, dir string, _ ...string) error {
			generated = dir
			return nil
		},
	})
	if err != nil {
		t.Fatalf("trial without --dir: %v", err)
	}
	if generated == "" {
		t.Fatalf("bootstrap never ran for the generated workspace")
	}
	if _, err := os.Stat(generated); !os.IsNotExist(err) {
		t.Fatalf("generated workspace %s survived teardown", generated)
	}
}

func TestTryRemovesWorkspaceByDefaultAndKeepsWithFlag(t *testing.T) {
	for _, keep := range []bool{false, true} {
		workspace := t.TempDir()

		var deleted string
		err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Keep: keep, Timeout: time.Second}, try.Dependencies{
			DeleteProject: func(_ context.Context, dir string) error { deleted = dir; return nil },
		})
		if err != nil {
			t.Fatalf("keep=%v Run: %v", keep, err)
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

	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(context.Context, string, ...string) error { return errors.New("interrupted") },
	})
	if err == nil {
		t.Fatalf("Run returned nil for a failed bootstrap")
	}
	if !strings.Contains(err.Error(), workspace) || !strings.Contains(err.Error(), "govard project delete") {
		t.Fatalf("error = %v, want the workspace path and the cleanup command", err)
	}
}

func TestTryInterruptedDuringTheWaitStillTearsDown(t *testing.T) {
	workspace := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deleted := false
	err := runTrial(ctx, try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		Probe: func(context.Context, try.Origin, time.Duration) error {
			cancel()
			return context.Canceled
		},
		DeleteProject: func(context.Context, string) error { deleted = true; return nil },
		Signal:        make(chan os.Signal),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !deleted {
		t.Fatalf("teardown did not run after an interrupt during the wait")
	}
}

func TestTryRefusesWhenFreeDiskIsLow(t *testing.T) {
	workspace := t.TempDir()

	ran := false
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		FreeDisk: func(string) (uint64, error) { return 1 << 30, nil },
		RunSelf:  func(context.Context, string, ...string) error { ran = true; return nil },
	})
	if err == nil {
		t.Fatalf("trial ran with 1 GiB free")
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
	if err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{}); err != nil {
		t.Fatalf("Run: %v", err)
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
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: 5 * time.Second}, try.Dependencies{
		Probe: func(_ context.Context, origin try.Origin, _ time.Duration) error {
			probed = origin
			return try.ErrNotReady
		},
	})
	if err == nil {
		t.Fatalf("Run returned nil when the probe failed")
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

func TestTryTeardownRunsWithLiveContextAfterInterrupt(t *testing.T) {
	workspace := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sawCanceled := true
	deleted := false
	err := runTrial(ctx, try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		Probe: func(context.Context, try.Origin, time.Duration) error {
			cancel()
			return nil
		},
		DeleteProject: func(cleanupCtx context.Context, _ string) error {
			deleted = true
			sawCanceled = cleanupCtx.Err() != nil
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !deleted {
		t.Fatalf("teardown never ran")
	}
	if sawCanceled {
		t.Fatalf("teardown ran on a cancelled context; docker compose down would be a no-op")
	}
}

func TestTryTeardownFailsWhenDockerResourcesSurvive(t *testing.T) {
	workspace := t.TempDir()

	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		CollectArtifacts: func(string, string, []string) engine.ProjectArtifacts {
			return engine.ProjectArtifacts{Name: "trial", Docker: true}
		},
	})
	if err == nil {
		t.Fatalf("Run reported success while Docker resources survived")
	}
	if !strings.Contains(err.Error(), "Docker resources") {
		t.Fatalf("error = %v, want it to name the surviving resources", err)
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
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(context.Context, string, ...string) error { ran = true; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "already a registered Govard project") {
		t.Fatalf("error = %v, want a refusal naming the registered project", err)
	}
	if ran {
		t.Fatalf("bootstrap ran on an already registered project")
	}
}

func TestTryWithoutSetupRunsNoPrivilegedStep(t *testing.T) {
	workspace := t.TempDir()

	var calls []string
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
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
	err := runTrial(context.Background(), try.Options{Framework: "mageos", Dir: workspace, Sample: true, Setup: true, Timeout: time.Second}, try.Dependencies{
		RunSelf: func(_ context.Context, _ string, args ...string) error {
			calls = append(calls, strings.Join(args, " "))
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(calls) < 2 || calls[0] != "svc up" || !strings.HasPrefix(calls[1], "bootstrap ") {
		t.Fatalf("calls = %v, want svc up then bootstrap", calls)
	}
}

func TestTryHostsHintNamesTheDomain(t *testing.T) {
	hint := try.TryHostsHintForTest("shop.test")
	if !strings.Contains(hint, "127.0.0.1 shop.test") || !strings.Contains(hint, "/etc/hosts") {
		t.Fatalf("hint = %q, want an exact hosts command", hint)
	}
}

func TestTryETALineStatesTheMeasuredDurations(t *testing.T) {
	withSample := try.TryETALineForTest(true)
	for _, want := range []string{"10 minutes", "15", "--no-sample"} {
		if !strings.Contains(withSample, want) {
			t.Fatalf("sampled ETA = %q, want it to mention %q", withSample, want)
		}
	}

	withoutSample := try.TryETALineForTest(false)
	if !strings.Contains(withoutSample, "6 to 10 minutes") || strings.Contains(withoutSample, "--no-sample") {
		t.Fatalf("unsampled ETA = %q, want the measured range only", withoutSample)
	}
}
