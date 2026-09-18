# GOVARD: Go-based Versatile Runtime & Development

[![Go Version](https://img.shields.io/github/go-mod/go-version/ddtcorex/govard)](https://go.dev/)
[![License](https://img.shields.io/github/license/ddtcorex/govard)](LICENSE)
[![Releases](https://img.shields.io/github/v/release/ddtcorex/govard)](https://github.com/ddtcorex/govard/releases)
[![Release downloads](https://img.shields.io/github/downloads/ddtcorex/govard/total?label=Release%20downloads)](https://github.com/ddtcorex/govard/releases)
[![CI Pipeline](https://github.com/ddtcorex/govard/actions/workflows/ci-pipeline.yml/badge.svg)](https://github.com/ddtcorex/govard/actions/workflows/ci-pipeline.yml)

**Govard** is a professional-grade local development orchestrator engineered in Go. It replaces legacy bash-based tooling with a fast native binary that manages complex containerized environments with a focus on stability, speed, and developer experience.

---

## 🆚 Why Govard Stands Out

| Area | Govard Advantage |
| :--- | :--- |
| Core architecture | Native Go binary with direct Docker SDK orchestration instead of shell-script glue. |
| Framework intelligence | Automatic framework discovery + framework-specific blueprints + custom stack wizard. |
| Magento depth | First-class Magento/OpenMage workflow (auto `env.php`/`local.xml` wiring, table prefixes, Varnish/Redis/queue/search, dedicated `php-debug` routing). |
| Local HTTPS/DNS | Built-in Caddy + `dnsmasq` + Root CA auto-trust for `*.test` domains. |
| Remote safety | `remote`/`sync` protections for sensitive targets (prod write blocking, scoped capabilities, audit logs). |
| Deployment | A framework recipe drives a neutral task pipeline (`deploy`), a container sandbox rehearses it locally, and an artifact mode keeps CI jobs toolchain-free. |
| Team reproducibility | `govard lock` + `lock.strict` to detect environment drift across machines. |
| Recovery workflow | `govard snapshot` for quick local DB/media checkpoints before risky operations. |

---

## 🚀 Key Features

- **First-Class Deployment**: `govard deploy` publishes a revision over SSH + rsync with a framework recipe, atomic symlink swap or in-place publish, maintenance windows, database backup, verification, rollback and resume — plus `govard sandbox` to rehearse the pipeline against a container on your machine.
- **Remote Management (Flagship)**: named remotes with scope-based capabilities (`files,media,db,deploy`), flexible auth (`keychain`, `ssh-agent`, `keyfile`), safe sync with dry-run planning, and connectivity diagnostics (`govard remote test`).
- **Shared SSH Gateway**: `govard-proxy-sshd` gives every sandbox a stable address (`ssh -p 2222 <project>@127.0.0.1`, sftp included) instead of an ephemeral port.
- **Infra Host Access**: Elasticsearch/OpenSearch at `http://<project>.test:9200` and the RabbitMQ management UI at `http://<project>.test:15672` — per-project Caddy routes, no extra config.
- **Framework Discovery**: Magento 1/OpenMage, Magento 2, Mage-OS, Laravel, Next.js, Emdash, Drupal, Symfony, Shopware, CakePHP, PrestaShop, WordPress, and Django, plus an interactive custom-framework wizard.
- **Zero-Config Debugging**: Xdebug 2 & 3 with one-click toggling and project-specific isolation.
- **Database Observability**: live query monitoring (`govard db top`), progress bars for imports/syncs, Redis/Valkey management.
- **Static Analysis & Profiling**: `govard audit` (lint, integrity, profiler) with persisted sessions, diffs, and reruns.
- **VSCode Integration**: `govard vscode setup [--global]` runs Intelephense, PHPStan, PHPCS, PHPUnit, and Xdebug inside the container.
- **Global Services**: Caddy proxy, Mailpit, PHPMyAdmin, Portainer out of the box.
- **Team Safety Nets**: snapshots, drift detection, 1Password (`op://`) secret references, resumable transfers.
- **CLI + Desktop**: the same engine in a terminal binary and a Wails desktop app; `govard self-update` with checksum validation.

---

## 🛠️ Installation

Pick one channel and stick to it:

| Channel | Command | Notes |
|---|---|---|
| npm | `npm i -g @ddtcorex/govard` | Node 20+, CLI only, any OS |
| Homebrew | `brew install ddtcorex/tap/govard` | macOS + Linuxbrew, CLI only |
| Docker | `docker run ghcr.io/ddtcorex/govard:<version> version` | No install needed; CI-friendly |
| CI | `uses: ddtcorex/setup-govard@v1` | GitHub Actions, pinnable version |
| Script | `curl -fsSL .../install.sh \| bash` | Full installer (CLI + Desktop where supported) |

```bash
curl -fsSL https://raw.githubusercontent.com/ddtcorex/govard/master/install.sh | bash
```

The installer handles system dependencies, starts global services, and configures SSL trust. Tagged releases also ship `.deb`/`.pkg` installers — see the [releases page](https://github.com/ddtcorex/govard/releases). Do not mix channels on one machine (conflicting binaries across `/usr/bin` and `/usr/local/bin`).

Govard runs without Docker for host-side commands (`govard capabilities` lists every command's requirement); container-backed commands exit `3` with `CAPABILITY_MISSING` instead of failing midway. Details: [Runs Without Docker](docs/reference/docker-free.md).

Contributors build from source (`./install.sh --source -y`, needs Go 1.25+, Node 20+) — see [CONTRIBUTING.md](CONTRIBUTING.md).

---

## 💻 Usage

### 1. Initialize and start

```bash
govard init          # scan the project, generate .govard.yml
govard env up        # render compose, start the stack (govard up works too)
govard shell         # enter the application container
```

### 2. Remotes and sync

```bash
govard remote add staging --host staging.example.com --user deploy --path /var/www/app
govard remote copy-id staging
govard remote test staging
govard sync --source staging --destination local --full --plan   # dry-run first
govard sync --source staging --destination local --full
```

`prod` remotes are write-protected by default; file/media sync is resumable rsync. Full docs: [Remotes and Sync](docs/workflows/remotes-and-sync.md).

### 3. Deployment

```bash
govard deploy plan staging     # the whole task list, connecting nowhere
govard deploy check staging    # preflight: connectivity, layout, permissions, php, disk, lock
govard deploy staging --yes    # deploy the local HEAD (or --revision <sha>)
govard deploy releases staging # what is on the target
govard deploy rollback staging # put the previous release back
```

Rehearse against a container first, or split the build off to CI:

```bash
govard sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
govard sandbox down --purge
```

- Full guide: [Deployment](docs/workflows/deployment.md)
- Worked configurations (Luma, Hyvä, themes, modes, webroots): [Deployment case studies](docs/workflows/deploy-case-studies.md)

### 4. Everyday operations

```bash
govard db dump -e staging      # dump / import / query / top
govard debug on                # toggle Xdebug for the current project
govard snapshot create         # checkpoint before risky upgrades
govard audit run               # static analysis with persisted sessions
govard tunnel start            # expose locally via cloudflare (needs the binary)
```

Command reference (shortcuts, aliases, every command): [CLI Commands](docs/reference/cli-commands.md).

---

## SSL & HTTPS

Govard serves every `.test` domain over HTTPS via Caddy + a local Root CA, with `dnsmasq` resolving `*.test` to loopback. Point your resolver at it once:

| OS | Setup |
|---|---|
| Linux (systemd-resolved) | `DNS=127.0.0.1` + `Domains=~test` under `/etc/systemd/resolved.conf.d/` |
| macOS | `echo "nameserver 127.0.0.1" \| sudo tee /etc/resolver/test` |

Then `govard svc up` auto-trusts the Root CA (system store + best-effort browser import). Details and troubleshooting: [SSL and Domains](docs/workflows/ssl-and-domains.md).

---

## 🌍 Global Services

Shared across all projects (via `govard svc`):

| Service | URL | Credentials |
| :--- | :--- | :--- |
| **Mailpit** | `https://mail.govard.test` | No auth; SMTP: `mail:1025` |
| **PHPMyAdmin** | `https://pma.govard.test` | Project DB credentials |
| **Portainer** | `https://portainer.govard.test` | `admin` / `AdminGovard123$` |
| **Search API** | `http://<project>.test:9200` | No auth (Elasticsearch/OpenSearch) |
| **RabbitMQ UI** | `http://<project>.test:15672` | `guest` / `guest` |
| **SSH gateway** | `ssh -p 2222 <project>@127.0.0.1` | Your allowlisted key (`govard gateway allow-key`) |

CLI shortcuts: `govard open mail|db|portainer`.

---

## 📚 Documentation

Full documentation (auto-synced to the [GitHub Wiki](https://github.com/ddtcorex/govard/wiki)):

- [Getting Started](docs/getting-started/getting-started.md) - Installation and first project workflow
- [CLI Commands](docs/reference/cli-commands.md) - Shortcuts, tools, diagnostics, utilities
- [Configuration](docs/reference/configuration.md) - `.govard.yml`, profiles, remotes, blueprint registry
- [Deployment](docs/workflows/deployment.md) + [Case studies](docs/workflows/deploy-case-studies.md)
- [Remotes and Sync](docs/workflows/remotes-and-sync.md) - Setup, flows, audit logs, remote DB work
- [SSL and Domains](docs/workflows/ssl-and-domains.md) - HTTPS, CA trust, routing
- [Frameworks](docs/reference/frameworks.md) - Support matrix and framework notes
- [Desktop App](docs/workflows/desktop-app.md) - Desktop surface and dev-mode workflow

---

## 🤝 Contributing

Builds, tests, and the mandatory brainstorming → writing-plans → executing-plans workflow live in [CONTRIBUTING.md](CONTRIBUTING.md) and `AGENTS.md`. Quick gate before pushing: `make test && make build`.

---

## 📜 License

Distributed under the MIT License. See `LICENSE` for more information.

---

**Developed with ❤️ by [ddtcorex](https://github.com/ddtcorex)**
