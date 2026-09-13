package wordpress

import (
	"govard/internal/deploy"
)

// DeployRecipe returns WordPress's deployment recipe for the classic layout: the
// repository holds the core files and `wp-content/`, and the docroot is the
// repository root. That is the layout govard already answers for elsewhere (an
// empty `NGINXPUBLIC`, media under `wp-content/uploads`) and the one a real
// project uses.
//
// Three tasks are hybrids: `wp` when the target has wp-cli, and a PHP bootstrap
// through `wp-load.php` when it does not. wp-cli is what a real server runs and
// what an operator recognises, but it is not something govard can install on a
// target, and a deploy that needs a binary the target does not have fails for a
// reason that has nothing to do with the release.
func DeployRecipe() deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	recipe.ID = "wordpress"
	recipe.Extends = "default"
	recipe.Defaults = map[string]any{
		// The target's own configuration: a repository `wp-config.php` names the
		// development database service, which is exactly the file that must not
		// reach production.
		"shared_files": []string{"wp-config.php"},
		// Uploads is the only directory that is guaranteed to hold data no
		// release owns. `.htaccess` is deliberately absent: WordPress rewrites it,
		// and the cache step regenerates the rules.
		"shared_dirs":   []string{"wp-content/uploads"},
		"writable_dirs": []string{"wp-content/uploads", "wp-content/cache", "wp-content/upgrade", "wp-content/languages"},
		// Only read by the in-place strategy, and only meaningful for a project
		// that manages plugins or themes through Composer.
		"sync_paths": []string{"vendor"},

		"frontend_dir":           deploy.ArgsSpec{Words: true},
		"frontend_command":       "npm ci && npm run build",
		"worker_control":         false,
		"runtime_reload_command": "",
		"writable_mode":          "chmod",
		"owner":                  "",
	}

	fill := func(id, title, command string) {
		task := recipe.Task(id)
		task.Title = title
		task.Command = command
		recipe.ReplaceTask(task)
	}

	// A classic checkout usually has no composer.json, so the step is guarded
	// rather than unconditional. The guard is a test, not `|| true`: a project
	// that has one and fails to install must fail the deploy.
	fill(deploy.TaskVendors, "install Composer dependencies when the project has them",
		"cd {{release_path}} && if [ -f composer.json ]; then "+
			"{{composer_bin}} install --no-dev --optimize-autoloader --no-interaction --prefer-dist; fi")

	fill(deploy.TaskFrontend, "build frontend assets",
		`cd {{release_path}} && for dir in {{settings.frontend_dir_args}}; do (cd "$dir" && {{settings.frontend_command}}) || exit 1; done`)

	fill(deploy.TaskDBMigrate, "run the core database upgrade",
		`cd {{release_path}} && `+wordpressWpGuard+` wp core update-db; else `+wordpressPHPLoad+
			`require ABSPATH . "wp-admin/includes/upgrade.php"; wp_upgrade();'; fi`)

	fill(deploy.TaskAppCacheFlush, "flush the object cache and the rewrite rules",
		`cd {{release_path}} && `+wordpressWpGuard+` wp cache flush && wp rewrite flush --hard; else `+wordpressPHPLoad+
			`wp_cache_flush(); flush_rewrite_rules(true);'; fi`)

	// Maintenance is written to the application the web server is serving. See
	// wordpressMaintenanceEnable for why the timestamp is written ahead of the
	// clock.
	fill(deploy.TaskMaintenanceEnable, "enable maintenance mode", wordpressMaintenanceEnable)
	fill(deploy.TaskMaintenanceDisable, "disable maintenance mode", wordpressMaintenanceDisable)

	// The engine owns the path and the release record; the recipe owns the dump.
	// wp-cli reads the connection from wp-config.php, so credentials are never
	// duplicated into the project configuration.
	backup := recipe.Task(deploy.TaskDBBackup)
	backup.Title = "dump the database with wp-cli"
	backup.Core = deploy.CoreDBBackup("cd {{release_path}} && wp db export {{backup_path}}")
	recipe.ReplaceTask(backup)

	recipe.Restore = "cd {{release_path}} && wp db import {{backup_path}}"

	// The one check the core cannot supply: the site has to be installed against
	// its database. `wp core is-installed` asks WordPress itself, and the fallback
	// asks the same function the installer uses.
	recipe.Checks = []deploy.Check{
		{
			ID:    "app",
			Title: "WordPress is installed and answers against its database",
			Command: `cd {{release_path}} && ` + wordpressWpGuard + ` wp core is-installed; else ` + wordpressPHPLoad +
				`exit(is_blog_installed() ? 0 : 1);'; fi`,
		},
	}

	recipe.Settings = append(recipe.Settings, []deploy.Setting{
		{Key: "frontend_dir", Kind: deploy.SettingArgs, Title: "the directories whose frontend assets are built (one path or a list); empty skips the build"},
		{Key: "frontend_command", Kind: deploy.SettingCommand, Title: "the command run inside frontend_dir"},
		{Key: "runtime_reload_command", Kind: deploy.SettingCommand, Title: "run after the caches are flushed, for example an FPM reload"},
	}...)

	recipe.Sandbox = deploy.SandboxRequirements{
		// `wp db export` shells out to mysqldump, and `wp core update-db` needs
		// the database server the profile provides.
		Packages:   []string{"default-mysql-client"},
		Extensions: []string{"mysqli", "curl", "gd", "intl", "mbstring", "xml", "zip"},
		Services:   []string{"mariadb", "redis-server"},
		Tools:      []string{"wp-cli"},
	}
	return recipe
}

