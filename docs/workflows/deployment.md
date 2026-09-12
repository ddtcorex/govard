---
title: Deployment
description: Deploy a git revision to a remote with govard — the neutral task pipeline, framework recipes, hooks, the two-job CI shape with artifacts, rollback, and the sandbox that rehearses the whole thing on your machine.
---

# Deployment

`govard deploy` publishes one git revision to one remote environment. It needs
only SSH and rsync — no Docker, no local PHP, no other deploy tool — which is
what lets the same command run on a laptop and in a CI job.

```bash
govard deploy staging --revision <sha>   # deploy an exact commit
govard deploy staging                    # ... or the local HEAD
govard deploy plan staging               # print the plan, connect nowhere
govard deploy check staging              # preflight and report what the target implies
```

The target is a remote from `.govard.yml`. The branch, repository, deploy path
and publish strategy come from the project `deploy:` block; a remote overrides
them. `deploy_path` has no default, so a remote that omits it gets the layout the
target already has — `releases/`, `shared/`, `.dep/` or a `current` symlink — and
govard adopts it only when exactly one candidate matches, saying which. No layout
or several is a configuration error naming what was probed.

## The pipeline

Deployment is a fixed sequence of framework-neutral tasks, ordered by the engine
rather than by a recipe:

| Stage | Tasks |
| --- | --- |
| `prepare` | preflight, lock, release directory, code, shared files, permissions |
| `build` | dependencies, patches, code generation, frontend assets, static assets — or the artifact |
| `publish` | maintenance, workers, database backup, configuration, migrations, activation, caches, release record |

The maintenance window is opened only when it buys something: a symlink
activation is an atomic rename, so nothing serving the site is rewritten, and the
window appears only if the same plan also migrates or imports configuration. An
in-place activation always opens it, because the docroot itself is rewritten
while serving.
| `verify` | post-publish checks |
| `cleanup` | prune old releases, release the lock |

A framework contributes a **recipe** that fills the tasks it supports; anything
it leaves empty is reported as skipped, not as a failure. A project customises
the pipeline by anchoring **hooks** on a task id, a stage alias or another hook:

```yaml
deploy:
  hooks:
    - { name: varnish-purge, on: "publish:activate", position: after, order: 10, run: "varnishadm ban req.url ~ /" }
```

`govard deploy plan` prints the resolved tree with each step's source and
implementation, so a hook's placement can be reviewed without connecting to
anything.

Every enhanced behaviour is behind a flag and the defaults are the optimised
ones: `--no-verify`, `--no-db-backup` and `--lock=false` turn work off, `--force`
re-deploys a revision the target already runs, and `--build`, `--artifact-dir`
and `--publish` change how the release is produced and published.

`deploy.settings` is validated against the recipe before anything runs: a key the
framework does not know, or a value with the wrong shape, is a configuration error
(exit 4) that names the key and suggests the near miss. A misspelling used to be a
silent no-op — `static_content_locale: en_US` deployed every locale the recipe
defaulted to. String values must be quoted if they look numeric (`php_version:
"8.2"`): the engine reads those settings as strings, so an unquoted `8.2` reads as
empty.

### The static content split

`split_static_deployment` deploys the adminhtml and frontend static content in two
passes instead of one, with `--area=adminhtml` then `--area=frontend`. The admin
pass uses `magento_themes_backend` (the admin theme by default) and
`static_content_locales_backend`, which defaults to the frontend languages so the
two passes agree unless the project says otherwise. The frontend pass uses
`magento_themes` and `static_content_locales`.

It exists for the two cases a single pass handles badly: a theme list that covers
only frontend themes (the admin theme would be missed) and a large deployment
where one process holding every area runs out of memory. The second pass is
chained to the first, so a failed admin pass stops the deploy rather than
publishing half of the static content.

### Frontend builds (Hyvä)

A Hyvä theme is built by Node inside its own directory, which the project names
with `settings.frontend_dir`:

```yaml
deploy:
  settings:
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
    frontend_command: npm ci && npm run build   # the default
```

