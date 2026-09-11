package tests

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
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

// sandboxHost builds a local host marked as a govard sandbox, rooted at a real
// checkout so the mirror refresh has something to fetch from.
func sandboxHost(t *testing.T, work string, runner deploy.Runner) deploy.Host {
	t.Helper()
	host := deploy.HostForTest(t.TempDir(), runner)
	host.Name = "sandbox"
	host.Remote = engine.RemoteConfig{Sandbox: true, Host: "127.0.0.1", User: "deployer", Port: 49153}
	_ = work
	return host
}

func TestCoreCheckRefreshesTheSandboxMirror(t *testing.T) {
	work, _ := seedBuildRepo(t)
	mirror := filepath.Join(work, ".govard", "sandbox", "repo.git")
	// The mirror exists because `sandbox up` created it; the commit that
	// follows is one the mirror has never seen.
	if err := deploy.RefreshSandboxMirrorForTest(context.Background(), deploy.LocalRunner{}, work, mirror); err != nil {
		t.Fatalf("seed the mirror: %v", err)
	}
	writeFile(t, filepath.Join(work, "app.php"), "<?php echo 'v2';\n")
	runGitInDir(t, work, "add", ".")
	runGitInDir(t, work, "commit", "-q", "-m", "v2")
	revision := strings.TrimSpace(runGitInDir(t, work, "rev-parse", "HEAD"))

	host := sandboxHost(t, work, deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "sandbox", Publish: deploy.PublishSymlink})
	sc.WorkDir = work

	if err := deploy.CoreCheck(context.Background(), sc); err != nil {
		t.Fatalf("check: %v", err)
	}
	mirrored := strings.TrimSpace(runGitInDir(t, mirror, "rev-parse", "refs/heads/main"))
	if mirrored != revision {
		t.Fatalf("the mirror holds %q, want the commit just made %q", mirrored, revision)
	}
	if joined := strings.Join(sc.Notes, " | "); !strings.Contains(joined, "mirror refreshed") {
		t.Errorf("the notes must say the mirror was refreshed, got %q", joined)
	}
}

func TestCoreCheckRefusesASandboxWithoutAMirror(t *testing.T) {
	work, _ := seedBuildRepo(t)
	host := sandboxHost(t, work, deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "sandbox", Publish: deploy.PublishSymlink})
	sc.WorkDir = work

	err := deploy.CoreCheck(context.Background(), sc)
	if err == nil {
		t.Fatal("want a refusal when the sandbox mirror is missing")
	}
	if !strings.Contains(err.Error(), "sandbox up") {
		t.Fatalf("the refusal must name the command that creates it, got %q", err.Error())
	}
}

func TestCoreCheckNamesTheSandboxWhenTheContainerIsGone(t *testing.T) {
	work, _ := seedBuildRepo(t)
	host := sandboxHost(t, work, scriptedRunner{base: deploy.LocalRunner{}, failSubstring: "true"})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "sandbox", Publish: deploy.PublishSymlink})
	sc.WorkDir = work

	err := deploy.CoreCheck(context.Background(), sc)
	if err == nil {
		t.Fatal("want a failure when the sandbox container is gone")
	}
	// A generic "unreachable" sends the operator looking at a network; the
	// actual remedy is to bring the container back.
	if !strings.Contains(err.Error(), "sandbox") || !strings.Contains(err.Error(), "sandbox up") {
		t.Fatalf("the failure must name the sandbox and the remedy, got %q", err.Error())
	}
}

func TestCoreCheckLeavesANonSandboxRemoteAlone(t *testing.T) {
	work, _ := seedBuildRepo(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Publish: deploy.PublishSymlink})
	sc.WorkDir = work

	if err := deploy.CoreCheck(context.Background(), sc); err != nil {
		t.Fatalf("check: %v", err)
	}
	if _, err := os.Stat(filepath.Join(work, ".govard", "sandbox", "repo.git")); !os.IsNotExist(err) {
		t.Fatalf("a normal remote must not create a sandbox mirror (%v)", err)
	}
}

