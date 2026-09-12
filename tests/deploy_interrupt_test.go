package tests

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"govard/internal/deploy"
	"govard/internal/engine"
)

// A deploy that is stopped mid-run — Ctrl-C, a cancelled CI job, a laptop lid —
// must not leave the target in a state nothing explains. The engine's own
// failure path decides that: a run that never reached the maintenance window
// releases its lock so the next attempt is not refused, and one that did keeps
// the lock, the release directory and the record so `--resume` can finish the
// job. What made that decision unreachable was the CLI: cobra ran on a context
// no signal ever cancelled, so the process died under the running step and left
// the lock — and, past `maintenance:enable`, the maintenance flag — behind with
// no message at all.

// interruptedDeploy starts a deploy whose hook blocks until it has demonstrably
// begun, then cancels the run and hands back the host, the release and the error.
func interruptedDeploy(t *testing.T, hookOn string) (deploy.Host, *deploy.Release, error) {
	t.Helper()

	origin, revision := seedGitRepo(t)
	root := t.TempDir()
	marker := filepath.Join(root, "hook-started")

	writeFile(t, filepath.Join(root, ".govard.yml"), `
project_name: sample
framework: generic
domain: sample.test
deploy:
  keep_releases: 2
  settings:
    shared_files: [app/etc/env.php]
    writable_dirs: [var]
  hooks:
    - name: block-here
      on: `+hookOn+`
      position: before
      run: touch `+marker+` && sleep 60
remotes:
  local:
    host: 127.0.0.1
    user: deployer
    path: `+filepath.Join(root, "public_html")+`
    deploy_path: `+filepath.Join(root, ".deployer")+`
    branch: main
    repository: `+origin+`
    local: true
`)
	cfg, _, err := engine.LoadConfigFromDir(root, true)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	options := deployOptionsForTest(t, cfg, revision)

	host, err := deploy.HostForConfigForTest(cfg, "local", options)
	if err != nil {
		t.Fatalf("host: %v", err)
	}
	// The hook has to be in the plan: a plan built without it would run the
	// pipeline to completion while the test waited for a marker that never comes.
	hooks := make([]deploy.Hook, 0, len(options.Hooks))
	for _, hookCfg := range options.Hooks {
		hook, hookErr := deploy.HookFromConfig(hookCfg)
		if hookErr != nil {
			t.Fatalf("hook config: %v", hookErr)
		}
		hooks = append(hooks, hook)
	}
	if len(hooks) == 0 {
		t.Fatal("the project's hook did not reach the options, so the run cannot be interrupted inside it")
	}
	plan, err := deploy.BuildPlanForTest(deploy.DefaultRecipe(), hooks, "local")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	writeFile(t, filepath.Join(host.SharedPath(), "app/etc/env.php"), "<?php return [];\n")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := deploy.NewReleaseForTest("", revision, "main")

	type result struct{ err error }
	done := make(chan result, 1)
	go func() {
		_, runErr := deploy.NewExecutor(host, options, io.Discard).Run(ctx, plan, deploy.NewVars(), release)
		done <- result{runErr}
	}()

	// Cancel only once the run is demonstrably inside the hook: the assertion is
	// about the interruption, not about racing the prepare stage.
	deadline := time.Now().Add(30 * time.Second)
	for {
		if _, statErr := os.Stat(marker); statErr == nil {
			break
		}
		select {
		case got := <-done:
			cancel()
			t.Fatalf("the run finished before the hook started: err = %v", got.err)
		default:
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the hook never started, so the interrupt would have raced the prepare stage")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()

	select {
	case got := <-done:
		return host, release, got.err
	case <-time.After(60 * time.Second):
		t.Fatal("the interrupted run never returned")
		panic("unreachable")
	}
}

// Before the maintenance window: the lock goes back, so a retry is allowed
// instead of the operator hunting for a lock nobody holds.
func TestAnInterruptedBuildReleasesTheLock(t *testing.T) {
	host, release, err := interruptedDeploy(t, "build:vendors")
	if err == nil {
		t.Fatal("an interrupted deploy must fail")
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("the failure must say the run was interrupted, got %q", err.Error())
	}
	if _, statErr := os.Stat(host.LockPath()); !os.IsNotExist(statErr) {
		t.Fatalf("a pre-publish interruption must release the lock, stat = %v (release %s)", statErr, release.Release)
	}
}

// Past `maintenance:enable`: everything stays, because the release is the only
// record of what the target is half-way through, and `--resume` is what finishes
// it. Releasing the lock here would let a second run start on top of a database
// that may be mid-migration.
func TestAnInterruptedPublishKeepsTheLock(t *testing.T) {
	host, release, err := interruptedDeploy(t, "db:migrate")
	if err == nil {
		t.Fatal("an interrupted deploy must fail")
	}
	if !strings.Contains(err.Error(), "interrupted") {
		t.Fatalf("the failure must say the run was interrupted, got %q", err.Error())
	}
	if _, statErr := os.Stat(host.LockPath()); statErr != nil {
		t.Fatalf("a publish-stage interruption must keep the lock, stat = %v (release %s)", statErr, release.Release)
	}
	// The record is what `--resume` continues: without it the operator has a held
	// lock and nothing that says which release to finish.
	stored, readErr := deploy.ReadRelease(context.Background(), host, release.Release)
	if readErr != nil {
		t.Fatalf("read the record of the interrupted release: %v", readErr)
	}
	if stored.Revision == "" {
		t.Fatalf("the kept record does not name its revision: %+v", stored)
	}
}
