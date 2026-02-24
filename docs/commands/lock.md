# govard lock

Generate and validate `govard.lock` for environment consistency.

## Usage

```bash
govard lock generate
govard lock check
govard lock generate --file .govard/govard.lock
govard lock check --file .govard/govard.lock
```

## Subcommands

### `generate`

Captures current values into a lock file:
- Govard version
- Host OS/architecture
- Docker version
- Docker Compose version
- Project/stack metadata from `govard.yml`
- Service image references from rendered compose (when available)

### `check`

Compares current environment values with lock file values and reports mismatches.

Current behavior:
- Returns success when fully compliant.
- Returns a non-zero exit code when mismatches are found.
- Used by `govard env up` in warning-only mode (does not block startup).

## Strict Mode (`govard env up`)

Enable strict lock enforcement in `govard.yml`:

```yaml
lock:
  strict: true
```

When enabled:
- `govard env up` fails if `govard.lock` is missing.
- `govard env up` fails if lock mismatches are detected.
- `govard env up` still prints lock mismatch details before exiting.

## Options

- `--file` Path to lock file (default: `./govard.lock`)
