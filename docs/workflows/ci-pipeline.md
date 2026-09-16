---
title: CI pipelines
description: Hand-written GitLab pipeline pattern for govard projects — integrity, lint, single-job ships and manual rollbacks using existing commands.
---

# CI pipelines

Govard ships no pipeline generator: the project's `.gitlab-ci.yml` is written
by hand from the pattern below, using only commands that already exist.
`.govard.yml` stays the source of truth for environments — when a remote, a
branch or a `mage_mode` changes there, update the matching `rules` and ship
commands here.

## The shape

Four stages: `integrity` → `lint` → `ship` → `rollback`.

| Job | When | What |
| --- | --- | --- |
| `integrity` | always | `govard audit run --checks integrity --format json` — container-free, fails fast |
| `lint:quick` | merge requests | phpcs over the changed PHP/PHTML files plus phpstan per the repo config, with a codequality report |
| `lint:custom` | manual + scheduled | the same tools over the whole custom tree — the deep tier the MR gate skips |
| `ship:<remote>` | its branch | build-then-deploy in one workspace (see below), one job per remote, `resource_group` per environment |
| `rollback:<remote>` | manual | `govard deploy rollback <remote> --yes` |

## The job image

Every job runs in one internal image: PHP + Composer + Node + rsync + govard,
so the toolchain is identical from `integrity` to `rollback`. Define it once
as a CI/CD variable, digest-pinned — the registry path is company-private, so
it lives in GitLab, never in this public file:

```yaml
image: $CI_GOVARD_IMAGE

variables:
  COMPOSER_CACHE_DIR: "$CI_PROJECT_DIR/.composer-cache"
  NPM_CONFIG_CACHE: "$CI_PROJECT_DIR/.npm-cache"
  # Speed flags proven on the reference project: fast zip handling and minimal
  # compression for caches and artifacts.
  FF_USE_FASTZIP: "true"
  CACHE_COMPRESSION_LEVEL: "fastest"
  ARTIFACT_COMPRESSION_LEVEL: "fastest"
```

## SSH and caches

Secrets appear by name only (`$SSH_KEY`, `$SSH_KNOWN_HOSTS`); values never
touch the file. Composer cache is keyed by its lock file, so a lock change
busts the cache instead of snowballing it:

```yaml
.ssh_setup: &ssh_setup
  - "command -v ssh-agent >/dev/null || ( apt-get update -y && apt-get install openssh-client -y )"
  - eval $(ssh-agent -s)
  - mkdir -p ~/.ssh
  - touch ~/.ssh/known_hosts
  - echo "$SSH_KEY" | tr -d '\r' | ssh-add -
  - chmod 700 ~/.ssh
  - echo "$SSH_KNOWN_HOSTS" > ~/.ssh/known_hosts

.composer_cache: &composer_cache
  key:
    files:
      - composer.lock
    prefix: ${CI_PROJECT_PATH_SLUG}
  fallback_keys:
    - ${CI_PROJECT_PATH_SLUG}-composer-default
  paths:
    - .composer-cache/
  policy: pull-push
```

## Lint

`lint:quick` runs on merge requests only: phpcs over the files the MR changed
(warnings never fail the job — `--runtime-set ignore_warnings_on_exit 1`)
plus phpstan per the repo config with a codequality report:

```yaml
lint:quick:
  stage: lint
  cache: *composer_cache
  rules:
    - if: $CI_PIPELINE_SOURCE == "merge_request_event"
  before_script:
    - composer install --prefer-dist --no-progress --no-interaction --no-dev
  script:
    - |
      files=$(git diff --name-only --diff-filter=ACMRT "$CI_MERGE_REQUEST_DIFF_BASE_SHA...$CI_COMMIT_SHA" -- '*.php' '*.phtml' | grep -v -e '^vendor/' -e '^generated/' -e '^var/' -e '^pub/media/' || true)
      if [ -n "$files" ]; then echo "$files" | xargs phpcs --standard=Magento2 --runtime-set ignore_warnings_on_exit 1 -p; else echo 'No PHP files changed.'; fi
    - phpstan analyse --memory-limit=2G --no-progress --error-format=gitlab > phpstan-gitlab.json
  artifacts:
    when: always
    reports:
      codequality: phpstan-gitlab.json
```

