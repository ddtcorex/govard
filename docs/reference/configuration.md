---
title: Govard Configuration Reference
description: How Govard's layered project configuration and framework blueprints work, including override precedence and available settings.
---

# Configuration

Govard uses layered project configuration plus framework blueprints.

---

## Config Layer Order

Govard loads config in this order (later layers override earlier ones):

| Priority | File | Description |
| :---: | :--- | :--- |
| 1 | `.govard.yml` | Base team config — main writable file |
| 2 | `.govard.<profile>.yml` | Team-shared profile override |
| 3 | `.govard.local.yml` | Legacy developer-local override |
| 4 | `.govard/.govard.local.yml` | **Preferred** developer-local override |
| 5 | `.govard.<env>.yml` | Legacy environment override |
| 6 | `.govard/.govard.<env>.yml` | **Preferred** environment override |

### Ownership Model

- **`.govard.yml`** — team-owned base config; target for all `govard config set` writes
- **Profile/local/env overrides** — read-only from CLI perspective; never auto-written by Govard

---

## Profiles

Use profiles when a team needs multiple runtime shapes for the same project.

```bash
govard config profile switch upgrade   # Switch to upgrade profile
govard env up --profile upgrade       # Or use --profile flag directly
govard db dump --profile perf
govard config profile clear            # Reset to default (no profile)
```

Govard loads `.govard.<profile>.yml` and creates an isolated compose file + separate data volumes, so profile switching does not contaminate existing data.

**Profile commands:**
- `govard config profile` - Show recommended profile for detected framework
- `govard config profile switch <name>` - Switch to a profile (persisted per-project)
- `govard config profile clear` - Reset to default profile

---

## Environment Override

```bash
export GOVARD_ENV=staging
govard env up
```

With `GOVARD_ENV=staging`, Govard additionally loads:
- `.govard.staging.yml`
- `.govard/.govard.staging.yml`

---

## Global Environment Variables

| Variable | Effect |
| :--- | :--- |
| `GOVARD_ENV` | Activate an environment-specific override layer (`.govard.<env>.yml` + `.govard/.govard.<env>.yml` are loaded last) |
| `GOVARD_HOME_DIR` | Override `~/.govard` — all state, compose files, certs, and caches |
| `GOVARD_BLUEPRINTS_DIR` | Override blueprint lookup location (merged `blueprints.FS`) |
| `GOVARD_IMAGE_REPOSITORY` | Override managed image repository prefix (e.g. `ghcr.io/my-org/govard-`) |
| `GOVARD_DOCKER_DIR` | Override local Docker build contexts for fallback builds |

`GOVARD_ENV` is the only variable that affects config layering; the other three affect filesystem/image resolution. Internal tests also use `GOVARD_GLOBAL_COMMANDS_DIR`, `GOVARD_OPERATIONS_LOG_PATH`, and `GOVARD_PROJECT_REGISTRY_PATH` to isolate global state — they are not needed for normal use.

---

## Example `.govard.yml`

```yaml
project_name: "my_project"
framework: "magento2"
framework_version: "2.4.7"
domain: "myproject.test"
table_prefix: "demo_"
lock:
  strict: false
blueprint_registry:
  provider: "http"
  url: "https://example.com/govard-blueprints.tar.gz"
  checksum: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  trusted: false
stack:
  php_version: "8.4"
  node_version: "24"
  db_version: "11.4"
  web_root: "/public"
  cache_version: "7.4"
  search_version: "3.4.0"
  queue_version: "3.13.7"
  xdebug_session: "PHPSTORM"
  xdebug_version: "3.4.5"
  composer_version: "latest"
  services:
    web_server: "nginx"
    db: "mariadb"
    search: "opensearch"
    cache: "redis"
    queue: "none"
  features:
    xdebug: true
    varnish: false
    isolated: false
    mftf: false
    frontend_sync: false
linked_projects:
    - "other-project"
    - "external-host.com:127.0.0.1"
```

---

## Key Fields

### Project Identity

