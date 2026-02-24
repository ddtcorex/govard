# Desktop Glue

This package hosts Wails-specific Go APIs and lifecycle glue for the
Govard Desktop app.

Currently implemented:
- Dashboard data from Docker + `govard.yml`
- Environment start/stop/open
- Quick actions (Mailpit, PHPMyAdmin, Xdebug toggle, health)
- Logs retrieval and live log streaming
- Shell launcher (service, user, shell)
- Shell user preference persistence
- Desktop settings persistence (theme, proxy target, preferred browser)
