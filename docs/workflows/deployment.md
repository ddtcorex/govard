---
title: Deployment
description: Deploy a git revision to a remote with govard — the neutral task pipeline, framework recipes, hooks, the two-job CI shape with artifacts, rollback, and the sandbox that rehearses the whole thing on your machine.
---

# Deployment

`govard deploy` publishes one git revision to one remote environment. It needs
only SSH and rsync — no Docker, no local PHP, no other deploy tool — which is
what lets the same command run on a laptop and in a CI job.

```bash
govard deploy staging --revision <sha>   # deploy an exact commit
govard deploy staging                    # ... or the local HEAD
govard deploy plan staging               # print the plan, connect nowhere
govard deploy check staging              # preflight and report what the target implies
```

The target is a remote from `.govard.yml`. The branch, repository, deploy path
and publish strategy come from the project `deploy:` block; a remote overrides
them. `deploy_path` has no default, so a remote that omits it gets the layout the
target already has — `releases/`, `shared/`, `.dep/` or a `current` symlink — and
govard adopts it only when exactly one candidate matches, saying which. No layout
or several is a configuration error naming what was probed.

This page describes the engine and every lever it has. For worked configurations —
a Luma store, a Hyvä storefront, several themes and store views, developer versus
production mode, and a symlinked versus a real webroot — see
[Deployment case studies](/workflows/deploy-case-studies).

## Setting up a Magento project

Four things have to exist before the first deploy: a target that already serves
the project, a remote govard can reach, credentials for whatever the release
installs, and a decision about how a release becomes live. Each one fails with its
own message, and no step infers another.

### 1. The target already runs the application

`govard deploy` publishes *into* a running installation; it does not create one.
For a Magento project the target needs:

- **an installed application** — `shared/app/etc/env.php`, the file `deploy:shared`
  links into every release, naming a database, a cache backend and a session
  handler the target can reach;
- **the database behind it**, with the store configuration in it.
  `setup:static-content:deploy` stops at `The default website isn't defined` when
  the store tables are missing, and migration cannot run at all;
- **a supported search engine** where the project uses one. `setup:upgrade`
  refuses Magento's MySQL fallback outright — `Your current search engine, 'MySQL',
  is not supported` — so an ElasticSuite or OpenSearch project needs its cluster
  reachable *before* the first deploy, not after;
- **SSH** for the deploy user with `rsync` on the target, and write access to the
  layout directory;
- **credentials** for private sources, on the target (next section).

A target that has never run the application is a target no recipe can publish to.
The sandbox is the place to find that out before a server is involved.

### 2. The remote

```yaml
remotes:
  staging:
    host: m2-staging.example.com
    user: m2-staging
    port: 22
    path: /home/m2-staging/public_html      # the docroot that is served
    auth:
      method: keyfile
      key_path: ~/.ssh/staging
    branch: main                            # optional; defaults to the local HEAD
    deploy:
      path: /home/m2-staging/.deployer      # releases/, shared/, .dep/ live here
      settings:
        php_bin: php8.3
        composer_bin: composer
        php_version: "8.3"                  # what the target runs, not a preference
        owner: m2-staging:m2-staging
        writable_mode: chmod+chown
      verify:
        url: https://staging.example.com/
