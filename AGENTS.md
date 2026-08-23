# Govard

Go-based local development orchestrator for PHP and web projects (Magento, Laravel, Symfony, WordPress, etc.).

`CLAUDE.md` at the repo root is a symlink to `AGENTS.md`, so Claude Code follows the same rules as Codex CLI. Edit `AGENTS.md` only — never edit `CLAUDE.md` directly or replace the symlink with a copy.

## Quick Reference

| Command | Purpose |
|---------|---------|
| `make test` | Full test suite (lint + fmt + vet + tests) |
| `make build` | Build CLI for current platform |
| `make test-integration` | Integration tests (requires Docker) |

**Runtime:** Go 1.25+, Node.js 20, Docker (for integration tests)

## Workflow: Superpowers skills are mandatory

Multi-step changes MUST follow the Superpowers skill workflow, in order:
brainstorming (design recorded locally under `docs/superpowers/specs/`),
writing-plans (`docs/superpowers/plans/`), then executing-plans with strict TDD.
Per existing repo policy these artifacts stay **local-only and gitignored**;
describe durable outcomes in the PR body instead of committing them. When a
superpowers-executed task completes, squash its per-task commits into one
commit before considering the work done.

## Repository Map

```
cmd/govard/main.go              # CLI entrypoint
cmd/govard-desktop/             # Desktop app (Wails)
desktop/frontend/               # Desktop frontend (vanilla JS)
internal/cmd/                   # Cobra commands
  bootstrap*.go                  # Bootstrap workflows
  config_*.go                    # Config management
  db*.go                         # Database commands
  doctor*.go                     # Diagnostics & fixes
  profile*.go                    # Profile detection/apply
  up*.go                         # Environment startup
internal/engine/                 # Core engine (framework-agnostic dispatch + registries)
  config*.go                     # Config structs, normalize, persist
  compose*.go                    # Docker compose generation
  blueprint*.go                  # Blueprint rendering
  profile*.go                    # Runtime profiles
  lockfile.go                    # Lock file management
  migrate.go                     # DDEV/Warden migration
  doctor*.go                     # Diagnostics
internal/frameworks/<name>/     # Per-framework definition, bootstrap, and blueprint assets
internal/blueprints/             # Shared blueprint templates (assets specific to one framework live in that framework's own package instead)
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
- `internal/conventions/` holds only constants used by ≥2 packages (or genuinely cross-cutting ones — admin/path/permission/network defaults). A constant read by exactly one framework package (DB credential defaults, tool binary paths, etc.) belongs in that framework's own package (`internal/frameworks/<name>/`), not in `conventions` — verify with `grep -rl "conventions\.<Name>"` before adding a new framework-specific constant there.
- This isolation applies beyond constants: `internal/engine`/`internal/cmd` must not branch on hardcoded framework names (`cfg.Framework == "magento2"`) — expose the behavior as a framework-registered capability instead (see `internal/engine/framework_capabilities.go`, `RegisterFrameworkCapabilities`) and let `internal/frameworks/<name>/` opt in at registration. `grep -rn '"magento2"\|"mageos"' internal/engine internal/cmd` before adding a new special case.

## Testing Conventions

- Keep tests hermetic: no real projects, containers, or machine-specific state
- Use neutral fixtures (e.g., `sample-project`), not legacy names like `magento2-test-instance`
- Prefer mocks over live network in unit tests
- Isolate state via `GOVARD_HOME_DIR` (use `TestMain` where appropriate)
- Gate external service tests with explicit env checks
- A framework's test functions for a given subject live in `tests/<subject>_<framework>_test.go` (e.g. `bootstrap_dagster_test.go`, `table_prefix_prestashop_test.go`) — never inside a shared/grab-bag file alongside other frameworks' tests. A test that genuinely compares/depends on ≥2 specific frameworks (priority ordering, package aliasing) stays in the framework-generic `<subject>_test.go`, written table-driven with framework names only in test data/`t.Run` labels, never in the Go function name.

**Test pattern for internal packages:**
```go
// Production: buildThing(...)
// Test wrapper: BuildThingForTest(...)
// Test location: tests/thing_test.go
```

**`package main` tools (e.g. `go:generate` helpers under `internal/**/gen/`):** this pattern cannot apply directly — Go does not allow importing a `package main` from another package (`go test` fails with "is a program, not an importable package"). Putting `_test.go` files alongside a `main.go` and testing unexported functions in-package is the wrong fix; it violates the `tests/` convention above and has already shipped once by mistake. Instead, extract the testable logic into a small importable library package (e.g. `internal/frameworks/gen/generator/`) with exported functions, leave `main.go` as a thin wrapper that just calls into it, and test the library package normally under `tests/`.

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

- Editing files under `internal/blueprints/files/**` (shared base.yml, generic nginx templates) or any `internal/frameworks/<name>/blueprint/**` (a framework's own compose fragment, nginx template, etc. — embedded via that package's `embed.go` and grafted into the merged `blueprints.FS` at init time) already busts that hash automatically via content fingerprinting — **no bump needed**.
- Bump `BlueprintVersion` only when Go rendering logic changes (`render.go`, `config_normalize.go`, `framework_config.go`, `profile.go`, etc.) in a way that changes rendered output *without* changing blueprint file bytes — those changes aren't hash-detected.
- When bumped, note it in `CHANGELOG.md` under a "Blueprint Lifecycle" bullet (see prior entries for wording).

## Release Checklist

`CHANGELOG.md` changes belong only to the release commit below — never add/edit `CHANGELOG.md` on a feature branch, even for a Blueprint Version bump; describe the change in the PR body instead.

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
- Release tags: `vX.Y.Z`; beta releases use `vX.Y.Z-beta.N` (e.g. `v1.60.0-beta.1`), tagged directly from HEAD — skip the Release Checklist's version-bump/CHANGELOG steps for a beta cut. `.goreleaser.yml`'s `prerelease: auto` marks the resulting GitHub Release as a prerelease automatically, so it's excluded from `GET /releases/latest` and won't reach stable-channel users; opt in via `govard self-update --channel beta`.
- Integration tests require built binary (`bin/govard-test`) and Docker
- When uncertain, prefer compatibility over broad refactors

## Documentation

Update `README.md` for: installation, upgrade flow, command/flag changes, release consumption

Update `docs/*.md` for: command names/aliases/flags, config behavior, remote/sync/db workflows, framework support, desktop behavior. `docs/**/*.md` auto-syncs to the GitHub Wiki on every push to `master` (`.github/workflows/sync-wiki.yml`) — no separate wiki edit needed.

**Treat stale docs as incomplete work.**

## Git Workflow

- Always start a new feature branch for each work session — never commit directly to `master`/`develop`.
- When development on a feature branch is complete (tests passing, ready for review), proactively create a GitHub issue with full details (problem/motivation, scope, what changed) and a GitHub PR with full details (summary, rationale, test plan) that links back to that issue (e.g. `Closes #<issue>` in the PR body) — don't wait to be asked.
- After every squash/force-push to an existing PR branch, re-read the PR description and linked issue (`gh pr view`, `gh issue view`) and update them if the shipped diff no longer matches — a squash easily leaves stale Summary/Validation bullets behind.

## Superpowers Workflow Preferences

- `docs/superpowers/**` (specs, plans) are local-only working artifacts — gitignored (see `.gitignore`). Never `git add`/commit them; keep them on disk for reference within the session.
- When a task executed via superpowers (subagent-driven-development, executing-plans, or any multi-commit implementation) is complete, proactively squash all commits made for that task into a single commit before considering the work done — don't leave the per-step/per-task commit history in place unless the user asks to keep it granular.

## Pre-Completion Checklist

1. Full `make test` passes (lint + fmt + vet + unit + integration) — not just `go test` on the changed package. A failure found here is presumed in scope for this branch until proven otherwise (e.g. also reproducible fresh on `master`) — don't dismiss it as "unrelated."
2. `gofmt -s -l .` shows no drift on changed files
3. Command help/flags still coherent
4. `README.md` and relevant `docs/*.md` updated for user-visible changes
5. `git status` reviewed for unintended file changes