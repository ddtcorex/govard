package tests

import (
	"slices"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/frameworks/laravel"
)

// The tasks this recipe fills. `build:compile` and `build:assets` are absent on
// purpose: Laravel compiles nothing ahead of time, and its caches are built on
// the target, where the environment they bake in actually holds.
var laravelFilledTasks = []string{
	"build:vendors", "build:frontend", "app:configure",
	"maintenance:enable", "app:workers:pause", "db:migrate",
	"app:cache:flush", "maintenance:disable",
}

func TestLaravelRecipeOnlyDeclaresNeutralTaskIDs(t *testing.T) {
	recipe := laravel.DeployRecipe()
	if recipe.ID != "laravel" {
		t.Fatalf("recipe ID = %q, want laravel", recipe.ID)
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
			t.Fatalf("the laravel recipe does not declare %q", id)
		}
	}
}

func TestLaravelRecipeLeavesTheEngineTasksToTheEngine(t *testing.T) {
	recipe := laravel.DeployRecipe()

	for _, id := range []string{"deploy:check", "deploy:lock", "deploy:release", "deploy:code",
		"deploy:shared", "deploy:writable", "publish:activate", "deploy:record", "deploy:verify",
		"deploy:cleanup", "deploy:unlock"} {
		if recipe.Task(id).Command != "" {
			t.Fatalf("task %q has a recipe command %q; the engine implements it", id, recipe.Task(id).Command)
		}
	}

	for _, id := range laravelFilledTasks {
		if recipe.Task(id).IsEmpty() {
			t.Fatalf("the laravel recipe leaves %q unimplemented", id)
		}
	}
	// Laravel has no compile step and no static-content deployment.
	for _, id := range []string{"build:compile", "build:assets"} {
		if !recipe.Task(id).IsEmpty() {
			t.Errorf("task %q should be empty for Laravel:\n%s", id, recipe.Task(id).Command)
		}
	}
}

// Every step runs as a fresh remote command, so it enters the absolute directory
// it acts on: the release being built, or the *current* path for maintenance,
// which belongs to the application the web server is serving.
func TestLaravelRecipeCommandsEnterTheDirectoryTheyActOn(t *testing.T) {
	for _, task := range laravel.DeployRecipe().Tasks {
		if task.Command == "" {
			continue
		}
		if !strings.Contains(task.Command, "{{release_path}}") && !strings.Contains(task.Command, "{{current_path}}") {
			t.Errorf("task %q enters no directory; each step runs as a fresh remote command:\n%s", task.ID, task.Command)
		}
	}
}

// A template that references an undefined variable dies mid-pipeline, and the
// shell reports the empty word rather than the typo.
func TestLaravelRecipeCommandsAllExpand(t *testing.T) {
	recipe := laravel.DeployRecipe()

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
	for _, check := range recipe.Checks {
		if _, err := vars.Expand(check.Command); err != nil {
			t.Errorf("check %q does not expand: %v\n%s", check.ID, err, check.Command)
		}
	}
}

func TestLaravelRecipeDeclaresEachSettingOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, setting := range laravel.DeployRecipe().Settings {
		if seen[setting.Key] {
			t.Errorf("deploy.settings.%s is declared twice", setting.Key)
		}
		seen[setting.Key] = true
		if setting.Title == "" {
			t.Errorf("deploy.settings.%s has no description", setting.Key)
		}
	}
}

// Maintenance mode attaches to the application the web server is serving, and
// the guard has to require a working application: a docroot with no autoloader
// has nothing to protect and must not receive the flag. Laravel writes the flag
// into `storage/framework/down`, which is inside a shared directory, so it
// survives the release swap.
func TestLaravelMaintenanceGuardsOnTheServedApplication(t *testing.T) {
	recipe := laravel.DeployRecipe()
	for _, id := range []string{"maintenance:enable", "maintenance:disable"} {
		command := recipe.Task(id).Command
		for _, want := range []string{"{{current_path}}/artisan", "{{current_path}}/vendor/autoload.php"} {
			if !strings.Contains(command, want) {
				t.Errorf("%s does not guard on %s:\n%s", id, want, command)
			}
		}
		if strings.Contains(command, "{{release_path}}") {
			t.Errorf("%s must run against the served application, not the release being built:\n%s", id, command)
		}
	}
	if shared, ok := recipe.Defaults["shared_dirs"].([]string); !ok || !slices.Contains(shared, "storage") {
		t.Errorf("storage must be shared, or the maintenance flag does not survive the swap: %v", recipe.Defaults["shared_dirs"])
	}
}

