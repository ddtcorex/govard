package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/deploy"
)

func TestLockIsAtomicAndRefusesASecondHolder(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})

	if err := deploy.CoreLock(ctx, sc); err != nil {
		t.Fatalf("first lock: %v", err)
	}
	if err := deploy.CoreLock(ctx, sc); !errors.Is(err, deploy.ErrLockHeld) {
		t.Fatalf("second lock err = %v, want ErrLockHeld", err)
	}
	if err := deploy.CoreUnlock(ctx, sc); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if err := deploy.CoreLock(ctx, sc); err != nil {
		t.Fatalf("re-lock after unlock: %v", err)
	}
}

func TestLockRefusesWhileDeployerHoldsItsOwnLock(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.DepPath()+" && touch "+host.DeployerLockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed deployer lock: %v", err)
	}
	err := deploy.CoreLock(ctx, deploy.StepContextForTest(host, deploy.Options{Remote: "local"}))
	if !errors.Is(err, deploy.ErrDeployerLockHeld) {
		t.Fatalf("err = %v, want ErrDeployerLockHeld", err)
	}
}

func TestCoreReleaseRefusesToReuseAnExistingNumber(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})

	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release dir: %v", err)
	}
	next, err := deploy.NextReleaseNumberForTest(ctx, host)
	if err != nil {
		t.Fatalf("next release number: %v", err)
	}
	if next != "2" {
		t.Fatalf("next release = %q, want 2", next)
	}

	// A release whose number is already fixed (a resume, or a retry) must not
	// be created twice.
	sc.Release = deploy.NewReleaseForTest("2", "abc", "local")
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.ReleasePath("2"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release dir 2: %v", err)
	}
	if err := deploy.CoreRelease(ctx, sc); !errors.Is(err, deploy.ErrReleaseExists) {
		t.Fatalf("err = %v, want ErrReleaseExists", err)
	}

	sc.Release = deploy.NewReleaseForTest("3", "abc", "local")
	if err := deploy.CoreRelease(ctx, sc); err != nil {
		t.Fatalf("create release 3: %v", err)
	}
	if _, err := host.Runner().Run(ctx, "test -d "+host.ReleasePath("3"), deploy.RunOptions{}); err != nil {
		t.Fatalf("release 3 missing: %v", err)
	}
}

func TestCoreCheckRefusesSubmodules(t *testing.T) {
	root := t.TempDir()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	if err := os.WriteFile(filepath.Join(root, ".gitmodules"), []byte("[submodule \"x\"]\n"), 0o644); err != nil {
		t.Fatalf("seed .gitmodules: %v", err)
	}
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local"})
	sc.WorkDir = root
	if err := deploy.CoreCheck(context.Background(), sc); !errors.Is(err, deploy.ErrSubmodulesUnsupported) {
		t.Fatalf("err = %v, want ErrSubmodulesUnsupported", err)
	}
}

func TestCoreCodeFetchesIntoTheMirrorAndExtractsExactlyTheRevision(t *testing.T) {
	origin, revision := seedGitRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()

	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Repository: origin, Revision: revision, Branch: "main"})
	sc.Release = deploy.NewReleaseForTest("1", revision, "main")
	if err := deploy.CoreRelease(ctx, sc); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := deploy.CoreCode(ctx, sc); err != nil {
		t.Fatalf("code: %v", err)
	}

	content, err := host.Runner().Run(ctx, "cat "+host.ReleasePath("1")+"/app.txt", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("read extracted file: %v", err)
	}
	if strings.TrimSpace(content.Stdout) != "v2" {
		t.Fatalf("extracted content = %q, want v2 (the requested revision)", content.Stdout)
	}
	if _, err := host.Runner().Run(ctx, "test -d "+host.RepoPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("bare mirror missing: %v", err)
	}
}

func TestCoreCodeFailsLoudlyForAnUnknownRevision(t *testing.T) {
	origin, _ := seedGitRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Repository: origin, Revision: "0000000000000000000000000000000000000000", Branch: "main"})
	sc.Release = deploy.NewReleaseForTest("1", "deadbeef", "main")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreCode(context.Background(), sc); !errors.Is(err, deploy.ErrRevisionMissing) {
		t.Fatalf("err = %v, want ErrRevisionMissing", err)
	}
}

// seedGitRepo creates a bare origin with two commits and returns its path and
// the second commit's SHA.
func seedGitRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")

	runGit := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out))
	}

	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	runGit(root, "init", "--bare", "-q", origin)
	runGit(work, "init", "-q", "-b", "main")
	writeFile(t, filepath.Join(work, "app.txt"), "v1\n")
	runGit(work, "add", ".")
	runGit(work, "commit", "-q", "-m", "v1")
	writeFile(t, filepath.Join(work, "app.txt"), "v2\n")
	runGit(work, "add", ".")
	runGit(work, "commit", "-q", "-m", "v2")
	revision := runGit(work, "rev-parse", "HEAD")
	runGit(work, "remote", "add", "origin", origin)
	runGit(work, "push", "-q", "origin", "main")
	return origin, revision
}

