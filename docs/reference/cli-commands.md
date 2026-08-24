---
title: Govard CLI Commands Reference
description: Complete reference for Govard CLI commands, aliases, and root lifecycle shortcuts for managing local Docker development environments.
---

# CLI Commands

This is the canonical CLI reference for Govard.

---

## Aliases and Shortcuts

### Root Lifecycle Shortcuts

| Shortcut | Equivalent |
| :--- | :--- |
| `govard up` | `govard env up` |
| `govard down` | `govard env down` |
| `govard restart` | `govard env restart` |
| `govard ps` | `govard env ps` |
| `govard logs` | `govard env logs` |

### Command Aliases

| Alias | Full Command |
| :--- | :--- |
| `govard boot` | `govard bootstrap` |
| `govard cfg` | `govard config` |
| `govard dbg` | `govard debug` |
| `govard gui` | `govard desktop` |
| `govard diag` | `govard doctor` |
| `govard ext` | `govard extensions` |
| `govard prj` | `govard project` |
| `govard rmt` | `govard remote` |
| `govard sh` | `govard shell` |
| `govard snap` | `govard snapshot` |

### `govard tool` Aliases

| Alias | Full Command |
| :--- | :--- |
| `govard tool mr` | `govard tool magerun` |

### `govard sync` Aliases

- `--from` is an alias for `--source`
- `--to` is an alias for `--destination`
- `-e, --environment` remains a supported source-environment option

---

## 🌿 Environment Commands

### `govard audit`

Run framework-declared, persistent project audits. `lint` runs static analysis;
Magento 2 and Mage-OS also declare a Govard-native stock CSV `profiler` check.
Later phases will add browser jobs without changing session semantics.

```bash
govard audit run
govard audit run --checks profiler --url 'https://shop.test/category.html?product_list_limit=48'
govard audit run --checks lint,profiler --url 'https://shop.test/'
govard audit diff --base origin/master
govard audit rerun --session 20260816T010203Z-a1b2c3d4
govard audit status --session 20260816T010203Z-a1b2c3d4
govard audit result --session 20260816T010203Z-a1b2c3d4 --run run-0001
govard audit cleanup --older-than 168h
```

`run` defaults to `--scope project`, `--checks lint`, `--lint-provider govard`,
`--mode auto`, and `--lint-jobs 2`. The default `text` format prints a compact
human-readable summary: the verdict first (PASSED/FAILED/CANCELLED), then scope,
duration, environment, per-PHP results with cache state and up to ten findings,
plus next-step hints pointing at the persisted report and the exact rerun
command. A completed run whose checks did not pass (failed or cancelled) still
prints its summary and then exits non-zero, so scripts and CI observe the
outcome. Color is applied only on an interactive terminal (`NO_COLOR` disables
it), so piped or redirected output stays free of escape codes.
`--format json` writes one undecorated JSON object to stdout; diagnostics and
backend logs remain out of that stream. Only `text` and `json` formats are
accepted, and `--lint-jobs` must be between 1 and
the number of PHP versions declared by the framework.

`profiler` requires an explicit absolute HTTP(S) `--url` on the first run and a
whole Govard project target (standalone modules and module-only targets are
rejected before runtime mutation). The exact URL is persisted in the run and
reused by `audit rerun`, so before/after runs capture the same page. Govard uses
Magento's stock `MAGE_PROFILER=csvfile`; it does not install a Magento module,
depend on a third-party repository/image, or edit `app/etc/env.php`.

Run `govard env up` with the current Govard version before the first capture so
the generated Compose stack mounts the project-owned custom configuration
directory. During the leased capture Govard atomically creates one uniquely
named include, reloads the active server, performs a bounded HTTP GET, collects
`var/log/profiler.csv` through the PHP container, and restores both the include
and runtime CSV. Nginx receives the temporary FastCGI parameter inside
Magento's PHP location. Apache mode uses the `web` service; hybrid configures
and reloads its `apache` service rather than nginx. The collected CSV is stored
under `runs/<run-id>/artifacts/profiler/profile.csv` with a SHA-256 digest.

