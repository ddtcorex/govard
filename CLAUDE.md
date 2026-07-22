# Govard

Go-based local development orchestrator for PHP and web projects (Magento, Laravel, Symfony, WordPress, etc.).

## Quick Reference

| Command | Purpose |
|---------|---------|
| `make test` | Full test suite (lint + fmt + vet + tests) |
| `make build` | Build CLI for current platform |
| `make test-integration` | Integration tests (requires Docker) |

**Runtime:** Go 1.25+, Node.js 20, Docker (for integration tests)

## Repository Map

```
cmd/govard/main.go              # CLI entrypoint
cmd/govard-desktop/             # Desktop app (Wails)
desktop/frontend/               # Desktop frontend (vanilla JS)
internal/cmd/                   # Cobra commands (70 files)
  bootstrap*.go                  # Bootstrap workflows
  config_*.go                    # Config management
  db*.go                         # Database commands
  doctor*.go                     # Diagnostics & fixes
  profile*.go                    # Profile detection/apply
  up*.go                         # Environment startup
internal/engine/                 # Core engine (20 files)
  config*.go                     # Config structs, normalize, persist
  compose*.go                    # Docker compose generation
  blueprint*.go                  # Blueprint rendering
  profile*.go                    # Runtime profiles
  lockfile.go                    # Lock file management
  migrate.go                     # DDEV/Warden migration
  doctor*.go                     # Diagnostics
internal/blueprints/             # Blueprint templates
internal/conventions/            # Constants, conventions
internal/desktop/               # Desktop backend
internal/proxy/                 # Caddy/proxy TLS
internal/ui/                    # Terminal rendering
internal/updater/               # Self-update
tests/                          # Unit tests
tests/integration/              # Integration tests
tests/frontend/                 # Frontend JS tests
docs/                           # Documentation (VitePress)
```

## Build & Test

```bash
make test                       # lint + fmt-check + vet + all tests
make test-unit                  # unit tests only
make test-integration           # integration tests (requires Docker)
make build                      # build CLI for current platform

# Direct commands
go test ./...                   # all unit tests
go test -tags integration ./...  # integration tests
go vet ./...                    # static analysis
gofmt -s -w .                   # format
```

## Code Standards

- Run `gofmt` after Go edits
- Keep code ASCII unless file already requires Unicode
- Prefer small pure helpers for parsing/formatting
- Do not swallow errors for critical flows (network, file, process)
- Never log secrets, tokens, private keys, or DB passwords

## Testing Conventions

- Keep tests hermetic: no real projects, containers, or machine-specific state
- Use neutral fixtures (e.g., `sample-project`), not legacy names like `magento2-test-instance`
- Prefer mocks over live network in unit tests
- Isolate state via `GOVARD_HOME_DIR` (use `TestMain` where appropriate)
- Gate external service tests with explicit env checks

**Test pattern for internal packages:**
```go
// Production: buildThing(...)
// Test wrapper: BuildThingForTest(...)
// Test location: tests/thing_test.go
```

## CLI Commands

`internal/cmd/root.go` owns root registration.

When adding/modifying commands:
1. Define in `internal/cmd/<area>.go`
2. Register with `rootCmd.AddCommand(...)`
3. Ensure flags are explicit, help text is actionable
4. Return errors with context (`fmt.Errorf("operation: %w", err)`)
5. Add tests in `tests/`
6. Update docs for user-visible changes

## Blueprint Versioning

`internal/engine/render.go`'s `BlueprintVersion` const forces existing projects to re-render (`govard env up`) by invalidating a stored content hash.

- Editing files under `internal/blueprints/files/**` (base.yml, framework yml, nginx templates) already busts that hash automatically via content fingerprinting — **no bump needed**.
- Bump `BlueprintVersion` only when Go rendering logic changes (`render.go`, `config_normalize.go`, `framework_config.go`, `profile.go`, etc.) in a way that changes rendered output *without* changing blueprint file bytes — those changes aren't hash-detected.
- When bumped, note it in `CHANGELOG.md` under a "Blueprint Lifecycle" bullet (see prior entries for wording).

## Release Checklist

Update version in:
1. `internal/cmd/root.go` (`var Version`)
2. `internal/desktop/app.go` (`var Version`)
3. `desktop/frontend/package.json` (`"version"`)
4. `desktop/wails.json` (`"info": { "productVersion" }`)
5. `CHANGELOG.md` (add new version section)

**Verification:** `make test && make build && ./bin/govard version`

## Desktop App Development

**Dev mode (live backend):**
```bash
DISPLAY=:1 govard desktop --dev
```
Compiles backend and serves frontend at `http://localhost:34115`

**Testing UI:** Navigate to `http://localhost:34115` to see real projects from Docker

| Path | Purpose |
|------|---------|
| `desktop/frontend/index.html` | Main HTML entry |
| `desktop/frontend/main.js` | Bootstrap, event wiring |
| `desktop/frontend/services/bridge.js` | Wails Go backend RPC |

- Via Wails dev: full backend, real project data
- Direct file open: mock data, bridge unavailable

## Project-Specific Notes

- CI tracks `main`, `master`, `develop`; default is `master`
- Release tags: `vX.Y.Z`
- Integration tests require built binary (`bin/govard-test`) and Docker
- When uncertain, prefer compatibility over broad refactors

## Documentation

Update `README.md` for: installation, upgrade flow, command/flag changes, release consumption

Update `docs/*.md` for: command names/aliases/flags, config behavior, remote/sync/db workflows, framework support, desktop behavior. `docs/**/*.md` auto-syncs to the GitHub Wiki on every push to `master` (`.github/workflows/sync-wiki.yml`) — no separate wiki edit needed.

**Treat stale docs as incomplete work.**

## Git Workflow

- Always start a new feature branch for each work session — never commit directly to `master`/`develop`.
- When development on a feature branch is complete (tests passing, ready for review), proactively create a GitHub issue with full details (problem/motivation, scope, what changed) and a GitHub PR with full details (summary, rationale, test plan) that links back to that issue (e.g. `Closes #<issue>` in the PR body) — don't wait to be asked.

## Superpowers Workflow Preferences

- `docs/superpowers/**` (specs, plans) are local-only working artifacts — gitignored (see `.gitignore`). Never `git add`/commit them; keep them on disk for reference within the session.
- When a task executed via superpowers (subagent-driven-development, executing-plans, or any multi-commit implementation) is complete, proactively squash all commits made for that task into a single commit before considering the work done — don't leave the per-step/per-task commit history in place unless the user asks to keep it granular.

## Pre-Completion Checklist

1. `go test` on affected scope passes
2. `gofmt -s -l .` shows no drift on changed files
3. Command help/flags still coherent
4. `README.md` and relevant `docs/*.md` updated for user-visible changes
5. `git status` reviewed for unintended file changes