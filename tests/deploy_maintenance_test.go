package tests

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"govard/internal/cmd"
	"govard/internal/deploy"
)

// The window's purpose is that the *served* release is in maintenance while the
// state it depends on is being changed, and that it is out again afterwards.
//
// A symlink activation changes which directory is served halfway through the
// plan, so the window has to close before the swap. Closing it afterwards removes
// a flag the incoming release never had, while the outgoing release keeps one —
// and a rollback onto that release serves maintenance mode to every visitor.
// Executed, because the property is about what the commands do to a directory
// tree, not about the order of two ids in a list.
func TestExecutorMaintainsTheServedReleaseAcrossASymlinkSwap(t *testing.T) {
	deployPath := t.TempDir()
	host := deploy.HostForTest(deployPath, deploy.LocalRunner{})
	host.CurrentPath = filepath.Join(deployPath, "public_html")

	live := filepath.Join(deployPath, "releases", "1")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatalf("seed the live release: %v", err)
	}
	if err := os.Symlink(live, host.CurrentPath); err != nil {
		t.Fatalf("seed the current symlink: %v", err)
	}

	recipe := deploy.RecipeForTest("window", []deploy.Task{
		{ID: deploy.TaskRelease, Core: deploy.CoreRelease},
		{ID: deploy.TaskMaintenanceEnable, Command: "mkdir -p {{current_path}}/var && : > {{current_path}}/var/.maintenance.flag"},
		// The migration is what the window is for, so it asserts the property
		// rather than trusting the plan's shape.
		{ID: deploy.TaskDBMigrate, Command: "test -f {{current_path}}/var/.maintenance.flag"},
		{ID: deploy.TaskActivate, Core: deploy.CoreActivate},
		{ID: deploy.TaskMaintenanceDisable, Command: "rm -f {{current_path}}/var/.maintenance.flag"},
		{ID: deploy.TaskRecord, Core: deploy.CoreRecord},
	})

	plan, err := deploy.BuildPlanForTest(recipe, nil, "local")
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	plan = plan.ForPublishStrategy(deploy.PublishSymlink)

	executor := deploy.NewExecutor(host, deploy.Options{
		Remote:         "local",
		Publish:        deploy.PublishSymlink,
		CommandTimeout: time.Minute,
	}, io.Discard)
	// The variable set is the real one: `current_path` is what the window's
	// commands address, and the executor layers the per-step `release_path` on top.
	vars := cmd.DeployVarsForTest(host, deploy.Options{Remote: "local", Publish: deploy.PublishSymlink})
	if _, err := executor.Run(context.Background(), plan, vars, deploy.NewReleaseForTest("2", "abc123", "main")); err != nil {
		t.Fatalf("the window must protect the served release: %v", err)
	}

	if _, err := os.Stat(filepath.Join(live, "var", ".maintenance.flag")); err == nil {
		t.Fatal("a flag left in the release the swap replaced takes the site down again on a rollback")
	}
	target, err := os.Readlink(host.CurrentPath)
	if err != nil {
		t.Fatalf("the activation must leave a symlink: %v", err)
	}
	if filepath.Base(target) != "2" {
		t.Fatalf("current must point at the new release, got %q", target)
	}
	if _, err := os.Stat(filepath.Join(deployPath, "releases", "2", "var", ".maintenance.flag")); err == nil {
		t.Fatal("the release that is now live must not be in maintenance")
	}
}
