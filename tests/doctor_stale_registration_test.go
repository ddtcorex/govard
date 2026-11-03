package tests

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func seedRegistry(t *testing.T, paths ...string) {
	t.Helper()

	registry := filepath.Join(t.TempDir(), "projects.json")
	t.Setenv(engine.ProjectRegistryPathEnvVar, registry)

	for _, path := range paths {
		entry := engine.ProjectRegistryEntry{
			Path:        path,
			ProjectName: filepath.Base(path),
			Framework:   "mageos",
		}
		if err := engine.UpsertProjectRegistryEntry(entry); err != nil {
			t.Fatalf("seed registry entry %q: %v", path, err)
		}
	}
}

func TestStaleProjectRegistryEntriesReportsMissingDirectories(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	live := t.TempDir()
	seedRegistry(t, missing, live)

	stale, err := engine.StaleProjectRegistryEntries()
	if err != nil {
		t.Fatalf("StaleProjectRegistryEntries: %v", err)
	}
	if len(stale) != 1 || stale[0] != missing {
		t.Fatalf("stale = %v, want exactly [%s]", stale, missing)
	}
}

func TestDoctorRegistryCheckNamesTheStalePath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "gone")
	seedRegistry(t, missing)

	err := engine.CheckProjectRegistryHealth()
	if err == nil {
		t.Fatalf("CheckProjectRegistryHealth returned nil for a missing project directory")
	}
	if !strings.Contains(err.Error(), missing) {
		t.Fatalf("error = %v, want it to name %s", err, missing)
	}
}

func TestDoctorStaleRegistryCheckIsAWarningAndSkipsCleanRegistries(t *testing.T) {
	check, ok := engine.DoctorStaleRegistryCheck(func() error { return errors.New("registry lists 1 project path(s) that no longer exist: /tmp/gone") })
	if !ok {
		t.Fatalf("DoctorStaleRegistryCheck reported nothing for a failing check")
	}
	if check.Status != engine.DoctorStatusWarn {
		t.Fatalf("status = %q, want warn", check.Status)
	}
	if !strings.Contains(check.Message, "/tmp/gone") {
		t.Fatalf("message = %q, want it to name the stale path", check.Message)
	}

	if _, ok := engine.DoctorStaleRegistryCheck(func() error { return nil }); ok {
		t.Fatalf("DoctorStaleRegistryCheck reported a check for a clean registry")
	}
}
