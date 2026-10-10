package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
)

func baseCacheKey() cmd.BuildCacheKeyForTest {
	return cmd.BuildCacheKeyForTest{
		Revision:           "1111111111111111111111111111111111111111",
		ComposerLockSHA256: "aa",
		PHP:                "8.2",
		Mode:               "artifact/host",
		GovardVersion:      "v1",
	}
}

func TestBuildCacheKeyChangesWithEachComponent(t *testing.T) {
	base := baseCacheKey()
	seen := map[string]string{cmd.BuildCacheIDForTest(base): "base"}
	for name, mutate := range map[string]func(*cmd.BuildCacheKeyForTest){
		"revision": func(k *cmd.BuildCacheKeyForTest) { k.Revision = "2222222222222222222222222222222222222222" },
		"lock":     func(k *cmd.BuildCacheKeyForTest) { k.ComposerLockSHA256 = "bb" },
		"php":      func(k *cmd.BuildCacheKeyForTest) { k.PHP = "8.3" },
		"mode":     func(k *cmd.BuildCacheKeyForTest) { k.Mode = "artifact/container" },
		"version":  func(k *cmd.BuildCacheKeyForTest) { k.GovardVersion = "v2" },
		"inputs":   func(k *cmd.BuildCacheKeyForTest) { k.InputsSHA256 = "cc" },
		"binary":   func(k *cmd.BuildCacheKeyForTest) { k.Binary = "other-binary" },
		"tools":    func(k *cmd.BuildCacheKeyForTest) { k.Tools = "other-tools" },
	} {
		key := base
		mutate(&key)
		id := cmd.BuildCacheIDForTest(key)
		if previous, dup := seen[id]; dup {
			t.Errorf("changing the %s gives the same id as %s", name, previous)
		}
		seen[id] = name
	}
	// A field boundary must matter: ("ab","c") and ("a","bc") are different keys.
	a, b := base, base
	a.PHP, a.Mode = "ab", "c"
	b.PHP, b.Mode = "a", "bc"
	if cmd.BuildCacheIDForTest(a) == cmd.BuildCacheIDForTest(b) {
		t.Error("the key must separate its fields")
	}
}

func builtArtifact(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "built")
	for name, content := range files {
		writeFile(t, filepath.Join(dir, name), content)
	}
	return dir
}

func TestBuildCacheStoreThenLookupRoundTrips(t *testing.T) {
	cacheDir := t.TempDir()
	built := builtArtifact(t, map[string]string{"manifest.json": "{}", "app/a.php": "<?php"})
	key := baseCacheKey()
	if _, ok := cmd.BuildCacheLookupForTest(cacheDir, key); ok {
		t.Fatal("an empty cache must miss")
	}
	if err := cmd.BuildCacheStoreForTest(cacheDir, key, built); err != nil {
		t.Fatal(err)
	}
	tree, ok := cmd.BuildCacheLookupForTest(cacheDir, key)
	if !ok {
		t.Fatal("a stored entry must hit")
	}
	if body, err := os.ReadFile(filepath.Join(tree, "app", "a.php")); err != nil || string(body) != "<?php" {
		t.Fatalf("cached content = %q, %v", body, err)
	}
}

func TestInterruptedStoreLeavesNoServableEntry(t *testing.T) {
	cacheDir := t.TempDir()
	built := builtArtifact(t, map[string]string{"manifest.json": "{}", "app/a.php": "<?php"})
	key := baseCacheKey()
	// The copy dies midway, as a full disk or a kill does.
	restore := cmd.BuildCacheSetCopyForTest(func(src, dst string) error {
		_ = os.MkdirAll(dst, 0o755)
		_ = os.WriteFile(filepath.Join(dst, "manifest.json"), []byte("{}"), 0o644)
		return errors.New("no space left on device")
	})
	defer restore()
	if err := cmd.BuildCacheStoreForTest(cacheDir, key, built); err == nil {
		t.Fatal("a failed copy must be reported")
	}
	if _, ok := cmd.BuildCacheLookupForTest(cacheDir, key); ok {
		t.Fatal("a half-written entry must never be a hit")
	}
	entries, _ := os.ReadDir(cacheDir)
	for _, entry := range entries {
		t.Errorf("a failed store must leave nothing behind, found %s", entry.Name())
	}
}