func TestCoreSharedLinksExistingSharedEntries(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()

	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.ReleasePath("1"), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:   "local",
		Settings: map[string]any{"shared_files": []string{"app/etc/env.php"}},
	})
	sc.Release = deploy.NewReleaseForTest("1", "abc", "local")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreShared(ctx, sc); err != nil {
		t.Fatalf("shared: %v", err)
	}

	link, err := host.Runner().Run(ctx, "readlink -f "+host.ReleasePath("1")+"/app/etc/env.php", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("read shared link: %v", err)
	}
	if strings.TrimSpace(link.Stdout) != host.SharedPath()+"/app/etc/env.php" {
		t.Fatalf("shared link -> %q, want the shared entry", strings.TrimSpace(link.Stdout))
	}
}

func TestCoreWritableAppliesTheMode(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()

	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.ReleasePath("1")+"/var && chmod 0700 "+host.ReleasePath("1")+"/var", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed release: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:   "local",
		Settings: map[string]any{"writable_dirs": []string{"var"}},
	})
	sc.Release = deploy.NewReleaseForTest("1", "abc", "local")
	sc.Release.Path = host.ReleasePath("1")
	if err := deploy.CoreWritable(ctx, sc); err != nil {
		t.Fatalf("writable: %v", err)
	}

	mode, err := host.Runner().Run(ctx, "stat -c %a "+host.ReleasePath("1")+"/var", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if strings.TrimSpace(mode.Stdout) != "775" {
		t.Fatalf("mode = %q, want 775", strings.TrimSpace(mode.Stdout))
	}
}

func TestDefaultRecipeWiresTheImplementedCoreTasks(t *testing.T) {
	recipe := deploy.DefaultRecipe()
	for _, id := range []string{
		deploy.TaskCheck, deploy.TaskLock, deploy.TaskUnlock, deploy.TaskRelease,
		deploy.TaskCode, deploy.TaskShared, deploy.TaskWritable,
	} {
		if recipe.Task(id).Core == nil {
			t.Errorf("%s has no core implementation", id)
		}
	}
	for _, id := range []string{deploy.TaskCompile, deploy.TaskAssets, deploy.TaskDBMigrate, deploy.TaskAppConfigure, deploy.TaskVendors} {
		if !recipe.Task(id).IsEmpty() {
			t.Errorf("%s must stay empty so a framework recipe can fill it", id)
		}
	}
}

// scriptedRunner fails any command containing a marker, so a probe's failure
// path can be tested without a broken server.
type scriptedRunner struct {
	base            deploy.Runner
	failSubstring   string
	answerSubstring string
	answerStdout    string
}

func (r scriptedRunner) Run(ctx context.Context, command string, opts deploy.RunOptions) (deploy.Result, error) {
	if r.failSubstring != "" && strings.Contains(command, r.failSubstring) {
		result := deploy.Result{ExitCode: 1, Stderr: "scripted failure"}
		return result, &deploy.CommandError{Command: command, ExitCode: 1, Stderr: "scripted failure"}
	}
	if r.answerSubstring != "" && strings.Contains(command, r.answerSubstring) {
		return deploy.Result{Stdout: r.answerStdout}, nil
	}
	return r.base.Run(ctx, command, opts)
}

func TestCoreCheckRefusesATargetWithoutAtomicRename(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), scriptedRunner{base: deploy.LocalRunner{}, failSubstring: "mv -T"})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Publish: deploy.PublishSymlink})
	if err := deploy.CoreCheck(context.Background(), sc); !errors.Is(err, deploy.ErrMoveAtomicUnsupported) {
		t.Fatalf("err = %v, want ErrMoveAtomicUnsupported", err)
	}
}

func TestCoreCheckReportsAnUnreachableRepository(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), scriptedRunner{base: deploy.LocalRunner{}, failSubstring: "ls-remote"})
	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:     "local",
		Repository: "git@example.invalid:nope.git",
		Branch:     "main",
	})
	err := deploy.CoreCheck(context.Background(), sc)
	if err == nil || !strings.Contains(err.Error(), "cannot reach") {
		t.Fatalf("err = %v, want a repository-reachability failure", err)
	}
}

func TestCoreCheckCollectsNotesOnAHealthyTarget(t *testing.T) {
	origin, _ := seedGitRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote:     "local",
		Repository: origin,
		Branch:     "main",
		Publish:    deploy.PublishSymlink,
	})
	if err := deploy.CoreCheck(context.Background(), sc); err != nil {
		t.Fatalf("check: %v", err)
	}
	joined := strings.Join(sc.Notes, " | ")
	for _, want := range []string{"publish strategy", "atomic symlink rename", "repository reachable", "free space"} {
		if !strings.Contains(joined, want) {
			t.Errorf("notes %q missing %q", joined, want)
		}
	}
}

