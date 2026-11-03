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

Each arm measures its own copy of the same project and database, and one warm-up
iteration is discarded before timing. Even so, the two columns do not measure the
same work. Read this before drawing a conclusion.

**Container set.** Govard starts seven containers (nginx, php, php-debug,
Varnish, Redis, OpenSearch, MariaDB). DDEV starts two (web, database) plus a
shared router. Peak memory follows that difference almost exactly, and the search
engine alone is most of it.

**Page cache, and what happens without it.** Govard puts Varnish in front, so a
normal request is a cache hit: the response carries `age` and
`x-magento-cache-debug: HIT`. DDEV has no reverse-proxy cache, so the same
request renders PHP. Measured both ways on this machine, the direction flips:

- with the cache as it ships: Govard 17 ms, DDEV 174 ms;
- with the cache emptied before every sample (a Varnish ban on one side, a
  `cache:flush full_page` on the other): Govard 342 ms, DDEV 185 ms.

So Govard wins the cached request and loses the cold render, because its path has
one more hop (Varnish to nginx to php-fpm) than DDEV's nginx plus php-fpm. Which
one matters depends on whether your visits hit a warm cache.

**CLI wrapper overhead.** Running the same trivial command (`php -r 'echo 1;'`)
directly and through each tool's wrapper gives 40 ms for `govard tool` and 331 ms
for `ddev exec`. This row corrects an earlier claim on this page that Govard's
wrapper added about four seconds: that number came from a raw `docker exec` that
had failed in 1.1 s rather than run. Measuring a no-op both ways is what settles
it, and the harness now does that on every run.

**Static deploy and DI compile.** With separate trees and a discarded warm-up,
static deploy lands at 4.9 s for Govard and 5.1 s for DDEV, and DI compile at
31.8 s against 46.0 s. Earlier published numbers (16.4 s against 2.8 s) were an
artifact: both arms shared one tree, so the first arm generated `pub/static` for
the second, and Govard's `env up` clears `generated/code` and `var/cache`, which
made its first deploy pay for a full regeneration.

**Noise.** Ranges are published next to each median. DDEV's start ranged from
21.5 s to 51.3 s across three runs on an otherwise busy workstation; treat a
single run as noise.

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
| Start (up to first 200) | 14713 ms (14667-15316 ms) | 26061 ms (21469-51303 ms) |
| Stop | 11262 ms (5329-11828 ms) | 2641 ms (2341-4987 ms) |
| TTFB p50 (cached) | 16.7 ms (15.8-21.8 ms) | 173.9 ms (114.7-196.0 ms) |
| TTFB p95 (cached) | 21.5 ms (17.5-31.7 ms) | 266.2 ms (147.6-289.5 ms) |
| TTFB p50 (cache bypassed) | 341.6 ms (244.3-405.6 ms) | 184.9 ms (107.3-221.0 ms) |
| TTFB p95 (cache bypassed) | 343.7 ms (255.3-407.9 ms) | 251.1 ms (109.9-286.2 ms) |
| Static deploy | 4875 ms (4608-6156 ms) | 5055 ms (4381-10160 ms) |
| DI compile | 31837 ms (31816-33068 ms) | 46028 ms (35882-73548 ms) |
| Peak memory | 1696 MB (1666-1788 MB) | 446 MB (444-463 MB) |
| CLI wrapper overhead | 40 ms (34-95 ms) | 331 ms (152-812 ms) |

- **govard**: 3 run(s), images warm (pulls excluded); stack: nginx, php, php-debug, varnish, redis, opensearch, mariadb. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-10-gea2e9648-dirty at commit ea2e9648, measured 2026-11-03T09:52:08Z.
- **ddev**: 3 run(s), images warm (pulls excluded); stack: web (nginx + php-fpm), database; ddev magento default, no varnish, redis or search. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-10-gea2e9648-dirty at commit ea2e9648, measured 2026-11-03T10:12:53Z.

<!-- bench:end -->

## Reproducing it

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # once, about ten minutes
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` rewrites the table above from the JSON files in `OUT`. It never asks you to type a number, and the JSON keeps the machine, the versions and the commit alongside the measurements.
