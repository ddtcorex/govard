---
title: Govard Architecture
description: A high-level look at Govard's internal architecture — the Go engine, blueprint rendering, Docker Compose generation, and CLI/desktop parity.
---

# Architecture

This document describes the current system shape of Govard at a high level.

---

## Product Shape

Govard is a Go-native local development orchestrator with two user surfaces:

- **CLI** powered by Cobra
- **Desktop app** powered by Wails

Both surfaces reuse the same runtime engine for discovery, rendering, Docker orchestration, proxy integration, and remote workflows.

---

## Core Runtime

### CLI Layer

```
cmd/govard/main.go           CLI entrypoint
internal/cmd/                Command implementations (Cobra)
internal/ui/                 Terminal rendering helpers (pterm)
```

### Engine Layer

```
internal/engine/             Discovery, config, blueprint logic
internal/engine/bootstrap/   Framework bootstrap workflows
internal/engine/remote/      Remote sync/deploy/SSH helpers
internal/proxy/              Caddy/proxy route and TLS helpers
internal/updater/            Update-check notification logic
```

### Startup Pipeline

`govard env up` follows the same core phases across all frameworks:

```
1. Detect framework context
       ↓
2. Validate config and host prerequisites
       ↓
3. Render compose file into ~/.govard/compose/
       ↓
4. Start runtime containers
       ↓
5. Verify proxy and host wiring
```

---

## Networking

| Component | Role |
| :--- | :--- |
| **Caddy** | Shared reverse proxy; terminates HTTPS for all `.test` domains |
| **dnsmasq** | Local DNS service; resolves `*.test` to loopback |
| **Docker networks** | Per-project PHP/DB networks + shared `govard-proxy` network |

The proxy terminates HTTPS and forwards requests into the current project stack.
For PHP runtimes, Govard also injects known Govard `.test` host aliases via `host-gateway`, so one project can call another through Caddy without attaching `php` or `php-debug` directly to the shared proxy network.

---

## Configuration Model

Govard composes layered config files on top of framework blueprints:

```
.govard.yml                  Base config (team-owned, writable)
   ↓
.govard.<profile>.yml        Profile override (read-only)
   ↓
.govard.local.yml            Local developer override (read-only)
   ↓
.govard.<env>.yml            Environment override (read-only)
```

Key design points:
- `.govard.yml` is the only writable config surface
- Runtime defaults are framework-aware and optionally version-aware
- Remote definitions, hooks, and project extensions live in `.govard/*`

See [Configuration](/reference/configuration) for the complete contract.

---

## Framework Support

Each of Govard's 13 supported frameworks is registered in `internal/frameworks/<name>/` as a `types.FrameworkDefinition` — one struct carrying the framework's detection signature, runtime/manifest data, and dispatch hooks (bootstrap factory, tunnel base-URL rewriter, `govard bootstrap` support flags). `internal/frameworks/all.go`'s `init()` registers all 13 into a package-level registry (`internal/frameworks/registry.go`) that `internal/frameworks/run.go` and `internal/frameworks/base_url.go` dispatch through, instead of hardcoded `switch framework { ... }` statements scattered across the codebase.

Discovery (`engine.DetectFramework`) inspects project manifests — composer.json requires, package.json deps, auth.json hosts, file-path signatures — and maps to framework defaults for web root, PHP/Node versions, database engine, and optional cache/search/queue/Varnish services, sourced from `engine.GetFrameworkConfig`/`engine.GetFrameworkManifestConfig` (still the authoritative data, composed into each `FrameworkDefinition`).

Magento 2 and its Mage-OS fork receive the deepest integration: auto-configuration, version-aware search/cache defaults, and dedicated debug routing.

See [Adding a New Framework](/developer/adding-a-framework) for the full internal structure and a file-by-file guide to adding one.

---

## Desktop Architecture

The desktop app focuses on operational workflows via a modular vanilla JS frontend.

```
cmd/govard-desktop/          Desktop entrypoint
internal/desktop/            Wails bindings
desktop/frontend/            Frontend assets (embedded in binary)
  ├── index.html             Main HTML
  ├── main.js                Bootstrap + event wiring
  ├── services/bridge.js     Backend RPC bridge
  ├── state/store.js         Shared UI state
  ├── modules/               Feature modules
  ├── ui/                    Toast, notifications
  └── utils/                 DOM helpers
```

Desktop operations call the CLI command surface directly (e.g., `govard up`, `govard svc up`) rather than bypassing it — ensuring CLI and desktop behavior stay aligned.

---

## Project Layout

```
.
├── cmd/
│   ├── govard/              CLI entry point
│   └── govard-desktop/      Desktop entry point (Wails)
├── desktop/                 Desktop app assets (Wails frontend/config)
├── internal/
│   ├── cmd/                 CLI command definitions (Cobra)
│   ├── blueprints/          Docker Compose templates per framework
│   ├── engine/              Core logic (Docker SDK, discovery, rendering)
│   │   └── bootstrap/       Per-framework FrameworkBootstrap implementations
│   ├── frameworks/          Framework registry (one FrameworkDefinition per framework)
│   ├── desktop/             Desktop app glue (Wails bindings)
│   ├── proxy/               Caddy/proxy route and TLS helpers
│   ├── ui/                  Styled terminal output (pterm)
│   └── updater/             Background update checking
├── docker/                  PHP Dockerfiles and build contexts
├── tests/                   Unit + integration tests
│   ├── fixtures/            Shared test fixtures
│   └── integration/         Integration test projects per framework
├── docs/                    Project documentation
└── scripts/                 Build helpers (macOS pkg, etc.)
```

---

## Extension Points

| Extension Point | How |
| :--- | :--- |
| Add a new framework | `internal/frameworks/<name>/` — see [Adding a New Framework](/developer/adding-a-framework) |
| Extend runtime selection | Profile/config engine |
| Add blueprint fragments | Compose template logic |
| Project-level extensions | `.govard/commands`, `.govard/hooks`, `.govard/docker-compose.override.yml` |

---

## Release Shape

| Artifact | Description |
| :--- | :--- |
| Platform archives | CLI binaries via GoReleaser (`.tar.gz` / `.zip`) |
| `govard_<version>_linux_<arch>.deb` | Linux installer with `govard` + `govard-desktop` |
| `govard_<version>_Darwin_<arch>.pkg` | macOS installer with `govard` + `govard-desktop` |
| `checksums.txt` | SHA-256 checksums for all artifacts |

`govard self-update` resolves the target release, downloads the platform artifact, verifies SHA-256, and atomically replaces installed binaries.

---

[Desktop App](/workflows/desktop-app) | [Contributing](/developer/contributing)
