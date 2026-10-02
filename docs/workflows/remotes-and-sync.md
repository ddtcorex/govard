---
title: Remote Environments & Database Sync
description: Manage named remotes with scoped capabilities, safe production write-blocking, and bi-directional file/media/database sync with dry-run planning.
---

# Remotes and Sync

This is the canonical guide for Govard remote environments, sync operations, and remote-backed database workflows.

---

## Remote Setup

### Add a Remote

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
```

**`remote add` flags:**

| Flag | Description |
| :--- | :--- |
| `--host` | Remote hostname or IP |
| `--user` | SSH username |
| `--path` | Remote project path |
| `--port` | SSH port (default: 22) |
| `--capabilities` | Comma-separated capabilities (`files,media,db,deploy`) |
| `--auth-method` | Auth method (`keychain`, `ssh-agent`, `keyfile`) |
| `--key-path` | Path to SSH key (for `keyfile` method) |
| `--strict-host-key` | Enable strict host-key verification |
| `--known-hosts-file` | Custom known_hosts file |
| `--protected` | Write-protect this remote |

::: tip TIP
To use the remote user's home directory, quote the path so the local shell does not expand it:
```bash
govard remote add staging --host staging.example.com --user deploy --path '~/public_html'
```
:::

### Validate Connectivity

```bash
govard remote copy-id staging    # Copy your SSH public key to remote authorized_keys
govard remote test staging       # Validate SSH + rsync, measure latency, classify failures
```

`remote test` identifies failure types: `network`, `auth`, `permission`, `host_key`, `dependency`.

Setting up key authentication is always something you ask for: the explicit way
is `govard remote copy-id <remote>`. The one other place govard offers to copy a
key is `govard remote test`, whose whole job is diagnosing an auth failure — and
that offer now answers **No** unless you say yes, so a single Enter copies
nothing. A read or a sync against a remote you have not keyed never writes to
that remote's `authorized_keys` as a side effect. On a **write-protected**
remote (`--protected`, or an environment govard recognises as production) the
offer is refused outright, and the remote is not even contacted to ask — set the
key up explicitly when you mean to.

### Exec and Audit

```bash
govard remote exec staging -- ls -la
govard remote audit tail --lines 20
govard remote audit tail --status failure --lines 50
govard remote audit stats --lines 200
```

`remote exec` runs whatever shell command you give it and sits **outside** the
write-protection gate, protected remotes included: nothing inspects the command, so
the operator owns what is run (the example above is read-only on purpose). An alias
such as `stg` resolves to the configured remote first, so its key and the audit
events use the configured name.

**Audit log paths:**
- `~/.govard/remote.log`
- `~/.govard/operations.log`

---

## Remote Safety Model

| Protection | Behavior |
| :--- | :--- |
| **Production write protection** | `prod` remotes are write-protected by default: protection covers **writes**, and reads from a protected remote are allowed |
| **Capability enforcement** | Each operation checks `files`, `media`, `db`, `deploy` scopes |
| **Strict host-key** | Opt-in per remote, not enforced by default |
| **1Password integration** | Remote fields support `op://...` secret references |

### Configuring the Sandbox Remote

`sandbox` is the one remote name that is also resolvable *without* being
configured: `govard sandbox up` creates a container on this machine and govard
resolves the name from that container. So the rule for a `remotes.sandbox` block
is not "declare the host" — it is "shape the rehearsal, never the machine".

**What stays container-derived, always:** `host`, `port`, `user`, `path`, `url`,
every `auth` field (`method`, `key_path`, `strict_host_key`, `known_hosts_file`),
`paths`, the `db_*` credentials, and the two topology flags `local` and
`sandbox`. A leaked identity does not fail loudly: the rehearsal would silently
run against a different machine, at a different path, with a different key, and
report success.

**What the block may set:** `capabilities`, `protected`, and the `deploy.*`
fields a per-remote override actually copies — `keep_releases`,
`command_timeout`, `artifact_dir`, `repository`, `branch`, `publish`,
`deploy_path`, `db_backup`, `verify.url`, `verify.timeout`, `hooks`, and
`settings` key by key. `deploy_path` is the one that moves the rehearsal: it
repoints where the release is published *inside the sandbox* and cannot reach
outside it. `lock_stale_after`, `maintenance_timeout` and
`verify.follow_redirects` are project-level only and a sandbox block is ignored
for them — put them in the project-level `deploy:` block where they are read.