| Field | Description |
| :--- | :--- |
| `project_name` | Unique project name (must be unique across all tracked projects) |
| `framework` | Detected or forced framework |
| `framework_version` | Framework version (used for version-aware profiles) |
| `domain` | Primary project domain (e.g. `myproject.test`) |
| `extra_domains` | Additional hostnames routed through the local proxy |
| `store_domains` | Magento multi-store hostname → scope code map |
| `table_prefix` | Magento 2, Mage-OS, Magento 1, OpenMage, or PrestaShop database table prefix; omit or leave empty for unprefixed schemas |
| `linked_projects` | List of dependencies (project names or IP:domain) for cross-project connectivity |

::: important IMPORTANT
`project_name` and `domain` must be **unique** across all tracked Govard projects. Govard blocks `init` and `env up` when another project uses the same identity.
:::

#### `store_domains` — Scalar Form (Legacy)

```yaml
store_domains:
  brand-b.test: brand_b
  brand-c.test: brand_c
```

#### `store_domains` — Object Form (Explicit Routing)

```yaml
store_domains:
  brand-b.test:
    code: base
    type: website
  brand-c.test:
    code: brand_c
    type: store
```

Object form instructs Govard to emit `MAGE_RUN_CODE` / `MAGE_RUN_TYPE` host mappings automatically.

#### `table_prefix` — Magento Schemas

Use `table_prefix` when the Magento database tables are prefixed, for example `demo_core_config_data`:

```yaml
table_prefix: "demo_"
```

Govard uses this value for Magento 2/Mage-OS `env.php`, Magento 1/OpenMage `local.xml`, PrestaShop `parameters.php`, `config auto` SQL, DB sync privacy filters, and Warden migration. The value must contain only letters, numbers, and underscores.

---

### Runtime Stack

| Field | Options | Description |
| :--- | :--- | :--- |
| `stack.services.web_server` | `nginx`, `apache`, `hybrid` | Web server |
| `stack.services.db` | `mariadb`, `mysql`, `none` | Database service |
| `stack.services.search` | `opensearch`, `elasticsearch`, `none` | Search engine |
| `stack.services.cache` | `redis`, `valkey`, `none` | Cache service |
| `stack.services.queue` | `rabbitmq`, `none` | Queue service |
| `stack.php_version` | e.g. `8.4`, `none` | PHP version (`none` = no PHP container) |
| `stack.node_version` | e.g. `24` | Node.js version |
| `stack.python_version` | e.g. `3.12` | Python version (Django, Dagster only; default `3.12`) |
| `stack.db_version` | e.g. `11.4` | Database version |
| `stack.web_root` | e.g. `/pub`, `/public` | Web root directory |
| `stack.composer_version` | `1`, `2`, `2.2`, `latest`, or any point version | Composer version |
| `stack.xdebug_session` | e.g. `PHPSTORM` | Xdebug session name |
| `stack.xdebug_version` | e.g. `3.4.5` | Override the PECL Xdebug version installed in `php-debug` (default: Govard's recommended version per PHP version — currently `3.4.5` for PHP 8.1-8.4, `3.5.3` for PHP 8.5 since it has no 3.4.x option). Forces a local image build since the exact version is baked into the image. |
| `stack.features.frontend_sync` | `true`, `false` | Enable built-in frontend synchronization for Magento 2 and Mage-OS only |
| `stack.features.varnish` | `true`, `false` | Enable Varnish cache service |
| `stack.features.xdebug` | `true`, `false` | Enable Xdebug and php-debug service |
| `stack.features.isolated` | `true`, `false` | Isolate network from external access |
| `stack.features.mftf` | `true`, `false` | Enable Magento Functional Testing Framework |

#### Frontend Synchronization

Set `stack.features.frontend_sync: true` only for Magento 2 or Mage-OS projects. This enables explicit frontend runtime discovery; `govard env up` does not start, wait for, or proxy frontend development services.

#### Connecting to Elasticsearch/OpenSearch from the Host

When `stack.services.search` is `elasticsearch` or `opensearch`, Govard automatically exposes the search engine's REST API on the host at:

```
http://<your-domain>:9200
```

For example, if your project's domain is `myshop.test`, run:

```bash
curl http://myshop.test:9200/_cluster/health
```

This works for every project simultaneously — Govard's shared proxy routes port `9200` by hostname, the same way it already routes `443`. If your project was created before this feature shipped, run `govard env up` once to re-render its compose file and recreate the `elasticsearch`/`opensearch` container on the `govard-proxy` network before `:9200` becomes reachable. There is no authentication or TLS on this port (matching the engine's own local-dev configuration), so treat it as local-development-only and do not expose it beyond your machine.

