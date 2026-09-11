package cmd

import (
	"context"
	"errors"
	"testing"

	"govard/internal/cli"
	"govard/internal/runtime"

	"github.com/spf13/cobra"
)

func TestGateSkipsAlwaysRunnableCommands(t *testing.T) {
	cmd := &cobra.Command{Use: "doctor"}
	cmd.RunE = func(*cobra.Command, []string) error { return nil }
	if err := gateError(cmd); err != nil {
		t.Fatalf("gateError(doctor) = %v, want nil", err)
	}
}

func TestGateAllowsRequirementFreeCommand(t *testing.T) {
	cmd := &cobra.Command{
		Use:         "init",
		Annotations: map[string]string{runtime.AnnotationRequires: "none"},
	}
	cmd.RunE = func(*cobra.Command, []string) error { return nil }
	if err := gateError(cmd); err != nil {
		t.Fatalf("gateError(init) = %v, want nil", err)
	}
}

func TestGateReportsMissingDocker(t *testing.T) {
	restore := runtime.StubProbesForTest(
		func(context.Context) error { return errors.New("cannot connect to the docker daemon") },
		func(context.Context) error { return nil },
	)
	defer restore()

	cmd := &cobra.Command{
		Use:         "up",
		Annotations: map[string]string{runtime.AnnotationRequires: "docker"},
	}
	cmd.RunE = func(*cobra.Command, []string) error { return nil }

	err := gateError(cmd)
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("gateError = %v, want *runtime.MissingError", err)
	}
	if got := cli.Code(err); got != runtime.CodeCapabilityMissing {
		t.Fatalf("exit code = %d, want %d", got, runtime.CodeCapabilityMissing)
	}
}

// TestGateBlocksNestedDoctorCommand pins the always-runnable set to TOP-LEVEL
// commands. `doctor` is the diagnostic entry point that must survive a host
// without Docker, but `desktop doctor` is a different, container-backed command:
// matching the leaf name let it through the gate on a Docker-free host even
// though its manifest requirement is docker.
func TestGateBlocksNestedDoctorCommand(t *testing.T) {
	stubMissingDocker(t)
	root := &cobra.Command{Use: "govard"}
	desktop := &cobra.Command{
		Use:         "desktop",
		Annotations: map[string]string{runtime.AnnotationRequires: "docker"},
	}
	desktop.RunE = func(*cobra.Command, []string) error { return nil }
	doctor := &cobra.Command{Use: "doctor"}
	doctor.RunE = func(*cobra.Command, []string) error { return nil }
	root.AddCommand(desktop)
	desktop.AddCommand(doctor)

	err := gateError(doctor)
	var missing *runtime.MissingError
	if !errors.As(err, &missing) {
		t.Fatalf("gateError(desktop doctor) = %v, want *runtime.MissingError", err)
	}
	if got := cli.Code(err); got != runtime.CodeCapabilityMissing {
		t.Fatalf("exit code = %d, want %d", got, runtime.CodeCapabilityMissing)
	}
}

// TestGateSkipsAlwaysRunnableSubcommands is the mirror case: the always-runnable
// groups carry subcommands (`completion bash`, ...) that are part of the same
// operator-facing surface, so blocking them on a host without Docker denies
// shell setup for exactly the reason the set exists.
func TestGateSkipsAlwaysRunnableSubcommands(t *testing.T) {
	stubMissingDocker(t)
	root := &cobra.Command{Use: "govard"}
	completion := &cobra.Command{Use: "completion"}
	completion.RunE = func(*cobra.Command, []string) error { return nil }
	bash := &cobra.Command{Use: "bash"}
	bash.RunE = func(*cobra.Command, []string) error { return nil }
	root.AddCommand(completion)
	completion.AddCommand(bash)

	if err := gateError(bash); err != nil {
		t.Fatalf("gateError(completion bash) = %v, want nil", err)
	}
}

// TestRealTreeAlwaysRunnableSet covers the shipped command tree rather than a
// synthetic one, because the defect it guards against is a name collision that
// only exists in the real tree.
func TestRealTreeAlwaysRunnableSet(t *testing.T) {
	// The help command is added lazily by cobra, exactly as the first Execute
	// would; the completion children are only registered by Execute, which is
	// why their case is covered by the synthetic test above.
	rootCmd.InitDefaultHelpCmd()
	byName := map[string]bool{}
	walkCommandTree(rootCmd, func(cmd *cobra.Command) {
		if cmd != rootCmd && runtime.AlwaysRunnable(cmd) {
			byName[cmd.CommandPath()] = true
		}
	})
	for _, path := range []string{
		"govard doctor",
		"govard version",
		"govard capabilities",
		"govard help",
	} {
		if !byName[path] {
			t.Errorf("%s must stay runnable on any host", path)
		}
	}
	if byName["govard desktop doctor"] {
		t.Error("govard desktop doctor declares docker, so it must not be always-runnable")
	}
}
