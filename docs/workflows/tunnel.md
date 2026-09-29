---
title: Public Tunnel — Share Your Local Project
description: Expose a local Govard project publicly via Cloudflare Tunnel with automatic base-URL rewriting and Caddy alias routing.
---

# Tunnel

Share your local Govard project on a public URL without deploying — ideal for client demos, webhook testing, or mobile device checks.

---

## Prerequisites

Install [`cloudflared`](https://github.com/cloudflare/cloudflared/releases) on your host:

```bash
# Linux (.deb from Cloudflare releases)
curl -fsSL https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-amd64.deb -o /tmp/cloudflared.deb
sudo dpkg -i /tmp/cloudflared.deb

# macOS (Homebrew)
brew install cloudflared
```

Verify:

```bash
cloudflared --version
```

Govard never bundles `cloudflared` — you manage its install/upgrade yourself.

---

## Quick Start

```bash
# Start a tunnel for the current project (auto-detect URL from cloudflared output)
govard tunnel start

# Or pass an explicit URL you already created
govard tunnel start https://my-demo.trycloudflare.com

# Dry-run: show what would happen without launching
govard tunnel start --plan
```

While the tunnel is running:

- Govard registers the tunnel domain as a **Caddy alias** for the project (so Caddy routes it like `*.test`, no extra DNS change needed).
- The original `Host` header is kept intact — Magento/Laravel don't see a foreign host and won't redirect.
- Framework base URL is **rewritten** to the tunnel URL via the framework's `BaseURLManager` (Magento 2 writes `web/unsecure/base_url` etc.). The original URL is restored on stop or `Ctrl+C`.

Stop the tunnel:

```bash
govard tunnel stop
# or just Ctrl+C the start process
```

`tunnel start` records the process it launched — its PID and the argv it was
started with — under `$GOVARD_HOME_DIR/tunnels/<project>.pid`, and `tunnel stop`
signals that one process. It never searches the host for a process by name, so
there is no pattern it can overreach with; what it refuses is any PID whose argv
does not **begin with** the one it recorded — every argument from the first on,
compared by the executable's name rather than its path, because the record
stores the resolved path while the host reports the bare one. A `cloudflared`
you started yourself, one belonging to another project, and one belonging to
another tool are all left alone — including a `cloudflared tunnel` unit of
another tool, which shares the binary and the `tunnel` verb and is separated by
the arguments that follow. The one shape that is *not* separable is another copy
of govard's own command line: a different install of the same binary running
exactly what the record names is indistinguishable from the tunnel govard
started. The argv is checked before any signal:
if the recorded PID has been recycled by an unrelated program, or its argv
cannot be read, `tunnel stop` refuses with an error and signals nothing — remove
the record by hand once you have checked the tunnel yourself.

A record govard cannot parse at all — a truncated or hand-edited file — makes
`start`, `stop` and `status` all fail with an error rather than assume there is
no tunnel, because a record nobody can interpret is not the same as no record
and assuming so would strand a running tunnel. Remove the file once you have
checked the tunnel by hand.

With no record, `tunnel stop` is a no-op that says so and exits 0, so it is
safe to run twice. The project's base URL is restored either way, except when
a signal was refused — there the tunnel is still up, so the base URL keeps
pointing at it.

Check status:

```bash
govard tunnel status
```

`status` reads the same record rather than searching the host, so it reports
`INACTIVE` whenever Govard has no tunnel of its own running — including when
some other program happens to own the recorded PID.

---

## Flags

| Flag | Effect |
| :--- | :--- |
| `[url]` | Optional tunnel URL. When omitted, Govard parses it from `cloudflared` stdout. |
| `--provider <name>` | Tunnel provider. Only `cloudflare` exists today. |
| `--no-tls-verify` | Skip TLS verification for the tunnel endpoint (useful behind corporate proxies). |
| `--plan` | Print the start plan and exit — no process launched. |

---

## How It Works

1. `govard tunnel start` resolves the target URL (arg, `--url` flag, or auto-detect).
2. Provider `BuildStartPlan` creates a Caddy alias route and a framework base-URL patch plan.
3. The tunnel process (`cloudflared tunnel --url http://localhost:80` or `--hello-world` style) is spawned.
4. On exit (requested or interrupted), Govard removes the Caddy alias and reverts the base URL.

> **Scope:** `tunnel` rewrites only the primary store/domain. Multi-store `store_domains` keep their `.test` hosts — they still resolve locally via `dnsmasq`.

---

## Troubleshooting

| Symptom | Fix |
| :--- | :--- |
| `cloudflared: command not found` | Install `cloudflared` first (see Prerequisites). |
| Tunnel URL shows Govard 404 | Run `govard env up` first — the project must be running so Caddy has a backend. |
| Base URL not restored after Ctrl+C | Run `govard tunnel stop` or `govard config auto` (Magento 2) to re-apply the local URL. |
| `tunnel status` says no tunnel | Govard has no tunnel of its own running. `status` reads the recorded PID, so it also says this when the tunnel died and left a stale record (which it clears), or when another program took over that PID. |

---

[SSL and Domains](/workflows/ssl-and-domains) | [CLI Commands](/reference/cli-commands#govard-tunnel) | [Architecture](/developer/architecture)
