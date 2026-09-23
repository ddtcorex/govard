package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"govard/internal/cmd"
	"govard/internal/deploy"
	"govard/internal/frameworks/symfony"
)

// The tasks this recipe fills. Symfony has no core maintenance mode, no compile
// step and no declarative configuration to import, so those stay empty and the
// engine reports them as skipped.
var symfonyFilledTasks = []string{
	"build:vendors", "build:assets", "build:frontend",
	"db:migrate", "app:cache:flush", "app:workers:pause",
}

func TestSymfonyRecipeOnlyDeclaresNeutralTaskIDs(t *testing.T) {
	recipe := symfony.DeployRecipe()
	if recipe.ID != "symfony" {
		t.Fatalf("recipe ID = %q, want symfony", recipe.ID)
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
			t.Fatalf("the symfony recipe does not declare %q", id)
		}
	}
	for _, id := range symfonyFilledTasks {
		if recipe.Task(id).IsEmpty() {
			t.Fatalf("the symfony recipe leaves %q unimplemented", id)
		}
	}
}

// Composer runs the package's scripts on install, and a Symfony project puts
// `cache:clear` and `assets:install` there. Both have to happen where the
// application lives: a cache warmed for the build machine is worthless, and the
// asset links resolve against the vendor tree that is actually present.
func TestSymfonyComposerInstallDoesNotRunTheApplicationScripts(t *testing.T) {
	command := symfony.DeployRecipe().Task("build:vendors").Command
	if !strings.Contains(command, "--no-scripts") {
		t.Fatalf("build:vendors must pass --no-scripts: auto-scripts runs cache:clear and assets:install on the builder:\n%s", command)
	}
	if !strings.Contains(command, "{{composer_bin}} install") || !strings.Contains(command, "{{release_path}}") {
		t.Fatalf("build:vendors must install inside the release with the configured Composer:\n%s", command)
	}
}

// `public/bundles` is gitignored in a real project and produced by
// assets:install, so the step belongs where the application is. `--relative`
// makes the links resolve from wherever the docroot ends up, which an in-place
// deploy needs: measured on the real project, the link is written as
// `etl -> ../../vendor/sutunam/etl-bundle/src/Resources/public/`.
func TestSymfonyAssetInstallRunsOnTheTargetWithRelativeLinks(t *testing.T) {
	task := symfony.DeployRecipe().Task("build:assets")
	if !task.NeedsApplication {
		t.Fatal("assets:install must be marked NeedsApplication")
	}
	for _, want := range []string{"assets:install", "--symlink", "--relative", "{{release_path}}"} {
		if !strings.Contains(task.Command, want) {
			t.Errorf("build:assets is missing %q:\n%s", want, task.Command)
		}
	}
}

// An empty migrations directory is a healthy project — the real one has one —
// and without the flag Doctrine exits non-zero on it.
func TestSymfonyMigrationsAllowAnEmptyDirectory(t *testing.T) {
	command := symfony.DeployRecipe().Task("db:migrate").Command
	for _, want := range []string{"doctrine:migrations:migrate", "--allow-no-migration", "--no-interaction", "{{php_bin}}"} {
		if !strings.Contains(command, want) {
			t.Errorf("db:migrate is missing %q:\n%s", want, command)
		}
	}
}

// Which environment a deploy runs under is the deploy's decision: the real
// project's committed `.env` says `APP_ENV=dev`, which is a development default
// rather than an instruction to production.
func TestSymfonyCachesUseTheConfiguredEnvironment(t *testing.T) {
	recipe := symfony.DeployRecipe()
	command := recipe.Task("app:cache:flush").Command
	for _, want := range []string{"cache:clear", "cache:warmup", "{{settings.symfony_env}}"} {
		if !strings.Contains(command, want) {
			t.Errorf("app:cache:flush is missing %q:\n%s", want, command)
		}
	}
	if recipe.Defaults["symfony_env"] != "prod" {
		t.Errorf("symfony_env defaults to %v, want prod", recipe.Defaults["symfony_env"])
	}
	declared := false
	for _, setting := range recipe.Settings {
		if setting.Key == "symfony_env" {
			declared = true
		}
	}
	if !declared {
		t.Error("symfony_env must be declared, or a project cannot set it")
	}
}

