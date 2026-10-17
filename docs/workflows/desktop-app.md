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
| **Quick Actions** | PHPMyAdmin, Xdebug toggle, Mailpit, DB client |
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

## Health: Doctor and Integrity Audit

The health header at the top of the dashboard runs two read-only checks for
the selected project: `RunDoctor` (host and project diagnostics with per-check
fixes) and `RunIntegrityAudit` (container-free project integrity audit). Both
are reached through `HealthService` routes in
`desktop/frontend/services/bridge.js` (`runDoctor(project)`,
`runIntegrityAudit(project)`); audit progress streams on the
`audit:progress` event. Long runs can be cancelled (`CancelDoctorRun`,
`CancelAudit`); cancelling an audit discards its late result and the banner
keeps the last completed run.

The doctor run is scoped to the selected project's root: project-bound
checks read that project's files, and the report is bucketed under that
project. A non-empty project that resolves to no directory is refused with
a select-a-project error instead of auditing the working directory. With no
project selected the run stays host-level (working directory) under the
empty scope.

### Doctor fixes: what each button means

Every doctor check carries one of three fix affordances. `Fix` applies in one
click. `Preview fix` opens a dry-run plan that must be confirmed explicitly
before anything changes. Checks with no safe fix show no fix button at all;
follow the hint and the suggested CLI command instead. A check ID the
allowlist has never seen is refused rather than given a button.

| Check ID | Button | What the fix does |
| :--- | :--- | :--- |
| `host.govard.home` | Fix | Repairs Govard runtime directory permissions |
| `host.compose.spam` | Fix | Purges stale compose files older than 7 days |
| `host.govard.registry` | Fix | Repairs the project registry path (a directory at the file path is refused, never auto-deleted) |
| `host.search.index_block` | Preview fix | Unblocks the search index with a write into the running search container |
| `project.runtime.images` | Preview fix | Pulls missing Docker runtime images (network heavy) |
| `project.config.audit` | Preview fix | Migrates legacy config keys to the services block in `.govard.yml` |
| `project.config.drift` | Preview fix | Syncs `.govard.yml` with the detected framework profile |
| `project.profile.sync` | Preview fix | Fills gaps from the framework profile, preserving explicit user settings |
| `docker.daemon` | None | No fix in the desktop; follow the hint |
| `docker.compose` | None | No fix in the desktop; follow the hint |
| `host.port.80` | None | No fix in the desktop; follow the hint |
| `host.port.443` | None | No fix in the desktop; follow the hint |
| `host.disk.scratch` | None | No fix in the desktop; follow the hint |
| `host.network.outbound` | None | No fix in the desktop; follow the hint |
| `host.ssh.agent` | None | No fix in the desktop; follow the hint |
| `host.deps.docker` | None | No fix in the desktop; follow the hint |
| `host.deps.remote` | None | No fix in the desktop; follow the hint |

Project-touching fixes (`host.search.index_block` and every `project.*`
check) need a project selected in the desktop app; host-global fixes may run
without one. After any applied fix the panel re-runs doctor silently, so the
banner clears only when a fresh run is clean.

### Risky fixes: preview, confirm, expiry

`PreviewDoctorFix` returns a dry-run plan (summary plus actions) without
changing anything; `ApplyDoctorFix` with explicit confirm applies it. In user
terms: a preview stays valid for a short time (about 15 minutes) and can be
used once. If project files or container state change between the preview and
the apply, the apply is refused and the panel asks for a fresh preview, so
confirm again on the new plan. Closing the preview or switching projects drops
the pending preview.

### Integrity audit scope

The desktop runs exactly the container-free `integrity` check, the same
analysis `govard audit run --checks integrity` executes. The `lint` and
`profiler` rows stay visible in the result with `Enabled=false` and a reason
pointing at the CLI (`govard audit run --checks lint`, `govard audit run
--checks profiler`); they are never run from the desktop and never hidden.
`GetAuditResult` reads one stored run, `GetAuditDiff` summarizes the finding
delta between two stored runs, and `LastAuditResult` fills the banner from
the last completed run without running anything. The panel queries
`RunningAuditOp` on mount, so Cancel is offered even when the run started
before the panel did.

### Banners and per-project scoping

Banners derive from the latest results for the currently selected project
only. Switching projects re-derives that project's banners and shows
loading or empty meanwhile, never another project's banner.

| Banner | Meaning |
| :--- | :--- |
| Doctor, red (`error`) | The latest doctor report has at least one failing check. The banner names the issue count and how many are fixable below |
| Doctor, amber (`warning`) | Warnings only, no failures |
| No doctor banner | No report yet, or the latest report passes all checks. A clean re-run is what clears a stale banner |
| Audit, amber (`warning`) | The latest integrity result has findings (or a non-passed status). The banner names the finding count; the panel lists each finding plus the disabled `lint` and `profiler` rows |
| No audit banner | No audit ran yet for this project, or the latest run passed clean. Cancelled and failed runs never replace the last completed result |

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

