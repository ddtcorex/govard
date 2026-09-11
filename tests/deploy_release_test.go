package tests

import (
	"context"
	"encoding/json"
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

// Spec 6's example record carries a `ci` block. The field did not exist at all,
// so "which pipeline deployed this" was unanswerable from the record.
func TestReleaseRecordCarriesTheCIRun(t *testing.T) {
	t.Setenv("CI_PIPELINE_ID", "12345")
	t.Setenv("CI_JOB_NAME", "deploy-production")
	t.Setenv("GITHUB_RUN_ID", "")
	t.Setenv("GITHUB_JOB", "")

	release := deploy.NewReleaseForTest("", "abc", "main")
	if release.CI == nil || release.CI.Pipeline != "12345" || release.CI.Job != "deploy-production" {
		t.Fatalf("ci = %+v, want the GitLab pipeline and job", release.CI)
	}
	encoded, err := json.Marshal(release)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(encoded), `"ci":{"pipeline":"12345","job":"deploy-production"}`) {
		t.Fatalf("the record does not carry the ci block: %s", encoded)
	}

	// A local run is not a CI run, and the block is omitted rather than empty.
	t.Setenv("CI_PIPELINE_ID", "")
	t.Setenv("CI_JOB_NAME", "")
	local := deploy.NewReleaseForTest("", "abc", "main")
	if local.CI != nil || strings.Contains(mustJSON(t, local), `"ci"`) {
		t.Fatalf("a non-CI run must not claim a pipeline: %+v", local.CI)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(encoded)
}