// The check has to reach the database, and it has to exist in the console the
// project actually has: DoctrineBundle renamed `doctrine:query:sql` to
// `dbal:run-sql`, so the recipe asks which one is there rather than guessing.
func TestSymfonyVerifyCheckProvesTheDatabaseConnection(t *testing.T) {
	checks := symfony.DeployRecipe().Checks
	if len(checks) != 1 {
		t.Fatalf("want exactly one framework check, got %d", len(checks))
	}
	command := checks[0].Command
	if !strings.Contains(command, "dbal:run-sql") {
		t.Errorf("the check must use dbal:run-sql:\n%s", command)
	}
	if !strings.Contains(command, "doctrine:query:sql") {
		t.Errorf("the check must fall back to doctrine:query:sql for an older DoctrineBundle:\n%s", command)
	}
	if !strings.Contains(command, "bin/console list --raw") {
		t.Errorf("the branch has to be chosen by asking the console, not by guessing a version:\n%s", command)
	}
	if !strings.Contains(command, "SELECT 1") {
		t.Errorf("the check must run a query, or it proves only that the kernel boots:\n%s", command)
	}
}

// Symfony has no core maintenance mechanism. An empty task is skipped by the
// engine; a fabricated one would be worse, because it would look like a window.
func TestSymfonyDeclaresNoMaintenanceMode(t *testing.T) {
	recipe := symfony.DeployRecipe()
	for _, id := range []string{"maintenance:enable", "maintenance:disable"} {
		if !recipe.Task(id).IsEmpty() {
			t.Fatalf("%s is not empty; Symfony has no core maintenance mode, and a window that does not close is worse than none", id)
		}
	}
}

func TestSymfonyRecipeCommandsAllExpand(t *testing.T) {
	recipe := symfony.DeployRecipe()
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
}

func TestSymfonyRecipeDeclaresEachSettingOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, setting := range symfony.DeployRecipe().Settings {
		if seen[setting.Key] {
			t.Errorf("deploy.settings.%s is declared twice", setting.Key)
		}
		seen[setting.Key] = true
		if setting.Title == "" {
			t.Errorf("deploy.settings.%s has no description", setting.Key)
		}
	}
}

// `var/cache` is deliberately not shared: the compiled container is built per
// release and for one environment. `var/log` is, because a release is thrown
// away and its logs are not.
func TestSymfonySharesLogsAndBuiltPathsOnly(t *testing.T) {
	recipe := symfony.DeployRecipe()
	shared, _ := recipe.Defaults["shared_dirs"].([]string)
	for _, dir := range shared {
		if strings.HasPrefix(dir, "var/cache") {
			t.Fatalf("var/cache must not be shared between releases: %v", shared)
		}
	}
	if !strings.Contains(strings.Join(shared, " "), "var/log") {
		t.Errorf("var/log must be shared: %v", shared)
	}
	sync, _ := recipe.Defaults["sync_paths"].([]string)
	joined := strings.Join(sync, " ")
	for _, want := range []string{"vendor", "public/bundles"} {
		if !strings.Contains(joined, want) {
			t.Errorf("sync_paths is missing the built path %q: %v", want, sync)
		}
	}
}

func TestSymfonySandboxRequirementsCoverTheFramework(t *testing.T) {
	sandbox := symfony.DeployRecipe().Sandbox
	extensions := strings.Join(sandbox.Extensions, " ")
	for _, want := range []string{"intl", "mbstring", "xml", "zip"} {
		if !strings.Contains(extensions, want) {
			t.Errorf("the sandbox is missing the %s extension:\n%s", want, extensions)
		}
	}
	if !strings.Contains(strings.Join(sandbox.Services, " "), "mariadb") {
		t.Errorf("the default sandbox does not start a database:\n%v", sandbox.Services)
	}
}

