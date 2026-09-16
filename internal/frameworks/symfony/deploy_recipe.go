package symfony

import (
	"govard/internal/deploy"
)

// DeployRecipe returns Symfony's deployment recipe.
//
// Three things separate it from a generic PHP recipe, and each comes from what a
// real Symfony project does:
//
//   - Composer's `auto-scripts` runs `cache:clear` and `assets:install`. That is
//     right for a developer running `composer install` and wrong for a deploy,
//     where both have to happen on the target.
//   - `public/bundles` is gitignored and produced by `assets:install`, so the
//     built asset set has to be recreated where the application lives.
//   - Doctrine's migration command fails on a project with no migrations yet,
//     which is a healthy project, so the recipe asks it not to.
//
// Maintenance mode is deliberately absent: Symfony has no core mechanism, and an
// empty task is reported as skipped by the engine. `db:migrate` therefore runs
// without a window, which the case-studies page states.
func DeployRecipe() deploy.Recipe {
	recipe := deploy.DefaultRecipe()
	recipe.ID = "symfony"
	recipe.Defaults = map[string]any{
		// `.env.local` is the machine's environment file and is gitignored in a
		// real project. `var/cache` is deliberately *not* shared: the compiled
		// container is built per release and for one environment.
		"shared_files":  []string{".env.local"},
		"shared_dirs":   []string{"var/log"},
		"writable_dirs": []string{"var"},
		// Both are gitignored in a real project and both are produced by the
		// release, which is the definition of a path the in-place strategy has to
		// copy in.
		"sync_paths": []string{"vendor", "public/bundles"},

		"frontend_dir":           deploy.ArgsSpec{Words: true},
		"frontend_command":       "npm ci && npm run build",
		"worker_control":         false,
		"runtime_reload_command": "",
		"writable_mode":          "chmod",
		"owner":                  "",
		// The environment a deploy runs under is the deploy's decision: a
		// committed `.env` saying `APP_ENV=dev` is a development default, not an
		// instruction to production.
		"symfony_env": "prod",
	}

	fill := func(id, title, command string) {
		task := recipe.Task(id)
		task.Title = title
		task.Command = command
		recipe.ReplaceTask(task)
	}

	fill(deploy.TaskVendors, "install Composer dependencies",
		"cd {{release_path}} && {{composer_bin}} install --no-dev --optimize-autoloader --no-interaction --prefer-dist --no-scripts")

	// NeedsApplication: the links `assets:install` writes resolve against the
	// vendor tree that is actually present, so the step runs where the
	// application does — the same rule that keeps Magento's static content off
	// the build machine. `--relative` was measured on the real project: the link
	// it writes is `etl -> ../../vendor/sutunam/etl-bundle/src/Resources/public/`,
	// which resolves from wherever the docroot ends up.
	assets := recipe.Task(deploy.TaskAssets)
	assets.Title = "install the bundle assets"
	assets.Command = "cd {{release_path}} && {{php_bin}} bin/console assets:install public --symlink --relative"
	assets.NeedsApplication = true
	recipe.ReplaceTask(assets)

	fill(deploy.TaskFrontend, "build frontend assets",
		`cd {{release_path}} && for dir in {{settings.frontend_dir_args}}; do (cd "$dir" && {{settings.frontend_command}}) || exit 1; done`)

	// `--allow-no-migration` is required rather than defensive: a project with an
	// empty migrations directory is a healthy project, and without the flag
	// Doctrine exits non-zero on it.
	fill(deploy.TaskDBMigrate, "run the Doctrine migrations",
		"cd {{release_path}} && {{php_bin}} bin/console doctrine:migrations:migrate "+
			"--env={{settings.symfony_env}} --no-interaction --allow-no-migration")

	// The two-step cache build: clear writes the container, warmup fills it, and
	// `--no-warmup` keeps the first command from doing the work twice.
	fill(deploy.TaskAppCacheFlush, "rebuild the container cache",
		"cd {{release_path}} && {{php_bin}} bin/console cache:clear --env={{settings.symfony_env}} --no-warmup && "+
			"{{php_bin}} bin/console cache:warmup --env={{settings.symfony_env}} && {{settings.runtime_reload_command}}")

	// Symfony's own graceful stop for Messenger consumers: each one finishes the
	// message it holds and exits. Restarting them is the process manager's job,
	// which is why there is no resume step.
	fill(deploy.TaskWorkersPause, "pause the Messenger consumers",
		`cd {{release_path}} && if [ {{settings.worker_control}} = true ]; then `+
			`{{php_bin}} bin/console messenger:stop-workers --env={{settings.symfony_env}}; fi`)

	// The one check the core cannot supply: the application has to answer against
	// its real dependencies. DoctrineBundle renamed `doctrine:query:sql` to
	// `dbal:run-sql`, so the branch is chosen by asking the console rather than by
	// guessing a version.
	recipe.Checks = []deploy.Check{
		{
			ID:    "app",
			Title: "the application answers against its real dependencies",
			Command: "cd {{release_path}} && if {{php_bin}} bin/console list --raw 2>/dev/null | grep -q '^dbal:run-sql'; " +
				`then {{php_bin}} bin/console dbal:run-sql "SELECT 1" --env={{settings.symfony_env}}; ` +
				`else {{php_bin}} bin/console doctrine:query:sql "SELECT 1" --env={{settings.symfony_env}}; fi`,
		},
	}

	recipe.Settings = append(recipe.Settings, []deploy.Setting{
		{Key: "frontend_dir", Kind: deploy.SettingArgs, Title: "the directories whose frontend assets are built (one path or a list); empty skips the build"},
		{Key: "frontend_command", Kind: deploy.SettingCommand, Title: "the command run inside frontend_dir"},
		{Key: "worker_control", Kind: deploy.SettingBool, Title: "stop the Messenger consumers around the migration"},
		{Key: "runtime_reload_command", Kind: deploy.SettingCommand, Title: "run after the cache is rebuilt, for example an FPM reload"},
		// No Enum: a project may name its production environment anything, and
		// refusing a legitimate name would be worse than accepting a typo the
		// console reports on its first run.
		{Key: "symfony_env", Kind: deploy.SettingString, Title: "the console environment the deploy runs under (not validated; a wrong name builds the wrong cache directory)"},
	}...)

	recipe.Sandbox = deploy.SandboxRequirements{
		Packages:   []string{"default-mysql-client"},
		Extensions: []string{"intl", "mysql", "mbstring", "xml", "curl", "zip"},
		Services:   []string{"mariadb", "redis-server"},
	}
	return recipe
}