`frontend_command` runs inside `frontend_dir`, inside the release, as one shell
command: the default chains two commands, so a single-command value such as
`npx tailwindcss -i input.css -o output.css` is written exactly the same way.
Leaving `frontend_dir` empty skips the step, which is what a Luma or stock-theme
project wants.

Node is needed where the *build* runs, not where the deploy runs: with
`--artifact-dir` the theme is built in the CI build job and `build:frontend` is
skipped on the target, so the deploy job's image stays govard + ssh + rsync.

### Private Composer repositories

A release built on the target installs its dependencies there, so it needs
credentials for any repository that is not packagist. `COMPOSER_AUTH` from the
environment is forwarded to the dependency step — on its standard input, not in the
command, so it stays out of the target's process list and out of the deploy log —
and a `shared/auth.json` on the target works as it does for the other deploy tool.
Govard stores neither.

`govard deploy check` reports which source is in play, and warns when the project
declares a private repository and neither is available. It warns rather than
refuses: the declaration it can read is a URL, and a URL cannot tell it whether a
repository needs credentials. Set `COMPOSER_AUTH` in the CI build job as well as in
the deploy job when the artifact is built there.

## Build modes

`--build=auto` (the default) resolves **by presence**, never by sniffing the
environment: an artifact directory means the build already happened, otherwise
the target builds.

| Mode | Where the build runs | Use it when |
| --- | --- | --- |
| `server` | on the target | a hotfix from a laptop, or a project with no CI |
| `artifact` | on the machine running `govard deploy build` | CI, so the deploy job needs no toolchain |

### The two-job CI shape

```yaml
build:
  stage: build
  script:
    - govard deploy build production --output artifacts --revision $CI_COMMIT_SHA
  artifacts:
    paths: [artifacts/]

deploy:
  stage: deploy
  script:
    - govard deploy production --artifact-dir artifacts --revision $CI_COMMIT_SHA --yes
```

The `deploy` job's image needs govard, SSH and rsync and nothing else: no PHP, no
Composer, no Node, no container runtime. The five build tasks are skipped on the
target in this mode — the artifact carries what they generate — so nothing on the
server runs `composer install`, `setup:di:compile` or
`setup:static-content:deploy`. The artifact is uploaded into the release and its
manifest is checked against the files themselves, the revision being deployed and
the PHP the target runs — a CI image that does not match the server, or an
artifact whose bytes changed after the build, is refused before anything is
published. `govard deploy plan --artifact-dir artifacts` shows which branch of
the build stage is in effect.

An output directory that is not empty is refused, so a file left over from an
earlier build cannot ship. Pass `--force` to replace its contents.

## Publishing

`--publish=auto` reads the target instead of guessing:

- the current path is missing or a symlink → **symlink**: releases live in
  `releases/<n>` and the swap is an atomic `mv -T`, so no visitor ever sees a
  half-published tree;
- the current path is a real directory → **in_place**: the docroot's objects are
  fetched during `prepare`, outside any maintenance window, then the docroot is
  reset to the exact revision, the configured paths are copied in, and the static
  content version file is written last.

`deploy check` reports which one the layout implies and why.

## Verification, backup and rollback

