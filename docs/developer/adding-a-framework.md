---
title: Adding a New Framework to Govard
description: How Govard's framework registry is structured internally, and a concrete, file-by-file walkthrough for adding support for a new framework.
---

# Adding a New Framework

Govard ships support for a growing list of frameworks — Magento 2, Mage-OS, Magento 1, OpenMage, Laravel, Symfony, Drupal, WordPress, Next.js, Emdash, Shopware, CakePHP, PrestaShop, Django, and more over time. This page documents how that support is structured internally, and what to touch to add a new one.

---

## The Registry: `internal/frameworks`

Each framework has one small package under `internal/frameworks/<name>/` that produces a `types.FrameworkDefinition` — a single struct that's the framework's identity card inside Govard:

```go
// internal/frameworks/types/definition.go
type FrameworkDefinition struct {
    Name        string   // canonical key, e.g. "magento2"
    Aliases     []string // e.g. "magento" -> "magento2"
    DisplayName string   // human-facing label, e.g. "Magento 2"

    Config   engine.FrameworkConfig         // PHP/Node version, nginx template, DB defaults...
    Manifest engine.FrameworkManifestConfig // sync excludes, sensitive tables, feature flags
    Detect   engine.DetectionSpec           // composer/package.json/auth.json/file-path signatures

    Bootstrap      BootstrapFactory              // func(bootstrap.Options) bootstrap.FrameworkBootstrap
    BaseURLManager func() tunnel.BaseURLManager  // nil if the framework needs no tunnel base-URL rewriting

    SupportsBootstrap    bool // allow `govard bootstrap` (remote/clone workflow)
    SupportsFreshInstall bool // allow `govard bootstrap --fresh`
}
```

`internal/frameworks/all.go`'s `init()` calls `Register(<pkg>.Definition())` for each registered framework package, in a specific order (more on why, below), populating a package-level registry that the rest of Govard reads through three small, focused files:

| File | Purpose |
| :--- | :--- |
| `internal/frameworks/registry.go` | `Get(name)`, `All()`, `Normalize(name)` — the registry itself, alias resolution |
| `internal/frameworks/run.go` | `RunBootstrap(name, opts)` — dispatches to `def.Bootstrap` instead of a switch |
| `internal/frameworks/base_url.go` | `NewBaseURLManager(name)` — dispatches to `def.BaseURLManager`, falls back to `tunnel.NoopManager` |

Everything that reads framework data by name — `govard bootstrap`'s allowlists, `govard tunnel`'s base-URL rewriting, the bootstrap dispatcher — goes through one of those three files instead of a hardcoded `switch framework { case "magento2": ... }`. Adding a framework to the registry means it automatically participates in all three, no switch to edit.

### What's *not* on the registry yet

Two things are still framework-name switches, on purpose, not by omission:

1. **`internal/cmd/bootstrap_fresh_install.go` / `bootstrap_remote.go`** — the actual fresh-install/clone *orchestration* (which shell commands run, in what order, with which `bootstrap.Options` fields populated). Six frameworks (Symfony, Laravel, Drupal, WordPress, Shopware, CakePHP) share one generic `CreateProject → Install → govard config auto` sequence via a small lookup table (`genericFreshInstallFrameworks` in `bootstrap_fresh_install.go`) — but OpenMage, Next.js, Emdash, and the Magento family each do materially different things (git clone vs. `composer create-project` vs. HTTP download; admin-user creation; `.env` generation; sample data), not just "call a different constructor." That can't be data-driven without either forcing frameworks into an ill-fitting common shape or restructuring how `internal/cmd` and `internal/frameworks` depend on each other. See the "Fresh-install orchestration" section below for what a new framework needs here.
2. **`engine.GetFrameworkConfig` / `engine.GetFrameworkManifestConfig`** (`internal/engine/framework_config.go`, `internal/engine/framework_manifest.json`) — the actual PHP/Node/DB version defaults and sync/manifest data. The registry's `Config`/`Manifest` fields are a read-through of these — `internal/engine` remains the authoritative data source; `internal/frameworks` composes it into one struct per framework alongside detection and dispatch.

---

## Adding a new framework: a checklist

Say you're adding a fictional framework called `whimsy`. Every step below has a real, working example already in the codebase — the file references point at the closest existing analog to copy from.

### 1. Runtime defaults — `internal/engine/framework_config.go`

Add a `FrameworkConfig` entry (PHP/Node version, nginx template, DB engine/version, includes list). Copy the closest existing framework's shape — e.g. `"cakephp"` for a vanilla PHP+MariaDB stack, `"nextjs"` for a Node-only one with no DB.

### 2. Manifest data — `internal/engine/framework_manifest.json`

Add an entry under `frameworks.whimsy`: sync excludes (`local_media`/`remote_media` paths, web-root candidates), sensitive/ignored DB tables, and the `features` block:

```json
"whimsy": {
  "paths": { "local_media": "public/uploads", "remote_media": "public/uploads", "web_root_candidates": [] },
  "features": {
    "requires_running_env_for_fresh_install": false,
    "supports_post_clone": true
  }
}
```

