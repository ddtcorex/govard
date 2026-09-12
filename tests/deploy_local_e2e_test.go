package tests

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/engine"
)

// deployRemoteConfig writes a project whose only remote is a local one: the
// pipeline runs the real commands through the local shell against a temporary
// deploy path, so the whole flow is exercised without a server.
func deployRemoteConfig(t *testing.T, root, repository string) engine.Config {
	t.Helper()
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  keep_releases: 2
  settings:
    shared_files: [app/etc/env.php]
    writable_dirs: [var]
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: `+filepath.Join(root, "public_html")+`
    deploy_path: `+filepath.Join(root, ".deployer")+`
    branch: main
    repository: `+repository+`
    local: true
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	return cfg
}

func deployOptionsForTest(t *testing.T, cfg engine.Config, revision string) deploy.Options {
	t.Helper()
	options, err := deploy.ResolveOptionsForTest(cfg, "local", deploy.Overrides{Revision: revision, Yes: true})
	if err != nil {
		t.Fatalf("resolve options: %v", err)
	}
	return options
}

func TestLocalEndToEndDeployPublishesAndVerifies(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	// Seed the shared file the config declares, as a first deploy of a real
	// project would find it.
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")

	release := deploy.NewReleaseForTest("", revision, "main")
	outcome, err := deploy.NewExecutor(host, options, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release)
	if err != nil {
		t.Fatalf("deploy: %v\nsteps: %+v", err, outcome.Steps)
	}
	if release.Release == "" {
		t.Fatal("the pipeline did not assign a release number")
	}

	resolved, err := host.Runner().Run(context.Background(), "readlink -f "+host.CurrentPath, deploy.RunOptions{})
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if strings.TrimSpace(resolved.Stdout) != host.ReleasePath(release.Release) {
		t.Fatalf("current -> %q, want %q", strings.TrimSpace(resolved.Stdout), host.ReleasePath(release.Release))
	}

	stored, err := deploy.ReadRelease(context.Background(), host, release.Release)
	if err != nil {
		t.Fatalf("read release: %v", err)
	}
	if stored.Status != deploy.StatusOK {
		t.Fatalf("release status = %q, want ok", stored.Status)
	}
	if stored.Revision != revision {
		t.Fatalf("stored revision = %q, want %q", stored.Revision, revision)
	}
	if stored.Verify.Status != "ok" {
		t.Fatalf("verify status = %q, want ok", stored.Verify.Status)
	}

	content, err := host.Runner().Run(context.Background(), "cat "+host.ReleasePath(release.Release)+"/app.txt", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("read deployed file: %v", err)
	}
	if strings.TrimSpace(content.Stdout) != "v2" {
		t.Fatalf("deployed content = %q, want v2", content.Stdout)
	}

	// The lock is released on success.
	if _, err := host.Runner().Run(context.Background(), "test ! -e "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("lock must be released after a successful deploy: %v", err)
	}
}

func TestLocalEndToEndStopsOnFailureAndKeepsTheLock(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}

	recipe := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&recipe, deploy.TaskActivate, deploy.Task{
		ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "exit 9",
	})
	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	release := deploy.NewReleaseForTest("", revision, "main")
	outcome, err := deploy.NewExecutor(host, options, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release)
	if err == nil {
		t.Fatal("want the deploy to fail")
	}
	var cmdErr *deploy.CommandError
	if !errors.As(err, &cmdErr) || cmdErr.ExitCode != 9 {
		t.Fatalf("err = %v, want a CommandError with exit 9", err)
	}
	if !outcome.LockHeld {
		t.Fatal("a publish-stage failure must keep the lock")
	}
	if _, err := host.Runner().Run(context.Background(), "test -d "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("lock directory must still exist: %v", err)
	}
	// The release directory survives so the deploy can be resumed.
	if _, err := host.Runner().Run(context.Background(), "test -d "+host.ReleasePath(release.Release)+"/.git", deploy.RunOptions{}); err == nil {
		t.Fatal("release must not be a git checkout")
	}
	if _, err := host.Runner().Run(context.Background(), "test -s "+host.ReleaseRecordPath(release.Release), deploy.RunOptions{}); err != nil {
		t.Fatalf("a failed deploy must leave a release record: %v", err)
	}

	if err := deploy.CoreUnlock(context.Background(), deploy.StepContextForTest(host, options)); err != nil {
		t.Fatalf("unlock after failure: %v", err)
	}
	if _, err := host.Runner().Run(context.Background(), "test ! -e "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("lock must be gone after unlock: %v", err)
	}
}

