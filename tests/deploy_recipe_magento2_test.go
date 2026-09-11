package tests

import (
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/frameworks/magento2"
)

// recipeTaskIDs lists the neutral ids the Magento 2 recipe is expected to fill.
var magento2FilledTasks = []string{
	"build:vendors", "build:patches", "build:compile", "build:frontend", "build:assets",
	"maintenance:enable", "app:workers:pause", "db:backup", "app:configure", "db:migrate",
	"app:cache:flush", "app:workers:resume", "maintenance:disable",
}

func TestMagento2RecipeOnlyDeclaresNeutralTaskIDs(t *testing.T) {
	recipe := magento2.DeployRecipe()
	if recipe.ID != "magento2" {
		t.Fatalf("recipe ID = %q, want magento2", recipe.ID)
	}
	for _, task := range recipe.Tasks {
		if !deploy.IsKnownTaskID(task.ID) {
			t.Fatalf("recipe declares %q, which is not a neutral task id", task.ID)
		}
		if stage, _ := deploy.StageForTask(task.ID); task.Stage != stage {
			t.Fatalf("task %q declares stage %q, want %q (a recipe does not choose the stage)", task.ID, task.Stage, stage)
		}
	}
	// The neutral lifecycle is the contract; a recipe fills it, it does not
	// shorten it.
	for _, id := range deploy.TaskIDList() {
		if recipe.Task(id).ID != id {
			t.Fatalf("the magento2 recipe does not declare %q", id)
		}
	}
}

func TestMagento2RecipeFillsTheFrameworkSpecificTasks(t *testing.T) {
	recipe := magento2.DeployRecipe()

	// The engine implements these itself; a recipe must not shadow them.
	for _, id := range []string{"deploy:check", "deploy:lock", "deploy:release", "deploy:code",
		"deploy:shared", "deploy:writable", "publish:activate", "deploy:record", "deploy:verify",
		"deploy:cleanup", "deploy:unlock"} {
		if recipe.Task(id).Command != "" {
			t.Fatalf("task %q has a recipe command %q; it is implemented in the engine", id, recipe.Task(id).Command)
		}
	}

	for _, id := range magento2FilledTasks {
		task := recipe.Task(id)
		if task.IsEmpty() {
			t.Fatalf("the magento2 recipe leaves %q unimplemented", id)
		}
	}
}

func TestMagento2RecipeCommandsEnterTheReleaseDirectory(t *testing.T) {
	recipe := magento2.DeployRecipe()

	for _, task := range recipe.Tasks {
		if task.Command == "" {
			continue
		}
		if !strings.Contains(task.Command, "{{release_path}}") {
			t.Errorf("task %q does not reference {{release_path}}; each step runs as a fresh remote command:\n%s", task.ID, task.Command)
		}
	}
}

func TestMagento2RecipeUsesTheConfiguredPHPBinary(t *testing.T) {
	recipe := magento2.DeployRecipe()

	for _, id := range []string{"build:compile", "build:assets", "app:configure", "db:migrate", "app:cache:flush"} {
		if command := recipe.Task(id).Command; !strings.Contains(command, "{{php_bin}} bin/magento") {
			t.Errorf("task %q does not run bin/magento through {{php_bin}}:\n%s", id, command)
		}
	}
}

func TestMagento2RecipeStaticContentIsDeterministic(t *testing.T) {
	command := magento2.DeployRecipe().Task("build:assets").Command

	for _, want := range []string{
		"setup:static-content:deploy",
		"-f",
		"--content-version={{settings.content_version}}",
		"-j {{settings.static_jobs}}",
		"{{settings.static_content_locales_args}}",
		"{{settings.magento_themes_args}}",
	} {
		if !strings.Contains(command, want) {
			t.Errorf("build:assets is missing %q:\n%s", want, command)
		}
	}
}

func TestMagento2RecipeSkipsStaticContentInDeveloperMode(t *testing.T) {
	command := magento2.DeployRecipe().Task("build:assets").Command

	if !strings.Contains(command, `"{{settings.mage_mode}}" != "developer"`) {
		t.Fatalf("build:assets does not skip developer mode, where static files are generated on demand:\n%s", command)
	}
}

func TestMagento2RecipePatchesStepIsGuardedNotSwallowed(t *testing.T) {
	command := magento2.DeployRecipe().Task("build:patches").Command

	if !strings.Contains(command, "ece-patches") {
		t.Fatalf("build:patches does not apply ece-patches:\n%s", command)
	}
	if strings.Contains(command, "|| true") {
		t.Fatalf("build:patches swallows its own failure:\n%s", command)
	}
}

func TestMagento2RecipeBackupWritesThroughTheEngine(t *testing.T) {
	task := magento2.DeployRecipe().Task("db:backup")

	if task.Command != "" {
		t.Fatalf("db:backup has a shell command %q; the engine owns the path and the record", task.Command)
	}
	if task.Core == nil {
		t.Fatal("db:backup has no core implementation")
	}
	if !strings.Contains(magento2.DeployRecipe().Restore, "{{backup_path}}") {
		t.Fatal("the recipe's restore command does not reference {{backup_path}}")
	}
}

func TestMagento2RecipeDefaultsCoverTheSharedLayout(t *testing.T) {
	defaults := magento2.DeployRecipe().Defaults

	for _, key := range []string{"shared_files", "shared_dirs", "writable_dirs", "sync_paths"} {
		value, ok := defaults[key]
		if !ok {
			t.Fatalf("defaults do not declare %q", key)
		}
		if _, isList := value.([]string); !isList {
			t.Fatalf("default %q is %T, want []string", key, value)
		}
	}

	shared := strings.Join(defaults["shared_files"].([]string), " ")
	for _, want := range []string{"app/etc/env.php", "var/.maintenance.ip"} {
		if !strings.Contains(shared, want) {
			t.Errorf("shared_files is missing %q: %s", want, shared)
		}
	}

	dirs := strings.Join(defaults["shared_dirs"].([]string), " ")
	for _, want := range []string{"pub/media", "var/log", "pub/static/_cache"} {
		if !strings.Contains(dirs, want) {
			t.Errorf("shared_dirs is missing %q: %s", want, dirs)
		}
	}
}

func TestMagento2RecipeDeclaresArgumentListsAsArgsSpecs(t *testing.T) {
	defaults := magento2.DeployRecipe().Defaults

	locales, ok := defaults["static_content_locales"].(deploy.ArgsSpec)
	if !ok || locales.Flag != "--language" {
		t.Fatalf("static_content_locales default = %#v, want ArgsSpec{Flag: --language}", defaults["static_content_locales"])
	}
	themes, ok := defaults["magento_themes"].(deploy.ArgsSpec)
	if !ok || themes.Flag != "-t" {
		t.Fatalf("magento_themes default = %#v, want ArgsSpec{Flag: -t}", defaults["magento_themes"])
	}
}

// Every command a recipe ships must expand against the variables the command
// layer actually defines, with nothing configured but the defaults. A template
// that references an undefined variable is a deploy that dies mid-pipeline.
func TestMagento2RecipeCommandsAllExpand(t *testing.T) {
	recipe := magento2.DeployRecipe()

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{},
	})
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
	// The restore command additionally references {{backup_path}}, which the
	// engine supplies from the release record at restore time.
	restoreVars := vars.SetRaw("backup_path", "/srv/app/shared/backups/deploy/1/dump.sql")
	if _, err := restoreVars.Expand(recipe.Restore); err != nil {
		t.Errorf("the restore command does not expand: %v\n%s", err, recipe.Restore)
	}
}
