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

// Every step runs as a fresh remote command, so it has to enter the absolute
// directory it acts on rather than relying on an ambient working directory. That
// is the release being built for everything that builds or migrates it, and the
// *current* path for the maintenance tasks: maintenance mode belongs to the
// application the web server is serving, which before the swap is the previous
// release.
func TestMagento2RecipeCommandsEnterTheDirectoryTheyActOn(t *testing.T) {
	recipe := magento2.DeployRecipe()

	for _, task := range recipe.Tasks {
		if task.Command == "" {
			continue
		}
		if !strings.Contains(task.Command, "{{release_path}}") && !strings.Contains(task.Command, "{{current_path}}") {
			t.Errorf("task %q enters no directory; each step runs as a fresh remote command:\n%s", task.ID, task.Command)
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

// `magento/magento-cloud-patches` is a Composer plugin, and Magento Cloud
// projects run it from `post-install-cmd`, so `build:vendors` has already applied
// the patch set by the time this step runs. An unconditional `apply` then fails
// with "can't be applied to clean Magento instance" and kills a deploy whose
// patches are in fact applied — observed on a real 2.4.9 project, where the
// release could not get past `build:patches`.
func TestMagento2RecipePatchesStepDoesNotReapplyAnAppliedSet(t *testing.T) {
	command := magento2.DeployRecipe().Task("build:patches").Command

	gate := strings.Index(command, "verify --cloud-only")
	apply := strings.Index(command, "ece-patches apply")
	if gate < 0 {
		t.Fatalf("build:patches must ask the tool whether the set is applied before applying it:\n%s", command)
	}
	// `verify` without the flag also reports the project's deliberately unapplied
	// optional quality patches and exits non-zero on a healthy target, so it cannot
	// be the gate.
	if strings.Contains(command, "ece-patches verify >") {
		t.Fatalf("build:patches gates on a bare `verify`, which fails on a healthy target:\n%s", command)
	}
	// The gate decides whether to apply; it must not decide whether to succeed. A
	// genuinely unapplied required patch has to reach `apply` and fail the step.
	if apply < 0 {
		t.Fatalf("build:patches no longer applies anything:\n%s", command)
	}
	if apply < gate {
		t.Fatalf("the verify gate must come before the apply:\n%s", command)
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

// The map form is the multi-website one, so its locales have to reach Magento as
// `--language` options, and they have to add to the project's locale list rather
// than replace it. Asserted on the command line the stub actually received: the
// difference between `-t Acme/other de_DE` (a loose argument that silently
// overrides `--language`) and `-t Acme/other --language de_DE` is invisible in the
// template and decides whether a store view gets its locales.
func TestMagento2ThemeMapLocalesReachMagentoAsLanguageFlags(t *testing.T) {
	calls := assetCalls(t, map[string]any{
		"magento_themes": map[string]any{
			"Acme/theme": []any{"en_US"},
			"Acme/other": []any{"de_DE"},
		},
		"static_content_locales": []string{"fr_FR"},
	})
	if len(calls) != 1 {
		t.Fatalf("without the split there is one pass, got %v", calls)
	}
	want := "setup:static-content:deploy -f --content-version=abcdef12 -j 4" +
		" --language fr_FR -t Acme/other -t Acme/theme --language de_DE --language en_US"
	if calls[0] != want {
		t.Fatalf("the map form rendered\n  %q\nwant\n  %q", calls[0], want)
	}
}

// The reference tool exposes the static content command's own flags as one
// passthrough setting (`static_deploy_options`, e.g. `--no-parent`). Without it a
// project that needs `--no-parent`, `-s standard` or `--exclude-theme` has to
// write raw flags into the theme list, which is a setting about themes.
func TestMagento2StaticDeployOptionsArePassedThroughEveryPass(t *testing.T) {
	calls := assetCalls(t, map[string]any{"static_deploy_options": "--no-parent -s standard"})
	if len(calls) != 1 {
		t.Fatalf("one pass without the split, got %v", calls)
	}
	want := "setup:static-content:deploy -f --content-version=abcdef12 -j 4 --no-parent -s standard"
	if calls[0] != want {
		t.Fatalf("the passthrough rendered\n  %q\nwant\n  %q", calls[0], want)
	}

	// A list is the same thing written the other way, and the split pass carries
	// the flags too.
	split := assetCalls(t, map[string]any{
		"static_deploy_options":   []string{"--no-parent"},
		"split_static_deployment": true,
	})
	if len(split) != 2 {
		t.Fatalf("the split must deploy two passes, got %v", split)
	}
	for index, area := range []string{"adminhtml", "frontend"} {
		if !strings.Contains(split[index], "--no-parent") {
			t.Fatalf("the %s pass must carry the passthrough flags, got %q", area, split[index])
		}
	}

	// A project that sets nothing gets exactly the command it got before the
	// setting existed.
	plain := assetCalls(t, map[string]any{})
	if len(plain) != 1 || strings.Contains(plain[0], "  ") || !strings.HasSuffix(plain[0], "-j 4") {
		t.Fatalf("an unset passthrough must render nothing, got %q", plain)
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

// The Hyvä path: `settings.frontend_dir` names the theme's Tailwind directory and
// `frontend_command` builds it. Everything about it used to be asserted by reading
// the template, and reading it could not see that the command was substituted as
// ONE quoted word — `'npm ci && npm run build'` — so the shell reported a command
// that does not exist and every Hyvä deploy failed at build:frontend.
func TestMagento2FrontendBuildRunsTheConfiguredCommandInsideTheTheme(t *testing.T) {
	release := t.TempDir()
	log := filepath.Join(t.TempDir(), "frontend.log")
	theme := filepath.Join(release, "app/design/frontend/Acme/hyva/web/tailwind")
	if err := os.MkdirAll(theme, 0o755); err != nil {
		t.Fatalf("mkdir theme: %v", err)
	}

	recipe := magento2.DeployRecipe()
	first := filepath.Join(t.TempDir(), "first")
	second := filepath.Join(t.TempDir(), "second")
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{
			"frontend_dir": "app/design/frontend/Acme/hyva/web/tailwind",
			// A command shaped like the default one: two commands joined, plus a
			// working directory that only exists inside the theme directory.
			"frontend_command": "pwd > " + log + " && touch " + first + " && touch " + second,
		},
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	vars := cmd.DeployVarsForTest(host, options).SetPath("release_path", release)

	command, err := vars.Expand(recipe.Task(deploy.TaskFrontend).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("the frontend build must run: %v\n%s", err, command)
	}
	for _, want := range []string{first, second} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("the second half of the command did not run: %v\n%s", err, command)
		}
	}
	ran, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read the working directory: %v", err)
	}
	if got := strings.TrimSpace(string(ran)); !strings.HasSuffix(got, "/app/design/frontend/Acme/hyva/web/tailwind") {
		t.Fatalf("the build must run inside the theme directory, ran in %q", got)
	}

	// The default command has to survive substitution as a command.
	spec := recipe.Defaults["frontend_command"]
	if _, isString := spec.(string); !isString {
		t.Fatalf("frontend_command default = %#v, want a command string", spec)
	}
	plain := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Revision: "abcdef123456", Settings: map[string]any{}})
	expanded, err := cmd.DeployVarsForTest(host, plain).Expand("cd {{release_path}} && {{settings.frontend_command}}")
	if err != nil {
		t.Fatalf("expand the default frontend command: %v", err)
	}
	if !strings.Contains(expanded, "npm ci && npm run build") {
		t.Fatalf("the default command must expand as a command, got %q", expanded)
	}
	if strings.Contains(expanded, "'npm ci") {
		t.Fatalf("a shell fragment must not be quoted, got %q", expanded)
	}
}

// A project can have more than one theme that is built by Node — two Hyvä
// storefronts, or a Hyvä theme plus a custom one — and a step that builds only the
// first leaves the second storefront without its assets. Executed, because the
// step is a shell loop and reading it proves neither that both iterations run nor
// that each runs in its own directory.
func TestMagento2FrontendBuildBuildsEveryThemeDirectory(t *testing.T) {
	release := t.TempDir()
	log := filepath.Join(t.TempDir(), "frontend.log")
	dirs := []string{
		"app/design/frontend/Acme/hyva/web/tailwind",
		"app/design/frontend/Acme/other/web/tailwind",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(release, dir), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}

	recipe := magento2.DeployRecipe()
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{
			"frontend_dir":     dirs,
			"frontend_command": "pwd >> " + log,
		},
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	command, err := cmd.DeployVarsForTest(host, options).SetPath("release_path", release).
		Expand(recipe.Task(deploy.TaskFrontend).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("the frontend build must run for every theme: %v\n%s", err, command)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("read the build log: %v", err)
	}
	lines := strings.Fields(string(raw))
	if len(lines) != len(dirs) {
		t.Fatalf("%d theme directories configured, %d builds ran: %q", len(dirs), len(lines), string(raw))
	}
	for index, dir := range dirs {
		if !strings.HasSuffix(lines[index], "/"+dir) {
			t.Fatalf("build %d ran in %q, want it inside %q", index, lines[index], dir)
		}
	}

	// A failing build must stop the deploy rather than let the next theme's build
	// hide it, which is what a loop without an exit status would do: the broken
	// directory comes first and a working one after it.
	broken := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{
			"frontend_dir":     []string{"app/design/frontend/Acme/missing/web/tailwind", dirs[0]},
			"frontend_command": "true",
		},
	})
	brokenCommand, err := cmd.DeployVarsForTest(host, broken).SetPath("release_path", release).
		Expand(recipe.Task(deploy.TaskFrontend).Command)
	if err != nil {
		t.Fatalf("expand the broken build: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), brokenCommand, deploy.RunOptions{}); err == nil {
		t.Fatalf("a theme directory that cannot be entered must fail the step:\n%s", brokenCommand)
	}
}

func TestMagento2FrontendBuildIsSkippedWithoutAThemeDirectory(t *testing.T) {
	release := t.TempDir()
	marker := filepath.Join(t.TempDir(), "ran")
	recipe := magento2.DeployRecipe()
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{"frontend_command": "touch " + marker},
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	command, err := cmd.DeployVarsForTest(host, options).SetPath("release_path", release).
		Expand(recipe.Task(deploy.TaskFrontend).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("a project without a frontend_dir must not fail the step: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a Luma/stock project must not run a frontend build")
	}
}

// The same defect made the runtime reload a command that does not exist, so the
// opcache/realpath mitigation spec 9.1 asks for never ran.
func TestMagento2RuntimeReloadCommandActuallyRuns(t *testing.T) {
	release := t.TempDir()
	marker := filepath.Join(t.TempDir(), "reloaded")
	recipe := magento2.DeployRecipe()

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{"runtime_reload_command": "printf reloaded > " + marker},
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	stub, _ := cmd.DeployVarsForTest(host, options).SetPath("release_path", release).
		Expand(recipe.Task(deploy.TaskAppCacheFlush).Command)
	// The stub `php` stands in for bin/magento here: this test is about the
	// reload fragment, not about the cache flush.
	command := strings.Replace(stub, "'php' bin/magento cache:flush", "true", 1)
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("the cache step must run with a reload command configured: %v\n%s", err, command)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the configured reload must run: %v\n%s", err, command)
	}
}