func TestCoreCheckComparesTheDeclaredPHPVersion(t *testing.T) {
	// The target's PHP is stubbed so the comparison is exercised on any host,
	// including one without php installed.
	runner := scriptedRunner{base: deploy.LocalRunner{}, answerSubstring: "PHP_VERSION", answerStdout: "8.3.6\n"}

	match := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:   "local",
		Publish:  deploy.PublishSymlink,
		Settings: map[string]any{"php_bin": "php", "php_version": "8.3"},
	})
	if err := deploy.CoreCheck(context.Background(), match); err != nil {
		t.Fatalf("a matching php version must pass: %v", err)
	}

	mismatch := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:   "local",
		Publish:  deploy.PublishSymlink,
		Settings: map[string]any{"php_bin": "php", "php_version": "9.9"},
	})
	err := deploy.CoreCheck(context.Background(), mismatch)
	if err == nil || !strings.Contains(err.Error(), "declares 9.9") {
		t.Fatalf("err = %v, want a php version mismatch", err)
	}
}

// artifactGateFixture writes an artifact whose manifest records a PHP version,
// so the parity gate has something to compare.
func artifactGateFixture(t *testing.T, revision, phpVersion string) string {
	t.Helper()
	artifactDir := t.TempDir()
	writeFile(t, filepath.Join(artifactDir, "composer.json"), `{"name":"sample/project"}`)
	manifest, err := deploy.BuildManifest(artifactDir, revision, phpVersion)
	if err != nil {
		t.Fatalf("build manifest: %v", err)
	}
	if err := deploy.WriteManifest(artifactDir, manifest); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return artifactDir
}

func TestCoreCheckGatesAnArtifactOnTheTargetsPHPVersion(t *testing.T) {
	runner := scriptedRunner{base: deploy.LocalRunner{}, answerSubstring: "PHP_VERSION", answerStdout: "8.2.11\n"}

	matching := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:      "local",
		Publish:     deploy.PublishSymlink,
		Build:       deploy.BuildArtifact,
		ArtifactDir: artifactGateFixture(t, "abc123", "8.2.11"),
		Revision:    "abc123",
	})
	if err := deploy.CoreCheck(context.Background(), matching); err != nil {
		t.Fatalf("an artifact built for the target's php must pass: %v", err)
	}
	if joined := strings.Join(matching.Notes, " | "); !strings.Contains(joined, "artifact php 8.2.11") {
		t.Errorf("the notes must record the comparison, got %q", joined)
	}

	mismatched := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:      "local",
		Publish:     deploy.PublishSymlink,
		Build:       deploy.BuildArtifact,
		ArtifactDir: artifactGateFixture(t, "abc123", "8.3.6"),
		Revision:    "abc123",
	})
	err := deploy.CoreCheck(context.Background(), mismatched)
	if err == nil {
		t.Fatal("want a refusal when the artifact was built for a different php")
	}
	for _, want := range []string{"8.3.6", "8.2.11", "deploy build"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must name %q, got %q", want, err.Error())
		}
	}
}

func TestCoreCheckRefusesAnArtifactBuiltForAnotherRevision(t *testing.T) {
	runner := scriptedRunner{base: deploy.LocalRunner{}, answerSubstring: "PHP_VERSION", answerStdout: "8.2.11\n"}
	sc := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:      "local",
		Publish:     deploy.PublishSymlink,
		Build:       deploy.BuildArtifact,
		ArtifactDir: artifactGateFixture(t, "abc123", ""),
		Revision:    "def456",
	})
	err := deploy.CoreCheck(context.Background(), sc)
	if err == nil {
		t.Fatal("want a refusal when the artifact was built for another revision")
	}
	if !strings.Contains(err.Error(), "abc123") || !strings.Contains(err.Error(), "def456") {
		t.Fatalf("the refusal must name both revisions, got %q", err.Error())
	}
}

func TestCoreCheckDoesNotGateAnArtifactWithoutAPHPVersion(t *testing.T) {
	// A project that is not a PHP project records no version, and the gate must
	// not invent one: a probe that cannot compare is not a failure.
	runner := scriptedRunner{base: deploy.LocalRunner{}, failSubstring: "PHP_VERSION"}
	sc := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:      "local",
		Publish:     deploy.PublishSymlink,
		Build:       deploy.BuildArtifact,
		ArtifactDir: artifactGateFixture(t, "abc123", ""),
		Revision:    "abc123",
	})
	if err := deploy.CoreCheck(context.Background(), sc); err != nil {
		t.Fatalf("an artifact without a php version must not be gated: %v", err)
	}
	if joined := strings.Join(sc.Notes, " | "); !strings.Contains(joined, "no PHP version") {
		t.Errorf("the operator must be told the comparison did not happen, got %q", joined)
	}
}

func TestCoreCheckLeavesAServerBuildAlone(t *testing.T) {
	runner := scriptedRunner{base: deploy.LocalRunner{}, failSubstring: "PHP_VERSION"}
	sc := deploy.StepContextForTest(deploy.HostForTest(t.TempDir(), runner), deploy.Options{
		Remote:      "local",
		Publish:     deploy.PublishSymlink,
		Build:       deploy.BuildServer,
		ArtifactDir: artifactGateFixture(t, "abc123", "8.3.6"),
	})
	if err := deploy.CoreCheck(context.Background(), sc); err != nil {
		t.Fatalf("a server build must not read an artifact: %v", err)
	}
}
