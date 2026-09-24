package tests

import (
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
