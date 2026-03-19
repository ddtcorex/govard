# AGENTS.md

This document is the project-specific operating manual for AI coding agents working in `govard`.

## 1. Mission and Product Context

`Govard` is a Go-based local development orchestrator for PHP and web projects (Magento, Laravel, Symfony, WordPress, etc.), with:

- CLI orchestration (`govard ...`)
- Container runtime automation (Docker)
- Blueprint/template rendering
- Remote environment tooling
- SSL/proxy utilities
- Wails-based Desktop GUI Application (`govard desktop`)

Primary goals for contributions:

1. Preserve CLI stability and predictable behavior.
2. Keep workflows fast for local developers.
3. Avoid regressions in core command families (bootstrap/proxy/db/sync/remote/lock/domain/desktop).
4. Maintain UI reliability and backend bridge logic for the Desktop App.
5. Maintain release quality (GoReleaser, checksums, install paths).

## 2. Runtime and Toolchain Requirements

- Go: `1.25+` (module uses `go 1.25.0`)
- Node.js: `20` (for frontend UI and frontend tests)
- Wails: `v2.11+` (for desktop app development)
- Docker: required for integration tests and runtime orchestration
- GitHub CLI (`gh`): useful for release inspection (optional)

Local sanity checks:

```bash
go version
node --version
wails version
docker --version
```

## 3. Repository Map

- `cmd/govard/main.go`: CLI entrypoint
- `cmd/govard-desktop/`: Desktop app entrypoint (built by Wails)
- `desktop/`: Wails desktop app codebase (Go backend bridge + HTML/vanilla JS frontend in `desktop/frontend/`)
- `docs/`: Project documentation, architectural decisions, and AI agent test reports
- `internal/cmd/`: Cobra command implementations
- `internal/engine/`: orchestration, config, blueprint logic
- `internal/engine/bootstrap/`: framework bootstrap workflows
- `internal/engine/remote/`: remote sync/deploy/ssh helpers
- `internal/proxy/`: caddy/proxy route and TLS helpers
- `internal/updater/`: update-check notification logic
- `internal/ui/`: terminal rendering helpers
- `tests/`: unit/contract tests (default location for tests)
- `tests/integration/`: integration tests (tagged and heavier)
- `tests/frontend/`: Node test runner suite for frontend pieces
- `install.sh`: unified installer (release binary by default, `--source` for source build)
- `scripts/build-macos-pkg.sh`: helper script to build macOS `.pkg` installer artifacts
- `.goreleaser.yml`: release artifact config
- `.github/workflows/`: CI/release/security automation

## 4. Core Build and Test Commands

Preferred commands:

```bash
make test-fast           # frontend + unit tests
make test-unit           # unit tests only
make test-integration    # integration tests (requires build + docker)
make test                # full test suite
make vet                 # go vet
make fmt                 # go fmt ./...
make build               # build Govard for the current platform
```

Useful direct commands:

```bash
go test ./...
go test ./tests/... -v
go test -tags integration ./tests/integration/... -v -timeout 30m
```

## 5. CI and Quality Gates

Key workflows:

- `ci-pipeline.yml`: vet + gofmt check + fast tests + integration + binary build
- `release.yml`: triggered by tag `v*.*.*`, runs GoReleaser and uploads macOS `.pkg` installers
- `codeql.yml`: code scanning
- `govulncheck.yml`: weekly vulnerability scan

Do not assume local success if you skipped:

1. formatting (`gofmt -s -w` / `make fmt`)
2. tests relevant to changed surface
3. at least one command-level smoke check for CLI behavior changes

## 6. Testing Conventions (Important)

Project convention is to keep most tests in `tests/` package `tests`.

Test isolation and fixture hygiene rules:

- Keep tests hermetic: do not rely on the user's real projects, real containers, or machine-specific state.
- Do not use real/legacy project fixture names (for example `magento2-test-instance`); use neutral fixtures such as `sample-project`.
- Prefer mocks/fakes over live network dependencies in unit tests (for HTTP, inject client/transport and use a mock `RoundTripper`).
- Ensure test suites isolate state via temporary `GOVARD_HOME_DIR` (use `TestMain` setup where appropriate).
- If an integration test must touch external services, gate it with explicit env checks and clear skip reasons.

When you need to test internal logic from `internal/cmd` (or other internal packages):

1. keep production helpers unexported where possible
2. add narrow exported wrappers suffixed with `ForTest` (example: `BuildLocalDBResetScriptForTest`)
3. consume those wrappers from test files in `tests/`

Example pattern:

- production function: `buildThing(...)`
- test wrapper: `BuildThingForTest(...)`
- test location: `tests/thing_test.go`

Avoid broad export of internals just for tests.

## 7. CLI Architecture and Command Work

`internal/cmd/root.go` owns root registration and `Version` binding.

When adding/modifying a command:

1. define command in `internal/cmd/<area>.go`
2. register with `rootCmd.AddCommand(...)` (or relevant subcommand group)
3. ensure flags are explicit and help text is actionable
4. return errors with context (`fmt.Errorf("operation: %w", err)`)
5. add/adjust tests in `tests/`
6. update docs for user-visible command/flag changes

For update/release-sensitive commands:

