package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func writeFanoutProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	config := strings.TrimSpace(`
project_name: `+name+`
framework: laravel
domain: `+name+`.test
`) + "\n"
	if err := os.WriteFile(filepath.Join(root, ".govard.yml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// The registry fan-out builds one Environment per entry without any Docker
// dependency, so it is pinned directly: name, domain and framework come
// from the path config, and an env with no sync stays stopped.
func TestDesktopPkgBuildRegistryEnvironmentEntry(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	t.Setenv("HOME", t.TempDir())
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	root := writeFanoutProject(t, "fanoutdemo")
	env := desktop.BuildRegistryEnvironmentEntryForTest(engine.ProjectRegistryEntry{
		Path:        root,
		ProjectName: "fanoutdemo",
		Domain:      "fanoutdemo.test",
	}, "fanoutdemo")

	if env.Project != "fanoutdemo" {
		t.Fatalf("env.Project = %q, want %q", env.Project, "fanoutdemo")
	}
	if env.Domain != "fanoutdemo.test" {
		t.Fatalf("env.Domain = %q, want %q", env.Domain, "fanoutdemo.test")
	}
	if env.Status != "stopped" {
		t.Fatalf("env.Status = %q, want stopped", env.Status)
	}
}

// The fan-out is semaphore-bounded: with a limit of one, six entries must
// still all complete (a lost release would starve and hang here). Docker is
// pointed at a dead socket so the dashboard runs registry-only.
func TestDesktopPkgDashboardRegistryFanoutBounded(t *testing.T) {
	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("DOCKER_HOST", "unix:///nonexistent-govard-test.sock")
	desktop.ResetStateForTest()
	desktop.ResetSyncingProjectsForTest()

	names := []string{"fanA", "fanB", "fanC", "fanD", "fanE", "fanF"}
	for _, name := range names {
		root := writeFanoutProject(t, name)
		if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
			Path:        root,
			ProjectName: name,
			Domain:      name + ".test",
		}); err != nil {
			t.Fatal(err)
		}
	}

	restore := desktop.SetRegistryFanoutLimitForTest(1)
	defer restore()

	app := desktop.NewApp()
	dashboard, err := app.Environment.GetDashboard()
	if err != nil {
		t.Fatalf("GetDashboard failed: %v", err)
	}
	found := map[string]bool{}
	for _, env := range dashboard.Environments {
		found[env.Project] = true
	}
	for _, name := range names {
		if !found[name] {
			t.Fatalf("registry project %q missing from dashboard", name)
		}
	}
}

// One corrupt entry must not take the dashboard down: the fan-out runs on
// worker goroutines where GetDashboard's RecoverPanic cannot reach, so the
// entry renderer converts panics into a skipped environment plus a warning.
func TestDesktopPkgBuildRegistryEnvironmentEntrySafeRecovers(t *testing.T) {
	restore := desktop.SetBuildRegistryEnvironmentEntryForTest(func(engine.ProjectRegistryEntry, string) desktop.Environment {
		panic("simulated corrupt entry")
	})
	defer restore()

	env, warning := desktop.BuildRegistryEnvironmentEntrySafeForTest(engine.ProjectRegistryEntry{Path: "nowhere"}, "broken")
	if warning == "" {
		t.Fatal("expected a warning for the recovered panic, got none")
	}
	if env.Project != "" {
		t.Fatalf("expected the corrupt entry to be skipped, got %+v", env)
	}
}
