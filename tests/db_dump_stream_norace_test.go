//go:build !race

package tests

import (
	"os"
	"path/filepath"
	"testing"
)

// This file is excluded from -race builds on purpose. `db import --stream-db`
// starts a pterm spinner whose goroutine reads IsActive while Stop writes it, an
// upstream race in pterm that any in-process run of the command trips (the
// unit suite runs with -race). The test still runs in every non-race build.

// The --stream-db intermediate file holds the same data as a dump, so it gets
// the same owner-only mode, including when it overwrites a broader file.
func TestStreamDBDumpFileIsCreated0600(t *testing.T) {
	tempDir := t.TempDir()
	chdirForTest(t, tempDir)
	writeProtectedRemoteProject(t, tempDir)

	logPath := filepath.Join(t.TempDir(), "shim.log")
	installLoggingShim(t, "docker", dockerResetShim, logPath)
	installLoggingShim(t, "ssh", "#!/bin/sh\nprintf 'CREATE TABLE demo (id INT);\\n'\n", logPath)

	streamPath := filepath.Join(tempDir, "stream.sql")
	if err := os.WriteFile(streamPath, []byte("stale previous content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(streamPath, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runDBCommandForTest(t, "import", "--stream-db", "--environment", "prod", "--file", streamPath); err != nil {
		t.Fatalf("stream-db import failed: %v", err)
	}
	assertMode0600(t, streamPath)
}
