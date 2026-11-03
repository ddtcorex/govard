# Windows and WSL2

Govard is developed and tested on Linux, and runs on macOS. On Windows, the
supported path is WSL2: install Govard inside the distribution, not on the
Windows host.

This page states what is supported, what is not, and why, so you can decide
before spending an afternoon on it.

## What works on native Windows

- **The binary builds.** GoReleaser publishes `windows/amd64` and
  `windows/arm64` archives on every release.
- **npm installs it.** `npm i -g @ddtcorex/govard` downloads the matching
  archive and extracts it with PowerShell; the `govard` shim then runs.
- **Host-side commands that do not touch containers** work: `govard version`,
  `govard capabilities`, `govard doctor` and `govard help`.

## What does not

- **CI does not exercise Windows.** Every workflow runs on `ubuntu-latest`, so
  the Windows build is compiled rather than tested. Treat it as unsupported
  until that changes.
- **The desktop app is Linux only.** It needs GTK 4 and WebKitGTK 6.0, and the
  release pipeline builds it for `linux/amd64` alone.
- **`.test` domains and HTTPS are not automated.** Govard writes the Unix hosts
  file and trusts its local CA in the Unix system store. On Windows you would
  have to add entries to `C:\Windows\System32\drivers\etc\hosts` and import the
  CA into the Trusted Root store by hand.
- **`install.sh` does not run.** It is a bash script that calls `uname`; use
  npm or the release archive instead.

## WSL2, the supported path

Run everything inside the distribution:

```bash
# inside the WSL2 distribution
npm i -g @ddtcorex/govard
govard version
govard svc up        # proxy, database UI, mail catcher
govard init
govard env up
```

Two things to know:

- **Docker:** install Docker Engine inside the distribution, or enable Docker
  Desktop's WSL integration for that distribution. Either way Docker must be
  reachable from inside WSL2, which is how Govard talks to it.
- **Browsing from Windows:** the `.test` domains resolve inside the
  distribution. To open them from a Windows browser, add the same host entries
  to the Windows hosts file, or browse the distribution's IP directly.

## Reporting a Windows problem

Open a bug report and include `govard doctor` output. Since Windows is not in
CI, a report with the exact command and version is what makes a fix possible.
