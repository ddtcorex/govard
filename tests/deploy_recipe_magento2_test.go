package tests

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

// The compiler clears `generated/code` before it compiles, while an optimized
// autoloader built with that directory present maps every class inside it. The
// map then points at files the compiler has just deleted, and the first lookup
// dies with "Failed to open stream" instead of generating the class — a resumed
// deploy that re-ran `build:vendors` over a release the previous attempt had
// already compiled, observed on a real 2.4.9 project.
//
// A fresh release cannot reach it, so the order inside the step is the fix:
// clear the tree, rebuild the map without it, and only then compile. Asserting
// the three markers are present would pass for any order, including the broken
// one.
func TestMagento2CompileStepRebuildsTheAutoloaderWithoutStaleGeneratedCode(t *testing.T) {
	template := magento2.DeployRecipe().Task("build:compile").Command

	clear := strings.Index(template, "rm -rf generated/code")
	dump := strings.Index(template, "dump-autoload")
	compile := strings.Index(template, "setup:di:compile")
	if clear < 0 || dump < 0 || compile < 0 {
		t.Fatalf("build:compile must clear the generated tree, rebuild the autoloader and compile:\n%s", template)
	}
	if clear > dump || dump > compile {
		t.Fatalf("build:compile must clear before it rebuilds the map and rebuild the map before it compiles:\n%s", template)
	}
	if !strings.Contains(template, "{{composer_bin}} dump-autoload") {
		t.Fatalf("build:compile must rebuild the map with the configured Composer binary:\n%s", template)
	}

	// The markers above say the step is written in the right order. This runs it,
	// because a template can be reordered into the broken order and still read
	// correctly: what makes the order the fix is the effect it has on the
	// classmap, and the classmap is what the compiler then loads from.
	//
	// Two stand-ins model the only two behaviours that matter, so no PHP and no
	// Composer are needed: the optimizer lists the project's generated tree in
	// the map, and the compiler clears that tree before it loads what the map
	// still points at.
	dir := t.TempDir()
	vars := deploy.NewVars().
		SetPath("release_path", dir).
		Set("composer_bin", filepath.Join(dir, "bin", "composer")).
		Set("php_bin", filepath.Join(dir, "bin", "php"))
	command, expandErr := vars.Expand(template)
	if expandErr != nil {
		t.Fatalf("render build:compile: %v", expandErr)
	}
	seedStandInDeployTools(t, dir)

	// The state a previous compile leaves behind: a generated class, and the
	// optimized map that lists it. A fresh release cannot be in this state, which
	// is why the defect only appeared on a resumed or rebuilt one.
	seedOptimizedRelease(t, dir)
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{Dir: dir}); err != nil {
		t.Fatalf("a release that was compiled before must still compile: %v", err)
	}
	if classmap := readClassmap(t, dir); strings.Contains(classmap, "generated/code") {
		t.Fatalf("the rebuilt map still lists the tree the compiler clears:\n%s", classmap)
	}

	// The other direction, in the same test: the same step with the clear removed
	// must fail exactly the way the real defect failed. Without this the test
	// would pass for a step that merely happens to work.
	withoutClear := strings.Replace(command, "rm -rf generated/code generated/metadata && ", "", 1)
	if withoutClear == command {
		t.Fatalf("build:compile no longer clears the tree in the shape this test removes:\n%s", command)
	}
	seedOptimizedRelease(t, dir)
	_, err := (deploy.LocalRunner{}).Run(context.Background(), withoutClear, deploy.RunOptions{Dir: dir})
	if err == nil {
		t.Fatal("without the clear, a compile over a previously compiled release must fail on the stale map")
	}
	if !strings.Contains(err.Error(), "Failed to open stream") {
		t.Fatalf("the failure must be the one the clear removes, got %v", err)
	}
}

// composerStandIn is `composer dump-autoload --optimize` reduced to the part this
// defect turns on: the optimizer builds the classmap from the project's autoload
// roots, and a Magento project registers `generated/code` as one of them.
const composerStandIn = `#!/bin/sh
map=vendor/composer/autoload_classmap.php
mkdir -p vendor/composer
{
  echo '<?php return array('
  find generated/code -name '*.php' 2>/dev/null | while read -r file; do
    echo "  '"$file"' => __DIR__ . '/../../"$file"',"
  done
  echo ');'
} > "$map"
`