func TestLocalEndToEndHooksRunWhereThePlanSays(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}

	// The config declares a required shared file; verify refuses a release that
	// cannot see it, so the fixture provides it like a real project would.
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")

	hooks := []deploy.Hook{
		{Name: "after-activate", On: deploy.TaskActivate, Position: deploy.PositionAfter, Run: "printf done >> " + root + "/hook.log"},
	}
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), hooks, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	release := deploy.NewReleaseForTest("", revision, "main")
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(context.Background(), plan, deploy.NewVars(), release); err != nil {
		t.Fatalf("deploy: %v", err)
	}

	hookLog, err := os.ReadFile(filepath.Join(root, "hook.log"))
	if err != nil {
		t.Fatalf("hook did not run: %v", err)
	}
	if strings.TrimSpace(string(hookLog)) != "done" {
		t.Fatalf("hook log = %q, want done", string(hookLog))
	}
}

func TestDeployKeepsHookPreDeployBehaviour(t *testing.T) {
	root := t.TempDir()
	marker := filepath.Join(root, "hooked.txt")
	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
hooks:
  pre_deploy:
    - name: touch-marker
      run: touch `+marker+`
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: `+filepath.Join(root, "public_html")+`
    deploy_path: `+filepath.Join(root, ".deployer")+`
    branch: main
    local: true
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if err := engine.RunHooks(cfg, engine.HookPreDeploy, io.Discard, io.Discard); err != nil {
		t.Fatalf("run hooks: %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("pre_deploy hook did not run: %v", err)
	}
}

func TestExecutorSkipsAnAlreadyDeployedRevisionUnlessForced(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")

	ctx := context.Background()
	first, err := deploy.NewExecutor(host, options, io.Discard).Run(ctx, plan, deploy.NewVars(), deploy.NewReleaseForTest("", revision, "main"))
	if err != nil {
		t.Fatalf("first deploy: %v", err)
	}
	if first.AlreadyDeployed {
		t.Fatal("the first deploy must not take the no-op path")
	}

	second, err := deploy.NewExecutor(host, options, io.Discard).Run(ctx, plan, deploy.NewVars(), deploy.NewReleaseForTest("", revision, "main"))
	if err != nil {
		t.Fatalf("second deploy: %v", err)
	}
	if !second.AlreadyDeployed {
		t.Fatal("re-deploying the same revision must take the no-op fast path")
	}
	if len(second.Steps) != 0 {
		t.Fatalf("the no-op path must run no steps, ran %+v", second.Steps)
	}
	releases, err := host.Runner().Run(ctx, "ls -1 "+host.ReleasesPath()+" | wc -l", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("count releases: %v", err)
	}
	if strings.TrimSpace(releases.Stdout) != "1" {
		t.Fatalf("release count = %q, want 1 (the no-op path must not create a release)", strings.TrimSpace(releases.Stdout))
	}

	forcedOptions := options
	forcedOptions.Force = true
	forced, err := deploy.NewExecutor(host, forcedOptions, io.Discard).Run(ctx, plan, deploy.NewVars(), deploy.NewReleaseForTest("", revision, "main"))
	if err != nil {
		t.Fatalf("forced deploy: %v", err)
	}
	if forced.AlreadyDeployed {
		t.Fatal("--force must bypass the no-op fast path")
	}
	releases, err = host.Runner().Run(ctx, "ls -1 "+host.ReleasesPath()+" | wc -l", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("count releases: %v", err)
	}
	if strings.TrimSpace(releases.Stdout) != "2" {
		t.Fatalf("release count after --force = %q, want 2", strings.TrimSpace(releases.Stdout))
	}
}

func TestIncompleteReleaseFindsTheNewestFailedRelease(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()

	ok := deploy.NewReleaseForTest("1", "aaa", "main")
	ok.Status = deploy.StatusOK
	if err := deploy.WriteRelease(ctx, host, ok); err != nil {
		t.Fatalf("seed ok release: %v", err)
	}
	failed := deploy.NewReleaseForTest("2", "bbb", "main")
	failed.Status = deploy.StatusFailed
	if err := deploy.WriteRelease(ctx, host, failed); err != nil {
		t.Fatalf("seed failed release: %v", err)
	}

	found, err := deploy.IncompleteRelease(ctx, host)
	if err != nil {
		t.Fatalf("incomplete release: %v", err)
	}
	if found == nil || found.Release != "2" {
		t.Fatalf("found = %+v, want release 2", found)
	}

	allOK := deploy.NewReleaseForTest("3", "ccc", "main")
	allOK.Status = deploy.StatusOK
	if err := deploy.WriteRelease(ctx, host, allOK); err != nil {
		t.Fatalf("seed third release: %v", err)
	}
	if found, err = deploy.IncompleteRelease(ctx, host); err != nil || found == nil || found.Release != "2" {
		t.Fatalf("found = %+v (err %v), want release 2 still", found, err)
	}

	if err := deploy.WriteRelease(ctx, host, func() *deploy.Release {
		rec := deploy.NewReleaseForTest("2", "bbb", "main")
		rec.Status = deploy.StatusOK
		return rec
	}()); err != nil {
		t.Fatalf("mark release 2 ok: %v", err)
	}
	if found, err = deploy.IncompleteRelease(ctx, host); err != nil || found != nil {
		t.Fatalf("found = %+v (err %v), want nothing to resume", found, err)
	}
}

func TestResumeContinuesTheFailedReleaseInsteadOfStartingANewOne(t *testing.T) {
	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	cfg := deployRemoteConfig(t, root, origin)
	options := deployOptionsForTest(t, cfg, revision)

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	ctx := context.Background()
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")

	// First attempt fails at publish, which keeps the lock and the release.
	broken := deploy.DefaultRecipe()
	deploy.OverrideTaskForTest(&broken, deploy.TaskActivate, deploy.Task{
		ID: deploy.TaskActivate, Stage: deploy.StagePublish, Command: "exit 9",
	})
	brokenPlan, err := deploy.BuildPlanForTest(broken, nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	first := deploy.NewReleaseForTest("", revision, "main")
	if _, err := deploy.NewExecutor(host, options, io.Discard).Run(ctx, brokenPlan, deploy.NewVars(), first); err == nil {
		t.Fatal("want the first attempt to fail")
	}
	if first.Release != "1" {
		t.Fatalf("first release = %q, want 1", first.Release)
	}

	// Resume: the incomplete release is found, the stale lock is cleared, and
	// the already-succeeded steps are not repeated.
	incomplete, err := deploy.IncompleteRelease(ctx, host)
	if err != nil || incomplete == nil {
		t.Fatalf("incomplete = %+v (err %v), want release 1", incomplete, err)
	}
	if err := deploy.CoreUnlock(ctx, deploy.StepContextForTest(host, options)); err != nil {
		t.Fatalf("clear the stale lock: %v", err)
	}

	resumeOptions := options
	resumeOptions.Resume = true
	resumeRelease := deploy.NewReleaseForTest(incomplete.Release, incomplete.Revision, incomplete.Branch)
	resumeRelease.Path = incomplete.Path
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), nil, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	outcome, err := deploy.NewExecutor(host, resumeOptions, io.Discard).Run(ctx, plan, deploy.NewVars(), resumeRelease)
	if err != nil {
		t.Fatalf("resume: %v\nsteps: %+v", err, outcome.Steps)
	}

	if resumeRelease.Release != "1" {
		t.Fatalf("resume created release %q, want to continue release 1", resumeRelease.Release)
	}
	releases, err := host.Runner().Run(ctx, "ls -1 "+host.ReleasesPath()+" | wc -l", deploy.RunOptions{})
	if err != nil {
		t.Fatalf("count releases: %v", err)
	}
	if strings.TrimSpace(releases.Stdout) != "1" {
		t.Fatalf("release count = %q, want 1 (a resume must not add a release)", strings.TrimSpace(releases.Stdout))
	}

	// The lock is re-acquired during the resume and released at the end.
	if _, err := host.Runner().Run(ctx, "test ! -e "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("the resumed deploy must release its lock: %v", err)
	}
	stored, err := deploy.ReadRelease(ctx, host, "1")
	if err != nil {
		t.Fatalf("read release: %v", err)
	}
	if stored.Status != deploy.StatusOK {
		t.Fatalf("release status = %q, want ok after a successful resume", stored.Status)
	}

	// Safety-relevant steps run again rather than being skipped.
	skipped := map[string]bool{}
	for _, step := range outcome.Steps {
		if step.Status == deploy.StepSkipped {
			skipped[step.ID] = true
		}
	}
	for _, id := range []string{deploy.TaskCheck, deploy.TaskLock, deploy.TaskUnlock} {
		if skipped[id] {
			t.Errorf("%s must run again on a resume", id)
		}
	}
	if !skipped[deploy.TaskCode] {
		t.Error("deploy:code already succeeded and must be skipped on resume")
	}
}

// A resume continues the stored record, so the record must reach the run whole.
//
// Copying a few fields into a fresh Release looks equivalent and is not: the new
// object is an empty one, and `CoreVerify` switches on
// `Release.Publish.Strategy` while `deploy rollback --with-db` reads
// `Release.Database`. A resume that dropped them would run no revision check at
// all (and still report success) and would lose the dump path of a release whose
// dump is sitting on the server.
func TestResumeContinuesTheWholeStoredRecord(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()

	record := deploy.NewReleaseForTest("2", "bbb", "main")
	record.Status = deploy.StatusFailed
	record.CreatedAt = "2026-09-11T10:00:00Z"
	record.Build = deploy.BuildRecord{Mode: deploy.BuildServer, DurationMS: 1234}
	record.Publish = deploy.PublishRecord{Strategy: deploy.PublishInPlace, Docroot: "/srv/public_html"}
	record.Database = deploy.DatabaseRecord{Backup: "shared/backups/deploy/2/dump.sql"}
	if err := deploy.WriteRelease(ctx, host, record); err != nil {
		t.Fatalf("seed failed release: %v", err)
	}
	// The failed run left its lock behind, which is what resuming clears.
	if _, err := host.Runner().Run(ctx, "mkdir -p "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Fatalf("seed stale lock: %v", err)
	}

	release := deploy.NewReleaseForTest("", "local-head", "main")
	if err := cmd.PrepareResumeForTest(ctx, host, release); err != nil {
		t.Fatalf("prepare resume: %v", err)
	}

	if release.Release != "2" {
		t.Fatalf("release = %q, want the stored release 2", release.Release)
	}
	if release.Publish.Strategy != deploy.PublishInPlace || release.Publish.Docroot != "/srv/public_html" {
		t.Errorf("publish record = %+v, want the stored strategy and docroot", release.Publish)
	}
	if release.Database.Backup != "shared/backups/deploy/2/dump.sql" {
		t.Errorf("database record = %+v, want the stored dump path", release.Database)
	}
	if release.Build.Mode != deploy.BuildServer || release.Build.DurationMS != 1234 {
		t.Errorf("build record = %+v, want the stored build", release.Build)
	}
	if release.CreatedAt != "2026-09-11T10:00:00Z" {
		t.Errorf("created_at = %q, want the original creation time", release.CreatedAt)
	}
	if release.Path != host.ReleasePath("2") {
		t.Errorf("path = %q, want %q", release.Path, host.ReleasePath("2"))
	}
	if _, err := host.Runner().Run(ctx, "test ! -e "+host.LockPath(), deploy.RunOptions{}); err != nil {
		t.Errorf("a resume must clear the lock its failed run left: %v", err)
	}
}

func TestResumeWithoutAnUnfinishedReleaseLeavesTheNewRecordAlone(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	ctx := context.Background()

	release := deploy.NewReleaseForTest("", "local-head", "main")
	if err := cmd.PrepareResumeForTest(ctx, host, release); err != nil {
		t.Fatalf("prepare resume: %v", err)
	}
	if release.Release != "" || release.Revision != "local-head" {
		t.Fatalf("a resume with nothing to continue must start a new release, got %+v", release)
	}
}
