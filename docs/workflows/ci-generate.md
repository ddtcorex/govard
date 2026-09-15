---
title: Generating CI pipelines
description: Render a working GitLab pipeline from .govard.yml with govard ci generate — single source of truth, drift check, and the single-job ship shape.
---

# Generating CI pipelines

`govard ci generate` renders a working GitLab pipeline from the project's
`.govard.yml`. The config is the single source of truth: remotes, branches and
`mage_mode` flow into stages, rules and ship commands, so a branch rename or a
new environment updates the pipeline by regenerating — never by hand-editing
generated YAML.

```bash
govard ci generate --provider gitlab --output .gitlab-ci.yml
govard ci generate --provider gitlab --check --output .gitlab-ci.yml
```

Generation reads the config only: no containers, no network, no targets. It
runs anywhere — laptop, CI, a host with nothing installed.

## Generate vs hand-write

Generate the pipeline when the project already describes its environments in
`.govard.yml` — which is every project that deploys with govard. Hand-write
only the two things that are not project data and therefore stay out of the
generated file:

- `CI_GOVARD_IMAGE`: a CI/CD variable pointing at the one internal job image
  (PHP + Composer + Node + rsync + govard), digest-pinned. The registry path
  is company-private, so it lives in GitLab, not in this public file.
- a tiny `ci-check` job (see below) that fails the pipeline when the committed
  YAML drifted from `.govard.yml`.

## What the generated pipeline contains

Four stages: `integrity` → `lint` → `ship` → `rollback`.

| Job | When | What |
| --- | --- | --- |
| `integrity` | always | `govard audit run --checks integrity --format json` — container-free, fails fast |
| `lint:quick` | merge requests | phpcs over the changed PHP/PHTML files plus phpstan per the repo config, with a codequality report |
| `lint:custom` | manual + scheduled | the same tools over the recipe's lint scope — the deep tier the MR gate skips |
| `ship:<remote>` | its branch | build-then-deploy in one workspace (see below), one job per remote, `resource_group` per environment |
| `rollback:<remote>` | manual | `govard deploy rollback <remote> --yes` |

Secrets appear by name only (`$SSH_KEY`, `$SSH_KNOWN_HOSTS`); values never
touch the file. Composer and npm caches are keyed by their lock files, and the
`variables` block pins `GOVARD_VERSION` to the binary that rendered the file —
a dev binary records `dev`, a release binary its tag — so the file always says
which Govard the job image must bundle.

The lint inputs are framework-owned, not hard-coded: two neutral settings the
framework recipe defaults and the project may override under `deploy.settings`
(or per remote):

| Setting | Magento recipe default | Unset behaviour |
| --- | --- | --- |
| `ci_lint_phpcs_standard` | `Magento2` | `PSR-12`, the language default |
| `ci_lint_paths` | `app/code app/design` | the scoped phpcs line is omitted (phpstan still runs) |

Generation layers the recipe defaults under the project settings — the same
direction the deploy itself resolves — so what you see is what the framework
declares. A framework that declares no scope gets phpstan-only deep lint
until it does.

## The single-job ship

Each `ship:<remote>` job builds and deploys inside the same workspace instead
of the classic build-job/uploads-artifact/deploys-job split. The rehearsal
measured why: a production artifact is 101,733 files / 772 MiB, and moving it
between jobs (let alone downloading it back) is pure overhead when the runner
can do both halves itself. Nothing floats between jobs, so nothing can drift
between them either.

The shape follows the effective `mage_mode` (remote override wins key by key
over the project block, resolved by the same merge the deploy itself uses):

- `developer` → server build: `govard deploy --remote <name> --revision "$CI_COMMIT_SHA" --yes`
- anything else → `govard deploy build <name> --output "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA"`,
  then `govard deploy <name> --artifact-dir "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA" --yes`

Measured on the reference project (Magento 2.4.9 + Hyva): server build in
production with the full 3×3 theme×locale matrix 11m48s, artifact deploy 8m18s,
trimmed 2×1 matrix 5m36s, maintenance window ~1 min in every mode. Targets: dev
push-to-green ≤ ~4 min, production ≤ ~9 min full matrix. The matrix — not the
mode — moves minutes: 9 static combos take ~7 min, 2 take ~1m19s.

## The drift check in CI

Every push should prove the committed pipeline still matches the config. Add
one small hand-written job (it cannot be generated — it guards the generated
file):

```yaml
ci-check:
  stage: integrity
  script:
    - govard ci generate --provider gitlab --check --output .gitlab-ci.yml
```

It behaves like `gofmt -l`: exit 0 when the file matches, exit 1 with the
first 20 differing lines when it drifted. The fix is always to regenerate,
never to hand-edit below the `DO NOT EDIT` header.

## One image, human output

Every job runs in the same internal image, so PHP, Composer, Node, rsync and
govard are identical from `integrity` to `rollback`. Pin it by digest, never by
a floating tag.

A red job reports like a person, not a trace: what failed, in text logs, with
the reproduction command (`govard deploy releases <remote>`, `govard deploy
status`) right in the log. Errors block the pipeline; warnings (phpcs
warnings, phpstan level noise below the repo's baseline) never do.

- The two-job artifact shape this replaces, and what an artifact can and
  cannot carry: [Deployment](/workflows/deployment#the-two-job-ci-shape)
- Which steps run where for a given project shape: [Deployment case studies](/workflows/deploy-case-studies)
