package tests

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	// A developer's global excludes must not decide what this test proves.
	command.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func stagedUnderSandbox(t *testing.T, root string) []string {
	t.Helper()
	gitIn(t, root, "add", "-A")
	var staged []string
	for _, line := range strings.Split(gitIn(t, root, "diff", "--cached", "--name-only"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), ".govard/sandbox/") {
			staged = append(staged, line)
		}
	}
	return staged
}

// The private key used to land in the project's history on `git add -A`: the
// code claimed the directory was gitignored and nothing ignored it.
func TestSandboxStateIsNeverStagedByGitAddAll(t *testing.T) {
	root := sandboxProject(t)
	fake := sandboxFake()
	fake.answers["image inspect"] = "sha256:abc\n"
	if _, err := deploy.SandboxUp(context.Background(), deploy.NewDockerCLIForTest(fake.run), deploy.LocalRunner{}, deploy.SandboxRequest{
		ProjectRoot: root,
		ProjectName: "sample-project",
		Profile:     deploy.SandboxProfilePHP,
		Probe:       func(context.Context, string, int, time.Duration) error { return nil },
	}); err != nil {
		t.Fatalf("up: %v", err)
	}

	keyPath := filepath.Join(deploy.SandboxStateDir(root), deploy.SandboxKeyName)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("the sandbox key must exist after up: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("the private key mode = %v, want 0600", info.Mode().Perm())
	}
	if staged := stagedUnderSandbox(t, root); len(staged) != 0 {
		t.Fatalf("git add -A staged sandbox state: %v", staged)
	}
	// The guard is the state directory's own file, so no project .gitignore is touched.
	if _, err := os.Stat(filepath.Join(root, ".gitignore")); err == nil {
		t.Error("the project's own .gitignore must not be created or edited")
	}
}

// Positive control: without the guard the same `git add -A` does stage the key,
// so the test above can fail.
func TestSandboxStateIsStagedWithoutTheGuard(t *testing.T) {
	root := sandboxProject(t)
	if _, err := deploy.EnsureSandboxKey(deploy.SandboxStateDir(root)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(deploy.SandboxStateDir(root), ".gitignore")); err != nil {
		t.Fatalf("EnsureSandboxKey must write the guard: %v", err)
	}
	staged := strings.Join(stagedUnderSandbox(t, root), " ")
	if !strings.Contains(staged, deploy.SandboxKeyName) {
		t.Fatalf("control: without the guard git add -A must stage the key, got %q", staged)
	}
}

func TestSandboxStateGuardNeverClobbersAUserFile(t *testing.T) {
	root := sandboxProject(t)
	dir := deploy.SandboxStateDir(root)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(custom, []byte("*\n!keep.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.EnsureSandboxKey(dir); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(custom)
	if string(got) != "*\n!keep.txt\n" {
		t.Fatalf("an existing .gitignore must be left alone, got %q", got)
	}
}

// A second run, and a state directory that predates the guard, both end up guarded.
func TestSandboxStateGuardIsIdempotentAndRepairsAnOldDirectory(t *testing.T) {
	root := sandboxProject(t)
	dir := deploy.SandboxStateDir(root)
	if _, err := deploy.EnsureSandboxKey(dir); err != nil {
		t.Fatal(err)
	}
	guard := filepath.Join(dir, ".gitignore")
	if err := os.Remove(guard); err != nil {
		t.Fatal(err)
	}
	// The key exists now, as it does in a directory created before the guard.
	if _, err := deploy.EnsureSandboxKey(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.EnsureSandboxKey(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(guard)
	if err != nil || string(got) != "*\n" {
		t.Fatalf("guard = %q, %v; want \"*\\n\"", got, err)
	}
}