// A shell fragment that is not configured must render as a no-op that still fits
// in the command line. Rendering nothing leaves `cmd && ` behind, which made the
// cache step a syntax error — caught by running the recipe in the sandbox, not by
// reading it.
func TestMagento2UnsetRuntimeReloadIsANoOp(t *testing.T) {
	release := t.TempDir()
	recipe := magento2.DeployRecipe()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{Revision: "abcdef123456", Settings: map[string]any{}})
	command, err := cmd.DeployVarsForTest(host, options).SetPath("release_path", release).
		Expand(recipe.Task(deploy.TaskAppCacheFlush).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if strings.Contains(command, "&& ;") || strings.Contains(command, "&&  ") || strings.HasSuffix(strings.TrimSpace(command), "&&") {
		t.Fatalf("an unset fragment must not leave a dangling operator:\n%s", command)
	}
	stub := strings.Replace(command, "'php' bin/magento cache:flush", "true", 1)
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), stub, deploy.RunOptions{}); err != nil {
		t.Fatalf("the cache step must run with no reload configured: %v\n%s", err, stub)
	}
}

// The sandbox image the recipe asks for has to satisfy the platform check a real
// project's `composer install` performs. A first trial of the deploy sandbox against
// a real Magento project stopped at build:vendors with "magento/framework requires
// ext-curl * -> it is missing from your system"; installing php-curl in the running
// container moved it to the next (project-side) failure, and `composer
// check-platform-reqs` there then reported every other extension as satisfied — so
// `curl` was the one gap in this list.
func TestMagento2SandboxRequirementsCoverThePlatformCheck(t *testing.T) {
	requirements := magento2.DeployRecipe().Sandbox

	extensions := map[string]bool{}
	for _, extension := range requirements.Extensions {
		extensions[extension] = true
	}
	// Every extension Magento's own composer platform check names and Debian does not
	// ship in php-cli. `curl` was the missing one.
	for _, want := range []string{"curl", "bcmath", "gd", "intl", "mysql", "soap", "sockets", "xsl", "zip"} {
		if !extensions[want] {
			t.Errorf("sandbox requirements do not install ext-%s, so a real composer install cannot pass its platform check", want)
		}
	}

	// The services the recipe's own commands need: setup:upgrade and setup:db:status
	// talk to a database.
	services := strings.Join(requirements.Services, " ")
	if !strings.Contains(services, "mariadb") {
		t.Errorf("sandbox services = %q, want a database for the recipe's own commands", services)
	}
}