```

`php_version` is a gate: `deploy:check` runs `<php_bin> -r 'echo PHP_VERSION;'` on
the target and refuses the deploy when the series does not match, because a release
built by the wrong interpreter fails later and says less about why. `deploy:check`
also reports the layout it found, the publish strategy that implies, the free space
at the deploy path, and whether the repository is reachable from the target — run
it before the first deploy, and read it as the answer to "is this remote ready".

### 3. What the Magento recipe already does

The recipe fills the framework tasks and ships defaults for the shared layout, so
a stock project needs no `deploy.settings` at all:

| Setting | Default |
| --- | --- |
| `shared_files` | `app/etc/env.php`, `var/.maintenance.ip` |
| `shared_dirs` | `var/log`, `var/report`, `var/session`, `var/backups`, `var/tmp`, `pub/media`, `pub/sitemap`, `pub/static/_cache` |
| `writable_dirs` | `var`, `pub/static`, `pub/media`, `generated`, `app/etc` |
| `sync_paths` | `vendor`, `generated`, `pub/static/adminhtml`, `pub/static/frontend` — in-place publishing only |

Override what your project differs on, and nothing else:

- **frontend**: `frontend_dir` (one or more Hyvä theme paths) and
  `frontend_command` (default `npm ci && npm run build`) — see *Frontend builds*;
- **static content**: `static_jobs`, `static_content_locales`,
  `static_deploy_options`, the adminhtml/frontend split, and the theme lists — see
  *The static content split* and *Multiple stores, websites and themes*;
- **mode**: `mage_mode` (`developer` skips static content deployment) — see
  *Developer and production mode*;
- **workers**: `worker_control: true` runs `cron:remove`/`queue:consumers:stop`
  around the deploy and restores them after — see the pipeline table;
- **opcache**: `runtime_reload_command` after the cache flush, for a target whose
  opcache is reachable from the deploying user — see *Caches, opcache and the
  symlink swap*;
- **ownership**: `owner`, `writable_mode` (`chmod`, `chown`, `chmod+chown`, `acl`,
  `skip`) and `writable_permissions` — see *Permissions and ownership*.

Every key either recipe declares, with its default and its shape, is tabulated in
[Deployment case studies § Reference](/workflows/deploy-case-studies#reference-every-setting-the-framework-recipes-read).

### 4. Where Composer credentials come from

Three routes, in the order Composer resolves them, and the order matters:

| Route | How it reaches the build |
| --- | --- |
| `COMPOSER_AUTH` in the deploying environment | forwarded to `build:vendors` on the command's **standard input** (never argv, never the log) and **overrides every file on the target** |
| `auth.json` in the project | materialised into the release by `deploy:code`, so Composer reads it from the release root — this is what a project that commits one relies on |
| `shared/auth.json` on the target | only read if the release has an `auth.json` pointing at it, which means listing `auth.json` in `deploy.settings.shared_files` |

Because `COMPOSER_AUTH` wins, a **machine-wide token that is wrong for the project
is worse than none at all**: it overrides the project's own working `auth.json` and
the build fails with the source's authentication error, looking exactly like a
project that has no credentials. Export the project's credentials for the deploy,
or export nothing and let the target's own file answer.

A `git`-type package is a different problem: Composer clones it over SSH, so the
*target* needs a key and a `known_hosts` entry for the host. No environment
variable carries those.

`govard deploy check` says which route it found — `COMPOSER_AUTH is set for this
run`, `shared/auth.json exists on the target`, or a warning that a project
declaring private repositories has no credentials available.

### 5. How a release becomes live

`auto` (the default) resolves from the target: an absent or symlinked docroot
publishes by an atomic rename of a `current` symlink, an existing docroot that is a
real checkout is updated in place. Pick one explicitly with `publish: symlink` or
`publish: in_place` on the remote when the target is ambiguous — for example its
docroot is a directory that is not a checkout.

In place, the release is reset into the docroot and only `sync_paths` is copied, so
that list has to name what the release *built*; see *In-place publishing needs
`sync_paths`*. In both strategies the maintenance window opens on the release that
is being **served**, and for a symlink it closes before the swap.

### 6. The first deploy

```bash
govard deploy plan staging     # the entire task list, connecting nowhere
govard deploy check staging    # preflight: connectivity, layout, permissions, php, disk, lock
govard deploy staging --yes    # ... or --remote staging
```

Watch it with `--verbose`, which streams each command's own output under its task
and never batches it. When a step fails the run says which step, and what to do
next: a failure after the maintenance window opened keeps the lock and points at
`govard deploy --remote staging --resume`, while a failure before it releases the
lock and points at a plain retry. Exit codes are the CLI contract (`0` success, `1`
execution, `2` usage, `3` missing capability, `4` configuration), and `--json`
emits one document instead of the timeline.

### 7. Rehearse it on this machine first

```bash
govard deploy sandbox up --profile full --php 8.4   # a real target, on loopback
# provision it: credentials, shared/app/etc/env.php, a database, a search engine
govard deploy --remote sandbox --yes
govard deploy sandbox down --purge
```

The sandbox is a production deploy pointed at a container — same SSH, same mirror,
same recipe — so a failure there is a failure you would have met on the server,
without a server. It needs the same application prerequisites as any target; the
section below says what each failure means, and
[Deployment case studies](/workflows/deploy-case-studies#rehearsing-any-case-in-the-sandbox)
gives the profile and `--docroot` shape each kind of project needs.

## Laravel, Symfony and WordPress

The engine is framework-neutral; a recipe is what fills the pipeline's neutral
steps with the commands one application needs. Everything above — releases,
publishing strategies, rollback, `deploy check`, the sandbox — applies to these
three unchanged. What follows is only what each recipe adds, and the three
places where it does **not** behave like the Magento one.

| Task | Laravel | Symfony | WordPress |
| --- | --- | --- | --- |
| `build:vendors` | `composer install --no-dev --optimize-autoloader` | same, plus `--no-scripts` | only when `composer.json` exists |
| `build:assets` | — | `assets:install public --symlink --relative` (on the target) | — |
| `build:frontend` | `frontend_dir` × `frontend_command` | same | same |
| `app:configure` | `artisan storage:link` | — | — |
| `db:migrate` | `artisan migrate --force` | `doctrine:migrations:migrate --allow-no-migration` | `wp core update-db` |
| `maintenance:enable` / `disable` | `artisan down` / `up` | **empty** | two files in the served path |
| `app:cache:flush` | `artisan optimize:clear` then `optimize` | `cache:clear --no-warmup` then `cache:warmup` | `wp cache flush` + `wp rewrite flush --hard` |
| `app:workers:pause` | `artisan queue:restart` (+ `horizon:terminate`) | `messenger:stop-workers` | — |
| `db:backup` / restore | — | — | `wp db export` / `wp db import` |
| `deploy:verify` check | `artisan db:show` | `dbal:run-sql "SELECT 1"` | `wp core is-installed` |

Shared state, by framework:

| Framework | `shared_files` | `shared_dirs` | `sync_paths` (in-place only) |
| --- | --- | --- | --- |
| Laravel | `.env` | `storage` | `vendor`, `public/build` |
| Symfony | `.env.local` | `var/log` | `vendor`, `public/bundles` |
| WordPress | `wp-config.php` | `wp-content/uploads` | `vendor` |

### Where the build steps run, and what an artifact carries

In `--build=server` every step runs on the target. In `--build=artifact` the build
job runs `govard deploy build` on its own machine, and the target skips the five
build tasks the artifact replaces — **except** the ones a recipe marks *needs the
application* (`NeedsApplication`), which no build machine can produce because they
read the installed application's own configuration:

| Recipe | Build step that still runs on the target in artifact mode | What the artifact has to carry |
| --- | --- | --- |
| Magento 2 | `build:assets` (`setup:static-content:deploy`) | `vendor/`, `generated/`, the frontend build output |
| Laravel | none | `vendor/`, the frontend build output (`public/build`) |
| Symfony | `build:assets` (`assets:install public --symlink --relative`) | `vendor/`, the frontend build output |
| WordPress | none | `vendor/` when the project has one, the frontend build output |

No framework cache belongs in the artifact, and none of these recipes can put one
there: `app:cache:flush` is a publish-stage step in all four, so it always runs on
the target, where the environment the cache bakes in actually holds.

### The steps these recipes leave empty

An empty step is reported as **skipped**, never as a failure, and each one is a
decision rather than an omission. `build:compile` and `build:patches` are empty for
all three: none of them does ahead-of-time code generation, and none has a patch
step. `app:workers:resume` is empty for Laravel and Symfony — `queue:restart` and
`messenger:stop-workers` are the whole signal, and restarting the workers is the
process manager's job — while WordPress has no worker tasks at all. Symfony leaves
both maintenance steps and `app:configure` empty; Laravel and Symfony additionally
leave `db:backup` empty, which is why `--db-backup` on them is refused instead of
silently skipped.

### What each recipe asks the sandbox for {#sandbox-recipe-defaults}

These are the lists a recipe declares — the application's default, not a policy. A
project overrides any of them with `deploy.settings.sandbox_*`, and each list
**replaces** the recipe's rather than extending it:

| Recipe | `sandbox_packages` | `sandbox_extensions` | `sandbox_services` | `sandbox_tools` |
| --- | --- | --- | --- | --- |
| Magento 2 | `libxslt1-dev`, `libzip-dev`, `libpng-dev`, `libjpeg-dev`, `libfreetype6-dev`, `default-mysql-client` | `bcmath`, `curl`, `gd`, `intl`, `mysql`, `soap`, `sockets`, `xsl`, `zip` | `mariadb`, `redis-server` | — |
| Laravel | `default-mysql-client` | `bcmath`, `curl`, `gd`, `intl`, `mbstring`, `mysql`, `sqlite3`, `xml`, `zip` | `mariadb`, `redis-server` | — |
| Symfony | `default-mysql-client` | `intl`, `mysql`, `mbstring`, `xml`, `curl`, `zip` | `mariadb`, `redis-server` | — |
| WordPress | `default-mysql-client` | `mysqli`, `curl`, `gd`, `intl`, `mbstring`, `xml`, `zip` | `mariadb`, `redis-server` | `wp-cli` |

A service name from the engine's table carries the package that provides it, so
naming `postgresql` is enough to install and start it; a service the image cannot
start is named on container start rather than skipped in silence. `sandbox_tools`
accepts only the binaries the engine has an install recipe for — today that is
`wp-cli` — and an unknown name is refused when the image is rendered, not when it
fails to build.

### The three places these recipes differ from Magento's

**Symfony has no maintenance task.** Symfony has no core mechanism for it, so
`maintenance:enable` and `maintenance:disable` stay empty and the engine reports
them as skipped. `db:migrate` therefore runs against a live site. If a project
needs a window, that is a project hook:

```yaml
deploy:
  hooks:
    - { name: down, on: "maintenance:enable", position: after, order: 10, run: "touch {{current_path}}/maintenance.lock" }
    - { name: up,   on: "maintenance:disable", position: after, order: 10, run: "rm -f {{current_path}}/maintenance.lock" }