func TestConcurrentStoresOfTheSameKeyLeaveOneValidEntry(t *testing.T) {
	cacheDir := t.TempDir()
	built := builtArtifact(t, map[string]string{"manifest.json": "{}", "app/a.php": "<?php"})
	key := baseCacheKey()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- cmd.BuildCacheStoreForTest(cacheDir, key, built)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("a concurrent store must not fail: %v", err)
		}
	}
	entries, _ := os.ReadDir(cacheDir)
	if len(entries) != 1 {
		names := []string{}
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("want exactly one entry and no temp leftovers, got %v", names)
	}
	if _, ok := cmd.BuildCacheLookupForTest(cacheDir, key); !ok {
		t.Fatal("the surviving entry must be servable")
	}
}

func TestCachePrunesToTheThreeNewest(t *testing.T) {
	cacheDir := t.TempDir()
	built := builtArtifact(t, map[string]string{"manifest.json": "{}"})
	var keys []cmd.BuildCacheKeyForTest
	for i := 0; i < 5; i++ {
		key := baseCacheKey()
		key.Revision = fmt.Sprintf("%040d", i)
		keys = append(keys, key)
		if err := cmd.BuildCacheStoreForTest(cacheDir, key, built); err != nil {
			t.Fatal(err)
		}
		// mtime orders the entries; place each one strictly after the last, all
		// in the past so the entry stored next is the newest.
		entry := filepath.Join(cacheDir, cmd.BuildCacheIDForTest(key))
		stamp := time.Now().Add(-2*time.Hour + time.Duration(i)*time.Minute)
		if err := os.Chtimes(entry, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	// One more store triggers the prune with the five on disk.
	last := baseCacheKey()
	last.Revision = fmt.Sprintf("%040d", 9)
	if err := cmd.BuildCacheStoreForTest(cacheDir, last, built); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(cacheDir)
	if len(entries) != cmd.BuildCacheKeepForTest {
		t.Fatalf("kept %d entries, want %d", len(entries), cmd.BuildCacheKeepForTest)
	}
	if _, ok := cmd.BuildCacheLookupForTest(cacheDir, keys[0]); ok {
		t.Error("the oldest entry must be pruned")
	}
	if _, ok := cmd.BuildCacheLookupForTest(cacheDir, last); !ok {
		t.Error("the entry just stored must survive the prune")
	}
}

// sandboxBuildProject is a git project plus the stub that makes `sandbox`
// resolve as a running synthetic remote, which is the whole setup a
// `deploy build sandbox` needs.
func sandboxBuildProject(t *testing.T) (root, revision string) {
	t.Helper()
	root, revision = seedBuildRepo(t)
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
`)
	// The checkout needs a committed file for the build to archive; .govard.yml
	// is gitignored in real projects, so it stays untracked here too.
	restore := deploy.StubResolveSyntheticSandboxRemoteForTest(func(ctx context.Context, projectName string) (engine.RemoteConfig, deploy.SandboxLiveness, error) {
		return engine.RemoteConfig{
			Host: "127.0.0.1", User: deploy.SandboxUser, Sandbox: true,
			Deploy: &engine.DeployConfig{Branch: "main", Repository: deploy.SandboxRepoPath,
				Settings: map[string]any{"php_version": "8.2"}},
		}, deploy.SandboxLivenessRunning, nil
	})
	t.Cleanup(restore)
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })
	t.Cleanup(func() {
		flags := cmd.DeployBuildCommand().Flags()
		for _, name := range []string{"output", "revision", "tag", "branch", "remote"} {
			_ = flags.Set(name, "")
		}
		_ = flags.Set("force", "false")
		_ = flags.Set("no-cache", "false")
		_ = flags.Set("json", "false")
		_ = flags.Set("runner", "host")
	})
	return root, revision
}

func runDeployBuildCommand(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	command := cmd.RootCommandForTest()
	command.SetArgs(append([]string{"deploy", "build"}, args...))
	command.SetOut(&out)
	command.SetErr(io.Discard)
	err := command.ExecuteContext(context.Background())
	return out.String(), err
}

func TestDeployBuildSandboxResolvesTheSyntheticRemote(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	output := filepath.Join(root, "artifact-out")
	if _, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", output); err != nil {
		t.Fatalf("deploy build sandbox with no configured remote: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, deploy.ArtifactManifestName)); err != nil {
		t.Fatalf("the artifact manifest is missing: %v", err)
	}
}

func TestSandboxBuildHitsTheCacheOnTheSameKey(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	first := filepath.Join(root, "out-1")
	second := filepath.Join(root, "out-2")
	if out, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", first); err != nil || strings.Contains(out, "artifact cache hit") {
		t.Fatalf("first build: err=%v out=%q (a cold build is not a hit)", err, out)
	}
	out, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "artifact cache hit") {
		t.Fatalf("the second build of the same key must hit the cache: %q", out)
	}
	a, _ := os.ReadFile(filepath.Join(first, deploy.ArtifactManifestName))
	b, _ := os.ReadFile(filepath.Join(second, deploy.ArtifactManifestName))
	if len(a) == 0 || !bytes.Equal(a, b) {
		t.Fatal("a cache hit must hand back the cached artifact, manifest included")
	}
}

func TestNewCommitWithANewLockMissesTheCache(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	if _, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", filepath.Join(root, "out-1")); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) string {
		c := exec.Command("git", args...)
		c.Dir = root
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.com", "GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.com")
		out, err := c.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	// The lock hash is derived from the commit, so an end-to-end miss cannot
	// isolate it from the revision; TestBuildCacheKeyChangesWithEachComponent
	// proves the lock component on its own.
	writeFile(t, filepath.Join(root, "composer.lock"), `{"packages":[{"name":"x/y"}]}`)
	git("add", "composer.lock")
	git("commit", "-q", "-m", "lock")
	out, err := runDeployBuildCommand(t, "sandbox", "--revision", git("rev-parse", "HEAD"), "--output", filepath.Join(root, "out-2"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "artifact cache hit") {
		t.Fatalf("a new revision with a new lock must not hit: %q", out)
	}
}

func TestNoCacheForcesARebuild(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	if _, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", filepath.Join(root, "out-1")); err != nil {
		t.Fatal(err)
	}
	out, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", filepath.Join(root, "out-2"), "--no-cache")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "artifact cache hit") {
		t.Fatalf("--no-cache must rebuild: %q", out)
	}
}

func TestRealRemoteNeverReadsOrWritesTheCache(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
remotes:
  staging:
    host: staging.invalid
    user: deploy
    path: /srv/sample
    deploy:
      branch: main
      deploy_path: /srv/sample
`)
	for i, extra := range [][]string{{}, {}, {"--no-cache"}} {
		args := append([]string{"staging", "--revision", revision, "--output", filepath.Join(root, fmt.Sprintf("out-%d", i)), "--force"}, extra...)
		out, err := runDeployBuildCommand(t, args...)
		if err != nil {
			t.Fatalf("build %d: %v", i, err)
		}
		if strings.Contains(out, "artifact cache") {
			t.Fatalf("a real remote must not touch the cache: %q", out)
		}
	}
	if _, err := os.Stat(filepath.Join(deploy.SandboxStateDir(root), "build-cache")); !os.IsNotExist(err) {
		t.Fatalf("a real remote must not create the cache directory (%v)", err)
	}
}

func TestRestoreRefusesANonEmptyOutputWithoutForce(t *testing.T) {
	cacheDir := t.TempDir()
	built := builtArtifact(t, map[string]string{"manifest.json": "{}"})
	key := baseCacheKey()
	if err := cmd.BuildCacheStoreForTest(cacheDir, key, built); err != nil {
		t.Fatal(err)
	}
	tree, _ := cmd.BuildCacheLookupForTest(cacheDir, key)
	output := t.TempDir()
	writeFile(t, filepath.Join(output, "stale"), "x")
	if err := cmd.BuildCacheRestoreForTest(tree, output, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("err = %v, want the same refusal a build gives", err)
	}
	if err := cmd.BuildCacheRestoreForTest(tree, output, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, "stale")); !os.IsNotExist(err) {
		t.Fatal("--force replaces the output")
	}
}

