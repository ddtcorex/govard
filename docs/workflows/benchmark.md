---
title: Govard Benchmark
description: "Reproducible Govard performance numbers from a committed harness: start, stop, time to first byte, static deploy, DI compile and memory, on a named machine."
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

Each arm drives its own tool: `govard env up`, `ddev start`, `warden env up`
(through the container described below). The command behind every number is in
`scripts/bench/<tool>.sh`.

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

- with the cache as it ships: Govard 14 ms, DDEV 66 ms;
- with the cache emptied before every sample (a Varnish ban on one side, a
  `cache:flush full_page` on the other): Govard 251 ms, DDEV 75 ms.

So Govard wins the cached request and loses the cold render, because its path has
one more hop (Varnish to nginx to php-fpm) than DDEV's nginx plus php-fpm. Which
one matters depends on whether your visits hit a warm cache.

**CLI wrapper overhead.** Running the same trivial command (`php -r 'echo 1;'`)
directly and through each tool's wrapper gives 46 ms for `govard tool` and 123 ms
for `ddev exec`. This row corrects an earlier claim on this page that Govard's
wrapper added about four seconds: that number came from a raw `docker exec` that
had failed in 1.1 s rather than run. Measuring a no-op both ways is what settles
it, and the harness now does that on every run.

**Static deploy and DI compile.** With separate trees and a discarded warm-up,
static deploy lands at 4.3 s for Govard and 1.9 s for DDEV, and DI compile at
21.0 s against 15.7 s. Earlier published numbers (16.4 s against 2.8 s) were an
artifact: both arms shared one tree, so the first arm generated `pub/static` for
the second, and Govard's `env up` clears `generated/code` and `var/cache`, which
made its first deploy pay for a full regeneration.

**Warden.** Two things about how this arm runs, because they change how to read
its column. Its installer needs root: it chowns an SSH tunnel key, rewrites
`/etc/ssh/ssh_config` and installs a CA into the system trust store. Rather than
patch Warden or ask for sudo, the arm runs the untouched CLI in a container that
mounts the Docker socket, so the stack it starts is still the real one on this
host; the cost of that arrangement is the 213 ms wrapper overhead the table
reports. Its traefik publishes 80 and 443 with no way to move them, so the Govard
proxy is stopped for this arm, and traefik only discovers containers on a network
named `warden`, which the project network is attached to. The stack it runs is the
full one from its own `env-init`: nginx, php-fpm, php-debug, Varnish, Redis,
OpenSearch, RabbitMQ and MariaDB, which is why its memory sits between the other
two.

::: details Reproducing the Warden arm

```bash
# the CLI image: root inside, Docker socket mounted, nothing else from the host
docker build -f scripts/bench/warden-cli.Dockerfile -t warden-cli:bench .

# the environment, once
warden env-init warden-bench magento2      # inside the container, from the project dir
warden env up
warden db import < dump.sql

# the measurement, three timed runs after one discarded warm-up
scripts/bench/run.sh --target warden \
  --fixture "$WARDEN_PROJECT_DIR" --runs 3 --warmup 1 --no-copy \
  --out /tmp/govard-bench
```

`--no-copy` is there because a containerised CLI can only reach the paths it
mounts, so copying the project would measure a tree Warden cannot see.

:::

**Noise.** Ranges are published next to each median. DDEV's start ranged from
14.7 s to 16.0 s across three runs on this machine; treat a
single run as noise.

Per-tool difference pages, which state what each tool does beyond these numbers:
[Govard and DDEV](/compare/ddev), [Govard and Warden](/compare/warden), [Govard and Lando](/compare/lando).

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
- Warden's traefik cannot move off 80 and 443, so the Govard proxy is stopped while that arm runs, and its CLI runs from a container because its installer needs root. Both are stated above and on the row itself.
- Lando has no column, because Lando has no official Magento recipe: a Magento project there is a Landofile the user writes, so a number would describe that file rather than the tool. [Govard and Lando](/compare/lando) explains what would change that.

## Results

<!-- bench:start -->

