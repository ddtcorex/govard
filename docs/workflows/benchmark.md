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

## How to read the numbers

A single measurement on a shared workstation is noise: two runs of this harness
on the same machine have differed by a factor of two on the compile metric. Run
the harness at least three times and publish the median, which is what the table
below does. Treat the numbers as an order of magnitude, not a stopwatch.

## What is not measured

- Nothing here measures parallel work, editor latency or deploy time.
- The two arms do not run the same stack. Govard ran the project's default profile, which includes Varnish, Redis and OpenSearch; DDEV ran its Magento default, which is web and database only. The memory difference is mostly that, and the page says so rather than presenting a single score.
- DDEV runs on alternate router ports (8080/8443) so it never fights the Govard proxy for 80/443; a request to DDEV therefore crosses a different port than a request to Govard.

## Results

<!-- bench:start -->

| Metric | govard | ddev |
| --- | --- | --- |
| Start (up to first 200) | 12626.5 ms | 15041 ms |
| Stop | 11349.5 ms | 1261 ms |
| TTFB p50 | 16.8 ms | 91.4 ms |
| TTFB p95 | 21.3 ms | 108.1 ms |
| Static deploy | 5898 ms | 9571.5 ms |
| DI compile | 35090 ms | 17133.5 ms |
| Peak memory | 1624.5 MB | 506.5 MB |

- **govard**: 2 run(s), images warm (pulls excluded). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-5-gf71a9892 at commit f71a9892, measured 2026-11-03T06:46:03Z.
- **ddev**: 2 run(s), images warm (pulls excluded); stack: web+db (ddev magento default; no varnish, redis or search). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-5-gf71a9892 at commit f71a9892, measured 2026-11-03T06:30:36Z.

<!-- bench:end -->

## Reproducing it

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # once, about ten minutes
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` rewrites the table above from the JSON files in `OUT`. It never asks you to type a number, and the JSON keeps the machine, the versions and the commit alongside the measurements.
