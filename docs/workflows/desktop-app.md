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
| **Deploy Tab** | Read-only status/releases/plan/check plus preview-gated rollback and sandbox rehearse |
| **Data Tab** | Masked DB fact sheet, guarded ad-hoc queries, dump/import, snapshots (see below) |
| **Resource Monitor** | CPU, RAM, network, OOM hints |
| **Logs** | Multi-service selection, severity filtering, text search, live streaming |
| **Shell Launcher** | Service, user, and shell selection |
| **Tool Runner** | Preset framework tool commands with streamed output (see below) |
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

## Tool Runner

The Tool Runner palette runs framework tool commands from a backend-owned
preset allowlist. Pick a project, pick a preset, fill its arguments, then Run.
Output streams into the pane (last 2,000 lines, same cap as the log panes).
The Recent runs dropdown re-applies a past preset plus its arguments
(scrubbed values shown as `***` are not re-applied). With no project selected
the palette shows `Select a project to run tools.` and stays disabled.

The UI lives in the React island `desktop/frontend/islands/ToolPalette.tsx`
with pure helpers in `desktop/frontend/modules/tool-palette.js`. It talks to
Go only through `services/bridge.js`:

| Frontend wrapper | Backend route |
| :--- | :--- |
| `listToolPresets(project)` | `ToolService.ListToolPresets` |
| `runTool(project, presetId, args)` | `ToolService.RunTool` |
| `cancelToolRun(opId)` | `ToolService.CancelToolRun` |
| `getToolHistory(project)` | `ToolService.GetToolHistory` |

Progress streams on `tool:output`; the outcome arrives on `tool:completed` or
`tool:failed`.

### Presets

Presets are defined in `internal/desktop/tool_runner.go`. The palette lists
the framework-scoped presets for the selected project plus the generic set;
an unknown or empty framework yields the generic set only.

| Preset ID | Runs | Arguments | Applies to |
| :--- | :--- | :--- | :--- |
| `magento-cache-flush` | `bin/magento cache:flush` | `type` (optional, `^[a-z_]+$`) | Magento family |
| `magento-cache-clean` | `bin/magento cache:clean` | `type` (optional, `^[a-z_]+$`) | Magento family |
| `magento-setup-di-compile` | `bin/magento setup:di:compile` | none | Magento family |
| `magento-indexer-reindex` | `bin/magento indexer:reindex` | `indexer` (optional, `^[a-z_]+$`) | Magento family |
| `laravel-migrate` | `php artisan migrate` | `database` (optional flag), `force` (optional flag, `true/false/1/0`) | Laravel |
| `laravel-cache-clear` | `php artisan cache:clear` | none | Laravel |
| `laravel-config-clear` | `php artisan config:clear` | none | Laravel |
| `composer-install` | `composer install` | `no-dev` (optional flag, `true/false/1/0`) | All projects |
| `npm-install` | `npm install` | none | All projects |
| `npm-run-build` | `npm run build` | none | All projects |

Magento family covers `magento1`, `magento2`, `mageos` and `openmage`
(including framework aliases registered as Magento-derived).

Slow presets (`magento-setup-di-compile`, `magento-indexer-reindex`,
`laravel-migrate`, `composer-install`, `npm-install`, `npm-run-build`) run
with the 15-minute long ceiling; the fast cache clears keep the 2-minute
default. A timeout reports its duration, and the history entry records the
run as failed (timeout) or cancelled (cancel).

### No-shell guarantee

The palette sends a preset ID plus an args map only. The exec argv stays
backend-owned: unknown preset IDs are refused before any process exists, and
args are validated against the preset's `ArgSpec` (unknown names, missing
required args, values over 256 characters, and values outside the arg
pattern are rejected; args without an explicit pattern use
`^[A-Za-z0-9_@+.,:/=-]+$`, which rejects whitespace and every shell
metacharacter with no quoting exemption). Validated args render onto the
preset argv in `ArgSpec` order (flag args become `--name` or
`--name=value`), and the result runs via `exec.CommandContext` directly:
no shell, no `sh -c`, no string joining, ever.

### Cancel guarantee

One tool run per project at a time. A second Run while one is active is
refused with an already-running error, and the lock is freed on completion
or cancel. `CancelToolRun` kills the running process through the op registry
and frees the lock; cancelling an unknown or finished ID is a harmless no-op
reporting `false`, and the synchronous return value is the cancelling
caller's confirmation.

Terminal events carry the run generation (`OpID`, `Gen` in
`ToolResultPayload`). The palette applies a `tool:completed` or `tool:failed`
receipt only when its generation is still current
(`IsCurrentToolGeneration` on the backend, `applyToolTerminalEvent` in
`modules/tool-palette.js`), and a finished run never flips back to running.
Streamed `tool:output` batches from a cancelled, timed-out, or superseded
run are dropped, so a fast rerun never mixes tails; the terminal receipt
itself always arrives so history and status settle.

### History and secret scrubbing

