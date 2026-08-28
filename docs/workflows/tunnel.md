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

Check status:

```bash
govard tunnel status
```

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
| `tunnel status` says no tunnel | No active tunnel — `tunnel start` must be running in another terminal. |

---

[SSL and Domains](/workflows/ssl-and-domains) | [CLI Commands](/reference/cli-commands#govard-tunnel) | [Architecture](/developer/architecture)