```

**WordPress maintenance is written, not commanded.** WordPress's own mechanism is
two files: `.maintenance` in the docroot, which `wp_is_maintenance_mode()` looks
for, and the `wp-content/maintenance.php` drop-in, which `wp_maintenance()`
serves with a 503. `wp_is_maintenance_mode()` treats a flag older than **ten
minutes** as expired, so the recipe writes `$upgrading = time() + 86400`:
writing the value WordPress itself writes would reopen the site in the middle of
a window longer than ten minutes, and put traffic back on a half-migrated
database if the deploy failed at minute nine. The drop-in carries a marker and
only a marked file is removed, so a project that ships its own maintenance page
keeps it.

Under the `symlink` strategy these files are written into whichever release is
live when the window opens, so after the swap they belong to the previous
release — the new one is served normally, which is correct, but a rollback to
that previous release would find it still in maintenance. `maintenance:disable`
removes them from the path it can address, and this is the manual remedy:

```bash
rm -f {{current_path}}/.maintenance {{current_path}}/wp-content/maintenance.php
```

**`db:backup` is opt-in per framework.** It defaults to off everywhere. Magento
has it through `setup:backup` and WordPress has it through `wp db export/import`;
Laravel and Symfony have **no dump command** in their recipes. Turning
`--db-backup` on for a framework without one is refused before the run starts
(exit 4), with a message naming the recipe and the remedy — deliberately, because
silently skipping it would let an operator believe a backup exists immediately
before a destructive `db:migrate`. A project that wants one adds a hook:

```yaml
deploy:
  hooks:
    - name: dump
      on: "db:backup"
      position: after
      order: 10
      run: "mysqldump --single-transaction \"$DATABASE_URL\" > {{shared_path}}/backups/manual.sql"
```

### Framework-specific notes

**Laravel.** `storage` is shared rather than merely writable because the
maintenance flag lives at `storage/framework/down`: a shared directory is what
carries it across the release swap. The framework caches are built in
`app:cache:flush`, on the target, never at build time — `artisan optimize`
writes `bootstrap/cache/config.php`, and once that file exists the process
environment no longer overrides `.env`, so a cache built elsewhere would carry
another machine's configuration to production. `worker_control` sends
`queue:restart`, which exits 0 with any cache store, so a project whose cache
cannot carry the signal needs a persistent one for the step to mean anything.
The `app` check prefers `artisan db:show` and falls back to `migrate:status` on
Laravel 10 and older, where `db:show` does not exist; the fallback exits 1 on a
project that has no migrations table yet, which is a healthy zero-migration
project.

**Symfony.** Composer's `auto-scripts` runs `cache:clear` and `assets:install`,
which is right for a developer and wrong for a deploy: both have to happen where
the application lives, so `build:vendors` passes `--no-scripts` and
`build:assets` runs `assets:install` on the target with `--relative` (the links
it writes then resolve from wherever the docroot ends up, which an in-place
deploy needs). `doctrine:migrations:migrate` takes `--allow-no-migration`,
because a project with an empty migrations directory is a healthy project.
`var/cache` is deliberately **not** shared: the compiled container belongs to one
release and one environment. `symfony_env` (default `prod`) decides the
environment the deploy runs under, because a committed `.env` saying
`APP_ENV=dev` is a development default rather than an instruction to production;
the value is not validated, so a typo builds the wrong cache directory. The
`app` check prefers `dbal:run-sql` and falls back to `doctrine:query:sql`,
chosen by asking `bin/console` rather than by guessing a DoctrineBundle version.

**WordPress.** Only the classic layout — core files and `wp-content/` in the
repository root, no `composer.json` — is supported; a Bedrock layout (core in
`vendor/`, docroot `web/`) and a content-only checkout are not. `wp-config.php`
is a shared file, and on a first deploy `shared/` is empty, so **the target's
`wp-config.php` has to be seeded before the deploy runs** or the release keeps
the repository's copy — which names the development database. `wp db export`
shells out to `mysqldump`, so a target needs both wp-cli and a MySQL client.

### What the sandbox provides for these recipes

A project whose database is not the framework's default says so in the project
layer, which is what makes it rehearsable at all — a Symfony project on PostgreSQL
replaces the recipe's lists instead of extending them:

```yaml
deploy:
  settings:
    sandbox_packages: [postgresql]
    sandbox_extensions: [intl, pgsql, mbstring, xml, curl, zip]
    sandbox_services: [postgresql, redis-server]
    sandbox_tools: [wp-cli]
