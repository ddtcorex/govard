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
		"frontend_dir":           "",
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
		// passed through verbatim so raw flags keep working.
		"static_content_locales": deploy.ArgsSpec{Flag: "--language"},
		"magento_themes":         deploy.ArgsSpec{Flag: "-t"},
	}

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

	// Tailwind/Hyva build. Skipped unless the project points at its theme
	// directory, which is what keeps the stock-theme case dependency-free.
	fill(deploy.TaskFrontend, "build frontend assets",
		`cd {{release_path}} && if [ -n {{settings.frontend_dir}} ]; then cd {{settings.frontend_dir}} && {{settings.frontend_command}}; fi`)

	fill(deploy.TaskAssets, "deploy static content",
		`cd {{release_path}} && if [ "{{settings.mage_mode}}" != "developer" ]; then {{php_bin}} bin/magento setup:static-content:deploy -f --content-version={{settings.content_version}} -j {{settings.static_jobs}} {{settings.static_content_locales_args}} {{settings.magento_themes_args}}; fi`)

	fill(deploy.TaskMaintenanceEnable, "enable maintenance mode",
		"cd {{release_path}} && {{php_bin}} bin/magento maintenance:enable")

	fill(deploy.TaskWorkersPause, "pause cron and message consumers",
		`cd {{release_path}} && if [ "{{settings.worker_control}}" = "true" ]; then {{php_bin}} bin/magento cron:remove && {{php_bin}} bin/magento queue:consumers:stop; fi`)

	fill(deploy.TaskAppConfigure, "import application configuration",
		"cd {{release_path}} && {{php_bin}} bin/magento app:config:import --no-interaction")

	fill(deploy.TaskDBMigrate, "run setup:upgrade",
		"cd {{release_path}} && {{php_bin}} bin/magento setup:upgrade --keep-generated")

	fill(deploy.TaskAppCacheFlush, "flush caches",
		`cd {{release_path}} && {{php_bin}} bin/magento cache:flush && if [ -n {{settings.runtime_reload_command}} ]; then {{settings.runtime_reload_command}}; fi`)

	fill(deploy.TaskWorkersResume, "resume cron and message consumers",
		`cd {{release_path}} && if [ "{{settings.worker_control}}" = "true" ]; then {{php_bin}} bin/magento cron:install && {{php_bin}} bin/magento queue:consumers:restart; fi`)

	fill(deploy.TaskMaintenanceDisable, "disable maintenance mode",
		"cd {{release_path}} && {{php_bin}} bin/magento maintenance:disable")

	// The engine owns the path and the release record; the recipe owns the dump.
	backup := recipe.Task(deploy.TaskDBBackup)
	backup.Title = "dump the database into shared/backups"
	backup.Core = deploy.CoreDBBackup(magentoDumpCommand)
	recipe.ReplaceTask(backup)

	recipe.Restore = magentoRestoreCommand
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
