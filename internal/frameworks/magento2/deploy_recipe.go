package magento2

import (
	"govard/internal/deploy"
)

// DeployRecipe returns Magento 2's deployment recipe. It fills the neutral task
// ids with the commands Magento actually needs and leaves the rest empty, so
// the engine's shared implementation (release creation, the lock, the symlink
// swap, verification) is used unchanged.
//
// Every command enters {{release_path}} first: the pipeline runs each step as a
// fresh remote command, and only the release directory has the code. The PHP
// binary comes from {{php_bin}} so a project with a non-default interpreter
// (a version-specific path, a container wrapper) is honoured without govard
// knowing about it.
func DeployRecipe() deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	recipe.ID = "magento2"
	recipe.Defaults = map[string]any{
		// Shared state: a release is thrown away, the data it points at is not.
		"shared_files": []string{"app/etc/env.php", "var/.maintenance.ip"},
		"shared_dirs": []string{
			"var/log", "var/report", "var/session", "var/backups",
			"var/tmp", "pub/media", "pub/sitemap", "pub/static/_cache",
		},
		"writable_dirs": []string{
			"var", "pub/static", "pub/media", "generated", "app/etc",
		},
		// Only read by the in-place strategy, where the docroot is reset to the
		// revision and everything the build produced under a gitignored path has to
		// be copied in — otherwise the new code runs on the previous deployment's
		// `vendor/`, `generated/` and `pub/static/`. The list is what the release
		// *builds*, and never a path `deploy:shared` links from `shared/`: those
		// symlinks are relative to the release and resolve elsewhere from a docroot.
		// `pub/static/_cache` is shared, which is why `pub/static` is named by its
		// two built children rather than as a whole.
		"sync_paths": []string{"vendor", "generated", "pub/static/adminhtml", "pub/static/frontend"},

		// Deployment settings with a sane default. Each one is overridable in
		// .govard.yml under deploy.settings.
		// frontend_dir is a word list rather than a single path: a project can
		// have two themes that are built by Node (two Hyvä storefronts), and
		// naming one of them would leave the other without its assets.
		"frontend_dir":           deploy.ArgsSpec{Words: true},
		"frontend_command":       "npm ci && npm run build",
		"content_version":        "",
		"static_jobs":            "4",
		"worker_control":         false,
		"runtime_reload_command": "",
		// developer mode generates static files on demand, so deploying them
		// would be wasted work and a stale-asset risk.
		"mage_mode":     "",
		"writable_mode": "chmod",
		"owner":         "",

		// A list or a map is rendered into command arguments; a string is
		// passed through verbatim so raw flags keep working. The themes list
		// renders the locales as `--language` options: a bare locale after
		// `-t <theme>` is Magento's positional `languages` argument, and that
		// argument is assigned over `--language`, so a theme map would silently
		// replace the project's locale list instead of adding to it.
		"static_content_locales": deploy.ArgsSpec{Flag: "--language"},
		"magento_themes":         deploy.ArgsSpec{Flag: "-t", ValueFlag: "--language"},

		// Extra flags for every static content pass, in the reference tool's
		// shape: `--no-parent`, `-s standard`, `--exclude-theme ...`. A string is
		// passed through verbatim, a list contributes one word per entry.
		"static_deploy_options": deploy.ArgsSpec{},

		// The adminhtml/frontend split. The admin pass deploys the backend
		// theme, which a frontend theme list never covers, and its languages
		// default to the frontend ones so the two passes agree unless the
		// project says otherwise.
		"split_static_deployment":        false,
		"magento_themes_backend":         deploy.ArgsSpec{Flag: "-t", ValueFlag: "--language", Default: []string{"Magento/backend"}},
		"static_content_locales_backend": deploy.ArgsSpec{Flag: "--language", DefaultFrom: "static_content_locales"},
	}

	// A setting is substituted shell-quoted, so a guard writes the value bare:
	// `[ {{settings.mage_mode}} != developer ]` reaches `test` as `developer`.
	// Wrapping it in quotes compares the literal quotes — `[ "'developer'" !=
	// "developer" ]` is always true — which silently makes the guard inert.
	fill := func(id, title, command string) {
		task := recipe.Task(id)
		task.Title = title
		task.Command = command
		recipe.ReplaceTask(task)
	}

	// fillOnTarget fills a task the target has to run itself: the command asks
	// the application about itself, and a build machine has no application, no
	// database and no store configuration. `govard deploy build` skips it —
	// measured: `setup:static-content:deploy` compiled every theme on the builder
	// and then failed with "The default website isn't defined", with and without
	// explicit themes and locales — and artifact mode leaves it in the deploy so
	// the target runs it after receiving the artifact.
	fillOnTarget := func(id, title, command string) {
		task := recipe.Task(id)
		task.Title = title
		task.Command = command
		task.NeedsApplication = true
		recipe.ReplaceTask(task)
	}

	fill(deploy.TaskVendors, "install Composer dependencies",
		"cd {{release_path}} && {{composer_bin}} install --no-dev --optimize-autoloader --no-interaction --prefer-dist")

	// ece-patches only ships with Magento Cloud / Mage-OS projects. The guard is
	// a test, not `|| true`: a project that has the binary and fails to apply a
	// patch must fail the deploy.
	//
	// The step applies the patch set only when it is not applied yet, because
	// `magento/magento-cloud-patches` is a Composer plugin and Magento Cloud
	// projects run it from `post-install-cmd`: by the time this step runs, the
	// patches `build:vendors` installed are applied, and a second `apply` fails
	// hard — "Patch MCLOUD-… can't be applied to clean Magento instance" — which
	// fails a deploy whose patches are, in fact, applied. The tool answers the
	// question itself: `verify --cloud-only` exits 0 when the required patch set is
	// applied and non-zero when it is not (measured against a real 2.4.9 project,
	// both ways). `verify` without the flag is not usable: it also reports the
	// project's deliberately unapplied *optional* quality patches, so it exits
	// non-zero on a healthy target.
	fill(deploy.TaskPatches, "apply Magento patches",
		`cd {{release_path}} && if [ -x vendor/bin/ece-patches ]; then `+
			`if {{php_bin}} vendor/bin/ece-patches verify --cloud-only >/dev/null 2>&1; then `+
			`echo "the Magento patch set is already applied"; `+
			`else {{php_bin}} vendor/bin/ece-patches apply; fi; fi`)

	// The optimizer builds its classmap from the autoload roots, and a Magento
	// project registers `generated/code/` as one of them, so an
	// `--optimize-autoloader` run scans whatever a previous compile left there
	// and maps every generated class. The compiler clears `generated/code`
	// before it compiles, which leaves those entries pointing at files that no
	// longer exist: the first lookup of such a class dies with "Failed to open
	// stream" instead of generating it. A fresh release cannot reach that state
	// (nothing is generated when the dependencies are installed) and a resumed
	// or rebuilt one reaches it every time, so the step owns the state it
	// compiles from — clear the generated tree, rebuild the map without it, then
	// compile. Measured on a real 2.4.9 release: 5421 classmap entries under
	// `generated/code` before the rebuild, none after, and the compile passes.
	fill(deploy.TaskCompile, "compile dependency injection",
		"cd {{release_path}} && rm -rf generated/code generated/metadata && "+
			"{{composer_bin}} dump-autoload --optimize --no-interaction && "+
			"{{php_bin}} bin/magento setup:di:compile")

	// Tailwind/Hyva build. Every configured theme directory is built in place, and
	// the loop is skipped entirely when none is configured, which is what keeps the
	// stock-theme case dependency-free. Each build is a subshell so the working
	// directory does not leak into the next one, and `|| exit 1` stops the deploy
	// on the first failure instead of letting the next theme's build hide it.
	fill(deploy.TaskFrontend, "build frontend assets",
		`cd {{release_path}} && for dir in {{settings.frontend_dir_args}}; do (cd "$dir" && {{settings.frontend_command}}) || exit 1; done`)

	// The two-area split runs the admin pass first and chains the frontend pass
	// with `&&`, so a failed admin pass stops the deploy instead of publishing
	// half the static content. The single pass is unchanged when the split is
	// off, which is the default.
	fillOnTarget(deploy.TaskAssets, "deploy static content",
		`cd {{release_path}} && if [ {{settings.mage_mode}} != developer ]; then `+
			`if [ {{settings.split_static_deployment}} = true ]; then `+
			`{{php_bin}} bin/magento setup:static-content:deploy -f --area=adminhtml --content-version={{settings.content_version}} -j {{settings.static_jobs}} {{settings.static_deploy_options_args}} {{settings.static_content_locales_backend_args}} {{settings.magento_themes_backend_args}} && `+
			`{{php_bin}} bin/magento setup:static-content:deploy -f --area=frontend --content-version={{settings.content_version}} -j {{settings.static_jobs}} {{settings.static_deploy_options_args}} {{settings.static_content_locales_args}} {{settings.magento_themes_args}}; `+
			`else `+
			`{{php_bin}} bin/magento setup:static-content:deploy -f --content-version={{settings.content_version}} -j {{settings.static_jobs}} {{settings.static_deploy_options_args}} {{settings.static_content_locales_args}} {{settings.magento_themes_args}}; `+
			`fi; fi`)

	// Maintenance mode belongs to the application the web server is *serving*,
	// not to the release being built: the flag is read from the docroot the
	// request lands in. Running these commands in {{release_path}} opens a window
	// nothing serves — the live release keeps answering while db:migrate changes
	// the schema its code depends on — and leaves a flag in the incoming release
	// that switches the site off the moment it goes live.
	//
	// The guard is the served application itself, not the directory: a symlink
	// target has no current release on a first deploy, and an in-place docroot is
	// a git checkout that may not have been deployed to yet. Neither has anything
	// to protect, and neither may fall back to the release being built.
	fill(deploy.TaskMaintenanceEnable, "enable maintenance mode",
		magentoServedAppGuard+" && {{php_bin}} bin/magento maintenance:enable; fi")

	// Same question as maintenance mode, and the same guard: there is nothing to
	// pause until an application is being served, and the crontab block
	// `cron:remove` deletes is keyed by the install root it runs in
	// (`#~ MAGENTO START <sha256(install root)>`), so it has to run where
	// `cron:install` last ran. It is not path-independent: run from the release
	// it matches nothing and the previously installed block survives.
	//
	// `queue:consumers:stop` does not exist — Magento 2.4 ships
	// `queue:consumers:list`, `:start` and `:restart` only (checked in the 2.4.6,
	// 2.4.8, 2.4.9 and Commerce vendor trees), so a `worker_control: true` deploy
	// died here with "Command ... is not defined", before the migration it was
	// meant to protect. `queue:consumers:restart` is the only stop signal
	// Magento has: it puts the poison pill that running consumers check between
	// messages, so they exit instead of working through the migration.
	fill(deploy.TaskWorkersPause, "pause cron and message consumers",
		magentoServedAppGuard+" && if [ {{settings.worker_control}} = true ]; then {{php_bin}} bin/magento cron:remove && {{php_bin}} bin/magento queue:consumers:restart; fi; fi")

	fill(deploy.TaskAppConfigure, "import application configuration",
		"cd {{release_path}} && {{php_bin}} bin/magento app:config:import --no-interaction")

	fill(deploy.TaskDBMigrate, "run setup:upgrade",
		"cd {{release_path}} && {{php_bin}} bin/magento setup:upgrade --keep-generated")

	// The flush belongs to the application the web server is serving, not to the
	// release being built. With a symlink layout `current` is the release that
	// just went live, so both spellings coincide; an in-place docroot is a
	// *different* directory the release copies into, and a flush in the release
	// clears a cache nothing reads while the live application keeps the old one.
	// Measured on a real in-place target (app/magento2-test-instance, release 16):
	// the release's 27 MB var/cache was emptied and the docroot kept its own
	// 34 MB plus a 9.1 MB var/page_cache, with no cache backend but Magento's
	// file default — so the site kept serving the old configuration.
	fill(deploy.TaskAppCacheFlush, "flush caches",
		// The reload is a fragment, so it is run directly: with nothing
		// configured it renders as `true` (see deployVars), which is why there is
		// no `[ -n ... ]` guard to get wrong here.
		`cd {{current_path}} && {{php_bin}} bin/magento cache:flush && {{settings.runtime_reload_command}}`)

	fill(deploy.TaskWorkersResume, "resume cron and message consumers",
		// `cron:install` writes the application's absolute path into the crontab,
		// so it belongs to the docroot — from the release, cron would run a
		// directory no web server serves. `queue:consumers:restart` puts the
		// poison pill again (a database row, so path-independent, and it catches a
		// consumer started between the pause and now). Runs after activation, so
		// the served root always exists here and needs no guard.
		`cd {{current_path}} && if [ {{settings.worker_control}} = true ]; then {{php_bin}} bin/magento cron:install && {{php_bin}} bin/magento queue:consumers:restart; fi`)

	fill(deploy.TaskMaintenanceDisable, "disable maintenance mode",
		magentoServedAppGuard+" && {{php_bin}} bin/magento maintenance:disable; fi")

	// The downtime block runs only when the database drifted. Adobe's contract
	// for setup:db:status is the probe: exit 0 means every module is up to
	// date, 1 means code and database versions differ, 2 means an upgrade is
	// required — anything else fails the deploy rather than guessing. Compile,
	// the cache flush and the backup deliberately stay outside the gate: the
	// compiler catches DI errors early, code changes need a flush even with a
	// current schema, and the backup is the rollback insurance.
	recipe.MigrationProbe = &deploy.MigrationProbe{
		Title:   "check whether the database schema is current",
		Command: "cd {{release_path}} && {{php_bin}} bin/magento setup:db:status",
	}
	for _, id := range []string{
		deploy.TaskMaintenanceEnable, deploy.TaskWorkersPause, deploy.TaskAppConfigure,
		deploy.TaskDBMigrate, deploy.TaskWorkersResume, deploy.TaskMaintenanceDisable,
	} {
		task := recipe.Task(id)
		task.NeedsMigration = true
		recipe.ReplaceTask(task)
	}

	// The engine owns the path and the release record; the recipe owns the dump.
	backup := recipe.Task(deploy.TaskDBBackup)
	backup.Title = "dump the database into shared/backups"
	backup.Core = deploy.CoreDBBackup(magentoDumpCommand)
	recipe.ReplaceTask(backup)

	recipe.Restore = magentoRestoreCommand

	// The keys this recipe reads. The engine's own keys arrive already declared
	// through DefaultRecipe, so only the framework-specific ones are named here.
	recipe.Settings = append(recipe.Settings, []deploy.Setting{
		{Key: "frontend_dir", Kind: deploy.SettingArgs, Title: "the theme Tailwind directories to build (one path or a list); empty skips the frontend build"},
		{Key: "frontend_command", Kind: deploy.SettingCommand, Title: "the command run inside frontend_dir"},
		{Key: "static_jobs", Kind: deploy.SettingInt, Title: "parallelism for static content deployment"},
		{Key: "static_deploy_options", Kind: deploy.SettingArgs, Title: "extra flags for every setup:static-content:deploy pass"},
		{Key: "static_content_locales", Kind: deploy.SettingArgs, Title: "locales to deploy (string, list or map)"},
		{Key: "magento_themes", Kind: deploy.SettingArgs, Title: "themes to deploy, each with its locales (string, list or theme-to-locales map); locales are added to static_content_locales"},
		{Key: "mage_mode", Kind: deploy.SettingString, Title: "production or developer; developer skips static content"},
		{Key: "split_static_deployment", Kind: deploy.SettingBool, Title: "deploy adminhtml and frontend static content in two passes"},
		{Key: "magento_themes_backend", Kind: deploy.SettingArgs, Title: "adminhtml themes; defaults to the admin theme"},
		{Key: "static_content_locales_backend", Kind: deploy.SettingArgs, Title: "adminhtml languages; defaults to the frontend ones"},
		{Key: "worker_control", Kind: deploy.SettingBool, Title: "remove cron and stop consumers around the migration"},
		{Key: "runtime_reload_command", Kind: deploy.SettingCommand, Title: "run after the cache flush, for example an FPM reload"},
	}...)

	// Magento's installer lands on /setup/ whenever the application believes it
	// is not installed — and the deploy engine's own docs record the case where
	// every request was redirected there while the deploy reported success. A
	// followed redirect must never be able to turn that into a passing check.
	recipe.VerifyRejectPaths = []string{"/setup/"}

	// The verification the core cannot supply, because it needs the deployed
	// application rather than only its files. The in-place artifact comparison
	// this recipe used to declare is gone: the engine now dry-runs the activation
	// copy for every sync path, which covers the whole tree instead of one marker
	// file the activation had just written from the release itself.
	recipe.Checks = []deploy.Check{
		{
			ID:    "app",
			Title: "the application answers against its real dependencies",
			// setup:db:status reads app/etc/env.php and queries the database, and
			// reports a schema that is out of date. `--version` succeeds with
			// neither a working env.php nor a reachable database, which is
			// exactly the failure this check exists to catch.
			Command: "cd {{release_path}} && {{php_bin}} bin/magento setup:db:status",
		},
	}

	// What the recipe's own commands need from a sandbox container. The list is
	// the application's, not govard's: a different framework asks for a
	// different set, and nothing here is interpreted by the core.
	recipe.Sandbox = deploy.SandboxRequirements{
		Packages: []string{"libxslt1-dev", "libzip-dev", "libpng-dev", "libjpeg-dev", "libfreetype6-dev", "default-mysql-client"},
		// `curl` is not optional: Magento's own composer platform check requires
		// ext-curl, so without it a real project stops at build:vendors — a first
		// trial against a real project failed exactly there, and the rest of this
		// list was already satisfied (composer check-platform-reqs).
		Extensions: []string{"bcmath", "curl", "gd", "intl", "mysql", "soap", "sockets", "xsl", "zip"},
		// Both services are ones a real Magento target has, and both are needed
		// for the rehearsal to mean anything: the database for `setup:upgrade`,
		// and the cache because an env.php written for a server names a Redis (or
		// Valkey) for cache and sessions — without it the target cannot start
		// `bin/magento` at all, so the rehearsal would need a hand-edited env.php
		// that no server would have.
		//
		// The names are the distribution's init scripts, not the product names:
		// Debian installs `/etc/init.d/redis-server`, and the entrypoint starts what
		// it is told and nothing else. `redis` reads better and starts nothing —
		// found by asking a fresh sandbox whether its cache answered.
		Services: []string{"mariadb", "redis-server"},
	}
	return recipe
}

