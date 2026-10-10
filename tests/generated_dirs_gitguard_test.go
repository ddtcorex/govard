package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/engine"
)

func isolatedGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Dir = root
	c.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		// core.excludesFile defaults to ~/.config/git/ignore by path, which
		// GIT_CONFIG_GLOBAL does not neutralize: pin it so a developer's
		// global excludes cannot decide what these tests prove.
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.excludesFile", "GIT_CONFIG_VALUE_0=/dev/null")
	out, err := c.CombinedOutput()
	if err != nil {
		t.Skipf("git %v unavailable: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestSnapshotCreateStagesNothingInGit(t *testing.T) {
	shimDir := t.TempDir()
	installDBCommandRuntimeDockerShim(t, shimDir)
	t.Setenv("DB_COMMAND_RUNTIME_LOG", filepath.Join(shimDir, "docker.log"))
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	projectDir := t.TempDir()
	isolatedGit(t, projectDir, "init", "-q")
	if _, err := engine.CreateSnapshot(projectDir, engine.Config{ProjectName: "sample-project"}, "snap"); err != nil {
		t.Fatalf("CreateSnapshot: %v", err)
	}
	isolatedGit(t, projectDir, "add", "-A")
	if got := isolatedGit(t, projectDir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("snapshot files were staged: %q", got)
	}
}

func TestSnapshotRootRefusesSymlinkedDirectory(t *testing.T) {
	projectDir := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".govard"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, engine.SnapshotRoot(projectDir)); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureSnapshotRoot(projectDir); err == nil {
		t.Fatal("expected a symlinked snapshots directory to be refused")
	}
	if _, err := engine.CreateSnapshot(projectDir, engine.Config{ProjectName: "p"}, "snap"); err == nil {
		t.Fatal("expected CreateSnapshot to refuse a symlinked snapshots directory")
	}
}

// A .gitignore the operator wrote in a governard-owned directory is kept, but it
// must not leave the directory exposed: when it does not ignore everything, the
// ignore-all rule is appended and nothing is staged.
func TestSnapshotRootKeepsAUserGitignoreAndStillIgnoresEverything(t *testing.T) {
	projectDir := t.TempDir()
	isolatedGit(t, projectDir, "init", "-q")
	root := engine.SnapshotRoot(projectDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureSnapshotRoot(projectDir); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.HasPrefix(string(data), "mine\n") {
		t.Fatalf("the user's rules were lost: %q", data)
	}
	if err := os.WriteFile(filepath.Join(root, "dump.sql"), []byte("customer data"), 0o600); err != nil {
		t.Fatal(err)
	}
	isolatedGit(t, projectDir, "add", "-A")
	if got := isolatedGit(t, projectDir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("a partial user .gitignore left the dump stageable: %q", got)
	}
}

// A trailing un-ignore of everything is the same exposure.
func TestSnapshotRootRepairsAGitignoreThatUnignoresEverything(t *testing.T) {
	projectDir := t.TempDir()
	isolatedGit(t, projectDir, "init", "-q")
	root := engine.SnapshotRoot(projectDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("*\n!*\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureSnapshotRoot(projectDir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dump.sql"), []byte("customer data"), 0o600); err != nil {
		t.Fatal(err)
	}
	isolatedGit(t, projectDir, "add", "-A")
	if got := isolatedGit(t, projectDir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("a .gitignore ending in !* left the dump stageable: %q", got)
	}
}

// A symlinked .gitignore (it could point anywhere, or at a file that ignores
// nothing) is refused rather than trusted, and nothing is written through it.
func TestSnapshotRootRefusesASymlinkedGitignore(t *testing.T) {
	projectDir := t.TempDir()
	root := engine.SnapshotRoot(projectDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(outside, []byte("untouched\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".gitignore")); err != nil {
		t.Fatal(err)
	}
	if err := engine.EnsureSnapshotRoot(projectDir); err == nil {
		t.Fatal("a symlinked .gitignore must be refused")
	}
	if b, _ := os.ReadFile(outside); string(b) != "untouched\n" {
		t.Fatalf("the symlink target was written: %q", b)
	}
}

func TestDoctorPackDefaultDirIsSelfIgnoring(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	isolatedGit(t, home, "init", "-q")
	path, err := cmd.CreateDoctorDiagnosticsPack("", t.TempDir(), engine.DoctorReport{})
	if err != nil {
		t.Fatalf("CreateDoctorDiagnosticsPack: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	isolatedGit(t, home, "add", "-A")
	if got := isolatedGit(t, home, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("diagnostics were staged: %q", got)
	}
}

func TestProjectExtensionsDirIsNotIgnoredBySnapshots(t *testing.T) {
	projectDir := t.TempDir()
	isolatedGit(t, projectDir, "init", "-q")
	if err := engine.EnsureSnapshotRoot(projectDir); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(projectDir, ".govard", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre_up.sh"), []byte("echo"), 0o755); err != nil {
		t.Fatal(err)
	}
	isolatedGit(t, projectDir, "add", "-A")
	if got := isolatedGit(t, projectDir, "diff", "--cached", "--name-only"); got != ".govard/hooks/pre_up.sh" {
		t.Fatalf("extension dir must stay committable, staged: %q", got)
	}
}
