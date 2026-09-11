---
title: Commands That Run Without Docker — Govard
description: Every Govard command whose manifest requirement excludes Docker — container-free audit analysis, host diagnostics, remote and tunnel workflows — plus the capability gate and its exit codes.
---

# Runs Without Docker

Govard installs and runs on a host with no container runtime. Every command
declares its requirement in the binary's manifest, and `govard capabilities`
prints the resolved set for the machine you are on — **that output is the source
of truth, and this page mirrors it.**

```bash
govard capabilities          # command, requirement, host status
govard capabilities --json   # machine-readable (schema_version 1)
```

## Commands that do not need Docker

| Command | Requirement |
| :--- | :--- |
| `govard audit cleanup` | `none` |
| `govard audit diff` | `none` |
| `govard audit rerun` | `none` |
| `govard audit result` | `none` |
| `govard audit run` | `none` |
| `govard audit status` | `none` |
| `govard blueprint` | `none` |
| `govard blueprint cache` | `none` |
| `govard blueprint cache clear` | `none` |
| `govard blueprint cache list` | `none` |
| `govard capabilities` | `none` |
| `govard completion bash` | `none` |
| `govard completion fish` | `none` |
| `govard completion powershell` | `none` |
| `govard completion zsh` | `none` |
| `govard config` | `none` |
| `govard config get` | `none` |
| `govard config profile` | `none` |
| `govard config profile clear` | `none` |
| `govard config set` | `none` |
| `govard custom` | `none` |
| `govard custom list` | `none` |
| `govard desktop doctor` | `none` |
| `govard doctor` | `none` |
| `govard doctor trust` | `none` |
| `govard domain list` | `none` |
| `govard help` | `none` |
| `govard init` | `none` |
| `govard project list` | `none` |
| `govard project open` | `none` |
| `govard remote add` | `ssh,rsync` |
| `govard remote audit stats` | `ssh,rsync` |
| `govard remote audit tail` | `ssh,rsync` |
| `govard remote copy-id` | `ssh,rsync` |
| `govard remote exec` | `ssh,rsync` |
| `govard remote test` | `ssh,rsync` |
| `govard self-update` | `net` |
| `govard sync` | `ssh,rsync` |
| `govard trust` | `none` |
| `govard tunnel` | `cloudflared` |
| `govard tunnel start` | `cloudflared` |
| `govard tunnel status` | `cloudflared` |
| `govard tunnel stop` | `cloudflared` |
| `govard version` | `none` |
| `govard vscode setup` | `none` |

The `Requirement` column is the exact token `govard capabilities` reports:

| Token | Meaning |
| :--- | :--- |
| `none` | Nothing beyond the Govard binary itself. |
| `ssh,rsync` | A working SSH client and rsync. Still no container runtime. |
| `cloudflared` | The `cloudflared` binary, for tunnels. Still no container runtime. |
| `net` | Outbound network access, for `self-update`. Still no container runtime. |

Commands whose requirement includes `docker` are gated instead: the stack
lifecycle (`env`, `restart`, `down`, `ps`, `logs`, `svc`, `db`, `shell`, `tool`,
`test`, `frontend`), the parts of project and domain management that touch
containers (`project delete`, `project orphans`, `domain add`, `domain remove`),
the `vscode <tool>` wrappers, `deploy`, `bootstrap`, `debug`, launching
`desktop`, and the container-backed audit paths (`audit toolchain`,
`audit run --checks lint`, `audit run --checks profiler`).

## Docker-free features

- **Container-free analysis.** `govard audit run --checks integrity` (Magento 2 /
  Mage-OS) reads the checkout with Go analyzers — Composer manifest/lock
  agreement, module/DI/sequence consistency — with no Docker, no PHP, and no
  toolchain image. The rest of the audit lifecycle is host-side as well:
  `status`, `result`, `cleanup`, and `diff`. `rerun` repeats the checks recorded
  in the session, so rerunning a `lint` or `profiler` session needs Docker.
- **Diagnostics.** `govard doctor` runs anywhere; Docker is one optional check
  and its absence does not fail the command (`--strict` restores the hard gate
  for bootstrap scripts). `govard trust` installs the local CA into the host
  trust store.
- **Configuration.** `govard config get|set` and `govard config profile`
  read and write project configuration; `govard config profile clear` and
  `govard blueprint cache list|clear` work on files and the local cache.
  Applying a profile to a running stack (`config profile apply|switch`),
  `govard config auto` (it configures the framework inside the container), and
  every `govard lock` command are container work: the lock file records the
  resolved docker/compose versions and service image digests.
- **Project scaffolding.** `govard init` and `govard custom list`.
- **Registry and domains.** `project list` and `project open` read the registry;
  `domain list` prints the project's domains; `vscode setup` derives the editor
  settings from the project's own files. `project orphans` inspects Docker
  resources, so it keeps the requirement.
- **Remote and sync.** `govard remote add|test|copy-id|exec`,
  `govard remote audit stats|tail`, and `govard sync` need SSH and rsync, not
  Docker.
- **Tunnels.** `govard tunnel start|stop|status` drive `cloudflared` on the host.
- **Self-update, help, completion.** `govard self-update` needs only network
  access; `govard version`, `govard help`, and `govard completion
  bash|zsh|fish|powershell` need nothing at all.

## When something is missing

The gate refuses before the command does any work, and names what is missing:

| Exit code | Meaning |
| :--- | :--- |
| `0` | Success. |
| `1` | The command ran and failed. |
| `2` | Usage error: unknown flag or invalid argument. |
| `3` | `CAPABILITY_MISSING` — a declared requirement is unavailable. |
| `4` | Configuration error. |

`--error-json` prints the failure as a machine-readable envelope on stdout
(`schema_version`, `code`, `capability`, `command`, `message`, `hint`), so a
script never has to parse the text form:

```bash
govard tunnel status --error-json
# {"schema_version":1,...,"error":{"code":"CAPABILITY_MISSING","capability":"cloudflared",...}}
```

A container-backed audit check points at the container-free alternative:

```bash
govard audit run --checks lint
# Hint: run `govard audit run --checks integrity` for container-free analysis on this host
```

## Related

- [Installation](/getting-started/installation#runs-without-docker) — installing
  and running the CLI without Docker.
- [Audit](/workflows/audit) — the `integrity` check and the container-backed
  lint path.
- [CLI Commands](/reference/cli-commands) — the full command reference.