// magentoServedAppGuard is the opening of every command that must act on the
// application the web server serves: both maintenance commands and the worker
// pause. It asks whether the *application* is being served, not whether a
// directory exists: the flags those steps write are read from the docroot the web
// server reads, and a docroot that cannot run — no `bin/magento`, or no installed
// dependencies behind it — has no window to open and nothing to protect.
//
// The dependency half is what a first in-place deploy onto a fresh docroot needs.
// `sandbox reset --docroot real` seeds the docroot as a checkout of the revision,
// which is exactly what the reference tool leaves behind on a target that has
// never been deployed to; running Magento there fails with `Autoload error: Vendor
// autoload is not found`, and the deploy died at `maintenance:enable` — before the
// activation that would have copied `vendor/` in and repaired it. Found by running
// the in-place path against a real project. The same first deploy has no running
// consumers to stop, which is why `app:workers:pause` shares the guard.
//
// The marker is the autoloader, not the directory. This project commits
// `vendor/.htaccess`, as Magento projects do, so a checkout that has never been
// built *does* have a `vendor/` directory — `[ -d vendor ]` passed the guard and
// the step failed anyway. `vendor/autoload.php` is what `bin/magento` actually
// requires, so that is what is asked for.
//
// A docroot with a working application is unaffected: it has both.
const magentoServedAppGuard = "if [ -f {{current_path}}/bin/magento ] && [ -f {{current_path}}/vendor/autoload.php ]; then cd {{current_path}}"

