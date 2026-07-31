package tests

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/engine"
)

// Reproduces the reported bug: after `govard config profile switch <name>`, running an
// unrelated command that doesn't take an explicit --profile flag (e.g. `govard lock
// generate`, `govard sync`, `govard db dump`) silently resets the active profile back
// to empty in both the project registry and govard.lock, so the next `govard env up`
// no longer picks up the previously selected profile.
func TestLockGenerateDoesNotResetActiveProfile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: magento2
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.upgrade.yml"), []byte(`profile: upgrade
stack:
  php_version: "8.2"
`), 0o644); err != nil {
		t.Fatal(err)
	}

	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	// Simulate a prior `govard config profile switch upgrade`.
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:    tempDir,
		Profile: "upgrade",
	}); err != nil {
		t.Fatal(err)
	}

	restore := cmd.SetLockDependenciesForTest(engine.LockDependencies{
		ReadDockerVersion:        func() (string, error) { return "27.2.1", nil },
		ReadDockerComposeVersion: func() (string, error) { return "2.29.7", nil },
		ReadServiceImages:        func(composePath string) (map[string]string, error) { return map[string]string{}, nil },
		Now:                      func() time.Time { return time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC) },
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	// Run an unrelated command that never passes --profile.
	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"lock", "generate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute lock generate: %v", err)
	}

	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.Profile != "upgrade" {
		t.Fatalf("lock generate reset the active profile in the registry: expected %q, got %q", "upgrade", entry.Profile)
	}

	lock, err := engine.ReadLockFile(filepath.Join(tempDir, "govard.lock"))
	if err != nil {
		t.Fatalf("read generated lockfile: %v", err)
	}
	if lock.Project.Profile != "upgrade" {
		t.Fatalf("lock generate wrote wrong profile into govard.lock: expected %q, got %q", "upgrade", lock.Project.Profile)
	}
}

// Reproduces a related bug: any command that records registry activity (sync, lock,
// db, bootstrap, remote, tunnel) wipes previous_profile because trackProjectRegistry
// builds a brand-new registry entry without carrying it forward. previous_profile is
// what lets `govard env up` detect a profile shift and warn about it (e.g. Redis RDB
// version incompatibilities); silently losing it right after a profile switch defeats
// that safety net.
func TestLockGenerateDoesNotResetPreviousProfile(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, ".govard.yml"), []byte(`project_name: demo
domain: demo.test
framework: magento2
`), 0o644); err != nil {
		t.Fatal(err)
	}

	registryPath := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registryPath)

	// Simulate the state right after `govard config profile switch upgrade`:
	// profile is now "upgrade", previous_profile records what it shifted from.
	if err := engine.UpsertProjectRegistryEntry(engine.ProjectRegistryEntry{
		Path:            tempDir,
		Profile:         "upgrade",
		PreviousProfile: "staging",
	}); err != nil {
		t.Fatal(err)
	}

	restore := cmd.SetLockDependenciesForTest(engine.LockDependencies{
		ReadDockerVersion:        func() (string, error) { return "27.2.1", nil },
		ReadDockerComposeVersion: func() (string, error) { return "2.29.7", nil },
		ReadServiceImages:        func(composePath string) (map[string]string, error) { return map[string]string{}, nil },
		Now:                      func() time.Time { return time.Date(2026, 2, 20, 12, 0, 0, 0, time.UTC) },
	})
	defer restore()

	cwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}

	root := cmd.RootCommandForTest()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"lock", "generate"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute lock generate: %v", err)
	}

	entry, ok := engine.GetProjectRegistryEntry(tempDir)
	if !ok {
		t.Fatal("expected project registry entry")
	}
	if entry.PreviousProfile != "staging" {
		t.Fatalf("lock generate reset previous_profile: expected %q, got %q", "staging", entry.PreviousProfile)
	}
}
