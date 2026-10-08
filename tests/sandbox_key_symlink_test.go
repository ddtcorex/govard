package tests

import (
	"os"
	"path/filepath"
	"testing"

	"govard/internal/deploy"
)

// A project is untrusted input: a symlink planted in .govard/sandbox/ must never
// make govard chmod or overwrite a file outside the state directory.

func TestSandboxKeyNeverChmodsTheTargetOfASymlinkedPrivateKey(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(root, "victim")
	if err := os.WriteFile(victim, []byte("not a key"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, filepath.Join(dir, deploy.SandboxKeyName)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, deploy.SandboxKeyName+".pub"), []byte("ssh-ed25519 AAAA govard-sandbox\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.EnsureSandboxKey(dir); err == nil {
		t.Fatal("a symlinked private key must be refused")
	}
	info, err := os.Stat(victim)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Fatalf("the symlink target was chmodded to %o", info.Mode().Perm())
	}
}

func TestSandboxKeyNeverWritesThroughASymlinkWhenGenerating(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(root, "victim")
	if err := os.WriteFile(victim, []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Present private path (valid link), absent public key: the generator runs.
	if err := os.Symlink(victim, filepath.Join(dir, deploy.SandboxKeyName)); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.EnsureSandboxKey(dir); err == nil {
		t.Fatal("generating over a symlink must be refused")
	}
	if b, _ := os.ReadFile(victim); string(b) != "precious" {
		t.Fatalf("the symlink target was overwritten: %q", b)
	}
	// A dangling link must not be created through either.
	dangling := filepath.Join(root, "created-through-link")
	dir2 := filepath.Join(root, "state2")
	if err := os.MkdirAll(dir2, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(dangling, filepath.Join(dir2, deploy.SandboxKeyName)); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.EnsureSandboxKey(dir2); err == nil {
		t.Fatal("generating over a dangling symlink must be refused")
	}
	if _, err := os.Lstat(dangling); err == nil {
		t.Fatal("the key was written through a dangling symlink")
	}
}

func TestSandboxStateDirThatIsASymlinkIsRefused(t *testing.T) {
	root := t.TempDir()
	elsewhere := filepath.Join(root, "elsewhere")
	if err := os.MkdirAll(elsewhere, 0o755); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "state")
	if err := os.Symlink(elsewhere, dir); err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.EnsureSandboxKey(dir); err == nil {
		t.Fatal("a symlinked state directory must be refused")
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Fatalf("files were written through the symlinked directory: %v", entries)
	}
}

func TestSandboxKeyIsCreatedPrivateAndRegular(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	pair, err := deploy.EnsureSandboxKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(pair.PrivatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("private key mode %v, want a regular 0600 file", info.Mode())
	}
}