#### Connecting to the RabbitMQ Management UI from the Host

When `stack.services.queue` is `rabbitmq`, Govard automatically exposes the management UI on the host at:

```
http://<your-domain>:15672
```

For example, if your project's domain is `myshop.test`, open `http://myshop.test:15672` in a browser and log in with `guest` / `guest`. Routing works exactly like the search engine's `:9200` access above — the shared proxy routes port `15672` by hostname, so every project keeps its own UI simultaneously. There is no TLS on this port; treat it as local-development-only and do not expose it beyond your machine. If your project was created before this feature shipped, run `govard env up` once to re-render its compose file and recreate the `rabbitmq` container on the `govard-proxy` network before `:15672` becomes reachable.

Node-first frameworks auto-detect the package manager from `package.json`, `pnpm-workspace.yaml`, or lockfiles.

#### Composer Versioning Optimization
Govard provides first-class support for common Composer versions to ensure instant environment startup:
- **Pre-baked (Instant)**: `1`, `2`, `2.2`, `latest`. These versions are bundled in the PHP image and do not require downloading at runtime.
- **Dynamic (Auto-Download)**: Any other valid point release (e.g., `2.7.2`) can be specified. Govard will automatically download and verify the binary upon the first `env up`.

---

### Safety and Reproducibility

| Field | Description |
| :--- | :--- |
| `lock.strict` | Fail `env up` when lock state is missing or mismatched |
| `lock.ignore_fields` | Fields to skip during compliance checks (e.g. `host.docker_version`) |
| `blueprint_registry.*` | Opt-in remote blueprint source with checksum + trust requirements |

---

### Remotes

Remote definitions live under `remotes.<name>`. The name can be any valid identifier — Govard accepts standard names (`dev`, `staging`, `prod`) as well as **any custom name** using lowercase letters, digits, hyphens, or underscores (e.g. `qa`, `preprod`, `demo`, `client-uat`).

```yaml
remotes:
  staging:
    host: staging.example.com
    user: deploy
    path: /var/www/app
    port: 22
    capabilities:
      files: true
      media: true
      db: true
    protected: false
    auth:
      method: ssh-agent

  qa:
    host: qa.example.com
    user: deploy
    path: /var/www/app
    auth:
      method: keychain

  preprod:
    host: preprod.example.com
    user: deploy
    path: /var/www/app
    protected: true   # opt-in write protection for custom environments
    auth:
      method: ssh-agent
```

::: info NOTE
Only remotes whose name normalizes to `prod` (`prod`, `production`, `live`) are **automatically** write-protected. All other remotes — including custom names — default to unprotected. Use `protected: true` to opt in.
:::

Key subfields:

| Field | Description |
| :--- | :--- |
| `capabilities` | Scope flags: `files`, `media`, `db`, `deploy` |
| `protected` | Write-protect this remote |
| `auth.method` | `keychain`, `ssh-agent`, or `keyfile` |
| `auth.key_path` | Path to SSH key (for `keyfile` method) |
| `auth.strict_host_key` | Enable strict host-key verification |
| `auth.known_hosts_file` | Custom known_hosts file path |

Remote fields support `op://...` references resolved through the 1Password CLI.

→ Full guide: [Remotes and Sync](/workflows/remotes-and-sync)

---

### Deploy

The `deploy:` block configures how a git revision becomes a release on a target. A
remote may override any of its keys through `remotes.<name>.deploy.<key>` (and the
topology fields `branch`, `repository`, `deploy_path`, `publish`, `local` directly on
the remote); a command-line flag wins over both.

```yaml
deploy:
  keep_releases: 5
  command_timeout: 90m            # every step outside the maintenance window
  maintenance_timeout: 15m        # one step inside it
  lock_stale_after: 2h            # how old a lock may be before `unlock` takes it
  db_backup: true
  artifact_dir: artifacts         # its presence alone selects artifact mode
  verify:
    url: https://shop.example.com/
    timeout: 30s
  settings:                       # validated against the recipe; an unknown key exits 4
    php_bin: php8.3
    php_version: "8.3"
    mage_mode: production
  hooks:
    - { name: varnish-purge, on: "publish:activate", position: after, order: 10, run: "varnishadm ban req.url ~ /" }
```

