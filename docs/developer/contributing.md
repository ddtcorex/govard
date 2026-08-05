---
title: Contributing to Govard
description: How to set up a development environment, follow code standards, and submit changes to the Govard project.
---

# Contributing

This guide covers the expected workflow for contributors working on Govard.

## Adding a new documentation page

`sitemap.xml`, canonical links, hreflang, and Open Graph tags are all generated automatically at build time from `docs/.vitepress/seo.ts` — never hand-edit `sitemap.xml`. Every new page under `docs/` must get a unique `title` and `description` in its frontmatter; pages without one fall back to the generic site description, which hurts search rankings.

---

## Toolchain Requirements

| Tool | Required Version | Check |
| :--- | :--- | :--- |
| Go | `1.25+` | `go version` |
| Node.js | `20+` | `node --version` |
| Docker Engine + Compose | latest | `docker --version` |
| Wails | `v2.11+` (desktop only) | `wails version` |
| golangci-lint | `v2.11+` | `golangci-lint --version` |

```bash
# Verify all tools
go version
node --version
wails version
docker --version
golangci-lint --version
```

---

## Repository Map

```
cmd/govard/main.go           CLI entrypoint
cmd/govard-desktop/          Desktop entry (built by Wails)
desktop/                     Wails desktop app (Go backend + vanilla JS frontend)
internal/cmd/                Cobra command implementations
internal/engine/             Orchestration, config, blueprint logic
internal/engine/bootstrap/   Framework bootstrap workflows
internal/engine/remote/      Remote sync/deploy/SSH helpers
internal/proxy/              Caddy/proxy route and TLS helpers
internal/updater/            Update-check notification logic
internal/ui/                 Terminal rendering helpers
tests/                       Unit/contract tests
tests/integration/           Integration tests (tagged and heavier)
tests/frontend/              Node test runner suite for frontend
install.sh                   Unified installer
scripts/                     Build helpers (macOS pkg, etc.)
.goreleaser.yml              Release artifact config
.github/workflows/           CI/release/security automation
```

---

## Build Commands

```bash
make build           # Build Govard for current platform
make install         # Build + install to system PATH
make install-release # Install release binary

# Direct build
go build -o govard cmd/govard/main.go
```

---

## Cutting a Beta Release

Beta releases use the tag format `vX.Y.Z-beta.N` (e.g. `v1.60.0-beta.1`),
tagged directly from the current commit — no version bump in `root.go`,
`app.go`, `package.json`, `wails.json`, or `CHANGELOG.md` is needed for a
beta. Pushing the tag triggers the same `.github/workflows/release.yml`
pipeline as a stable release; `.goreleaser.yml`'s `prerelease: auto` marks
the resulting GitHub Release as a prerelease, so it won't appear via
`GET /releases/latest` and won't reach stable-channel users automatically.
Users opt in with `govard self-update --channel beta`.

---

## Test Commands

### Preferred Makefile Commands

```bash
make test            # Full suite: lint + fmt-check + vet + frontend + unit + integration
make test-unit       # Go unit tests only
make test-integration # Integration tests (requires Docker)
make vet             # go vet
make fmt             # gofmt ./...
```

### CI-Equivalent Commands

```bash
make lint            # golangci-lint (matches CI version)
make fmt-check       # Check if any files need gofmt -s
make vet             # go vet
make test-unit       # Go unit tests
make test-frontend   # Node.js frontend tests
make test-integration-ci # Integration tests in parallel (CI behavior)
```

### Direct Commands

```bash
go test ./...
go test ./tests/... -v
go test -tags integration ./tests/integration/... -v -timeout 30m
```

---

## Desktop Development

Run Wails dev mode through the CLI wrapper:

```bash
DISPLAY=:1 govard desktop --dev
```

For browser-based UI testing, navigate to:

```
http://localhost:34115
```

This loads the real backend bridge with live project data.

---

## Test Layout and Conventions