```

The four lists **replace** the recipe's, rather than extending them: a project on
PostgreSQL replaces `[mariadb]`, it does not run both.

## The pipeline

Deployment is a fixed sequence of framework-neutral tasks, ordered by the engine
rather than by a recipe:

| Stage | Tasks |
| --- | --- |
| `prepare` | preflight, lock, release directory, code, shared files, permissions |
| `build` | dependencies, patches, code generation, frontend assets, static assets — or the artifact |
| `publish` | maintenance, workers, database backup, configuration, migrations, activation, caches, release record |
| `verify` | post-publish checks |
| `cleanup` | prune old releases, release the lock |

The maintenance window is opened only when it buys something: a symlink
activation is an atomic rename, so nothing serving the site is rewritten, and the
window appears only if the same plan also migrates or imports configuration. An
in-place activation always opens it, because the docroot itself is rewritten
while serving.

For a Magento project that means a symlink deploy opens the window too: the recipe
imports configuration and runs the schema upgrade on every deploy, so its plan
always migrates. The alternative is asking the target whether anything is pending
and trusting the answer — a probe the reference deploy tool's own recipe documents
as missing cases — and a window that is not needed costs a migration's worth of
downtime, while a schema change seen by the release still serving traffic costs
the site. What bounds the cost is `deploy.maintenance_timeout` (15m by default) and
the fact that the database dump inside the window is opt-in (`--no-db-backup`).

The window is opened and closed **on the release being served** (`current`), not
on the release being built. Maintenance mode is read from the docroot a request
lands in, so a flag written into the incoming release would protect nothing while
`setup:upgrade` changes the schema the live code depends on, and would switch the
site off the moment that release became live. For a symlink that also decides
where the window *ends*: it closes before the swap, because the swap changes
which release is being served — closing it afterwards would leave a flag in the
release the swap replaced, which is what a rollback would then serve. In place
there is one directory throughout, so the window stays open across the rewrite.
The guard is the served application (`bin/magento` in the served directory), not
just the directory: a first deploy has no served release at all, and an in-place
target's docroot is a git checkout that may never have been deployed to — neither
has anything to protect, so both steps are a no-op there.

A framework contributes a **recipe** that fills the tasks it supports; anything
it leaves empty is reported as skipped, not as a failure. A project customises
the pipeline by anchoring **hooks** on a task id, a stage alias or another hook:

```yaml
deploy:
  hooks:
    - { name: varnish-purge, on: "publish:activate", position: after, order: 10, run: "varnishadm ban req.url ~ /" }
```

`govard deploy plan` prints the resolved tree with each step's implementation, and
the source of every hook, so a hook's placement can be reviewed without connecting
to anything.

Every enhanced behaviour is behind a flag and the defaults are the optimised
ones: `--no-verify`, `--no-db-backup` and `--lock=false` turn work off, `--force`
re-deploys a revision the target already runs, and `--build`, `--artifact-dir`
and `--publish` change how the release is produced and published.

`deploy.settings` is validated against the recipe before anything runs: a key the
framework does not know, or a value with the wrong shape, is a configuration error
(exit 4) that names the key and suggests the near miss. A misspelling used to be a
silent no-op — `static_content_locale: en_US` deployed every locale the recipe
defaulted to. String values must be quoted if they look numeric (`php_version:
"8.2"`): the engine reads those settings as strings, so an unquoted `8.2` reads as
empty.

### Watching a running deploy

The default output is one line per task when it finishes: quiet enough for a CI log,
and enough to see which step is running. Two things fill the gap while a step is in
flight:

- a **heartbeat** every ten seconds — `… build:compile still running (42s)` — for
  every step, whether or not its command prints anything. A silent `composer install`
  and a stalled transfer look identical without it, and telling them apart matters
  long before a timeout fires;
- **`--verbose`**, which streams each command's own output live, indented under the
  task it belongs to. Nothing is batched or reordered: what the command writes is what
  you see, while it writes it.

`--json` wins over `--verbose`: with both, the stream is a no-op and stdout stays
exactly the one parseable document (`--json` already moves the timeline to stderr).

Watching a command never costs the run its own record. What the engine *keeps* of a
command's output is a bounded tail (256 KiB per step, introduced by a marker that says
how much was dropped): that copy is what the error message and the release record carry,
and a step is free to print far more than that. `--verbose` is the only thing that shows
every byte, and only while it arrives. The bound is not cosmetic — a Magento static
content deploy that fails inside a theme loop printed the same exception 27,333 times,
and an unbounded copy made the release record 170 MiB, which cannot be written to the
target as a command and left `status`, `--resume` and `rollback` with no record of the
release at all.
Rsync transfers — the in-place `sync_paths` copy and the artifact upload — add
`--info=progress2` only when the output is a terminal *and* `--verbose` is on: without
a terminal rsync cannot redraw its progress line, so every update would become another
line in a log instead of a moving one.

### Stopping a deploy

`Ctrl-C` (or a `SIGTERM`) cancels the run rather than killing the process where it
stands. Govard reports the step as *the run was interrupted*, then applies the same
rule a failure does: before the maintenance window the lock is released, because
nothing live has changed and a retry must not be refused; once the window is open the
lock, the release directory and its record stay, because the release is the only thing
that says what the target is half-way through. `--resume` finishes it.

Interrupting also stops the **work**, not just govard's own bookkeeping. Every step is
a shell chain — <span v-pre>`cd {{release_path}} && composer install …`</span> — so the process govard
starts is a shell and the compile, the install or the transfer is its child. A local
step runs in its own process group: the group is sent `SIGTERM` when the run is
cancelled, and anything that ignores it is killed as soon as the stopped command
returns. Over SSH, killing the local client stops nothing on the other machine, so the
step first records the remote shell's pid — `sshd` gives it a session and process group
of its own — and the cancel path signals that group over a second, short-lived
connection. The record is removed when the step ends — including a step that replaces its own
shell with `exec` or installs its own `EXIT` trap — and by the teardown when the step
is interrupted, so a normal run leaves nothing behind.

On Windows there is no process group to signal and govard does not create a job
object, so a cancelled step kills the shell govard started and a child of that shell
may outlive the run. Interrupting is a request to stop, not a guarantee that a step
which had already started did — check with `govard deploy releases` and `status`
before starting another attempt.

### Developer and production mode

`mage_mode` is the one setting that changes *what the deploy does* rather than how it
does it, so it is worth being explicit about in every project:

| `mage_mode` | `build:assets` runs | When to use it |
| --- | --- | --- |
| not set (the default) | yes | production behaviour; equivalent to `production` |
| `production` | yes | a target whose static content is compiled at deploy time |
| `developer` | no | a target Magento generates static files for on demand |

The recipe's guard is a plain string comparison, so an empty value behaves exactly
like `production`; only the literal `developer` skips the step. A developer-mode
deploy is therefore minutes faster on a large storefront — no
`setup:static-content:deploy` at all — and the application compiles what a page needs
on first request.

```yaml
deploy:
  settings:
    mage_mode: developer      # staging a team browses; skip static content