Each admitted run is recorded in a per-project JSONL history file under
`tool-history/` in the Govard home directory, capped at the most recent 50
runs (the oldest entries evict first). The admission entry starts as
`unknown` and is promoted to `success`, `failed`, or `cancelled` before its
terminal event emits, so a run whose terminal never arrives (crash) reads
`unknown`, never success. A history failure never fails the run itself.

Args are scrubbed on write, so raw secrets never reach the file: values under
secret key names (`password`, `passwd`, `passphrase`, `secret`, `token`,
`apikey`, `accesskey`, `privatekey`, `pwd`, including suffix matches such as
`db-password` or `apiKey`) become `***`, and secret-shaped value text
(`KEY=value` pairs, `scheme://user:pass@` credentialed URLs, PEM private-key
blocks) is masked the same way. One boundary applies: a bare secret value
under a non-secret key name is indistinguishable from an innocent value and
is not detectable, so only the key name and the value shape drive scrubbing.
The file is written atomically with strict permissions.

## Data Tab

The Data tab manages the selected project database and its snapshots. It
covers the masked connection fact sheet, guarded ad-hoc queries,
dump/import with streaming progress, and snapshot list/create/export plus
preview-gated restore.

### Read-only queries by default, explicit opt-in for writes

`DBService.RunDBQuery(project, sql, allowWrite, confirmToken)` classifies
every statement before it touches anything:

- Read-only openers are `SELECT`, `WITH`, `SHOW`, `DESCRIBE`, `DESC`,
  `EXPLAIN`. Anything else takes the write path.
- A read-looking opener is not enough on its own: the whole statement is
  scanned for data-modifying words (`DELETE`, `UPDATE`, `INSERT`,
  `INTO`, `UNION` and friends, locking reads such as `FOR UPDATE`), so a
  write hidden in the body still takes the write path.
- Multi-statement input is always refused. Refusal happens before project
  discovery, so a refused statement touches nothing.

A write needs two things together: `allowWrite=true` plus the per-project
confirm token from `DBService.GetDBWriteConfirmToken(project)`. The UI
flow is: the mode badge reads anything uncertain as "write", the write
checkbox enables the confirm dialog, the dialog fetches the token and
passes it back to `RunDBQuery`. The token format is
`confirm-write:<lowercased-project>` and stays Go-owned; the island only
carries it. The token guards against accidental clicks, not against
malice: the desktop already holds the credentials.

### Restore is preview plus confirm, never one-shot

`SnapshotService.PreviewSnapshotRestore(project, name, dbOnly, mediaOnly)`
writes nothing. It reports exactly what an apply would replace
(`willReplaceDb`, `willReplaceMedia`, plus a human `summary` line) and
returns a single-use `previewToken`. The confirm control stays disabled
until that preview renders.

`SnapshotService.ApplySnapshotRestore(project, name, previewToken,
confirm, dbOnly, mediaOnly)` requires the token plus `confirm=true`.
Either missing, the restore never starts. The token is bound to the
project, snapshot, and scope, expires after 15 minutes, and is consumed by
a validated apply, so a replay never reaches the engine. A refused apply
consumes nothing, so confirming afterwards still works. Scope is `full`
by default, or `dbOnly` / `mediaOnly` from the scope radio.

### Scrubbing guarantees

- `DBService.GetDBInfo` returns a struct with no password field by
  construction. `hasPassword` only reports whether one is configured, and
  `displayUrl` renders the password as `****`.
- Query output passes through `sanitizeUIText`, capped at 20,000 runes,
  which also strips ANSI escapes and control bytes.
- The frontend renders DB info through the `pickDisplayDBInfo` allowlist
  (`project`, `engine`, `host`, `port`, `database`, `user`,
  `hasPassword`, `displayUrl`): any unexpected key, including a password
  under any name, is dropped before it can reach the DOM.
- There is deliberately no query history: nothing typed into the query
  box is stored, so no secret can linger in one.

### Cancel semantics

`DBService.CancelDBOp(opId)` and
`SnapshotService.CancelSnapshotOp(opId)` stop desktop tracking first: the
op identity is dropped so late events from the dying goroutine never
reach the UI, and the user gets a terminal `db:failed` /
`snapshot:failed` event. What happens underneath differs:

- Dump/import run the transfer through a `docker` process started with
  the op context, so cancelling fires that context and kills the process.
  A cancelled dump also removes its `.partial` sibling, and an earlier
  file at the destination survives.
- Snapshot restore calls the engine, which is not context-aware once the
  import starts. Cancelling stops the desktop op and its events while the
  underlying import may run to completion, the same as interrupting the
  CLI. The message says exactly that: "`<Kind>` tracking stopped by user; the
  underlying operation may run to completion" (for example "Snapshot restore
  tracking stopped by user; ..."). It never claims a
  cancellation that was not delivered.

### Disk preflight

- `DumpDB` estimates the database size, then probes free space against
  the nearest existing ancestor of the destination before creating any
  directory or starting any process. A refusal leaves no file, no
  directory, and no started process behind.
