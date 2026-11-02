package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/try"
)

func TestCreateWorkspaceUsesFrameworkAndTimestamp(t *testing.T) {
	parent := t.TempDir()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	got, err := try.CreateWorkspace(parent, "mageos", now)
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if base := filepath.Base(got); base != "mageos-20261101-120000" {
		t.Fatalf("base name = %q, want %q", base, "mageos-20261101-120000")
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat workspace: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("workspace is not a directory")
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("workspace mode = %o, want 700", perm)
	}
}

func TestCreateWorkspaceSuffixesWhenTimestampCollides(t *testing.T) {
	parent := t.TempDir()
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	first, err := try.CreateWorkspace(parent, "mageos", now)
	if err != nil {
		t.Fatalf("first CreateWorkspace: %v", err)
	}
	second, err := try.CreateWorkspace(parent, "mageos", now)
	if err != nil {
		t.Fatalf("second CreateWorkspace: %v", err)
	}
	if first == second {
		t.Fatalf("second workspace reused the first path %q", first)
	}
	if base := filepath.Base(second); !strings.HasSuffix(base, "-2") {
		t.Fatalf("second base name = %q, want a -2 suffix", base)
	}
	for _, dir := range []string{first, second} {
		if _, err := os.Stat(dir); err != nil {
			t.Fatalf("workspace %q missing: %v", dir, err)
		}
	}
}

func TestPrepareDirRefusesNonEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "occupied"), []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	err := try.PrepareDir(dir)
	if err == nil {
		t.Fatalf("PrepareDir accepted a non-empty directory")
	}
	if !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), "not empty") {
		t.Fatalf("error = %v, want it to name %q and say it is not empty", err, dir)
	}
}

func TestPrepareDirCreatesMissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "workspace")

	if err := try.PrepareDir(path); err != nil {
		t.Fatalf("PrepareDir: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Fatalf("mode = %o, want 700", perm)
	}
}
