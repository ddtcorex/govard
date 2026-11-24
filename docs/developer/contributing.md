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
| Go | `1.27.4+` | `go version` |
| Node.js | `20+` | `node --version` |
| Docker Engine + Compose | latest | `docker --version` |
| Wails | `v3.1.0`, a Go module: `go tool wails3 version` (desktop only) |
| golangci-lint | `v2.15+` | `golangci-lint --version` |

```bash
# Verify all tools
go version
node --version
go tool wails3 version
docker --version
golangci-lint --version
```

---

## Dependency holds

The Go stack tracks the newest stable release. Two modules stay below their
latest release because upstream makes the move impossible, and both are declared
in `internal/depsaudit/holds.json`:

| Module | Held at | Why the latest release cannot be used | Retire the hold when |
| :--- | :--- | :--- | :--- |
| `github.com/gobwas/glob` | `v0.2.3` | `v1.1.0` removed the `syntax/ast` and `syntax/lexer` packages that `goreleaser/fileglob`, reached through the `wails3` packager, still imports; forcing it fails the build with `no required module provides package github.com/gobwas/glob/syntax/ast` | `goreleaser/fileglob` ships a release on the new glob API, or `wails3` stops using `nfpm` |
| `github.com/wailsapp/task/v3` | `v3.40.1-patched3` | the advertised update `v3.40.1` declares its module path as `github.com/go-task/task/v3`, so it cannot be required under the fork path | wails republishes the fork under a matching path, or drops it |

`go run ./scripts/depsaudit` prints three buckets (held, upstream-owned, movable)
and its exit code is the contract: `0` means every hold still holds, `1` means a
declared hold no longer suppresses anything (re-read the table), and `2` means the
run could not be verified, either because the module proxy was unreachable or
because a module could not be classified. On `2` the holds were not evaluated and
must not be edited. It runs weekly inside the vulnerability workflow and by hand,
alongside the quarterly version drift grep.

The "upstream-owned" bucket is decided with the desktop build tags
(`desktop,production`) in force, because a dependency imported only by
desktop-tagged code must not read as unused.

To retry a hold, run the command in its row: a failure there is the proof the hold
is still needed, and success is the signal to drop it and bump. A module whose
version is owned by the module that requires it is reported, never pinned:
`go mod tidy` deletes a requirement nothing imports, so a pin there would only
look enforced. The second hold is retried with
`go get github.com/wailsapp/task/v3@v3.40.1`, which fails with `module declares
its path as: github.com/go-task/task/v3`.

## Repository Map

```
cmd/govard/main.go           CLI entrypoint
cmd/govard-desktop/          Desktop entry (built by Wails)
desktop/                     Wails 3 desktop app (Go backend + Vite frontend, React islands)
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
`app.go`, `package.json`, or `CHANGELOG.md` is needed for a
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
- **Neutral fixture names** — use `sample-project`, never a project-specific name from a developer machine
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