`lint:custom` is the same tools over the whole custom tree, on schedule and
on demand. The standard and the scope below are the Magento values — other
frameworks swap in their own (PSR-12 is the neutral standard):

```yaml
lint:custom:
  stage: lint
  cache: *composer_cache
  rules:
    - if: $CI_PIPELINE_SOURCE == "schedule"
    - when: manual
  before_script:
    - composer install --prefer-dist --no-progress --no-interaction --no-dev
  script:
    - phpcs --standard=Magento2 --runtime-set ignore_warnings_on_exit 1 -p app/code app/design
    - phpstan analyse --memory-limit=2G --no-progress --error-format=gitlab > phpstan-gitlab.json
  artifacts:
    when: always
    reports:
      codequality: phpstan-gitlab.json
```

## Ship

Each `ship:<remote>` job builds and deploys inside the same workspace instead
of the classic build-job/uploads-artifact/deploys-job split. The rehearsal
measured why: a production artifact is 101,733 files / 772 MiB, and moving it
between jobs (let alone downloading it back) is pure overhead when the runner
can do both halves itself. Nothing floats between jobs, so nothing can drift
between them either.

The shape follows the remote's `mage_mode` in `.govard.yml`:

- `developer` → server build, everything happens on the target:
  `govard deploy --remote <name> --revision "$CI_COMMIT_SHA" --yes`
- anything else → build an artifact and deploy it in the same workspace:

```yaml
ship:production:
  stage: ship
  resource_group: production
  rules:
    - if: $CI_COMMIT_BRANCH == "master"
  cache: *composer_cache
  before_script:
    - *ssh_setup
  script:
    - govard version
    - govard deploy build production --output "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA"
    - govard deploy production --artifact-dir "$CI_PROJECT_DIR/artifacts" --revision "$CI_COMMIT_SHA" --yes
  environment:
    name: production
```

```yaml
ship:dev1:
  stage: ship
  resource_group: dev1
  rules:
    - if: $CI_COMMIT_BRANCH == "develop"
  before_script:
    - *ssh_setup
  script:
    - govard version
    - govard deploy --remote dev1 --revision "$CI_COMMIT_SHA" --yes
  environment:
    name: dev1
```

Measured on the reference project (Magento 2.4.9 + Hyva): server build in
production with the full 3×3 theme×locale matrix 11m48s, artifact deploy 8m18s,
trimmed 2×1 matrix 5m36s, maintenance window ~1 min in every mode. Targets: dev
push-to-green ≤ ~4 min, production ≤ ~9 min full matrix. The matrix — not the
mode — moves minutes: 9 static combos take ~7 min, 2 take ~1m19s.

## Rollback

```yaml
rollback:production:
  stage: rollback
  resource_group: production
  rules:
    - if: $CI_COMMIT_BRANCH == "master"
      when: manual
  before_script:
    - *ssh_setup
  script:
    - govard deploy rollback production --yes
```

One per remote, same branch rule, always manual.

## Human output

A red job reports like a person, not a trace: what failed, in text logs, with
the reproduction command (`govard deploy releases <remote>`, `govard deploy
status`) right in the log. Errors block the pipeline; warnings (phpcs
warnings, phpstan level noise below the repo's baseline) never do. Pin the job
image by digest, never by a floating tag.

- The two-job artifact shape this replaces, and what an artifact can and
  cannot carry: [Deployment](/workflows/deployment#the-two-job-ci-shape)
- Which steps run where for a given project shape: [Deployment case studies](/workflows/deploy-case-studies)