| Path | Purpose |
| :--- | :--- |
| `tests/` | Package `tests` — most unit tests |
| `tests/fixtures/` | Shared fixture files |
| `tests/integration/` | Integration test projects per framework |
| `tests/frontend/` | Node test runner suite |

### Test Hygiene Rules

- **Keep tests hermetic** — no user-local projects, no real container state
- **Neutral fixture names** — use `sample-project`, not `magento2-test-instance`
- **Mock over live network** — inject HTTP transport mocks; use fake `RoundTripper`
- **Isolate `GOVARD_HOME_DIR`** — tests that need runtime state use `TestMain` setup
- **Gate external services** — explicit env checks and skip reasons for tests touching real services

### Exporting Internals for Tests

When a test needs access to internal logic from `internal/cmd`:

1. Keep production helpers unexported where possible
2. Add narrow exported wrappers suffixed with `ForTest`
3. Consume those wrappers from `tests/` package

```go
// internal/cmd/thing.go
func buildThing(...) { ... }  // unexported

// ForTest export (only add when tests need it)
func BuildThingForTest(...) { return buildThing(...) }

// tests/thing_test.go
result := cmd.BuildThingForTest(...)
```

---

## CLI Architecture

When adding or modifying a command:

1. Define the command in `internal/cmd/<area>.go`
2. Register with `rootCmd.AddCommand(...)` (or relevant subcommand group)
3. Ensure flags are explicit with actionable help text
4. Return errors with context: `fmt.Errorf("operation: %w", err)`
5. Add/adjust tests in `tests/`
6. Update docs for user-visible command/flag changes

---

## Coding Standards

| Rule | Notes |
| :--- | :--- |
| **`gofmt` after every Go edit** | `gofmt -s -w` on changed files |
| **ASCII-only** | Unless file already requires Unicode |
| **Small pure helpers** | For parsing/formatting logic |
| **Explicit platform branching** | Use `runtime.GOOS`, `runtime.GOARCH` |
| **No swallowed errors** | Critical flows (network, file, process) must surface errors |
| **Preserve pterm UX tone** | Match existing output style and help strings |

---

## Security Guidelines

- No new dependencies without measurable benefit — prefer Go stdlib
- Never log secrets, tokens, private keys, or DB passwords
- Remote/SSH/DB commands: safe defaults, explicit opt-in for write ops

---

## Contribution Rules

Before declaring work done:

1. Run tests relevant to the changed area
2. Run `gofmt -s -l .` — output should be empty for changed Go files
3. Update canonical `docs/*.md` files when behavior changes
4. Check `git status` for unintended file changes
5. Ensure command help/flags remain coherent

---

## CI Quality Gates

| Job | Command | Description |
| :--- | :--- | :--- |
| Quality Checks | `make lint fmt-check vet` | golangci-lint + gofmt + go vet |
| Full Tests | `make test` | Lint + format + vet + frontend + unit tests |
| Integration Tests | `make test-integration` | Builds binary + runs Docker tests |
| Build Binaries | `make build` | Verifies compilation |

CI tracks `main`, `master`, and `develop`. Default branch is `master`.

---

## Documentation Update Rules

Update `README.md` when changes affect:
- Installation or upgrade flow
- Command names or flags
- Release consumption

Update canonical `docs/*.md` when changes affect:
- Command names, aliases, or flags
- Configuration behavior or layering
- Remote/sync/DB workflows
- Framework support or runtime defaults
- Desktop behavior or testing workflow

**If behavior changed and docs are stale → treat as incomplete work.**

---

## Pre-Completion Checklist

- [ ] `go test` on affected scope passes
- [ ] `gofmt -s -l .` is empty for changed Go files
- [ ] Command help/flags still coherent
- [ ] `README.md` updated if needed
- [ ] Relevant `docs/*.md` updated
- [ ] `git status` reviewed for unintended changes

---

[Architecture](/developer/architecture) | [FAQ](/more/faq)
