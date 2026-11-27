package cmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/engine"
)

func writeTreeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func treeHas(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

// The framework's own local-only paths are root relative: Magento's `var` is the
// project's var directory, not every directory named var anywhere in the tree.
// Unanchored, `--exclude var` also stops a nested lib/var from being synced.
func TestSyncProtectedExcludesAnchorTheFrameworkPaths(t *testing.T) {
	excludes := engine.GetSyncProtectedExcludes("magento2")
	has := func(pattern string) bool {
		for _, entry := range excludes {
			if entry == pattern {
				return true
			}
		}
		return false
	}
	for _, anchored := range []string{"/app/etc/env.php", "/generated", "/pub/static", "/var"} {
		if !has(anchored) {
			t.Errorf("framework path %q is not anchored to the transfer root: %v", anchored, excludes)
		}
	}
	for _, unanchored := range []string{"var", "generated", "pub/static", "app/etc/env.php"} {
		if has(unanchored) {
			t.Errorf("framework path %q is still unanchored: %v", unanchored, excludes)
		}
	}
	// Source control and secrets stay unanchored on purpose: a nested checkout or
	// a nested .env is just as unrecoverable.
	for _, shared := range []string{".git/", ".env", "*.pem", "node_modules/"} {
		if !has(shared) {
			t.Errorf("shared protected path %q is missing: %v", shared, excludes)
		}
	}
}

// Real rsync, remote source through an ssh shim. The root var and the local
// env.php survive a --delete pull; a nested directory that merely shares the name
// is an ordinary directory and is synced.
func TestSyncDeleteProtectsRootPathsAndStillSyncsNestedOnes(t *testing.T) {
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync is not installed")
	}
	source, destination := t.TempDir(), t.TempDir()
	writeTreeFile(t, source, "index.php", "new")
	writeTreeFile(t, source, "lib/var/keep.txt", "nested var is ordinary source")
	writeTreeFile(t, source, "var/cache/from-release", "must not be copied over local var")
	writeTreeFile(t, destination, "index.php", "old")
	writeTreeFile(t, destination, "stale.txt", "gone on the source")
	writeTreeFile(t, destination, "var/sessions/s1", "local session")
	writeTreeFile(t, destination, "app/etc/env.php", "LOCAL_CREDENTIALS")
	writeTreeFile(t, destination, ".git/HEAD", "ref: refs/heads/main")

	shimDir := t.TempDir()
	shim := "#!/bin/sh\nwhile [ $# -gt 0 ]; do case \"$1\" in -o|-i|-p|-l|-F|-J|-c|-S|-b|-D|-L|-R|-w|-E|-e) shift 2;; -*) shift;; *) break;; esac; done\nshift\nexec sh -c \"$*\"\n"
	if err := os.WriteFile(filepath.Join(shimDir, "ssh"), []byte(shim), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", shimDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())

	endpoints := ResolvedSyncEndpoints{
		Source:      SyncEndpoint{Name: "staging", IsLocal: false, RemoteCfg: engine.RemoteConfig{Host: "example.invalid", User: "deploy"}, RootPath: source},
		Destination: SyncEndpoint{Name: "local", IsLocal: true, RootPath: destination},
	}
	plan, err := buildSyncExecutionPlan(engine.Config{Framework: "magento2"}, endpoints, SyncExecutionOptions{Files: true, Delete: true}, io.Discard)
	if err != nil {
		t.Fatalf("buildSyncExecutionPlan: %v", err)
	}
	if output, err := plan.RsyncCommands[0].CombinedOutput(); err != nil {
		t.Fatalf("rsync failed: %v\n%s", err, output)
	}

	for _, kept := range []string{".git/HEAD", "app/etc/env.php", "var/sessions/s1"} {
		if !treeHas(destination, kept) {
			t.Errorf("%s was deleted by --delete", kept)
		}
	}
	if treeHas(destination, "var/cache/from-release") {
		t.Error("the release's var was copied over the project's protected var")
	}
	if !treeHas(destination, "lib/var/keep.txt") {
		t.Error("a nested directory named var was excluded: the pattern is not anchored")
	}
	if treeHas(destination, "stale.txt") {
		t.Error("--delete no longer removes files the source does not have")
	}
	body, _ := os.ReadFile(filepath.Join(destination, "app/etc/env.php"))
	if !strings.Contains(string(body), "LOCAL_CREDENTIALS") {
		t.Errorf("local env.php was overwritten: %q", body)
	}
}