Four settings are pinned back to the container: `deploy.settings.owner`,
`writable_mode`, `php_bin` and `php_version`. Those four describe what the image
shipped (which deployer account owns the files, how they are written, which PHP
binary and series it carries), and a deploy refuses to run when the target's PHP
does not match the declared series. `writable_permissions` and `composer_bin`
are *not* pinned.

**Why:** an operator has to be able to state the rehearsal target's shape — its
retention, its capabilities, whether a confirmation is required — and the
alternative (refuse `remote add sandbox`, ignore a hand-written block) left the
block parsed by one path and ignored by every identity consumer, which is worse
than either extreme.

```yaml
remotes:
  sandbox:
    capabilities:
      db: false
    protected: true
    deploy:
      keep_releases: 3
```

```bash
govard remote add sandbox --capabilities db --protected   # same allowlist
```

Every identity flag passed to `remote add sandbox` — `--host`, `--user`,
`--port`, `--path`, `--auth-method`, `--key-path`, `--known-hosts-file` and
`--strict-host-key` — is dropped and named on stderr; the block is written with
no identity value in it, and later saves keep it that way. `govard remote list`
still prints a single sandbox row, showing the liveness in the host column and
the block's capabilities in the capabilities column, and reports a configured
block as layered over the synthetic one.

The desktop app resolves `sandbox` the same way: its Open admin / Open SFTP /
Open SSH actions go through the container, never through the block, so a block
with no host cannot make the app open `https://localhost/admin` on your own
machine. It lists the sandbox whenever the container answers — the same trigger
`govard remote list` uses, not the same row rule: the panel drops a sandbox it
cannot resolve, where `remote list` always prints a row carrying the liveness in
its HOST column. When a `remotes.sandbox` block is configured but no
container is running, the row is dropped and the reason comes back as a warning
in the project's remote panel; a project with neither a block nor a container
gets no row and no warning.

---

## Sync Overview

`govard sync` moves files, media, and database data between local and named remotes.

```bash
govard sync --source staging --destination local --full --plan
govard sync --from staging --to local --media
govard sync -s dev --db --no-noise --no-pii
govard sync -s prod --file --path app/etc/config.php
govard sync -s dev --file app/design/frontend/MyTheme
```

Auto-selects `staging` if no `--source` provided, falling back to `dev`.
Bare `--media` defaults to the `optimized` media mode.

A `sandbox` source or destination resolves from the live container, not from the
config file, so `govard sync -s sandbox` works with no `remotes.sandbox` block
at all. The transfer is built from the container's own identity — `127.0.0.1`,
user `deployer`, the published port, the key `sandbox up` generated — so a
configured block can only shape the rehearsal, never repoint it. On a host with
no Docker the sandbox cannot be resolved at all, and asking for it exits `3`
with `CAPABILITY_MISSING` rather than reporting the name as unconfigured.

### Endpoint Flags

| Flag | Description |
| :--- | :--- |
| `-s, --source` / `--from` | Source environment |
| `-d, --destination` / `--to` | Destination environment |
| `-e, --environment` | Alias for `--source` |

### Scope Flags

| Flag | Syncs |
| :--- | :--- |
| `-A, --full` | Everything (files + media + database) |
| `-f, --file` | Source code and generic files |
| `-m, --media [mode]` | Framework-specific media assets; bare `--media` defaults to `optimized` |
| `-b, --db` | Project database |

### Transfer Flags

| Flag | Description |
| :--- | :--- |
| `--plan` | Print plan and exit without executing |
| `-D, --delete` | Delete destination files missing from source |
| `-R, --resume` | Enable resumable transfers (default: `true`) |
| `--no-resume` | Disable resumable transfers |
| `-C, --no-compress` | Disable rsync compression |
| `-y, --yes` | Skip confirmation prompts |
| `-p, --path` | Specific file/directory relative to project root |
| `-I, --include` | Rsync include pattern (repeatable) |
| `-X, --exclude` | Rsync exclude pattern (repeatable) |

The plan and the confirmation summary show the database password as
`export MYSQL_PWD=***;` (`export PGPASSWORD=***;` for PostgreSQL); the command that
runs still carries the real value. `--db` and `--full` also need a container runtime
(the database moves through the local database container), except with `--plan`.