```

Two consequences worth stating plainly:

- **`developer` is wrong on a production target** unless its `env.php` sets
  `static_content_on_demand_in_production`. A production `env.php` normally leaves it
  off, so a theme whose static files were never deployed answers `404` for them. A
  shared staging target whose performance is measured should also use `production`,
  because on-demand generation changes the numbers.
- **The split does not apply in developer mode.** `split_static_deployment` only
  describes *how* the static content passes are arranged, and in developer mode there
  are no passes. The recipe's own verification follows suit: the in-place static
  content version check is guarded on the version file existing, so it passes
  vacuously when nothing was deployed.

`govard deploy plan <remote>` shows which mode the guard will compare, before
anything connects: the static content step is present either way, and the command the
plan prints carries the comparison, for example `[ developer != developer ]` on a
developer-mode target.

The mode is a deploy setting, not a local environment setting: `.govard.yml`'s local
environment picks its own Magento mode for development, and the two do not have to
agree. It is also **descriptive**: govard never runs `bin/magento deploy:mode:set`, so
setting `developer` here tells the deploy what the target runs, it does not make the
target run it. Check the target with `bin/magento deploy:mode:show` and set the
setting to match.

### The static content split

`split_static_deployment` deploys the adminhtml and frontend static content in two
passes instead of one, with `--area=adminhtml` then `--area=frontend`. The admin
pass uses `magento_themes_backend` (the admin theme by default) and
`static_content_locales_backend`, which defaults to the frontend languages so the
two passes agree unless the project says otherwise. The frontend pass uses
`magento_themes` and `static_content_locales`.

`static_deploy_options` passes extra flags to every pass — `--no-parent` for a
theme whose parent is deployed on its own, `-s standard`, or `--exclude-theme`.
A string is passed through verbatim and a list contributes one word per entry:

```yaml
deploy:
  settings:
    static_deploy_options: --no-parent
```

It exists for the two cases a single pass handles badly: a theme list that covers
only frontend themes (the admin theme would be missed) and a large deployment
where one process holding every area runs out of memory. The second pass is
chained to the first, so a failed admin pass stops the deploy rather than
publishing half of the static content.

### Frontend builds (Hyvä)

A Hyvä theme is built by Node inside its own directory, which the project names
with `settings.frontend_dir`:

```yaml
deploy:
  settings:
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
    frontend_command: npm ci && npm run build   # the default
```

`frontend_command` runs inside each directory, inside the release, as one shell
command: the default chains two commands, so a single-command value such as
`npx tailwindcss -i input.css -o output.css` is written exactly the same way.
Leaving `frontend_dir` empty skips the step, which is what a Luma or stock-theme
project wants.

A project with more than one Node-built theme names all of them, and each is built
in place:

```yaml
deploy:
  settings:
    frontend_dir:
      - app/design/frontend/Acme/hyva/web/tailwind
      - app/design/frontend/Acme/other/web/tailwind
```

The directories are built in order, each in its own subshell, and the first failure
stops the deploy rather than letting the next theme's build hide it. Every entry is
quoted, so one list entry is exactly one directory: a path that contains a space
has to be written as a list entry, because the single-path form is read as a
whitespace-separated list.

Node is needed where the *build* runs, not where the deploy runs: with
`--artifact-dir` the themes are built in the CI build job and `build:frontend` is
skipped on the target, so the deploy job's image stays govard + ssh + rsync.

[Deployment case studies](/workflows/deploy-case-studies) works through one Hyvä
theme (case 3), the same theme in developer mode (case 4) and two Node-built themes
(case 5), with the configuration and the sandbox command for each.

### Multiple stores, websites and themes

A multi-store project deploys one release that serves every website, so the deploy
set is **themes × locales**: every store view's theme has to be in
`magento_themes` and every store view's locale in `static_content_locales`.
A storefront whose theme or locale was not built has no static files, and
production mode answers `404` for them: Magento only re-publishes a missing static
resource on demand when `env.php` sets `static_content_on_demand_in_production`,
which is a per-request PHP cost rather than a substitute for deploying.

```yaml
deploy:
  settings:
    # Every theme a store view uses, each with the locales it serves.
    magento_themes:
      Acme/hyva: [en_US, fr_CA]
      Acme/other: [de_DE]
      Magento/luma: [en_US]
    # Optional, and additive: the list applies to every theme above.
    static_content_locales: [en_US]
