---
title: Lock File — Detect Environment Drift
description: Use govard.lock to snapshot and enforce reproducible team environments — generate, check, diff, and strict mode.
---

# Lock File

`govard.lock` snapshots the resolved environment (framework, stack versions, compose hash, host Docker version, blueprint versions) so the team can detect drift and enforce reproducibility across machines.

---

## Quick Start

```bash
# Snapshot the current project
govard lock generate
govard lock check          # pass/fail vs current disk state

# Compare without failing
govard lock diff

# Custom path (e.g. for .govard/ layouts)
govard lock generate --file .govard/govard.lock
```

Commit `govard.lock` (or `.govard/govard.lock`) to Git — it is the team contract.

---

## Strict Mode

In `.govard.yml`:

```yaml
lock:
  strict: true
  ignore_fields: ["host.docker_version"]  # skip noisy host fields
```

| `lock.strict` | Behavior on `govard env up` |
| :--- | :--- |
| `false` (default) | Warns on mismatch but starts anyway. |
| `true` | Fails fast when lock is missing or mismatched — the developer must `govard lock generate` after an intentional change. |

`lock.ignore_fields` lists JSON paths to skip during compliance (e.g. `host.docker_version` which varies per machine).

---

## When to Regenerate

Regenerate after any intentional stack change:

- `stack.php_version`, `stack.db_version`, `stack.search_version`, …
- `.govard.yml` / profile edits
- Govard upgrade that bumps `BlueprintVersion` (see `CHANGELOG.md` “Blueprint Lifecycle”)

```bash
govard env up --update-lock   # update lock atomically if a mismatch is detected
# or
govard lock generate
```

---

## CI Example

```yaml
# .github/workflows/ci.yml
- run: govard lock check
```

`lock check` is non-zero on drift, so CI fails until the lock is updated intentionally.

---

→ Reference: [CLI Commands](/reference/cli-commands#govard-lock) · [Configuration](/reference/configuration#safety-and-reproducibility)
