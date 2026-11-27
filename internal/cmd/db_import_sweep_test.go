package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A staged stream dump holds a full database. A normal exit and an error return
// remove it, but a process that dies without running defers (SIGHUP, SIGKILL,
// power loss) leaves it, and the file is randomly named so no later run reuses or
// overwrites it. The next stream import sweeps the ones old enough to be orphans.
func TestSweepStaleStreamDumpsRemovesOnlyOldStagedDumps(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	touch := func(name string, modified time.Time) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("dump"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	touch("stream-db-111.sql", old)
	touch("stream-db-222.sql.partial", old)
	touch("stream-db-333.sql", time.Now())
	touch("notes.sql", old)
	touch("stream-db-444.txt", old)

	sweepStaleStreamDumps(dir, 24*time.Hour)

	for _, gone := range []string{"stream-db-111.sql", "stream-db-222.sql.partial"} {
		if _, err := os.Stat(filepath.Join(dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s is an orphaned staged dump and should have been removed (%v)", gone, err)
		}
	}
	for _, kept := range []string{"stream-db-333.sql", "notes.sql", "stream-db-444.txt"} {
		if _, err := os.Stat(filepath.Join(dir, kept)); err != nil {
			t.Errorf("%s must not be touched: %v", kept, err)
		}
	}
}

func TestSweepStaleStreamDumpsToleratesAMissingDirectory(t *testing.T) {
	sweepStaleStreamDumps(filepath.Join(t.TempDir(), "does-not-exist"), time.Hour)
}
