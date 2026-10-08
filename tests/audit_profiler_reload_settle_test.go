package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func newSettleRuntime(t *testing.T, run func(args []string) []byte, warns *[]string) (audit.ProfilerRuntime, audit.ProfilerRequest) {
	t.Helper()
	projectRoot := t.TempDir()
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("Magento 2 definition is not registered")
	}
	config := &engine.Config{ProjectName: "audit-shop", Framework: "magento2"}
	config.Stack.Services.WebServer = "nginx"
	target := types.AuditTarget{Framework: "magento2", ProjectRoot: projectRoot, TargetPath: projectRoot, Mode: types.AuditTargetProject}
	runtime, err := cmd.NewAuditProfilerRuntimeForTest(cmd.AuditRunnerRequest{
		ProjectRoot: projectRoot, Definition: definition, Target: target, Config: config, ProfilerRuntimeRequired: true,
	}, cmd.AuditProfilerRuntimeDependenciesForTest{
		RunDocker:      func(_ context.Context, args ...string) ([]byte, error) { return run(args), nil },
		HTTPGet:        func(context.Context, string) (int, error) { return 200, nil },
		SettleInterval: time.Millisecond,
		SettleTimeout:  200 * time.Millisecond,
		Warn:           func(message string) { *warns = append(*warns, message) },
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(projectRoot, ".govard"), 0o755)
	return runtime, audit.ProfilerRequest{
		ProjectRoot: projectRoot, ProjectID: "p", SessionID: "s", RunID: "run-0001",
		URL: "https://audit-shop.test/", Target: target,
	}
}

func isWorkerProbe(args []string) bool {
	return len(args) > 2 && args[0] == "exec" && args[1] == "audit-shop-web-1" && args[2] == "sh"
}

// The probe prints `<pid> <process title>` for every process. One probe runs
// BEFORE the reload to learn which workers are the old ones; the polls after it
// must see a worker that is not one of them.
const oldWorkers = "1 nginx: master process\n10 nginx: worker process\n11 nginx: worker process\n"
const newWorkers = "1 nginx: master process\n20 nginx: worker process\n21 nginx: worker process\n"

// `nginx -s reload` only signals the master. Right after it returns the master
// may not have acted yet: every worker is still the old one and none is shutting
// down. That state must NOT count as settled, or the capture is served by an old
// worker without the profiler variable (the live failure).
func TestAuditProfilerDoesNotTreatTheOldWorkersAsSettled(t *testing.T) {
	probes := 0
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if !isWorkerProbe(args) {
			return nil
		}
		probes++
		switch {
		case probes <= 3: // the pre-reload snapshot, then two polls before the master reacts
			return []byte(oldWorkers)
		case probes == 4: // new workers are up, but the old ones still run normally
			return []byte(newWorkers + "10 nginx: worker process\n11 nginx: worker process\n")
		default: // the old ones were told to quit: they no longer accept connections
			return []byte(newWorkers + "10 nginx: worker process is shutting down\n11 nginx: worker process is shutting down\n")
		}
	}, &warns)
	if err := runtime.Activate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if probes != 5 {
		t.Fatalf("worker probes = %d, want 5 (snapshot, 2 polls on the old workers, 1 with old and new both running, 1 settled)", probes)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
}

func TestAuditProfilerContinuesWithWarningWhenReloadNeverSettles(t *testing.T) {
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if isWorkerProbe(args) {
			return []byte(oldWorkers) // the master never reacts
		}
		return nil
	}, &warns)
	if err := runtime.Activate(context.Background(), request); err != nil {
		t.Fatalf("Activate must not fail on settle timeout: %v", err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "did not settle") {
		t.Fatalf("warnings = %v, want one 'did not settle' warning", warns)
	}
}

// When the workers cannot be listed before the reload there is nothing to compare
// against: fall back to "some worker exists and none is shutting down".
func TestAuditProfilerFallsBackWhenTheOldWorkersCannotBeListed(t *testing.T) {
	probes := 0
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if !isWorkerProbe(args) {
			return nil
		}
		probes++
		if probes == 1 {
			return nil // no listing before the reload
		}
		return []byte(newWorkers)
	}, &warns)
	if err := runtime.Activate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if probes != 2 || len(warns) != 0 {
		t.Fatalf("probes = %d warns = %v, want 2 probes and no warning", probes, warns)
	}
}

func TestAuditProfilerRestoreAlsoWaitsForReload(t *testing.T) {
	probes := 0
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if !isWorkerProbe(args) {
			return nil
		}
		probes++
		if probes == 1 {
			return []byte(oldWorkers)
		}
		return []byte(newWorkers)
	}, &warns)
	if err := runtime.Restore(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if probes != 2 {
		t.Fatalf("worker probes around the restore reload = %d, want 2 (snapshot + settled)", probes)
	}
}
