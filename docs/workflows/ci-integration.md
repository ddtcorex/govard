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