`deploy:verify` runs after publish and is on by default: the live revision
(the resolved `current` symlink, or the docroot's `HEAD` for an in-place target),
the shared files the recipe requires, the recipe's own checks, and an HTTP check
when `deploy.verify.url` is set. The recipe's checks are the ones the engine
cannot supply — for Magento that is `bin/magento setup:db:status`, which needs a
working `app/etc/env.php` *and* a reachable database, plus a comparison of the
docroot's static content version against the release when publishing in place.

Without a configured URL verification is SSH-only: it proves the right files are
in place, not that the application serves. A deploy that runs `db:migrate` with
no verify URL says so before its first step, rather than leaving the operator to
assume the opposite.

`--db-backup` dumps the database into `shared/backups/deploy/<n>/` immediately
before the first database-mutating task and records the path in the release.
`deploy:cleanup` prunes those dumps on the same `keep_releases` window as the
releases they belong to, so backups cannot accumulate forever on a production
box.

```bash
govard deploy releases staging                  # what is on the target
govard deploy status                            # what every environment serves
govard deploy rollback staging                  # put the previous release back
govard deploy rollback staging --to 12          # ... or a named one
govard deploy rollback staging --with-db --yes  # ... and its database dump
```

Rollback never rebuilds: a symlink layout is re-pointed, and an in-place layout
re-runs the publish tail from the release directory already on the server.

`deploy.lock_stale_after` (default 2h) is how old a lock must be for
`govard deploy unlock` to release it without `--force`; the refusal a held lock
produces names the holder, its revision and how long it has been held.

`deploy.maintenance_timeout` (default 15m) bounds any single step that runs with
the site in maintenance mode, which is far below `deploy.command_timeout` on
purpose: a slow step there is holding the site down. Raise it for a project whose
database dump legitimately takes longer.

A failed deploy keeps its release directory and its record. Where it failed
decides the lock: a failure in `prepare` or `build` releases it, because nothing
live has changed, so the fault can simply be fixed and the deploy retried — while
a failure from `publish` onwards keeps it, because the target may be
half-changed, and the way forward is `govard deploy <remote> --resume`, which
continues the newest unfinished release instead of starting a new one.
`--from <task>` starts at a named task or hook, and `govard deploy unlock`
releases a lock a failed run left behind.

## Machine-readable output

`--json` writes exactly one JSON document to stdout and everything a human would
read to stderr, so a CI job can pipe stdout into a parser and keep stderr in the
log:

```json
{"schema_version":1,"remote":"production","branch":"main","revision":"0123abc…",
 "release":"42","build":{"mode":"server"},"publish":{"strategy":"symlink"},
 "verify":"ok","result":"ok","duration_ms":94300,
 "tasks":[{"id":"deploy:check","stage":"prepare","status":"ok","duration_ms":19}]}
```

A failing deploy produces the same document with `"result":"failed"` and an
`"error"` that names the task, host and command; the process exits 1. The release
record carries `ci.pipeline`/`ci.job` when the run is a CI run, which is how "which
pipeline deployed this" is answered afterwards from `govard deploy status`.

## The sandbox

`govard deploy sandbox` gives a project a real deployment target on your
machine — a container that plays the remote — so a deploy can be rehearsed
before it touches a server. Nothing in the pipeline knows the difference, which
is what makes it a rehearsal rather than a simulation.

```bash
govard deploy sandbox up                      # create it (php profile by default)
govard deploy sandbox up --profile basic      # sshd, rsync, git only
govard deploy sandbox status
govard deploy sandbox reset --layout deployer # seed a target the other tool owns
govard deploy sandbox ssh
govard deploy sandbox down [--purge]
```

`up` publishes SSH on a free loopback port, generates a dedicated key under
`.govard/sandbox/` (gitignored), mounts a mirror of your local repository
read-only, and writes a `sandbox` remote into `.govard.local.yml`. The mirror is
refreshed before every deploy, so a commit you have never pushed is deployable.
Because `sandbox` is also a subcommand, deploy to it with the flag form:

```bash
govard deploy --remote sandbox --yes
```

`--docroot` shapes the target so the publish strategy resolves the way you want
to exercise it: `absent` or `symlink` selects the atomic swap, `real` selects
in-place publishing. `down` removes the container and the remote it wrote;
`--purge` also removes the image, the key and the mirror.

## Capabilities and exit codes

Every deploy command declares what it needs, and a missing requirement is exit
`3` with an actionable message before any work happens:

| Command | Requirement |
| --- | --- |
| `govard deploy` / `rollback` | `ssh,rsync` |
| `govard deploy check` / `releases` / `status` / `unlock` | `ssh` |
| `govard deploy plan` / `build` | `none` |
| `govard deploy sandbox *` | `docker` |

Exit codes: `0` success, `1` execution failure, `2` usage, `3` missing
capability, `4` configuration. The deploy job in CI therefore runs on a host
with nothing but govard, SSH and rsync.
