---
title: Govard Desktop App
description: Govard Desktop is a Wails 3 GUI sharing the same core engine as the CLI, with live logs, quick actions, a system tray, and a project dashboard.
---

# Desktop App

Govard Desktop is the Wails 3 GUI that reuses the same core engine as the CLI. It runs on GTK 4 and WebKitGTK 6.0, which Ubuntu 24.04+ and Debian 13+ ship; Ubuntu 22.04 and Debian 12 install the CLI only.

---

## Launch Modes

```bash
govard desktop              # Launch the built desktop binary
govard desktop --dev        # Vite dev server plus a Go rebuild against it
govard desktop --background # Start hidden, reuse running instance on relaunch
```

| Mode | Description |
| :--- | :--- |
| `govard desktop` | Standard launch — uses built binary |
| `govard desktop --dev` | Dev mode — Vite HMR on `http://localhost:5173`, live Go backend |
| `govard desktop --background` | Start hidden, keep running with a tray icon. With no tray host it starts visible instead |

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
| **System Tray** | Show/hide the window, start/stop/open projects, quit |
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

Prerequisites: Go, Node.js 24+, pnpm, and on Linux `libgtk-4-dev` plus
`libwebkitgtk-6.0-dev`. The desktop UI is bundled by Vite into
`desktop/frontend/dist`, which the Go binary embeds.

```bash
make frontend                      # Vite build into desktop/frontend/dist
DISPLAY=:1 govard desktop --dev    # Vite HMR + Go backend rebuild
make bindings                      # regenerate the JS bindings from the Go services
```

`govard desktop --dev` starts Vite on `http://localhost:5173` and runs the app
with `FRONTEND_DEVSERVER_URL` pointing at it, which Wails proxies in a build
without the `production` tag. There is no Wails CLI step and no `wails.json`.

`make frontend` is not optional: a desktop build that skips it embeds an empty
`dist/` and shows a blank window. Every automated desktop build path (CI,
goreleaser, `scripts/build-macos-pkg.sh`, `install.sh --source`) runs it first.

Opening the Vite server on `http://localhost:5173` or `dist/` directly renders
the same shell with mock data and a "Desktop bridge not available" notice, which
is enough for styling and layout work; real project data needs the app window.

---

## Preview Mode and Behaviour Tests

Preview mode runs the real `main.js` in plain Chrome with every Go binding call
answered from JSON fixtures, so UI work and automated scenarios need no Go
backend and no app window.

```bash
pnpm --dir desktop/frontend dev    # Vite on http://localhost:5173
# then open http://localhost:5173/preview.html
```

`preview.html` is a copy of `index.html` whose only differences are the preview
bootstrap script and the demo island container. Every markup change in
`index.html` must be mirrored into `preview.html`;
`TestPreviewHTMLMirrorsIndexHTML` fails when they drift. Nothing under
`desktop/frontend/preview/` reaches the production build.

The page exposes one control surface, `window.__govardPreview`:

| Member | Purpose |
| :--- | :--- |
| `reset()` | Clear the installed fixtures and the recorded calls |
| `installFixtures(name)` | Load `desktop/frontend/preview/fixtures/<name>.json` |
| `pushEvent(name, data)` | Dispatch a backend event to the app's subscriptions |
| `getCalls()` | Return the binding calls the app has made so far |

A fixture file is a JSON array of `{ "service", "method", "args", "result" }`
entries, with `"error"` (a message string) in place of `"result"` for a failing
call. Values use the JSON wire field names the Go services emit (for example
`cpuUsage`, not `CPUUsage`). A call matches the entry with the same service,
method and arguments, then falls back to the first entry for that service and
method, then to the generated route default in
`preview/route-defaults.generated.js`.

Record mode captures real fixtures from the running app:

```bash
GOVARD_PREVIEW_RECORD=1 go run ./cmd/govard desktop --dev
```

Every binding call still reaches the real backend and is also appended to
`preview/fixtures/<mode>.json`, where `<mode>` is the current sidebar mode.
Rename the file after the module it belongs to before committing it.

