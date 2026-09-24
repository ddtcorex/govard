# Desktop Glue

This package hosts the Go side of the Govard Desktop app: its services and the
seam onto the Wails v3 runtime.

## Shape

- `App` is the composition root. It owns the services and the platform, and is
  NOT bound to the frontend: the eight services are (`boundServices` in
  `run_desktop.go`), plus the operation watcher for its lifecycle alone.
- Services embed `serviceBase`, which carries the `Platform` and the lifecycle
  context Wails hands to `ServiceStartup` and cancels at shutdown. Services never
  hold a runtime handle; `Platform` is the only runtime surface, and its two
  implementations are the v3 adapter (`platform_wails.go`, `desktop` tag) and the
  stub (`platform_stub.go`, used by the CLI and the untagged tests).
- Every bound method that returns an error opens with
  `defer RecoverPanic(&err, "<Name>")`, because a panic in a service call must
  reach the UI as an error instead of killing the app.
  `tests/desktop_service_panic_guard_test.go` fails when one does not.
- `close.go` holds the whole close decision (`decideClose` plus the `closeGate`
  that lets an explicit quit through the window hook), `sni_linux.go` probes the
  session bus for a tray host, and `tray_wails.go` builds the tray menu.
- `events.go` names every backend event and `events_wails.go` registers each with
  its payload type, so `make bindings` emits typed declarations.

Frontend entry: `cmd/govard-desktop`. The Go bindings it generates live in
`desktop/frontend/bindings`, and `make bindings-check` (run by CI) keeps them
honest.

Currently implemented:
- Dashboard data from Docker + `.govard.yml`
- Environment start/stop/open
- Quick actions (Mailpit, PHPMyAdmin, Xdebug toggle, health)
- Logs retrieval and live log streaming
- Shell launcher (service, user, shell)
- Shell user preference persistence
- Desktop settings persistence (theme, proxy target, preferred browser)
- Remote management and synchronization
- Self-update checks and installer
- System tray with show/hide, project start/stop/open and quit
