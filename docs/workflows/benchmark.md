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

- No competitor is measured yet. The DDEV arm needs the Govard proxy stopped, because `ddev-router` and the Govard proxy both want ports 80 and 443; that adapter is a separate change.
- Nothing here measures parallel work, editor latency or deploy time.

## Results

<!-- bench:start -->

| Metric | govard |
| --- | --- |
| Start (up to first 200) | 19202 ms |
| Stop | 5872 ms |
| TTFB p50 | 26.1 ms |
| TTFB p95 | 38.4 ms |
| Static deploy | 7817.5 ms |
| DI compile | 37504 ms |
| Peak memory | 1644 MB |

- **govard**: 2 run(s), images warm (pulls excluded). 12th Gen Intel(R) Core(TM) i7-1255U, 12 cores, 30 GiB RAM, disk ssd, Linux 7.0.0-42-generic, Docker 29.9.2, govard v1.82.0-2-g85e6bc95 at commit 41bb66c8, measured 2026-11-03T05:14:25Z.

<!-- bench:end -->

## Reproducing it

```bash
make build
scripts/bench/fixture.sh --dir /tmp/govard-bench-fixture     # once, about ten minutes
scripts/bench/run.sh --target govard --fixture /tmp/govard-bench-fixture --runs 3
make bench-report OUT=/tmp/govard-bench
```

`make bench-report` rewrites the table above from the JSON files in `OUT`. It never asks you to type a number, and the JSON keeps the machine, the versions and the commit alongside the measurements.
