package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"govard/internal/runtime"
)

func stubMissingDocker(t *testing.T) {
	t.Helper()
	restore := runtime.StubProbesForTest(
		func(context.Context) error { return errors.New("cannot connect to the docker daemon") },
		func(context.Context) error { return nil },
	)
	t.Cleanup(restore)
}

func TestIntegrityOnlySelectionDoesNotRequireContainerRuntime(t *testing.T) {
	checks := []string{"integrity"}
	required := auditChecksInclude(checks, "lint") || auditChecksInclude(checks, "profiler")
	if required {
		t.Fatal("an integrity-only selection must not require a container runtime")
	}
}

func TestLintSelectionRequiresContainerRuntime(t *testing.T) {
	checks := []string{"lint"}
	required := auditChecksInclude(checks, "lint") || auditChecksInclude(checks, "profiler")
	if !required {
		t.Fatal("a lint selection must require a container runtime")
	}
}

func TestRequireContainerRuntimeBlocksWithoutDocker(t *testing.T) {
	stubMissingDocker(t)
	err := requireContainerRuntime()
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("requireContainerRuntime = %v, want *runtime.MissingError", err)
	}
	if !strings.Contains(missing.Hint, "--checks integrity") {
		t.Fatalf("hint must point at the container-free check, got %q", missing.Hint)
	}
	if missing.ExitCode() != runtime.CodeCapabilityMissing {
		t.Fatalf("exit code = %d, want %d", missing.ExitCode(), runtime.CodeCapabilityMissing)
	}
}

func TestRequireContainerRuntimeSatisfiedWithDocker(t *testing.T) {
	restore := runtime.StubProbesForTest(
		func(context.Context) error { return nil },
		func(context.Context) error { return nil },
	)
	t.Cleanup(restore)
	// The docker binary itself must resolve for the capability to be satisfied.
	restoreLookPath := runtime.StubLookPathForTest(func(string) (string, error) { return "/usr/bin/docker", nil })
	t.Cleanup(restoreLookPath)

	if err := requireContainerRuntime(); err != nil {
		t.Fatalf("requireContainerRuntime = %v, want nil", err)
	}
}
