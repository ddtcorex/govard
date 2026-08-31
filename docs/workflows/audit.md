---
title: Project Audit — Lint & Profiler
description: Run Govard's persistent lint and profiler audits — Govard-native backend, external providers, caching, and target modes.
---

# Audit

Govard persists every audit as an immutable session under `~/.govard/audit/<project-id>/sessions/<session-id>/`. Use it to lint Magento code, catch webshells, and capture the stock Magento CSV profiler without editing `env.php`.

---

## Quick Start

```bash
# Lint the whole project (auto-detect mode) — supported for Magento 2, Laravel, Symfony, WordPress
govard audit run --checks lint               # Magento2
govard audit run --checks lint --mode project # Laravel/Symfony/WordPress (auto resolves)

# Lint + profiler on one URL (first run needs --url) — profiler is Magento-only
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

## Lint Matrix

Govard-native lint (`govard audit run --checks lint --mode project`) matrix:

| Framework | CodingStandard | PHPStanLevel | ProjectPHPVersions | StandalonePHPVersions | Linters |
| :--- | :--- | :---: | :--- | :--- | :--- |
| Magento 2 | Magento2 | 5 | 8.1-8.4 (policy) | 8.1-8.5 | phpcs, phpstan |
| Laravel | PSR12 | 5 | 8.1-8.4 | 8.1-8.4 | phpcs, phpstan |
| Symfony | Symfony | 5 | 8.1-8.4 | 8.1-8.4 | phpcs, phpstan |
| WordPress | WordPress | 5 | 8.1-8.4 | 8.1-8.4 | phpcs, phpstan |

Image `govard-magelint` (now `glint`, `docker/audit`, `govard-local/glint:` — `magelint` kept as symlink for compat, `ghcr.io/ddtcorex/govard-magelint` unchanged) bundles WPCS 3.1 (`wp-coding-standards/wpcs`) + Symfony CS + `phpstan-symfony`/`phpstan-wordpress` so WordPress (classic `wp-includes/version.php` + Bedrock `web/wp`) and Symfony (`bin/console`) run natively — no fallback to PSR12. Laravel excludes `bootstrap/cache/*` and `storage/*` (`storage/framework/*`, `storage/logs`) — filtered in `docker/audit/bin/glint` (`--ignore` + `excludePaths`) and Go-side `govardLintDigest` filter, so fresh `laravel 11` no longer fails on generated `packages.php`/`services.php`/`storage/framework/views/*.php`.

## PHP Versions (`--php`)

Lint image provides `7.4`, `8.0`, `8.1`, `8.2`, `8.3`, `8.4`, `8.5`.

- `project` / `module_in_project`: exactly one version — the project's active `stack.php_version`. Passing `--php` only accepted when it repeats that version; refused if the running container's PHP differs from config.
- `standalone`: accepts `8.1`–`8.5` (defaults to all five). `7.4`/`8.0` rejected before any image work (`unsupported_php:`).

---

## Scanned Paths & Media Guard

Analyzers skip `vendor/`, `generated/`, `var/`, `pub/static/`, `pub/media/` (never shipped code). Because `pub/media` is where uploaded webshells land, every PHP version also runs a **media guard**: name-only scan of `pub/media` for `.php/.phtml/.pht`. Each hit is `M2-LINT-MEDIA` (`PHP file in pub/media`) and fails the run — milliseconds, names only. Guard phase is `media-guard` in the per-PHP `phases` array (`failed` when found, `passed` otherwise). Hygiene also blocks commits via `.gitignore`:

```
pub/media/*.php
pub/media/**/*.php
pub/media/*.phtml
pub/media/**/*.phtml
pub/media/*.pht
pub/media/**/*.pht
```

See `internal/blueprints/files/.gitignore` (shared, single source; rendered as project-root `.gitignore`); `govard audit run --checks lint` is the enforcement gate (container `media-guard` phase plus host `ScanMediaGuard` fallback so every provider enforces `M2-LINT-MEDIA`).

---

## Providers (`--lint-provider`)

| Value | Backend |
| :--- | :--- |
| `govard` (default) | Govard-native: embedded build context, pinned by digest. If pinned image can't be pulled or labels mismatch, Govard builds locally and continues. |
| `<external>` | Must name a key under `audit.lint.external_providers` in `.govard.yml` ([Configuration](/reference/configuration#audit-lint-providers)). Never a fallback; unknown name = error. |

`--provider` is a hidden alias for `--lint-provider`. Standalone modules have no project config, so only `govard` is available.

## Scope (`--scope diff --base auto`)

- `govard audit run --scope project` (default) audits the full target.
- `govard audit run --scope diff --base auto --format json` is for review/PR workflows: it auto-detects the base via `git merge-base HEAD origin/HEAD || origin/master || gh pr view --json baseRefName` (merge-base returns a commit SHA) and records it in the session manifest. `lint` then runs only on the changed files (`git diff --name-only --diff-filter=ACMRT <base>...HEAD` plus staged/unstaged vs `HEAD`, filtered to `php/phtml` under the target via `diff-files.txt` mounted as `GOVARD_LINT_DIFF_FILE`); an empty diff of relevant files short-circuits as `passed` with `diff-empty` cache without launching the container. `gh pr view` normalizes to `origin/<branch>` when the prefix is missing. `govard audit diff --base <ref>` is a shorthand that forces `scope diff`.

## Concurrency

Runs for the same project are queued via `~/.govard/audit/<projectId>/lock` (under `GovardHomeDir`, respecting `GOVARD_HOME_DIR`; wait up to 30s, `audit run waiting for prior run`), not cancelled. Since `v1.68.0` a stale lock is auto-removed if `mtime>10m` or holder PID dead (`syscall.Signal(0)`), otherwise after 30s fails with a hint to remove the lock or run `govard audit cleanup`.

## Xdebug guard

With `stack.features.xdebug: true` the audit hard-fails unless `--allow-xdebug` is set (`Xdebug enabled, ~10-20% tax; disable with govard config set stack.features.xdebug false or --allow-xdebug`).

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