// The artifact depends on what the project asks the build to do, not only on the
// commit: a rehearsal tunes deploy settings in an untracked .govard.yml without
// committing anything, and a cache hit would hand back the old artifact.
func TestChangedDeploySettingsMissTheCache(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	if _, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", filepath.Join(root, "out-1")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample-project
framework: generic
domain: sample.test
deploy:
  settings:
    content_version: "rehearsal-2"
`)
	out, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", filepath.Join(root, "out-2"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "artifact cache hit") {
		t.Fatalf("changed deploy settings must not reuse the old artifact: %q", out)
	}
}

func TestBuildInputsDigestFollowsSettingsHooksAndRecipe(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	options := deploy.Options{Settings: map[string]any{"frontend_command": "npm run build"}}
	hooks := []deploy.Hook{{Name: "h", On: "build", Run: "echo a"}}
	base := cmd.BuildInputsDigestForTest(recipe, hooks, options)
	if base != cmd.BuildInputsDigestForTest(recipe, hooks, options) {
		t.Fatal("the digest must be stable")
	}
	changedSettings := deploy.Options{Settings: map[string]any{"frontend_command": "npm run other"}}
	if base == cmd.BuildInputsDigestForTest(recipe, hooks, changedSettings) {
		t.Error("a changed setting must change the digest")
	}
	if base == cmd.BuildInputsDigestForTest(recipe, []deploy.Hook{{Name: "h", On: "build", Run: "echo b"}}, options) {
		t.Error("a changed hook must change the digest")
	}
	changedRecipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&changedRecipe, deploy.TaskAssets, deploy.Task{ID: deploy.TaskAssets, Command: "other"})
	if base == cmd.BuildInputsDigestForTest(changedRecipe, hooks, options) {
		t.Error("a changed recipe command must change the digest")
	}
}

func manifestRevision(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, deploy.ArtifactManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var m deploy.ArtifactManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m.Revision
}

// The deploy compares the manifest's revision text with the revision it is asked
// to deploy, so a hit must describe the build that was just asked for, not the
// spelling the first build used; and it must do so without editing the cached
// copy the output shares its data blocks with.
func TestCacheHitCarriesTheRevisionSpellingOfThisBuild(t *testing.T) {
	stubBuilderToolchain(t)
	root, revision := sandboxBuildProject(t)
	first, second, third := filepath.Join(root, "out-1"), filepath.Join(root, "out-2"), filepath.Join(root, "out-3")
	if _, err := runDeployBuildCommand(t, "sandbox", "--revision", "HEAD", "--output", first); err != nil {
		t.Fatal(err)
	}
	out, err := runDeployBuildCommand(t, "sandbox", "--revision", revision, "--output", second)
	if err != nil || !strings.Contains(out, "artifact cache hit") {
		t.Fatalf("the same commit under another spelling must hit: err=%v out=%q", err, out)
	}
	if got := manifestRevision(t, second); got != revision {
		t.Fatalf("a hit must record the revision it was asked for (%s), got %q", revision, got)
	}
	if got := manifestRevision(t, first); got != "HEAD" {
		t.Fatalf("retargeting the second output must not edit the first through the shared link, got %q", got)
	}
	if _, err := runDeployBuildCommand(t, "sandbox", "--revision", "HEAD", "--output", third); err != nil {
		t.Fatal(err)
	}
	if got := manifestRevision(t, third); got != "HEAD" {
		t.Fatalf("the cached copy must be unchanged by the earlier hit, got %q", got)
	}
}

type toolsRunner struct {
	out string
	err error
	ran []string
}

func (r *toolsRunner) Run(_ context.Context, command string, _ deploy.RunOptions) (deploy.Result, error) {
	r.ran = append(r.ran, command)
	return deploy.Result{Stdout: r.out}, r.err
}

// The builder's own PHP, Composer and Node decide what a build produces, so a
// different toolchain must not be served an artifact another one made.
func TestBuilderToolVersionsDecideTheirDigest(t *testing.T) {
	a := cmd.BuilderToolsForTest(context.Background(), &toolsRunner{out: "PHP 8.2.26\nComposer version 2.7.1\nv20.11.0\n"})
	b := cmd.BuilderToolsForTest(context.Background(), &toolsRunner{out: "PHP 8.2.26\nComposer version 2.8.0\nv20.11.0\n"})
	if a == "" || a == b {
		t.Fatalf("a different Composer must give a different digest: %q vs %q", a, b)
	}
	if again := cmd.BuilderToolsForTest(context.Background(), &toolsRunner{out: "PHP 8.2.26\nComposer version 2.7.1\nv20.11.0\n"}); again != a {
		t.Fatal("the digest must be stable")
	}
	unknown := cmd.BuilderToolsForTest(context.Background(), &toolsRunner{err: errors.New("no shell")})
	if unknown == "" || unknown == a {
		t.Fatalf("an unreadable toolchain is its own state, not an empty or shared one: %q", unknown)
	}
}
