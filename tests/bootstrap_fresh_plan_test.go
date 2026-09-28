package tests

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// freshPlanCmd is a cobra command whose stdout we can inspect.
func freshPlanCmd() (*cobra.Command, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	c := &cobra.Command{}
	c.SetOut(buf)
	return c, buf
}

func TestBootstrapFreshPlanDoesNotInstall(t *testing.T) {
	called := false
	restore := cmd.SetBootstrapFreshInstallForTest(func(*cobra.Command, engine.Config, cmd.BootstrapRuntimeOptions) error {
		called = true
		return errors.New("fresh install must not run under --plan")
	})
	defer restore()

	c, out := freshPlanCmd()
	config := engine.Config{ProjectName: "sample-project", Framework: "magento2"}
	opts := cmd.BootstrapRuntimeOptions{Plan: true, MetaVersion: "2.4.9"}

	planned, err := cmd.RunBootstrapFreshForTest(c, config, opts)
	if err != nil {
		t.Fatalf("RunBootstrapFreshForTest(--plan) = %v, want nil", err)
	}
	if !planned {
		t.Fatal("RunBootstrapFreshForTest(--plan) reported planned=false; it must report that it only planned")
	}
	if called {
		t.Fatal("the fresh install ran under --plan; it must be skipped")
	}
	if !strings.Contains(out.String(), "env up") {
		t.Fatalf("plan output does not show the env-up step:\n%s", out.String())
	}
}

func TestBootstrapFreshPlanNamesTheMetaPackage(t *testing.T) {
	lines, err := cmd.BuildBootstrapFreshPlanForTest(
		engine.Config{ProjectName: "sample-project", Framework: "magento2"},
		"magento2",
		cmd.BootstrapRuntimeOptions{Plan: true, MetaVersion: "2.4.9"},
	)
	if err != nil {
		t.Fatalf("BuildBootstrapFreshPlanForTest: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "project") {
		t.Fatalf("fresh plan does not describe the install:\n%s", joined)
	}
	if !strings.Contains(joined, "2.4.9") {
		t.Fatalf("fresh plan does not carry the requested version:\n%s", joined)
	}
}

func TestBootstrapFreshPlanSkipsEnvUpWhenAsked(t *testing.T) {
	lines, err := cmd.BuildBootstrapFreshPlanForTest(
		engine.Config{ProjectName: "sample-project", Framework: "magento2"},
		"magento2",
		cmd.BootstrapRuntimeOptions{Plan: true, SkipUp: true},
	)
	if err != nil {
		t.Fatalf("BuildBootstrapFreshPlanForTest: %v", err)
	}
	if strings.Contains(strings.Join(lines, "\n"), "env up") {
		t.Fatalf("--skip-up must drop the env-up step:\n%s", strings.Join(lines, "\n"))
	}
}

// TestBootstrapFreshInstallSeamIsLiveUnderNoPlan is the positive control for
// TestBootstrapFreshPlanDoesNotInstall: it proves the "install was called"
// detector actually fires when a real install runs. Without this, the --plan
// test could pass because the detector is dead rather than because the install
// was skipped. SkipUp keeps every container out of it.
func TestBootstrapFreshInstallSeamIsLiveUnderNoPlan(t *testing.T) {
	called := false
	restore := cmd.SetBootstrapFreshInstallForTest(func(*cobra.Command, engine.Config, cmd.BootstrapRuntimeOptions) error {
		called = true
		return nil
	})
	defer restore()

	c, _ := freshPlanCmd()
	config := engine.Config{ProjectName: "sample-project", Framework: "magento2"}

	planned, err := cmd.RunBootstrapFreshForTest(c, config, cmd.BootstrapRuntimeOptions{SkipUp: true})
	if err != nil {
		t.Fatalf("RunBootstrapFreshForTest(no plan) = %v, want nil", err)
	}
	if planned {
		t.Fatal("planned=true without --plan; a real run must report that it installed")
	}
	if !called {
		t.Fatal("the fresh install seam did not fire without --plan; the detector is dead")
	}
}
