# Govard Desktop

This folder hosts the Wails desktop app and lightweight frontend dashboard.

Contents:
- `frontend/` desktop UI built with Vite and served by Wails
- `wails.json` build configuration

## Development

Prerequisites: Go, Node.js 24+, pnpm, and the Wails v2 CLI.

```bash
make frontend                      # Vite build into desktop/frontend/dist
go run ./cmd/govard desktop --dev  # wails dev: Vite HMR + Go rebuild
```

If Wails is not installed, the command falls back to `go run -tags desktop ./cmd/govard-desktop`.

The frontend talks to Go only through `frontend/services/bridge.js` and
`frontend/services/events.js`; a Go test fails if any other file touches
`window.go` or `window.runtime`.

`govard-desktop` embeds `desktop/frontend/dist`, so every desktop build has to
run `make frontend` first. `dist/` is not committed; only `dist/.gitkeep` is, so
the package still compiles on a machine without Node.

## Build

```bash
make frontend                 # from the repo root
wails build -tags desktop     # from desktop/
govard desktop
```

`wails.json` wires:
- `frontend:install`: `pnpm install --frozen-lockfile`
- `frontend:build`: `pnpm build`
- `frontend:dev:watcher`: `pnpm dev`
- `frontend:dev:serverUrl`: `auto`

Lightweight dashboard highlights:
- Environment list with start/stop/open
- Project workspace layout (environments, quick actions, onboarding)
- Quick actions (PHPMyAdmin, Xdebug toggle, health)
- Log viewer with service selection and live streaming
- OS Terminal launcher for service shells
- Settings drawer (theme, proxy target, preferred browser)

Frontend file management:
- `frontend/main.js` bootstrap + wiring
- `frontend/services/bridge.js` Wails bridge wrappers
- `frontend/services/events.js` backend event subscriptions
- `frontend/state/store.js` local state
- `frontend/modules/*.js` feature modules (`dashboard`, `actions`, `logs`, `settings`)
- `frontend/ui/toast.js` notifications
- `frontend/utils/dom.js` DOM helpers
