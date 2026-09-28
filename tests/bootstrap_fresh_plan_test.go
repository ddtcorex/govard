package tests

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
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

// TestBootstrapFreshPlanDoesNotNameMagentaPackageForLaravel pins the sentinel
// handling: opts.MetaPackage arrives pre-filled with the Magento default, and a
// framework that declares no fresh meta package of its own must not have
// Magento's named in its plan.
func TestBootstrapFreshPlanDoesNotNameMagentaPackageForLaravel(t *testing.T) {
	lines, err := cmd.BuildBootstrapFreshPlanForTest(
		engine.Config{ProjectName: "sample-laravel", Framework: "laravel"},
		"laravel",
		cmd.BootstrapRuntimeOptions{
			Plan:        true,
			MetaPackage: "magento/project-community-edition", // what the flag default supplies
		},
	)
	if err != nil {
		t.Fatalf("BuildBootstrapFreshPlanForTest: %v", err)
	}
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "magento/project-community-edition") {
		t.Fatalf("a Laravel plan named Magento's package:\n%s", joined)
	}
	if !strings.Contains(strings.ToLower(joined), "laravel") {
		t.Fatalf("a Laravel plan does not name Laravel:\n%s", joined)
	}
}

// TestBootstrapPlanDoesNotInitialiseTheProject pins the "--plan touches nothing"
// promise at the step that broke it: the command used to run `govard init`
// (creating .govard.yml and rendering compose/proxy config under the govard
// home) before any plan existed.
func TestBootstrapPlanDoesNotInitialiseTheProject(t *testing.T) {
	initialised := false
	restore := cmd.SetBootstrapEnsureInitForTest(func(*cobra.Command, string) error {
		initialised = true
		return errors.New("init must not run under --plan")
	})
	defer restore()

	project := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	t.Setenv("GOVARD_HOME_DIR", filepath.Join(project, "govard-home"))

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"bootstrap", "--fresh", "--plan"})
	_ = root.Execute()

	if initialised {
		t.Fatal("--plan ran the project initialisation step")
	}
}
