---
title: Govard and Lando
description: Why the benchmark page has no Lando column, what Lando ships for Magento today, and when each tool is the better choice.
---

# Govard and Lando

Lando is a general-purpose local development tool: a Landofile describes services
and tooling, a proxy routes local names, and recipes package common stacks.
Govard is narrower: one CLI, a declared stack per framework, and a Magento profile
that ships Varnish, Redis, OpenSearch, RabbitMQ and the `env.php` wiring. This
page explains why the [benchmark page](/workflows/benchmark) has no Lando column,
and what would have to change for it to get one.

## Why there is no measured column

Lando's official plugin index (consulted 2026-11-04, Lando 3.26.9) lists recipes
for Drupal, Backdrop, Joomla, Laravel, Symfony, WordPress, LAMP, LEMP, MEAN and
Dotnet. There is no Magento recipe. A Magento project on Lando is therefore a
Landofile you write yourself: the LAMP or LEMP recipe, plus the Varnish, Redis,
Elasticsearch and MariaDB service plugins, plus the Magento-specific environment
and cron wiring around them.

Measuring that would measure the person who wrote it, not the tool. Two Lando
users can produce stacks that differ from each other by more than Govard and DDEV
differ from one another, so a single "Lando number" would be a claim about our
Landofile wearing the tool's name. The benchmark page publishes what can be
reproduced from a named commit on a named machine; this is not that.

What would add the column: an official Magento recipe whose defaults can be
stated, or a publicly versioned community recipe that can be held constant. Either
gives the measurement a definition that survives a reader running it themselves.

## What can be compared without a stopwatch

| | Govard | Lando |
| :--- | :--- | :--- |
| Magento support | a shipped profile: Varnish, Redis, OpenSearch, RabbitMQ, a separate `php-debug` container, `env.php` wiring | a stack assembled from LAMP or LEMP plus service plugins |
| Configuration | `.govard.yml` | Landofile: `recipe`, `services`, `tooling`, `proxy` |
| Local domains | `*.test` with a local CA | Lando's proxy names |
| Proxy ports | 80 and 443, shared by every Govard project | 80 and 443 |
| Importing an existing project | `govard init --migrate-from ddev` and `--migrate-from warden`, each reporting every key it mapped, defaulted or ignored | nothing to import from here; a Landofile has no Govard equivalent |

## Moving a project from Lando to Govard

There is no `--migrate-from lando`, and the two models do not line up for
automation: a Landofile names services and images, while `.govard.yml` names a
framework and a profile, and the framework decides what runs.

The manual path is short:

```bash
govard init --framework magento2
```

Then set the versions the Landofile declared in `.govard.yml` (`stack.php_version`,
`stack.db_version`), run `govard env up`, and compare what started against what the
Landofile defined. A service that is missing and blocks you is exactly what the
[stack request form](https://github.com/ddtcorex/govard/issues/new?template=request.yml)
exists for.

## When Lando is the better choice

- A mixed team running Drupal, WordPress and Laravel side by side, where one
  Landofile convention across all of them is worth more than per-framework depth.
- You depend on a hosting integration Lando ships as a plugin, such as Pantheon,
  Acquia, Platform.sh or Lagoon.
- You want to describe the stack yourself and would rather compose services than
  accept a profile someone else shipped.

## When Govard is the better choice

- The project is Magento or Mage-OS and you want the stack that makes it serve,
  including Varnish, the search engine and the cache, without assembling it.
- Deploy, named remotes with a write-protected `prod`, audit and snapshots should
  live in the same tool you develop with.
- You are coming from DDEV or Warden, where the importer already does the boring
  part and reports what it could not carry.
