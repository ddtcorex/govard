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
| `GOVARD_HOME_DIR` | Override `~/.govard` |
| `GOVARD_BLUEPRINTS_DIR` | Override blueprint lookup location |
| `GOVARD_IMAGE_REPOSITORY` | Override managed image repository prefix |
| `GOVARD_DOCKER_DIR` | Override local Docker build contexts for fallback builds |

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
    livereload: false
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
| `stack.db_version` | e.g. `11.4` | Database version |
| `stack.web_root` | e.g. `/pub`, `/public` | Web root directory |
| `stack.composer_version` | `1`, `2`, `2.2`, `latest`, or any point version | Composer version |
| `stack.xdebug_session` | e.g. `PHPSTORM` | Xdebug session name |
| `stack.xdebug_version` | e.g. `3.4.5` | Override the PECL Xdebug version installed in `php-debug` (default: Govard's recommended version per PHP version — currently `3.4.5` for PHP 8.1-8.4, `3.5.3` for PHP 8.5 since it has no 3.4.x option). Forces a local image build since the exact version is baked into the image. |
| `stack.features.livereload` | `true`, `false` | Enable LiveReload port mapping (35729) |
| `stack.features.varnish` | `true`, `false` | Enable Varnish cache service |
| `stack.features.xdebug` | `true`, `false` | Enable Xdebug and php-debug service |
| `stack.features.isolated` | `true`, `false` | Isolate network from external access |
| `stack.features.mftf` | `true`, `false` | Enable Magento Functional Testing Framework |

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

### Project Extensions

| Path | Purpose |
| :--- | :--- |
| `.govard/docker-compose.override.yml` | Compose overrides merged after framework includes |
| `.govard/commands/*` | Custom commands exposed via `govard custom` |
| `.govard/hooks/*` | Scripts referenced by `hooks.*.run` |
| `.govard/nginx/custom/*.conf` | Extra nginx directives included inside the rendered `server {}` block (nginx web server only) |
| `.govard/apache/custom/*.conf` | Extra Apache directives included inside the rendered `<VirtualHost>` block (Apache web server only) |

**Lifecycle hook events:**

- `pre-up` / `post-up`
- `pre-down` / `post-down`
- `pre-deploy` / `post-deploy`
- `pre-delete` / `post-delete`

::: tip TIP
Govard fingerprints `.govard/docker-compose.override.yml`, `.govard/nginx/custom/`, and `.govard/apache/custom/`. If any of them change, the next `env up` auto-re-renders the compose output.

When overriding services, prefer additive merges (extra environment variables, labels, ports). Replacing full lists like `services.web.volumes` can discard required Govard-managed mounts. `.govard/nginx/custom/` and `.govard/apache/custom/` exist precisely so you don't have to replace the whole web server config just to add a directive.
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
