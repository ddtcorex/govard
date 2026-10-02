//go:build integration
// +build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
)

// `db import --stream-db --file` writes the remote dump to an intermediate file
// before importing it. It runs out of process here because the command starts a
// pterm spinner whose goroutine races with Stop (an upstream data race), which
// the in-process unit suite cannot run under -race.
func TestStreamDBImportFileIsOwnerOnly(t *testing.T) {
	env := NewTestEnvironment(t)

	for name, preexisting := range map[string]bool{
		"NewFile":               false,
		"OverwriteOfAn0644File": true,
	} {
		preexisting := preexisting
		t.Run(name, func(t *testing.T) {
			projectDir := env.CreateProjectFromFixture(t, "magento2/options-local", "db-stream-mode-"+name)
			streamPath := filepath.Join(projectDir, "stream.sql")
			if preexisting {
				if err := os.WriteFile(streamPath, []byte("stale previous content\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(streamPath, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			shim := env.SetupRuntimeShims(t, map[string]int{"docker": 0, "ssh": 0, "rsync": 0})
			result := env.RunGovardWithEnv(t, projectDir, shim.Env(), "db", "import", "--stream-db", "--environment", "dev", "--file", streamPath)
			result.AssertSuccess(t)

			info, err := os.Stat(streamPath)
			if err != nil {
				t.Fatalf("stream dump file was not written: %v", err)
			}
			if got := info.Mode().Perm(); got != 0o600 {
				t.Fatalf("stream dump file mode = %o, want 600", got)
			}
			data, err := os.ReadFile(streamPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) == "stale previous content\n" {
				t.Fatal("the pre-existing file was not truncated")
			}
		})
	}
}