| Field | Default | Description |
| :--- | :--- | :--- |
| `keep_releases` | `5` | how many releases `deploy:cleanup` keeps, with their database dumps |
| `command_timeout` | `30m` | bounds every step outside the maintenance window |
| `maintenance_timeout` | `15m` | bounds one step inside the window |
| `lock_stale_after` | `2h` | age at which `govard deploy unlock` releases a lock without `--force` |
| `db_backup` | `false` | dump the database before the first mutating task (`--db-backup` per run) |
| `artifact_dir` | — | an artifact directory; its presence resolves `--build=auto` to `artifact` |
| `verify.url` | — | the HTTP check `deploy:verify` runs after publish |
| `verify.timeout` | `30s` | how long that request may take |
| `settings` | recipe defaults | framework and engine settings, validated against the recipe |
| `hooks` | — | steps anchored on a task id, a stage alias (`stage:build`) or another hook |

Four engine settings decide what `govard sandbox` has to provide beyond the
profile. They are read from this project layer (a remote-level override is not
consulted: the sandbox remote is synthetic and never lives in configuration),
and each one **replaces** the recipe's list rather than extending it:

| Setting | Default | What it does |
| :--- | :--- | :--- |
| `sandbox_packages` | recipe | extra apt packages the sandbox image installs |
| `sandbox_extensions` | recipe | PHP extensions, installed as `php<series>-<name>` |
| `sandbox_services` | recipe | init services started in the container before sshd |
| `sandbox_tools` | recipe | binaries installed from the engine's known list (`wp-cli`) |

```yaml
deploy:
  settings:
    sandbox_packages: [postgresql]
    sandbox_extensions: [intl, pgsql, mbstring, xml, curl, zip]
    sandbox_services: [postgresql, redis-server]
```

A service name from the engine's table carries the package that provides it, so
`postgresql` is enough to install and start it. A service the image cannot start
is named when the container starts rather than skipped in silence.

Remote-level fields the deploy engine reads:

| Field | Description |
| :--- | :--- |
| `path` | the **served docroot**; whether it is absent, a symlink or a real directory decides the publish strategy |
| `deploy_path` | the layout root holding `releases/`, `shared/` and `.dep/`; probed from the target when omitted |
| `deploy.publish` | `auto` (default), `symlink` or `in_place` |
| `deploy.branch` / `deploy.repository` | overrides for the project-level values |
| `deploy.local` | run the pipeline against this machine instead of over SSH |

`deploy.settings` is validated against the framework's recipe before anything runs: an
unknown key or a value with the wrong shape exits `4` with the key named. String
settings must be quoted if they look numeric (`php_version: "8.2"`).

→ Full guides: [Deployment](/workflows/deployment) and
[Deployment case studies](/workflows/deploy-case-studies)

---

### Project Extensions

| Path | Purpose |
| :--- | :--- |
| `.govard/docker-compose.override.yml` | Compose overrides merged after framework includes |
| `.govard/commands/*` | Custom commands exposed via `govard custom` |
| `.govard/hooks/*` | Scripts referenced by `hooks.*.run` |
| `.govard/nginx/custom/*.conf` | Extra nginx directives included inside the rendered `server {}` block (nginx web server only) |
| `.govard/apache/custom/*.conf` | Extra Apache directives included inside the rendered `<VirtualHost>` block (Apache and hybrid web-server modes) |

**Lifecycle hook events:**

- `pre-up` / `post-up`
- `pre-down` / `post-down`
- `pre-deploy` / `post-deploy`
- `pre-delete` / `post-delete`

::: tip TIP
Govard fingerprints `.govard/docker-compose.override.yml`, `.govard/nginx/custom/`, and `.govard/apache/custom/`. If any of them change, the next `env up` auto-re-renders the compose output.

When overriding services, prefer additive merges (extra environment variables, labels, ports). Replacing full lists like `services.web.volumes` can discard required Govard-managed mounts. `.govard/nginx/custom/` and `.govard/apache/custom/` exist precisely so you don't have to replace the whole web server config just to add a directive.

