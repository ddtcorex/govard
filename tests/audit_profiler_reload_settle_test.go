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

func TestAuditProfilerWaitsForNginxReloadToSettleBeforeCapture(t *testing.T) {
	probes := 0
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if !isWorkerProbe(args) {
			return nil
		}
		probes++
		if probes <= 3 {
			return []byte("nginx: master process\nnginx: worker process is shutting down\nnginx: worker process\n")
		}
		return []byte("nginx: master process\nnginx: worker process\n")
	}, &warns)
	if err := runtime.Activate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if probes != 4 {
		t.Fatalf("worker probes = %d, want 4 (3 unsettled then settled)", probes)
	}
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
}

func TestAuditProfilerContinuesWithWarningWhenReloadNeverSettles(t *testing.T) {
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if isWorkerProbe(args) {
			return []byte("nginx: master process\nnginx: worker process is shutting down\n")
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

func TestAuditProfilerRestoreAlsoWaitsForReload(t *testing.T) {
	probes := 0
	var warns []string
	runtime, request := newSettleRuntime(t, func(args []string) []byte {
		if !isWorkerProbe(args) {
			return nil
		}
		probes++
		return []byte("nginx: master process\nnginx: worker process\n")
	}, &warns)
	if err := runtime.Restore(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if probes != 1 {
		t.Fatalf("worker probes after restore reload = %d, want 1", probes)
	}
}