Example `govard audit run` summary:

```
== Audit run run-0001 / session 20260822T005406Z-14bd9570 ==
  Status:      FAILED
  Scope:       project
  Duration:    4.5s
  Environment: magento2 | nginx | Govard 1.63.0

  Checks
    - lint - failed - 4.5s - provider govard
      PHP 8.5 | failed | 3.9s | cache cold | 12 findings
      - phpcs Squiz.Classes.ClassFileName app/code/Acme/Catalog/Model/Item.php:12: Class name is not camel case

  What next
  Full findings: ~/.govard/audit/<project-id>/sessions/<session-id>/runs/run-0001/report.json
  Re-run:        govard audit rerun --session <session-id>
```

Each run creates `~/.govard/audit/<project-id>/sessions/<session-id>/manifest.json`
and writes its result atomically to
`~/.govard/audit/<project-id>/sessions/<session-id>/runs/<run-id>/audit-result.json`,
alongside the provider's own `report.json`. `rerun`, `status`, and `result`
require the exact `--session` value (and `result` also requires `--run`); Govard
never chooses a latest session implicitly.

A `rerun` without an explicit `--checks` repeats the check selection of the
latest run in that session — including its persisted profiler URL — instead of
falling back to the lint default. Passing `--checks` reruns exactly what was
requested; either form reconstructs only the backends that selection needs.

#### Target modes

`--mode` selects what is analyzed. `auto` (the default) classifies the current
directory:

| Mode | Resolved when | What is analyzed |
|------|---------------|------------------|
| `project` | The directory tree contains a Magento project root (a `bin/magento` plus a Magento Composer requirement) and no enclosing module | The whole project |
| `module_in_project` | The directory is a module — either `etc/module.xml` (how `app/code` modules are declared) or a Composer package of type `magento2-module` — inside a Magento project | Only the module, with the whole project mounted read only so its autoloader resolves |
| `standalone` | The directory is a module with no Magento project anywhere above it | Only the module; its dependencies are installed into a scratch worktree and scanned for symbols |

`--mode project`, `--mode module_in_project`, and `--mode standalone` force a
classification and fail when the directory does not support it.

```bash
# project: run from the Magento project root
cd ~/projects/storefront
govard audit run

# module_in_project: run from inside an app/code module
cd ~/projects/storefront/app/code/Acme/Catalog
govard audit run

# module_in_project: run from inside a vendor package (Composer type magento2-module)
cd ~/projects/storefront/vendor/acme/module-catalog
govard audit run

# standalone: run from a module with no Magento project above it
cd ~/work/module-catalog
govard audit run --php 8.1,8.5
```

Each of these selects its mode automatically from the current directory —
`--mode` only needs to be passed to force or refuse a classification (for
example `--mode project` fails outside a project root instead of silently
reclassifying).

#### PHP versions

The lint image provides `7.4`, `8.0`, `8.1`, `8.2`, `8.3`, `8.4`, and `8.5`.

- `project` and `module_in_project` targets analyze exactly one version: the
  project's active `stack.php_version`. Any of the seven is valid there. Passing
  `--php` is only accepted when it repeats that active version, and the run is
  refused outright when a running application container reports a different PHP
  than the configuration does.
- `standalone` targets accept `8.1` through `8.5` and default to all five.
  `7.4` and `8.0` are **not** available for standalone modules and are rejected
  before any image is pulled, built, or run.

A rejected version fails with an `unsupported_php:` message and performs no
container work at all.

#### Scanned paths

Both native analyzers skip trees that never contain shipped code:
`vendor/`, `generated/`, `var/`, `pub/static/`, and `pub/media/`. Skipping
user-content trees keeps full-project runs fast on content-heavy stores.

