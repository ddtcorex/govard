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

Long operations get long ceilings: `up`, `bootstrap`, restarts and pulls wait up to 15 minutes (10 for global-services start) instead of failing at two. A timeout names its duration; when the operation may have left containers half-started, the message says so and points at the dashboard's orphan detection. One mutating environment operation runs per project — a double-clicked Start or a Start-then-Stop race reports what is already running instead of starting a duplicate, and the lock is held until the backend call settles even if the UI's own backstop fires first.

Live log panes keep the last 2,000 lines and pause their polling while the window is hidden, resuming with fresh data on return. If a panel crashes, it shows a notice with a retry button rather than going blank.

---

## Operation Center

The Operations tab is the visible home for long operations. It shows two
lists, both served by `OperationService` through the `bridge.js` routes
`ListOps`, `ListHistory`, `CancelOp` and `RetryOp`, which map onto the Go
methods of the same names (`internal/desktop/operation_service.go`). The
tab itself is the `OperationCenter` React island with pure helpers in
`desktop/frontend/modules/operation-center.js`.

| List | Source | Order |
| :--- | :--- | :--- |
| Running | `ListOps` | Oldest first, with a Cancel button per op |
| History | `ListHistory` | Newest first; `ListHistory(limit)` caps the read, `limit <= 0` means all |

The island refreshes both lists on the `sync:progress` and
`operations:notification` backend events (subscribed through
`services/events.js`; the constants `SYNC_PROGRESS_EVENT` and
`OPERATIONS_NOTIFICATION_EVENT` live in `modules/operation-center.js`).

### Concurrency: one sync per project

- One background sync runs per project. `RunRemoteSync` claims the project
  through `TryRegisterSyncingProject` under a single mutex hold, where the
  claim itself is the guard, so two concurrent starters cannot both pass. A
  second start is refused with `cannot start sync for "X" while sync from
  "Y" is in progress` (or `another sync is already in progress` when no
  remote is recorded). Keys are canonical project IDs, so a start addressed
  by domain cannot slip past a sync started by path.
- While a sync runs, mutating environment actions on the same project
  (start, stop, restart, pull, toggle, delete) are refused, because the sync
  owns the containers.
- The registry rejects a duplicate op id (`RegisterOp` and
  `RegisterOpWithArgs` return false and keep the original entry), so a
  duplicate can never displace a cancel func and orphan its context.
- A second `RetryOp` for a record that already has a retry in flight is
  refused; wait for the current run to finish before retrying.

### Cancel

- `CancelOp(id)` on a tracked running op removes the entry, appends one
  `cancelled` terminal record, and fires the stored cancel func exactly once
  (after releasing the lock, so a re-entrant cancel func cannot deadlock). A
  cancel racing a finish records exactly one terminal state. An unknown or
  already finished id returns false, which the UI treats as a harmless no-op
  (`<id> already finished`) rather than an error.
- Limits, stated plainly: background syncs (`RemoteService.RunRemoteSync`)
  and one-shot environment CLI calls are not tracked registry ops in this
  release, so they do not appear under Running and the center cannot cancel
  them. They run to completion and report through the `sync:failed` /
  `sync:completed` events and the operation notifications below. Syncs
  launched in an OS terminal (`RunRemoteSyncInTerminal`) run outside the app
  entirely. Live log streams are separate: `StopLogStream` and
  `StopGlobalServiceLogStream` stop the stream and kill the underlying
  `docker logs` process.

### History retention and crash recovery

- Terminal records persist as JSONL in `<govard-home>/desktop/operation-history.jsonl`
  (honoring `GOVARD_HOME_DIR`), one line per terminal state: `completed`,
  `failed`, `cancelled`, `interrupted`. Args are scrubbed at write (secrets
  become `***` placeholders); messages are sanitized and capped at 2,000
  characters. Corrupt or blank lines are skipped and never consume budget.
- Cap: `OperationHistoryCap` is 200 valid records. Appending past the cap
  evicts the oldest records while the newest stay intact. Full rewrites
  publish atomically (temp file plus rename), so a crash mid-write never
  leaves a half-truncated history.
