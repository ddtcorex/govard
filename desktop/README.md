# Govard Desktop

This folder hosts the Wails desktop app and lightweight frontend dashboard.

Contents:
- `frontend/` desktop UI served by Wails
- `wails.json` build configuration

Quick start (dev):
1. Install Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
2. Run `govard desktop --dev` from repo root

If Wails is not installed, the command falls back to `go run -tags desktop ./cmd/govard-desktop`.

Quick start (build):
1. `wails build -tags desktop` (from `desktop/`)
2. `govard desktop`

`wails.json` currently wires:
- `frontend:install`: `yarn install`
- `frontend:build`: `yarn run build:css`
- `frontend:dev`: empty (frontend is served through Wails dev runtime)

Lightweight dashboard highlights:
- Environment list with start/stop/open
- Project workspace layout (environments, quick actions, onboarding)
- Quick actions (PHPMyAdmin, Xdebug toggle, health)
- Log viewer with service selection and live streaming
- Shell launcher with persisted user + shell override
- Settings drawer (theme, proxy target, preferred browser)

Frontend file management:
- `frontend/main.js` bootstrap + wiring
- `frontend/services/bridge.js` Wails bridge wrappers
- `frontend/state/store.js` local state
- `frontend/modules/*.js` feature modules (`dashboard`, `actions`, `logs`, `shell`, `settings`)
- `frontend/ui/toast.js` notifications
- `frontend/utils/dom.js` DOM helpers
