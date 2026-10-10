package gitguard_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/gitguard"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// core.excludesFile defaults to ~/.config/git/ignore by path, which
	// GIT_CONFIG_GLOBAL does not neutralize: pin it so a developer's global
	// excludes cannot decide what these tests prove.
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.excludesFile", "GIT_CONFIG_VALUE_0=/dev/null")
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = root
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git unavailable: %v %s", err, out)
	}
	return root
}

func staged(t *testing.T, root string) string {
	t.Helper()
	env := append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=core.excludesFile", "GIT_CONFIG_VALUE_0=/dev/null")
	for _, args := range [][]string{{"add", "-A"}, {"diff", "--cached", "--name-only"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		if args[0] == "diff" {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func TestEnsureDirStagesNothing(t *testing.T) {
	root := gitRepo(t)
	dir := filepath.Join(root, ".govard", "snapshots")
	if err := gitguard.EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "snap1"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "snap1", "db.sql.gz"), []byte("customer data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := staged(t, root); got != "" {
		t.Fatalf("git add -A staged guarded files: %q", got)
	}
}

func TestEnsureDirKeepsUserRulesAndAppendsIgnoreAll(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "d")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	guard := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(guard, []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitguard.EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(guard)
	if string(data) != "mine\n*\n" {
		t.Fatalf("want the user's rule kept and ignore-all appended, got %q", data)
	}
	// Idempotent: a second call changes nothing.
	if err := gitguard.EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if again, _ := os.ReadFile(guard); string(again) != "mine\n*\n" {
		t.Fatalf("second call changed the file: %q", again)
	}
}

func TestEnsureDirLeavesAnIgnoreAllFileAlone(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	guard := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(guard, []byte("*\n!keep.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitguard.EnsureDir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(guard); string(data) != "*\n!keep.txt\n" {
		t.Fatalf("an existing ignore-all file must stay as it is, got %q", data)
	}
}

func TestEnsureDirRefusesSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := gitguard.EnsureDir(link, 0o700); err == nil {
		t.Fatal("expected symlink refusal")
	}
	if _, err := os.Stat(filepath.Join(target, ".gitignore")); err == nil {
		t.Fatal("guard written through symlink")
	}
}

func TestExtensionsDirIsNotIgnored(t *testing.T) {
	root := gitRepo(t)
	if err := gitguard.EnsureDir(filepath.Join(root, ".govard", "snapshots"), 0o700); err != nil {
		t.Fatal(err)
	}
	hooks := filepath.Join(root, ".govard", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooks, "pre_up.sh"), []byte("echo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := staged(t, root); got != ".govard/hooks/pre_up.sh" {
		t.Fatalf("extension dir must stay committable, staged: %q", got)
	}
}
