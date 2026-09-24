---
title: Govard Desktop App
description: Govard Desktop is a Wails-based GUI sharing the same core engine as the CLI, with live logs, quick actions, and a project dashboard.
---

# Desktop App

Govard Desktop is the Wails-based GUI that reuses the same core engine as the CLI.

---

## Launch Modes

```bash
govard desktop              # Launch the built desktop binary
govard desktop --dev        # Run Wails dev mode (live backend)
govard desktop --background # Start hidden, reuse running instance on relaunch
```

| Mode | Description |
| :--- | :--- |
| `govard desktop` | Standard launch — uses built binary |
| `govard desktop --dev` | Dev mode — live Go backend, hot reload frontend |
| `govard desktop --background` | Background process — keeps alive when window closes |

---

## Current Surface

The desktop focuses on operational essentials:

| Feature | Description |
| :--- | :--- |
| **Environment Dashboard** | Start/stop/open/delete — including orphaned Docker project detection |
| **Project Workspace** | Environments list, quick actions, onboarding flow |
| **Quick Actions** | PHPMyAdmin, Xdebug toggle, health check, Mailpit, DB client |
| **Remotes Tab** | Add/test/open/sync-plan workflows for remote environments |
| **Resource Monitor** | CPU, RAM, network, OOM hints |
| **Logs** | Multi-service selection, severity filtering, text search, live streaming |
| **Shell Launcher** | Service, user, and shell selection |
| **Native Notifications** | Operation success/failure alerts |
| **Settings Drawer** | Theme, proxy target, preferred browser, database client |

> Desktop environment start/stop/pull and global-services operations call the Govard CLI command surface (`govard up`, `govard env ...`, `govard svc ...`), keeping desktop behavior aligned with all CLI updates.

---

## Keyboard Shortcuts

| Shortcut | Action |
| :--- | :--- |
| `Ctrl+,` / `Cmd+,` | Open Settings |
| `Esc` | Close Settings |

---

## Desktop Remote Actions

| Action | Behavior |
| :--- | :--- |
| Open Database (Remote) | Calls `govard open db -e <remote> --client` |
| Open SSH (Remote) | Prefers native Linux terminal launchers, falls back to `ssh://` |
| Open SFTP (Remote) | Prefers FileZilla, falls back to `sftp://` |

For `auth.method: ssh-agent`, Desktop reuses `SSH_AUTH_SOCK` and also probes `/run/user/<uid>/keyring/ssh` on Linux.

### Local Database Open

- Resolves published Docker host and port first
- Falls back to PHPMyAdmin if the configured DB client fails

---

## Desktop Preferences

Preferences are stored in:

```
~/.govard/desktop-preferences.json
```

Current persisted preferences:
- Theme (light/dark)
- Proxy target
- Preferred browser
- Database client preference

---

## Dev Mode

Prerequisites: Go, Node.js 24+, pnpm and the Wails v2 CLI. The desktop UI is
bundled by Vite into `desktop/frontend/dist`, which the Go binary embeds.

```bash
make frontend                      # Vite build into desktop/frontend/dist
DISPLAY=:1 govard desktop --dev    # wails dev: Vite HMR + Go backend rebuild
```

`make frontend` is not optional: a desktop build that skips it embeds an empty
`dist/` and shows a blank window. Every automated desktop build path (CI,
goreleaser, `scripts/build-macos-pkg.sh`, `install.sh --source`) runs it first.

The Vite dev server listens on `http://localhost:5173`; Wails dev mode proxies it
and also exposes the compiled backend at:

```
http://localhost:34115
```

`http://localhost:34115` is the preferred browser-testing path because the Go
backend bridge stays live and loads real project data. Opening the Vite server or
`dist/` directly renders the same shell with mock data and a "Desktop bridge not
available" notice, which is enough for styling and layout work.

---

## Frontend Layout

| File | Purpose |
| :--- | :--- |
| `desktop/frontend/index.html` | Main HTML entry |
| `desktop/frontend/main.js` | Bootstrap, event wiring, tab/state management |
| `desktop/frontend/services/bridge.js` | Wails Go backend RPC bridge; the only module allowed to call Go |
| `desktop/frontend/services/events.js` | Backend event subscriptions; the only module allowed to use `window.runtime` |
| `desktop/frontend/types/wails-v2.d.ts` | Declared shape of the Wails v2 globals |
| `desktop/frontend/state/store.js` | Shared UI state (selected project, filters) |
| `desktop/frontend/modules/` | Feature modules (dashboard, logs, remotes, etc.) |
| `desktop/frontend/ui/toast.js` | Toast notification system |
| `desktop/frontend/utils/dom.js` | Shared DOM helpers |

### Test Mode Behavior

| Access Method | Backend | Data |
| :--- | :--- | :--- |
| Wails dev (`localhost:34115`) | Full backend bridge active | Real project data |
| Vite dev (`localhost:5173`) or `dist/` | Bridge unavailable | Mock fallback data + warning toast |

A Go test (`tests/desktop_frontend_bridge_guard_test.go`) fails if any frontend
file other than `services/bridge.js`, `services/events.js` and
`types/wails-v2.d.ts` touches `window.go`, `window.runtime` or
`desktopBridge.runtime`. Both modules are JSDoc-typed with `// @ts-check`, so
`pnpm typecheck` catches a Go/JS contract mismatch at build time.

---

## Architecture Notes

The desktop app is intentionally focused on operational workflows:

- Desktop entrypoint: `cmd/govard-desktop`
- Wails bindings: `internal/desktop`
- Frontend shell: `desktop/frontend/index.html`
- Bootstrap/events: `desktop/frontend/main.js`
- Backend bridge: `desktop/frontend/services/bridge.js`
- State: `desktop/frontend/state/store.js`
- Feature modules: `desktop/frontend/modules/`

For deeper architecture context, see [Architecture](/developer/architecture).

---

[SSL and Domains](/workflows/ssl-and-domains) | [Architecture](/developer/architecture)
