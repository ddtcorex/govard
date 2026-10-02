package tests

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

// A dump target that is not a regular file (/dev/null, a FIFO, a device) has no
// mode to restrict, and chmod on it fails for a non-owner. That must not turn
// `db dump --file /dev/null` into an error, and chmod must not even be tried.
func TestCreatePrivateDumpFileSkipsChmodForNonRegularTargets(t *testing.T) {
	chmodCalls := 0
	restore := engine.SetPrivateDumpHooksForTest(
		func(*os.File, os.FileMode) error {
			chmodCalls++
			return errors.New("operation not permitted")
		}, &bytes.Buffer{})
	defer restore()

	file, err := engine.CreatePrivateDumpFile(os.DevNull)
	if err != nil {
		t.Fatalf("a non-regular target must not fail: %v", err)
	}
	_ = file.Close()
	if chmodCalls != 0 {
		t.Fatalf("chmod must not be attempted on a non-regular file, got %d calls", chmodCalls)
	}
}

// A pre-existing broader regular file is still tightened to 0600.
func TestCreatePrivateDumpFileStillTightensRegularFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dump.sql")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := engine.CreatePrivateDumpFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	assertMode0600(t, path)
}

// On a filesystem that ignores chmod the tightening cannot be enforced: the
// dump still proceeds, and the operator is told on stderr which path stayed
// broad.
func TestCreatePrivateDumpFileWarnsWhenChmodFailsOnARegularFile(t *testing.T) {
	warned := &bytes.Buffer{}
	restore := engine.SetPrivateDumpHooksForTest(
		func(*os.File, os.FileMode) error { return errors.New("operation not permitted") }, warned)
	defer restore()

	path := filepath.Join(t.TempDir(), "dump.sql")
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := engine.CreatePrivateDumpFile(path)
	if err != nil {
		t.Fatalf("a failed chmod must not fail the dump: %v", err)
	}
	_ = file.Close()
	out := warned.String()
	if !strings.Contains(out, path) || !strings.Contains(out, "could not be restricted") {
		t.Fatalf("the warning must name the path and say permissions could not be restricted, got %q", out)
	}
}

// The wiring: `db dump --file /dev/null` succeeds end to end.
func TestDBDumpToDevNullSucceeds(t *testing.T) {
	driveLocalDump(t, os.DevNull)
}
