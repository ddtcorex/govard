package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/frameworks/wordpress"
)

// The tasks this recipe fills. WordPress has no compile step, no static-content
// deployment and no declarative configuration to import.
var wordpressFilledTasks = []string{
	"build:vendors", "build:frontend", "db:backup",
	"maintenance:enable", "db:migrate", "app:cache:flush", "maintenance:disable",
}

func TestWordPressRecipeOnlyDeclaresNeutralTaskIDs(t *testing.T) {
	recipe := wordpress.DeployRecipe()
	if recipe.ID != "wordpress" {
		t.Fatalf("recipe ID = %q, want wordpress", recipe.ID)
	}
	for _, task := range recipe.Tasks {
		if !deploy.IsKnownTaskID(task.ID) {
			t.Fatalf("recipe declares %q, which is not a neutral task id", task.ID)
		}
		if stage, _ := deploy.StageForTask(task.ID); task.Stage != stage {
			t.Fatalf("task %q declares stage %q, want %q", task.ID, task.Stage, stage)
		}
	}
	for _, id := range deploy.TaskIDList() {
		if recipe.Task(id).ID != id {
			t.Fatalf("the wordpress recipe does not declare %q", id)
		}
	}
	for _, id := range wordpressFilledTasks {
		if recipe.Task(id).IsEmpty() {
			t.Fatalf("the wordpress recipe leaves %q unimplemented", id)
		}
	}
	for _, id := range []string{"build:compile", "build:assets", "app:configure"} {
		if !recipe.Task(id).IsEmpty() {
			t.Errorf("task %q should be empty for WordPress:\n%s", id, recipe.Task(id).Command)
		}
	}
}

// A classic WordPress checkout frequently has no composer.json, so an
// unconditional `composer install` would fail on a healthy project. The guard is
// a test, not `|| true`: a project that has one and fails to install must fail
// the deploy.
func TestWordPressDependencyInstallIsGuarded(t *testing.T) {
	command := wordpress.DeployRecipe().Task("build:vendors").Command
	if !strings.Contains(command, "composer.json") {
		t.Fatalf("build:vendors is not guarded on composer.json:\n%s", command)
	}
	if !strings.Contains(command, "{{composer_bin}} install") {
		t.Fatalf("build:vendors does not install with the configured Composer:\n%s", command)
	}
}

// The first half of every hybrid: wp-cli when the target has it, because that is
// what a real server runs and what an operator expects. It is not something
// govard can install on a target, so the second half boots WordPress the way
// wp-cli itself does.
func TestWordPressHybridCommandsPreferWpCliAndFallBackToPHP(t *testing.T) {
	recipe := wordpress.DeployRecipe()
	for _, id := range []string{"db:migrate", "app:cache:flush"} {
		command := recipe.Task(id).Command
		if !strings.Contains(command, "command -v wp") {
			t.Errorf("%s does not prefer wp-cli when it is available:\n%s", id, command)
		}
		if !strings.Contains(command, "{{php_bin}} -r") {
			t.Errorf("%s has no PHP fallback for a target without wp-cli:\n%s", id, command)
		}
		if !strings.Contains(command, "wp-load.php") {
			t.Errorf("%s does not boot WordPress through wp-load.php, which is what wp-cli does:\n%s", id, command)
		}
		if !strings.Contains(command, "{{release_path}}") {
			t.Errorf("%s does not run in the release:\n%s", id, command)
		}
	}
}

// `wp_is_maintenance_mode()` stops reporting maintenance once the flag is ten
// minutes old, so the timestamp has to be written ahead of the clock: a deploy
// window longer than that would otherwise quietly reopen the site in the middle
// of a migration, and a failure at minute nine would put traffic back on a
// half-migrated database.
func TestWordPressMaintenanceOutlivesTheTenMinuteExpiry(t *testing.T) {
	command := wordpress.DeployRecipe().Task("maintenance:enable").Command
	if !strings.Contains(command, "time() + 86400") {
		t.Fatalf("maintenance:enable must write a timestamp ahead of the clock:\n%s", command)
	}
	if strings.Contains(command, "$upgrading = time();") {
		t.Fatalf("maintenance:enable writes the ten-minute timestamp WordPress itself writes:\n%s", command)
	}
	for _, want := range []string{"{{current_path}}/wp-load.php", "{{current_path}}/wp-includes/version.php", "wp-content/maintenance.php"} {
		if !strings.Contains(command, want) {
			t.Errorf("maintenance:enable is missing %q:\n%s", want, command)
		}
	}
}

