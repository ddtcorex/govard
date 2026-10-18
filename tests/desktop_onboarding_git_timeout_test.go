package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/desktop"
)

// A hung git remote must not wedge onboarding: the default validators kill
// the subprocess past their ceiling and report a timeout.
func TestDesktopPkgDefaultGitValidatorsTimeOutOnHungRemote(t *testing.T) {
	binDir := t.TempDir()
	gitShim := "#!/bin/sh\nsleep 10\n"
	if err := os.WriteFile(filepath.Join(binDir, "git"), []byte(gitShim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	restore := desktop.SetDesktopGitTimeoutsForTest(300*time.Millisecond, 300*time.Millisecond)
	defer restore()

	started := time.Now()
	err := desktop.DefaultValidateGitConnectionForTest("https", "https://example.test/repo.git")
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("expected a timeout error from a hung git ls-remote, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("expected a timeout error, got: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("hung git ls-remote was not killed: took %v", elapsed)
	}

	dest := t.TempDir()
	started = time.Now()
	err = desktop.DefaultCloneGitRepoForTest("https://example.test/repo.git", dest)
	elapsed = time.Since(started)
	if err == nil {
		t.Fatal("expected a timeout error from a hung git clone, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "timed out") {
		t.Fatalf("expected a timeout error, got: %v", err)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("hung git clone was not killed: took %v", elapsed)
	}
}