// compileStandIn is `bin/magento setup:di:compile` reduced the same way: the
// compiler clears the generated tree, and the class loader then loads every
// class the optimized map still points at. Magento's own words for the failure
// are what the assertions look for.
const compileStandIn = `#!/bin/sh
if [ "$2" != "setup:di:compile" ]; then
  exit 0
fi
rm -rf generated/code generated/metadata
for file in $(grep -o "generated/code[^']*" vendor/composer/autoload_classmap.php 2>/dev/null); do
  if [ ! -f "$file" ]; then
    echo "Warning: include($file): Failed to open stream: No such file or directory in vendor/composer/ClassLoader.php on line 576" >&2
    exit 1
  fi
done
exit 0
`

// seedOptimizedRelease puts a release in the state a compile leaves behind: a
// generated class, and the optimized map that already lists it.
func seedOptimizedRelease(t *testing.T, dir string) {
	t.Helper()

	writeFile(t, filepath.Join(dir, "generated/code/Acme/Framework/Proxy.php"), "<?php\n")
	writeFile(t, filepath.Join(dir, "vendor/composer/autoload_classmap.php"),
		"<?php return array(\n"+
			"  'generated/code/Acme/Framework/Proxy.php' => __DIR__ . '/../../generated/code/Acme/Framework/Proxy.php',\n"+
			");\n")
}

// seedStandInDeployTools installs the two stand-ins the rendered command calls.
func seedStandInDeployTools(t *testing.T, dir string) {
	t.Helper()

	for name, script := range map[string]string{"composer": composerStandIn, "php": compileStandIn} {
		path := filepath.Join(dir, "bin", name)
		writeFile(t, path, script)
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatalf("make the %s stand-in executable: %v", name, err)
		}
	}
}

func readClassmap(t *testing.T, dir string) string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, "vendor/composer/autoload_classmap.php"))
	if err != nil {
		t.Fatalf("read the classmap: %v", err)
	}
	return string(raw)
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
	restoreVars := vars.SetPath("backup_path", "/srv/app/shared/backups/deploy/1/dump.sql")
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
	// The check answers for the application the web server serves. On an
	// in-place target the docroot is a different directory from the release the
	// build ran in, so a check run against the release proves nothing about the
	// site — the same reason the cache flush and the maintenance flag moved.
	if !strings.Contains(app.Command, "{{current_path}}") || !strings.Contains(app.Command, "{{php_bin}}") {
		t.Fatalf("the check must run the served code with the configured interpreter: %q", app.Command)
	}
	if strings.Contains(app.Command, "{{release_path}}") {
		t.Fatalf("the served application is not the release on an in-place target: %q", app.Command)
	}

	// The in-place artifact comparison is gone: it compared a version file with
	// the copy the activation had just made from that same file, so it could only
	// fail if the copy failed. The engine now dry-runs the activation copy for
	// every sync path, which covers the whole tree
	// (TestVerifyInPlaceCatchesADocrootThatDiffersFromTheRelease).
	if _, exists := byID["artifact"]; exists {
		t.Fatal("the engine owns the in-place sync comparison; the recipe must not declare an artifact check")
	}
	if len(byID) != 1 {
		t.Fatalf("the recipe declares one framework check, got %d: %v", len(byID), byID)
	}
}

// The stale-version-file scenario the recipe's artifact check used to cover now
// lives in the engine's sync comparison, where the version file is compared as
// part of the tree it sits in
// (TestVerifyInPlaceCatchesADocrootThatDiffersFromTheRelease in deploy_publish_test.go).
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
	log := filepath.Join(t.TempDir(), "calls.log")

	settings["php_bin"] = "sh"
	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: settings,
	})
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	// The release lives under the deploy path here, as it does on a target
	// (`<deploy path>/releases/<n>`), so a test asserting which directory a step
	// entered can tell the release and the served root apart.
	release := host.ReleasePath("1")
	vars := cmd.DeployVarsForTest(host, options).SetPath("release_path", release)

	stub := "#!/bin/sh\nprintf '%s\\n' \"$PWD $*\" >> " + log + "\n"
	// The step is supposed to act on the served application, so the stub exists
	// in both directories and the recorded working directory is what the tests
	// assert.
	for _, root := range []string{release, host.CurrentPath} {
		writeFile(t, filepath.Join(root, "bin", "magento"), stub)
		if err := os.Chmod(filepath.Join(root, "bin", "magento"), 0o755); err != nil {
			t.Fatalf("chmod %s: %v", root, err)
		}
	}
	// The served-application guard asks for the autoloader, not the directory:
	// a checkout carries `vendor/.htaccess`, so `[ -d vendor ]` is not the test.
	writeFile(t, filepath.Join(host.CurrentPath, "vendor", "autoload.php"), "<?php\n")

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