// magentoDumpCommand writes a plain SQL dump to {{backup_path}}. It reads the
// connection settings from app/etc/env.php through bin/magento, so credentials
// are never duplicated into the project configuration.
//
// The file the tool writes is `<timestamp>_db.sql` — `.sql.gz` only when the
// project configures backup compression — and this command used to look for
// `*.gz` alone: on a real project the dump succeeded, the glob matched nothing,
// `test -n` failed, and `>/dev/null` had thrown away the tool's own output, so
// the deploy said nothing but "exit 1". It now takes whichever of the two the
// tool wrote and lets its output through — the engine bounds what it keeps, so a
// failure carries its own explanation.
//
// It *moves* that file rather than copying it. `var/backups` is a shared
// directory, and nothing ever pruned it: every `--db-backup` deploy and every
// `rollback --with-db` left a full dump — customer data, admin hashes — behind
// forever, next to the copy govard kept. The file this run produced is
// identified by a marker taken before the command, never by "newest mtime": an
// operator who runs a manual `setup:backup` moments earlier would otherwise lose
// their own dump to the engine's retention.
//
// `setup:backup` toggles maintenance mode around the dump, which is safe inside
// the deploy's window: Magento's own MaintenanceModeEnabler records that the flag
// was already on and skips disabling it (verified in the vendored
// framework/App/Console/MaintenanceModeEnabler.php of a real release).
const magentoDumpCommand = "cd {{release_path}} && marker=\"$(mktemp)\" || exit 1; rc=0; " +
	"{{php_bin}} bin/magento setup:backup --db --no-interaction || rc=$?; " +
	"if [ \"$rc\" -eq 0 ]; then latest=\"$(find var/backups -maxdepth 1 -type f -newer \"$marker\" \\( -name '*_db.sql' -o -name '*_db.sql.gz' \\) -printf '%T@ %p\\n' 2>/dev/null | sort -rn | head -n 1 | cut -d' ' -f2-)\"; fi; " +
	"rm -f \"$marker\"; " +
	"[ \"$rc\" -eq 0 ] && test -n \"$latest\" && mv \"$latest\" {{backup_path}}"

