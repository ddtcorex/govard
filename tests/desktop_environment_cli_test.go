package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func writeDesktopProjectConfigForTest(t *testing.T, root string, projectName string, framework string, domain string) {
	t.Helper()
	content := strings.TrimSpace(
		"project_name: "+projectName+"\n"+
			"framework: "+framework+"\n"+
			"domain: "+domain+"\n",
	) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write .govard.yml: %v", err)
	}
}

func registerDesktopProjectForTest(t *testing.T, root string, projectName string, domain string, framework string) {
	t.Helper()
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:        root,
		ProjectName: projectName,
		Domain:      domain,
		Framework:   framework,
		LastSeenAt:  time.Now().UTC(),
		LastCommand: "desktop-test",
	}); err != nil {
		t.Fatalf("register project: %v", err)
	}
}

func TestDesktopStartEnvironmentUsesGovardUpForProjectForTest(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	var gotDir string
	var gotArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		gotDir = dir
		gotArgs = append([]string{}, args...)
		return "env started via cli", nil
	})
	defer restore()

	app := desktop.NewApp()
	message, err := app.Environment.StartEnvironment("sample-project")
	if err != nil {
		t.Fatalf("StartEnvironment failed: %v", err)
	}
	if !sameTestFile(t, gotDir, root) {
		t.Fatalf("expected govard dir %q, got %q", root, gotDir)
	}
	if !reflect.DeepEqual(gotArgs, []string{"up", "--force-recreate", "--remove-orphans"}) {
		t.Fatalf("unexpected govard args: %#v", gotArgs)
	}
	if !strings.Contains(message, "env started via cli") {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestDesktopStopEnvironmentUsesGovardEnvStopForProjectForTest(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	var gotDir string
	var gotArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		gotDir = dir
		gotArgs = append([]string{}, args...)
		return "env stopped via cli", nil
	})
	defer restore()

	app := desktop.NewApp()
	message, err := app.Environment.StopEnvironment("sample-project")
	if err != nil {
		t.Fatalf("StopEnvironment failed: %v", err)
	}
	if !sameTestFile(t, gotDir, root) {
		t.Fatalf("expected govard dir %q, got %q", root, gotDir)
	}
	if !reflect.DeepEqual(gotArgs, []string{"env", "stop"}) {
		t.Fatalf("unexpected govard args: %#v", gotArgs)
	}
	if !strings.Contains(message, "env stopped via cli") {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestDesktopPullEnvironmentUsesGovardEnvPullForProjectForTest(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	var gotDir string
	var gotArgs []string
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		gotDir = dir
		gotArgs = append([]string{}, args...)
		return "env pulled via cli", nil
	})
	defer restore()

	app := desktop.NewApp()
	message, err := app.Environment.PullEnvironment("sample-project")
	if err != nil {
		t.Fatalf("PullEnvironment failed: %v", err)
	}
	if !sameTestFile(t, gotDir, root) {
		t.Fatalf("expected govard dir %q, got %q", root, gotDir)
	}
	if !reflect.DeepEqual(gotArgs, []string{"env", "pull"}) {
		t.Fatalf("unexpected govard args: %#v", gotArgs)
	}
	if !strings.Contains(message, "env pulled via cli") {
		t.Fatalf("unexpected message: %q", message)
	}
}

func TestDesktopEnvironmentActionsRefuseDuringSyncForTest(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	called := false
	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		called = true
		return "should not run", nil
	})
	defer restore()

	desktop.RegisterSyncingProject("sample-project", "staging")
	defer desktop.UnregisterSyncingProject("sample-project")

	app := desktop.NewApp()
	actions := map[string]func(string) (string, error){
		"start":   app.Environment.StartEnvironment,
		"stop":    app.Environment.StopEnvironment,
		"restart": app.Environment.RestartEnvironment,
		"pull":    app.Environment.PullEnvironment,
		"delete":  app.Environment.DeleteProject,
		"toggle":  app.Environment.ToggleEnvironment,
	}
	for name, action := range actions {
		if _, err := action("sample-project"); err == nil {
			t.Fatalf("expected %s to refuse during sync", name)
		} else if !strings.Contains(err.Error(), "staging") || !strings.Contains(strings.ToLower(err.Error()), "sync") {
			t.Fatalf("expected %s error to name the sync remote, got: %v", name, err)
		}
	}
	if called {
		t.Fatal("expected no CLI command to run during sync")
	}
}

func TestDesktopRestartEnvironmentNamesFallbackForTest(t *testing.T) {
	desktop.ResetStateForTest()

	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
		if len(args) >= 2 && args[0] == "env" && args[1] == "restart" {
			return "", fmt.Errorf("restart not supported here")
		}
		return "up ran", nil
	})
	defer restore()

	app := desktop.NewApp()
	message, err := app.Environment.RestartEnvironment("sample-project")
	if err != nil {
		t.Fatalf("RestartEnvironment fallback failed: %v", err)
	}
	if !strings.Contains(strings.ToLower(message), "fallback") {
		t.Fatalf("expected fallback restart to say so, got %q", message)
	}
}

func TestDesktopComposeProjectNameForTogglePrefersRegistryForTest(t *testing.T) {
	root := t.TempDir()
	writeDesktopProjectConfigForTest(t, root, "sample-project", "laravel", "sample-project.test")
	registerDesktopProjectForTest(t, root, "sample-project", "sample-project.test", "laravel")

	if got := desktop.ComposeProjectNameForToggleForTest("sample-project.test"); got != "sample-project" {
		t.Fatalf("expected registry project name, got %q", got)
	}
	if got := desktop.ComposeProjectNameForToggleForTest("sample-project"); got != "sample-project" {
		t.Fatalf("expected registry project name, got %q", got)
	}
	if got := desktop.ComposeProjectNameForToggleForTest("unknown"); got != "unknown" {
		t.Fatalf("expected fallback to input, got %q", got)
	}
}
