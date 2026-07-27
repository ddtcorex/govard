package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	"govard/internal/frameworks/django"
	"govard/internal/frameworks/wordpress"

	"github.com/spf13/cobra"
)

func TestFrameworkFreshInstallManagesOwnEnvUpForTest(t *testing.T) {
	cases := []struct {
		framework string
		want      bool
	}{
		{"django", true},
		{"laravel", false},
		{"nextjs", false},
		{"wordpress", false},
	}

	for _, tc := range cases {
		if got := cmd.FrameworkFreshInstallManagesOwnEnvUpForTest(tc.framework); got != tc.want {
			t.Errorf("FrameworkFreshInstallManagesOwnEnvUpForTest(%q) = %v, want %v", tc.framework, got, tc.want)
		}
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestRejectsUnsupportedFramework(t *testing.T) {
	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{Framework: "custom"},
		"dev",
		"",
	)
	if err == nil {
		t.Fatal("expected unsupported framework error")
	}
	if !strings.Contains(err.Error(), "fresh install not supported for framework: custom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestNextJSUsesThrowawayContainer(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedProjectDir string
	var capturedCommand string
	defer cmd.SetNodeCreateProjectRunnerForTest(func(config engine.Config, projectDir string, commandLine string) error {
		capturedProjectDir = projectDir
		capturedCommand = commandLine
		stageDir := extractStageHostDir(t, commandLine)
		return os.WriteFile(filepath.Join(stageDir, "package.json"), []byte("{\"name\":\"nextjs-app\"}\n"), 0o644)
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "nextjs",
			Domain:      "sample.test",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if capturedProjectDir != tempDir {
		t.Fatalf("expected runner to receive project dir %q, got %q", tempDir, capturedProjectDir)
	}
	if !strings.Contains(capturedCommand, "npx create-next-app@latest") {
		t.Fatalf("expected create-next-app invocation, got: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "package.json")); err != nil {
		t.Fatalf("expected staged package.json to land in the project dir: %v", err)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestWordPressDoesNotRestartEnvironment(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	restoreDownloader := wordpress.SetWordPressCoreDownloaderForTest(func(projectDir string) error {
		samplePath := filepath.Join(projectDir, "wp-config-sample.php")
		return os.WriteFile(samplePath, []byte("<?php\n"), 0o644)
	})
	defer restoreDownloader()

	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		return nil
	})()

	calls := make([][]string, 0, 2)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		captured := append([]string{}, args...)
		calls = append(calls, captured)
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "wordpress",
			Domain:      "sample.test",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestCakePHPUsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "cakephp",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "composer create-project --prefer-dist cakephp/app") {
		t.Fatalf("expected a CakePHP create-project invocation, got: %s", capturedCommands[0])
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestLaravelUsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "laravel",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "composer create-project laravel/laravel") {
		t.Fatalf("expected a Laravel create-project invocation, got: %s", capturedCommands[0])
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestDrupalUsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "drupal",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "composer create-project drupal/recommended-project") {
		t.Fatalf("expected a Drupal create-project invocation, got: %s", capturedCommands[0])
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestShopwareUsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "shopware",
			Domain:      "sample.test",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "composer create-project shopware/production") {
		t.Fatalf("expected a Shopware create-project invocation, got: %s", capturedCommands[0])
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestSymfonyUsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	calls := make([][]string, 0, 1)
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "symfony",
		},
		"dev",
		"",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "composer create-project symfony/skeleton") {
		t.Fatalf("expected a Symfony create-project invocation, got: %s", capturedCommands[0])
	}

	want := [][]string{
		{"config", "auto"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("subcommand calls = %#v, want %#v", calls, want)
	}
}

// TestRunBootstrapFrameworkFreshInstallForTestMagento2UsesRegistryFreshInstall
// exercises the actual registry dispatch path for Magento 2 end-to-end
// (RunBootstrapFrameworkFreshInstallWithOptionsForTest ->
// runBootstrapRegistryFreshInstall -> magento2.freshInstall ->
// bootstrap.MagentoFamilyFreshInstall), rather than only unit-testing the
// pure bootstrap.BuildMagentoFreshCreateProjectCommand/
// BuildMagentoSetupInstallArgs builder functions in isolation
// (tests/magento_family_fresh_install_test.go) - this is the test that
// would catch a dispatcher wiring regression (e.g. magento2 silently
// falling through to the wrong case, or losing FreshInstall/DisplayName).
//
// AssumeYes: true is required here (unlike every other framework's
// dispatch test in this file) because the Magento family's FreshInstall
// is the only one that calls helpers.EnsureAuthJSON ->
// ensureBootstrapAuthJSON, which - without AssumeYes - falls into a real
// pterm interactive prompt (Show()) asking whether to reuse
// ~/.composer/auth.json. That prompt reads real stdin, which hangs a
// `go test` run indefinitely (there is no keystroke coming) instead of
// failing fast, so the test would appear to "pass" in CI (where stdin
// isn't a TTY) yet hang forever for any developer running it locally with
// a real terminal attached and a real auth.json on disk. AssumeYes
// mirrors `govard bootstrap --fresh --yes` and makes
// ensureBootstrapAuthJSON short-circuit to the non-interactive
// "use global auth.json" (or, absent one, the warning-and-continue)
// branch instead.
func TestRunBootstrapFrameworkFreshInstallForTestMagento2UsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	var calls [][]string
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallWithOptionsForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   "magento2",
		Domain:      "sample.test",
	}, cmd.BootstrapRuntimeOptions{
		Source:      "dev",
		MetaPackage: "magento/project-community-edition",
		AssumeYes:   true,
	})
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallWithOptionsForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "https://repo.magento.com") {
		t.Fatalf("expected magento2 create-project command to reference repo.magento.com, got: %s", capturedCommands[0])
	}
	if strings.Contains(capturedCommands[0], "repo.mage-os.org") {
		t.Fatalf("expected magento2 create-project command to NOT reference repo.mage-os.org, got: %s", capturedCommands[0])
	}

	var setupInstallArgs []string
	configuredAuto := false
	for _, args := range calls {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "setup:install") {
			setupInstallArgs = args
		}
		if joined == "config auto" {
			configuredAuto = true
		}
	}
	if len(setupInstallArgs) == 0 {
		t.Fatalf("expected a magento setup:install subcommand, calls: %#v", calls)
	}
	joined := strings.Join(setupInstallArgs, " ")
	if !strings.Contains(joined, "--db-name=magento") || !strings.Contains(joined, "--db-user=magento") || !strings.Contains(joined, "--db-password=magento") {
		t.Fatalf("expected magento db credentials in setup args, got %q", joined)
	}
	if !strings.Contains(joined, "--admin-email=admin@sample.test") {
		t.Fatalf("expected admin email derived from config.Domain in setup args, got %q", joined)
	}
	if !configuredAuto {
		t.Fatalf("expected govard config auto to run, calls: %#v", calls)
	}
}

