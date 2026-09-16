package laravel

import (
	"govard/internal/deploy"
)

// DeployRecipe returns Laravel's deployment recipe.
//
// It fills the neutral task ids Laravel supports and leaves the rest empty, so
// the engine's shared implementation — release creation, the lock, the symlink
// swap, verification — is used unchanged. `build:compile` and `build:assets`
// stay empty: Laravel compiles nothing ahead of time, and its caches are built
// on the target, where the environment they bake in actually holds.
func DeployRecipe() deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	recipe.ID = "laravel"
	recipe.Defaults = map[string]any{
		// The target's environment file, and the runtime state that must outlive
		// a release. `storage` is shared rather than merely writable because the
		// maintenance flag lives in `storage/framework/down`: a shared directory
		// is what carries it across the release swap.
		"shared_files":  []string{".env"},
		"shared_dirs":   []string{"storage"},
		"writable_dirs": []string{"storage", "bootstrap/cache"},
		// Only the in-place strategy reads this: the paths a release builds
		// under a name the repository ignores.
		"sync_paths": []string{"vendor", "public/build"},

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

	fill(deploy.TaskVendors, "install Composer dependencies",
		"cd {{release_path}} && {{composer_bin}} install --no-dev --optimize-autoloader --no-interaction --prefer-dist")

	// The same per-directory loop the Magento recipe uses, for the same reason:
	// a project may build more than one frontend, and the loop is skipped
	// entirely when none is configured.
	fill(deploy.TaskFrontend, "build frontend assets",
		`cd {{release_path}} && for dir in {{settings.frontend_dir_args}}; do (cd "$dir" && {{settings.frontend_command}}) || exit 1; done`)

	// `storage:link` republishes `public/storage` for the new release. It exits 0
	// when the link already exists (measured: the second run prints an error and
	// still returns 0), so it is safe on a re-run and on an in-place docroot that
	// already has one.
	fill(deploy.TaskAppConfigure, "link the public storage disk",
		"cd {{release_path}} && {{php_bin}} artisan storage:link")

	fill(deploy.TaskDBMigrate, "run the database migrations",
		"cd {{release_path}} && {{php_bin}} artisan migrate --force --no-interaction")

	// Maintenance belongs to the application the web server is *serving*, not to
	// the release being built: Laravel writes the flag into the working
	// directory, so running it in the release would open a window nothing serves
	// and leave an incoming release switched off.
	fill(deploy.TaskMaintenanceEnable, "enable maintenance mode",
		laravelMaintenanceGuard+" && {{php_bin}} artisan down; fi")
	fill(deploy.TaskMaintenanceDisable, "disable maintenance mode",
		laravelMaintenanceGuard+" && {{php_bin}} artisan up; fi")

	// `queue:restart` is Laravel's graceful stop: every worker finishes the job
	// it holds and exits, and the process manager starts it again on the new
	// release. It runs before the migration, which is the point. It exits 0 even
	// when the cache store cannot carry the signal, so the docs say the store has
	// to be persistent for this step to mean anything.
	fill(deploy.TaskWorkersPause, "pause the queue workers",
		`cd {{release_path}} && if [ {{settings.worker_control}} = true ]; then {{php_bin}} artisan queue:restart; `+
			`if {{php_bin}} artisan list --raw 2>/dev/null | grep -q '^horizon:terminate'; then {{php_bin}} artisan horizon:terminate; fi; fi`)

	// The caches are rebuilt on the target because `optimize` writes
	// `bootstrap/cache/config.php`, and once that file exists the process
	// environment no longer overrides `.env` — measured by pointing a cached
	// release at a dead database port and watching it still connect. A release
	// that shipped such a file would carry another machine's configuration to
	// production.
	fill(deploy.TaskAppCacheFlush, "rebuild the framework caches",
		`cd {{release_path}} && {{php_bin}} artisan optimize:clear && {{php_bin}} artisan optimize && {{settings.runtime_reload_command}}`)

	// The one check the core cannot supply: the application has to answer against
	// its real dependencies. `db:show` reports the live connection and exits 0 on
	// a project with no migrations; `migrate:status` proves the same connection
	// but exits 1 when the migrations table is absent, which a healthy
	// zero-migration project is. `about --only=environment` passes with no
	// database at all, so it is not a check.
	recipe.Checks = []deploy.Check{
		{
			ID:    "app",
			Title: "the application answers against its real dependencies",
			// The branch is chosen by asking artisan rather than by guessing a
			// version: `db:show` arrived in Laravel 11.
			Command: "cd {{release_path}} && if {{php_bin}} artisan list --raw 2>/dev/null | grep -q '^db:show'; " +
				"then {{php_bin}} artisan db:show; else {{php_bin}} artisan migrate:status; fi",
		},
	}

	recipe.Settings = append(recipe.Settings, []deploy.Setting{
		{Key: "frontend_dir", Kind: deploy.SettingArgs, Title: "the directories whose frontend assets are built (one path or a list); empty skips the build"},
		{Key: "frontend_command", Kind: deploy.SettingCommand, Title: "the command run inside frontend_dir"},
		{Key: "worker_control", Kind: deploy.SettingBool, Title: "stop the queue workers around the migration (needs a persistent cache store)"},
		{Key: "runtime_reload_command", Kind: deploy.SettingCommand, Title: "run after the caches are rebuilt, for example an FPM reload"},
	}...)

	// The list is the application's, not govard's: a project that needs one more
	// extension adds it with `deploy.settings.sandbox_extensions`. `sqlite3` is
	// here because Laravel's own default connection is sqlite, so a project
	// scaffolded today cannot be rehearsed without it.
	recipe.Sandbox = deploy.SandboxRequirements{
		Packages:   []string{"default-mysql-client"},
		Extensions: []string{"bcmath", "curl", "gd", "intl", "mbstring", "mysql", "sqlite3", "xml", "zip"},
		// Both services are ones a real Laravel target has: the database for
		// `artisan migrate`, and a cache because a project's `.env` routinely
		// names one for cache, sessions and the queue restart signal.
		Services: []string{"mariadb", "redis-server"},
	}
	return recipe
}

// laravelMaintenanceGuard asks whether the *served* application is there, not
// whether a directory exists: `artisan down` writes the flag the framework reads
// at boot, and a docroot that cannot run — no `artisan`, or no installed
// dependencies behind it — has no window to open.
//
// The dependency half is what a first in-place deploy onto a fresh docroot
// needs: `artisan` exists in a checkout that has never been built, while
// `vendor/autoload.php` does not.
const laravelMaintenanceGuard = "if [ -f {{current_path}}/artisan ] && [ -f {{current_path}}/vendor/autoload.php ]; then cd {{current_path}}"
