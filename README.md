# GOVARD: Go-based Versatile Runtime & Development

[![Go Version](https://img.shields.io/github/go-mod/go-version/ddtcorex/govard)](https://go.dev/)
[![License](https://img.shields.io/github/license/ddtcorex/govard)](LICENSE)
[![Releases](https://img.shields.io/github/v/release/ddtcorex/govard)](https://github.com/ddtcorex/govard/releases)
[![CI Pipeline](https://github.com/ddtcorex/govard/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/ddtcorex/govard/actions/workflows/ci-pipeline.yml)

**Govard** is a professional-grade local development orchestrator engineered in Go. It is designed to replace legacy bash-based tools with a high-performance, native binary that manages complex containerized environments with a focus on stability, speed, and a premium developer experience.

---

## 🚀 Key Features

- **Framework Discovery**: Automatically detects Magento 1/OpenMage, Magento 2, Laravel, Next.js, Drupal, Symfony, Shopware, CakePHP, and WordPress to generate tailored configurations.
- **Custom Recipe**: Interactive prompt to pick web server, database, cache, search, queue, and varnish for bespoke stacks.
- **Xdebug Routing**: Dedicated `php-debug` container, activated only when `XDEBUG_SESSION` cookie is present.
- **Queue Support**: Optional RabbitMQ service for async workloads.
- **High Performance**: Built with Go and utilizes the native Docker SDK for direct container orchestration.
- **Smart Templating**: Uses the Go `text/template` engine to render dynamic Docker Compose files from framework-specific Blueprints.
- **Magento 2 Optimized**: Deep integration for Magento 2, including automated `env.php` configuration, Varnish 7.x support, and Redis caching.
- **SSL Management**: Professional CA management for "Green Lock" HTTPS on local `.test` domains.
- **Zero-Config Debugging**: Seamless Xdebug 3 integration with one-click toggling.
- **Rich CLI UX**: Powered by `pterm` for beautiful terminal output, progress bars, and interactive prompts.
- **Desktop Dashboard**: Wails-based UI with live logs, quick actions, and settings.

---

## 🛠️ Installation

### One-Line Install (Linux/macOS)

Install the latest release binary with a single command:

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/scripts/install-release.sh | bash
```

Using `wget`:

```bash
wget -qO- https://raw.githubusercontent.com/ddtcorex/govard/master/scripts/install-release.sh | bash
```

Common options:

```bash
# Install to ~/.local/bin (no sudo)
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/scripts/install-release.sh | bash -s -- --local

# Install a specific version
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/scripts/install-release.sh | bash -s -- --version v1.3.0
```

By default this installs `govard` to `/usr/local/bin` and uses `sudo` when needed.

### Quick Install from Source

Ensure you have Go 1.24+ installed, then run:

```bash
go version
git clone https://github.com/ddtcorex/govard.git
cd govard
./scripts/install.sh
```

### Install from Source (Manual)

Ensure you have Go 1.24+ installed:

```bash
go version
git clone https://github.com/ddtcorex/govard.git
cd govard
make install
```

If `go` is installed but not found:

```bash
export PATH="$HOME/go/bin:$PATH"
```

### Docker Images (Build Args)

Govard uses a single PHP Dockerfile with build args instead of versioned folders.

```bash
docker build -f docker/php/Dockerfile -t govard/php:8.4 --build-arg PHP_VERSION=8.4 docker/php
docker build -f docker/php/magento2/Dockerfile -t govard/php-magento2:8.4 --build-arg PHP_VERSION=8.4 docker/php
```

---

## 💻 Usage

### 1. Initialize a Project

Navigate to your project root and run:

```bash
govard init
```

This scans your project (via `composer.json` or `package.json`) and generates a `govard.yml` configuration.

### 2. Start the Environment

```bash
govard env up
```

This renders a per-project compose file under `~/.govard/compose/` and starts your specialized stack in detached mode.

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

---

## SSL & HTTPS

Govard provides automated local HTTPS for all `.test` domains using a built-in certificate authority (Caddy).

### 1. DNS Resolver for `.test` Domains

You need a local DNS resolver that maps `*.test` to `127.0.0.1`. If you already have one, skip this.

Linux (systemd-resolved + dnsmasq example):

```bash
sudo apt-get install dnsmasq
echo "address=/.test/127.0.0.1" | sudo tee /etc/dnsmasq.d/govard-test.conf
sudo systemctl restart dnsmasq
```

Linux (systemd-resolved only):

```bash
sudo mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' | sudo tee /etc/systemd/resolved.conf.d/govard-test.conf
[Resolve]
DNS=127.0.0.1
Domains=~test
EOF
sudo systemctl restart systemd-resolved
```

Ubuntu (resolvconf):

```bash
sudo apt-get install resolvconf
echo "nameserver 127.0.0.1" | sudo tee /etc/resolvconf/resolv.conf.d/tail
sudo resolvconf -u
```

Arch Linux (systemd-resolved):

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

Fedora (systemd-resolved):

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

Before trusting the CA, ensure the global proxy is running:

```bash
govard svc up
```

To trust certificates generated by Govard on your machine, run:

```bash
govard doctor trust
```

macOS note:

`govard doctor trust` currently expects the Caddy root certificate at `/tmp/govard-ca.crt`.
Export it from the proxy container first:

```bash
docker cp proxy-caddy-1:/data/caddy/pki/authorities/local/root.crt /tmp/govard-ca.crt
govard doctor trust
```

### 3. Browser Configuration (Chrome/Edge/Brave)

On Linux, Chromium-based browsers may require a manual import of the Root CA to remove the "Your connection is not private" warning:

1.  **Locate the CA**: On Linux, the certificate is stored in `~/.govard/ssl/root.crt` (or `$HOME/.govard/ssl/root.crt`) after running `govard doctor trust`.
2.  **Open Settings**: Go to `chrome://settings/certificates` in your browser.
3.  **Import**: Navigate to the **Authorities** tab and click **Import**.
4.  **Select File**: Select the `root.crt` file from the path above.
5.  Restart your browser.

_Note: Once trusted, all `*.test` domains managed by Govard will show a "Green Lock" without further configuration._

---

## Project Structure

```text
.
├── blueprints/      # Docker Compose templates for specific frameworks
├── cmd/
│   ├── govard/      # CLI entry point
│   └── govard-desktop/ # Desktop entry point (Wails)
├── desktop/         # Desktop app assets (Wails frontend/config)
├── internal/
│   ├── cmd/         # CLI Command definitions (Cobra)
│   ├── engine/      # Core logic (Docker SDK, Discovery, Rendering)
│   ├── desktop/     # Desktop app glue (Wails bindings)
│   ├── ui/          # Styled terminal output logic
│   └── updater/     # Background update checking
├── Makefile         # Build and installation automation
└── govard.yml       # Project-specific configuration (Generated)
```

---

## 🔍 CLI Command Reference

| Command             | Description                                                   |
| :------------------ | :------------------------------------------------------------ |
| `govard init`       | Initialize a new project configuration                        |
| `govard bootstrap`  | Bootstrap local project setup and clone a remote environment |
| `govard env`        | Project-scoped lifecycle and service wrapper                 |
| `govard svc`        | Manage global services and workspace sleep state             |
| `govard tool`       | Run framework/tooling CLIs inside project containers         |
| `govard shell`      | Enter the application container                              |
| `govard db`         | Database operations (`connect`, `dump`, `import`, `stream`) |
| `govard debug`      | Toggle Xdebug for the current environment                    |
| `govard open`       | Open common service URLs                                     |
| `govard remote`     | Manage remote environments                                   |
| `govard sync`       | Synchronize files, media, and databases between environments |
| `govard status`     | List running project environments across workspace           |
| `govard doctor`     | Run system diagnostics and remediation helpers               |
| `govard config`     | Manage `govard.yml` configuration from CLI                   |
| `govard deploy`     | Deploy the application                                       |
| `govard snapshot`   | Manage local snapshots for database and media                |
| `govard lock`       | Generate and validate `govard.lock` snapshots               |
| `govard tunnel`     | Start a public tunnel to a local project URL                 |
| `govard custom`     | Run project custom commands from `.govard/commands`          |
| `govard projects`   | Query known projects from local registry                     |
| `govard extensions` | Manage project extension contract in `.govard`               |
| `govard desktop`    | Launch the Govard Desktop app (`--background` supported)     |
| `govard self-update` | Upgrade the Govard binary                                    |
| `govard upgrade`    | Upgrade the framework version                                |
| `govard version`    | Print the version number of Govard                           |

---

## 📚 Documentation

Documentation is organized by audience:

**For Users:**
- [Getting Started](./docs/user/getting-started.md) - Installation and basic workflow
- [Configuration](./docs/user/configuration.md) - `govard.yml` and blueprints
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

| Gate | Command |
| :--- | :------ |
| Quality Checks (Vet + Format) | `make vet` + `gofmt -s -l .` |
| Fast Tests (Frontend + Unit) | `make test-fast` |
| Integration Tests | `make test-integration-ci` |
| Build Binaries | `make build` |

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