Frameworks that declare a runtime audit profiler cause `govard env up` to
prepare and mount the active custom directory even when the project has no
user-authored snippets. `govard audit run --checks profiler` uses a nested
`.govard/nginx/custom/audit-profiler/` include inside Magento's PHP FastCGI
location for nginx, or a temporary `.govard/apache/custom/` vhost include for
Apache and hybrid. Files are uniquely named per audit run and removed during
lease-protected cleanup; no profiler setting is written to `.govard.yml` or
Magento's `app/etc/env.php`.
:::

### Audit Lint Providers

`audit.lint` configures which backend [`govard audit`](/reference/cli-commands#govard-audit)
uses for lint checks. Both keys are optional; with neither set, audits run the
Govard-owned native backend.

```yaml
audit:
  lint:
    # Default provider for this project. "govard" (the native backend) is the
    # default when omitted. Any other value must name a key below.
    provider: govard

    # Explicitly configured third-party lint containers. Nothing is discovered
    # automatically and none of these is ever a fallback for the native backend.
    external_providers:
      house-standard:
        type: docker            # required; "docker" is the only supported type
        image: example.invalid/lint@sha256:...  # required
        command: ["lint", "--report", "/output/report.json"]  # required, non-empty
```

| Key | Required | Purpose |
| :--- | :--- | :--- |
| `audit.lint.provider` | No | Project default provider. Must be `govard` or a key under `external_providers`. Lowercase letters, digits, hyphen, and underscore only. |
| `audit.lint.external_providers.<name>.type` | Yes | Provider kind. Only `docker` is supported. |
| `audit.lint.external_providers.<name>.image` | Yes | Container image to run. Pin it by digest so the evidence names an immutable runtime. |
| `audit.lint.external_providers.<name>.command` | Yes | Argument array for the container. Every element must be non-empty; there is no shell. |

Precedence is explicit: an `--lint-provider` flag always wins, then
`audit.lint.provider`, then the `govard` default. Credentials are deliberately not
configuration fields — Composer auth is read from `~/.composer/auth.json` and SSH
agent forwarding is opt-in per run via `--allow-lint-ssh-agent`.

::: warning WARNING
An external provider must produce Govard's lint report schema and echo back this
run's exact identity, or its report is quarantined rather than accepted as
evidence. Naming a provider that is not configured is an error, never a silent
fall back to `govard`. A standalone module target has no project configuration at
all, so only `govard` is available there.
:::

---

## Config Commands

```bash
govard config get stack.php_version
govard config set stack.php_version 8.4
govard config profile --json
govard config profile apply --framework laravel --framework-version 11
```

`govard config set` writes only to `.govard.yml` (the base config).

---

## Blueprint Registry

If `blueprint_registry` is enabled:

- `provider` must be `git` or `http`
- `url` is required
- `checksum` must be a 64-character SHA-256 hex string
- `trusted` must be `true`
- Remote payloads are cached under `~/.govard/blueprint-registry/`

Govard fails fast if the checksum does not match.

---

## Inter-Project Connectivity

By default, Govard projects are isolated. To allow a project to communicate with another Govard project via its `.test` domain, use the `linked_projects` field.

### Key Behaviors

- **Opt-in Visibility**: Hostnames for other projects are only injected into `/etc/hosts` if the project is explicitly listed in `linked_projects`.
- **Automatic Domain Resolution**: Listing a project name will automatically map its primary domain and all extra domains to the shared proxy IP.
- **Targeted Container Refresh**: When you start a project, Govard identifies which other running projects depend on it and restarts **only** those specific projects to update their host mappings.
- **Manual Mappings**: You can also provide raw mappings in the format `hostname:ip`.

```yaml
linked_projects:
  - "my-api-project"             # Project name
  - "custom.site:192.168.1.10"   # Manual mapping
```

**Example: a Dagster sync pipeline calling a Magento 2 store**

```yaml
# In the Dagster project's .govard.yml
linked_projects:
  - "shop"   # the Magento 2 project's name
```

The Dagster framework's compose blueprint already mounts and trusts
Govard's local root CA inside the container, so once `linked_projects`
resolves `shop`'s domain, pipeline code calling `https://shop.test`
(Magento 2's REST/GraphQL API) verifies TLS correctly - no
`verify=False`/`InsecureSkipVerify` needed.

---

[← CLI Commands](/reference/cli-commands) | [Frameworks →](/reference/frameworks)
