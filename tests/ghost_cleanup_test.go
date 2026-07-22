package tests

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"govard/internal/engine"
)

func TestGhostProjectCleanup(t *testing.T) {
	// A ghost project: a directory with no running containers and no
	// registry entry (GOVARD_HOME_DIR is isolated per TestMain). Exercises
	// DeleteProject's resilience - it must still succeed when there's
	// nothing to clean up, rather than failing because the project isn't
	// "really" there.
	projectPath := t.TempDir()
	projectName := filepath.Base(projectPath)

	err := engine.DeleteProject(context.Background(), projectPath, io.Discard, io.Discard)
	if err != nil {
		t.Errorf("Resilient cleanup failed: %v", err)
	}

	// Verify registry removal (indirectly by trying to find it)
	if _, _, err := engine.FindProjectByQuery(projectName); err == nil {
		t.Errorf("project %q still exists in registry after deletion", projectName)
	}
}