## Health: Doctor and Audit

The header shows one banner per source, each scoped to the selected project.
Switching projects shows that project's banners; a scope that never ran shows
none. Both banners derive from the latest result only, so a clean re-run
clears a stale banner instead of letting it linger. The Doctor and Audit
panels sit below the mesh snapshot in the same view.

### Doctor banner

Run doctor from the Doctor panel. The banner appears only when the latest
report has a failing or warning check:

| Banner | Meaning |
| :--- | :--- |
| Red (error), `Doctor: N issues` | At least one check failed. When at least one failing or warning check has a fix button, `, M fixable below` is appended. |
| Yellow (warning), same text | Warnings only, no failures. |
| No banner | The latest run passed everything, or doctor never ran for this project. |

`Re-run` on the banner runs doctor again. Long runs can be cancelled; a
cancelled run leaves the previous banner untouched.

### Doctor fixes: what each check offers

Every doctor check carries one of three fix kinds. The button label tells
which one before anything runs:

| Check ID | What the fix does | How it applies |
| :--- | :--- | :--- |
| `host.govard.home` | Repairs Govard runtime directories (permissions) | `Fix` button, one click |
| `host.compose.spam` | Prunes stale compose files | `Fix` button, one click |
| `host.govard.registry` | Repairs a corrupt project registry file path (a directory at that path is refused, nothing is ever deleted) | `Fix` button, one click |
| `host.search.index_block` | Unblocks the search index (writes into the running search container, so it is never one click) | `Preview fix`, then confirm |
| `project.runtime.images` | Pulls missing runtime images (network download) | `Preview fix`, then confirm |
| `project.config.audit` | Migrates legacy config keys (config write) | `Preview fix`, then confirm |
| `project.config.drift` | Syncs `.govard.yml` with the detected profile (config write) | `Preview fix`, then confirm |
| `project.profile.sync` | Syncs the profile without overwriting explicit user settings (config write) | `Preview fix`, then confirm |
| `docker.daemon`, `docker.compose`, `host.port.80`, `host.port.443`, `host.disk.scratch`, `host.network.outbound`, `host.ssh.agent`, `host.deps.docker`, `host.deps.remote` | No fix path, follow the check's hint or suggested command instead | No fix button |

A check ID the desktop does not recognise is refused with an `unknown doctor
check` error rather than silently offering nothing, so a new engine check
forces an allowlist review instead of landing on the wrong button.

Project fixes (every `project.*` check plus `host.search.index_block`) need a
project selected first; with nothing selected the preview and the apply both
refuse and say so. Host-global one-click fixes run without a project.

### Preview and confirm, in plain terms

Risky fixes always open a preview first. The preview is a dry run: it lists
what would change and changes nothing. Confirming (`Confirm and apply`)
applies exactly the previewed plan, then doctor re-runs so fixed issues
disappear from fresh results.

Each preview mints a one-time approval that is valid for 15 minutes, for that
check and that plan only. It works once: confirming consumes it, cancelling
the dialog drops it, and switching projects drops it. If anything changed
between preview and confirm, the apply is refused and asks for a fresh
preview instead of applying a stale plan. The approval itself is never shown
on screen; it lives in memory only, so restarting the app invalidates pending
previews. Safe one-click fixes skip all of this and apply immediately.

### Audit scope and banner

`Run audit` from the Audit panel runs the container-free integrity audit only,
the same analyzers as `govard audit run --checks integrity`. It never needs
Docker and never touches a container.

The result lists three rows. The integrity row runs; the other two stay
visible but disabled, never hidden:

| Row | State | Reason shown |
| :--- | :--- | :--- |
| `integrity` | Runs | Container-free |
| `lint` | Disabled | Requires Docker and is not run from the desktop; run `govard audit run --checks lint` from the CLI |
| `profiler` | Disabled | Requires Docker and is not run from the desktop; run `govard audit run --checks profiler` from the CLI |

A session with no integrity run shows the integrity row disabled with a `Session has no integrity run` reason instead of claiming a run.

The audit banner is yellow (warning) with `Audit: N findings` and a `Run
audit` button. There is no banner when the audit never ran for this project,
or when the latest run passed with zero findings. Cancelling an audit
discards the in-flight result: the analysis may finish in the background, but
its result is never recorded and the previous banner stays. `Cancel` is also
available when the run started before the panel did, via the running-operation
lookup on mount.

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
