---
title: Migrate from DDEV or Warden to Govard
description: Step-by-step guide to migrating an existing DDEV or Warden project to Govard without data loss — Govard reads your existing configuration automatically.
---

# Migration Guide 🚚

Transitioning your project from another local development tool to Govard is designed to be seamless. Govard understands configuration from popular tools like Warden and DDEV and can automatically orchestrate the transition without data loss.

This guide walks you through migrating an existing project (e.g., from Warden) to Govard.

---

## Migrating from DDEV to Govard

DDEV stores its settings in `.ddev/config.yaml` plus any `.ddev/config.*.yaml`
overrides. Govard reads all of them, in order, with the later file winning, the
same way DDEV does.

```bash
govard init --migrate-from ddev
```

The command prints one line per key it handled, so the result is inspectable
before you boot anything:

- `mapped ...` for values carried into `.govard.yml`;
- `defaulted ...` for services the DDEV file never mentions, which keep the
  framework profile's default rather than being switched off;
- `not migrated ...` for keys Govard has no equivalent for, with the reason.

### What carries over

| DDEV key | Govard field |
| :--- | :--- |
| `name` | `project_name` and the `*.test` domain |
| `type` | `framework` |
| `docroot` | `stack.web_root` |
| `php_version`, `nodejs_version`, `composer_version` | the matching `stack.*` versions |
| `webserver_type` | `stack.services.web_server` |
| `database.type`, `database.version` | `stack.services.db`, `stack.db_version` |
| `additional_hostnames` | `extra_domains`, expanded to `<host>.<project>.test` |
| `additional_fqdns` | `extra_domains`, unchanged |
| `xdebug_enabled` | `stack.features.xdebug` |

### What is reported, not applied

`upload_dirs`, `webimage_extra_packages`, `omit_containers`, `hooks`,
`performance_mode`, `mutagen_enabled`, `web_extra_exposed_ports`,
`database_snapshot`, `timeout`, `disable_settings_management`, the router ports
and `composer_root` are named in the output with the reason each one has no
equivalent. Nothing is dropped silently.

Services DDEV does not manage (`search`, `cache`, `queue`, `varnish`) keep the
profile default, so a migrated Magento project still gets OpenSearch, Redis and
RabbitMQ. `govard doctor` is the check: a correct migration reports the runtime
as synced with the profile instead of warning that search and cache are `none`.

One deliberate exception: the framework profile pins PHP, Node and Composer
versions, so a profile pin can override a version DDEV declared. That override is
printed as a drift line (`Synced stack.node_version 20→24`) rather than happening
silently.

## Migrating from Warden to Govard

When moving a project from Warden, the primary goals are:
1. Translating the PHP/Node/Database versions to Govard's ecosystem (`.govard.yml`).
2. Ensuring no data loss by transferring the Database volume.
3. Flushing temporary files/caches with hardcoded paths.

### Step 1: Stop the Warden Environment

Before starting the migration, verify your old environment is stopped to prevent data corruption during database synchronization.

```bash
cd /path/to/your/project
warden env stop
```

### Step 2: Clean up Application Caches

Warden paths and Govard paths inside the container might be identical (like `/var/www/html`), but Redis or file-based caching generated via Warden can cause errors if they expect different internal environment variables or configuration states.

For **Magento 2** projects, clear out the generated/cache folders:

```bash
rm -rf var/cache/* var/page_cache/* generated/code/*
```

### Step 3: Run the Automated Govard Migration

Run the `govard init` command with the `--migrate-from warden` flag.

Govard will parse your `.env` (Warden's variables) and `.warden/warden-env.yml` settings to automatically suggest the best runtime profile.
For Magento 2, Magento 1, and OpenMage projects, Govard also migrates Warden's `WARDEN_TABLE_PREFIX` value into `.govard.yml` as `table_prefix`.

```bash
govard init --migrate-from warden
```

**During this process:**
1. Govard will generate a `.govard.yml` mapping your Warden settings to Govard stacks.
2. It will detect your Warden database volume (typically `<project>_dbdata`).
3. If `WARDEN_TABLE_PREFIX` is set, Govard will persist it as `table_prefix` so Magento table names like `demo_core_config_data` continue to work.
4. You will be prompted:
   > `Do you want to clone the existing database volume from Warden ('myproject_dbdata') into Govard? [y/N]`
5. Type **`y` (Yes)**. Govard will automatically clone the SQL raw data to its own isolated docker volume behind the scenes.

*(Note: The database cloning uses raw `cp -a` mounted inside Docker, ensuring that file permissions are strictly secured and the process finishes in seconds, bypassing slow mysqldump pipelines).*

### Step 4: Boot the New Environment

With the configurations parsed and the database safely cloned, simply start Govard.

```bash
govard env up
```

Wait until you see the `✅ php runtime is ready` message.

### Step 5: Post-Migration Sync (Optional)

Since Govard connects natively via `app/etc/env.php` (on Magento), you should auto-update the connection strings using Govard's native configuration injector:

```bash
govard config auto
```

If you use Elasticsearch/OpenSearch, remember that search index volumes are **not** migrated automatically (only primary relational DBs like MariaDB/MySQL). You should re-index the data via your framework:

```bash
govard tool magento indexer:reindex
```

### Step 6: Cleanup Warden (Optional)

Once you verify that `.test` domain works flawlessly and your Admin Panel is blazing fast, you can safely completely tear down Warden to recover SSD space:

```bash
warden env down -v
```

---

## Manual Database Migration

If you skipped the automated prompt during `govard init`, you can always clone a database volume manually at any time!

Make sure Govard's DB container is stopped first:

```bash
govard env stop db
```

Then trigger the manual clone command pointing to your old Docker volume:

```bash
govard db clone-volume <source_volume_name>
```

Example for Warden:
```bash
govard db clone-volume warden_myproject_dbdata
```

Example for DDEV:
```bash
govard db clone-volume ddev-myproject-db
```

---

**[← Getting Started](/getting-started/getting-started)** | **[Remotes and Sync →](/workflows/remotes-and-sync)**
