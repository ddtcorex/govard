#!/usr/bin/env bash
# DDEV adapter for the benchmark harness.
#
# Measures the same metrics as the govard adapter against a DDEV project that
# serves the same code and database. DDEV runs its own router on alternate ports
# (8080/8443) so it never fights the Govard proxy for 80/443.
#
# DDEV is never installed by this script: a missing binary is a clear error, not
# a download. The stack this adapter measures is DDEV's Magento default (web and
# db); the govard arm runs the project's full default stack. The JSON records
# that difference so the published table can state it.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=scripts/bench/lib.sh
source "$SCRIPT_DIR/lib.sh"

result="${1:-}"
[[ -n "$result" ]] || bench_die "adapter needs an output file path"

command -v ddev >/dev/null 2>&1 ||
    bench_die "ddev is not on PATH; install it yourself and re-run (this harness never installs another tool)"

fixture="${BENCH_FIXTURE:?BENCH_FIXTURE is not set}"
runs="${BENCH_RUNS:-1}"

config="$fixture/.ddev/config.yaml"
[[ -f "$config" ]] || bench_die "$fixture has no .ddev/config.yaml; configure DDEV in that project first"

project=$(awk -F': *' '/^name:/ {print $2; exit}' "$config" | tr -d '"')
https_port=$(awk -F': *' '/^router_https_port:/ {print $2; exit}' "$config" | tr -d '"')
https_port="${https_port:-8443}"
[[ -n "$project" ]] || bench_die "could not read the DDEV project name from $config"
domain="$project.ddev.site"
base_url="https://$domain:$https_port"

readiness_timeout_s=300

ddev_cmd() {
    (cd "$fixture" && ddev "$@")
}

app_status() {
    curl -skL -o /dev/null -w '%{http_code} %{time_total}' \
        --resolve "$domain:$https_port:127.0.0.1" "$base_url/" || printf '000 0'
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
        if ! docker ps --format '{{.Names}}' | grep -q "^ddev-${project}-"; then
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

peak_mem_mb() {
    docker stats --no-stream --format '{{.Name}} {{.MemUsage}}' 2>/dev/null |
        awk -v prefix="ddev-${project}-" '
            index($1, prefix) == 1 {
                v=$2
                if (v ~ /GiB$/) { sub(/GiB$/,"",v); total += v*1024 }
                else if (v ~ /MiB$/) { sub(/MiB$/,"",v); total += v }
                else if (v ~ /KiB$/) { sub(/KiB$/,"",v); total += v/1024 }
            }
            END { printf "%d\n", total }'
}

time_ms() {
    local start end
    start=$(bench_now_ms)
    if ! "$@" >/dev/null 2>&1; then
        return 1
    fi
    end=$(bench_now_ms)
    printf '%d\n' $((end - start))
}

# DDEV and Govard both write app/etc/env.php. Before measuring DDEV, point the
# file at the DDEV database so the arms never share credentials silently.
point_env_at_ddev() {
    local env_file="$fixture/app/etc/env.php"
    [[ -f "$env_file" ]] || return 0
    sed -i \
        -e "s/'dbname' => '[^']*'/'dbname' => 'db'/" \
        -e "s/'username' => '[^']*'/'username' => 'db'/" \
        -e "s/'password' => '[^']*'/'password' => 'db'/" \
        "$env_file"
}

bench_log "pointing app/etc/env.php at the ddev database"
point_env_at_ddev
cp "$fixture/app/etc/env.php" "$fixture/.bench-env-ddev.php"

bench_log "stopping any running ddev project"
ddev_cmd stop >/dev/null 2>&1 || true
wait_for_stop || bench_die "could not stop the ddev project before measuring"

bench_log "warming images"
ddev_cmd start >/dev/null 2>&1 || true
wait_for_app || bench_die "ddev project never answered before the timed runs"
ddev_cmd stop >/dev/null 2>&1 || true
wait_for_stop || bench_die "could not stop the ddev project after warming"

tmpdir=$(mktemp -d)
trap 'rm -rf "$tmpdir"' EXIT

runs_json="["
for i in $(seq 1 "$runs"); do
    bench_log "run $i/$runs: start"
    start_begin=$(bench_now_ms)
    ddev_cmd start >/dev/null 2>&1 || bench_die "ddev start failed on run $i"
    wait_for_app || bench_die "ddev storefront never answered on run $i"
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
    static_ms=$(time_ms bash -c "cd '$fixture' && ddev exec bin/magento setup:static-content:deploy -f --no-interaction") ||
        bench_die "static-content:deploy failed on run $i"

    bench_log "run $i/$runs: di compile"
    di_ms=$(time_ms bash -c "cd '$fixture' && ddev exec bin/magento setup:di:compile") ||
        bench_die "setup:di:compile failed on run $i"

    mem_mb=$(peak_mem_mb)

    bench_log "run $i/$runs: stop"
    stop_begin=$(bench_now_ms)
    ddev_cmd stop >/dev/null 2>&1 || bench_die "ddev stop failed on run $i"
    wait_for_stop || bench_die "ddev containers did not stop on run $i"
    stop_end=$(bench_now_ms)
    stop_ms=$((stop_end - stop_begin))

    ddev_cmd start >/dev/null 2>&1 || true
    ddev_cmd exec bin/magento cache:flush >/dev/null 2>&1 || true
    ddev_cmd stop >/dev/null 2>&1 || true
    wait_for_stop || true

    [[ "$i" -gt 1 ]] && runs_json+=","
    runs_json+=$(printf '{"run":%d,"start_ms":%d,"stop_ms":%d,"ttfb_p50_ms":%s,"ttfb_p95_ms":%s,"static_deploy_ms":%d,"di_compile_ms":%d,"peak_mem_mb":%d}' \
        "$i" "$start_ms" "$stop_ms" "$p50" "$p95" "$static_ms" "$di_ms" "$mem_mb")
done
runs_json+="]"

meta=$(bench_meta_json "${BENCH_BINARY:-/bin/true}" "$REPO_ROOT" "$fixture")
printf '{"target":"ddev","images_warm":true,"readiness_timeout_s":%d,"stack":"web+db (ddev magento default; no varnish, redis or search)","meta":%s,"runs":%s}\n' \
    "$readiness_timeout_s" "$meta" "$runs_json" >"$result"
