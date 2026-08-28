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

### Exec and Audit

```bash
govard remote exec staging -- ls -la
govard remote audit tail --lines 20
govard remote audit tail --status failure --lines 50
govard remote audit stats --lines 200
```

**Audit log paths:**
- `~/.govard/remote.log`
- `~/.govard/operations.log`

---

## Remote Safety Model

| Protection | Behavior |
| :--- | :--- |
| **Production write protection** | `prod` remotes are write-protected by default |
| **Capability enforcement** | Each operation checks `files`, `media`, `db`, `deploy` scopes |
| **Strict host-key** | Opt-in per remote, not enforced by default |
| **1Password integration** | Remote fields support `op://...` secret references |

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
`--delete` combined with `--db` surfaces policy warnings. Production remotes are write-protected by default and will block destructive writes.
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