// magentoRestoreCommand loads a dump back over the live database. It is only
// reached through `govard deploy rollback --with-db`, which is why the
// destructive path is behind an explicit confirmation.
//
// The copy has to go back: leaving it is a full dump per rollback in the shared,
// never-pruned `var/backups`, which is the other half of why the dump command
// moves its file out. It is removed whether the tool succeeded or failed, and the
// tool's own exit code is what the step reports.
//
// `setup:rollback --db-file` is not "import this file". Magento validates the
// name against `/[0-9]_db.*\.sql$/` and looks it up inside the release's
// `var/backups/`:
//
//	if (!$rollbackFile || preg_match('/[0-9]_(db)(.*?).(sql)$/', $rollbackFile) !== 1) {
//	    throw new LocalizedException('The rollback file is invalid. ...');
//	}
//	if (!$this->file->isExists($this->backupsDir . '/' . $rollbackFile)) { ... }
//
// (Magento\Framework\Setup\BackupRollback::dbRollback, read in a real release's
// vendor tree.) A dump copied to `shared/backups/deploy/<n>/dump.sql` matches
// neither, so the restore failed with "The rollback file is invalid" — which is
// what running `rollback --with-db` against a real target showed. The dump is
// therefore copied back under a Magento-shaped name inside `var/backups`, where
// the release already links that directory from `shared/`.
const magentoRestoreCommand = "cd {{release_path}} && restore=\"$(date +%s)_db.sql\" && " +
	"cp {{backup_path}} \"var/backups/$restore\" && " +
	"{ {{php_bin}} bin/magento setup:rollback --db-file=\"$restore\" --no-interaction; rc=$?; rm -f \"var/backups/$restore\"; exit $rc; }"
