---
title: Govard and Warden
description: How Govard and Warden differ for a Magento project; what each starts by default, what the installer touches on the host, and how a migration carries the project over.
---

# Govard and Warden

Both start a Magento stack in containers and route it to a local domain. Warden
predates Govard: it pairs an opinionated `env-init` stack with a traefik router
and keeps its configuration in environment files. Govard ships a single binary, a
Caddy proxy and a declared profile in `.govard.yml`. This page states the
differences; the numbers live on the [benchmark page](/workflows/benchmark), so
they cannot drift away from the method that produced them.

## What starts, by default

| | Govard | Warden |
| :--- | :--- | :--- |
| Web + database | yes | yes |
| Varnish, Redis, OpenSearch, RabbitMQ | part of the default profile | part of the stack `env-init` writes |
| Router | Caddy on 80 and 443, shared by every Govard project | traefik on 80 and 443, with no way to move it |
| Configuration | `.govard.yml` | `.env` plus `.warden/warden-env.yml` |
| Installing the CLI | one binary into `~/.local/bin`, no privileges | root once: it chowns an SSH tunnel key, rewrites `/etc/ssh/ssh_config` and installs a CA into the system trust store |

The last row is the difference a team feels first, and it is also why the
benchmark arm runs Warden's CLI from a container rather than asking for sudo on
the measured machine.

## How the two measured

Same fixture, same machine, same window, three runs each after a discarded
warm-up. The full table and the ranges are on the
[benchmark page](/workflows/benchmark).

| Metric | Govard | Warden | Read |
| :--- | ---: | ---: | :--- |
| Start, until the storefront answers | 11.0 s | 11.6 s | close |
| Stop | 11.2 s | 3.7 s | Warden is about three times faster |
| TTFB p50, cache as it ships | 14.2 ms | 13.5 ms | close |
| TTFB p50, cache bypassed | 250.6 ms | 495.7 ms | Govard is about twice as fast on the cold render |
| Static deploy | 4.3 s | 7.7 s | Govard is faster |
| DI compile | 21.0 s | 25.5 s | Govard is faster |
| Peak memory, steady state | 1,601 MB | 1,020 MB | Warden uses less |
| CLI wrapper overhead | 46 ms | 213 ms | see the caveat below |

Two rows need the caveat before they are quoted:

- **Stop.** Govard's `env down` takes 11.2 s against Warden's 3.7 s. It removes
  more, and this page does not dress that up.
- **CLI overhead.** Warden's 213 ms includes running its CLI inside a container,
  which is how this arm had to work without root on the measured host. A native
  install would not pay that.

What was not equal: Warden's traefik pins ports 80 and 443, so the Govard proxy
was stopped while that arm ran, and its containers were reached directly. Both
arms served through Varnish, so the cache-bypassed row compares the same shape of
request on both sides.

## Moving a project from Warden to Govard

```bash
govard init --migrate-from warden
```

The importer reads `.env` and `.warden/warden-env.yml`, maps the project name,
framework, PHP, Node, Composer, MySQL and Varnish versions, the web root and the
table prefix, and offers to clone the existing Warden database volume into a
Govard one. Then `govard env up`. The full walkthrough, including the cache
cleanup that avoids stale Redis and file cache in the new environment, is on
[Migrate from DDEV or Warden](/getting-started/migration).

## When Warden is the better choice

- An existing Warden setup that a team already operates, where switching costs
  more than it returns.
- You want the stack and the router to come from one opinionated environment kept
  in shell and env files, and the root install is not a problem for you.
- You depend on Warden's own env templates for stacks Govard does not ship.

## When Govard is the better choice

- You want no privileged install and no host files rewritten to get a local
  environment.
- Deploy, remotes with a write-protected `prod`, audit, snapshots and a desktop
  app in the same tool you develop with.
- You came from DDEV as well, so one importer covers both, and it reports every
  key as mapped, defaulted or ignored instead of dropping it.
- A cold page render matters more to you than steady state memory, according to
  the numbers above.