```

The locales of a theme map are **added to** `static_content_locales`, and the
result is deployed for every theme in the map. That is deliberate and it is a
property of the application, not a simplification: one
`setup:static-content:deploy` invocation resolves `--language` once for the whole
run, so a single invocation cannot compile theme A for one set of locales and theme
B for another. Narrowing per theme would mean one invocation per locale group; the
union is what the command can express, and it is the safe direction — an extra
locale costs build time, a missing one costs a storefront.

The split still applies: `split_static_deployment` sends `magento_themes_backend`
(and `static_content_locales_backend`, which defaults to the frontend locales) to
the adminhtml pass and `magento_themes` to the frontend pass.

What the rest of the pipeline already covers for several websites:

- `app/etc/env.php` and `pub/media` are **shared** across releases, so per-scope
  configuration and media survive the swap;
- `app:config:import` applies the configuration that lives in `config.php` and
  `env.php`, which is where per-scope settings belong when they are versioned;
- `setup:upgrade`, the cache flush and worker control are global, which matches the
  single database every website shares.

What govard deliberately does not do: it never writes per-store configuration into
the database. Base URLs, scope configuration and anything else an operator changes
in the admin are the application's data, not the release's, and a deploy that
rewrote them would be a deploy that can overwrite a live storefront's settings.
`deploy.verify.url` checks one URL; a multi-store project that wants every
storefront checked should anchor a hook on `verify` and run the checks it wants.

[Deployment case studies § Case 6](/workflows/deploy-case-studies#case-6-multi-store-hyva-storefront-with-a-luma-admin)
is this section applied to a real project: a Hyvä storefront with a Luma admin,
the split enabled, the union-of-locales rule, and the verify hook that checks every
storefront.

### Permissions and ownership

`writable_dirs` lists the paths the application must be able to write, and
`writable_mode` decides how they are made writable:

| Mode | What it does |
|---|---|
| `chmod` (default) | `chmod -R` with `writable_permissions` (`0775`) |
| `chown` | `chown -R` to `owner` |
| `chmod+chown` | both |
| `acl` | `setfacl` access *and* default entries for `owner` |
| `skip` | nothing — for a target where an image or a provisioning step already set them |

`owner` is `user` or `user:group`, and the chown modes and `acl` require it: an
unknown owner produces a release the web server cannot read, which is worse than
refusing. A mode outside the table is a configuration error (exit 4), refused while
the settings are validated rather than as a step failure halfway through a deploy.

`acl` is the mode that also covers the files the application creates *later*: the
default ACL is inherited, so `var/`, `pub/static/` and `generated/` stay writable
after the deploy without a `chown -R` over the release. It needs `setfacl` on the
target, and `deploy check` refuses the deploy before the release directory exists
when it is missing.

### Caches, opcache and the symlink swap

The release flushes the application cache as part of the pipeline, so the new
release never serves a cache built by the old code. What a cache flush does not
touch is PHP's own state: after a swap, a worker that already resolved `current`
can keep the old release in its `realpath_cache` and its compiled files in
opcache for up to `realpath_cache_ttl`. That is how a deploy looks successful and
still serves the previous release's code.

`settings.runtime_reload_command` is the supported way to clear that state. It
runs as the last part of the `app:cache:flush` step — inside the window in place,
after the swap for a symlink:

```yaml
deploy:
  settings:
    runtime_reload_command: cachetool opcache:reset && cachetool stat:clear
```

Resetting opcache and the realpath cache is preferred to reloading PHP-FPM: a
reload can drop requests that are already in flight, which is why the reference
deploy tool's own PHP-FPM recipe warns against it and points at this cache reset
instead. The setting is a raw shell command, so anything equivalent works where
opcache is not reachable from the deploying user — a PHP-FPM reload, a container
restart hook, or the same command through a different tool.

### Private Composer repositories

A release built on the target installs its dependencies there, so it needs
credentials for any repository that is not packagist. The three routes and their
precedence are in *Where Composer credentials come from*: `COMPOSER_AUTH` from the
deploying environment is forwarded to the dependency step on standard input — not
in the command, so it stays out of the target's process list and out of the deploy
log — and it overrides every file on the target. `shared/auth.json` is read only
when the project lists `auth.json` in `shared_files`, which links it into the
release. Govard stores no credentials of its own.

`govard deploy check` reports which source is in play, and warns when the project
declares a private repository and none is available. It warns rather than refuses:
the declaration it can read is a URL, and a URL cannot tell it whether a repository
needs credentials. Set `COMPOSER_AUTH` in the CI build job as well as in the deploy
job when the artifact is built there.

## Build modes

`--build=auto` (the default) resolves **by presence**, never by sniffing the
environment: an artifact directory means the build already happened, otherwise
the target builds.

| Mode | Where the build runs | Use it when |
| --- | --- | --- |
| `server` | on the target | a hotfix from a laptop, or a project with no CI |
| `artifact` | on the machine running `govard deploy build` | CI, so the deploy job needs no toolchain |

### What an artifact can and cannot carry

The build job runs the same recipe the server build runs, minus the steps that
ask the application about itself. Static content deployment is the one that
matters: `setup:static-content:deploy` reads the store, website and locale
configuration out of the database, so a machine that has no application, no
`app/etc/env.php` and no database cannot run it — naming the themes and locales
explicitly does not change that, because the store it asks about is still in the
database.

A recipe marks those steps, and artifact mode leaves them **in the deploy**: the
target runs them after `deploy:artifact` has unpacked the artifact, which is the
same place a server build runs them. So the split is:

| Runs on the build machine | Runs on the target |
| --- | --- |
| Composer install, patches, DI compile, frontend (node) build | static content, `setup:upgrade`, configuration import, cache flush, verification |

The deploy job still needs no toolchain of its own — it needs govard, ssh and
rsync; the target runs the application steps over SSH, as it always did.

An artifact also never carries the paths the recipe declares **shared**
(`shared_files`, `shared_dirs`): those belong to the target, and `govard deploy
build` prints every one it drops. Without that, an artifact built on a machine
that happens to hold its own `app/etc/env.php` would replace the target's — a
regular file replacing the symlink `deploy:shared` made — and the release would
fail with the application's own words: `Connection "default" is not defined`.

### The two-job CI shape

```yaml
build:
  stage: build
  script:
    - govard deploy build production --output artifacts --revision $CI_COMMIT_SHA
  artifacts:
    paths: [artifacts/]

deploy:
  stage: deploy
  script:
    - govard deploy production --artifact-dir artifacts --revision $CI_COMMIT_SHA --yes
