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
| Magento depth | First-class Magento 2 workflow (auto `env.php` wiring, optional Varnish/Redis/queue/search, and dedicated `php-debug` routing). |
| Local HTTPS/DNS | Built-in Caddy + `dnsmasq` + Root CA auto-trust flow for `*.test` domains, reducing manual local proxy/cert setup. |
| Remote safety | `remote`/`sync` protections for sensitive targets (`prod` write blocking, scoped capabilities, audit logs, resumable transfers). |
| Team reproducibility | `govard lock` + `lock.strict` to detect environment drift and enforce consistency across machines. |
| Recovery workflow | `govard snapshot` for quick local DB/media checkpoints before risky operations or upgrades. |
| CLI + Desktop parity | Same core engine exposed in both CLI and Wails Desktop app (live logs, operation events, quick actions). |
| Update integrity | `govard self-update` validates release checksums before replacing installed binaries (`govard` + detected `govard-desktop`). |

---

## 🚀 Key Features

- **Framework Discovery**: Automatically detects Magento 1/OpenMage, Magento 2, Laravel, Next.js, Drupal, Symfony, Shopware, CakePHP, and WordPress to generate tailored configurations.
- **Custom Framework**: Interactive prompt to pick web server, database, cache, search, queue, and varnish for bespoke stacks.
- **Xdebug Routing**: Dedicated `php-debug` container, activated only when `XDEBUG_SESSION` cookie is present.
- **Queue Support**: Optional RabbitMQ service for async workloads.
- **High Performance**: Built with Go and utilizes the native Docker SDK for direct container orchestration.
- **Local Image Fallback**: Automatically build missing Govard-managed images locally from embedded blueprints if they cannot be pulled from Docker Hub (via `--fallback-local-build`).
- **Smart Templating**: Uses the Go `text/template` engine to render dynamic Docker Compose files from framework-specific Blueprints.
- **Magento 2 Optimized**: Deep integration for Magento 2, including automated `env.php` configuration, Varnish 7.x support, and Redis caching.
- **Remote Management (Flagship)**: Manage named remotes for sync/deploy/db workflows with scope-based capabilities (`files,media,db,deploy`) and flexible auth modes (`keychain`, `ssh-agent`, `keyfile`).
- **Remote Safety Guardrails**: Production remotes are write-protected by default, with policy checks to block risky destination writes and explicit capability enforcement per operation.
- **Safe Cross-Environment Sync**: Bi-directional file/media/database sync with dry-run planning (`--plan`), resumable rsync by default (`--partial --append-verify`), include/exclude filters, and risk warnings for destructive flags.
- **Remote Auditability & Observability**: Remote operations are logged to `~/.govard/remote.log` and also emitted to `~/.govard/operations.log` for command traceability and desktop notifications.
- **Remote Connectivity Diagnostics**: `govard remote test` validates SSH + `rsync`, reports probe latency, and classifies failures (`network`, `auth`, `permission`, `host_key`, `dependency`) with remediation hints.
- **Secrets-Aware Remote Config**: Remote fields support `op://...` references resolved through 1Password CLI for safer credential handling.
- **SSL Management**: Professional CA management for "Green Lock" HTTPS on local `.test` domains.
- **Zero-Config Debugging**: Seamless Xdebug 3 integration with one-click toggling.
- **Rich CLI UX**: Powered by `pterm` for beautiful terminal output, progress bars, and interactive prompts. Use `--verbose` for deeper diagnostic trace logging via `log/slog`.
- **Global Services**: Built-in Proxy (Caddy), Mailpit, PHPMyAdmin, and Portainer.
- **Desktop Dashboard**: Wails-based UI with live logs, quick actions, and settings.

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

By default this installs both `govard` and `govard-desktop` to `/usr/local/bin` and uses `sudo` when needed.
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
- **golangci-lint (v1.64+)**
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

### 2. Start the Environment

```bash
govard env up
```

This renders a per-project compose file under `~/.govard/compose/` and starts your specialized stack in detached mode. Use `--fallback-local-build` if you need to build missing images locally.

### 3. Configure the Stack

Specifically for Magento 2, you can auto-inject the container settings into your application:

```bash
govard config auto
```

### 4. Enter the Workspace

Access the application container immediately:

```bash
govard shell
```

### 5. Remote Management (Flagship)

