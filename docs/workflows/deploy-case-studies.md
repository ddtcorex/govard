---
title: Deployment case studies
description: Concrete govard deployment setups for Magento projects — Luma, Hyvä, multiple themes and store views, developer and production mode, a symlinked or a real webroot — each with the configuration, the sandbox rehearsal and the failure it prevents.
---

# Deployment case studies

[Deployment](/workflows/deployment) describes the engine: the neutral pipeline, the
recipe, the two build modes, publish, verify and rollback. This page is the other
half — what a **specific project** puts in `.govard.yml`, how to rehearse exactly
that project in the sandbox, and what a green run and a red run each look like.

Every case here is a real shape, in the order a team usually meets them: a stock
Luma store, a Hyvä storefront, several storefronts in one release, and the two
webroot layouts a target can already have. Read the first section, find your row in
the table, then read your case.

## The three decisions that define a case

Nothing else in the file matters as much as these three answers.

### 1. Where the build runs

| `--build` | Who runs the build tasks | Use it when |
| --- | --- | --- |
| `server` | the target, over SSH | a hotfix from a laptop; a project with no CI |
| `artifact` | the machine running `govard deploy build`, in CI | the job that touches production must have no toolchain |

`auto` (the default) resolves **by presence**: an artifact directory means the build
already happened, otherwise the target builds. It never sniffs the environment.

The recipe marks the steps that need the deployed application — for Magento,
`setup:static-content:deploy` is the one that matters, because it reads the store,
website and locale configuration out of the database. Those steps are **left in the
deploy** in artifact mode and run on the target after `deploy:artifact` unpacks the
artifact. So:

| Runs on the build machine (artifact mode) | Runs on the target (both modes) |
| --- | --- |
| Composer install, patches, DI compile, frontend (Node) build | static content, `setup:upgrade`, configuration import, cache flush, verification |

A build machine does not need a database, a web server or `app/etc/env.php`; the
deploy job needs govard, SSH and rsync and nothing else.

### 2. How the release becomes live — the webroot shape

`remotes.<name>.path` is the **docroot that is served**. `remotes.<name>.deploy_path`
is the **layout root** that holds `releases/`, `shared/` and `.dep/`. Those are two
different directories, and which kind the docroot is decides the whole publish path:

| The served docroot is… | Resolved strategy | What activation does |
| --- | --- | --- |
| absent (first deploy) | `symlink` | creates the docroot as a symlink to `releases/<n>`, atomically |
| a symlink | `symlink` | re-points it with `mv -T` — no visitor ever sees a half-published tree |
| a real directory | `in_place` | `git reset --hard` in the docroot, copies `sync_paths` in, writes the static version file last |

This is "symlink webroot" versus "no symlink webroot" below. `auto` reads the target
rather than guessing; `publish: symlink` / `publish: in_place` on the remote
overrides it for a target the probe cannot classify (for example a docroot that is a
plain directory but is not a git checkout).

::: warning The in-place webroot is the one that needs `sync_paths`
`git reset --hard` leaves the previous deployment's gitignored directories exactly
where they are. A path that is not copied is a path the site keeps from the **old**
release — new code over the old `vendor/`, `generated/` and `pub/static/`, reported
as a successful deploy. The Magento recipe ships the list
(`vendor`, `generated`, `pub/static/adminhtml`, `pub/static/frontend`); a project
that overrides it owns that consequence.
:::

### 3. Which mode the target runs in

`mage_mode` **describes** the target; it does not change it. Govard never runs
`bin/magento deploy:mode:set`, and nothing else in the pipeline reads this setting —
it is a deploy setting, not a local environment setting, and its only effect is on
the static content step:

| `mage_mode` | `build:assets` runs | Meaning |
| --- | --- | --- |
| not set (default) | yes | production behaviour: every configured theme and locale is compiled at deploy time |
| `production` | yes | the same, stated explicitly — the mode the target actually runs |
| `developer` | no | the target generates static files on demand, so deploying them is wasted work and a stale-asset risk |

The guard is a plain string comparison, so an empty value behaves exactly like
`production`. Only the literal `developer` skips the step. On a developer-mode
target the compiled assets are produced by the application on first request, which
is why a developer-mode deploy of a large storefront finishes in minutes rather
than in the quarter of an hour a production static content deploy costs.

::: warning The setting assumes the mode; it does not set it
Two consequences follow, and both are silent:

- **Setting `developer` on a target that runs in production does not make the target
  a developer target.** Its `env.php` keeps generating nothing on demand, so the
  theme's static files were never deployed and are not produced on request — the
  storefront answers `404` for them. Check what the target actually runs with
  `bin/magento deploy:mode:show` on it, and set `mage_mode` to match.
- **A value that is not exactly `developer` deploys static content.** Validation
  requires the setting to be a string, not one of a closed set, so a typo such as
  `Development` is accepted and behaves like production.
:::

::: info Which is right for which environment
A staging target that a team browses and debugs is usually `developer`. A production
target is `production` (or unset). A **shared** staging target that performance is
measured on should be `production`, because on-demand generation changes the
numbers. `govard deploy plan` shows which one the guard will compare: the rendered
`build:assets` command carries it, for example `[ developer != developer ]` on a
developer-mode target.
:::

## Choosing a case at a glance

Pick the row that matches the project you are deploying. "Webroot" is the served
docroot's shape, per decision 2.

