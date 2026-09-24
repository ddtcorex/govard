# Desktop Frontend

The desktop UI served by Wails. Plain ES modules bundled by Vite, styled with
Tailwind 3 through PostCSS.

## Commands

```bash
pnpm install          # once
pnpm dev              # Vite dev server on http://localhost:5173 with HMR
pnpm build            # production build into dist/
pnpm typecheck        # tsc --noEmit (checks the // @ts-check modules)
```

`make frontend` from the repo root runs install plus build. Every desktop binary
build depends on it, because `embed.go` embeds `dist/`.

## Layout

| Path | Purpose |
| :--- | :--- |
| `index.html` | Entry document; Vite rewrites its asset URLs on build |
| `main.js` | Bootstrap, event wiring, tab and state management |
| `services/bridge.js` | The only module allowed to call the Go backend |
| `services/events.js` | The only module allowed to subscribe to backend events |
| `types/wails-v2.d.ts` | Shape of the globals Wails v2 injects |
| `modules/` | Feature modules (dashboard, logs, remotes, settings, ...) |
| `state/store.js` | Shared UI state |
| `ui/`, `utils/` | Toasts and DOM helpers |
| `assets/styles-src.css` | Tailwind source; `styles.css` is build output and is not committed |
| `dist/` | Vite build output, not committed (`dist/.gitkeep` keeps the embed valid) |

A Go test (`tests/desktop_frontend_bridge_guard_test.go`) fails if any file other
than the two `services/` modules and the type declaration touches `window.go`,
`window.runtime` or `desktopBridge.runtime`.

## Opening the UI without Wails

`pnpm dev` (or any static server over `dist/`) renders the shell from the
committed HTML with mock data and a "Desktop bridge not available" notice. That
is the browser-testing path for styling and layout work.