// recipeCallArgs returns the arguments of a recorded call, dropping the working
// directory the stub logs in front of them.
func recipeCallArgs(call string) string {
	if index := strings.Index(call, " "); index >= 0 {
		return call[index+1:]
	}
	return ""
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
	if got := recipeCallArgs(calls[0]); got != want {
		t.Fatalf("the map form rendered\n  %q\nwant\n  %q", got, want)
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
	if got := recipeCallArgs(calls[0]); got != want {
		t.Fatalf("the passthrough rendered\n  %q\nwant\n  %q", got, want)
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
//
// The pause used `queue:consumers:stop`, which no Magento ships (2.4 ships
// `list`, `start` and `restart`), so the step failed before the migration.
// `queue:consumers:restart` is the poison pill running consumers check between
// messages — the only stop signal there is.
func TestMagento2WorkerControlActuallyRuns(t *testing.T) {
	pause := recipeCalls(t, deploy.TaskWorkersPause, map[string]any{"worker_control": true})
	if !strings.Contains(strings.Join(pause, "\n"), "cron:remove") ||
		!strings.Contains(strings.Join(pause, "\n"), "queue:consumers:restart") {
		t.Fatalf("worker_control must stop cron and consumers before the migration, got %v", pause)
	}
	if strings.Contains(strings.Join(pause, "\n"), "queue:consumers:stop") {
		t.Fatalf("app:workers:pause must not call queue:consumers:stop, which Magento does not have: %v", pause)
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

// The served application owns the crontab entry: `cron:remove` deletes only the
// block keyed by the install root it runs in, and `cron:install` writes the
// application's *absolute* path — run from the release, the pause matches
// nothing while the resume schedules `.deployer/releases/<n>/bin/magento`, a
// directory no web server serves.
func TestMagento2WorkerControlActsOnTheServedApplication(t *testing.T) {
	for id, want := range map[string]string{
		deploy.TaskWorkersPause:  "queue:consumers:restart",
		deploy.TaskWorkersResume: "cron:install",
	} {
		calls := recipeCalls(t, id, map[string]any{"worker_control": true})
		if len(calls) == 0 {
			t.Fatalf("%s recorded no call", id)
		}
		for _, call := range calls {
			if strings.Contains(call, "/releases/") {
				t.Errorf("%s ran in the release, not the served application: %s", id, call)
			}
			if !strings.Contains(call, "/current ") {
				t.Errorf("%s did not run in the served root: %s", id, call)
			}
		}
		if !strings.Contains(strings.Join(calls, "\n"), want) {
			t.Errorf("%s did not run %s: %v", id, want, calls)
		}
	}
}

// A first in-place deploy has no served application yet: the docroot is a fresh
// checkout with no autoloader behind it. Pausing workers has nothing to pause
// there, and the step must not fail the run — the same tolerance
// `maintenance:enable` already has.
func TestMagento2WorkerPauseToleratesAnUnrunnableDocroot(t *testing.T) {
	recipe := magento2.DeployRecipe()
	release := t.TempDir()
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	marker := filepath.Join(t.TempDir(), "ran")
	// `bin/magento` exists and is runnable; only the autoloader is missing — the
	// state a checkout of a Magento project is in before its first build. The
	// marker is what a weakened guard would produce: a guard that asks for the
	// `vendor/` *directory* passes here (Magento projects commit
	// `vendor/.htaccess`) and then fails on the first `bin/magento` call.
	writeFile(t, filepath.Join(host.CurrentPath, "bin", "magento"), "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(filepath.Join(host.CurrentPath, "bin", "magento"), 0o755); err != nil {
		t.Fatalf("chmod the stub: %v", err)
	}
	writeFile(t, filepath.Join(host.CurrentPath, "vendor", ".htaccess"), "Deny from all\n")

	options := deploy.WithRecipeDefaultsForTest(recipe, deploy.Options{
		Revision: "abcdef123456",
		Settings: map[string]any{"worker_control": true, "php_bin": "sh"},
	})
	command, err := cmd.DeployVarsForTest(host, options).SetPath("release_path", release).
		Expand(recipe.Task(deploy.TaskWorkersPause).Command)
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if _, err := (deploy.LocalRunner{}).Run(context.Background(), command, deploy.RunOptions{}); err != nil {
		t.Fatalf("a docroot with no autoloader must be skipped, not failed: %v\n%s", err, command)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("the guard accepted a docroot with no autoloader, so the pause ran:\n%s", command)
	}
}

// The flush is the step that was measurably wrong on a real in-place target: the
// cache the served application reads is the docroot's.
func TestMagento2CacheFlushRunsInTheServedRoot(t *testing.T) {
	calls := recipeCalls(t, deploy.TaskAppCacheFlush, map[string]any{})
	if len(calls) != 1 {
		t.Fatalf("one cache:flush call expected, got %v", calls)
	}
	if !strings.Contains(calls[0], "cache:flush") {
		t.Fatalf("the flush did not run: %v", calls)
	}
	if strings.Contains(calls[0], "/releases/") || !strings.Contains(calls[0], "/current ") {
		t.Fatalf("the flush must run in the served root: %v", calls)
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
	// The cache step enters the served application, which this host spells
	// `<deploy path>/current`; without it the step fails at its own `cd` before
	// reaching the reload fragment this test is about.
	if err := os.MkdirAll(host.CurrentPath, 0o755); err != nil {
		t.Fatalf("mkdir the served root: %v", err)
	}
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
	// Same reason as the reload test above: the step enters the served root.
	if err := os.MkdirAll(host.CurrentPath, 0o755); err != nil {
		t.Fatalf("mkdir the served root: %v", err)
	}

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
	// talk to a database, and a Magento env.php written for a server names a Redis
	// or Valkey for cache and sessions — without one the target cannot run
	// `bin/magento` at all, and a rehearsal would need a hand-edited env.php that
	// no server has.
	services := strings.Join(requirements.Services, " ")
	for _, want := range []string{"mariadb", "redis"} {
		if !strings.Contains(services, want) {
			t.Errorf("sandbox services = %q, want %q for the recipe's own commands", services, want)
		}
	}
}

// The image is built from the recipe's requirements, so the service list has to
// reach the container's entrypoint: `sandbox up` bakes it into the image, and a
// recipe that asks for a cache the entrypoint never starts is a rehearsal that
// fails for a reason no target would have.
func TestMagento2SandboxImageStartsTheServicesTheRecipeAsksFor(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileFull, PHP: "8.4", Requirements: magento2.DeployRecipe().Sandbox})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	services := sandboxServicesLine(t, dockerfile)
	for _, want := range []string{"mariadb", "redis-server", "govard-sandbox-web"} {
		if !strings.Contains(services, want) {
			t.Fatalf("GOVARD_SANDBOX_SERVICES = %q, want it to start %s", services, want)
		}
	}
	// The packages those services need come from the profile; `full` promises
	// both, and the entrypoint skips a service whose init script is absent.
	for _, want := range []string{"mariadb-server", "redis-server"} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the full profile must install %s", want)
		}
	}
}

// Static content deployment asks the application about itself — the store, the
// website, the locales — and that configuration lives in the target's database.
// A build machine cannot run it: measured on a real project,
// `setup:static-content:deploy` compiled every theme and then failed with "The
// default website isn't defined", with and without explicit themes and locales.
//
// The recipe therefore marks the task as needing the application, and artifact
// mode leaves it in the deploy: the target runs it after receiving the artifact.
func TestMagento2StaticContentRunsOnTheTargetNotTheBuilder(t *testing.T) {
	recipe := magento2.DeployRecipe()
	if task := recipe.Task(deploy.TaskAssets); !task.NeedsApplication {
		t.Fatal("build:assets must be marked as needing the deployed application")
	}

	plan, err := deploy.BuildPlanForTest(recipe, nil)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	plan = plan.ForBuildMode(deploy.BuildArtifact)

	artifactAt, assetsAt := -1, -1
	for idx, step := range plan.Steps {
		switch step.ID {
		case deploy.TaskArtifact:
			artifactAt = idx
		case deploy.TaskAssets:
			assetsAt = idx
			if step.Skipped {
				t.Fatalf("artifact mode must leave static content to the target: %s", step.SkipReason)
			}
		}
	}
	if artifactAt < 0 || assetsAt < 0 || artifactAt > assetsAt {
		t.Fatalf("the artifact must be received before static content runs: artifact at %d, static content at %d", artifactAt, assetsAt)
	}
}

// The downtime block runs only when the database drifted: the recipe gates it
// on setup:db:status (exit 0 = current, 1/2 = migrate) while compile, the
// cache flush and the backup always run.
func TestMagento2ConditionalMigrateProbeAndFlags(t *testing.T) {
	recipe := magento2.DeployRecipe()
	if recipe.MigrationProbe == nil || recipe.MigrationProbe.Command == "" {
		t.Fatal("magento2 recipe declares no migration probe")
	}
	if !strings.Contains(recipe.MigrationProbe.Command, "setup:db:status") {
		t.Fatalf("the probe must be setup:db:status, got %q", recipe.MigrationProbe.Command)
	}
	flagged := map[string]bool{
		deploy.TaskMaintenanceEnable:  true,
		deploy.TaskWorkersPause:       true,
		deploy.TaskAppConfigure:       true,
		deploy.TaskDBMigrate:          true,
		deploy.TaskWorkersResume:      true,
		deploy.TaskMaintenanceDisable: true,
	}
	for _, task := range recipe.Tasks {
		if want := flagged[task.ID]; task.NeedsMigration != want {
			t.Errorf("task %s NeedsMigration = %v, want %v", task.ID, task.NeedsMigration, want)
		}
	}
	for _, id := range []string{deploy.TaskCompile, deploy.TaskAppCacheFlush, deploy.TaskDBBackup, deploy.TaskAssets} {
		if task := recipe.Task(id); task.NeedsMigration {
			t.Errorf("task %s must not carry NeedsMigration", id)
		}
	}
}

// Composer on the sandbox clones private git repos the image cannot know at
// build time, so host-key checking would fail every private VCS package
// (found live: "Host key verification failed" for a git.sutunam.com repo on a
// fresh sandbox). The dev environments already disable it in base.yml for the
// same reason; the rehearsal box follows suit while production targets keep
// real known_hosts.
func TestMagento2SandboxImageDisablesGitHostKeyChecking(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileFull, PHP: "8.5", Requirements: magento2.DeployRecipe().Sandbox})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(dockerfile, "GIT_SSH_COMMAND") || !strings.Contains(dockerfile, "StrictHostKeyChecking=no") {
		t.Fatalf("the sandbox image must let composer clone private repos without known_hosts, got:\n%s", dockerfile)
	}
}