Behaviour tests drive `preview.html` over raw CDP in headless Chrome, each
scenario against its own throwaway Vite server:

```bash
make test-frontend-behaviour                                     # finds google-chrome or chromium
make test-frontend-behaviour CHROME_BIN=/opt/google/chrome/chrome
```

The scenarios live in `tests/frontend/behaviour/*.behaviour.test.mjs` and CI
runs them after `make test-frontend`.

React islands mount through `mountIsland(containerId, element)` from
`desktop/frontend/islands/mount.js`. It returns `{ unmount() }`, or `null` when
the container is missing, so `main.js` keeps booting on a page without it.

---

## Frontend Layout

| File | Purpose |
| :--- | :--- |
| `desktop/frontend/index.html` | Main HTML entry |
| `desktop/frontend/main.js` | Bootstrap, event wiring, tab/state management |
| `desktop/frontend/services/bridge.js` | Wails Go backend RPC bridge; the only module allowed to call Go |
| `desktop/frontend/services/events.js` | Backend event subscriptions; the only module allowed to use `@wailsio/runtime` |
| `desktop/frontend/bindings/` | Generated from the Go services by `make bindings`; committed, never edited by hand |
| `desktop/frontend/state/store.js` | Shared UI state (selected project, filters) |
| `desktop/frontend/modules/` | Feature modules (dashboard, logs, remotes, etc.) |
| `desktop/frontend/ui/toast.js` | Toast notification system |
| `desktop/frontend/utils/dom.js` | Shared DOM helpers |

### Test Mode Behavior

| Access Method | Backend | Data |
| :--- | :--- | :--- |
| The app window | Bindings active | Real project data |
| Vite dev (`localhost:5173`) or `dist/` | Bindings unavailable | Mock fallback data + warning toast |

A Go test (`tests/desktop_frontend_bridge_guard_test.go`) fails if any frontend
file other than `services/bridge.js` and `services/events.js` touches
`window.go`, `window.runtime`, `desktopBridge.runtime`, `@wailsio/runtime` or
`bindings/`. Both modules are JSDoc-typed with `// @ts-check`, so `pnpm typecheck`
checks the shapes they declare, and a second Go test
(`tests/desktop_bindings_contract_test.go`) fails when a route in `bridge.js` no
longer matches the generated bindings, so a renamed Go method is caught in CI
rather than by a user.

### Closing the window

Closing the window keeps Govard running in the tray when **Run in background** is
on (the default) and a tray host is available; otherwise it quits, so a window can
never be hidden with no way back to it. Vanilla GNOME has no tray host: install
the AppIndicator extension, or closing quits. `govard desktop doctor` reports
which case the machine is in, and the Settings drawer says so when no tray is
available. The tray menu shows or hides the window, lists projects with running
ones first, starts and stops them, opens them in the browser, and quits.

### Platform support

| Platform | Desktop app | Notes |
| :--- | :--- | :--- |
| Linux, Ubuntu 24.04+ / Debian 13+ | Yes | GTK 4 and WebKitGTK 6.0 |
| Linux, Ubuntu 22.04 / Debian 12 | No | The CLI installs; no WebKitGTK 6.0 |
| macOS | Not yet | The package ships the CLI only; `govard self-update` leaves an installed desktop binary unchanged |

### Launcher identity on Linux

The deb installs its launcher as `io.github.ddtcorex.govard.desktop`, and the
desktop app advertises that same string as its GTK application id
(`DesktopApplicationID` in `internal/desktop/launch.go`). On Wayland that id is
what GNOME groups windows by: it looks up the desktop file
`"<application id>.desktop"` and does not fall back to `StartupWMClass` for a
Wayland surface. A window whose id matches no launcher becomes a window-backed
app, so the dock shows a second, differently named icon next to the pinned one.
Keep the constant, the file name under `packaging/linux/`, its `StartupWMClass`
and the GoReleaser entry in lockstep — `tests/linux_desktop_entry_test.go` fails
when they drift.

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