`requires_running_env_for_fresh_install` controls whether `govard bootstrap --fresh` starts containers *before* or *after* running `CreateProject` — see the gotcha about this below before setting it `true`.

### 3. Compose blueprint — `internal/blueprints/files/whimsy/`

A `services.yml` (Docker Compose fragment) rendered as a Go template — copy the closest analog (`internal/blueprints/files/nextjs/services.yml` for a Node runtime, `internal/blueprints/files/cakephp/` for PHP). Not every framework needs its own directory: Mage-OS reuses Magento 2's compose/nginx/Varnish blueprint outright (see `varnishTemplateFramework` in `internal/engine/render.go`) since it's a drop-in fork with the same runtime shape.

### 4. Bootstrap implementation — `internal/engine/bootstrap/whimsy.go`

Implement the `FrameworkBootstrap` interface (`internal/engine/bootstrap/base.go`):

```go
type FrameworkBootstrap interface {
    Name() string
    SupportsFreshInstall() bool
    SupportsClone() bool
    FreshCommands() []string        // human-readable summary, not necessarily what actually runs
    CreateProject(projectDir string) error
    Install(projectDir string) error
    Configure(projectDir string) error
    PostClone(projectDir string) error
}
```

Copy `internal/engine/bootstrap/cakephp.go` for a PHP framework using the shared `runStagedCreateProject` helper (`internal/engine/bootstrap/staged_project.go`), or `internal/engine/bootstrap/emdash.go` for a framework whose `CreateProject` doesn't need any container at all (plain HTTP download).

**If your framework needs to run a CLI tool (`npx`, `composer`, etc.) to scaffold the project, it must do so inside a container — never assume host tooling is present.** See the "Container execution" gotcha below; it's the single most important lesson from this system's history.

### 5. Wire it into the registry — `internal/frameworks/whimsy/whimsy.go`

```go
package whimsy

import (
    "govard/internal/engine"
    "govard/internal/engine/bootstrap"
    "govard/internal/frameworks/types"
)

func Definition() types.FrameworkDefinition {
    config, _ := engine.GetFrameworkConfig("whimsy")
    manifest, _ := engine.GetFrameworkManifestConfig("whimsy")
    return types.FrameworkDefinition{
        Name:        "whimsy",
        DisplayName: "Whimsy",
        Config:      config,
        Manifest:    manifest,
        Detect: engine.DetectionSpec{
            ComposerPackages: []string{"whimsy/framework"}, // or PackageJSONDeps, AuthJSONHosts, FilePaths
        },
        Bootstrap: func(opts bootstrap.Options) bootstrap.FrameworkBootstrap {
            return bootstrap.NewWhimsyBootstrap(opts)
        },
        SupportsFreshInstall: true,
        SupportsBootstrap:    true, // only if it supports the remote/clone workflow too
    }
}
```

Only set `BaseURLManager` if the framework needs specialized base-URL rewriting for `govard tunnel` (most don't — the default `tunnel.NoopManager` is a no-op, which is correct for anything that doesn't store its own base URL in the database or a config file).

### 6. Register it — `internal/frameworks/all.go`

Add the import and one `Register(whimsy.Definition())` call inside `init()`. **Position matters**: detection walks the registered frameworks in registration order and returns the first match, so a framework whose detection signature could also match another framework's must be registered in the right relative position. The existing comment in `all.go` documents the one known case (Emdash before Next.js, preserving a legacy detector's tie-break).

### 7. Fresh-install / clone orchestration — `internal/cmd`

This is the part that stays a switch (see "What's not on the registry yet," above):

- If `whimsy` fits the generic `CreateProject → Install → govard config auto` shape, add one line to `genericFreshInstallFrameworks` in `bootstrap_fresh_install.go` (a `map[string]struct{ needsDB, needsDomain bool }`) instead of writing a whole function.
- If it needs bespoke steps, add a `case "whimsy":` to `runBootstrapFrameworkFreshInstall`'s switch calling a new `runBootstrapWhimsyFreshInstall` function — copy `runBootstrapOpenMageFreshInstall` or `runBootstrapNextJSFreshInstall` as a starting shape.
- If it supports the remote/clone workflow (`SupportsBootstrap: true`, not just fresh-install), it's picked up automatically by `bootstrap_remote.go`'s post-clone dispatch (`bootstrapPostCloneDefinition`) — no switch to edit there unless it's part of the Magento family (which is handled by a separate, earlier branch keyed on `engine.IsMagento2Family`).

### 8. Docs

Add a row to the support/runtime-defaults tables and a short section in [`docs/reference/frameworks.md`](/reference/frameworks) (and the Vietnamese mirror, `docs/vi/reference/frameworks.md`).

### 9. Tests

