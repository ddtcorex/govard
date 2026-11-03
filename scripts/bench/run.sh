#!/usr/bin/env bash
# Govard benchmark harness.
#
# One target per invocation: the harness never installs or starts another tool on
# its own. Every run writes a JSON object; the summary and the published table are
# built from those files, never typed by hand.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=scripts/bench/lib.sh
source "$SCRIPT_DIR/lib.sh"

usage() {
    cat <<'EOF'
usage: scripts/bench/run.sh --target <name> --fixture <dir> [--runs <n>] [--out <dir>]

  --target   tool to measure (govard); one target per invocation
  --fixture  an installed project to measure against (see scripts/bench/fixture.sh)
  --runs     how many times to repeat the measurement (default 1)
  --out      where the JSON results are written (default /tmp/govard-bench)
EOF
}

target=""
fixture=""
runs=1
out="/tmp/govard-bench"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --target) [[ $# -ge 2 ]] || bench_die "--target needs a value"; target="$2"; shift 2 ;;
        --fixture) [[ $# -ge 2 ]] || bench_die "--fixture needs a value"; fixture="$2"; shift 2 ;;
        --runs) [[ $# -ge 2 ]] || bench_die "--runs needs a value"; runs="$2"; shift 2 ;;
        --out) [[ $# -ge 2 ]] || bench_die "--out needs a value"; out="$2"; shift 2 ;;
        -h | --help) usage; exit 0 ;;
        *) bench_die "unknown argument: $1" ;;
    esac
done

[[ -n "$target" ]] || bench_die "missing --target"
[[ -n "$fixture" ]] || bench_die "missing --fixture"
[[ "$runs" =~ ^[1-9][0-9]*$ ]] || bench_die "--runs must be a positive integer"

if [[ ! -d "$fixture" ]]; then
    bench_die "fixture $fixture does not exist; create one with: scripts/bench/fixture.sh --dir $fixture"
fi
if [[ ! -f "$fixture/.govard.yml" ]]; then
    bench_die "fixture $fixture has no .govard.yml; create one with: scripts/bench/fixture.sh --dir $fixture"
fi

adapter="$SCRIPT_DIR/$target.sh"
if [[ ! -x "$adapter" ]]; then
    bench_die "no adapter for target $target (expected $adapter)"
fi

binary=$(bench_require_binary "$REPO_ROOT")
mkdir -p "$out"

stamp=$(date +%s)
result="$out/$target-$stamp.json"
summary="$out/$target-$stamp-summary.json"

bench_log "target=$target fixture=$fixture runs=$runs out=$result"

BENCH_BINARY="$binary"
BENCH_FIXTURE="$fixture"
BENCH_REPO_ROOT="$REPO_ROOT"
BENCH_RUNS="$runs"
export BENCH_BINARY BENCH_FIXTURE BENCH_REPO_ROOT BENCH_RUNS

"$adapter" "$result"

bench_log "wrote $result"

if [[ -x "$SCRIPT_DIR/summarize.sh" ]]; then
    "$SCRIPT_DIR/summarize.sh" "$result" "$summary"
    bench_log "wrote $summary"
fi
