---
title: Govard Benchmark
description: Reproducible Govard performance numbers from a committed harness; start, stop, time to first byte, static deploy, DI compile and memory, measured on a named machine.
---

# Benchmark

Govard's performance claims are worth exactly as much as the harness behind them. This page publishes numbers produced by `scripts/bench/`, on a named machine, from a named commit, with the commands that produced them. If you can run the same commands on a comparable machine, you should get numbers in the same range.

## What is measured

Each run measures one tool against one prepared project, from a stopped state:

| Metric | Definition |
| :--- | :--- |
| Start | `govard env up` until the storefront answers 200, images already warm |
| Stop | `govard env down` until no container of the project remains |
| TTFB p50 / p95 | 50 sequential requests to the home page; every sample must return 200 |
| Static deploy | `bin/magento setup:static-content:deploy -f` inside the container |
| DI compile | `bin/magento setup:di:compile` inside the container |
| Peak memory | Sum of the project containers' resident memory at steady state |

Image pulls are excluded from the start number. The provenance block below says so, and a run that includes pulls is labelled `cold`.

## Why the numbers differ

The two arms do not run the same stack, and two rows measure the wrapper rather
than the work. Read this before comparing columns; none of it is a claim about
which tool is faster.

**Container set.** Govard starts seven containers (nginx, php, php-debug,
Varnish, Redis, OpenSearch, MariaDB). DDEV starts two (web, database) plus a
shared router. Peak memory tracks that difference almost exactly, and the search
engine alone is most of it.

**Page cache.** Govard puts Varnish in front, so a storefront request is normally
a cache hit: the response carries `age` and `x-magento-cache-debug: HIT`. DDEV has
no Varnish, so the same request renders PHP. The TTFB rows therefore compare a
proxy hit against a PHP render, not two PHP renders. Forcing a miss on Govard
needs a cache ban or a session cookie, because its VCL ignores query strings in
the cache key.

**CLI overhead.** Static deploy and DI compile run through each tool's exec
wrapper. Measured directly on this machine: raw `docker exec` 1.1 s, `ddev exec`
2.4 to 3.2 s, `govard tool` 5.1 to 5.4 s. Most of the static deploy row is
wrapper overhead, not Magento.

**A shared project tree.** Both arms measured the same directory, so `pub/static`
was already populated by whichever ran first, and Govard's `env up` clears
`generated/code` and `var/cache`. That is why Govard's first deploy took 58 s and
the next ones 5 s, while DDEV started warm. The harness must give each arm its
own copy before this row can be compared.

**Noise.** TTFB samples for the same page ranged from 16 to 111 ms on this host
during the investigation. Two runs of two samples cannot separate a 1.5x
difference from noise.

## How to read the numbers

A single measurement on a shared workstation is noise: two runs of this harness
on the same machine have differed by a factor of two on the compile metric, and
by more than five on the govard static deploy. Run the harness at least three
times and publish the median, which is what the table below does; the range next
to each median is the min and max observed, and a wide range means the machine
was busy, not that the tool was slow. Treat the numbers as an order of magnitude,
not a stopwatch.

## What is not measured

- Nothing here measures parallel work, editor latency or deploy time.
- The two arms do not run the same stack. Govard ran the project's default profile, which includes Varnish, Redis and OpenSearch; DDEV ran its Magento default, which is web and database only. The memory difference is mostly that, and the page says so rather than presenting a single score.
- DDEV runs on alternate router ports (8080/8443) so it never fights the Govard proxy for 80/443; a request to DDEV therefore crosses a different port than a request to Govard.

## Results

<!-- bench:start -->

| Metric | govard | ddev |
| --- | --- | --- |
| Start (up to first 200) | 8958 ms (7209-10707 ms) | 22113 ms (17880-26346 ms) |
| Stop | 11348.5 ms (11188-11509 ms) | 2519.5 ms (1604-3435 ms) |
| TTFB p50 | 17.5 ms (16.4-18.6 ms) | 108.6 ms (97.2-120.0 ms) |
| TTFB p95 | 21.4 ms (20.6-22.2 ms) | 144.5 ms (133.9-155.0 ms) |
| Static deploy | 16433.5 ms (5015-27852 ms) | 2771.5 ms (2441-3102 ms) |
| DI compile | 24425.5 ms (18888-29963 ms) | 23280.5 ms (20697-25864 ms) |
| Peak memory | 1695.5 MB (1644-1747 MB) | 430 MB (416-444 MB) |

- **govard**: 2 run(s), images warm (pulls excluded). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-6-g85a08e53 at commit 85a08e53, measured 2026-11-03T07:22:37Z.
- **ddev**: 2 run(s), images warm (pulls excluded); stack: web+db (ddev magento default; no varnish, redis or search). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-6-g85a08e53 at commit 85a08e53, measured 2026-11-03T07:30:39Z.

<!-- bench:end -->

## Reproducing it

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # once, about ten minutes
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` rewrites the table above from the JSON files in `OUT`. It never asks you to type a number, and the JSON keeps the machine, the versions and the commit alongside the measurements.