// wordpressWpGuard opens the wp-cli half of a hybrid command.
const wordpressWpGuard = `if command -v wp >/dev/null 2>&1; then`

// wordpressPHPLoad boots WordPress without wp-cli: the same `wp-load.php` entry
// point wp-cli itself uses, with the theme layer switched off so the command
// never renders a page. It opens a quoted PHP fragment, so every string inside it
// is double-quoted and the whole fragment contains no single quote — which is
// what keeps `{{php_bin}} -r '...'` one shell word.
const wordpressPHPLoad = `{{php_bin}} -r 'define("WP_USE_THEMES", false); require "wp-load.php"; `

// wordpressMaintenanceEnable opens the maintenance window.
//
// Two files, and both are WordPress's own mechanism: `.maintenance` in the
// docroot is what `wp_is_maintenance_mode()` looks for, and
// `wp-content/maintenance.php` is the drop-in `wp_maintenance()` serves with a
// 503 when it exists.
//
// The timestamp is the part that is easy to get wrong. WordPress itself writes
// `$upgrading = time()`, and `wp_is_maintenance_mode()` treats a flag older than
// ten minutes as expired — so a deploy window longer than ten minutes would
// silently reopen the site in the middle of a migration, and a failure at minute
// nine would put traffic back on a half-migrated database. Writing the timestamp
// ahead of the clock keeps the window open until the deploy closes it.
//
// The drop-in is written only when the project does not ship one, and it carries
// a marker so the disable step can tell its own file from the project's.
const wordpressMaintenanceEnable = "if [ -f {{current_path}}/wp-load.php ] && [ -f {{current_path}}/wp-includes/version.php ]; then cd {{current_path}} && " +
	`printf '%s\n' '<?php $upgrading = time() + 86400;' > .maintenance && ` +
	`if [ ! -f wp-content/maintenance.php ]; then printf '%s\n' '` + wordpressMaintenanceDropIn + `' > wp-content/maintenance.php; fi; fi`

// wordpressMaintenanceDisable closes it. `rm -f` on both paths makes the step
// idempotent, so a resumed or in-place deploy that never enabled maintenance
// still succeeds; the drop-in is removed only when it carries the recipe's
// marker, so a maintenance page the project committed is left alone.
const wordpressMaintenanceDisable = `rm -f {{current_path}}/.maintenance {{release_path}}/.maintenance && ` +
	`for file in {{current_path}}/wp-content/maintenance.php {{release_path}}/wp-content/maintenance.php; do ` +
	`if [ -f "$file" ] && grep -q '` + wordpressMaintenanceMarker + `' "$file"; then rm -f "$file"; fi; done`

// wordpressMaintenanceMarker identifies the drop-in the recipe wrote.
const wordpressMaintenanceMarker = "govard deploy maintenance"

// wordpressMaintenanceDropIn is the page served while the window is open. It is
// one line so it can be written with a single printf, and it contains no single
// quote so the shell sees one word.
const wordpressMaintenanceDropIn = `<?php /* ` + wordpressMaintenanceMarker + ` */ http_response_code(503); ` +
	`header("Retry-After: 600"); header("Content-Type: text/html; charset=utf-8"); ` +
	`echo "<!doctype html><title>Maintenance</title><h1>Briefly unavailable for scheduled maintenance.</h1>";`
