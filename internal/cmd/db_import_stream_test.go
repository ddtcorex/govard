package cmd

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func stubDumpStream(t *testing.T, fn func(*exec.Cmd, io.Writer, bool, io.Writer) error) {
	t.Helper()
	original := writeDumpStream
	writeDumpStream = fn
	t.Cleanup(func() { writeDumpStream = original })
}

func TestWriteDumpFileKeepsExistingFileOnFailure(t *testing.T) {
	target := filepath.Join(t.TempDir(), "yesterday.sql")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatalf("seed target: %v", err)
	}

	stubDumpStream(t, func(_ *exec.Cmd, writer io.Writer, _ bool, _ io.Writer) error {
		if _, err := io.WriteString(writer, "partial"); err != nil {
			return err
		}
		return errors.New("ssh: connection reset")
	})

	if err := writeDumpFile(&exec.Cmd{}, target, io.Discard); err == nil {
		t.Fatal("writeDumpFile succeeded although the dump failed, want an error")
	}

	kept, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read target: %v", readErr)
	}
	if string(kept) != "old" {
		t.Fatalf("target now holds %q, want the previous dump %q", kept, "old")
	}
	if _, statErr := os.Stat(target + ".partial"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial file survived a failed dump (stat error: %v)", statErr)
	}
}

func TestWriteDumpFileReplacesOnSuccess(t *testing.T) {
	target := filepath.Join(t.TempDir(), "today.sql")

	stubDumpStream(t, func(_ *exec.Cmd, writer io.Writer, _ bool, _ io.Writer) error {
		_, err := io.WriteString(writer, "new")
		return err
	})

	if err := writeDumpFile(&exec.Cmd{}, target, io.Discard); err != nil {
		t.Fatalf("writeDumpFile: %v", err)
	}

	written, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(written) != "new" {
		t.Fatalf("target holds %q, want %q", written, "new")
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("target mode is %v, want 0600", info.Mode().Perm())
	}
}

// TestStageDumpThenResetNeverResetsAfterAFailedDump pins the ordering rule the
// stream path depends on: a dump that fails must not be followed by a reset.
func TestStageDumpThenResetNeverResetsAfterAFailedDump(t *testing.T) {
	var order []string
	err := stageDumpThenReset(
		func() error { order = append(order, "dump"); return errors.New("ssh: connection reset") },
		func() error { order = append(order, "reset"); return nil },
	)
	if err == nil {
		t.Fatal("stageDumpThenReset reported success although the dump failed")
	}
	if len(order) != 1 || order[0] != "dump" {
		t.Fatalf("order is %v, want only the dump to have run", order)
	}
}

func TestStageDumpThenResetRunsResetAfterTheDump(t *testing.T) {
	var order []string
	if err := stageDumpThenReset(
		func() error { order = append(order, "dump"); return nil },
		func() error { order = append(order, "reset"); return nil },
	); err != nil {
		t.Fatalf("stageDumpThenReset: %v", err)
	}
	if len(order) != 2 || order[0] != "dump" || order[1] != "reset" {
		t.Fatalf("order is %v, want dump then reset", order)
	}
}
