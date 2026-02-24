# Govard Desktop

The desktop app is Wails-based and reuses the same core engine as the CLI.

Current status:
- Wails entrypoint in `cmd/govard-desktop`
- Dashboard UI in `desktop/frontend`
- `govard desktop` command to launch app (`--dev` for Wails dev mode)
- Optional background mode via `govard desktop --background` (start hidden, hide-on-close, single-instance reopen)
- Shell user preferences stored in `~/.govard/desktop-preferences.json`
- Desktop settings include theme, proxy target, and preferred browser

Keyboard shortcuts:
- `Ctrl+,` / `Cmd+,` opens Settings
- `Esc` closes Settings

Lightweight dashboard features:
- Environment list with start/stop/open
- Quick actions (Mailpit, PHPMyAdmin, Xdebug toggle, health)
- Embedded Mailpit inbox panel with refresh/open controls
- Project onboarding panel (folder picker + add/init when `govard.yml` is missing)
- Remotes tab (list/add remotes, connectivity test, and sync plan presets)
- Resource monitor (CPU/RAM/NET per project, OOM hints, refresh + auto-refresh)
- Logs with multi-service (`all`) selection, severity filtering, text search, and live streaming
- Shell launcher (service, user, shell)
- Native notifications from operation success/failure events while desktop is running
- Warnings panel
- Settings drawer

Removed from desktop surface:
- Operations UI and operations output/history views
- Workflow snapshot cards
- Role-mode gated UI complexity

Frontend architecture:
- `desktop/frontend/main.js` bootstraps modules and event wiring
- `desktop/frontend/services/bridge.js` wraps Wails calls
- `desktop/frontend/state/store.js` holds UI state
- `desktop/frontend/modules/` contains feature modules
- `desktop/frontend/ui/toast.js` handles toasts
- `desktop/frontend/utils/dom.js` contains shared DOM helpers
