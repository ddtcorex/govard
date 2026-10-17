# Govard Desktop

This folder hosts the desktop frontend and the assets its Go binary embeds.
The Wails v3 runtime lives in Go, under `internal/desktop`; there is no Wails
CLI step and no `wails.json` any more.

Contents:
- `frontend/` desktop UI built with Vite and embedded by the Go binary

## Development

Prerequisites: Go, Node.js 24+, pnpm, and on Linux `libgtk-4-dev` plus
`libwebkitgtk-6.0-dev` (the runtime needs `libgtk-4-1` and
`libwebkitgtk-6.0-4`).

```bash
make frontend                      # Vite build into desktop/frontend/dist
make bindings                      # regenerate the JS bindings from the Go services
go run ./cmd/govard desktop --dev  # Vite HMR plus a Go rebuild against it
```

`govard desktop --dev` starts Vite on `http://localhost:5173` and runs the app
with `FRONTEND_DEVSERVER_URL` pointing at it; Wails proxies the dev server only
in a build without the `production` tag.

The frontend talks to Go only through `frontend/services/bridge.js` and
subscribes to events only through `frontend/services/events.js`; a Go test fails
if any other file touches the runtime or the generated bindings.

`govard-desktop` embeds `desktop/frontend/dist`, so every desktop build has to
run `make frontend` first. `dist/` is not committed; only `dist/.gitkeep` is, so
the package still compiles on a machine without Node.

## Build

```bash
make frontend                                    # from the repo root
go build -tags desktop,production -o bin/govard-desktop ./cmd/govard-desktop
govard desktop
```

The release build tags are `desktop,production`. `production` is what switches
off the dev asset proxy and the devtools, and no WebKit version tag is needed:
linking is decided by pkg-config.

Bindings:
- `make bindings` regenerates `frontend/bindings/**` from the Go services. The
  `-f "-tags desktop"` flag is mandatory; without it the generator finds no
  services and still exits 0.
- `make bindings-check` fails when the committed bindings no longer match the Go
  services, and CI runs it.

Lightweight dashboard highlights:
- Environment list with start/stop/open
- Project workspace layout (environments, quick actions, onboarding)
- Quick actions (PHPMyAdmin, Xdebug toggle, health)
- Data tab (masked DB info, guarded queries, dump/import, snapshots)
- Log viewer with service selection and live streaming
- Tool Runner palette: preset framework commands with streamed output
- OS Terminal launcher for service shells
- System tray: show/hide the window, start/stop/open projects, quit
- Settings drawer (theme, proxy target, preferred browser, run in background)
- Deploy tab: read-only status/releases/plan/check plus preview-gated rollback and sandbox rehearse
- Operation Center: running ops with cancel plus terminal history with
  failed-only retry (`OperationService` via the `ListOps`, `ListHistory`,
  `CancelOp`, `RetryOp` bridge routes; one sync per project; history capped
  at 200 records with restart recovery; at-least-once
  `operations:notification` events). Full behavior is documented in
  `docs/workflows/desktop-app.md` under Operation Center.
- Health: per-project doctor and audit banners plus Doctor/Audit panels below
  the mesh snapshot. Doctor fixes are allowlisted (one-click `Fix` for safe
  checks, `Preview fix` plus confirm for risky ones, no button otherwise);
  audits run the container-free integrity check only, with the Docker-backed
  lint/profiler rows shown disabled. Full table and banner meanings live in
  `docs/workflows/desktop-app.md` under "Health: Doctor and Audit".

## Deploy tab: CLI mapping and contracts

M1 is read-only: Status maps to `govard deploy status [remote]`, Releases to
`govard deploy releases [remote]`, Plan to `govard deploy plan [remote]`, and
Check to `govard deploy check [remote]`. One disclosed exception: on a
symlink-strategy target the check performs a transient probe write to verify
atomic swaps, then removes it, and the panel shows a note whenever that probe
ran.

M2 mutates through one contract: preview plus explicit confirm. Rollback
preview/apply maps to `govard deploy rollback [remote]`, sandbox
start preview/apply to `govard sandbox up`, sandbox removal preview/apply to
`govard sandbox down`, and connection info plus the terminal shell to
`govard sandbox ssh`. The preview shows exactly what the apply would do and
mints a single-use token (15-minute TTL) bound to that action and parameters;
the apply spends the token and requires a ticked checkbox confirm, so no
single click changes target state. Stale previews (a deploy landed in
between, or the sandbox container changed) are refused and need a fresh
preview. Rollback with `--with-db` and sandbox seeding stay CLI-only.