- `tests/framework_detection_test.go` — a `TestWhimsyDiscovery` matching whatever `Detect` signature you used.
- `tests/framework_definitions_test.go` or a new `whimsy`-specific test file — assert `Definition()`'s `Config`/`Manifest`/`Bootstrap` are populated as expected.
- `tests/framework_snapshot_test.go` — this is a golden-snapshot regression net covering *every* registered framework automatically (blueprint rendering, `FreshCommands()`, resolved config/profile, manifest/DB-credential defaults) via `allFrameworkNames`; registering `whimsy` in `all.go` makes it start running, but its golden fixtures under `tests/testdata/framework_snapshots/whimsy/` won't exist yet. Generate them once you're confident the rendered output is correct:

  ```bash
  UPDATE_GOLDEN=1 go test ./tests/... -run TestFrameworkSnapshot
  ```

  Always review the generated fixture diff before committing it — `UPDATE_GOLDEN=1` writes whatever the code currently produces, correct or not.

### 10. Validate for real

Unit tests check that rendering and dispatch produce the *expected* output — they don't catch a container that can't actually reach the internet, an image that's missing a binary, or a race between a container starting and it actually being ready to serve traffic. Before considering the framework done, actually run it:

```bash
mkdir -p /tmp/whimsy-test && cd /tmp/whimsy-test
govard bootstrap --framework whimsy --fresh --yes
curl -sk -o /dev/null -w '%{http_code}\n' https://whimsy-test.test/   # expect 200, not a docker/proxy error
govard env down
```

This isn't a formality — every real bug found while building this registry (a Mage-OS auto-configuration step silently using Magento 2's DB credentials; Next.js's `CreateProject` depending on the host's npm install; a proxy-registration race against a container that wasn't ready yet) was invisible to `go test ./...` and only showed up by actually running the command and checking the result.

---

## Gotchas learned the hard way

### Container execution, not host execution

Every `FrameworkBootstrap.CreateProject`/`Install` that shells out to a CLI tool (`composer`, `npx`, `npm`) must run that tool **inside a container**, never via a bare `exec.Command` on the host. PHP frameworks do this via `bootstrap.Options.Runner` (a `func(command string) error` closure that `internal/cmd` wires to `runPHPContainerShellCommand`, exec'ing into the already-running PHP container). Next.js originally ran `npx create-next-app` directly on the host — this meant its fresh-install depended on whatever npm/node happened to be installed (and configured) on the *developer's machine*, completely outside Govard's control. On one real machine, a stray global `~/.npmrc` setting broke every Next.js fresh-install silently (the container came up, `govard bootstrap` reported success, but the app was never actually installed — `next: not found` at runtime).

The fix (`internal/cmd/bootstrap.go`'s `nodeCreateProjectRunner`) runs the scaffolding command in a throwaway `docker run --rm -v <projectDir>:/app node:<version> ...` container instead — independent of both the host environment and of whether any compose-managed service is running yet.

### Don't assume a compose service is up yet

A tempting alternative to the throwaway-container approach above is to exec into the framework's own long-lived "web" service container instead (matching the PHP pattern). This works *only if* that container is already running by the time `CreateProject` executes, which for most frameworks it isn't — `govard bootstrap --fresh` runs fresh-install *before* `env up` for anything whose manifest doesn't set `requires_running_env_for_fresh_install: true`. Flipping that flag to force env-up first introduces a subtler problem: the container starts running its normal long-lived command (e.g. `npm run dev`) against what is still an *empty* project directory, so it exits immediately — and the bootstrap pipeline's domain/proxy-registration step runs during that same window, registering a route to a container that isn't actually serving anything yet. The registration silently succeeds, but the reverse proxy never gets a working backend, and the very first `https://<project>.test/` request 502s until a manual `env down && env up`.

If a framework genuinely needs its long-lived service container up before `CreateProject` can run inside it, that container's startup command needs to tolerate an empty/partial project directory (loop waiting for a marker file, or similar) *and* the domain-registration timing needs to happen after the app is actually serving — not just after the container process started. Emdash sidesteps this entirely: its `CreateProject` needs no container (a plain HTTP tarball download), and its compose command already has an "install if `node_modules` missing" guard for defense, but never a "wait for files to exist" one, because by the time its container starts, the files are already there.

### Family membership isn't automatic

Mage-OS is a drop-in fork of Magento 2 and reuses most of its runtime behavior, but "reuses most of" is not "reuses all of" — DB credential defaults, the search-engine version gate, and the exec user are each their own decision point, and each one needs an explicit check. `engine.IsMagento2Family(framework)` / `engine.Magento2FamilyDisplayName(framework)` (`internal/engine/framework_family.go`) exist as the single place that decision lives, applied at every call site that used to check `framework == "magento2"` literally. When adding a framework that's a close variant of an existing one, grep for every `== "<existing-framework>"` string comparison in `internal/cmd` and `internal/engine` and decide, case by case, whether the new framework belongs on each check — don't assume "looks similar" implies "behaves identically everywhere." A real bug shipped exactly this way: Mage-OS's `setup:config:set` auto-configuration step used Magento 2's hardcoded `"magento"`/`"magento"`/`"magento"` DB credentials instead of Mage-OS's own `"mageos"`/`"mageos"`/`"mageos"`, because that one call site was missed when Mage-OS was added — caught only by actually running a fresh Mage-OS bootstrap end-to-end.

---

[Architecture](/developer/architecture) | [Contributing](/developer/contributing) | [Frameworks Reference](/reference/frameworks)
