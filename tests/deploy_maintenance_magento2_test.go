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