// TestRunBootstrapFrameworkFreshInstallForTestMageOSUsesRegistryFreshInstall
// is TestRunBootstrapFrameworkFreshInstallForTestMagento2UsesRegistryFreshInstall's
// Mage-OS counterpart - both frameworks share
// bootstrap.MagentoFamilyFreshInstall, but the review that flagged this gap
// specifically called out that neither had end-to-end dispatch coverage,
// and MageOSVariant's repository URL/DB credentials genuinely differ from
// Magento2Variant's, so both are covered rather than assuming parity.
func TestRunBootstrapFrameworkFreshInstallForTestMageOSUsesRegistryFreshInstall(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedCommands []string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		capturedCommands = append(capturedCommands, commandLine)
		return nil
	})()

	var calls [][]string
	defer cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		calls = append(calls, append([]string{}, args...))
		return nil
	})()

	err := cmd.RunBootstrapFrameworkFreshInstallWithOptionsForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   "mageos",
		Domain:      "sample.test",
	}, cmd.BootstrapRuntimeOptions{
		Source:      "dev",
		MetaPackage: "mage-os/project-community-edition",
		AssumeYes:   true,
	})
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallWithOptionsForTest() error = %v", err)
	}

	if len(capturedCommands) == 0 {
		t.Fatal("expected at least one PHP container command to be run")
	}
	if !strings.Contains(capturedCommands[0], "https://repo.mage-os.org") {
		t.Fatalf("expected mageos create-project command to reference repo.mage-os.org, got: %s", capturedCommands[0])
	}
	if strings.Contains(capturedCommands[0], "repo.magento.com") {
		t.Fatalf("expected mageos create-project command to NOT reference repo.magento.com, got: %s", capturedCommands[0])
	}

	var setupInstallArgs []string
	configuredAuto := false
	for _, args := range calls {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "setup:install") {
			setupInstallArgs = args
		}
		if joined == "config auto" {
			configuredAuto = true
		}
	}
	if len(setupInstallArgs) == 0 {
		t.Fatalf("expected a magento setup:install subcommand, calls: %#v", calls)
	}
	joined := strings.Join(setupInstallArgs, " ")
	if !strings.Contains(joined, "--db-name=mageos") || !strings.Contains(joined, "--db-user=mageos") || !strings.Contains(joined, "--db-password=mageos") {
		t.Fatalf("expected mageos db credentials in setup args, got %q", joined)
	}
	if !strings.Contains(joined, "--admin-email=admin@sample.test") {
		t.Fatalf("expected admin email derived from config.Domain in setup args, got %q", joined)
	}
	if !configuredAuto {
		t.Fatalf("expected govard config auto to run, calls: %#v", calls)
	}
}

