#!/usr/bin/env bash
# Govard adapter for the benchmark harness.
#
# Measures a stopped fixture through govard only: start, stop, time to first
# byte, static deploy, DI compile and peak memory. Writes one JSON object with
# an array of runs.
#
# Image pulls are deliberately not part of the start number: images are warm
# before the timed runs begin, and the JSON records that with "images_warm":true.
# A reader who wants pull time back can time `govard env up` on a cold machine.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=scripts/bench/lib.sh
source "$SCRIPT_DIR/lib.sh"

result="${1:-}"
[[ -n "$result" ]] || bench_die "adapter needs an output file path"

binary="${BENCH_BINARY:?BENCH_BINARY is not set}"
fixture="${BENCH_FIXTURE:?BENCH_FIXTURE is not set}"
runs="${BENCH_RUNS:-1}"

domain=$(bench_domain_of "$fixture")
[[ -n "$domain" ]] || bench_die "could not read the domain from $fixture/.govard.yml"
project=$(awk -F': *' '/^project_name:/ {print $2; exit}' "$fixture/.govard.yml")
[[ -n "$project" ]] || project=$(basename "$fixture")

readiness_timeout_s=900

govard() {
    (cd "$fixture" && "$binary" "$@")
}

# app_status prints "code time_total" for one request to the storefront.
app_status() {
    curl -sk -o /dev/null -w '%{http_code} %{time_total}' \
        --resolve "$domain:443:127.0.0.1" "https://$domain/" || printf '000 0'
}

wait_for_app() {
    local deadline=$((SECONDS + readiness_timeout_s)) status
    while ((SECONDS < deadline)); do
        status=$(app_status | awk '{print $1}')
        [[ "$status" == "200" ]] && return 0
        sleep 0.5
    done
    return 1
}

wait_for_stop() {
    local deadline=$((SECONDS + 300))
    while ((SECONDS < deadline)); do
        if ! docker ps --format '{{.Names}}' | grep -q "^${project}"; then
            return 0
        fi
        sleep 0.5
    done
    return 1
}

percentile() {
    local file="$1" p="$2"
    sort -n "$file" | awk -v p="$p" '{a[NR]=$1} END {if (NR==0) {print 0; exit} print a[int((NR-1)*p)+1]}'
}

# peak_mem_mb sums the resident memory of the project's containers.
peak_mem_mb() {
    docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' 2>/dev/null |
        awk -v prefix="$project" '
            $1 ~ "^"prefix {
                v=$2
                if (v ~ /GiB$/) { sub(/GiB$/,"",v); total += v*1024 }
                else if (v ~ /MiB$/) { sub(/MiB$/,"",v); total += v }
                else if (v ~ /KiB$/) { sub(/KiB$/,"",v); total += v/1024 }
            }
            END { printf "%d\n", total }'
}

# time_ms runs one measured command and fails loudly when it fails: a command
# that errors in 20ms is not a fast build, and must never reach the table.
time_ms() {
    local start end
    start=$(bench_now_ms)
    if ! "$@" >/dev/null 2>&1; then
        return 1
    fi
    end=$(bench_now_ms)
    printf '%d\n' $((end - start))
}

# The DDEV arm writes app/etc/env.php too. Each arm keeps its own snapshot so a
# measurement never runs against the other tool's database by accident.
restore_env_snapshot() {
    local snapshot="$fixture/.bench-env-govard.php" env_file="$fixture/app/etc/env.php"
    if [[ -f "$snapshot" ]]; then
        cp "$snapshot" "$env_file"
    elif [[ -f "$env_file" ]]; then
        cp "$env_file" "$snapshot"
    fi
}

bench_log "restoring this arm's app/etc/env.php"
restore_env_snapshot

bench_log "stopping any running fixture"
govard env down >/dev/null 2>&1 || true
wait_for_stop || bench_die "could not stop the fixture before measuring"

bench_log "warming images"
govard env up >/dev/null 2>&1 || true
wait_for_app || bench_die "fixture never answered before the timed runs"
govard env down >/dev/null 2>&1 || true
wait_for_stop || bench_die "could not stop the fixture after warming"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

runs_json="["
for i in $(seq 1 "$runs"); do
    bench_log "run $i/$runs: start"

    start_begin=$(bench_now_ms)
    govard env up >/dev/null 2>&1 || bench_die "govard env up failed on run $i"
    wait_for_app || bench_die "storefront never answered on run $i"
    start_end=$(bench_now_ms)
    start_ms=$((start_end - start_begin))

    bench_log "run $i/$runs: ttfb"
    : >"$tmpdir/ttfb"
    for _ in $(seq 1 50); do
        sample=$(app_status)
        code=${sample%% *}
        seconds=${sample##* }
        [[ "$code" == "200" ]] || bench_die "ttfb sample returned $code, refusing to record it"
        awk -v s="$seconds" 'BEGIN {printf "%.3f\n", s * 1000}' >>"$tmpdir/ttfb"
    done
    p50=$(percentile "$tmpdir/ttfb" 0.50)
    p95=$(percentile "$tmpdir/ttfb" 0.95)

    bench_log "run $i/$runs: static deploy"
    static_ms=$(time_ms bash -c "cd '$fixture' && '$binary' tool magento setup:static-content:deploy -f --no-interaction") ||
        bench_die "static-content:deploy failed on run $i"

    bench_log "run $i/$runs: di compile"
    di_ms=$(time_ms bash -c "cd '$fixture' && '$binary' tool magento setup:di:compile") ||
        bench_die "setup:di:compile failed on run $i"

    mem_mb=$(peak_mem_mb)

    bench_log "run $i/$runs: stop"
    stop_begin=$(bench_now_ms)
    govard env down >/dev/null 2>&1 || bench_die "govard env down failed on run $i"
    wait_for_stop || bench_die "containers did not stop on run $i"
    stop_end=$(bench_now_ms)
    stop_ms=$((stop_end - stop_begin))

    # Drop any warm page cache so the next run starts cold.
    govard env up >/dev/null 2>&1 || true
    govard tool magento cache:flush >/dev/null 2>&1 || true
    govard env down >/dev/null 2>&1 || true
    wait_for_stop || true

    [[ "$i" -gt 1 ]] && runs_json+=","
    runs_json+=$(printf '{"run":%d,"start_ms":%d,"stop_ms":%d,"ttfb_p50_ms":%s,"ttfb_p95_ms":%s,"static_deploy_ms":%d,"di_compile_ms":%d,"peak_mem_mb":%d}' \
        "$i" "$start_ms" "$stop_ms" "$p50" "$p95" "$static_ms" "$di_ms" "$mem_mb")
done
runs_json+="]"

meta=$(bench_meta_json "$binary" "$REPO_ROOT" "$fixture")
printf '{"target":"govard","images_warm":true,"readiness_timeout_s":%d,"meta":%s,"runs":%s}\n' \
    "$readiness_timeout_s" "$meta" "$runs_json" >"$result"