- Crash recovery: every registered op is mirrored to the
  `operation-inflight.json` sidecar until it reaches a terminal state.
  `RecoverInterruptedOps` runs at startup and marks orphan entries
  `interrupted` with the message `Interrupted by app restart`, deduped by
  (op id, start time) so a terminal record that landed before the crash is
  never duplicated. An unreadable sidecar is quarantined aside
  (`operation-inflight.json.corrupt-<timestamp>`) for forensics: never
  silently dropped, never invented from.

### Retry

- The Retry button appears on failed records only. `RetryOp` replays the
  stored args through the original service path under a fresh op id; the old
  terminal record is never rewritten, so finish and cancel attribution keep
  their exactly-once semantics.
- Every refusal fails closed with an actionable error: an unknown id or an
  id with no terminal record; a record that ended `completed`, `cancelled`
  or `interrupted`; stored args carrying a `***` scrub placeholder (re-run
  with fresh credentials instead of replaying, since the placeholder no
  longer authenticates); an op kind with no registered replay handler
  (`RegisterOpRetryHandler`); a retry already in progress for the same
  record.
- Interrupted entries never replay. The history row shows a fresh-run hint
  instead: `Interrupted by an app restart. Start a fresh run; the stored
  args are kept for reference.`
- Replay handlers for the WS-A/B/C operation families land with those
  follow-ups. Until a kind registers its handler, `RetryOp` keeps failing
  closed (`no replay path`) and the Retry button explains re-running the op
  from its original view instead of replaying it here.

### Notifications

- Terminal operations emit `operations:notification`
  (`EventOperationsNotification`), polled every 2 seconds with a read limit
  of 256 events. Delivery is at-least-once: the watcher emits first and
  persists the cursor second, so a crash between the two redelivers (a
  visible duplicate) instead of silently losing a notification.
- A fresh install suppresses pre-existing log history. A restart resumes
  from the persisted cursor (`operation-notify-cursor` in the same state
  dir) and delivers the downtime gap on its first poll. If the cursor is
  gone (log rotation, or the read window slid past it while the app was
  down), the whole window is delivered rather than dropped.
- Failure toasts carry catalog actions (`retry`, `open-logs`, `copy`
  through `OpActionRetry`, `OpActionOpenLogs`, `OpActionCopy`). Retry is
  bound to the exact source terminal record id (`retryId`): the event's op
  id must name a history record that exists and ended `failed`, and that id
  is what `RetryOp` replays. No match means no Retry button, and a retry
  dispatched without a bound id fails closed as record unavailable instead
  of scanning for a substitute that could replay the wrong run. Success
  notifications carry no actions.

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
| Open admin URL (Remote) | `OpenRemoteURL` builds `https://<url or host>/<admin path>` from the framework's admin path |

For `auth.method: ssh-agent`, Desktop reuses `SSH_AUTH_SOCK` and also probes `/run/user/<uid>/keyring/ssh` on Linux.

Every action resolves the remote before it builds anything, and a remote that
resolves to neither a `url` nor a `host` is refused with an error rather than
opened. The admin URL and the SFTP/SSH launchers would each fall back to the
literal `localhost`, which opens the developer's own machine while reporting that
the remote was opened.

The synthetic `sandbox` remote is resolved from the container `govard sandbox up`
created, never from the project's `remotes.sandbox` block, so a block that names
no host cannot make an action point at `localhost`. It appears in the remote
panel whenever that container answers, and independently of whether the project
configures a block. That is the same trigger `govard remote list` uses, not the
same row rule: the panel drops a sandbox it cannot resolve, where `remote list`
always prints a row carrying the liveness in its HOST column. When a block is
configured but no container is running, the row is left out and the reason comes
back as a warning in the panel; a project with neither a block nor a container
gets no row and no warning. See
[Configuring the sandbox remote](/workflows/remotes-and-sync#configuring-the-sandbox-remote).

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
