package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/audit"
	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks"
	"govard/internal/frameworks/types"
)

func TestProfilerPreflightPermissionSetsPermissionCode(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{preflightErr: &audit.PermissionError{Path: "/p/.govard/apache/custom", OwnerUID: 0, Err: os.ErrPermission}}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})

	result, err := runner.Run(context.Background(), profilerRunRequest())
	if err == nil {
		t.Fatal("want a preflight failure")
	}
	if result.Status != audit.StatusFailed || len(result.Errors) != 1 {
		t.Fatalf("result = %#v, want one failed error", result)
	}
	if result.Errors[0].Code != audit.ErrorCodePermission {
		t.Fatalf("error code = %q, want %q", result.Errors[0].Code, audit.ErrorCodePermission)
	}
	for _, needle := range []string{"/p/.govard/apache/custom", "govard doctor --fix"} {
		if !strings.Contains(result.Errors[0].Message, needle) {
			t.Fatalf("message %q lacks %q", result.Errors[0].Message, needle)
		}
	}
	if want := []string{"preflight"}; !reflect.DeepEqual(runtime.events, want) {
		t.Fatalf("runtime events = %#v, want only the preflight (no activation)", runtime.events)
	}
	if _, err := store.AcquireLease(result.ProjectID, "diagnostics", "next-owner"); err != nil {
		t.Fatalf("a failed preflight must not hold the lease: %v", err)
	}
}

func TestInfrastructureErrorWithoutCodeStaysInfrastructure(t *testing.T) {
	store := newDeterministicAuditStore(t)
	runtime := &fakeProfilerRuntime{activateErr: errors.New("activate profiler")}
	runner := audit.NewRunner(audit.RunnerOptions{Store: store, ProfilerRuntime: runtime, Resources: audit.Resources{CPU: 2, MemoryMB: 2048}})
	result, _ := runner.Run(context.Background(), profilerRunRequest())
	if len(result.Errors) != 1 || result.Errors[0].Code != audit.ErrorCodeInfrastructure {
		t.Fatalf("errors = %#v, want one infrastructure error", result.Errors)
	}
}

func newPreflightRuntime(t *testing.T, projectRoot string) audit.ProfilerRuntime {
	t.Helper()
	definition, ok := frameworks.Get("magento2")
	if !ok {
		t.Fatal("Magento 2 definition is not registered")
	}
	config := &engine.Config{ProjectName: "sample-project", Framework: "magento2"}
	config.Stack.Services.WebServer = "apache"
	target := types.AuditTarget{Framework: "magento2", ProjectRoot: projectRoot, TargetPath: projectRoot, Mode: types.AuditTargetProject}
	runtime, err := cmd.NewAuditProfilerRuntimeForTest(cmd.AuditRunnerRequest{
		ProjectRoot: projectRoot, Definition: definition, Target: target, Config: config, ProfilerRuntimeRequired: true,
	}, cmd.AuditProfilerRuntimeDependenciesForTest{
		RunDocker: func(context.Context, ...string) ([]byte, error) { return nil, nil },
		HTTPGet:   func(context.Context, string) (int, error) { return 200, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func preflightRequest(projectRoot string) audit.ProfilerRequest {
	return audit.ProfilerRequest{
		ProjectRoot: projectRoot, ProjectID: "project-a", SessionID: "session-a", RunID: "run-0001",
		URL:    "https://sample-project.test/",
		Target: types.AuditTarget{Framework: "magento2", ProjectRoot: projectRoot, TargetPath: projectRoot, Mode: types.AuditTargetProject},
	}
}

func TestGovardProfilerPreflightDetectsUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	projectRoot := t.TempDir()
	custom := filepath.Join(projectRoot, ".govard", "apache", "custom")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(custom, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(custom, 0o755) })

	err := newPreflightRuntime(t, projectRoot).Preflight(context.Background(), preflightRequest(projectRoot))
	var permission *audit.PermissionError
	if !errors.As(err, &permission) {
		t.Fatalf("preflight error = %v, want *audit.PermissionError", err)
	}
	if permission.Path != custom {
		t.Fatalf("permission path = %q, want %q", permission.Path, custom)
	}
}

func TestGovardProfilerPreflightPassesWritableDir(t *testing.T) {
	projectRoot := t.TempDir()
	if err := newPreflightRuntime(t, projectRoot).Preflight(context.Background(), preflightRequest(projectRoot)); err != nil {
		t.Fatalf("preflight on a writable project: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(projectRoot, ".govard", "apache", "custom"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("preflight must leave the custom dir empty: %v %v", entries, err)
	}
}
