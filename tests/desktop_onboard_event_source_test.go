package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/desktop"
	"govard/internal/engine"
)

func readOnboardSource(t *testing.T) string {
	t.Helper()
	events, err := engine.ReadOperationEvents(50)
	if err != nil {
		t.Fatalf("read operation events: %v", err)
	}
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Operation == "desktop.project.onboard" {
			return events[i].Source
		}
	}
	t.Fatal("no desktop.project.onboard event recorded")
	return ""
}

// The onboard event's source names where the project came from: empty for a
// local directory, the repo URL for a clone. The project path is already the
// event's project field — repeating it as source mislabels every onboard.
func TestDesktopPkgOnboardEventSource(t *testing.T) {
	t.Run("local directory leaves source empty", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))
		t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))
		t.Setenv("HOME", t.TempDir())

		restore := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
			content := "project_name: srcdemo\nframework: laravel\ndomain: srcdemo.test\n"
			if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(content), 0o644); err != nil {
				return "", err
			}
			return "ok", nil
		})
		defer restore()

		if _, err := desktop.OnboardProjectForPathForTest(root, "laravel"); err != nil {
			t.Fatalf("onboard failed: %v", err)
		}
		if source := readOnboardSource(t); source != "" {
			t.Fatalf("local onboard source = %q, want empty", source)
		}
	})

	t.Run("git clone records the repo URL", func(t *testing.T) {
		root := t.TempDir()
		t.Setenv(engine.ProjectRegistryPathEnvVar, filepath.Join(t.TempDir(), "projects.json"))
		t.Setenv(engine.OperationsLogPathEnvVar, filepath.Join(t.TempDir(), "operations.log"))
		t.Setenv("HOME", t.TempDir())

		restoreValidate := desktop.SetValidateGitConnectionForDesktopForTest(func(protocol string, repoURL string) error {
			return nil
		})
		defer restoreValidate()
		restoreClone := desktop.SetCloneGitRepoForDesktopForTest(func(repoURL string, destination string) error {
			return nil
		})
		defer restoreClone()
		restoreInit := desktop.SetRunGovardCommandForDesktopForTest(func(dir string, args []string) (string, error) {
			content := "project_name: gitsrc\nframework: laravel\ndomain: gitsrc.test\n"
			if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte(content), 0o644); err != nil {
				return "", err
			}
			return "ok", nil
		})
		defer restoreInit()

		const repoURL = "https://example.com/acme/shop.git"
		if _, err := desktop.OnboardProjectFromGitForPathForTest(root, "laravel", "https", repoURL); err != nil {
			t.Fatalf("git onboard failed: %v", err)
		}
		if source := readOnboardSource(t); source != repoURL {
			t.Fatalf("git onboard source = %q, want %q", source, repoURL)
		}
	})
}