- confirm asset naming matches `.goreleaser.yml`
- verify checksums where possible
- avoid assuming `sudo` availability

## 8. Installer and Update Expectations

### Release artifacts

Current release artifacts include:

- CLI archives from GoReleaser:
  - non-Windows: `govard_<version>_<OS>_<arch>.tar.gz`
  - Windows: `govard_<version>_<OS>_<arch>.zip`
- Linux installer (GoReleaser nfpm): `govard_<version>_linux_<arch>.deb`
  - includes both `govard` and `govard-desktop`
- macOS installer (release workflow): `govard_<version>_Darwin_<arch>.pkg`
  - includes both `govard` and `govard-desktop`
- Checksum file: `checksums.txt`

### Installer scripts

- `install.sh`: unified installer (download release binary by default, `--source` for source build)
- `scripts/build-macos-pkg.sh`: macOS package artifact builder used by release workflow

### Self-update behavior

`govard self-update` should:

1. resolve target version (explicit or latest)
2. resolve platform-specific artifact name
3. download archive and checksum
4. verify SHA-256
5. extract binary
6. atomically replace executable (or fail with clear permissions guidance)

## 9. Coding Standards

- Always run `gofmt` after Go edits.
- Keep code ASCII unless file already requires Unicode.
- Prefer small pure helpers for parsing/formatting logic.
- Keep platform branching explicit (`runtime.GOOS`, `runtime.GOARCH`).
- Do not swallow errors silently for critical flows (network, file, process).
- Preserve existing UX tone from `pterm` output and command help strings.

## 10. Dependency and Security Guidance

- Avoid adding dependencies unless required by measurable benefit.
- Prefer Go stdlib for HTTP, archive, hash, file operations.
- Never log secrets, tokens, private keys, or DB passwords.
- For remote/ssh/db commands, keep safe defaults and explicit opt-in for write ops.

## 11. Documentation Update Rules

Update `README.md` when changes affect:

1. installation
2. upgrade/update flow
3. command names/flags
4. release consumption

If behavior changed and docs are stale, treat as incomplete work.

## 12. Git and Change Hygiene

- Keep commits focused by concern (installer, command logic, tests, docs).
- Do not revert unrelated working-tree changes.
- Avoid destructive git operations unless explicitly requested.
- Include test evidence in PR/hand-off notes.

## 13. Recommended Agent Workflow

1. Read impacted files and nearby tests.
2. Identify minimal change set.
3. Implement with small helpers and clear errors.
4. Add/adjust tests in `tests/`.
5. Run formatting and targeted tests.
6. Run broader suite as needed (`make test-fast` or `go test ./...`).
7. Update README/docs if user-facing behavior changed.
8. Summarize file-level changes and verification evidence.

## 14. Pre-Completion Checklist

Before declaring done:

1. `go test` on affected scope passes.
2. No formatting drift (`gofmt -s -l .` should be empty for changed Go files).
3. Command help/flags still coherent.
4. Docs updated for user-visible changes.
5. `git status` reviewed for unintended file changes.

## 15. Known Project-Specific Notes

- CI tracks `main`, `master`, and `develop`.
- Default branch currently appears as `master` in this repo.
- Release tags follow semantic style `vX.Y.Z`.
- Integration tests rely on built binary (`bin/govard-test`) and Docker.

When uncertain, prefer compatibility and least-surprise behavior over broad refactors.

## 16. Desktop App Development & Testing

### Dev Mode (Live Backend)

To launch the desktop app with the real Go backend connecting to local Docker projects, run Wails dev mode. **Critically, in AI headless environments, provide a display server to prevent Wails from crashing:**

```bash
DISPLAY=:1 govard desktop --dev
```

This uses `wails dev -tags desktop` under the hood, compiling the backend and serving the frontend.

### Browser Access for UI Testing (Real Data)

When the dev server is successfully running (it may take 10-20 seconds to compile), it exposes the frontend at:

```
http://localhost:34115
```

This is the **mandatory approach for agents to test the real desktop UI**. 
- Navigate to `http://localhost:34115` in the browser tool.
- You will see live, actual projects from the user's Docker environment, not dummy templates.

| Path                                     | Purpose                                |
| :--------------------------------------- | :------------------------------------- |
| `desktop/frontend/index.html`            | Main HTML entry                        |
| `desktop/frontend/main.js`               | Bootstrap, event wiring, tab/state mgmt|
| `desktop/frontend/services/bridge.js`    | Wails Go backend RPC bridge            |
| `desktop/frontend/state/store.js`        | UI state (selected project, filters)   |
| `desktop/frontend/modules/`              | Feature modules (dashboard, logs, etc) |
| `desktop/frontend/ui/toast.js`           | Toast notification system              |
| `desktop/frontend/utils/dom.js`          | Shared DOM helpers                     |

### Test Mode Behavior

- When accessed **via Wails dev** (`localhost:34115`): full backend bridge is active, real project data is loaded.
- When opened **directly as a file** (no backend): the bridge is unavailable and fallback mock data (`safeDashboard` in `main.js`) is used. A warning toast appears.

### Test Reports

Desktop UI test plans and results should be stored under `docs/desktop/`.
