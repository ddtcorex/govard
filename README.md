# GOVARD: Go-based Versatile Runtime & Development

[![Go Version](https://img.shields.io/github/go-mod/go-version/ddtcorex/govard)](https://go.dev/)
[![License](https://img.shields.io/github/license/ddtcorex/govard)](LICENSE)
[![Releases](https://img.shields.io/github/v/release/ddtcorex/govard)](https://github.com/ddtcorex/govard/releases)
[![CI Pipeline](https://github.com/ddtcorex/govard/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/ddtcorex/govard/actions/workflows/ci-pipeline.yml)

**Govard** is a professional-grade local development orchestrator engineered in Go. It is designed to replace legacy bash-based tools with a high-performance, native binary that manages complex containerized environments with a focus on stability, speed, and a premium developer experience.

---

## 🆚 Why Govard Stands Out

At a glance, these are the areas where Govard delivers stronger day-to-day value than typical local-dev wrappers and compose helpers:

| Area | Govard Advantage |
| :--- | :--- |
| Core architecture | Native Go binary with direct Docker SDK orchestration (instead of shell-script glue), for more predictable lifecycle behavior. |
| Framework intelligence | Automatic framework discovery + framework-specific blueprints + custom stack wizard for tailored environments. |
| Magento depth | First-class Magento/OpenMage workflow (auto `env.php`/`local.xml` wiring, table prefix support, optional Varnish/Redis/queue/search, and dedicated `php-debug` routing). |
| Local HTTPS/DNS | Built-in Caddy + `dnsmasq` + Root CA auto-trust flow for `*.test` domains, with automatic HTTP to HTTPS 308 redirection for all services. |
| Remote safety | `remote`/`sync` protections for sensitive targets (`prod` write blocking, scoped capabilities, audit logs, resumable transfers). |
| Team reproducibility | `govard lock` + `lock.strict` to detect environment drift and enforce consistency across machines. |
| Recovery workflow | `govard snapshot` for quick local DB/media checkpoints before risky operations or upgrades. |
| CLI + Desktop parity | Same core engine exposed in both CLI and Wails Desktop app (live logs, operation events, quick actions). |
| Update integrity | `govard self-update` validates release checksums before replacing installed binaries (`govard` + detected `govard-desktop`). |

---

## 🚀 Key Features

- **Snapshot Compression**: Database snapshots are gzipped by default to reduce disk usage.
- **Automatic Tunnel URL**: One-click public tunnels (`govard tunnel start`) with automatic base URL update/revert for supported frameworks (**requires `cloudflared` binary**).
- **Integrated Testing**: Run `phpunit`, `phpstan`, and `mftf` directly with `govard test`.
- **Redis & Valkey Management**: Full support for Redis and Valkey CLI, flushing, and info across local and remote environments.
- **Database Observability**: Live query monitoring with `govard db top` and real-time progress bars for imports and syncs.
- **Zero-Config Debugging**: Seamless Xdebug 2 & 3 integration with one-click toggling, project-specific isolation (`<project>-docker`), and structured subcommands.
- **VSCode Integration**: `govard vscode setup [--global]` wires Intelephense, PHPStan, PHP CS Fixer, PHPCS, PHPUnit, and Xdebug to run inside the project container instead of requiring PHP on the host, prompting to install any missing extension along the way.
- **Framework Discovery**: Automatically detects Magento 1/OpenMage, Magento 2, Laravel, Next.js, Emdash, Drupal, Symfony, Shopware, CakePHP, PrestaShop, and WordPress to generate tailored configurations.
- **Custom Framework**: Interactive prompt to pick web server, database, cache, search, queue, and varnish for bespoke stacks.
- **Xdebug Routing**: Dedicated `php-debug` container, activated only when `XDEBUG_SESSION` cookie is present.
- **Inter-Project Connectivity**: Projects can securely communicate with each other (e.g., `curl https://other-project.test`) by explicitly declaring dependencies via `linked_projects`. This ensures network isolation by default and enables targeted container refreshes.
- **Queue Support**: Optional RabbitMQ service for async workloads.
- **High Performance**: Built with Go and uses the native Docker SDK for direct container orchestration.
- **Local Image Fallback**: Automatically builds missing Govard-managed images locally from embedded blueprints if they cannot be pulled from Docker Hub. Disable this retry with `--no-fallback`.
- **Smart Templating**: Uses Go `text/template` to render dynamic Docker Compose files from framework-specific blueprints.
- **Magento 2 Optimized**: Deep integration for Magento 2, including automated `env.php` configuration, table prefix propagation, Varnish 7.x support, and Redis caching.
- **Remote Management (Flagship)**: Manage named remotes for sync/deploy/db workflows with scope-based capabilities (`files,media,db,deploy`) and flexible auth modes (`keychain`, `ssh-agent`, `keyfile`).
- **Remote Safety Guardrails**: Production remotes are write-protected by default, with policy checks to block risky destination writes and explicit capability enforcement per operation.
- **Safe Cross-Environment Sync**: Bi-directional file/media/database sync with dry-run planning (`--plan`), privacy filters (`--no-noise`, `--no-pii`), auto-selection of the `staging` remote by default, resumable rsync by default (`--partial --append-verify`), include/exclude filters, and risk warnings for destructive flags.
- **Remote Auditability & Observability**: Remote operations are logged to `~/.govard/remote.log` and also emitted to `~/.govard/operations.log` for command traceability and desktop notifications.
- **Remote Connectivity Diagnostics**: `govard remote test` validates SSH + `rsync`, reports probe latency, and classifies failures (`network`, `auth`, `permission`, `host_key`, `dependency`) with remediation hints.
- **Smart Cleanup**: Automatically prunes stale Docker Compose files in the background once a day and provides a `govard env cleanup` command for immediate maintenance. Use `govard project delete` to completely remove a project's orchestration resources (containers, volumes, and proxy rules). This command is resilient and can clean up "ghost" projects even if their `.govard.yml` is missing. Use `govard doctor` to monitor directory saturation.
- **Secrets-Aware Remote Config**: Remote fields support `op://...` references resolved through 1Password CLI for safer credential handling.
- **SSL Management**: Professional CA management for "Green Lock" HTTPS on local `.test` domains.
- **Rich CLI UX**: Powered by `pterm` for terminal output, progress bars, and interactive prompts.
- **Global Services**: Built-in Proxy (Caddy), Mailpit, PHPMyAdmin, and Portainer (Default login for Portainer is `admin` / `AdminGovard123$`).
- **Search Engine Host Access**: Elasticsearch/OpenSearch is automatically reachable from the host at `http://<project>.test:9200` — no extra config, reuses the same Caddy proxy that serves your project's HTTPS domain.
- **Desktop Dashboard**: Wails-based UI with live logs, quick actions, and settings.
- **Native Framework Upgrades**: Multi-framework upgrade pipeline (`govard upgrade`) for Magento 2, Laravel, Symfony, and WordPress that automates environment restarts, dependency updates, and database migrations.

---

## 🛠️ Installation

### One-Line Install (Linux/macOS)

Install the latest release binary with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

Using `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

Common options:

```bash
# Install to ~/.local/bin (no sudo)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --local

# Install building from source (auto-installs Go 1.25 if needed)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash -s -- --source
```

By default this installs both `govard` and `govard-desktop` to `/usr/local/bin`, automatically detects/installs missing system dependencies (`certutil`, `WebKitGTK`), starts global services, and configures SSL trust.
On Linux, if a standalone `govard-desktop` archive is missing in a release, the installer falls back to extracting `govard-desktop` from the release `.deb` package.

Do not mix install channels on the same machine (for example: `.deb` + `make install` + `self-update` across different paths).  
Use one channel only, otherwise you can end up with conflicting binaries in `/usr/bin` and `/usr/local/bin`.

### Release Installers (CLI + Desktop)

Every tagged release now publishes installer packages that install both:

- `govard` (CLI)
- `govard-desktop` (Desktop runtime used by `govard desktop`)

From the release page:

- Linux: `govard_<version>_linux_<arch>.deb`
- macOS: `govard_<version>_Darwin_<arch>.pkg`

Linux (`.deb`) example:

```bash
sudo dpkg -i govard_<version>_linux_amd64.deb
```

macOS (`.pkg`) example:

```bash
sudo installer -pkg govard_<version>_Darwin_arm64.pkg -target /
```

### Quick Install from Source

Ensure you have the following prerequisites installed:

- **Go 1.25+**
- **Node.js 20+**
- **Yarn (v1.x)**
- **golangci-lint (v2.11+)**
- **Docker & Docker Compose**
- **Wails v2.11+** (required for desktop app development)

```bash
go version
node --version
yarn --version
golangci-lint --version
git clone https://github.com/ddtcorex/govard.git
cd govard
./install.sh --source
```

### Local Setup (For Developers)

If you are contributing to Govard, follow these steps to set up your environment:

1. **Go 1.25+**: Install from [go.dev](https://go.dev/dl/).
2. **Yarn**: Enable with `corepack enable` or `npm install -g yarn`.
3. **golangci-lint**: Install the latest version:

   ```bash
   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
   ```

4. **Wails v2.11+** (for desktop app development):

   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   wails version
   ```

If you don't have `sudo` privileges, you can install everything to a local directory and update your `PATH`.

### Docker Images (Build Args)

Govard uses a single PHP Dockerfile with build args instead of versioned folders.

```bash
docker build -f docker/php/Dockerfile -t ddtcorex/govard-php:8.4 --build-arg PHP_VERSION=8.4 docker/php
docker build -f docker/php/magento2/Dockerfile -t ddtcorex/govard-php-magento2:8.4 --build-arg PHP_VERSION=8.4 docker/php
```

---

## 💻 Usage

### 1. Initialize a Project

Navigate to your project root and run:

```bash
govard init
```

This scans your project (via `composer.json` or `package.json`) and generates a `.govard.yml` configuration.

Fresh Emdash projects are also supported:

```bash
govard bootstrap --framework emdash --fresh
govard env up
govard open admin
```

Fresh framework scaffolders that require an empty project root now stage their generated files in a temporary directory before syncing them back into the initialized project. This keeps `.govard.yml` in place after `govard init`.

Fresh WordPress bootstrap downloads core directly from `wordpress.org` and installs via PHP bootstrap scripts, so `wp-cli` is no longer required for the initial setup flow.

### 2. Start the Environment

```bash
govard env up
govard up --quickstart
```

This renders a per-project compose file under `~/.govard/compose/` and starts your specialized stack in detached mode. Use `--fallback-local-build` if you need to build missing images locally.

`govard env up` also re-renders generated web-server assets under `~/.govard/` before container startup, so setup changes in the current Govard build are applied without depending on cached Apache or Nginx image configs.

`govard env start` and `govard env restart` also re-apply local domain routing after containers come back up, so HTTPS and proxy registration stay in sync with the running project.

Each local project must use a unique `project_name` and primary domain. Govard now blocks `init`/`env up` when another tracked project already uses the same identity, instead of silently colliding with an existing environment.

Common root shortcuts are also available for day-to-day lifecycle work:

- `govard up` → `govard env up`
- `govard down` → `govard env down`
- `govard restart` → `govard env restart`
- `govard ps` → `govard env ps`
- `govard logs` → `govard env logs`

### 3. Configure the Stack

After switching profiles, restart the environment to apply changes:

```bash
govard config profile switch upgrade
govard env up
```

When starting the environment after a profile change, you'll be prompted:

```
Profile shift detected: PHP version changed: 8.2 -> 8.3
Continue with profile switch? [Y/n]
...
Run Magento auto-configuration? [Y/n]
```

**Control tuning behavior:**
```bash
govard env up --no-tuning  # Skip auto-configuration prompts
govard config auto         # Run manually after environment is up
```

### 4. Enter the Workspace

Access the application container immediately:

```bash
govard shell
```

### 5. Remote Management (Flagship)

Set up and validate a remote:

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
govard remote copy-id staging
govard remote test staging
```

Plan and run a safe sync:

```bash
govard sync --source staging --destination local --full --plan
govard sync --source staging --destination local --full
govard sync --source dev --media
govard sync --source prod --file --path "app/etc/config.php"
```

`--media` can be used without an explicit mode and defaults to `optimized`.

Inspect remote audit events:

```bash
govard remote audit tail --status failure --lines 50
```

Remote defaults and protections:

- `remote add` is interactive if flags are missing.
- `remote copy-id` transfers your local public key to the remote `authorized_keys`.
- Remote paths support `~/` home directory expansion on the remote host. In shell examples, use an absolute remote path or quote the value, for example `--path '~/public_html'`, so the local shell does not expand it first.
- `prod` remotes are write-protected by default.
- Capability scopes (`files,media,db,deploy`) are enforced per operation.
- File/media sync uses resumable rsync mode by default.
- Full docs: [Remotes and Sync](https://github.com/ddtcorex/govard/wiki/Remotes-and-Sync).

### 6. Common Operational Workflows

- `govard db ...` for dump, import, query, and connection helpers.
- `govard debug on|off` to toggle Xdebug for the current project.
- `govard snapshot create` before risky local upgrades or imports.
- `govard lock generate` / `govard lock check` to detect environment drift.
- `govard tunnel start` to expose a local project publicly (**requires `cloudflared` binary**).

---

## SSL & HTTPS

Govard provides automated local HTTPS for all `.test` domains using a built-in certificate authority (Caddy).

### 1. DNS Resolver for `.test` Domains

Govard now runs a built-in `dnsmasq` service on the local loopback interface (port 53) to automatically resolve `*.test` domains to your local environment.

You need to configure your operating system to forward `.test` queries to this local service.

**Linux (Ubuntu/Debian with systemd-resolved - Recommended):**

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' | sudo tee /etc/systemd/resolved.conf.d/govard-test.conf
[Resolve]
DNS=127.0.0.1
Domains=~test
EOF
sudo systemctl restart systemd-resolved
```

**Ubuntu (resolvconf - Legacy):**

```bash
sudo apt-get install resolvconf
echo "nameserver 127.0.0.1" | sudo tee /etc/resolvconf/resolv.conf.d/tail
sudo resolvconf -u
```

**Arch Linux (systemd-resolved):**

```bash
sudo systemctl enable --now systemd-resolved
sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
sudo mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' | sudo tee /etc/systemd/resolved.conf.d/govard-test.conf
[Resolve]
DNS=127.0.0.1
Domains=~test
EOF
sudo systemctl restart systemd-resolved
```

**Fedora (systemd-resolved):**

```bash
sudo systemctl enable --now systemd-resolved
sudo ln -sf /run/systemd/resolve/stub-resolv.conf /etc/resolv.conf
sudo mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' | sudo tee /etc/systemd/resolved.conf.d/govard-test.conf
[Resolve]
DNS=127.0.0.1
Domains=~test
EOF
sudo systemctl restart systemd-resolved
```

Verify DNS:

```bash
resolvectl query laravel.test
dig +short laravel.test
```

### 1.1 Inter-Project Requests From PHP Containers

Govard allows PHP runtimes (`php` and `php-debug`) to call other Govard project domains through the shared proxy. To enable this, you must explicitly declare the dependency in your `.govard.yml` using the `linked_projects` field:

```yaml
linked_projects:
  - project-b
```

When a project is linked:
1.  **Isolation by Default**: Only projects explicitly linked will have their domains injected into the container's `/etc/hosts`.
2.  **Targeted Restarts**: When `project-b` starts, Govard will refresh only the projects that depend on it (like `project-a`), ensuring minimal downtime for your environment.
3.  **Automatic Resolution**: Linking a project automatically maps its primary domain and all extra domains.

If project `A` is running and you start project `B` which `A` depends on, Govard will automatically refresh project `A`'s PHP runtimes. If connectivity issues persist, run:

```bash
govard doctor trust
govard env restart
```

macOS (Create a resolver file):

```bash
sudo mkdir -p /etc/resolver
echo "nameserver 127.0.0.1" | sudo tee /etc/resolver/test
```

### 2. Install the Root CA

By default, `govard svc up` and `govard svc restart` now auto-trust the Govard Root CA:

```bash
govard svc up
```

You can also run trust manually at any time:

```bash
govard doctor trust
```

What happens automatically:

- Exports Root CA from Caddy to `~/.govard/ssl/root.crt`
- Installs it into system trust store (Linux/macOS)
- Best-effort import into browser NSS stores (Chromium/Firefox) when `certutil` is available
- Makes the exported CA available to Govard PHP runtimes on the next `govard env up` / `govard env restart`

Optional flags on `svc up`/`svc restart`:

```bash
govard svc up --no-trust
govard svc up --no-fallback
```

### 3. Browser Configuration

Govard now tries to import browser trust automatically. If your browser still shows trust warnings:

1. **Locate the CA**: `~/.govard/ssl/root.crt` (or `$HOME/.govard/ssl/root.crt`).
2. **Open Settings**: Go to `chrome://settings/certificates` in your browser.
3. **Import**: Navigate to the **Authorities** tab and click **Import**.
4. **Select File**: Select the `root.crt` file from the path above.
5. Restart your browser.

_Note: Once trusted, all `*.test` domains managed by Govard will show a "Green Lock" without further configuration._

---

## 🌍 Global Services

Built-in services shared across all projects:

| Service | URL | Credentials |
| :--- | :--- | :--- |
| **Mailpit** | `https://mail.govard.test` | No auth; SMTP: `mail:1025` |
| **PHPMyAdmin** | `https://pma.govard.test` | Project DB credentials |
| **Portainer** | `https://portainer.govard.test` | `admin` / `AdminGovard123$` |

CLI shortcuts: `govard open mail`, `govard open db`, `govard open portainer`

---

## Project Structure

```text
.
├── cmd/
│   ├── govard/      # CLI entry point
│   └── govard-desktop/ # Desktop entry point (Wails)
├── desktop/         # Desktop app assets (Wails frontend/config)
├── internal/
│   ├── cmd/         # CLI Command definitions (Cobra)
│   ├── blueprints/  # Docker Compose templates for specific frameworks
│   ├── engine/      # Core logic (Docker SDK, Discovery, Rendering)
│   ├── desktop/     # Desktop app glue (Wails bindings)
│   ├── proxy/       # Caddy/proxy route and TLS helpers
│   ├── ui/          # Styled terminal output logic
│   └── updater/     # Background update checking
├── Makefile         # Build and installation automation
└── .govard.yml       # Project-specific configuration (Generated)
```

---

## 🔍 CLI Command Reference

Root lifecycle shortcuts:

- `govard up` → `govard env up`
- `govard down` → `govard env down`
- `govard restart` → `govard env restart`
- `govard ps` → `govard env ps`
- `govard logs` → `govard env logs`

Common command aliases:

- `govard boot` → `govard bootstrap`
- `govard cfg` → `govard config`
- `govard dbg` → `govard debug`
- `govard gui` → `govard desktop`
- `govard diag` → `govard doctor`
- `govard ext` → `govard extensions`
- `govard prj` → `govard project`
- `govard rmt` → `govard remote`
- `govard sh` → `govard shell`
- `govard snap` → `govard snapshot`

| Command              | Description                                                        |
| :------------------- | :----------------------------------------------------------------- |
| `govard init`        | Initialize a new project configuration                             |
| `govard bootstrap`   | Bootstrap local project setup and clone a remote environment       |
| `govard env`        | Project-scoped lifecycle; intelligently proxies Docker Compose commands  |
| `govard domain`     | Manage additional domains for the project                          |
| `govard svc`        | Manage global services (`proxy`, `mail`, `pma`, `portainer`)       |
| `govard tool`        | Run framework/tooling CLIs inside project containers               |
| `govard vscode`      | Run PHP tooling for editor integrations; `govard vscode setup [--global]` wires VSCode to use the container instead of the host |
| `govard shell`       | Enter the application container                                    |
| `govard db`          | Database operations (`connect`, `dump`, `import`, `query`, `info`) |
| `govard debug`       | Toggle Xdebug for the current environment                          |
| `govard open`        | Open service URLs (Admin, DB, Mail, Portainer). `db` opens PMA; use `--client` for protocol URLs. |
| `govard remote`      | Manage remote environments                                         |
| `govard sync`        | Synchronize files, media, and databases between environments       |
| `govard status`      | List running project environments across workspace                 |
| `govard doctor`      | Run system diagnostics (including compose directory saturation) and remediation helpers |
| `govard config`      | Manage `.govard.yml` configuration from CLI (`auto`, `profile`)      |
| `govard deploy`      | Run deploy lifecycle hooks (pre/post deploy)                      |
| `govard snapshot`    | Manage local snapshots for database and media                      |
| `govard lock`        | Generate and validate `govard.lock` snapshots                      |
| `govard tunnel`      | Start a public tunnel to a local project URL                       |
| `govard custom`      | Run project custom commands from `.govard/commands`                |
| `govard project`     | Query known projects from local registry (`list`, `open`, `delete`) |
| <code>govard&nbsp;extensions</code> | Manage project extension contract in `.govard`                     |
| `govard desktop`     | Launch the Govard Desktop app (`--background` supported)           |
| <code>govard&nbsp;self&#8209;update</code> | Upgrade installed Govard binaries (`govard` + detected `govard-desktop`) |
| `govard upgrade`     | Native framework upgrade pipeline (Magento 2, Laravel, Symfony, WordPress) |
| `govard version`     | Print the version number of Govard                                 |
| `govard redis`       | Smart shortcut for project Redis Management                        |
| `govard varnish`     | Smart shortcut for project Varnish Management                      |
| `govard rabbitmq`    | Smart shortcut for project RabbitMQ Management                     |

---

## 📚 Documentation

Full documentation is available on the [**GitHub Wiki**](https://github.com/ddtcorex/govard/wiki):

- [Getting Started](https://github.com/ddtcorex/govard/wiki/Getting-Started) - Installation and first project workflow
- [CLI Commands](https://github.com/ddtcorex/govard/wiki/CLI-Commands) - CLI reference, shortcuts, tools, diagnostics, and utilities
- [Configuration](https://github.com/ddtcorex/govard/wiki/Configuration) - `.govard.yml`, profiles, remotes, and blueprint registry
- [Remotes and Sync](https://github.com/ddtcorex/govard/wiki/Remotes-and-Sync) - Remote setup, sync flows, audit logs, and remote DB work
- [Frameworks](https://github.com/ddtcorex/govard/wiki/Frameworks) - Support matrix and framework-specific notes
- [SSL and Domains](https://github.com/ddtcorex/govard/wiki/SSL-and-Domains) - Local HTTPS, CA trust, and domain routing
- [Desktop App](https://github.com/ddtcorex/govard/wiki/Desktop-App) - Desktop surface and dev-mode workflow
- [Architecture](https://github.com/ddtcorex/govard/wiki/Architecture) - System design and module layout
- [Contributing](https://github.com/ddtcorex/govard/wiki/Contributing) - Build, test, and contribution workflow
- [FAQ & Troubleshooting](https://github.com/ddtcorex/govard/wiki/FAQ) - Common issues and solutions

---

## ✅ Quality Gates

Govard CI runs these checks on every push and pull request:

| Pipeline Job | Local Command | Description |
| :--- | :--- | :--- |
| **Quality Checks** | `make lint fmt-check vet` | Runs `golangci-lint`, checks `gofmt -s` compliance, and `go vet`. |
| **Full Tests** | `make test` | Runs lint, format check, `go vet`, frontend tests, and Go unit tests. |
| **Integration Tests** | `make test-integration` | Builds a test binary and runs end-to-end framework tests in Docker. |
| **Build Binaries** | `make build` | Verifies that the project compiles for the current platform. |

### Recommended Local Workflow

To ensure your contribution passes the GitHub CI pipeline, run the following sequence before pushing:

#### Validation

Runs the full suite including linting, formatting checks, vet, frontend tests, Go unit tests, and integration tests (requires Docker):

```bash
make test
```

If the CI pipeline fails and you want to reproduce the exact check that failed locally, you can use these granular commands:

- `make lint` — Checks code style and static analysis (synchronized with CI version).
- `make fmt-check` — Checks if any files need `go fmt -s`.
- `make vet` — Runs `go vet`.
- `make test-unit` — Runs only Go unit tests.
- `make test-frontend` — Runs only Node.js frontend tests.
- `make test-integration-ci` — Runs integration tests in parallel (CI behavior).

---

## 🤝 Contributing

We welcome contributions! Please feel free to submit Pull Requests or open Issues on GitHub.

1. Fork the Repository
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 📜 License

Distributed under the MIT License. See `LICENSE` for more information.

---

**Developed with ❤️ by [ddtcorex](https://github.com/ddtcorex)**
