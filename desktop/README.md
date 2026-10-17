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
- Log viewer with service selection and live streaming
- OS Terminal launcher for service shells
- System tray: show/hide the window, start/stop/open projects, quit
- Settings drawer (theme, proxy target, preferred browser, run in background)
- Deploy tab: read-only status/releases/plan/check plus preview-gated rollback and sandbox rehearse

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

Frontend file management:
- `frontend/main.js` bootstrap + wiring
- `frontend/services/bridge.js` route table onto the generated bindings
- `frontend/services/events.js` backend event subscriptions
- `frontend/bindings/` generated and committed; never edit by hand
- `frontend/state/store.js` local state
- `frontend/modules/*.js` feature modules (`dashboard`, `actions`, `logs`, `settings`)
- `frontend/ui/toast.js` notifications
- `frontend/utils/dom.js` DOM helpers