Set up and validate a remote:

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/html
govard remote test staging
```

Plan and run a safe sync:

```bash
govard sync --source staging --destination local --full --plan
govard sync --source staging --destination local --full
```

Inspect remote audit events:

```bash
govard remote audit tail --status failure --lines 50
```

Remote defaults and protections:

- `prod` remotes are write-protected by default.
- Capability scopes (`files,media,db,deploy`) are enforced per operation.
- File/media sync uses resumable rsync mode by default.
- Full docs: `docs/commands/remote.md` and `docs/commands/sync.md`.

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

Optional flags on `svc up`/`svc restart`:

```bash
govard svc up --auto-trust=false
govard svc up --trust-browsers=false
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

| Command              | Description                                                        |
| :------------------- | :----------------------------------------------------------------- |
| `govard init`        | Initialize a new project configuration                             |
| `govard bootstrap`   | Bootstrap local project setup and clone a remote environment       |
| `govard env`         | Project-scoped lifecycle and service wrapper                       |
| `govard domain`      | Manage additional domains for the project                          |
| `govard svc`         | Manage global services and workspace sleep state                   |
| `govard tool`        | Run framework/tooling CLIs inside project containers               |
| `govard shell`       | Enter the application container                                    |
| `govard db`          | Database operations (`connect`, `dump`, `import`, `query`, `info`) |
| `govard debug`       | Toggle Xdebug for the current environment                          |
| `govard open`        | Open common service URLs (Admin, DB, Mail, Portainer). `db` opens PMA by default, use `--client` for protocol URLs. |
| `govard remote`      | Manage remote environments                                         |
| `govard sync`        | Synchronize files, media, and databases between environments       |
| `govard status`      | List running project environments across workspace                 |
| `govard doctor`      | Run system diagnostics and remediation helpers                     |
| `govard config`      | Manage `.govard.yml` configuration from CLI                        |
| `govard deploy`      | Run deploy lifecycle hooks (pre/post deploy)                      |
| `govard snapshot`    | Manage local snapshots for database and media                      |
| `govard lock`        | Generate and validate `govard.lock` snapshots                      |
| `govard tunnel`      | Start a public tunnel to a local project URL                       |
| `govard custom`      | Run project custom commands from `.govard/commands`                |
| `govard projects`    | Query known projects from local registry                           |
| `govard extensions`  | Manage project extension contract in `.govard`                     |
| `govard desktop`     | Launch the Govard Desktop app (`--background` supported)           |
| `govard self-update` | Upgrade installed Govard binaries (`govard` + detected `govard-desktop`) |
| `govard upgrade`     | Upgrade the framework version                                      |
| `govard version`     | Print the version number of Govard                                 |

---

## 📚 Documentation

Documentation is organized by audience:

**For Users:**

- [Getting Started](./docs/user/getting-started.md) - Installation and basic workflow
- [Configuration](./docs/user/configuration.md) - `.govard.yml` and blueprints
- [CLI Commands](./docs/user/commands.md) - Complete command reference
- [SSL & HTTPS](./docs/user/ssl-https.md) - Local HTTPS setup

**For Framework Users:**

- [Magento 1 (OpenMage)](./docs/frameworks/magento1.md) - Magento 1/OpenMage support
- [Magento 2](./docs/frameworks/magento2.md) - Magento-specific features
- [Custom](./docs/frameworks/custom.md) - Build your own service mix

**For Developers:**

- [Architecture](./docs/dev/architecture.md) - System design and specs
- [Contributing](./docs/dev/contributing.md) - Development workflow
- [Desktop](./docs/desktop/README.md) - Desktop app roadmap and integration

---

## ✅ Quality Gates

Govard CI runs the following checks on push/PR:

| Gate                          | Command                      |
| :---------------------------- | :--------------------------- |
| Quality Checks (Vet + Format) | `make vet` + `gofmt -s -l .` |
| Fast Tests (Frontend + Unit)  | `make test-fast`             |
| Integration Tests             | `make test-integration-ci`   |
| Build Binaries                | `make build`                 |

Recommended local pre-push sequence:

```bash
make test-fast
```

If CI fails and you want to reproduce locally, run:

```bash
make vet
gofmt -s -l .
make test-fast
make test-integration-ci
make build
```

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

Distributed under the GPL-3.0 License. See `LICENSE` for more information.

---

**Developed with ❤️ by [ddtcorex](https://github.com/ddtcorex)**
