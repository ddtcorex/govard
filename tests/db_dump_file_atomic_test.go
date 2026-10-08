package tests

import (
	"compress/gzip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
)

func TestDumpFileFailureLeavesNoStub(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "var", "dump.sql.gz")
	err := cmd.WriteDumpFileForTest(exec.Command("sh", "-c", "echo partial; exit 3"), target)
	if err == nil {
		t.Fatal("expected the failing dump to return an error")
	}
	entries, _ := os.ReadDir(filepath.Join(dir, "var"))
	if len(entries) != 0 {
		names := []string{}
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("failed dump must leave no file behind, found %v", names)
	}
}

func TestDumpFileFailureKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dump.sql")
	if err := os.WriteFile(target, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.WriteDumpFileForTest(exec.Command("sh", "-c", "exit 1"), target); err == nil {
		t.Fatal("expected error")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "previous" {
		t.Fatalf("previous dump must survive a failed run, got %q", got)
	}
}

func TestDumpFileSuccessIsRenamedIntoPlaceAndValid(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dump.sql.gz")
	if err := cmd.WriteDumpFileForTest(exec.Command("sh", "-c", "echo 'SELECT 1;'"), target); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	f, err := os.Open(target)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("gzip header: %v", err)
	}
	body, err := io.ReadAll(zr)
	if err != nil || !strings.Contains(string(body), "SELECT 1;") {
		t.Fatalf("gzip body = %q, err %v", body, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected only the final file, got %d entries", len(entries))
	}
}

func TestRemoteDumpSuccessMessageSaysWhereTheFileIs(t *testing.T) {
	msg := cmd.RemoteDumpSuccessMessageForTest("sandbox", "/tmp/r.sql.gz")
	for _, want := range []string{"written on sandbox: /tmp/r.sql.gz", "not on this machine", "--local"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q must mention %q", msg, want)
		}
	}
}

func TestSnapshotRestoreConfirmation(t *testing.T) {
	if err := cmd.ConfirmSnapshotRestoreForTest(true, false, false, "snap"); err != nil {
		t.Fatalf("--yes must skip the prompt: %v", err)
	}
	err := cmd.ConfirmSnapshotRestoreForTest(false, false, false, "snap")
	if err == nil || !strings.Contains(err.Error(), "-y") {
		t.Fatalf("non-interactive restore without --yes must refuse and name -y, got %v", err)
	}
	if err := cmd.ConfirmSnapshotRestoreForTest(false, true, true, "snap"); err != nil {
		t.Fatalf("interactive confirmation accepted: %v", err)
	}
	if err := cmd.ConfirmSnapshotRestoreForTest(false, true, false, "snap"); err == nil {
		t.Fatal("interactive decline must cancel the restore")
	}
}
