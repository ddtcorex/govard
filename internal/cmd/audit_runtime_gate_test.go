package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"govard/internal/cli"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
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

// auditToolchainCommand resolves one audit toolchain command path from the live
// command tree.
func auditToolchainCommand(t *testing.T, path string) *cobra.Command {
	t.Helper()
	command, _, err := rootCmd.Find(strings.Fields(path))
	if err != nil {
		t.Fatalf("resolve %q: %v", path, err)
	}
	want := "govard " + path
	if command.CommandPath() != want {
		t.Fatalf("resolve %q = %q, want %q", path, command.CommandPath(), want)
	}
	return command
}

// TestAuditToolchainCommandsDeclareContainerRuntime pins the requirement the
// audit parent's "none" annotation would otherwise mask. Every descendant of
// `audit` inherits the nearest annotation, and the audit group declares none so
// the container-free integrity check stays runnable without Docker — so the
// toolchain commands have to declare docker themselves to sit on the right side
// of the gate.
func TestAuditToolchainCommandsDeclareContainerRuntime(t *testing.T) {
	for _, path := range []string{
		"audit toolchain",
		"audit toolchain status",
		"audit toolchain pull",
		"audit toolchain build",
	} {
		command := auditToolchainCommand(t, path)
		requires := runtime.Requires(command)
		if len(requires) != 1 || requires[0] != runtime.CapDocker {
			t.Errorf("%s requires %v, want [docker]", command.CommandPath(), requires)
		}
	}
}

// TestAuditToolchainGateExitsCapabilityMissingWithoutDocker asserts the frozen
// exit-code contract for the three commands that inspect, pull, and build the
// lint image: without a container runtime they must fail through the gate with
// CAPABILITY_MISSING instead of leaking a raw docker error.
func TestAuditToolchainGateExitsCapabilityMissingWithoutDocker(t *testing.T) {
	stubMissingDocker(t)
	for _, path := range []string{
		"audit toolchain status",
		"audit toolchain pull",
		"audit toolchain build",
	} {
		command := auditToolchainCommand(t, path)
		gateErr := gateError(command)
		var missing *runtime.MissingError
		if !errors.As(gateErr, &missing) {
			t.Fatalf("gateError(%s) = %v, want *runtime.MissingError", path, gateErr)
		}
		if code := cli.Code(gateErr); code != runtime.CodeCapabilityMissing {
			t.Errorf("gateError(%s) exit code = %d, want %d", path, code, runtime.CodeCapabilityMissing)
		}
	}
}
