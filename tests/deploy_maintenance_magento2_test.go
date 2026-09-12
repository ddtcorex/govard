package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/frameworks/magento2"
)

// maintenanceStub is a `bin/magento` that behaves like the real one for the two
// maintenance commands: the flag is created in the working directory, which is
// what makes "where did the step run" observable.
const maintenanceStub = "#!/bin/sh\n" +
	"case \"$1\" in\n" +
	"  maintenance:enable) mkdir -p var && : > var/.maintenance.flag ;;\n" +
	"  maintenance:disable) rm -f var/.maintenance.flag ;;\n" +
	"esac\n"

// The maintenance flag is read by the application the web server is serving, so
// the window has to be opened and closed in *that* directory.
//
// Running `maintenance:enable` in the release being built opens a window nothing
// serves: the live release keeps answering requests while `setup:upgrade` changes
// the schema its code depends on, and the incoming release carries a flag that
// switches the site off the moment the swap lands — the opposite of the window's
// purpose. Executed against a stub, because which directory a step runs in is a
// property of the command line, not of the template text.
func TestMagento2MaintenanceModeIsSetOnTheServedRelease(t *testing.T) {
	deployPath := t.TempDir()
	release := filepath.Join(deployPath, "releases", "2")
	served := filepath.Join(deployPath, "public_html")

	for _, dir := range []string{release, served} {
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", "magento"), []byte(maintenanceStub), 0o755); err != nil {
			t.Fatalf("write the stub in %s: %v", dir, err)
		}
	}

	recipe := magento2.DeployRecipe()
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{"php_bin": "sh"},
	})
	host := deploy.HostForTest(deployPath, deploy.LocalRunner{})
	host.CurrentPath = served
	vars := cmd.DeployVarsForTest(host, options).SetPath("release_path", release)

	run := func(id string) {
		t.Helper()
		command, err := vars.Expand(recipe.Task(id).Command)
		if err != nil {
			t.Fatalf("expand %s: %v", id, err)
		}
		if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
			t.Fatalf("%s must run in the served directory: %v\n%s", id, err, command)
		}
	}

	run(deploy.TaskMaintenanceEnable)
	if _, err := os.Stat(filepath.Join(served, "var", ".maintenance.flag")); err != nil {
		t.Fatalf("the served release must be in maintenance while the window is open: %v", err)
	}
	if _, err := os.Stat(filepath.Join(release, "var", ".maintenance.flag")); err == nil {
		t.Fatal("the release being built must not carry the flag: it would switch the site off when it goes live")
	}

	run(deploy.TaskMaintenanceDisable)
	if _, err := os.Stat(filepath.Join(served, "var", ".maintenance.flag")); err == nil {
		t.Fatal("the served release must leave maintenance when the window closes")
	}
}

// A first deploy has no served release yet. The window must not turn that into a
// failed step, and it must not fall back to the release being built: there is
// nothing to protect, so nothing is written.
func TestMagento2MaintenanceWindowIsANoOpWithoutAServedRelease(t *testing.T) {
	deployPath := t.TempDir()
	release := filepath.Join(deployPath, "releases", "1")
	if err := os.MkdirAll(filepath.Join(release, "bin"), 0o755); err != nil {
		t.Fatalf("mkdir the release: %v", err)
	}
	if err := os.WriteFile(filepath.Join(release, "bin", "magento"), []byte(maintenanceStub), 0o755); err != nil {
		t.Fatalf("write the stub: %v", err)
	}

	recipe := magento2.DeployRecipe()
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{"php_bin": "sh"},
	})
	host := deploy.HostForTest(deployPath, deploy.LocalRunner{})
	host.CurrentPath = filepath.Join(deployPath, "public_html") // never created: a first deploy
	vars := cmd.DeployVarsForTest(host, options).SetPath("release_path", release)

	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		command, err := vars.Expand(recipe.Task(id).Command)
		if err != nil {
			t.Fatalf("expand %s: %v", id, err)
		}
		if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
			t.Fatalf("%s must be a no-op without a served release: %v\n%s", id, err, command)
		}
		if _, err := os.Stat(filepath.Join(release, "var", ".maintenance.flag")); err == nil {
			t.Fatalf("%s wrote a flag into the release being built; nothing serves it, and the next activation would take the site down", id)
		}
	}
}

// The Magento recipe imports configuration and upgrades the schema on every
// deploy, so its plan always needs the window and a symlink deploy to a Magento
// target always opens one. That is a deliberate answer, not an oversight: the
// alternative is probing the target (`app:config:status`, `setup:db:status`) and
// trusting the probe, and the reference deploy tool's own recipe records a case
// the probe misses — a newly configured message queue needs a full upgrade that
// `setup:db:status` does not report, which is why its "full upgrade needed" hook
// is left hardcoded to false with a TODO.
//
// The cost is the migration window on every symlink deploy; the guarantee is that
// a schema change is never visible to the release still serving traffic. A deploy
// that must keep the window short bounds it with `deploy.maintenance_timeout`, and
// a database dump — the one step inside it that can take minutes — is opt-in.
func TestMagento2SymlinkDeployAlwaysTakesTheMaintenanceWindow(t *testing.T) {
	plan, err := deploy.BuildPlanForTest(magento2.DeployRecipe(), nil, "staging")
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	symlink := plan.ForPublishStrategy(deploy.PublishSymlink)

	// The premise: without both of these the recipe would skip the window, and
	// this test would be asserting nothing.
	for _, id := range []string{deploy.TaskAppConfigure, deploy.TaskDBMigrate} {
		if !symlink.Runs(id) {
			t.Fatalf("the recipe must implement %s for the window question to be about Magento", id)
		}
	}
	for _, id := range []string{deploy.TaskMaintenanceEnable, deploy.TaskMaintenanceDisable} {
		if symlink.Steps[symlink.IndexOf(id)].Skipped {
			t.Fatalf("%s is skipped although the plan imports configuration and upgrades the schema: the window would not protect the live release", id)
		}
	}
}
