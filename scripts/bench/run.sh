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
usage: scripts/bench/run.sh --target <name> --fixture <dir> [--runs <n>] [--warmup <n>] [--no-copy] [--out <dir>]

  --target   tool to measure (govard, ddev); one target per invocation
  --fixture  an installed project to copy for this arm (see scripts/bench/fixture.sh)
  --runs     how many times to repeat the measurement (default 1)
  --warmup   iterations to run and discard before timing (default 1)
  --out      where the JSON results and the per-arm copy are written (default /tmp/govard-bench)
  --no-copy  measure the fixture in place; for tools that cannot see a copied tree
EOF
}

target=""
fixture=""
runs=1
warmup=1
no_copy=0
out="/tmp/govard-bench"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --target) [[ $# -ge 2 ]] || bench_die "--target needs a value"; target="$2"; shift 2 ;;
        --fixture) [[ $# -ge 2 ]] || bench_die "--fixture needs a value"; fixture="$2"; shift 2 ;;
        --runs) [[ $# -ge 2 ]] || bench_die "--runs needs a value"; runs="$2"; shift 2 ;;
        --warmup) [[ $# -ge 2 ]] || bench_die "--warmup needs a value"; warmup="$2"; shift 2 ;;
        --no-copy) no_copy=1; shift ;;
        --out) [[ $# -ge 2 ]] || bench_die "--out needs a value"; out="$2"; shift 2 ;;
        -h | --help) usage; exit 0 ;;
        *) bench_die "unknown argument: $1" ;;
    esac
done

[[ -n "$target" ]] || bench_die "missing --target"
[[ -n "$fixture" ]] || bench_die "missing --fixture"
[[ "$runs" =~ ^[1-9][0-9]*$ ]] || bench_die "--runs must be a positive integer"
[[ "$warmup" =~ ^[0-9]+$ ]] || bench_die "--warmup must be zero or a positive integer"

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

# Each arm measures its own copy of the project. Sharing one tree lets the first
# arm populate pub/static and generated/, so the second arm silently starts warm.
work_root="$out/work"
arm_fixture="$work_root/$target"
if [[ "$no_copy" == "1" ]]; then
    # Some tools already own an isolated tree (a containerised CLI can only see
    # the paths it mounts), so copying would measure a project the tool cannot
    # reach. Those arms pass --no-copy and document their own layout.
    arm_fixture="$fixture"
elif [[ ! -f "$arm_fixture/.govard.yml" ]]; then
    bench_log "copying the fixture to $arm_fixture (own tree for this arm)"
    mkdir -p "$arm_fixture"
    cp -a --reflink=auto "$fixture/." "$arm_fixture/" 2>/dev/null ||
        bench_die "could not copy $fixture to $arm_fixture"
fi
fixture="$arm_fixture"

stamp=$(date +%s)
result="$out/$target-$stamp.json"
summary="$out/$target-$stamp-summary.json"

bench_log "target=$target fixture=$fixture runs=$runs out=$result"

BENCH_BINARY="$binary"
BENCH_FIXTURE="$fixture"
BENCH_REPO_ROOT="$REPO_ROOT"
BENCH_RUNS="$runs"
BENCH_WARMUP="$warmup"
export BENCH_BINARY BENCH_FIXTURE BENCH_REPO_ROOT BENCH_RUNS BENCH_WARMUP

"$adapter" "$result"

bench_log "wrote $result"

command -v go >/dev/null 2>&1 || bench_die "summarising needs Go on PATH, the same toolchain that built govard"
(cd "$REPO_ROOT" && go run ./scripts/bench/report summarize "$result" "$summary") ||
    bench_die "could not summarise $result"
bench_log "wrote $summary"

