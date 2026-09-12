package tests

import (
	"context"
	"os"
	"path/filepath"
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

// The guard has to hold when the command runs, not merely be present in the
// template. This test used to assert the presence of `[ "{{settings.mage_mode}}"
// != "developer" ]` — which is always true, because the substituted value is
// already shell-quoted, so it compared `'developer'` with `developer` and passed
// while developer mode deployed static content anyway.
func TestMagento2RecipeSkipsStaticContentInDeveloperMode(t *testing.T) {
	if calls := assetCalls(t, map[string]any{"mage_mode": "developer"}); len(calls) != 0 {
		t.Fatalf("developer mode must deploy no static content, got %v", calls)
	}
	if calls := assetCalls(t, map[string]any{"mage_mode": "production"}); len(calls) == 0 {
		t.Fatal("production mode must deploy static content")
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
	// The verification checks are templates too, and a check that cannot expand
	// would fail the deploy after it had already published.
	for _, check := range recipe.Checks {
		if _, err := vars.Expand(check.Command); err != nil {
			t.Errorf("check %q does not expand: %v\n%s", check.ID, err, check.Command)
		}
	}

	// The restore command additionally references {{backup_path}}, which the
	// engine supplies from the release record at restore time.
	restoreVars := vars.SetRaw("backup_path", "/srv/app/shared/backups/deploy/1/dump.sql")
	if _, err := restoreVars.Expand(recipe.Restore); err != nil {
		t.Errorf("the restore command does not expand: %v\n%s", err, recipe.Restore)
	}
}

// The two checks the engine cannot supply: an application check that needs the
// database, and the in-place artifact comparison. Spec 11 names both.
func TestMagento2RecipeDeclaresItsVerificationChecks(t *testing.T) {
	recipe := magento2.DeployRecipe()
	byID := map[string]deploy.Check{}
	for _, check := range recipe.Checks {
		if check.ID == "" || check.Title == "" || check.Command == "" {
			t.Fatalf("an incomplete check: %+v", check)
		}
		byID[check.ID] = check
	}

	app, ok := byID["app"]
	if !ok {
		t.Fatal("the recipe must declare an application check")
	}
	if !strings.Contains(app.Command, "setup:db:status") {
		t.Fatalf("the application check must touch the database, got %q", app.Command)
	}
	// `--version` succeeds with neither env.php nor a database, which is why the
	// spec names it as the wrong check.
	if strings.Contains(app.Command, "--version") {
		t.Fatalf("`--version` proves nothing about the application: %q", app.Command)
	}
	if !strings.Contains(app.Command, "{{release_path}}") || !strings.Contains(app.Command, "{{php_bin}}") {
		t.Fatalf("the check must run the deployed code with the configured interpreter: %q", app.Command)
	}

	artifact, ok := byID["artifact"]
	if !ok {
		t.Fatal("the recipe must declare the in-place artifact check")
	}
	if artifact.OnlyForPublishStrategy != deploy.PublishInPlace {
		t.Fatalf("the artifact check only means something in place, got %q", artifact.OnlyForPublishStrategy)
	}
	if !strings.Contains(artifact.Command, "deployed_version.txt") {
		t.Fatalf("the artifact check must compare the version file, got %q", artifact.Command)
	}
}

// The artifact check's command has to mean what its title claims, so it is run
// for real against a matching, a stale and an absent version file.
func TestMagento2ArtifactCheckComparesTheDocrootWithTheRelease(t *testing.T) {
	var artifact deploy.Check
	for _, check := range magento2.DeployRecipe().Checks {
		if check.ID == "artifact" {
			artifact = check
		}
	}
	if artifact.Command == "" {
		t.Fatal("no artifact check to exercise")
	}

	root := t.TempDir()
	releaseDir := filepath.Join(root, "releases", "1")
	docroot := filepath.Join(root, "current")
	version := filepath.Join("pub", "static", "deployed_version.txt")
	for _, dir := range []string{releaseDir, docroot} {
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(version)), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	writeFile(t, filepath.Join(releaseDir, version), "abc123\n")
	writeFile(t, filepath.Join(docroot, version), "abc123\n")

	run := func() error {
		command, err := deploy.NewVars().
			SetPath("release_path", releaseDir).
			SetPath("current_path", docroot).
			Expand(artifact.Command)
		if err != nil {
			t.Fatalf("expand: %v", err)
		}
		_, err = (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{})
		return err
	}

	if err := run(); err != nil {
		t.Fatalf("matching version files must pass, got: %v", err)
	}
	writeFile(t, filepath.Join(docroot, version), "stale\n")
	if err := run(); err == nil {
		t.Fatal("a docroot serving a stale static content version must fail the check")
	}
	if err := os.Remove(filepath.Join(releaseDir, version)); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if err := run(); err != nil {
		t.Fatalf("a release with no version file has nothing to compare: %v", err)
	}
}

// assetCalls runs the recipe's real static-content command and returns the argv
// of every `bin/magento` invocation it made, one per line.
//
// The command is *executed*, not inspected: both the split and the single-pass
// path live inside the same shell command, so a text assertion cannot tell which
// branch ran — which is exactly the kind of test that passes while the feature is
// broken. `php_bin` is set to `sh` and the stub is a shell script, so no PHP is
// needed to run the branch Magento would run.
func assetCalls(t *testing.T, settings map[string]any) []string {
	t.Helper()
	return recipeCalls(t, deploy.TaskAssets, settings)
}

// recipeCalls expands and runs one recipe task against a recording stub.
func recipeCalls(t *testing.T, taskID string, settings map[string]any) []string {
	t.Helper()
	recipe := magento2.DeployRecipe()
	release := t.TempDir()
	log := filepath.Join(t.TempDir(), "calls.log")

	settings["php_bin"] = "sh"
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: settings,
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	vars := cmd.DeployVarsForTest(host, options).SetPath("release_path", release)

	stub := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + log + "\n"
	writeFile(t, filepath.Join(release, "bin", "magento"), stub)
	if err := os.Chmod(filepath.Join(release, "bin", "magento"), 0o755); err != nil {
		t.Fatalf("chmod stub: %v", err)
	}

	command, err := vars.Expand(recipe.Task(taskID).Command)
	if err != nil {
		t.Fatalf("expand %s: %v", taskID, err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("run %s: %v\n%s", taskID, err, command)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read the stub log: %v", err)
	}
	calls := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(line) != "" {
			calls = append(calls, line)
		}
	}
	return calls
}

// Spec 10.4: `split_static_deployment` deploys adminhtml and frontend
// separately. `--area` and its two values come from Magento's own option
// definition (Magento_Deploy\Console\DeployStaticOptions), and the order — admin
// first, frontend second — is the one the reference deploy tool uses.
func TestMagento2SplitStaticDeploymentRunsBothAreasInOrder(t *testing.T) {
	calls := assetCalls(t, map[string]any{
		"split_static_deployment": true,
		"magento_themes":          []string{"Acme/theme"},
		"static_content_locales":  []string{"en_US", "fr_FR"},
	})
	if len(calls) != 2 {
		t.Fatalf("the split must deploy two passes, got %d: %v", len(calls), calls)
	}
	if !strings.Contains(calls[0], "--area=adminhtml") {
		t.Fatalf("the first pass must be adminhtml, got %q", calls[0])
	}
	if !strings.Contains(calls[1], "--area=frontend") {
		t.Fatalf("the second pass must be frontend, got %q", calls[1])
	}
	// The admin pass carries the backend theme and the frontend languages it
	// defaults to; the frontend pass carries the project's themes.
	if !strings.Contains(calls[0], "-t Magento/backend") {
		t.Fatalf("the admin pass must deploy the backend theme, got %q", calls[0])
	}
	if strings.Contains(calls[0], "Acme/theme") {
		t.Fatalf("the admin pass must not deploy a frontend theme, got %q", calls[0])
	}
	if !strings.Contains(calls[0], "--language en_US") || !strings.Contains(calls[0], "--language fr_FR") {
		t.Fatalf("the admin languages must default to the frontend ones, got %q", calls[0])
	}
	if !strings.Contains(calls[1], "-t Acme/theme") || strings.Contains(calls[1], "Magento/backend") {
		t.Fatalf("the frontend pass must deploy the project's themes only, got %q", calls[1])
	}
	// The backend can be given its own theme and languages.
	calls = assetCalls(t, map[string]any{
		"split_static_deployment":        true,
		"magento_themes":                 []string{"Acme/theme"},
		"magento_themes_backend":         []string{"Acme/admin"},
		"static_content_locales_backend": []string{"de_DE"},
	})
	if !strings.Contains(calls[0], "-t Acme/admin") || !strings.Contains(calls[0], "--language de_DE") {
		t.Fatalf("a configured backend theme and languages must win, got %q", calls[0])
	}
}

func TestMagento2StaticContentStaysSinglePassByDefault(t *testing.T) {
	calls := assetCalls(t, map[string]any{"magento_themes": []string{"Acme/theme"}})
	if len(calls) != 1 {
		t.Fatalf("without the split there must be one pass, got %v", calls)
	}
	if strings.Contains(calls[0], "--area=") {
		t.Fatalf("without the split there must be no --area flag, got %q", calls[0])
	}
	if !strings.Contains(calls[0], "-t Acme/theme") {
		t.Fatalf("the project's themes must still be deployed, got %q", calls[0])
	}
}

func TestMagento2StaticContentIsSkippedInDeveloperMode(t *testing.T) {
	calls := assetCalls(t, map[string]any{
		"mage_mode":               "developer",
		"split_static_deployment": true,
	})
	if len(calls) != 0 {
		t.Fatalf("developer mode deploys no static content, got %v", calls)
	}
}

// A recipe that declares the same setting twice is a copy-paste bug that
// validation tolerates silently, because the last declaration wins.
func TestMagento2RecipeDeclaresEachSettingOnce(t *testing.T) {
	seen := map[string]bool{}
	for _, setting := range magento2.DeployRecipe().Settings {
		if seen[setting.Key] {
			t.Errorf("deploy.settings.%s is declared twice", setting.Key)
		}
		seen[setting.Key] = true
		if setting.Title == "" {
			t.Errorf("deploy.settings.%s has no description", setting.Key)
		}
	}
}

// The same quoting bug made worker control inert, which is the one that touches
// production: with `worker_control: true` the window is supposed to stop cron and
// consumers *before* the schema changes, and it never did.
func TestMagento2WorkerControlActuallyRuns(t *testing.T) {
	pause := recipeCalls(t, deploy.TaskWorkersPause, map[string]any{"worker_control": true})
	if !strings.Contains(strings.Join(pause, "\n"), "cron:remove") ||
		!strings.Contains(strings.Join(pause, "\n"), "queue:consumers:stop") {
		t.Fatalf("worker_control must stop cron and consumers before the migration, got %v", pause)
	}
	if calls := recipeCalls(t, deploy.TaskWorkersPause, map[string]any{"worker_control": false}); len(calls) != 0 {
		t.Fatalf("worker_control off must leave cron and consumers alone, got %v", calls)
	}

	resume := recipeCalls(t, deploy.TaskWorkersResume, map[string]any{"worker_control": true})
	if !strings.Contains(strings.Join(resume, "\n"), "cron:install") ||
		!strings.Contains(strings.Join(resume, "\n"), "queue:consumers:restart") {
		t.Fatalf("worker_control must restore cron and restart consumers afterwards, got %v", resume)
	}
	if calls := recipeCalls(t, deploy.TaskWorkersResume, map[string]any{}); len(calls) != 0 {
		t.Fatalf("worker_control unset must leave cron and consumers alone, got %v", calls)
	}
}
