package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestLockCheckMissingFileMessage(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "govard.lock")

	_, readErr := engine.ReadLockFile(lockPath)
	if readErr == nil {
		t.Fatal("ReadLockFile succeeded on a missing lock file, the test fixture is wrong")
	}

	err := describeLockReadError(lockPath, readErr)
	want := fmt.Sprintf("no govard.lock in %s; run 'govard lock generate' to create one", dir)
	if err == nil || err.Error() != want {
		t.Fatalf("missing lock file message = %v, want %q", err, want)
	}

	other := errors.New("permission denied")
	if got := describeLockReadError(lockPath, other); !errors.Is(got, other) {
		t.Fatalf("an unrelated read error was rewritten: %v", got)
	}
}

// TestLockCheckCommandUsesTheMessage pins the call site: the hint has to come out
// of the command, not only out of the helper.
func TestLockCheckCommandUsesTheMessage(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.WriteFile(filepath.Join(dir, ".govard.yml"), []byte("project_name: sample-project\nframework: custom\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	err := lockCheckCmd.RunE(lockCheckCmd, nil)
	if err == nil {
		t.Fatal("lock check succeeded on a project without a lock file")
	}
	want := fmt.Sprintf("no govard.lock in %s; run 'govard lock generate' to create one", dir)
	if err.Error() != want {
		t.Fatalf("lock check error = %q, want %q", err, want)
	}
}
