---
title: Project Audit — Lint & Profiler
description: Run Govard's persistent lint and profiler audits — Govard-native backend, external providers, caching, and target modes.
---

# Audit

Govard persists every audit as an immutable session under `~/.govard/audit/<project-id>/sessions/<session-id>/`. Use it to lint Magento code, catch webshells, and capture the stock Magento CSV profiler without editing `env.php`.

---

## Quick Start

```bash
# Lint the whole project (auto-detect mode)
govard audit run

# Lint + profiler on one URL (first run needs --url)
govard audit run --checks lint,profiler --url 'https://shop.test/category.html?product_list_limit=48'

# Profiler only
govard audit run --checks profiler --url 'https://shop.test/'

# Re-run the exact same checks (reuses frozen profiler URL)
govard audit rerun --session 20260816T010203Z-a1b2c3d4

# Inspect
govard audit status --session 20260816T010203Z-a1b2c3d4
govard audit result --session 20260816T010203Z-a1b2c3d4 --run run-0001
govard audit diff --base origin/master

# Machine-readable
govard audit run --format json
govard audit rerun --session <id> --format json
```

Exit code: `0` when all checks pass, non-zero after the summary when any check failed or was cancelled — suitable for CI.

---

## Checks

| Check | What it does |
| :--- | :--- |
| `lint` | Static analysis via the Govard-native backend (`phpcs` + `phpstan` + media guard). |
| `profiler` | Captures Magento's stock `MAGE_PROFILER=csvfile` via a lease-protected web-server include, one bounded `GET` to `--url`, then restores everything. |

`profiler` requires:
- A whole Govard project target (standalone/module-only rejected before any mutation).
- An absolute `http(s)://` `--url` on the first run (frozen in the run and reused by `rerun`).
- A prior `govard env up` with the current Govard version (so the custom config mount exists).

Artifacts: `runs/<run-id>/artifacts/profiler/profile.csv` + SHA-256 digest.

---

## Target Modes (`--mode`)

`auto` (default) classifies the current directory:

| Mode | When | What is analyzed |
| :--- | :--- | :--- |
| `project` | Directory contains a Magento project root (`bin/magento` + Magento Composer requirement) and no enclosing module | Whole project |
| `module_in_project` | Directory is a module (`etc/module.xml` or Composer type `magento2-module`) inside a Magento project | Only the module (whole project mounted read-only) |
| `standalone` | Directory is a module with no Magento project above it | Only the module (deps installed into a scratch worktree) |

Force with `--mode project|module_in_project|standalone` — fails if the directory doesn't support it.

---

## PHP Versions (`--php`)

Lint image provides `7.4`, `8.0`, `8.1`, `8.2`, `8.3`, `8.4`, `8.5`.

- `project` / `module_in_project`: exactly one version — the project's active `stack.php_version`. Passing `--php` only accepted when it repeats that version; refused if the running container's PHP differs from config.
- `standalone`: accepts `8.1`–`8.5` (defaults to all five). `7.4`/`8.0` rejected before any image work (`unsupported_php:`).

---

## Scanned Paths & Media Guard

Analyzers skip `vendor/`, `generated/`, `var/`, `pub/static/`, `pub/media/` (never shipped code). Because `pub/media` is where uploaded webshells land, every PHP version also runs a **media guard**: name-only scan of `pub/media` for `.php/.phtml/.pht`. Each hit is `M2-LINT-MEDIA` and fails the run — milliseconds, names only.

---

## Providers (`--lint-provider`)

| Value | Backend |
| :--- | :--- |
| `govard` (default) | Govard-native: embedded build context, pinned by digest. If pinned image can't be pulled or labels mismatch, Govard builds locally and continues. |
| `<external>` | Must name a key under `audit.lint.external_providers` in `.govard.yml` ([Configuration](/reference/configuration#audit-lint-providers)). Never a fallback; unknown name = error. |

Standalone modules have no project config, so only `govard` is available.

---

## Caching

Reusable state: `~/.govard/cache/audit/lint/<target-id>/` (survives `audit cleanup`). One generation per toolchain identity (image, runner, PHP matrix, analyzer policy).

- Changing `composer.json`/`composer.lock` or a ruleset (`phpcs.xml`, `phpstan.neon`, `*.dist`) discards analyzer state but keeps the Composer download cache warm.
- `--no-lint-result-cache` reports `bypassed` and keeps the download cache.

Evidence records per-PHP cache state (`cold`/`warm`/`bypassed`) + reason, image digest, toolchain digest, and per-phase timings.

Credentials: `~/.composer/auth.json` mounted read-only when present (linked into a private Composer home, never logged). `SSH_AUTH_SOCK` only forwarded with `--allow-lint-ssh-agent`. Source tree always read-only.

---

## Toolchain

Machine-wide lint image — no project needed, never calls external providers:

```bash
govard audit toolchain status  # local only — what to run next
govard audit toolchain pull    # only pinned official image, never builds
govard audit toolchain build   # only embedded context, never pulls
```

---

## Files

| Path | Content |
| :--- | :--- |
| `~/.govard/audit/<project-id>/sessions/<session-id>/manifest.json` | Session manifest |
| `~/.govard/audit/<project-id>/sessions/<session-id>/runs/<run-id>/audit-result.json` | Per-run evidence (atomic write) |
| `.../runs/<run-id>/report.json` | Provider-native report |
| `.../runs/<run-id>/artifacts/profiler/profile.csv` | Profiler CSV (when enabled) |
| `~/.govard/cache/audit/lint/<target-id>/` | Reusable lint cache |

→ Reference: [CLI Commands](/reference/cli-commands#govard-audit) · [Configuration](/reference/configuration#audit-lint-providers)