func TestLaravelMigrationForcesNonInteractiveMode(t *testing.T) {
	command := laravel.DeployRecipe().Task("db:migrate").Command
	if !strings.Contains(command, "artisan migrate --force") {
		t.Fatalf("db:migrate must pass --force: a production deploy cannot answer a prompt:\n%s", command)
	}
	if !strings.Contains(command, "{{php_bin}}") {
		t.Fatalf("db:migrate must use the configured PHP binary:\n%s", command)
	}
}

// `storage:link` exits 0 when the link already exists (measured: the second run
// prints an error and still returns 0), so the step is safe on a re-run and on
// an in-place docroot that already has the link.
func TestLaravelConfigureLinksThePublicStorageDisk(t *testing.T) {
	command := laravel.DeployRecipe().Task("app:configure").Command
	if !strings.Contains(command, "storage:link") {
		t.Fatalf("app:configure must create the public storage link, or a fresh release serves no media:\n%s", command)
	}
	if !strings.Contains(command, "{{release_path}}") {
		t.Fatalf("app:configure must run inside the release:\n%s", command)
	}
}

// The check has to reach the database, and it must not fail a healthy project
// that has no migrations: `migrate:status` exits 1 when the migrations table is
// absent, while `db:show` reports the connection and exits 0. `about` passes
// with no database at all, which is why it is not used.
func TestLaravelVerifyCheckProvesTheDatabaseConnection(t *testing.T) {
	checks := laravel.DeployRecipe().Checks
	if len(checks) != 1 {
		t.Fatalf("want exactly one framework check, got %d", len(checks))
	}
	command := checks[0].Command
	if !strings.Contains(command, "db:show") {
		t.Errorf("the check must use db:show:\n%s", command)
	}
	if !strings.Contains(command, "migrate:status") {
		t.Errorf("the check must fall back for a Laravel without db:show:\n%s", command)
	}
	if !strings.Contains(command, "artisan list --raw") {
		t.Errorf("the fallback has to be chosen by asking artisan, not by guessing a version:\n%s", command)
	}
	if checks[0].ID != "app" || checks[0].Title == "" {
		t.Errorf("the check needs an id and a description: %+v", checks[0])
	}
	// The application that answers is the one the web server serves: on an
	// in-place target the docroot is a different directory from the release the
	// build ran in, so the check has to run there — `artisan` resolves through
	// the current working directory.
	if !strings.Contains(command, "{{current_path}}") {
		t.Errorf("the check must run in the served path:\n%s", command)
	}
	if strings.Contains(command, "{{release_path}}") {
		t.Errorf("the served application is not the release on an in-place target:\n%s", command)
	}
}

// The cache build belongs to the target: `artisan optimize` writes
// `bootstrap/cache/config.php`, and after that the environment no longer
// overrides `.env` — measured by pointing a cached release at a dead database
// port and watching it still connect.
func TestLaravelCachesAreBuiltOnTheTarget(t *testing.T) {
	recipe := laravel.DeployRecipe()
	flush := recipe.Task("app:cache:flush").Command
	for _, want := range []string{"optimize:clear", "artisan optimize"} {
		if !strings.Contains(flush, want) {
			t.Errorf("app:cache:flush is missing %q:\n%s", want, flush)
		}
	}
	for _, id := range []string{"build:vendors", "build:frontend", "app:configure"} {
		if strings.Contains(recipe.Task(id).Command, "artisan optimize") {
			t.Errorf("%s builds the framework caches; that is the target's job:\n%s", id, recipe.Task(id).Command)
		}
	}
}

// The worker stop is Laravel's own graceful signal. It exits 0 with any cache
// store, so the recipe cannot detect a project whose cache would swallow it —
// the docs say the store has to be persistent.
func TestLaravelWorkerPauseIsGuardedAndGraceful(t *testing.T) {
	command := laravel.DeployRecipe().Task("app:workers:pause").Command
	if !strings.Contains(command, "{{settings.worker_control}} = true") {
		t.Fatalf("app:workers:pause must be guarded by worker_control:\n%s", command)
	}
	if !strings.Contains(command, "queue:restart") {
		t.Fatalf("app:workers:pause must send the graceful restart signal:\n%s", command)
	}
}

func TestLaravelSandboxRequirementsCoverTheFramework(t *testing.T) {
	sandbox := laravel.DeployRecipe().Sandbox
	extensions := strings.Join(sandbox.Extensions, " ")
	for _, want := range []string{"mbstring", "curl", "xml", "zip", "mysql", "sqlite3"} {
		if !strings.Contains(extensions, want) {
			t.Errorf("the sandbox is missing the %s extension:\n%s", want, extensions)
		}
	}
	services := strings.Join(sandbox.Services, " ")
	for _, want := range []string{"mariadb", "redis-server"} {
		if !strings.Contains(services, want) {
			t.Errorf("the sandbox does not start %s:\n%s", want, services)
		}
	}
}
