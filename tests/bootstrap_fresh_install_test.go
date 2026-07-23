package tests

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
	bootstrapengine "govard/internal/engine/bootstrap"

	"github.com/spf13/cobra"
)

func TestRunBootstrapFreshCreateProjectForTestBuildsExpectedCommand(t *testing.T) {
	var gotCommandLine string
	defer cmd.SetPHPContainerShellRunnerForTest(func(config engine.Config, commandLine string) error {
		gotCommandLine = commandLine
		return nil
	})()

	err := cmd.RunBootstrapFreshCreateProjectForTest(
		&cobra.Command{},
		engine.Config{ProjectName: "sample-project"},
		"magento/project-community-edition",
		"2.4.8",
	)
	if err != nil {
		t.Fatalf("RunBootstrapFreshCreateProjectForTest() error = %v", err)
	}

	if !strings.Contains(gotCommandLine, "composer create-project -n --ignore-platform-reqs --repository-url=https://repo.magento.com 'magento/project-community-edition' /tmp/govard-create-project '2.4.8'") {
		t.Fatalf("unexpected create-project command: %s", gotCommandLine)
	}
	if !strings.Contains(gotCommandLine, "rm -rf /tmp/govard-create-project") {
		t.Fatalf("expected cleanup commands in shell line: %s", gotCommandLine)
	}
}

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

	restoreDownloader := bootstrapengine.SetWordPressCoreDownloaderForTest(func(projectDir string) error {
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
	restoreExec := bootstrapengine.SetDjangoContainerExecRunnerForTest(func(containerName string, script string) error {
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

func TestRunBootstrapDjangoFreshInstallForTestSkipsUpAndMigrateWithNoUp(t *testing.T) {
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
	restoreExec := bootstrapengine.SetDjangoContainerExecRunnerForTest(func(containerName string, script string) error {
		execCalled = true
		return nil
	})
	defer restoreExec()

	err := cmd.RunBootstrapDjangoFreshInstallForTest(&cobra.Command{}, engine.Config{
		ProjectName: "sample-project",
		Framework:   "django",
	}, cmd.BootstrapRuntimeOptions{SkipUp: true})
	if err != nil {
		t.Fatalf("RunBootstrapDjangoFreshInstallForTest() error = %v", err)
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