```

The `deploy` job's image needs govard, SSH and rsync and nothing else: no PHP, no
Composer, no Node, no container runtime. The five build tasks are skipped on the
target in this mode — the artifact carries what they generate — so nothing on the
server runs `composer install`, `setup:di:compile` or
`setup:static-content:deploy`. The artifact is uploaded into the release and its
manifest is checked against the files themselves, the revision being deployed and
the PHP the target runs — a CI image that does not match the server, or an
artifact whose bytes changed after the build, is refused before anything is
published. `govard deploy plan --artifact-dir artifacts` shows which branch of
the build stage is in effect.

An output directory that is not empty is refused, so a file left over from an
earlier build cannot ship. Pass `--force` to replace its contents.

For the full pipeline around these commands — integrity, lint, per-remote
single-job ships sharing one workspace, manual rollbacks — see
[CI pipelines](/workflows/ci-pipeline).

## Publishing

`--publish=auto` reads the target instead of guessing:

- the current path is missing or a symlink → **symlink**: releases live in
  `releases/<n>` and the swap is an atomic `mv -T`, so no visitor ever sees a
  half-published tree;
- the current path is a real directory → **in_place**: the docroot's objects are
  fetched during `prepare`, outside any maintenance window, then the docroot is
  reset to the exact revision, the configured paths are copied in, and the static
  content version file is written last.

`deploy check` reports which one the layout implies and why.

### In-place publishing needs `sync_paths`

An in-place activation resets the docroot to the exact revision, copies the
configured `sync_paths` from the built release into it (with `--delete`), and writes
`pub/static/deployed_version.txt` last. `git reset --hard` leaves the previous
deployment's gitignored directories where they are, so **a path that is not copied is
a path the site keeps from the old release** — new code over the old `vendor/`,
`generated/` and `pub/static/`, reported as a successful deploy.

The Magento recipe ships the list the strategy needs, and it is the list to keep:

```yaml
deploy:
  settings:
    sync_paths: [vendor, generated, pub/static/adminhtml, pub/static/frontend]
```

Three rules the engine applies to whatever the project configures:

- **a path the release did not build is skipped, not fatal** — `generated/` only
  exists after `setup:di:compile` and `pub/static/adminhtml` only when the admin area
  was deployed; the activation prints the skip;
- **a path the release links from `shared/` is not copied** — `deploy:shared` links it
  with a symlink relative to the release, which resolves elsewhere from a docroot at a
  different depth, so the docroot keeps its own copy and the step says so. This is why
  `pub/static` is named by its two built children instead of as a whole:
  `pub/static/_cache` is shared;
- **a shared path inside a synced entry is excluded from the copy** rather than
  deleted, so a project that lists `pub/static` still keeps the docroot's `_cache`.

`sync_paths: []` is allowed and means "copy nothing"; the preflight warns, because that
has to be a deliberate decision rather than an omission.

## Verification, backup and rollback

`deploy:verify` runs after publish and is on by default: the live revision
(the resolved `current` symlink, or the docroot's `HEAD` for an in-place target),
the shared files the recipe requires, the recipe's own checks, and an HTTP check
when `deploy.verify.url` is set. The recipe's checks are the ones the engine
cannot supply — for Magento that is `bin/magento setup:db:status`, which needs a
working `app/etc/env.php` *and* a reachable database, plus a comparison of the
docroot's static content version against the release when publishing in place.

Without a configured URL verification is SSH-only: it proves the right files are
in place, not that the application serves. A deploy that runs `db:migrate` with
no verify URL says so before its first step, rather than leaving the operator to
assume the opposite.

`--db-backup` dumps the database into `shared/backups/deploy/<n>/` immediately
before the first database-mutating task and records the path in the release.
`deploy:cleanup` prunes those dumps on the same `keep_releases` window as the
releases they belong to, so backups cannot accumulate forever on a production
box.

```bash
govard deploy releases staging                  # what is on the target
govard deploy status                            # what every environment serves
govard deploy rollback staging                  # put the previous release back
govard deploy rollback staging --to 12          # ... or a named one
govard deploy rollback staging --with-db --yes  # ... and its database dump
```

Rollback never rebuilds: a symlink layout is re-pointed, and an in-place layout
re-runs the publish tail from the release directory already on the server.

`deploy.lock_stale_after` (default 2h) is how old a lock must be for
`govard deploy unlock` to release it without `--force`; the refusal a held lock
produces names the holder, its revision and how long it has been held.

`deploy.maintenance_timeout` (default 15m) bounds any single step that runs with
the site in maintenance mode, which is far below `deploy.command_timeout` on
purpose: a slow step there is holding the site down. Raise it for a project whose
database dump legitimately takes longer.

`deploy.command_timeout` (default 30m) bounds every other step, and it is the one
to raise first when a deploy times out: a *cold* `composer install` of a large
project — several hundred packages, private repositories cloned over the network —
can take longer than that on a fresh target, and the failure reads `command timed
out` on that step. A timed-out step is not a special case of anything: the lock
goes back if the run had not reached the maintenance window, the record says what
stopped, and the retry resumes from a warm Composer cache on the target.

```yaml
deploy:
  command_timeout: 90m
```

A failed deploy keeps its release directory and its record. Where it failed
decides the lock: a failure in `prepare` or `build` releases it, because nothing
live has changed, so the fault can simply be fixed and the deploy retried — while
a failure from `publish` onwards keeps it, because the target may be
half-changed, and the way forward is `govard deploy <remote> --resume`, which
continues the newest unfinished release instead of starting a new one.
`--from <task>` starts at a named task or hook, and `govard deploy unlock`
releases a lock a failed run left behind.

A resume can be repeated as often as it takes, and it continues the same release
every time. A step an earlier attempt already succeeded at is not run again, and
the record keeps the `ok` that attempt stored, so a resumed run shows it as
`already done in an earlier run` and nothing is built twice. The release
directory is only refused when it does not carry govard's own record for that
release: a directory another tool created is protected, while govard's own
half-finished release is continued rather than blocked.

## Machine-readable output

`--json` writes exactly one JSON document to stdout and everything a human would
read to stderr, so a CI job can pipe stdout into a parser and keep stderr in the
log:

```json
{"schema_version":1,"remote":"production","branch":"main","revision":"0123abc…",
 "release":"42","build":{"mode":"server"},"publish":{"strategy":"symlink"},
 "verify":"ok","result":"ok","duration_ms":94300,
 "tasks":[{"id":"deploy:check","stage":"prepare","status":"ok","duration_ms":19}]}