func TestMagento2SandboxSSHSeesGitSSHCommand(t *testing.T) {
	dockerfile, err := deploy.SandboxDockerfile(deploy.SandboxSpec{Profile: deploy.SandboxProfileFull, PHP: "8.5", Requirements: magento2.DeployRecipe().Sandbox})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// Container ENV never reaches an SSH session (sshd scrubs it), so the
	// variable additionally travels in ~/.ssh/environment, which sshd reads
	// only under PermitUserEnvironment.
	for _, want := range []string{"PermitUserEnvironment yes", ".ssh/environment", "GIT_SSH_COMMAND="} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("the image must carry GIT_SSH_COMMAND into SSH sessions (%q missing)", want)
		}
	}
}

// `setup:backup` writes into the release's shared `var/backups`, and govard used
// to *copy* the file it wanted out of there: the original stayed behind, in a
// directory nothing prunes, holding the customer data the dump exists for. It now
// moves only the file this run produced — identified by a marker taken before the
// command, not by "newest mtime", so an operator's own dump taken moments earlier
// stays where it is.
func TestMagento2DumpCommandMovesOnlyTheDumpItProduced(t *testing.T) {
	host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
	release := deploy.NewReleaseForTest("1", "abcdef", "main")
	release.Path = host.ReleasePath("1")

	backups := filepath.Join(release.Path, "var", "backups")
	for _, dir := range []string{filepath.Join(release.Path, "bin"), backups} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	// The stub writes what `setup:backup` writes: a Magento-shaped file inside
	// var/backups, named with its own timestamp.
	stub := "#!/bin/sh\nprintf 'fresh dump' > var/backups/$(date +%s%N)_db.sql\n"
	if err := os.WriteFile(filepath.Join(release.Path, "bin", "magento"), []byte(stub), 0o755); err != nil {
		t.Fatalf("write the stub: %v", err)
	}
	// The operator's own dump, taken an hour ago.
	operator := filepath.Join(backups, "1_db.sql")
	if err := os.WriteFile(operator, []byte("operator dump"), 0o600); err != nil {
		t.Fatalf("write the operator's dump: %v", err)
	}
	older := time.Now().Add(-time.Hour)
	if err := os.Chtimes(operator, older, older); err != nil {
		t.Fatalf("age the operator's dump: %v", err)
	}

	sc := deploy.StepContextForTest(host, deploy.Options{DBBackup: true, CommandTimeout: time.Minute})
	sc.Release = release
	// The recipe runs `{{php_bin}} bin/magento`, and the stub is a shell script:
	// `sh` stands in for the PHP binary so the test needs no PHP.
	sc.Vars = sc.Vars.Set("php_bin", "sh").SetPath("release_path", release.Path)

	task := magento2.DeployRecipe().Task(deploy.TaskDBBackup)
	if task.Core == nil {
		t.Fatal("the Magento recipe declares no db:backup implementation")
	}
	if err := task.Core(context.Background(), sc); err != nil {
		t.Fatalf("db:backup: %v", err)
	}

	if release.Database.Backup == "" {
		t.Fatal("db:backup recorded no file")
	}
	raw, err := os.ReadFile(release.Database.Backup)
	if err != nil {
		t.Fatalf("read the recorded backup: %v", err)
	}
	if string(raw) != "fresh dump" {
		t.Fatalf("the recorded backup holds %q, want the dump this run produced", raw)
	}
	left, err := os.ReadDir(backups)
	if err != nil {
		t.Fatalf("read var/backups: %v", err)
	}
	var names []string
	for _, entry := range left {
		names = append(names, entry.Name())
	}
	if len(names) != 1 || names[0] != "1_db.sql" {
		t.Fatalf("var/backups holds %v, want only the operator's 1_db.sql", names)
	}
	if kept, err := os.ReadFile(operator); err != nil || string(kept) != "operator dump" {
		t.Fatalf("the operator's dump was touched: %q, %v", kept, err)
	}
}