A cancelled run reports what the engine left behind (release record restore,
maintenance window state, whether a release was activated, the re-read
sandbox container state) and renders as cancelled, never as a failure.

The Data tab is read-only by default: ad-hoc queries run only
`SELECT`-style statements unless the write opt-in plus its per-project
confirm token (`DBService.GetDBWriteConfirmToken`) travels with the call,
snapshot restore needs a preview token plus explicit confirm
(`SnapshotService.PreviewSnapshotRestore` then `ApplySnapshotRestore`),
and cancel stops desktop tracking while a started restore may run to
completion. Full behavior, scrubbing guarantees, and disk preflight are
documented under Data Tab in `docs/workflows/desktop-app.md`.

Frontend file management:
- `frontend/main.js` bootstrap + wiring
- `frontend/services/bridge.js` route table onto the generated bindings
- `frontend/services/events.js` backend event subscriptions
- `frontend/bindings/` generated and committed; never edit by hand
- `frontend/state/store.js` local state
- `frontend/modules/*.js` feature modules (`dashboard`, `actions`, `logs`, `settings`, `health`, `tool-palette`, `data-tab`, `deploy`)
- `frontend/ui/toast.js` notifications
- `frontend/utils/dom.js` DOM helpers

## Health UX (doctor + integrity audit)

User documentation lives in `docs/workflows/desktop-app.md` ("Health: Doctor
and Integrity Audit", with a Vietnamese counterpart under `docs/vi/`).

Implementation map:

- Backend: `internal/desktop/doctor_service.go` (fix allowlist, preview
  tokens), `internal/desktop/audit_service.go` (docker-free integrity run,
  disabled `lint`/`profiler` rows), `internal/desktop/health_service.go`
  (`HealthService`, project scoping). The allowlist in `doctor_service.go` is
  the source of truth for which check IDs get `Fix`, `Preview fix`, or no
  button; the docs table must match it exactly.
- Frontend: `frontend/modules/health.js` (banner derivations, per-project
  scoping, ephemeral preview-token controller state),
  `frontend/islands/GlobalHealthHeader.tsx` (banners) and
  `frontend/islands/HealthPanels.tsx` (panels).
- Routes (`bridge.js` onto `HealthService`): `RunDoctor`,
  `PreviewDoctorFix`, `ApplyDoctorFix`, `CancelDoctorRun`,
  `RunIntegrityAudit`, `GetAuditResult`, `GetAuditDiff`, `LastAuditResult`,
  `CancelAudit`, `RunningAuditOp`. Progress streams on `audit:progress`
  (see `internal/desktop/events.go`).
- `frontend/islands/ToolPalette.tsx` Tool Runner palette island
- `frontend/islands/DataTab.tsx` Data tab island
- `frontend/ui/toast.js` notifications
- `frontend/utils/dom.js` DOM helpers

## Tool Runner

The Tool Runner palette (`frontend/islands/ToolPalette.tsx`, helpers in
`frontend/modules/tool-palette.js`) runs framework tool commands from a
backend-owned preset allowlist in `internal/desktop/tool_runner.go`. The
palette sends a preset ID plus an args map only; the exec argv stays
backend-owned and runs shell-free via `exec.CommandContext` (no shell, no
`sh -c`, no string joining). Unknown preset IDs are refused before any
process exists, and args are validated against the preset's `ArgSpec`
(unknown names, missing required args, values over 256 characters, and
values outside the arg pattern are rejected).

Presets: `magento-cache-flush`, `magento-cache-clean`,
`magento-setup-di-compile`, `magento-indexer-reindex` (Magento family);
`laravel-migrate`, `laravel-cache-clear`, `laravel-config-clear` (Laravel);
`composer-install`, `npm-install`, `npm-run-build` (all projects). Slow
presets get the 15-minute ceiling, fast cache clears keep 2 minutes.

One tool run per project at a time; `CancelToolRun` kills the process through
the op registry and frees the lock. Terminal events (`tool:completed`,
`tool:failed`) carry the run generation (`OpID`, `Gen`) and apply only when
current, so a stale receipt never marks the current run. Per-project history
(50 runs, JSONL under `tool-history/` in the Govard home) scrubs secrets on
write (secret key names, `KEY=value` pairs, credentialed URLs, PEM blocks);
scrubbed `***` values are not re-applied by Recent runs. Full details live in
`docs/workflows/desktop-app.md`.