```

A failing deploy produces the same document with `"result":"failed"` and an
`"error"` that names the task, host and command; the process exits 1. The release
record carries `ci.pipeline`/`ci.job` when the run is a CI run, which is how "which
pipeline deployed this" is answered afterwards from `govard deploy status`.

## The sandbox

`govard deploy sandbox` gives a project a real deployment target on your
machine — a container that plays the remote — so a deploy can be rehearsed
before it touches a server. Nothing in the pipeline knows the difference, which
is what makes it a rehearsal rather than a simulation.

```bash
govard deploy sandbox up                      # create it (php profile by default)
govard deploy sandbox up --profile basic      # sshd, rsync, git only
govard deploy sandbox up --profile full --php 8.4   # database, cache, web server, PHP 8.4
govard deploy sandbox status
govard deploy sandbox reset --layout deployer # seed a target the other tool owns
govard deploy sandbox ssh
govard deploy sandbox down [--purge]
```

`up` publishes SSH on a free loopback port, generates a dedicated key under
`.govard/sandbox/` (gitignored), mounts a mirror of your local repository
read-only, and writes a `sandbox` remote into `.govard.local.yml`. The mirror is
refreshed before every deploy, so a commit you have never pushed is deployable.
Because `sandbox` is also a subcommand, deploy to it with the flag form:

```bash
govard deploy --remote sandbox --yes
```

`--docroot` shapes the target so the publish strategy resolves the way you want
to exercise it: `absent` or `symlink` selects the atomic swap, `real` selects
in-place publishing. Shaping happens when the target is created and when you name
a shape, because `up` is also how a stopped sandbox is started and how the mirror
is refreshed before the next revision is deployed — neither may cost the
application currently being served. `reset` shapes unconditionally: wiping the
deploy directories and laying them out again is what it is for. `down` removes
the container and the remote it wrote; `--purge` also removes the image, the key
and the mirror.

A sandbox you already have is described by what it is, not by the flags of the
command that reached it: `up` reports the profile and the PHP series the container
was built for, and the image it actually came from, so a later `up` without
`--php` does not erase the series and does not describe a `full` sandbox as the
default profile. Naming a profile or series that disagrees is refused with the
flag that actually changes it (`--recreate`), rather than silently relabelling a
container whose image still ships the old one. `up` also waits for a real login
before it reports the sandbox ready — a published port that accepts a connection
is not a target a deploy can start against. The
`sandbox` remote block itself is rewritten from the container's state on every
`up` (host, port, paths, branch, mirror, verify URL, and the settings the profile
implies), so an edit made there by hand does not survive; put what you want to
keep in the project's own configuration instead.

The `php` and `full` profiles also ship a **web tier**: nginx serving the served
path plus the project's `stack.web_root` (`/pub` for Magento), and PHP-FPM running
as the deploy user, so the application can write the directories `deploy:writable`
hands over. `up` publishes that port on loopback too and points the sandbox
remote's `deploy.verify.url` at it, which means a sandbox deploy rehearses the
*whole* pipeline, HTTP check included — the one step a target without a web server
could never exercise.

That check is real: a target that does not answer yet fails the last step with the
HTTP status it returned (`verify http: http://127.0.0.1:PORT/ returned HTTP 403`),
which is the check doing its job rather than a defect. Provide the application
(env.php, a database, a search engine) and it passes; `--no-verify` turns it off
for a rehearsal that stops at the files.

The `full` profile starts a database and a cache, and the Magento recipe names
both, so a target's `env.php` can point at `127.0.0.1` for MariaDB and Redis/Valkey
— the shape a server has — instead of being hand-edited to use files. The `basic`
profile ships neither and advertises no verify URL.

`--php` picks the PHP series the image provides, for example `--php 8.4`; without
it the image keeps the base distribution's own version. The series comes from the
sury repository and the image's `php` binary, its extensions, `php_bin` and the
`php_version` the remote declares all follow it — so a project whose
`composer.lock` requires a newer PHP than the base image carries can be rehearsed
against the PHP its target actually runs, instead of failing in the middle of a
dependency install. The series is part of the image tag, so asking for a
different one builds a different image rather than reusing the old one.

A rehearsal is only as complete as the credentials the deployment has, and the
three routes are not interchangeable — see *Where Composer credentials come from*
above. What matters for a sandbox is that it is a fresh target: a `git`-type
package needs a key and a `known_hosts` entry *inside the container*, and a
credential that lives only in your shell profile is exactly the kind that can
override the project's own working `auth.json` and fail the build where a plain
`govard deploy` would have succeeded. Put the credentials where the target can use
them (`docker exec`, or a mounted file) and re-run `govard deploy --remote sandbox
--yes`; the failing step resumes from a clean release directory and the Composer
cache is kept.

The same is true of the application the target is supposed to be serving. A
brand-new sandbox has no installed application, so a pipeline that reaches
`build:assets` or `db:migrate` stops on a prerequisite rather than on a defect:
`setup:static-content:deploy` needs the store configuration, and `setup:upgrade`
needs a database and a *supported* search engine. `shared/app/etc/env.php` on the
target is what the release links for all of it, so a rehearsal against a real
project means writing that file (pointing at a database the target can reach, with
a cache backend and session handler it can reach) and having the data behind it —
a `govard bootstrap -e <env>` clone is the usual source. Those prerequisites are
the target's, not the engine's: a server that has never run the application cannot
publish a release to it, and the sandbox refuses to pretend otherwise.

## One connection per target

Every command a run issues shares a single SSH connection per target
(`ControlMaster` with a socket under `~/.govard/ssh/`, reused for 60 seconds after
the last command). A Magento pipeline runs around twenty commands, so the
handshake and authentication are paid once instead of twenty times — and about
half of those commands run inside the maintenance window, where a second saved is
a second of downtime avoided. The sockets are local to the machine running the
deploy, and they expire on their own; nothing is left on the target.

## Capabilities and exit codes

Every deploy command declares what it needs, and a missing requirement is exit
`3` with an actionable message before any work happens:

| Command | Requirement |
| --- | --- |
| `govard deploy` / `rollback` | `ssh,rsync` |
| `govard deploy check` / `releases` / `status` / `unlock` | `ssh` |
| `govard deploy plan` / `build` | `none` |
| `govard deploy sandbox *` | `docker` |

Exit codes: `0` success, `1` execution failure, `2` usage, `3` missing
capability, `4` configuration. The deploy job in CI therefore runs on a host
with nothing but govard, SSH and rsync.