::: tip
Omitting `--path` syncs the entire project root — `govard sync` warns you about this in the plan before asking for confirmation. You can also skip `-p`/`--path` entirely and pass the path as a trailing argument instead, e.g. `govard sync -s dev --file app/design/frontend/MyTheme`.
:::

### Database Privacy Filters

| Flag | Category | Magento 2 Exclusions | Laravel | WordPress |
| :--- | :--- | :--- | :--- | :--- |
| `--no-noise` | Ephemeral data | `cron_schedule`, `session`, `cache_tag`, `report_event` | `cache`, `sessions`, `failed_jobs` | `redirection_404`, `wflogs` |
| `--no-pii` | Sensitive data | `customer_entity`, `sales_order`, `quote`, `admin_user` | `users`, `password_resets` | `users`, `usermeta`, `comments` |

::: info NOTE
Database filters are optimized for Magento 2. For other frameworks, safe default patterns are used when available.
:::

---

### Remote Database Credentials

For `--db` operations Govard probes the remote's own configuration over SSH
instead of asking for credentials — but only for three framework families:
**dotenv** (`.env`), **WordPress** (`wp-config.php`) and **Magento 2 / MageOS**
(`app/etc/env.php`). Any other stack gets no probing: if nothing is found it
warns and falls back to framework defaults — a fallback dump that cannot
connect fails loudly instead of producing an empty file, so treat any
credential warning as a signal to check the remote path.

Each covered probe tries three candidate app roots in order — the configured
remote path first, then `<path>/public_html`, then `<path>/current` — so a
remote pointing at a layout root still resolves. The **first valid candidate
wins**: a stale `.env` sitting at the layout root shadows the real
application one level down, because the layout root is tried first. Point the
remote `path` at the real app root (or remove the stale file) when the probe
picks up the wrong database.

Every remote `mysql`/`mariadb` invocation passes `--no-defaults`, so the
probed credentials are the only ones in effect: a stale `~/.my.cnf` password
on the remote can never silently override them (client precedence is
command-line > option file > `MYSQL_PWD`).

## Sync Behavior

### Resumable Transfers

File and media sync use resumable rsync mode by default (`--partial` + `--append-verify`).

```bash
govard sync -s staging --file        # resumable by default
govard sync -s staging --file --no-resume  # disable resumable
```

### Include and Exclude Filters

`--include` and `--exclude` (or `-I` and `-X`) apply only to `-f, --file` and `-m, --media` scopes — they are ignored for DB-only sync.

In `govard bootstrap`, `--exclude` acts as a global ignore list for both the source code clone and the subsequent media sync.

### Smart Media Exclusions (Magento Only)

Govard implements automated filtering for Magento media sync to optimize bandwidth and disk usage.

| Category | Behavior | Excluded Paths |
| :--- | :--- | :--- |
| **None** | `--media none` | Skips media sync entirely |
| **Minimal** | `--media minimal` | `*.jpg`, `*.png`, `*.webp`, `*.mp4`, `*.pdf` (assets only) |
| **Optimized** | Default mode | `catalog/product/` (Magento), `*/cache/*` (WordPress) |
| **Catalog** | `--media catalog` | Like optimized but includes product images, skips product caches (Magento only) |
| **All** | `--media all` | Truly all (includes everything, use with caution) |

::: info NOTE
All modes except **All** automatically exclude framework noise like `tmp/`, `cache/`, and `logs/`.
:::

To download everything, use `--media all`. To sync only CSS/JS/Fonts, use `--media minimal`.

### Protected Destinations

::: warning WARNING
`--delete` combined with `--db` surfaces policy warnings. Production remotes are write-protected by default and will block destructive writes. Protection covers **writes**, not reads: `db info` or `db top` from a protected remote are still allowed (`db dump` stays allowed too — it only creates a new archive file on the remote), and only a write into it — `db import` without `--stream-db`, which reads the remote's dump and writes **your local** database instead, `db query`, `db connect`, `snapshot push` — is refused.
:::

### Integration with `bootstrap`

`govard bootstrap` uses the same sync/filter flags for environment initialization:

```bash
govard bootstrap --clone -e staging --no-pii --no-noise --delete
```

---

## Remote Name Resolution