| # | Project | Webroot | `mage_mode` | `frontend_dir` | Themes | Build |
| --- | --- | --- | --- | --- | --- | --- |
| [1](#case-1-luma-production-mode-symlinked-webroot) | Luma, one store view | symlink | production | (empty) | all | server |
| [2](#case-2-luma-production-mode-real-webroot) | Luma, inherited target | real | production | (empty) | all | server |
| [3](#case-3-hyva-one-theme-production-mode) | Hyvä, one theme | symlink | production | one path | that theme | server |
| [4](#case-4-hyva-developer-mode) | Hyvä, debugging target | symlink | developer | one path | that theme | server |
| [5](#case-5-two-node-built-themes) | Two Hyvä-style themes | symlink | production | two paths | both | server |
| [6](#case-6-multi-store-hyva-storefront-with-a-luma-admin) | Hyvä + Luma admin, several locales | symlink | production | one path | map | server |
| [7](#case-7-artifact-mode-in-ci) | any of the above | either | production | as above | as above | artifact |
| [8](#case-8-the-in-place-target-you-inherited) | any | real | production | as above | as above | either |

The sandbox command for each row is the same shape: build the target, then deploy
to the remote the sandbox writes. The per-case sections give the exact commands.

## Case 1: Luma, production mode, symlinked webroot

The default shape, and the one to start from: a stock storefront, one locale, a
target whose docroot is a symlink into the release layout.

**Project shape.** `Magento/luma` (or a child theme of it), no Node build step,
`stack.web_root: /pub`, one store view.

**Configuration.** Almost nothing is required — the recipe's defaults are this
case. State it anyway, so the file says what the deploy assumes:

```yaml
project_name: acme-shop
framework: magento2

stack:
  web_root: /pub            # the sandbox's nginx serves <current>/pub because of this

deploy:
  settings:
    mage_mode: production
    static_content_locales: [en_US]

remotes:
  production:
    host: m2.example.com
    user: m2-deploy
    path: /home/m2-deploy/public_html     # the served docroot: a symlink
    deploy:
      path: /home/m2-deploy/.deployer     # releases/, shared/, .dep/
      settings:
        php_bin: php8.3
        composer_bin: composer
        php_version: "8.3"
        owner: m2-deploy:m2-deploy
        writable_mode: chmod+chown
      verify:
        url: https://shop.example.com/
    auth:
      method: keyfile
      key_path: ~/.ssh/m2-production
```

**What runs where.** `server`: on the target, `composer install --no-dev`, DI
compile, then `setup:static-content:deploy` for `en_US` across the themes, then
`setup:upgrade`, `app:config:import`, `cache:flush`. `build:frontend` runs and does
nothing (no `frontend_dir`). Publish is a symlink swap; the maintenance window opens
because the plan imports configuration and migrates.

**Rehearse it.**

```bash
govard deploy sandbox up --profile full --php 8.3   # DB + cache + web tier
govard deploy sandbox status
govard deploy --remote sandbox --yes
govard deploy releases sandbox
```

Use `--profile full`: a Luma deploy reaches `setup:upgrade` (needs the database and
a supported search engine) and `cache:flush` (needs the cache backend the target's
`env.php` names). `basic` has no PHP or Composer at all, so a Magento project stops
at `build:vendors`; `php` has the toolchain but no database, so it stops at
`db:migrate`.

**What to expect.** A cold first deploy is the slow one: Composer downloads
everything, DI compile scans the whole codebase, and static content is compiled per
theme and locale. On a real 2.4.9 project of moderate size the production-mode
static pass alone is several minutes. What makes a later deploy cheaper is that the
target keeps its Composer cache between releases and `setup:upgrade` runs with
`--keep-generated`, so the dependency step downloads less. Two measured runs of this
project: a developer-mode server build finished in 2m35s, and a production-mode
deploy that received an artifact took 14m35s — and the difference is dominated by the
static content pass, which the developer-mode run skips entirely.

**What goes wrong.**

| Message | Meaning |
| --- | --- |
| `The default website isn't defined` | the target's database has no store configuration; the target is not an installed application yet |
| `Your current search engine, 'MySQL', is not supported` | `setup:upgrade` on a project configured for Elasticsearch/OpenSearch with no cluster reachable |
| `Connection "default" is not defined` | `shared/app/etc/env.php` is missing or was replaced by an artifact's own copy |
| `setup:db:status` fails at verify, after a green publish | the release is in place but the application cannot answer — `env.php` or the database, not the deploy |

## Case 2: Luma, production mode, real webroot

The same application, on a target somebody already deploys to by hand: the served
directory is a real directory and a git checkout, not a symlink.

**Project shape.** Identical to case 1. The difference is entirely on the server.

**Configuration.** The remote describes it; add the one setting the strategy needs:

```yaml
remotes:
  legacy-staging:
    host: staging.example.com
    user: deploy
    path: /var/www/shop                # a REAL directory, served directly
    deploy:
      path: /var/www/shop              # releases/ and shared/ live under it
      settings:
        php_bin: php8.2
        composer_bin: composer
        php_version: "8.2"
        owner: www-data:www-data
        writable_mode: acl
        # The in-place list. Keep the recipe's entries and add what this project
        # builds under a gitignored path.
        sync_paths: [vendor, generated, pub/static/adminhtml, pub/static/frontend]
```

`publish: in_place` is not needed when the docroot really is a directory —
`auto` resolves it. Name it explicitly if the directory is not a git checkout and
you still want in-place behaviour, so a future layout change cannot silently switch
the strategy.

**What runs where.** `prepare` fetches the revision outside the window; then
`publish:activate` resets the docroot to it, copies `sync_paths` with `--delete`,
restores the shared links and writes `pub/static/deployed_version.txt` **last**. The
maintenance window opens for the whole activation — the docroot is rewritten while it
is being served, so there is no moment to be clever about.

**Rehearse it.** The sandbox can build exactly this shape:

```bash
govard deploy sandbox up --profile full --php 8.2 --docroot real
govard deploy check sandbox          # expect: publish in_place
govard deploy plan sandbox           # expect: the in-place branch of publish
govard deploy --remote sandbox --yes
```

`--docroot real` seeds the docroot as a checkout of the mirror — the state a target
that has never been deployed to is actually in. That matters: the maintenance guard
asks whether the *served application* can run (`bin/magento` **and**
`vendor/autoload.php`), so a checkout with no dependencies is correctly treated as
having nothing to protect, and the deploy does not die at `maintenance:enable`.

**What to expect.** The in-place deploy is not slower than a symlink deploy, but it
is less forgiving: the same `pub/static` is being rewritten under live traffic,
which is why the window exists.

**What goes wrong.**

| Message | Meaning |
| --- | --- |
| The site serves new PHP with old assets | a path the release built is missing from `sync_paths` |
| `deployed_version.txt` mismatch at verify | the docroot's static content is not the release's — the sync copied the wrong set, or `build:assets` was skipped while the check expected it |
| The deploy refuses with "not a git checkout" | in-place was selected for a directory that is not one; fix the layout or name the strategy |

## Case 3: Hyvä, one theme, production mode

A Hyvä storefront adds exactly one new thing to case 1: a Node build **inside the
theme's Tailwind directory**, run on the build machine.

**Project shape.** A Hyvä theme at
`app/design/frontend/Acme/hyva`, with `package.json` inside
`app/design/frontend/Acme/hyva/web/tailwind`.

**Configuration.** Add the theme's directory and keep everything else:

```yaml
deploy:
  settings:
    mage_mode: production
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
    frontend_command: npm ci && npm run build     # the default; state it if you rely on it
    static_content_locales: [en_US, fr_CA]
    magento_themes:
      Acme/hyva: [en_US, fr_CA]
```

`frontend_command` runs **inside** each directory, inside the release, as one shell
command. The default chains two commands; a single-command value such as
`npx tailwindcss -i input.css -o output.css` is written exactly the same way.

**What runs where.** With `--build=server`: `build:frontend` runs on the target —
so the target needs Node and the theme's `node_modules`-capable environment. With
`--build=artifact`: the same step runs in CI, and the target skips it entirely. That
is the practical reason to prefer artifact mode for a Hyvä project: **Node belongs
where the build runs**, not on the production server.

**Rehearse it.**

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy plan sandbox            # build:frontend must show your theme path
govard deploy --remote sandbox --yes
```

`--profile full` ships Node, so the frontend step can run on the target the way a
server build would. If you want to prove the artifact path instead, build the
artifact on your machine and deploy it — the sandbox then never runs Node:

```bash
govard deploy build sandbox --output /tmp/acme-artifact
govard deploy --remote sandbox --artifact-dir /tmp/acme-artifact --yes
```

**What to expect.** `build:frontend` prints the theme's own npm output. It is a Node
build like any other: quick on a warm `node_modules`, slower when the theme's
dependencies have to be installed first. `build:assets` afterwards is the long one.

**What goes wrong.**

| Message | Meaning |
| --- | --- |
| `npm ci` fails with a missing lockfile | the theme directory has no committed `package-lock.json`; `npm ci` requires one |
| The frontend step succeeds and the storefront looks unstyled | `frontend_dir` names the wrong directory (a common slip: the theme root instead of its `web/tailwind`) |
| The deploy is green but the CSS is stale | the artifact was built from a different revision, or `--force` reused an old output directory |

## Case 4: Hyvä, developer mode

The same project pointed at a target a team browses and debugs.

**Configuration.** One line changes:

```yaml
deploy:
  settings:
    mage_mode: developer
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind
```

**What runs where.** `build:assets` is a no-op: the guard compares `developer`
against `developer` and skips the whole block, split or not. Magento then generates
static files on demand as the application is browsed. Everything else — Composer,
DI compile, the frontend Node build, `setup:upgrade`, `app:config:import`,
`cache:flush`, verify — still runs.

**Rehearse it.**

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
govard deploy releases sandbox        # confirm what went live
```

**What to expect.** Materially faster than case 3 on the same project: the static
pass is skipped. The first request to a page after the deploy is slower while
Magento compiles what it needs.

**What goes wrong.**

| Message | Meaning |
| --- | --- |
| A first page load takes seconds | the application is generating static content; this is the mode working, not a fault |
| `mage_mode: "developer"` has no effect | the value was quoted in a way that changed it, or it was set on the local environment instead of under `deploy.settings` — check `govard deploy plan` |
| The site is unstyled on a *production* target | `developer` was deployed to production; static content was never built, and production does not generate it on demand unless `env.php` sets `static_content_on_demand_in_production` |

::: warning `developer` on a production target is a misconfiguration
The mode is not a performance switch you can leave on because "the site still
works". A production `env.php` normally has
`static_content_on_demand_in_production` off, so a theme whose static files were
never deployed answers `404` for them.
:::

## Case 5: Two Node-built themes

Two storefronts (or a Hyvä theme plus a custom one), each with its own Tailwind
directory. The recipe builds **every** configured directory, in order, each in its
own subshell; the first failure stops the deploy rather than letting the second
theme's build hide it.

**Configuration.**

```yaml
deploy:
  settings:
    mage_mode: production
    frontend_dir:
      - app/design/frontend/Acme/hyva/web/tailwind
      - app/design/frontend/Acme/outlet/web/tailwind
    magento_themes:
      Acme/hyva: [en_US, fr_CA]
      Acme/outlet: [en_US, en_GB]
```

Every entry is quoted, so one list entry is exactly one directory. A path that
contains a space **has to** be a list entry: the single-path form is read as a
whitespace-separated list.

**The locale rule.** The locales of a theme map are **added to**
`static_content_locales`, and the union is deployed for every theme in the map. That
is deliberate, and it is a property of the application rather than a simplification:
one `setup:static-content:deploy` invocation resolves `--language` once for the whole
run, so a single invocation cannot compile theme A for one set of locales and theme
B for another. Here both storefronts get `en_US`, `fr_CA` and `en_GB`. Narrowing per
theme would mean one invocation per locale group; the union is what the command can
express, and it is the safe direction — an extra locale costs build time, a missing
one costs a storefront.

**Rehearse it.**

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy plan sandbox            # both directories must appear in build:frontend
govard deploy --remote sandbox --yes
```

**What to expect.** Both npm builds run, then one static content pass covering both
themes and the union of locales. The build time grows with the number of themes, and
the static pass grows with themes × locales.

**What goes wrong.**

| Message | Meaning |
| --- | --- |
| Only the first theme is built | `frontend_dir` is a single string containing two paths — it is read as a whitespace-separated list, and only a YAML list is a list |
| A theme is missing its CSS but the deploy is green | that theme is not in `frontend_dir` (its Node build never ran) |
| A store view 404s its static files | that store view's theme or locale is not in the deploy set — check the map against `setup:static-content:deploy` output |

## Case 6: Multi-store, Hyvä storefront with a Luma admin

One release serving several websites, with the adminhtml and frontend static content
deployed in two passes.

**Configuration.** The split exists for two cases a single pass handles badly: a
theme list that covers only frontend themes (the admin theme would be missed) and a
large deployment where one process holding every area runs out of memory.

```yaml
deploy:
  settings:
    mage_mode: production
    frontend_dir: app/design/frontend/Acme/hyva/web/tailwind

    # Frontend: the storefront theme, for every locale the stores serve.
    magento_themes:
      Acme/hyva: [en_US, fr_CA, de_DE]
    static_content_locales: [en_US]

    split_static_deployment: true
    # Adminhtml: defaults already cover it; state them if this project differs.
    magento_themes_backend: [Magento/backend]
    static_content_locales_backend: [en_US]

    worker_control: true          # cron:remove / queue:consumers:stop around the migration
```

The admin pass uses `magento_themes_backend` (the admin theme by default) and
`static_content_locales_backend`, which defaults to the frontend languages so the
two passes agree unless the project says otherwise. The frontend pass uses
`magento_themes` and `static_content_locales`. The second pass is chained to the
first, so a failed admin pass stops the deploy rather than publishing half of the
static content.

**What the rest of the pipeline already covers for several websites:**

- `app/etc/env.php` and `pub/media` are **shared** across releases, so per-scope
  configuration and media survive the swap;
- `app:config:import` applies the configuration that lives in `config.php` and
  `env.php`, which is where per-scope settings belong when they are versioned;
- `setup:upgrade`, the cache flush and worker control are global, matching the single
  database every website shares.

**What govard deliberately does not do:** it never writes per-store configuration
into the database. Base URLs, scope configuration and anything else an operator
changes in the admin are the application's data, not the release's, and a deploy that
rewrote them would be a deploy that can overwrite a live storefront's settings.

**Rehearse it.**

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy --remote sandbox --yes
```

**Verification for several storefronts.** `deploy.verify.url` checks one URL. A
multi-store project that wants every storefront checked anchors a hook on `verify`:

```yaml
deploy:
  hooks:
    - name: every-storefront
      on: "stage:verify"
      position: after
      order: 10
      optional: true
      run: |
        for url in https://shop.example.com/ https://outlet.example.com/; do
          code=$(curl -s -o /dev/null -w '%{http_code}' "$url")
          [ "$code" = "200" ] || { echo "$url returned $code"; exit 1; }
        done
```

## Case 7: Artifact mode in CI

Any of the cases above, with the build moved off the target. The point is that the
job which touches production needs **govard, SSH and rsync and nothing else**.

**Configuration.** Nothing in the project changes; the mode comes from the flags:

```yaml
# .gitlab-ci.yml (see also /workflows/ci-integration)
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

**What runs where.** The build tasks — dependencies, patches, DI compile and the
frontend (Node) build — run in the `build` job. Those are the five neutral build ids
artifact mode replaces with `deploy:artifact`; a recipe exempts the ones that need
the application by marking them, and for Magento that exemption is static content, so
it stays in the deploy and runs on the target after the artifact is unpacked.
`setup:upgrade`, configuration import, the cache flush and verification also run on
the target, as they always did.

**What the artifact refuses to carry.** The paths the recipe declares **shared**
(`shared_files`, `shared_dirs`) are dropped during the build, and
`govard deploy build` prints every one it drops. Without that, an artifact built on
a machine that happens to hold its own `app/etc/env.php` would replace the target's —
a regular file replacing the symlink `deploy:shared` made — and the release would
fail with the application's own words: `Connection "default" is not defined`.

**Rehearse it locally.** The two-job shape is rehearsable without CI:

```bash
rm -rf /tmp/acme-artifact
govard deploy build sandbox --output /tmp/acme-artifact
govard deploy plan sandbox --artifact-dir /tmp/acme-artifact   # shows the artifact branch
govard deploy --remote sandbox --artifact-dir /tmp/acme-artifact --yes
```

**What to expect.** On a real 2.4.9 project the artifact build produced 101,366
files / 752.8 MiB in about seven minutes, and the production-mode deploy that
received it finished in 14m35s (release 11), the largest single step being the static
content pass on the target — about five to six minutes of it, and the step that must
run there. An existing output directory is refused unless you pass `--force`, so a
file left over from an earlier build cannot ship.

**What goes wrong.**

| Message | Meaning |
| --- | --- |
| `artifact manifest does not match the revision` | the artifact directory is from an older build |
| PHP version mismatch is refused at preflight | the CI image's PHP is not the target's; the artifact was built by the wrong interpreter |
| The build machine fails at `setup:static-content:deploy` | it should not be running it — that step is left to the target; check that the recipe marks it and that `govard deploy plan` shows it in the deploy |
| The target fails at `app:configure` with `Connection "default" is not defined` | the artifact carried a shared path; find it in the "left to the target" lines of the build output |

## Case 8: The in-place target you inherited

The migration case: a server already serving the application from a real directory,
deployed to by another tool, which you want govard to take over.

**Step 1 — find out what is there before writing configuration.**

```bash
govard deploy check legacy-staging
```

The output names the layout it found, the publish strategy that implies, the free
space, the PHP the target runs, whether the repository is reachable from the target,
and which Composer credential route is in play. The notes come first, in the order
the probes ran, then the resolved fields:

```
Target legacy-staging is deployable
  publish strategy: in_place
  repository reachable from the target: refs/heads/main
  free space at the deploy path: 42.4 GiB
  php on the target: 8.2.18
  host:            legacy-staging
  deploy path:     /var/www/shop
  current path:    /var/www/shop
  publish:         in_place
  layout:          current path is a real directory: releases are copied into it
```

Which notes appear depends on the target and the run: the symlink strategy adds
`atomic symlink rename: supported`, a sandbox adds that its mirror was refreshed,
artifact mode adds the artifact's file count, revision and PHP comparison, and a
project with private repositories adds the credential route it found. Two notes are
warnings rather than facts — `deploy.settings.sync_paths is empty` for an in-place
target, and a `sync_paths` entry that the release links from `shared/`.

If the remote omits `deploy_path`, govard probes the layout the target already has
(`~`, `~/.deployer`) and adopts it **only when exactly one candidate matches**,
saying which. No layout, or several, is a configuration error (exit 4) naming what
was probed — the one case to resolve by reading the server, not by guessing.

**Step 2 — the other tool's lock.** A target that carries the other deploy tool's
lock is refused rather than raced. That is not a bug to work around: two tools
writing the same release directories is how a half-deployed target happens.

**Step 3 — rehearse the exact layout.** `sandbox reset --layout deployer` seeds a
target the other tool owns, so the refusal can be rehearsed too:

```bash
govard deploy sandbox up --profile full --php 8.3
govard deploy sandbox reset --layout deployer --docroot absent
govard deploy --remote sandbox --yes     # expect a refusal that names the lock
govard deploy sandbox reset --docroot real
govard deploy --remote sandbox --yes     # now the in-place path
```

## Case 9: Laravel, Vite frontend, symlinked webroot {#case-9-laravel}

**The three decisions:** build on the server (`--build=server`) — Laravel has no
compile step worth moving, and its framework caches must be built on the target
anyway; webroot is a symlink into `releases/` (`symlink`); target mode is
`production` through `APP_ENV` in the target's `.env`.

```yaml
deploy:
  keep_releases: 5
  settings:
    shared_files: [".env"]
    shared_dirs: ["storage"]
    writable_dirs: ["storage", "bootstrap/cache"]
    sync_paths: ["vendor", "public/build"]
    frontend_dir: ["."]
    frontend_command: "npm ci && npm run build"
```

- `build:vendors` runs `composer install --no-dev --optimize-autoloader`.
- `build:frontend` builds each `frontend_dir` (here the repository root, where
  Vite lives); the loop is skipped when `frontend_dir` is empty, so a project with
  a committed `public/build` pays nothing.
- `app:configure` runs `artisan storage:link`, so `public/storage` exists in every
  release. It exits 0 when the link is already there, which is why the in-place
  case is safe.
- `db:migrate` runs `artisan migrate --force`.
- `app:cache:flush` runs `optimize:clear` then `optimize` **on the target**:
  `optimize` writes `bootstrap/cache/config.php`, and once that file exists the
  process environment no longer overrides `.env`. A cache built on the build
  machine would ship that machine's configuration to production.
- `worker_control: true` sends `queue:restart` before the migration. It exits 0
  with any cache store, so the store has to be persistent for the workers to
  actually see the signal.
- The `app` check runs `artisan db:show` (Laravel 11+), falling back to
  `migrate:status`. `about --only=environment` is **not** used: it exits 0 with no
  database at all, so it would prove nothing.

## Case 10: Symfony, Doctrine migrations, PostgreSQL target {#case-10-symfony}

**The three decisions:** build on the server; symlink webroot; target mode is the
environment named by `symfony_env` (`prod` unless the project says otherwise).

```yaml
deploy:
  settings:
    shared_files: [".env.local"]
    shared_dirs: ["var/log"]
    writable_dirs: ["var"]
    sync_paths: ["vendor", "public/bundles"]
    symfony_env: prod
  # A project whose database is not the recipe's default says so here, and the
  # sandbox then provides it. Replace, not extend: one database, not two.
  #   sandbox_packages: [postgresql]
  #   sandbox_extensions: [intl, pgsql, mbstring, xml, curl, zip]
  #   sandbox_services: [postgresql, redis-server]
```

- `build:vendors` passes `--no-scripts`. Composer's `auto-scripts` runs
  `cache:clear` and `assets:install`, and both belong on the target: a cache
  warmed for the build machine is worthless, and the asset links resolve against
  the vendor tree that is actually present.
- `build:assets` runs `bin/console assets:install public --symlink --relative` —
  on the target, because `public/bundles` is gitignored and the relative links
  have to resolve from wherever the docroot ends up. That is also why
  `public/bundles` is in `sync_paths` for an in-place docroot.
- `db:migrate` passes `--allow-no-migration`: an empty `migrations/` directory is
  a healthy project, and without the flag Doctrine exits non-zero on it.
- `app:cache:flush` clears and warms the container for `symfony_env`.
- **There is no maintenance window.** Symfony has no core mechanism, so
  `maintenance:enable`/`disable` are skipped and `db:migrate` runs against a live
  site. Add a hook if the project needs one — the deployment page has the shape.
- The `app` check runs `dbal:run-sql "SELECT 1"`, or `doctrine:query:sql` on an
  older DoctrineBundle; the branch is chosen by asking the console.

## Case 11: WordPress, classic layout, wp-cli on the target {#case-11-wordpress}

**The three decisions:** build on the server; symlink webroot; the `full` sandbox
profile, because the recipe's `db:migrate`, cache flush and check all touch the
database.

```yaml
deploy:
  settings:
    shared_files: ["wp-config.php"]
    shared_dirs: ["wp-content/uploads"]
    writable_dirs: ["wp-content/uploads", "wp-content/cache", "wp-content/upgrade", "wp-content/languages"]
```

This is the classic layout only: core files and `wp-content/` in the repository
root, no `composer.json`. A Bedrock layout (core in `vendor/`, docroot `web/`) and
a content-only checkout are not supported.

- **Seed `shared/wp-config.php` before the first deploy.** `deploy:shared` links a
  shared entry only when it already exists, so an unseeded `shared/` leaves the
  first release with the repository's `wp-config.php` — the one that names the
  development database. The `app` check then fails against an unreachable
  database, which is the honest outcome and the signal to seed it.
- `build:vendors` runs `composer install` only when `composer.json` exists.
- `db:migrate` is `wp core update-db` with wp-cli, or a `wp-load.php` bootstrap
  calling `wp_upgrade()` without it. `app:cache:flush` has the same shape
  (`wp cache flush` and `wp rewrite flush --hard`, or the PHP equivalents).
- Maintenance writes `.maintenance` and the `wp-content/maintenance.php` drop-in
  into the served path. The timestamp is written **ahead of the clock**
  (`time() + 86400`), because WordPress expires a flag older than ten minutes: on
  a longer window the site would quietly come back in the middle of the
  migration. The drop-in carries a marker, so a maintenance page the project ships
  is kept.
- `--db-backup` uses `wp db export`, and `rollback --with-db` restores it with
  `wp db import`; both read the connection from `wp-config.php`. The sandbox asks
  for wp-cli and `default-mysql-client` (`mysqldump`) on its own.
- Laravel and Symfony have **no dump command**: turning `--db-backup` on for them
  fails with a message naming the reason instead of quietly producing no backup.

## Rehearsing any case in the sandbox

`govard deploy sandbox` gives the project a real deployment target on this machine:
a container that plays the remote, reached over real SSH and real rsync, with the
same pipeline a production deploy runs. Nothing in the pipeline knows the
difference, which is what makes it a rehearsal rather than a simulation.

```bash
govard deploy sandbox up [--profile basic|php|full] [--php 8.3] [--docroot absent|symlink|real]
govard deploy sandbox status
govard deploy sandbox ssh
govard deploy sandbox reset [--docroot …] [--layout deployer]
govard deploy sandbox down [--purge]
govard deploy --remote sandbox --yes
```

Because `sandbox` is also a subcommand, the deploy must use the flag form:
`govard deploy --remote sandbox --yes`, not `govard deploy sandbox`.

### Which profile can prove what

| Profile | Contains | What a rehearsal against it can prove |
| --- | --- | --- |
| `basic` | sshd, rsync, git | the pipeline itself: release layout, the symlink swap, the lock, `releases`/`status`/`rollback`. No PHP, Composer or Node — a Magento project stops at `build:vendors`. |
| `php` (default) | + php-cli, composer, node, and the web tier | everything `basic` proves, plus the recipe's command line, the dependency install and the frontend Node build, and the **HTTP half of `verify`** against the web tier. No database or cache, so nothing that reads the store can run. |
| `full` | + a database (MariaDB) and a cache (Redis/Valkey) | the whole Magento pipeline including `setup:upgrade`, `app:config:import` and `cache:flush`. This is the profile a real Magento rehearsal needs. |

The recipe also contributes what the framework needs beyond the profile: for
Magento, `libxslt1-dev`, `libzip-dev`, `libpng-dev`, `libjpeg-dev`,
`libfreetype6-dev`, `default-mysql-client`, the PHP extensions
`bcmath curl gd intl mysql soap sockets xsl zip`, and the two services. That list
is the application's, not govard's: a project that needs one more extension adds it
to the recipe, not to a flag.

The `php` and `full` profiles also ship the **web tier**: nginx serving the served
path plus the project's `stack.web_root` (`/pub` for Magento) and PHP-FPM running as
the deploy user, so the application can write the directories `deploy:writable`
hands over. `up` publishes that port on loopback and points the sandbox remote's
`deploy.verify.url` at it, so a sandbox deploy rehearses the whole pipeline, HTTP
check included — the one step a target without a web server could never exercise.
`basic` ships no web tier and advertises no verify URL.

::: warning `stack.web_root` is part of the sandbox image
The nginx `root` is `<current><web_root>`, and the web root is baked into the image
because the image tag is the hash of the rendered definition. A project whose
`stack.web_root` is wrong gets a sandbox that serves the wrong directory — and the
fix is `govard deploy sandbox up --recreate`, not a hand-edited container.
:::

### Shaping the webroot

`--docroot` is how you choose which publish strategy the rehearsal exercises:

| `--docroot` | Target state | Strategy resolved | Case it rehearses |
| --- | --- | --- | --- |
| `absent` | the served path does not exist | `symlink` | a first deploy |
| `symlink` (default) | a dangling symlink into `releases/` | `symlink` | every deploy after the first |
| `real` | a real directory, seeded as a git checkout of the mirror | `in_place` | cases 2 and 8 |

Shaping happens when the target is created and when you name a shape; `reset` shapes
unconditionally. A sandbox that already exists is described by what it is, not by
today's flags: `up` reports the profile and PHP series the container was built for,
and naming a profile or series that disagrees is refused with the flag that actually
changes it (`--recreate`) rather than silently relabelling the container.

### Provisioning the application inside a fresh sandbox

A brand-new sandbox has no installed application, so a pipeline that reaches
`build:assets` or `db:migrate` stops on a **prerequisite** rather than on a defect.
Three things have to exist, in this order:

1. **`shared/app/etc/env.php` on the target**, pointing at a database the container
   can reach (`127.0.0.1`, the shape a server has) and at the cache backend the
   `full` profile runs. This is the file the release links for all of it;
2. **the database behind it**, with the store configuration in it. A clone of a real
   environment (`govard bootstrap -e staging`, or an existing dump) is the usual
   source;
3. **a supported search engine** where the project uses one. `setup:upgrade` refuses
   Magento's MySQL fallback outright.

```bash
govard deploy sandbox ssh
# inside the container, as the deploy user:
#   write ~/.deployer/shared/app/etc/env.php
#   import the database dump
```

Those prerequisites are the target's, not the engine's: a server that has never run
the application cannot publish a release to it, and the sandbox refuses to pretend
otherwise.

### Credentials inside the sandbox

Three Composer routes exist and they are not interchangeable: `COMPOSER_AUTH` from
the deploying environment is forwarded on standard input and **overrides every file
on the target**; a committed `auth.json` in the project is materialised into the
release by `deploy:code`; a `shared/auth.json` on the target is read only when the
project lists `auth.json` in `shared_files`.

What matters for a sandbox is that it is a **fresh target**: a `git`-type package
needs a key and a `known_hosts` entry *inside the container*, and a credential that
lives only in your shell profile is exactly the kind that can override the project's
own working `auth.json` and fail the build where a plain `govard deploy` would have
succeeded. Put the credentials where the target can use them (`docker exec`, or a
mounted file) and re-run — the failing step resumes from a clean release directory
and the Composer cache is kept.

### Reading a failed rehearsal

| The rehearsal says | What it is |
| --- | --- |
| `the sandbox container behind remote "sandbox" is not running` | the container is stopped or was never created; `govard deploy sandbox up` |
| `the sandbox mirror … is missing` | the local git mirror was purged; `up` recreates it |
| `deploy path … is not writable` | the profile's `owner`/`writable_mode` was overridden in a hand-edit; `up` rewrites the remote from the container's state |
| `verify http: http://127.0.0.1:PORT/ returned HTTP 403` | the web tier is up but the application is not installed yet — a prerequisite, not a defect |
| `build:vendors` fails with `composer: not found` | the `basic` profile has no PHP toolchain; use `php` or `full` |
| `The default website isn't defined` | the target has no store configuration in its database |

### An end-to-end rehearsal script

The whole loop for one case, in order, with the parts that matter called out:

```bash
# 1. Build the target that matches the project (case 3/5/6 → full).
govard deploy sandbox up --profile full --php 8.3 --docroot symlink

# 2. Read what the target implies before anything runs. This is where a wrong
#    publish strategy or a missing PHP series shows up, in seconds.
govard deploy check sandbox

# 3. Print the resolved pipeline — every step, its source and where it runs.
govard deploy plan sandbox

# 4. Provision the application (env.php, database, search) — see above.

# 5. Deploy the commit you have not pushed. The mirror is refreshed for you.
govard deploy --remote sandbox --yes --verbose

# 6. Confirm what is live, then rehearse the recovery path.
govard deploy releases sandbox
govard deploy rollback sandbox --yes

# 7. Break it on purpose: Ctrl-C during the upload. Expect "the run was
#    interrupted", the lock released, and no rsync left running on either side.

# 8. Tear down. `--purge` also removes the image, the key and the mirror.
govard deploy sandbox down --purge
```

## Reference: every setting the framework recipes read

Engine-level settings (declared by the default recipe, applied by the core):

| Setting | Default | What it does |
| --- | --- | --- |
| `shared_files` | `app/etc/env.php`, `var/.maintenance.ip` | files linked from `shared/` into every release |
| `shared_dirs` | `var/log`, `var/report`, `var/session`, `var/backups`, `var/tmp`, `pub/media`, `pub/sitemap`, `pub/static/_cache` | directories linked from `shared/` |
| `writable_dirs` | `var`, `pub/static`, `pub/media`, `generated`, `app/etc` | paths made writable in the release |
| `writable_mode` | `chmod` | `chmod`, `chown`, `chmod+chown`, `acl`, `skip` |
| `writable_permissions` | `0775` | the mode used by `chmod` |
| `owner` | (empty; required by `chown`/`chmod+chown`/`acl`) | `user` or `user:group` |
| `sync_paths` | `vendor`, `generated`, `pub/static/adminhtml`, `pub/static/frontend` | paths copied into an **in-place** docroot |
| `php_bin` | `php` | the PHP interpreter on the target |
| `php_version` | (empty; not gated when unset) | the series the target runs — a gate, not a preference |
| `composer_bin` | `composer` | the Composer binary on the target |
| `content_version` | the short revision | `--content-version`; deterministic, so a retry writes the same asset URLs |

Magento settings (declared by the Magento recipe):

| Setting | Default | What it does |
| --- | --- | --- |
| `mage_mode` | (empty → production behaviour) | `production` deploys static content; `developer` skips it |
| `frontend_dir` | (empty → skip) | one path or a list: the Tailwind directories to build |
| `frontend_command` | `npm ci && npm run build` | the command run inside each `frontend_dir` |
| `static_jobs` | `4` | `-j` for `setup:static-content:deploy` |
| `static_content_locales` | (empty → Magento's default locale) | `--language`, one or more |
| `magento_themes` | (empty → every theme) | `-t`; a list of themes or a theme → locales map |
| `split_static_deployment` | `false` | deploy adminhtml and frontend in two passes |
| `magento_themes_backend` | `Magento/backend` | `-t` for the adminhtml pass |
| `static_content_locales_backend` | the frontend locales | `--language` for the adminhtml pass |
| `static_deploy_options` | (empty) | extra flags for **every** pass (`--no-parent`, `-s standard`, …) |
| `worker_control` | `false` | `cron:remove` / `queue:consumers:stop` around the migration, restored after |
| `runtime_reload_command` | (empty) | run as the last part of the cache flush (an opcache reset, an FPM reload) |

`deploy.settings` is validated against the recipe before anything runs: an unknown
key, or a value with the wrong shape, is a configuration error (exit 4) naming the
key and suggesting the near miss. String values must be quoted if they look numeric
(`php_version: "8.2"`) — the engine reads those settings as strings, so an unquoted
`8.2` reads as empty.

Laravel, Symfony and WordPress settings (declared by their recipes):

| Setting | Frameworks | Default | What it does |
| --- | --- | --- | --- |
| `frontend_dir` | all three | (empty → skip) | one path or a list: the directories whose frontend assets are built |
| `frontend_command` | all three | `npm ci && npm run build` | the command run inside each `frontend_dir` |
| `runtime_reload_command` | Laravel, Symfony, WordPress | (empty) | run as the last part of the cache step |
| `worker_control` | Laravel, Symfony | `false` | `queue:restart` / `messenger:stop-workers` before the migration |
| `symfony_env` | Symfony | `prod` | the console environment the deploy runs under; **not validated** |

What the sandbox provides for a project, beyond its recipe's defaults — each list
**replaces** the recipe's, and they are read from the project layer only:

| Setting | Default | What it does |
| --- | --- | --- |
| `sandbox_packages` | recipe | extra apt packages the sandbox image installs |
| `sandbox_extensions` | recipe | PHP extensions, installed as `php<series>-<name>` |
| `sandbox_services` | recipe | init services started before sshd; a name from the engine's table carries its package |
| `sandbox_tools` | recipe | binaries from the engine's known list (`wp-cli`) |

## Checklist before the first deploy to a real target

1. **The target already runs the application.** A server that has never served it
   cannot receive a release: no `shared/app/etc/env.php`, no database, no search
   engine means no first deploy.
2. **`govard deploy check <remote>` is green** and names the publish strategy you
   expect. A surprise here is far cheaper than a surprise in `publish`.
3. **`govard deploy plan <remote>` shows the case you configured**: the right build
   branch, the right `frontend_dir`, the static content step running on the target
   in artifact mode, and `mage_mode` matching the environment.
4. **The same project rehearsed in the sandbox** with a comparable profile and the
   matching `--docroot` shape.
5. **Credentials are where the build will look**, and you know which of the three
   Composer routes is in play.
6. **`deploy.verify.url` is set** if you want the deploy to prove the application
   answers, not only that the files landed.
7. **`--db-backup` is on** for the first deploy that migrates a production database;
   rollback without a dump can restore code but not data.
