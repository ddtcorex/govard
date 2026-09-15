---
title: CI Integration
description: Run Govard in CI with pinned versions via the setup-govard action, the GHCR image, or the npm wrapper.
---

# CI Integration

Three ways to run Govard in CI, from most convenient to most manual.
All support version pinning — never float on `latest` in a release pipeline.
Replace `<version>` with a tag from the [releases page](https://github.com/ddtcorex/govard/releases).

## GitHub Actions (recommended)

```yaml
- uses: ddtcorex/setup-govard@v1
  with:
    version: '<version>'

- run: govard audit --ci
```

Omit `version` (or pass `latest`) for throwaway branches only.

## Container jobs

Any runner with Docker can use the published image directly:

```yaml
jobs:
  audit:
    runs-on: ubuntu-latest
    container: ghcr.io/ddtcorex/govard:<version>
    steps:
      - run: govard version
```

```bash
# GitLab CI
audit:
  image: ghcr.io/ddtcorex/govard:<version>
  script:
    - govard version
```

## npm install step

When the job already runs on Node 20+:

```bash
npm i -g @ddtcorex/govard@<version>
govard version
```

Slow or restricted networks can point the wrapper at an internal mirror
via the `GOVARD_MIRROR` environment variable (base URL for release assets).

## Deploying from CI

Govard deploys a git revision over SSH and rsync, so the job that touches production
needs no PHP, Composer, Node or container runtime. The toolchain belongs to the build
job, and the artifact is what crosses between them:

| Job | Image needs | Command |
| --- | --- | --- |
| `build` | PHP, Composer, Node — whatever the recipe's build tasks need | `govard deploy build production --output artifacts --revision $CI_COMMIT_SHA` |
| `deploy` | govard, ssh, rsync | `govard deploy production --artifact-dir artifacts --revision $CI_COMMIT_SHA --yes` |

The artifact's manifest is checked against the files themselves, the revision being
deployed and the PHP the target runs, and the steps that need the deployed
application (static content) run on the target after the artifact is unpacked.

- Two-job shape in full, and what an artifact can and cannot carry: [Deployment](/workflows/deployment)
- Which steps run where for a given project shape: [Deployment case studies](/workflows/deploy-case-studies)
- Skip hand-writing the jobs: [`govard ci generate`](/workflows/ci-generate) renders integrity, lint, per-remote single-job ships and manual rollbacks from `.govard.yml`, with a `--check` mode that fails CI when the committed file drifted.

There is no Vietnamese counterpart of this page; the deploy pages themselves are
bilingual.