- `ImportDB` validates the file synchronously before the op is
  registered: it must be a non-empty file (not a directory) carrying a
  recognized dump header (`mysqldump`/`MariaDB` or PostgreSQL, gzip
  handled transparently), and its engine must match the project database.
  A refused file never opens the database writer.
- `ExportSnapshot` refuses a destination that is a directory or whose
  parent directory does not exist. Overwrite confirmation for an existing
  file stays the UI job: the save dialogs return `{ path, exists }` so
  the island can ask before calling dump or export.

### Routes and events

Bridge routes in `desktop/frontend/services/bridge.js` onto the generated
bindings (verified against `DBService`, `SnapshotService`, and
`data_dialog.go`):

| Route | Go service.method |
| :--- | :--- |
| `GetDBInfo` | `DBService.GetDBInfo` |
| `RunDBQuery` | `DBService.RunDBQuery` |
| `DumpDB` | `DBService.DumpDB` |
| `ImportDB` | `DBService.ImportDB` |
| `CancelDBOp` | `DBService.CancelDBOp` |
| `PickDumpDestination` | `DBService.PickDumpDestination` |
| `PickImportSource` | `DBService.PickImportSource` |
| `GetDBWriteConfirmToken` | `DBService.GetDBWriteConfirmToken` |
| `ListSnapshots` | `SnapshotService.ListSnapshots` |
| `CreateSnapshot` | `SnapshotService.CreateSnapshot` |
| `PreviewSnapshotRestore` | `SnapshotService.PreviewSnapshotRestore` |
| `ApplySnapshotRestore` | `SnapshotService.ApplySnapshotRestore` |
| `ExportSnapshot` | `SnapshotService.ExportSnapshot` |
| `CancelSnapshotOp` | `SnapshotService.CancelSnapshotOp` |
| `PickExportDestination` | `SnapshotService.PickExportDestination` |

Backend events in `internal/desktop/events.go`:

| Event | Purpose |
| :--- | :--- |
| `db:progress` | Structured dump/import progress (`opId`, `generation`, byte counts, percent) |
| `db:completed` | Finished dump/import |
| `db:failed` | Failed or cancelled dump/import |
| `snapshot:completed` | Finished snapshot create/restore/export |
| `snapshot:failed` | Failed or cancelled snapshot op |

Snapshot ops emit terminal events only, no progress events. An empty
dialog pick (save or open) with no error means the user cancelled, and
the island sends exactly the picked path to dump/import/export, never an
invented one. The dump dialog defaults to `<project>-dump.sql`; the
export dialog defaults to `<name>.tar.gz`, matching the CLI.

---


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

---

## Deploy Tab

The Deploy tab reuses the same engine paths as the CLI, split into two
milestones by blast radius.

### M1: read-only visibility

| Panel view | CLI equivalent |
| :--- | :--- |
| Status | `govard deploy status [remote]` |
| Releases | `govard deploy releases [remote]` |
| Plan | `govard deploy plan [remote]` |
| Check | `govard deploy check [remote]` |

These four calls never change target state, with one disclosed exception:
on a symlink-strategy target the check performs a transient probe write to
verify atomic swaps, then removes it. The panel shows a note whenever the
check reports that probe ran, so the check is never presented as purely
observational. Fetching stays gated behind the remote picker: with no remote
picked, no call leaves the panel.

### M2: guarded actions

| Panel action | CLI equivalent |
| :--- | :--- |
| Rollback preview + apply | `govard deploy rollback [remote]` |
| Sandbox start preview + apply | `govard sandbox up` |
| Sandbox removal preview + apply | `govard sandbox down` |
| Connection info + open shell | `govard sandbox ssh` |

Every mutation runs the same preview-plus-confirm contract:

1. **Preview** shows exactly what the apply would do (the rollback target
   release and strategy, or the sandbox container) and mints a single-use
   token bound to that action and those parameters.
2. **Apply** spends the token and additionally requires an explicit confirm:
   the panel keeps the apply button disabled until the preview is shown AND
   a checkbox ("I have read the preview ...") is ticked, so no single click
   can change target state.

Tokens expire after 15 minutes; unknown, spent, foreign, or expired tokens
are refused before any engine call runs. A preview that no longer matches
the target (a deploy landed in between, or the sandbox container changed)
is refused as stale, and the operator re-previews. Rollback with `--with-db`
stays CLI-only: database restore is destructive and keeps its own
confirmation there. The sandbox rehearse starts empty; seeding stays
CLI-only.

### Cancel truthfulness

Stopping a run mid-flight reports what the engine left behind, never a bare
"cancelled". An in-place rollback names whether the release record was
restored and whether the maintenance window was closed; a symlink rollback
names whether a release was activated before the stop; a sandbox start or
removal names the container state it re-read afterwards (still running, not
running, none left, or state that could not be determined). The panel renders
a cancelled run as cancelled with those notes, never as a failure, and offers
no retry prompt for it.

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