| Metric | govard | ddev | warden |
| --- | --- | --- | --- |
| Start (up to first 200) | 11044 ms (10756-11766 ms) | 14744 ms (14676-15955 ms) | 11570 ms (10624-12065 ms) |
| Stop | 11239 ms (11200-11297 ms) | 1169 ms (1105-1175 ms) | 3706 ms (3678-3779 ms) |
| TTFB p50 (cached) | 14.2 ms (14.1-15.1 ms) | 65.7 ms (64.6-67.0 ms) | 13.5 ms (12.2-13.6 ms) |
| TTFB p95 (cached) | 17.4 ms (16.5-17.7 ms) | 78.6 ms (75.6-79.1 ms) | 14.8 ms (14.0-17.5 ms) |
| TTFB p50 (cache bypassed) | 250.6 ms (245.4-277.3 ms) | 75.4 ms (71.8-81.2 ms) | 495.7 ms (465.6-541.8 ms) |
| TTFB p95 (cache bypassed) | 273.2 ms (259.2-277.7 ms) | 80.2 ms (72.4-81.4 ms) | 516.9 ms (497.1-567.1 ms) |
| Static deploy | 4266 ms (3480-4473 ms) | 1932 ms (1929-2689 ms) | 7739 ms (6654-8329 ms) |
| DI compile | 21031 ms (20155-22949 ms) | 15726 ms (15057-16362 ms) | 25548 ms (24061-27513 ms) |
| Peak memory | 1601 MB (1586-1683 MB) | 445 MB (444-446 MB) | 1020 MB (1011-1061 MB) |
| CLI wrapper overhead | 46 ms (40-48 ms) | 123 ms (115-138 ms) | 213 ms (192-244 ms) |

- **govard**: 3 run(s), images warm (pulls excluded); stack: nginx, php, php-debug, varnish, redis, opensearch, mariadb. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.3, govard v1.82.0-4-ga76c1fca at commit a76c1fca, measured 2026-11-04T05:06:56Z.
- **ddev**: 3 run(s), images warm (pulls excluded); stack: web (nginx + php-fpm), database; ddev magento default, no varnish, redis or search. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.3, govard v1.82.0-4-ga76c1fca at commit a76c1fca, measured 2026-11-04T05:17:43Z.
- **warden**: 3 run(s), images warm (pulls excluded); stack: traefik, nginx, php-fpm, php-debug, varnish, redis, opensearch, rabbitmq, mariadb; cli in a container. 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.3, govard v1.82.0-4-ga76c1fca at commit a76c1fca, measured 2026-11-04T04:59:34Z.

<!-- bench:end -->

### Re-measured after the v1.84 teardown fix

The govard column above was measured on 2026-11-04 at `a76c1fca`. v1.84.0-beta.3
changed the teardown path (#695 caps the varnish stop grace at 2 s), so the arm was
re-run on 2026-11-27 at `9e4d1f26`, same harness, same machine (12th Gen Intel
i7-1255U, 12 cores, 30 GiB, Docker 29.10.1), three runs after one discarded
warm-up, against a fixture rebuilt from `scripts/bench/fixture.sh`:

| Metric | 2026-11-04 (`a76c1fca`) | 2026-11-27 (`9e4d1f26`) | Read |
| :--- | ---: | ---: | :--- |
| Start (up to first 200) | 11044 ms (10756-11766 ms) | 10627 ms (10506-10802 ms) | inside the range |
| Stop | 11239 ms (11200-11297 ms) | 3978 ms (2961-4122 ms) | 2.8 times shorter |
| TTFB p50 (cached) | 14.2 ms (14.1-15.1 ms) | 13.0 ms (11.7-16.1 ms) | close |
| TTFB p50 (cache bypassed) | 250.6 ms (245.4-277.3 ms) | 215.8 ms (207.5-278.1 ms) | close |
| Static deploy | 4266 ms (3480-4473 ms) | 5207 ms (3801-5552 ms) | see below |
| DI compile | 21031 ms (20155-22949 ms) | 23874 ms (19602-28190 ms) | see below |
| Peak memory | 1601 MB (1586-1683 MB) | 1657 MB (1646-1661 MB) | close |
| CLI wrapper overhead | 46 ms (40-48 ms) | 36 ms (0-55 ms) | close |

Stop is the row the change was for, measured the same way on both dates: `env down`
until no container of the project remains. In the old window one container
dominated it: varnish hung in its shutdown path until Docker's default 10 s grace
killed it. With the grace capped at 2 s the same teardown finishes in 4.0 s, and
the two ranges do not overlap.

Start did not move, which is what the other change predicts: the readiness work
shortens a second `env up` of an already running project (about 2.3 s down to 1.5 s
on a different project with 8 containers), and a cold start waits for Magento to
answer either way.

The compile and deploy rows moved in opposite directions across the two dates.
Part of that is the rebuilt fixture: Mage-OS resolved newer packages on 2026-11-27
than on 2026-11-04, so those two rows carry that release as well as the code
change. Read them as the order of magnitude the section above describes, not as a
before and after.

The ddev and warden columns were not re-run: neither CLI is installed on this
machine, and the harness never installs another tool. Their rows keep the
2026-11-04 window, which is why the table above compares one column at two dates
instead of three columns at one date.

## Reproducing it

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # once, about ten minutes
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` rewrites the table above from the JSON files in `OUT`. It never asks you to type a number, and the JSON keeps the machine, the versions and the commit alongside the measurements.