func TestRunBootstrapFrameworkFreshInstallForTestDjangoScaffoldsAndMigrates(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	var capturedProjectDir string
	var capturedCommand string
	restorePython := cmd.SetPythonCreateProjectRunnerForTest(func(config engine.Config, projectDir string, commandLine string) error {
		capturedProjectDir = projectDir
		capturedCommand = commandLine
		stageDir := extractStageHostDir(t, commandLine)
		if err := os.WriteFile(filepath.Join(stageDir, "manage.py"), []byte("#!/usr/bin/env python\n"), 0o644); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(stageDir, "config"), 0o755); err != nil {
			return err
		}
		settingsContent := "from pathlib import Path\n\nBASE_DIR = Path(__file__).resolve().parent.parent\n\nDATABASES = {\n    'default': {\n        'ENGINE': 'django.db.backends.sqlite3',\n        'NAME': BASE_DIR / 'db.sqlite3',\n    }\n}\n"
		return os.WriteFile(filepath.Join(stageDir, "config", "settings.py"), []byte(settingsContent), 0o644)
	})
	defer restorePython()

	var subcommandCalls [][]string
	restoreSubcommand := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		subcommandCalls = append(subcommandCalls, append([]string{}, args...))
		return nil
	})
	defer restoreSubcommand()

	var execContainer, execScript string
	restoreExec := django.SetDjangoContainerExecRunnerForTest(func(containerName string, script string) error {
		execContainer = containerName
		execScript = script
		return nil
	})
	defer restoreExec()

	err := cmd.RunBootstrapFrameworkFreshInstallForTest(
		&cobra.Command{},
		engine.Config{
			ProjectName: "sample-project",
			Framework:   "django",
			Domain:      "sample.test",
		},
		"dev",
		"5.1",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallForTest() error = %v", err)
	}

	if capturedProjectDir != tempDir {
		t.Fatalf("expected python runner to receive project dir %q, got %q", tempDir, capturedProjectDir)
	}
	if !strings.Contains(capturedCommand, "django-admin startproject config") {
		t.Fatalf("expected django-admin invocation, got: %s", capturedCommand)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "manage.py")); err != nil {
		t.Fatalf("expected staged manage.py to land in the project dir: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "requirements.txt")); err != nil {
		t.Fatalf("expected requirements.txt to be generated: %v", err)
	}

	wantSubcommands := [][]string{
		{"env", "up", "--remove-orphans"},
	}
	if !reflect.DeepEqual(subcommandCalls, wantSubcommands) {
		t.Fatalf("subcommand calls = %#v, want %#v", subcommandCalls, wantSubcommands)
	}

	if execContainer != "sample-project-web-1" {
		t.Errorf("expected Install() to exec into sample-project-web-1, got %q", execContainer)
	}
	if execScript != "pip install --no-cache-dir -r requirements.txt && python manage.py migrate" {
		t.Errorf("unexpected Install() script: %q", execScript)
	}
}

func TestRunBootstrapFrameworkFreshInstallWithOptionsForTestDjangoSkipsUpAndMigrateWithNoUp(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)

	restorePython := cmd.SetPythonCreateProjectRunnerForTest(func(config engine.Config, projectDir string, commandLine string) error {
		stageDir := extractStageHostDir(t, commandLine)
		if err := os.WriteFile(filepath.Join(stageDir, "manage.py"), []byte("#!/usr/bin/env python\n"), 0o644); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(stageDir, "config"), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(stageDir, "config", "settings.py"), []byte("from pathlib import Path\n"), 0o644)
	})
	defer restorePython()

	subcommandCalled := false
	restoreSubcommand := cmd.SetGovardSubcommandRunnerForTest(func(subCmd *cobra.Command, args ...string) error {
		subcommandCalled = true
		return nil
	})
	defer restoreSubcommand()

	execCalled := false
	restoreExec := django.SetDjangoContainerExecRunnerForTest(func(containerName string, script string) error {
		execCalled = true
		return nil
	})
	defer restoreExec()

	err := cmd.RunBootstrapFrameworkFreshInstallWithOptionsForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   "django",
	}, cmd.BootstrapRuntimeOptions{SkipUp: true})
	if err != nil {
		t.Fatalf("RunBootstrapFrameworkFreshInstallWithOptionsForTest() error = %v", err)
	}

	if subcommandCalled {
		t.Error("expected env up NOT to be called when --no-up is set")
	}
	if execCalled {
		t.Error("expected Install()/migrate NOT to be called when --no-up is set")
	}
}

func TestPythonCreateProjectRunnerBuildsExpectedDockerCommand(t *testing.T) {
	var gotArgsCaptured bool
	restore := cmd.SetPythonCreateProjectRunnerForTest(func(config engine.Config, projectDir string, commandLine string) error {
		gotArgsCaptured = true
		if !strings.Contains(commandLine, "django-admin") {
			t.Errorf("expected commandLine to be forwarded unchanged, got %q", commandLine)
		}
		return nil
	})
	defer restore()

	err := cmd.RunPythonCreateProjectContainerForTest(
		engine.Config{Stack: engine.Stack{PythonVersion: "3.12"}},
		"/tmp/some-project",
		"django-admin startproject config .",
	)
	if err != nil {
		t.Fatalf("RunPythonCreateProjectContainerForTest() error = %v", err)
	}
	if !gotArgsCaptured {
		t.Fatal("expected overridden runner to be invoked")
	}
}
