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

	"govard/internal/cli"
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
	root.SetArgs([]string{"bootstrap", "--fresh", "--plan", "--framework", "magento2"})
	if err := root.Execute(); err != nil {
		t.Fatalf("bootstrap --fresh --plan in an empty directory failed: %v", err)
	}

	if initialised {
		t.Fatal("--plan ran the project initialisation step")
	}
}

// runFreshPlanIn runs the real command in dir and returns its output and error.
func runFreshPlanIn(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	cmd.ResetBootstrapFlags()
	t.Cleanup(cmd.ResetBootstrapFlags)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	t.Setenv("GOVARD_HOME_DIR", filepath.Join(t.TempDir(), "govard-home"))

	out := &bytes.Buffer{}
	root := cmd.RootCommandForTest()
	root.SetOut(out)
	root.SetErr(io.Discard)
	root.SetArgs(append([]string{"bootstrap"}, args...))
	err = root.Execute()
	return out.String(), err
}

func TestBootstrapFreshPlanInEmptyDirectoryNeedsNoConfig(t *testing.T) {
	dir := t.TempDir()
	out, err := runFreshPlanIn(t, dir, "--fresh", "--plan", "--framework", "magento2", "--framework-version", "2.4.6")
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	for _, want := range []string{"magento2", "2.4.6", "PHP", "opensearch", ".govard.yml", "govard init"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("plan output lacks %q:\n%s", want, out)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("--plan wrote files into the directory: %v", entries)
	}
}

func TestBootstrapPlanWithoutConfigOrFrameworkIsAUsageError(t *testing.T) {
	dir := t.TempDir()
	_, err := runFreshPlanIn(t, dir, "--fresh", "--plan")
	if err == nil {
		t.Fatal("plan without a framework and without .govard.yml succeeded")
	}
	var usage *cli.UsageError
	if !errors.As(err, &usage) {
		t.Fatalf("error is not a usage error (exit 2): %T %v", err, err)
	}
	for _, want := range []string{"--framework", "govard init"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err.Error(), want)
		}
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("directory not left empty: %v", entries)
	}
}

func TestBootstrapFreshPlanWithExistingConfigIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	config := "project_name: sample-project\nframework: magento2\nframework_version: 2.4.7\ndomain: sample-project.test\n"
	if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	out, err := runFreshPlanIn(t, dir, "--fresh", "--plan")
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	if strings.Contains(out, "govard init") {
		t.Fatalf("an existing config must not produce the would-init note:\n%s", out)
	}
	if !strings.Contains(out, "2.4.7") {
		t.Fatalf("plan ignored the configured version:\n%s", out)
	}
}