func runGitInDir(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	command.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

// seedOriginWithDevBranch builds a bare origin whose only branch is `dev`, plus
// a tag named `main`. The tag name is the trap: an unqualified `ls-remote main`
// matches it, so a branch check that is not fully qualified passes on a
// repository that has no such branch at all.
func seedOriginWithDevBranch(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	work := filepath.Join(root, "work")
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = work
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
	if _, err := exec.Command("git", "init", "--bare", "-q", origin).CombinedOutput(); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	runGit("init", "-q", "-b", "dev")
	writeFile(t, filepath.Join(work, "app.txt"), "v1\n")
	runGit("add", ".")
	runGit("commit", "-q", "-m", "v1")
	runGit("tag", "main")
	runGit("remote", "add", "origin", origin)
	runGit("push", "-q", "origin", "dev", "refs/tags/main")
	return origin
}

func TestRepositoryCheckFullyQualifiesTheBranchRef(t *testing.T) {
	origin := seedOriginWithDevBranch(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	reachable := func(branch string) error {
		sc := deploy.StepContextForTest(host, deploy.Options{
			Remote: "local", Repository: origin, Branch: branch, Revision: "abc",
		})
		return deploy.CheckRepositoryReachableForTest(context.Background(), sc)
	}

	// A tag named `main` must not satisfy a check for the branch `main`.
	if err := reachable("main"); err == nil {
		t.Fatal("a tag named main satisfied the branch check: the ref must be fully qualified")
	}
	if err := reachable("dev"); err != nil {
		t.Fatalf("the origin's real branch must pass: %v", err)
	}
}

func TestRepositoryCheckVerifiesATagExists(t *testing.T) {
	origin := seedOriginWithDevBranch(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	withTag := func(tag string) error {
		sc := deploy.StepContextForTest(host, deploy.Options{
			Remote: "local", Repository: origin, Tag: tag,
		})
		return deploy.CheckRepositoryReachableForTest(context.Background(), sc)
	}
	if err := withTag("main"); err != nil {
		t.Fatalf("a tag deploy must verify the tag it fetches, and it exists: %v", err)
	}
	if err := withTag("v9.9.9"); err == nil {
		t.Fatal("a tag that was never pushed must fail the preflight, not the deploy")
	}
	// A branch named `dev` must not satisfy a check for the tag `dev`.
	if err := withTag("dev"); err == nil {
		t.Fatal("a branch named dev satisfied the tag check: the ref must be fully qualified")
	}
}

func TestRepositoryCheckStillProvesReachabilityForARevisionOnlyDeploy(t *testing.T) {
	origin := seedOriginWithDevBranch(t)
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	sc := deploy.StepContextForTest(host, deploy.Options{
		Remote: "local", Repository: origin, Revision: "0123456789abcdef0123456789abcdef01234567",
	})
	if err := deploy.CheckRepositoryReachableForTest(context.Background(), sc); err != nil {
		t.Fatalf("a revision-only deploy still has to reach the repository: %v", err)
	}

	broken := deploy.StepContextForTest(host, deploy.Options{
		Remote: "local", Repository: filepath.Join(t.TempDir(), "nope.git"), Revision: "abc",
	})
	if err := deploy.CheckRepositoryReachableForTest(context.Background(), broken); err == nil {
		t.Fatal("an unreachable repository must fail even without a branch")
	}
}

// Spec 5.1: `deploy_path` has no default, but the layout the server already has
// is discoverable. Exactly one candidate must match — adopting one of several is
// the guess the no-default rule exists to prevent.
// discoveryHostForTest is a host with no deploy path configured, which is the
// only state discovery applies to.
func discoveryHostForTest() deploy.Host {
	return deploy.Host{Name: "local", Local: true}.WithRunner(deploy.LocalRunner{})
}

func TestDeployPathDiscoveryReadsTheLayoutFromTheServer(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	deployerDir := filepath.Join(home, ".deployer")
	for _, dir := range []string{deployerDir, filepath.Join(deployerDir, "releases"), filepath.Join(deployerDir, "shared")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	host := discoveryHostForTest()
	candidates := []string{home, deployerDir}

	found, err := deploy.DiscoverDeployPathForTest(context.Background(), host, candidates)
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if found != deployerDir {
		t.Fatalf("discovered %q, want %q", found, deployerDir)
	}

	// A configured deploy_path is never overridden by a probe.
	configured := host
	configured.DeployPath = filepath.Join(root, "chosen")
	got, err := deploy.DiscoverDeployPathForTest(context.Background(), configured, candidates)
	if err != nil || got != configured.DeployPath {
		t.Fatalf("a configured deploy_path must win: got %q (err %v)", got, err)
	}
}

func TestDeployPathDiscoveryRefusesToGuess(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "empty")
	if err := os.MkdirAll(bare, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	host := discoveryHostForTest()

	// Nothing there: the error has to name what was probed, because the operator
	// has to write deploy_path.
	if _, err := deploy.DiscoverDeployPathForTest(context.Background(), host, []string{bare}); !errors.Is(err, deploy.ErrDeployPathMissing) {
		t.Fatalf("err = %v, want ErrDeployPathMissing", err)
	} else if !strings.Contains(err.Error(), bare) {
		t.Fatalf("the refusal must name the candidates, got %q", err.Error())
	}

	// Two layouts: choosing either would point the pipeline at a directory
	// nobody picked.
	first, second := filepath.Join(root, "one"), filepath.Join(root, "two")
	for _, dir := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(dir, "releases"), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	_, err := deploy.DiscoverDeployPathForTest(context.Background(), host, []string{first, second})
	if !errors.Is(err, deploy.ErrDeployPathMissing) {
		t.Fatalf("two layouts must be a refusal, got %v", err)
	}
	if !strings.Contains(err.Error(), first) || !strings.Contains(err.Error(), second) {
		t.Fatalf("the refusal must name both candidates, got %q", err.Error())
	}
}

// A `current` symlink alone is a layout: an in-place target may have no
// releases directory at all.
func TestDeployPathDiscoveryAcceptsACurrentSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "public_html")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(home, "current")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	host := discoveryHostForTest()
	found, err := deploy.DiscoverDeployPathForTest(context.Background(), host, []string{home})
	if err != nil {
		t.Fatalf("discovery: %v", err)
	}
	if found != home {
		t.Fatalf("discovered %q, want %q", found, home)
	}
}

// Spec 12.1: a held lock refuses a second deploy "with a message naming the
// holder". The refusal named only the path, so an operator had to run
// `deploy unlock` just to find out who was holding it.
func TestLockRefusalNamesTheHolder(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	runner := host.Runner()

	// Another run holds the lock, with a known identity on disk.
	owner := `{"pid":4242,"actor":"ci-runner","revision":"0123456789abcdef","branch":"main","host":"production","started_at":"2026-01-01T00:00:00Z"}`
	seed := "mkdir -p " + host.LockPath() + " && printf '%s\n' '" + owner + "' > " + host.LockOwnerPath()
	if _, err := runner.Run(ctx, seed, deploy.RunOptions{}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	err := deploy.CoreLock(ctx, deploy.StepContextForTest(host, deploy.Options{Remote: "local"}))
	if !errors.Is(err, deploy.ErrLockHeld) {
		t.Fatalf("err = %v, want ErrLockHeld", err)
	}
	for _, want := range []string{"ci-runner", "01234567", host.LockPath()} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("the refusal must name %q, got %q", want, err.Error())
		}
	}
}

func TestDescribeLockOwnerReadsTheRecord(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	owner := `{"pid":4242,"actor":"ci","revision":"0123456789abcdef","branch":"main","host":"production","started_at":"2026-01-01T00:00:00Z"}`
	if _, err := host.Runner().Run(ctx, "cat > "+host.LockOwnerPath()+" <<'EOF'\n"+owner+"\nEOF", deploy.RunOptions{}); err != nil {
		t.Fatalf("seed owner: %v", err)
	}

	described, heldFor := deploy.DescribeLockOwnerForTest(ctx, host, time.Now())
	if !strings.Contains(described, "ci") || !strings.Contains(described, "01234567") {
		t.Fatalf("described = %q, want the actor and the short revision", described)
	}
	if heldFor <= 0 {
		t.Fatalf("heldFor = %s, want a positive duration for a lock from 2026-01-01", heldFor)
	}

	// A lock whose metadata is unreadable is still describable, because
	// refusing to unlock it would be worse than naming it "unknown".
	if _, err := host.Runner().Run(ctx, "rm -f "+host.LockOwnerPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("remove owner: %v", err)
	}
	described, heldFor = deploy.DescribeLockOwnerForTest(ctx, host, time.Now())
	if described != "unknown" || heldFor != -1 {
		t.Fatalf("missing owner.json -> (%q, %s), want (unknown, -1)", described, heldFor)
	}
}

// Spec 10.4 asks `deploy check` to verify Composer can authenticate before the
// deploy starts. What is checkable without running Composer on the target is
// whether credentials are *available* at all — and that is the failure that
// actually happens ("the first project to migrate an environment that pulls from a
// private repository has build:vendors fail with no preflight warning").
func TestCoreCheckWarnsWhenPrivateRepositoriesHaveNoCredentials(t *testing.T) {
	t.Setenv("COMPOSER_AUTH", "")

	private := `{"require":{"vendor/pkg":"^1.0"},"repositories":[{"type":"composer","url":"https://repo.example.com"},{"type":"composer","url":"https://repo.packagist.org"}]}`
	check := func(t *testing.T, composerJSON string, seedSharedAuth bool, options deploy.Options) []string {
		t.Helper()
		work := t.TempDir()
		writeFile(t, filepath.Join(work, "composer.json"), composerJSON)

		host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
		if seedSharedAuth {
			if _, err := host.Runner().Run(context.Background(),
				"mkdir -p "+host.SharedPath()+" && echo '{}' > "+host.SharedPath()+"/auth.json", deploy.RunOptions{}); err != nil {
				t.Fatalf("seed shared auth.json: %v", err)
			}
		}
		sc := deploy.StepContextForTest(host, options)
		sc.WorkDir = work
		if err := deploy.CoreCheck(context.Background(), sc); err != nil {
			t.Fatalf("check: %v", err)
		}
		return sc.Notes
	}

	notes := strings.Join(check(t, private, false, deploy.Options{Remote: "local", Build: deploy.BuildServer}), "\n")
	if !strings.Contains(notes, "COMPOSER_AUTH") || !strings.Contains(notes, "auth.json") {
		t.Fatalf("the warning must name both remedies, got %q", notes)
	}
	if !strings.Contains(notes, "repo.example.com") {
		t.Fatalf("the warning must name the repository, got %q", notes)
	}

	// A credential source silences it: the environment…
	notes = strings.Join(check(t, private, false, deploy.Options{Remote: "local", Build: deploy.BuildServer}), "\n")
	t.Setenv("COMPOSER_AUTH", `{"http-basic":{}}`)
	notes = strings.Join(check(t, private, false, deploy.Options{Remote: "local", Build: deploy.BuildServer}), "\n")
	if strings.Contains(notes, "warning") {
		t.Fatalf("a set COMPOSER_AUTH must silence the warning, got %q", notes)
	}

	// …or the shared file the other deploy tool leaves behind.
	t.Setenv("COMPOSER_AUTH", "")
	notes = strings.Join(check(t, private, true, deploy.Options{Remote: "local", Build: deploy.BuildServer}), "\n")
	if strings.Contains(notes, "warning") {
		t.Fatalf("a shared auth.json must silence the warning, got %q", notes)
	}

	// A packagist-only project needs nothing.
	public := `{"repositories":[{"type":"composer","url":"https://repo.packagist.org"}]}`
	notes = strings.Join(check(t, public, false, deploy.Options{Remote: "local", Build: deploy.BuildServer}), "\n")
	if strings.Contains(notes, "warning") {
		t.Fatalf("a public-only manifest must not warn, got %q", notes)
	}

}

// An artifact deploy installs nothing on the target: the build job already had
// whatever credentials it needed, and warning about the target would be noise.
//
// It is asserted at the check's own boundary rather than through CoreCheck,
// because the rest of the artifact preflight needs a real manifest and a PHP on
// the target, neither of which this case is about.
func TestComposerCredentialNoteIsSilentForAnArtifactDeploy(t *testing.T) {
	t.Setenv("COMPOSER_AUTH", "")

	work := t.TempDir()
	writeFile(t, filepath.Join(work, "composer.json"),
		`{"repositories":[{"type":"composer","url":"https://repo.example.com"}]}`)

	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	sc := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Build: deploy.BuildArtifact, ArtifactDir: t.TempDir()})
	sc.WorkDir = work
	if err := deploy.NoteComposerCredentialsForTest(context.Background(), sc); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(sc.Notes) != 0 {
		t.Fatalf("an artifact deploy must not warn about target-side credentials, got %v", sc.Notes)
	}

	// The same checkout in server mode warns, which is what makes the silence
	// above a decision rather than an accident.
	server := deploy.StepContextForTest(host, deploy.Options{Remote: "local", Build: deploy.BuildServer})
	server.WorkDir = work
	if err := deploy.NoteComposerCredentialsForTest(context.Background(), server); err != nil {
		t.Fatalf("note: %v", err)
	}
	if len(server.Notes) == 0 || !strings.Contains(strings.Join(server.Notes, "\n"), "warning") {
		t.Fatalf("a server build with no credentials must warn, got %v", server.Notes)
	}
}
