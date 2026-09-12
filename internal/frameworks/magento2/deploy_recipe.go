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
	recipe.Extends = "default"
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
		// Only meaningful for the in-place strategy; empty for a symlink
		// layout, where the whole release directory is published.
		"sync_paths": []string{},

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

	fill(deploy.TaskVendors, "install Composer dependencies",
		"cd {{release_path}} && {{composer_bin}} install --no-dev --optimize-autoloader --no-interaction --prefer-dist")

	// ece-patches only ships with Magento Cloud / Mage-OS projects. The guard is
	// a test, not `|| true`: a project that has the binary and fails to apply a
	// patch must fail the deploy.
	fill(deploy.TaskPatches, "apply Magento patches",
		"cd {{release_path}} && if [ -x vendor/bin/ece-patches ]; then {{php_bin}} vendor/bin/ece-patches apply; fi")

	fill(deploy.TaskCompile, "compile dependency injection",
		"cd {{release_path}} && {{php_bin}} bin/magento setup:di:compile")

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
	fill(deploy.TaskAssets, "deploy static content",
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
		"if [ -f {{current_path}}/bin/magento ]; then cd {{current_path}} && {{php_bin}} bin/magento maintenance:enable; fi")

	fill(deploy.TaskWorkersPause, "pause cron and message consumers",
		`cd {{release_path}} && if [ {{settings.worker_control}} = true ]; then {{php_bin}} bin/magento cron:remove && {{php_bin}} bin/magento queue:consumers:stop; fi`)

	fill(deploy.TaskAppConfigure, "import application configuration",
		"cd {{release_path}} && {{php_bin}} bin/magento app:config:import --no-interaction")

	fill(deploy.TaskDBMigrate, "run setup:upgrade",
		"cd {{release_path}} && {{php_bin}} bin/magento setup:upgrade --keep-generated")

	fill(deploy.TaskAppCacheFlush, "flush caches",
		// The reload is a fragment, so it is run directly: with nothing
		// configured it renders as `true` (see deployVars), which is why there is
		// no `[ -n ... ]` guard to get wrong here.
		`cd {{release_path}} && {{php_bin}} bin/magento cache:flush && {{settings.runtime_reload_command}}`)

	fill(deploy.TaskWorkersResume, "resume cron and message consumers",
		`cd {{release_path}} && if [ {{settings.worker_control}} = true ]; then {{php_bin}} bin/magento cron:install && {{php_bin}} bin/magento queue:consumers:restart; fi`)

	fill(deploy.TaskMaintenanceDisable, "disable maintenance mode",
		"if [ -f {{current_path}}/bin/magento ]; then cd {{current_path}} && {{php_bin}} bin/magento maintenance:disable; fi")

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

	// The two verifications the core cannot supply, because both need the
	// deployed application rather than only its files. Declaration order is the
	// order the verify stage runs them in.
	recipe.Checks = []deploy.Check{
		{
			ID:    "artifact",
			Title: "the docroot serves the release's static content version",
			// In place only: a symlink target publishes the whole release, so
			// `current` *is* the release directory and the file trivially
			// matches. The guard also covers a recipe that produced no version
			// file (mage_mode developer skips static content deployment).
			OnlyForPublishStrategy: deploy.PublishInPlace,
			Command: "if [ -e {{release_path}}/pub/static/deployed_version.txt ]; then " +
				"test \"$(cat {{release_path}}/pub/static/deployed_version.txt)\" = \"$(cat {{current_path}}/pub/static/deployed_version.txt)\"; fi",
		},
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
		Packages:   []string{"libxslt1-dev", "libzip-dev", "libpng-dev", "libjpeg-dev", "libfreetype6-dev", "default-mysql-client"},
		Extensions: []string{"bcmath", "gd", "intl", "mysql", "soap", "sockets", "xsl", "zip"},
		Services:   []string{"mariadb"},
	}
	return recipe
}

// magentoDumpCommand writes a plain SQL dump to {{backup_path}}. It reads the
// connection settings from app/etc/env.php through bin/magento, so credentials
// are never duplicated into the project configuration.
const magentoDumpCommand = "cd {{release_path}} && {{php_bin}} bin/magento setup:backup --db --no-interaction >/dev/null && latest=\"$(ls -1t var/backups/*.gz 2>/dev/null | head -n 1)\" && test -n \"$latest\" && cp \"$latest\" {{backup_path}}"

// magentoRestoreCommand loads a dump back over the live database. It is only
// reached through `govard deploy rollback --with-db`, which is why the
// destructive path is behind an explicit confirmation.
const magentoRestoreCommand = "cd {{release_path}} && {{php_bin}} bin/magento setup:rollback --db-file={{backup_path}} --no-interaction"