Govard accepts **any valid identifier** as a remote environment name. Names must use lowercase letters, digits, hyphens, or underscores (e.g. `qa`, `preprod`, `demo`, `client-uat`, `load-test`).

Remote flags support:
- Exact remote key lookup (e.g. `qa`, `preprod`)
- Normalized aliases for well-known environments (e.g. `stg` → `staging`, `live` → `production`)
- Case-insensitive fallback matching

### Well-Known Aliases

| Input | Resolved as |
| :--- | :--- |
| `dev`, `development`, `develop` | `development` |
| `staging`, `stage`, `stg` | `staging` |
| `prod`, `production`, `live` | `production` |
| Everything else (`qa`, `preprod`, `demo`, etc.) | Passed through as-is |

### Auto-Select Priority

When no remote is specified for `bootstrap` or `sync`, Govard resolves:

1. **`staging`** (or any alias: `stg`, `stage`)
2. **`development`** (or any alias: `dev`, `develop`)

If neither `staging` nor `development` exists, use `-e` to specify the remote explicitly:

```bash
govard sync -s qa --db
govard bootstrap -e preprod --yes
```

These are equivalent when the `staging` remote exists:

```bash
govard sync -s stg --db
govard sync --source staging --db
govard sync --from staging --db
```

### Custom Environment Examples

```bash
# Add a QA environment
govard remote add qa --host qa.example.com --user deploy --path /var/www/app

# Add a pre-production environment
govard remote add preprod --host preprod.example.com --user deploy --path /var/www/app

# Bootstrap from QA (must specify -e since it's not auto-selected)
govard bootstrap -e qa --yes

# Sync DB from preprod
govard sync -s preprod --db --no-pii
```

### Protection Policy

Custom environment names have **no automatic write protection**. Use `--protected` or the `protected: true` config flag to opt in:

```bash
govard remote add preprod --host preprod.example.com --user deploy --path /var/www/app --protected
```

Or in `.govard.yml`:

```yaml
remotes:
  preprod:
    host: preprod.example.com
    user: deploy
    path: /var/www/app
    protected: true
```

Only remotes whose name normalizes to `prod` (i.e. `prod`, `production`, `live`) are write-protected automatically.

---

## Deploy Fields on a Remote

Sync and deploy read two different paths out of the same remote block, and they are
not the same directory:

| Field | Meaning |
| --- | --- |
| `path` | the **served docroot** — what the web server answers from. Whether it is absent, a symlink or a real directory is what decides how `govard deploy` publishes a release. |
| `deploy.deploy_path` | the **layout root** holding `releases/`, `shared/` and `.dep/`. Omitted, it is probed from the target and adopted only when exactly one candidate matches. |

A remote overrides deploy behavior in its nested `deploy:` block only — there
is exactly one spelling per key, so no conflict is possible:

```yaml
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html       # served docroot (symlink on this target)
    deploy:
      deploy_path: /home/deploy/.deployer  # releases/, shared/, .dep/
      publish: symlink                   # auto | symlink | in_place
      branch: main
      settings:
        php_bin: php8.3
        php_version: "8.3"
        mage_mode: production
      verify:
        url: https://staging.example.com/
```

Four topology keys accept a project-wide default in the `deploy:` block, so
values shared by every remote are written once: `repository`, `branch`,
`publish` and `deploy_path`.

```yaml
deploy:
  repository: git@example.com:app/shop.git
  keep_releases: 5
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /home/deploy/public_html
    deploy:
      branch: staging        # differs per environment, stays per remote
```

Precedence per remote, top wins: the remote `deploy.*` override → the project
`deploy:` default → the previous behavior (branch still required without
flags, empty `deploy_path` still probes the target).

The flat remote-level forms (`remotes.<name>.branch`,
`remotes.<name>.repository`, `remotes.<name>.publish`,
`remotes.<name>.deploy_path`) were removed: the YAML decoder drops unknown
keys silently, so the loader refuses them loudly instead of deploying the
wrong ref:

```
remotes.staging: "branch" was removed; move it under remotes.staging.deploy.branch
```

Path values pass through untouched, including `~` forms — the remote shell
expands them, govard never does, so do not expand or quote them away.

`govard deploy check <remote>` reports the layout it found and the publish strategy
that implies, before anything is created.