// Magento's `setup:rollback --db-file` only accepts a name inside the release's
// own `var/backups`, so the restore has to copy the recorded dump back in. That
// copy used to stay there for good — one full dump per rollback, in the directory
// nothing prunes. It is removed again whether the tool succeeds or fails.
func TestMagento2RestoreLeavesNoCopyInVarBackups(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stub    string
		wantErr bool
	}{
		{
			name: "the restore succeeds",
			stub: `#!/bin/sh
file=""
for arg in "$@"; do
  case "$arg" in --db-file=*) file="${arg#--db-file=}" ;; esac
done
if [ -z "$file" ]; then echo "setup:rollback needs --db-file" >&2; exit 1; fi
if [ ! -f "var/backups/$file" ]; then echo "The rollback file is invalid." >&2; exit 1; fi
printf 'restored' > restored.txt
exit 0
`,
		},
		{
			name: "the restore fails",
			stub: `#!/bin/sh
echo "The rollback file is invalid." >&2
exit 1
`,
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			host := deploy.HostForTest(t.TempDir(), deploy.LocalRunner{})
			release := deploy.NewReleaseForTest("1", "abcdef", "main")
			release.Path = host.ReleasePath("1")
			backups := filepath.Join(release.Path, "var", "backups")
			for _, dir := range []string{filepath.Join(release.Path, "bin"), backups} {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dir, err)
				}
			}
			if err := os.WriteFile(filepath.Join(release.Path, "bin", "magento"), []byte(tc.stub), 0o755); err != nil {
				t.Fatalf("write the stub: %v", err)
			}
			dump := filepath.Join(host.BackupRootPath(), "1", "dump.sql")
			if err := os.MkdirAll(filepath.Dir(dump), 0o700); err != nil {
				t.Fatalf("mkdir the backup dir: %v", err)
			}
			if err := os.WriteFile(dump, []byte("dump"), 0o600); err != nil {
				t.Fatalf("write the recorded dump: %v", err)
			}
			release.Database.Backup = dump

			sc := deploy.StepContextForTest(host, deploy.Options{CommandTimeout: time.Minute})
			sc.Release = release
			sc.Vars = sc.Vars.Set("php_bin", "sh").SetPath("release_path", release.Path)

			err := deploy.CoreDBRestore(magento2.DeployRecipe().Restore)(context.Background(), sc)
			if tc.wantErr && err == nil {
				t.Fatal("a failing setup:rollback must fail the step")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("db:restore: %v", err)
			}
			left, readErr := os.ReadDir(backups)
			if readErr != nil {
				t.Fatalf("read var/backups: %v", readErr)
			}
			if len(left) != 0 {
				var names []string
				for _, entry := range left {
					names = append(names, entry.Name())
				}
				t.Fatalf("var/backups still holds %v after the restore", names)
			}
		})
	}
}