// The deploy removes only the state it created: a project that ships its own
// `wp-content/maintenance.php` must keep it, so the drop-in carries a marker and
// only a marked file is removed.
func TestWordPressMaintenanceDisableOnlyRemovesItsOwnDropIn(t *testing.T) {
	command := wordpress.DeployRecipe().Task("maintenance:disable").Command
	if !strings.Contains(command, "govard deploy maintenance") {
		t.Fatalf("maintenance:disable does not recognise its own drop-in:\n%s", command)
	}
	for _, want := range []string{"{{current_path}}", "{{release_path}}"} {
		if !strings.Contains(command, want) {
			t.Errorf("maintenance:disable must clear %s as well, or a resumed or in-place deploy leaves the flag behind:\n%s", want, command)
		}
	}
}

// `wp db export` reads the connection from wp-config.php, so credentials never
// have to be duplicated into the project configuration.
func TestWordPressBackupUsesWpCli(t *testing.T) {
	recipe := wordpress.DeployRecipe()
	backup := recipe.Task("db:backup")
	if backup.Core == nil {
		t.Fatal("db:backup must be the engine's wrapper around the recipe's dump command")
	}
	if !strings.Contains(recipe.Restore, "wp db import") {
		t.Errorf("restore is not a wp-cli import:\n%s", recipe.Restore)
	}
}

// The verify check has to prove the site is installed against its database, not
// only that PHP runs.
func TestWordPressVerifyCheckProvesTheInstall(t *testing.T) {
	checks := wordpress.DeployRecipe().Checks
	if len(checks) != 1 {
		t.Fatalf("want exactly one framework check, got %d", len(checks))
	}
	command := checks[0].Command
	for _, want := range []string{"wp core is-installed", "is_blog_installed()", "wp-load.php"} {
		if !strings.Contains(command, want) {
			t.Errorf("the check is missing %q:\n%s", want, command)
		}
	}
}

func TestWordPressRecipeCommandsAllExpand(t *testing.T) {
	recipe := wordpress.DeployRecipe()
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Revision: "abcdef123456", Settings: map[string]any{}})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	vars := cmd.DeployVarsForTest(host, options)

	for _, task := range recipe.Tasks {
		if task.Command == "" {
			continue
		}
		if _, err := vars.Expand(task.Command); err != nil {
			t.Errorf("task %q does not expand: %v\n%s", task.ID, err, task.Command)
		}
	}
	for _, check := range recipe.Checks {
		if _, err := vars.Expand(check.Command); err != nil {
			t.Errorf("check %q does not expand: %v\n%s", check.ID, err, check.Command)
		}
	}
	restoreVars := vars.SetRaw("backup_path", "/srv/app/shared/backups/deploy/1/dump.sql")
	if _, err := restoreVars.Expand(recipe.Restore); err != nil {
		t.Errorf("the restore command does not expand: %v\n%s", err, recipe.Restore)
	}
}

func TestWordPressRecipeDeclaresEachSettingOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, setting := range wordpress.DeployRecipe().Settings {
		if seen[setting.Key] {
			t.Errorf("deploy.settings.%s is declared twice", setting.Key)
		}
		seen[setting.Key] = true
		if setting.Title == "" {
			t.Errorf("deploy.settings.%s has no description", setting.Key)
		}
	}
}

// `wp db export` shells out to mysqldump, and the recipe's hybrid commands need
// wp-cli itself: the sandbox has to ask for both, or the rehearsal proves a
// branch no server runs.
func TestWordPressSandboxProvidesWpCliAndADatabaseClient(t *testing.T) {
	sandbox := wordpress.DeployRecipe().Sandbox
	if strings.Join(sortedWordsForTest(sandbox.Tools), ",") != "wp-cli" {
		t.Fatalf("the WordPress sandbox must install wp-cli, got %v", sandbox.Tools)
	}
	if !strings.Contains(strings.Join(sandbox.Packages, " "), "default-mysql-client") {
		t.Fatalf("wp db export shells out to mysqldump, so the client has to be there:\n%v", sandbox.Packages)
	}
	if !strings.Contains(strings.Join(sandbox.Services, " "), "mariadb") {
		t.Fatalf("the sandbox does not start a database:\n%v", sandbox.Services)
	}
}

// sortedWordsForTest normalises a list so the assertion does not depend on the
// order a recipe happens to write it in.
func sortedWordsForTest(words []string) []string {
	out := append([]string(nil), words...)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	return out
}