→ Full guides: [Deployment](/workflows/deployment) and
[Deployment case studies](/workflows/deploy-case-studies)

---

## Remote Snapshots

```bash
govard snapshot create -e staging
govard snapshot list -e staging
govard snapshot restore latest -e staging
govard snapshot delete latest -e staging
```

Remote snapshots run `mysqldump` and `tar` directly on the remote server without transferring data over the network. Stored in `~/.govard/snapshots/` within the remote project path.

### Bidirectional Transfer

```bash
# Pull from staging to local
govard snapshot pull before-upgrade -e staging

# Push local snapshot to production (blocked by default protection policy)
govard snapshot push fallback-state -e prod
```

---

## Remote Database Workflows

### Dump

```bash
govard db dump                        # Local DB to project var/
govard db dump -e staging             # Remote DB → saved on remote (~backup/)
govard db dump -e staging --local     # Remote DB → streamed to local var/
govard db dump --no-noise --no-pii    # With privacy filters
```

A remote dump stages through a temporary file in the remote's `/tmp`: the raw
uncompressed dump — the full database, PII included — sits there while it is
being compressed for transfer, and is removed with `rm -f` afterwards. If the
SSH connection is interrupted mid-dump, that cleanup never runs and the raw
file stays behind (there is no trap on the remote side — a known limitation),
so re-run the dump and delete the leftover yourself rather than assuming it
is gone.

The dump file itself is private to its owner: a local file is created `0600` (a
broader existing one is tightened first), and the remote file written by
`db dump -e <remote>` is created under `umask 077`, so a directory it creates, such
as `~/backup`, is `0700`.

### Import

```bash
govard db import --file backup.sql --drop
govard db import --stream-db -e staging --drop
```

`--stream-db` pulls from the remote and imports into the local database. `--drop` performs a safe reset before import.

### Query, Info, and Live Monitoring

```bash
govard db query "SELECT COUNT(*) FROM sales_order"
govard db info -e staging
govard db top -e staging    # Live process monitoring
```

---

## Desktop Remote Actions

Desktop remote actions call the same backend paths as CLI commands:

- **Open Database (Remote)** → `govard open db -e <remote> --client`
- **Open SSH (Remote)** → native Linux terminal launchers, fallback to `ssh://`
- **Open SFTP (Remote)** → prefers FileZilla, fallback to `sftp://`

For `auth.method: ssh-agent`, Govard reuses `SSH_AUTH_SOCK` and probes `/run/user/<uid>/keyring/ssh` on Linux.

---

## Recommended Patterns

**Safe review before execution:**

```bash
govard sync --source staging --destination local --full --plan
```

**Target one file:**

```bash
govard sync --source prod --file --path app/etc/config.php
```

**Create a local-safe DB snapshot:**

```bash
govard db dump -e staging --local --no-noise --no-pii
```

**Full workflow: clone from staging with privacy:**

```bash
govard bootstrap --clone -e staging --no-pii --no-noise --yes
```

**Case Study: Efficient Magento Bootstrap**

Imagine you are bootstrapping a large Magento 2 project but only want the code and a subset of media:

```bash
# Clone code, sync DB without PII, and sync media WITHOUT heavy product images (default: optimized)
govard bootstrap --clone -e staging --no-pii --no-noise --yes
```

**Case Study: Targeted Media Sync with Excludes**

If you need to sync media but want to skip a specific large folder that isn't covered by default smart exclusions:

```bash
# Sync media from staging but skip a custom 'large-assets' directory
govard sync -s staging --media -X "large-assets/*"
```

**Case Study: Including Products during Bootstrap**

If you actually need the product images for a front-end task:

```bash
govard bootstrap --clone -e staging --media all --yes
```

**Case Study: The "Broom" vs the "Tweezers"**

Combine predefined smart modes (the Broom) with custom excludes (the Tweezers) for ultimate control:

```bash
# Get all media, but skip a specific legacy backup folder left on the server
govard bootstrap --clone -e staging --media all -X "pub/media/backup_2022/*" --yes
```

**Case Study: Ultra-Fast Sync (Minimal)**

If you are working on frontend CSS/JS and don't care about images at all:

```bash
# Sync only static assets (css, js, fonts, json)
govard sync -s staging --media minimal
```

---

[Framework Reference](/reference/frameworks) | [SSL and Domains](/workflows/ssl-and-domains)
