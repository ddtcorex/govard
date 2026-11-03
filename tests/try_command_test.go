package tests

import (
	"errors"
	"testing"
	"time"

	"govard/internal/cli"
	"govard/internal/cmd"
	"govard/internal/runtime"
)

func resetTryFlags(t *testing.T) {
	t.Helper()

	c := cmd.TryCommandForTest()
	for _, name := range []string{"no-sample", "keep", "setup", "no-open"} {
		if err := c.Flags().Set(name, "false"); err != nil {
			t.Fatalf("reset flag %s: %v", name, err)
		}
	}
	if err := c.Flags().Set("dir", ""); err != nil {
		t.Fatalf("reset dir: %v", err)
	}
	if err := c.Flags().Set("timeout", (20 * time.Minute).String()); err != nil {
		t.Fatalf("reset timeout: %v", err)
	}
}

func TestResolveTryOptionsInstallsSampleDataByDefault(t *testing.T) {
	resetTryFlags(t)

	opts, err := cmd.ResolveTryOptionsForTest(nil)
	if err != nil {
		t.Fatalf("ResolveTryOptionsForTest: %v", err)
	}
	if !opts.Sample {
		t.Fatalf("Sample = false, want sample data installed by default")
	}
}

func TestResolveTryOptionsHonoursTheNoSampleFlag(t *testing.T) {
	resetTryFlags(t)
	if err := cmd.TryCommandForTest().Flags().Set("no-sample", "true"); err != nil {
		t.Fatalf("set flag: %v", err)
	}
	t.Cleanup(func() { resetTryFlags(t) })

	opts, err := cmd.ResolveTryOptionsForTest(nil)
	if err != nil {
		t.Fatalf("ResolveTryOptionsForTest: %v", err)
	}
	if opts.Sample {
		t.Fatalf("Sample = true with --no-sample, want false")
	}
}

func TestResolveTryOptionsRejectsAnUnknownFrameworkAsUsageError(t *testing.T) {
	resetTryFlags(t)

	_, err := cmd.ResolveTryOptionsForTest([]string{"wordpress2"})
	if err == nil {
		t.Fatalf("ResolveTryOptionsForTest accepted an unknown framework")
	}

	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("error = %T, want *cli.UsageError so the exit code is 2", err)
	}
}

func TestTryCommandDefaultsToMageOS(t *testing.T) {
	resetTryFlags(t)

	opts, err := cmd.ResolveTryOptionsForTest(nil)
	if err != nil {
		t.Fatalf("ResolveTryOptionsForTest: %v", err)
	}
	if opts.Framework != "mageos" {
		t.Fatalf("framework = %q, want mageos", opts.Framework)
	}
}

func TestTryCommandDeclaresDockerCapability(t *testing.T) {
	got := cmd.TryCommandForTest().Annotations[runtime.AnnotationRequires]
	if want := string(runtime.CapDocker); got != want {
		t.Fatalf("try requirement = %q, want %q", got, want)
	}
}
