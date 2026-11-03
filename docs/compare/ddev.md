# Govard and DDEV

Both tools run a Magento project in containers and give it a local domain. They
differ in what they start, what they automate outside the container, and how the
project is configured. This page states the differences; the numbers live on the
[benchmark page](/workflows/benchmark), so they cannot drift apart from the
method that produced them.

## What starts, by default

| | Govard | DDEV |
| :--- | :--- | :--- |
| Web + database | yes | yes |
| Varnish, Redis, OpenSearch, RabbitMQ | part of the default profile | add-ons |
| Router ports | 80 and 443, shared with every Govard project | 80 and 443 by default, configurable |
| Configuration file | `.govard.yml` | `.ddev/config.yaml` |

That difference is the first thing to read in any comparison: a stack with
Varnish and a search engine is not the same product as a stack without them, and
the benchmark page says which services each arm ran.

## What Govard does that is outside this comparison

- `govard deploy` with a sandbox rehearsal, rollback and resume.
- Named remotes with scoped capabilities and a write-protected `prod`.
- `govard audit` with persisted sessions, and a profiler capture.
- A shared SSH gateway, snapshots, and a desktop app over the same engine.

None of that is measured here; it is either present in the tool or it is not.

## Moving a project from DDEV to Govard

```bash
govard init --migrate-from ddev
```

The importer reads `.ddev/config.yaml` and reports every key it mapped, defaulted
or ignored. It does not silently drop settings.

## When DDEV is the better choice

- Windows: DDEV's native Windows support is mature; Govard's supported path there
  is WSL2 (see [Windows and WSL2](/more/windows)).
- A large add-on ecosystem for services Govard does not ship.
- A team already standardised on DDEV and happy with it. Switching costs more
  than it returns unless something is actually hurting.

## When Govard is the better choice

- Magento and Mage-OS depth: `env.php` wiring, table prefixes, Varnish,
  multi-store domains, and a `php-debug` container that does not slow the normal
  one.
- Deployment and remote work inside the same tool you develop with.
- A local CA and `*.test` domains that need no per-project setup.
