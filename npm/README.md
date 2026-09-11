# @ddtcorex/govard

Thin binary installer for the [Govard](https://github.com/ddtcorex/govard)
local development orchestrator CLI. The package downloads the matching
prebuilt binary from GitHub Releases on `postinstall`, verifies its sha256
checksum, and exposes it as the `govard` bin.

CLI only — the Desktop app is distributed via `.deb` / `.pkg` releases.

The package installs and runs without Docker. Stack commands (`govard env up`,
`svc`, `db`, `shell`, `test`, `audit`) need it; run `govard capabilities` for the
list of commands that work without a container runtime.

## Usage

```sh
npx @ddtcorex/govard --version
npm i -g @ddtcorex/govard
govard --help
```

Pin a version for CI:

```sh
npm i -g @ddtcorex/govard@1.70.3
```

> npm 11+ holds `postinstall` scripts for approval: after install, run
> `npm approve-scripts @ddtcorex/govard` (or pre-approve in CI config)
> so the binary download step is allowed to run.

## Environment

- `GOVARD_MIRROR` — base URL override for release assets (e.g. an internal
  mirror) instead of `github.com/ddtcorex/govard/releases/download`.
- `GOVARD_SKIP_POSTINSTALL=1` — skip the binary download (for pre-seeded
  environments).

Installs via this package report install source `npm`, so
`govard self-update` defers to the package manager (`npm i -g
@ddtcorex/govard@latest`) instead of overwriting the binary.