// The restart signal `messenger:stop-workers` writes lives in a cache pool
// (`cache.app`), which by default is a filesystem adapter under
// `var/cache/<env>/pools` — and `var/cache` is deliberately *not* shared between
// releases. Written from the release, the signal sits in a directory no running
// consumer polls, and that release's own `cache:clear` then wipes it, so with
// `worker_control: true` the consumers keep working through
// `doctrine:migrations:migrate`. The pause has to run where the application is
// served.
func TestSymfonyWorkerPauseActsOnTheServedApplication(t *testing.T) {
	calls := symfonyWorkerCalls(t, map[string]any{"worker_control": true})
	if len(calls) == 0 {
		t.Fatal("app:workers:pause recorded no call")
	}
	for _, call := range calls {
		if strings.Contains(call, "/releases/") {
			t.Errorf("app:workers:pause ran in the release, not the served application: %s", call)
		}
		if !strings.Contains(call, "/current ") {
			t.Errorf("app:workers:pause did not run in the served root: %s", call)
		}
		if !strings.Contains(call, "messenger:stop-workers") {
			t.Errorf("app:workers:pause did not stop the consumers: %s", call)
		}
	}
}

// A docroot that cannot run the console has no consumers to signal: the step is
// skipped rather than failed, the same rule the Magento recipe applies. The stub
// is there so a weakened guard is caught — `console` exists and is runnable, only
// the autoloader is missing, and a guard that asked for the `vendor/` directory
// would run it and leave the marker behind.
func TestSymfonyWorkerPauseToleratesAnUnrunnableDocroot(t *testing.T) {
	recipe := symfony.DeployRecipe()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	marker := filepath.Join(t.TempDir(), "ran")
	writeFile(t, filepath.Join(host.CurrentPath, "bin", "console"), "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(filepath.Join(host.CurrentPath, "bin", "console"), 0o755); err != nil {
		t.Fatalf("chmod the stub: %v", err)
	}
	writeFile(t, filepath.Join(host.CurrentPath, "vendor", ".gitignore"), "!/autoload.php\n")

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{"worker_control": true, "php_bin": "sh"},
	})
	command, err := cmd.DeployVarsForTest(host, options).SetPath("release_path", t.TempDir()).
		Expand(recipe.Task(deploy.TaskWorkersPause).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("a docroot with no console must be skipped, not failed: %v\n%s", err, command)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("the guard accepted a docroot with no autoloader, so the pause ran:\n%s", command)
	}
}

// symfonyWorkerCalls runs app:workers:pause against a recording stub and returns
// one "<working directory> <arguments>" line per call.
func symfonyWorkerCalls(t *testing.T, settings map[string]any) []string {
	t.Helper()
	recipe := symfony.DeployRecipe()
	log := filepath.Join(t.TempDir(), "calls.log")

	settings["php_bin"] = "sh"
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Revision: "abcdef123456", Settings: settings})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := host.ReleasePath("1")
	vars := cmd.DeployVarsForTest(host, options).SetPath("release_path", release)

	stub := "#!/bin/sh\nprintf '%s\\n' \"$PWD $*\" >> " + log + "\n"
	// The stub exists in both directories; the recorded working directory is what
	// the test asserts.
	for _, root := range []string{release, host.CurrentPath} {
		writeFile(t, filepath.Join(root, "bin", "console"), stub)
		if err := os.Chmod(filepath.Join(root, "bin", "console"), 0o755); err != nil {
			t.Fatalf("chmod %s: %v", root, err)
		}
	}
	// The served-application guard asks for the autoloader, not the directory: a
	// checkout carries `vendor/.htaccess`, so `[ -d vendor ]` is not the test.
	writeFile(t, filepath.Join(host.CurrentPath, "vendor", "autoload.php"), "<?php\n")

	command, err := vars.Expand(recipe.Task(deploy.TaskWorkersPause).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("run app:workers:pause: %v\n%s", err, command)
	}
	raw, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read the stub log: %v", err)
	}
	var calls []string
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls
}
