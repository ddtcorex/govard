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
