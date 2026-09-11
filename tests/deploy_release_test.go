package tests

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestReleaseRecordRoundTripsAtomicallyOnALocalHost(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	record := deploy.NewReleaseForTest("7", "abc123", "staging")
	record.Status = deploy.StatusOK
	if err := deploy.WriteRelease(context.Background(), host, record); err != nil {
		t.Fatalf("write release: %v", err)
	}

	// No temp file may survive: the record is written to a temp path and moved.
	if _, err := os.Stat(host.ReleaseRecordPath("7") + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp record left behind: %v", err)
	}

	read, err := deploy.ReadRelease(context.Background(), host, "7")
	if err != nil {
		t.Fatalf("read release: %v", err)
	}
	if read.Revision != "abc123" || read.Status != deploy.StatusOK || read.Tool != deploy.ReleaseTool {
		t.Fatalf("round trip = %+v, want revision abc123 status ok tool govard", read)
	}
	if read.Path != host.ReleasePath("7") {
		t.Fatalf("record path = %q, want %q", read.Path, host.ReleasePath("7"))
	}
}

func TestHistoryAppendsOneLinePerDeploy(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	for _, revision := range []string{"aaa", "bbb"} {
		record := deploy.NewReleaseForTest("1", revision, "staging")
		if err := deploy.AppendHistory(context.Background(), host, record); err != nil {
			t.Fatalf("append history: %v", err)
		}
	}
	content, err := os.ReadFile(filepath.Join(host.DepPath(), "history.jsonl"))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}
	if lines := strings.Count(strings.TrimSpace(string(content)), "\n") + 1; lines != 2 {
		t.Fatalf("history has %d lines, want 2", lines)
	}
}

func TestHostPathsAndShellQuotingForATildePath(t *testing.T) {
	host := deploy.HostForTest("~/.deployer", deploy.LocalRunner{})
	if host.LockPath() != "~/.deployer/.dep/govard.lock" {
		t.Fatalf("lock path = %q", host.LockPath())
	}
	if got := deploy.Shell(host.LockPath()); !strings.HasPrefix(got, "$HOME/") {
		t.Fatalf("Shell(%q) = %q, want a $HOME/ prefix so the remote shell expands it", host.LockPath(), got)
	}
}