Because `pub/media` is skipped by the analyzers but is exactly where uploaded
webshells land, every analyzed PHP version also runs a **media guard** phase: a
name-only scan of `pub/media` for `.php`, `.phtml`, and `.pht` files. Each hit
is reported as an `M2-LINT-MEDIA` finding with a path relative to the target
root, and the phase fails the run. The scan costs milliseconds even on
multi-gigabyte media trees; it inspects file names only and never reads file
contents into the report.

#### Providers

`--lint-provider` selects the lint backend. `govard` (the default) is the
Govard-owned native backend: it runs Govard's own Magento lint image, whose build
context is embedded in the Govard binary. A release pins that image by immutable
digest; when the pinned image cannot be pulled or its labels do not match the
embedded context, Govard builds the embedded context locally instead and the run
continues. There is no private registry and no external credential involved in
the default path.

Any other value must name an entry under `audit.lint.external_providers` in the
project configuration (see
[Configuration](./configuration.md#audit-lint-providers)). External providers are
never a fallback for the native backend and are never inferred: an unknown name
is an error, and a native failure stays a native failure. A standalone target has
no project configuration, so only `govard` is available there.

#### Caching

Reusable lint state lives under `~/.govard/cache/audit/lint/<target-id>/` and is
deliberately **not** removed by `audit cleanup`, which only prunes persisted
sessions. Each target keeps one cache generation per toolchain identity (image,
runner, PHP matrix, analyzer policy). Within a generation:

- Changing `composer.json`, `composer.lock`, or an analyzer ruleset
  (`phpcs.xml`, `phpstan.neon`, and their `.dist` variants) discards the cached
  analyzer state but keeps the Composer download cache warm, so a lock change
  does not force a full dependency re-download.
- `--no-lint-result-cache` ignores analyzer state for one run and is reported
  back as cache state `bypassed`. The Composer download cache is still kept.

Every run's evidence records the cache state (`cold`, `warm`, or `bypassed`) and
its reason per PHP version, plus the immutable image digest, the toolchain
digest, and timings for every phase.

#### Credentials and cancellation

Composer credentials at `~/.composer/auth.json` are mounted read only when the
file exists, and are linked into a private Composer home inside the container —
never copied, logged, or written into a report. SSH agent forwarding is strictly
opt-in through `--allow-lint-ssh-agent`; without that flag `SSH_AUTH_SOCK` is
never forwarded even when it is set on the host. The source tree is always
mounted read only.

Cancelling a run stops the lint container and then removes it, and the run is
reported as cancelled rather than as an infrastructure failure.

`diff` records the requested base ref in the session manifest, but lint currently
analyzes the full target; result evidence therefore reports
`effective_scope: project`.

### `govard audit toolchain`

Manage the machine-wide Govard lint image. These commands do not need to run
inside a Govard project and never invoke an external lint provider.

```bash
govard audit toolchain status
govard audit toolchain pull
govard audit toolchain build
```

- `status` inspects local images only — it never pulls and never builds — and
  reports this build's embedded context digest, the pinned official reference,
  whether that official image is present and label-verified, and whether a local
  build already exists. When nothing usable is present it also prints what to run
  next.
- `pull` resolves only the pinned official image. It never builds, so an
  unusable official path is reported rather than quietly replaced by a local
  image. A build with no pinned digest has nothing to pull and says so.
- `build` builds only the embedded context and never pulls. The resulting image
  is content addressed, so an existing image for the same context digest is
  reused as-is.

### `govard init`

Detect the project framework and generate `.govard.yml`.

```bash
govard init
govard init --framework magento2
govard init --framework custom
govard init --migrate-from warden
```

When migrating from Warden, `govard init --migrate-from warden` maps `WARDEN_TABLE_PREFIX` to Govard's `table_prefix` field for Magento 2, Magento 1, and OpenMage projects.

### `govard bootstrap`

Run bootstrap flows for clone or fresh-install setups.

```bash
govard bootstrap
govard bootstrap --clone --environment staging --yes
govard bootstrap --framework magento2 --fresh --framework-version 2.4.9
govard bootstrap -e staging --no-pii --no-noise
```

**Mode selection:**
- `--fresh` + `--framework` + `--framework-version` — fresh install via scaffolder
- `--clone` + `--environment` — rsync the whole source from a remote server

**Source selection:**
- `-e, --environment` — source remote name; accepts standard names (`staging`, `production`, `dev`) and any custom identifier (`qa`, `preprod`, `demo`, `client-uat`)
- `--remote` — alias for `--environment`
- `--db-dump` — import database from a local SQL file path

**Privacy & performance filters:**

| Flag | Effect |
| :--- | :--- |
| `-N, --no-noise` | Exclude ephemeral data (logs, sessions, cache tags, cron history) |
| `-S, --no-pii` | Exclude sensitive data (customers, orders, admin users, passwords) |
| `--delete` | Delete destination files not present on source |
| `--no-compress` | Disable rsync compression |
| `-X, --exclude` | Custom rsync exclude patterns (repeatable) |
| `--no-db` | Skip database import |
| `--no-media` | Skip media sync |
| `--media [mode]` | Media sync mode (`none`, `minimal`, `optimized`, `all`) |
| `--no-composer` | Skip `composer install` |
| `--no-admin` | Skip admin user creation (Magento 2 only) |
| `--no-stream-db` | Use local temp file for DB transfer |
| `--no-up` | Skip starting local containers before bootstrap steps |

For Magento 2/Mage-OS, Magento 1/OpenMage, or PrestaShop projects with `table_prefix` set, DB privacy filters target prefixed table names automatically.

**Magento special flags:**

| `--include-sample` | Install sample data (fresh install) |
| `--hyva-install` | Auto-install Hyva theme |

**Plan & confirmation:**
- `--plan` — print plan and exit without executing
- `-y, --yes` — skip interactive confirmation (CI/non-interactive)

### `govard env`

Project lifecycle and Docker Compose wrapper.

```bash
govard env up
govard env start
govard env stop
govard env restart
govard env down
govard env ps
govard env logs php -f
govard env pull
govard env build
govard env cleanup
```

**`govard env up` flags:**

| Flag | Effect |
| :--- | :--- |
| `--pull` | Pull images before starting |
| `--fallback-local-build` | Build missing images locally |
| `--remove-orphans` | Remove orphaned containers |
| `--quickstart` | Fastest startup path |
| `--update-lock` | Auto-update `govard.lock` on mismatches |
| `--no-tuning` | Skip framework auto-configuration prompts |

**`govard env pull` behavior:**

Images are pulled one by one. If an image cannot be pulled (removed from the
registry, unsupported tag, network failure), Govard retries the remaining
images and builds Govard-managed failures locally instead of aborting the
whole pull. Pass `--no-fallback` to disable the local build retry.

Search images: Govard-managed `elasticsearch`/`opensearch` tags track the
minor version (e.g. `7.17`), while `search_version` in `.govard.yml` accepts
either minor (`7.17`) or full patch (`7.17.28`) versions. A patch version is
pulled as-is when published; otherwise the minor image is used and the local
fallback build targets the closest real upstream release.

**Files re-rendered on `env up`:**
- `~/.govard/compose/<project-hash>.yml`
- `~/.govard/nginx/<project>/default.conf`
- `~/.govard/apache/<project>/httpd.conf`
- `~/.govard/nginx/<project>/mage-run-map.conf`

**`govard env down` flags:**
- `-v, --volumes` — remove volumes
- `--rmi local` — remove local images

### `govard frontend`

Manage the explicit, project-owned frontend development runtime for supported
projects with `stack.features.frontend_sync: true`. Start the application with
`govard env up` first. `env up` allocates no BrowserSync, LiveReload, Grunt,
Tailwind watcher, or HTML-injection container; frontend resources exist only
between explicit `frontend start` and `frontend stop` commands.

```bash
govard frontend start
govard frontend logs -f
govard frontend logs watch-vendor-theme -f
govard frontend stop
```

`start` renders and starts only the dedicated frontend Compose services
(`sync`, any discovered `watch-<theme>`, and `inject`), then waits for
BrowserSync/Luma and every discovered watcher to be healthy. After health
succeeds, it registers the active runtime through Caddy's Admin API. Both
modes expose a narrow client-asset path on the application's own domain and
run an HTML-injection proxy that shadows the application route (matching every
path) so the client script appears on real page loads without editing project
or theme files: Hyva exposes `/browser-sync/*` on port 3000 and injects
`<script src="/browser-sync/browser-sync-client.js"></script>`; Luma exposes
`/livereload/*` on port 35729 and injects
`<script src="/livereload/livereload.js?snipver=1&port=443&path=livereload/livereload"></script>`.
Both injectors buffer only HTML responses; everything else passes through
unchanged.

`stop` removes the Caddy routes (including the injection proxy) before
removing only the frontend services; dependency volumes are retained. Once
removed, the application's own route resumes serving every request. If Caddy
registration fails during `start`, Govard removes the newly started frontend
services so no hidden runtime continues consuming resources. `logs` accepts
only a discovered frontend service; omit the service to use `sync`.

### `govard svc`

Manage global services (proxy, Mailpit, PHPMyAdmin, Portainer).

```bash
govard svc up
govard svc restart --no-trust
govard svc logs --tail 50
govard svc sleep
govard svc wake
```

> **Portainer** is accessible at `https://portainer.govard.test`
> Default login: `admin` / `AdminGovard123$`

### `govard domain`

Manage extra local domains for the current project.

```bash
govard domain add brand-b.test
govard domain remove brand-b.test
govard domain list
```

### `govard status`

List running Govard environments across the workspace.

```bash
govard status
```

### `govard desktop`

Launch the Wails desktop app.

```bash
govard desktop
govard desktop --dev
govard desktop --background
```

See [Desktop App](/workflows/desktop-app) for details.

---

## 🛠️ Development Commands

### `govard shell`

Open a shell in the application container.

```bash
govard shell
govard shell --no-tty
```

- PHP frameworks → `php` container at `/var/www/html`
- Node-first frameworks (Next.js, Emdash) → `web` container at `/app`

PHP containers intentionally do not include Node.js. Use `govard tool npm …`
for Node package commands.

### `govard debug`

Manage Xdebug status and sessions.

```bash
govard debug status
govard debug on
govard debug off
govard debug shell
```

Requests route to `php-debug` only when the `XDEBUG_SESSION` cookie matches `stack.xdebug_session`.

### `govard test`

Run project test tools inside the application container.

```bash
govard test phpunit
govard test phpstan
govard test mftf
govard test integration
```

### `govard custom`

Run custom commands from `.govard/commands` or `~/.govard/commands`.

```bash
govard custom list
govard custom hello
govard custom deploy -- --dry-run
```

### `govard project`

Browse and manage known projects.

```bash
govard project list
govard project list --orphans
govard project open billing
govard project delete demo
govard project delete --yes demo
```

::: warning CAUTION
`govard project delete` removes persistent database volumes by default. Project source code is **never** deleted.
:::

**Deletion process:**
1. Runs `pre-delete` lifecycle hooks
2. Executes `docker compose down -v` (removes containers + volumes)
3. Unregisters proxy domains
4. Removes project from registry (`projects.json`)
5. Runs `post-delete` hooks

---

## 🔗 Remote, Sync, and Data Commands

### `govard remote`

Manage named remotes for sync, deploy, shell, and database workflows.

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
govard remote copy-id staging
govard remote test staging
govard remote exec staging -- ls -la
govard remote audit tail --status failure --lines 50
```

For home-relative remote paths, quote the value:

```bash
govard remote add staging --host staging.example.com --user deploy --path '~/public_html'
```

Key features:
- Capabilities: `files`, `media`, `db`, `deploy`
- Auth methods: `keychain`, `ssh-agent`, `keyfile`
- Production write protection by default
- Audit logs: `~/.govard/remote.log`

→ Full guide: [Remotes and Sync](/workflows/remotes-and-sync)

### `govard sync`

Synchronize files, media, or databases between local and named remotes.

```bash
govard sync --source staging --destination local --full --plan
govard sync --from staging --to local --media
govard sync -s prod --file --path app/etc/config.php
govard sync --db --no-noise --no-pii
```

Auto-selects `staging` remote if no `--source` is provided, falling back to `dev`.
When `--media` is used without a mode, Govard defaults it to `optimized`.

**Key flags:**

| Flag | Effect |
| :--- | :--- |
| `-s, --source` / `--from` | Source environment |
| `-d, --destination` / `--to` | Destination environment |
| `--file`, `--media`, `--db`, `--full` | Scope selection |
| `--plan` | Print plan and exit |
| `-I, --include` | Rsync include pattern (repeatable) |
| `-X, --exclude` | Rsync exclude pattern (repeatable) |
| `-m, --media [mode]` | Media sync scope (`none`, `minimal`, `optimized`, `all`); bare `--media` defaults to `optimized` |
| `-N, --no-noise` | Exclude ephemeral data |
| `-P, --no-pii` | Exclude sensitive data |

### `govard db`

Database utilities for local and remote-backed workflows.

```bash
govard db connect
govard db dump
govard db dump -e staging --local
govard db query "SELECT COUNT(*) FROM sales_order"
govard db info
govard db top
govard db import --file backup.sql --drop
govard db import --stream-db -e staging --drop
govard db clone-volume warden_magento2_dbdata
```

### `govard deploy`

Run deploy lifecycle hooks for the current project.

```bash
govard deploy
```

### `govard snapshot`

Manage local and remote snapshots for DB and media.

```bash
govard snapshot create
govard snapshot create -e staging
govard snapshot list
govard snapshot list -e staging
govard snapshot restore latest
govard snapshot pull latest -e staging
govard snapshot push before-deploy -e prod
```

### `govard open`

Open common browser targets.

```bash
govard open app
govard open admin
govard open mail
govard open db
govard open db --pma
govard open db --client
govard open db -e staging
```

### `govard tunnel`

Manage public tunnels (requires `cloudflared`).

```bash
govard tunnel start
govard tunnel status
govard tunnel stop
```

::: important IMPORTANT
The `cloudflared` binary must be installed separately.
Install via the [official Cloudflare repository](https://developers.cloudflare.com/cloudflare-one/connections/connect-networks/install-run/install-threads/) or [GitHub releases](https://github.com/cloudflare/cloudflared/releases).
:::

---

## 🔧 Tool Commands

Run framework CLIs inside project containers:

```bash
govard tool magento [command]    # Magento 2
govard tool magerun [command]    # Magento 1 / Magento 2 (Shortcut: mr)
govard tool artisan [command]    # Laravel
govard tool drush [command]      # Drupal
govard tool symfony [command]    # Symfony
govard tool shopware [command]   # Shopware
govard tool cake [command]       # CakePHP
govard tool wp [command]         # WordPress
govard tool prestashop [command] # PrestaShop

# General tools
govard tool composer [command]
govard tool php [command]        # Run the PHP CLI directly (e.g. editor/IDE integrations)
govard tool npm [command]
govard tool yarn [command]
govard tool npx [command]
govard tool pnpm [command]
govard tool grunt [command]
```

Node package commands (`npm`, `npx`, `yarn`, `pnpm`, and `grunt`) run in a
one-shot `node:<stack.node_version>-alpine` container with the project mounted
at `/var/www/html`. This keeps builds aligned with Govard's frontend watchers
and BrowserSync. `govard shell` still opens the framework runtime container and
does not expose Node.js for PHP frameworks.

`govard tool php` requires the current directory to be the project root. For editor/IDE integrations (see below), use `govard vscode` instead.

---

## 🧩 Editor Integration Commands

### `govard vscode setup`

Writes (or merges into) the VSCode settings needed to run PHP tooling inside the project's container instead of the host:

```bash
# Run from inside a project (or any subdirectory of one)
govard vscode setup
#   -> .vscode/settings.json: intelephense.environment.phpVersion, phpstan.paths,
#                             phpunit.paths (if vendor/bin/phpunit exists),
#                             and (if vendor/bin/phpcs exists) phpcs.standard + phpcs.autoConfigSearch=false
#   -> .vscode/launch.json:   a "Listen for Xdebug (Govard)" configuration (port 9003)

# Run once, applies to every Govard project
govard vscode setup --global
#   -> creates ~/.govard/bin/govard-php, govard-php-cs-fixer, and govard-phpcs wrapper scripts
#   -> user settings.json: php.validate.executablePath, phpstan.binCommand,
#                          php-cs-fixer.executablePath, phpcs.executablePath, phpunit.command
```

Settings reflect the project's last-used profile (e.g. an upgrade profile pinning a newer PHP version) if one is registered, rather than always the base `.govard.yml` — so `intelephense.environment.phpVersion` matches whatever's actually running.

The PHPCS coding standard is auto-detected from `composer.json` (`magento/magento-coding-standard` -> `Magento2`, `wp-coding-standards/wpcs` -> `WordPress`, `drupal/coder` -> `Drupal`), falling back to `PSR12`. `phpcs.autoConfigSearch` is disabled because it would otherwise auto-detect a `phpcs.xml`/`.dist` ruleset and pass its *host* absolute path as `--standard`, which the container can't read.

If `vendor/bin/phpstan` exists but the project has **no** `phpstan.neon`/`.dist`/`dist.neon` of its own, `setup` sets `phpstan.options` to a `--level=0` default (`--autoload-file=vendor/autoload.php` plus `app/code`+`app/design` for Magento 2 or `app`+`src` otherwise — the same convention `govard test phpstan` already falls back to) so PHPStan has something to analyze. This intentionally lives in `.vscode/settings.json`, not a generated `phpstan.neon` at the project root — that file is normally git-tracked and not ours to create. As soon as the project gets its own config, re-running `setup` removes `phpstan.options` again so it can never override the project's real rules — the project's config always wins.

`phpunit.command` (recca0120.vscode-phpunit) needs no wrapper script — it's a template the extension tokenizes itself, so it's set directly to `govard vscode phpunit ${phpunitargs}`. This gives you the Testing sidebar (run/rerun individual tests) without installing PHPUnit on the host. Debugging an individual test through this extension isn't wired up yet — it would need Xdebug environment variables forwarded into the `docker exec` call.

Each setting group requires a specific VSCode extension (Intelephense, PHPStan, PHP CS Fixer, PHPCS, PHPUnit, PHP Debug). If one isn't installed, `setup` warns and asks whether to install it now via `code --install-extension` — accept and the corresponding setting is still wired up in that same run. Pass `--yes` to install everything missing without asking (useful for scripting); with no TTY attached and no `--yes`, missing extensions are skipped without prompting.

Existing keys and unrelated `launch.json` configurations are preserved — only the keys Govard manages are added or overwritten. Note: settings.json is parsed as plain JSON, so any comments in it are dropped when rewritten.

### `govard vscode <tool>`

The underlying tool runners that the settings written by `setup` point to:

```bash
govard vscode php [args]
govard vscode composer [args]
govard vscode phpstan [args]       # vendor/bin/phpstan
govard vscode php-cs-fixer [args]  # vendor/bin/php-cs-fixer
govard vscode phpcs [args]         # vendor/bin/phpcs
govard vscode phpunit [args]       # vendor/bin/phpunit, with memory_limit=-1
```

Unlike `govard tool`, these resolve the project by walking up from the current directory to find the nearest `.govard.yml` — editors often invoke tooling with a working directory that isn't the workspace root (e.g. the active file's directory), so an exact cwd match isn't reliable.

---

## ⚙️ Configuration Commands

```bash
govard config get stack.php_version
govard config set stack.php_version 8.4
govard config set table_prefix demo_
govard config profile              # Show recommended profile for current framework
govard config profile --json      # Output profile as JSON
govard config profile apply       # Apply recommended profile to .govard.yml
govard config auto                # Magento 2: inject settings into env.php
```

### `govard config profile`

Display the recommended runtime profile for the detected framework.

```bash
govard config profile
govard config profile --json
```

Output includes detected framework, recommended PHP version, database, cache, search, and other service configurations.

### `govard config profile switch`

Switch to a different environment profile. Profiles allow running the same project with different runtime configurations (e.g., PHP 8.2 for production testing, PHP 8.3 for development).

```bash
govard config profile switch upgrade
govard config profile switch staging
govard config profile switch          # Interactive selection
```

Profile files are stored as `.govard.<name>.yml` in the project root. The selected profile is persisted per-project in `~/.govard/projects.json`.

After switching, run `govard env up` to apply the new environment. You'll be prompted for confirmation when a profile change requires restarting containers.

### `govard config profile clear`

Reset to the default profile (no profile active).

```bash
govard config profile clear
```

### `govard extensions`

Initialize `.govard/*` extension scaffolding.

```bash
govard extensions init
govard extensions init --force
```

### `govard blueprint cache`

Manage the remote blueprint registry cache.

```bash
govard blueprint cache list
govard blueprint cache clear
```

---

## 🩺 Diagnostics

### `govard doctor`

Run startup diagnostics with actionable remediation.

```bash
govard doctor
govard doctor --fix
govard doctor --json
govard doctor --pack
govard doctor trust
```

Checks include: Docker, Compose, ports, disk sanity, Govard home, compose directory health, SSH agent, and outbound connectivity.

- **`--fix`** — Automatically detect and repair common issues
- **`trust`** — Install Root CA into system trust store + browser NSS

---

## 🔁 Utility Commands

### `govard lock`

Generate or validate `govard.lock` snapshots for environment drift detection.

```bash
govard lock generate
govard lock check
govard lock diff
govard lock generate --file .govard/govard.lock
```

### `govard self-update`

Download release artifacts, verify checksums, and replace binaries atomically.

```bash
govard self-update                    # update within your current channel
govard self-update --channel beta     # opt into beta releases (persists)
govard self-update --channel stable   # switch back to stable (persists)
govard self-update --version v1.60.0-beta.1  # install one specific version
```

The update channel is remembered across runs (both the CLI and Govard
Desktop read the same setting), so a plain `govard self-update` keeps
following whichever channel you last selected.

### `govard upgrade`

Native framework upgrade pipeline.

```bash
govard upgrade --version 2.4.8-p4     # Magento 2
govard upgrade --version 11            # Laravel
govard upgrade --version 7             # Symfony
govard upgrade --version 6.7           # WordPress
govard upgrade --version 11 --dry-run  # Preview steps
```

**Flags:**

| Flag | Effect |
| :--- | :--- |
| `--version` | Target version (required) |
| `--dry-run` | Show steps without executing |
| `--no-db-upgrade` | Skip DB migrations |
| `--no-env-update` | Skip profile update and container restart |
| `-y, --yes` | Auto-confirm all prompts |

### `govard version`

```bash
govard version
```

### `govard redis`

Smart shortcut for Redis/Valkey management.

```bash
govard redis cli
govard redis flush
govard redis info
```

### `govard varnish`

Smart shortcut for Varnish management.

```bash
govard varnish purge
govard varnish status
```

### `govard rabbitmq`

Smart shortcut for RabbitMQ management.

```bash
govard rabbitmq status
govard rabbitmq queues
govard rabbitmq cli list_exchanges
```

---

## 🌐 Global Flags

All commands support:

- `-h, --help` — Show help

---

[← Getting Started](/getting-started/getting-started) | [Configuration →](/reference/configuration)
