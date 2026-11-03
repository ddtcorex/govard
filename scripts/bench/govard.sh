#!/usr/bin/env bash
# Govard adapter for the benchmark harness.
#
# Measures one arm through govard: start, stop, time to first byte (cached and
# cache-bypassed), static deploy, DI compile, peak memory and the CLI wrapper
# overhead. Writes one JSON object with an array of timed runs.
#
# Two things the harness deliberately separates, because measurement showed they
# otherwise get attributed to the tool:
#   - the first `BENCH_WARMUP` iterations are executed and discarded, so a cold
#     page cache or a fresh generated/ directory never lands in the table;
#   - the wrapper overhead is measured next to the wrapper's own work, so a slow
#     CLI exec does not get reported as a slow Magento command.
#
# Image pulls are not part of the start number: images are warm before the timed
# runs begin, and the JSON records that with "images_warm":true.

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
warmup="${BENCH_WARMUP:-1}"

domain=$(bench_domain_of "$fixture")
[[ -n "$domain" ]] || bench_die "could not read the domain from $fixture/.govard.yml"
project=$(awk -F': *' '/^project_name:/ {print $2; exit}' "$fixture/.govard.yml")
[[ -n "$project" ]] || project=$(basename "$fixture")

readiness_timeout_s=900

govard() {
    (cd "$fixture" && "$binary" "$@")
}

# app_status prints "code time_total" for one request to the storefront.
# Unlocked requests are the ones a visitor makes; a session cookie makes Magento
# and Varnish skip the page cache, which is how the cache-bypassed row is taken.
app_status() {
    curl -sk -o /dev/null -w '%{http_code} %{time_total}' \
        --resolve "$domain:443:127.0.0.1" "https://$domain/" || printf '000 0'
}

# varnish_ban empties the page cache. A session cookie does not bypass this VCL
# (measured: cached and cookie requests both came back in ~16 ms), so a ban is the
# only honest way to measure a cold render.
varnish_ban() {
    local container
    container=$(docker ps --format '{{.Names}}' | grep -E "^${project}-varnish-[0-9]+$" | head -1)
    [[ -n "$container" ]] || return 0
    docker exec "$container" varnishadm ban 'req.url ~ "."' >/dev/null 2>&1 || true
}

wait_for_app() {
    local deadline=$((SECONDS + readiness_timeout_s)) status
    while ((SECONDS < deadline)); do
        status=$(app_status cached | awk '{print $1}')
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

median() {
    sort -n | awk '{a[NR]=$1} END {if (NR==0) {print 0; exit} if (NR%2) print a[(NR+1)/2]; else printf "%.1f\n", (a[NR/2]+a[NR/2+1])/2}'
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

time_ms() {
    local start end
    start=$(bench_now_ms)
    if ! "$@" >/dev/null 2>&1; then
        return 1
    fi
    end=$(bench_now_ms)
    printf '%d\n' $((end - start))
}

# sample_ttfb <mode> <samples> prints "p50 p95" in milliseconds. In uncached mode
# each sample starts from an empty page cache, so it measures a cold render.
sample_ttfb() {
    local mode="$1" count="$2" sample code seconds file
    file=$(mktemp)
    for _ in $(seq 1 "$count"); do
        [[ "$mode" == "uncached" ]] && varnish_ban
        sample=$(app_status)
        code=${sample%% *}
        seconds=${sample##* }
        [[ "$code" == "200" ]] || bench_die "ttfb $mode sample returned $code, refusing to record it"
        awk -v s="$seconds" 'BEGIN {printf "%.3f\n", s * 1000}' >>"$file"
    done
    printf '%s %s\n' "$(percentile "$file" 0.50)" "$(percentile "$file" 0.95)"
    rm -f "$file"
}

# cli_overhead_ms compares the same trivial command run directly in the container
# and through the tool's wrapper. The difference is what the wrapper costs.
cli_overhead_ms() {
    local container raw_vals=() wrap_vals=() raw wrap value
    container=$(docker ps --format '{{.Names}}' | grep -E "^${project}-php-[0-9]+$" | head -1)
    [[ -n "$container" ]] || { printf '0\n'; return 0; }

    for _ in 1 2 3; do
        if value=$(time_ms docker exec "$container" php -r 'echo 1;'); then
            raw_vals+=("$value")
        fi
        if value=$(time_ms bash -c "cd '$fixture' && '$binary' tool php -r 'echo 1;'"); then
            wrap_vals+=("$value")
        fi
    done
    [[ ${#raw_vals[@]} -gt 0 && ${#wrap_vals[@]} -gt 0 ]] || { printf '0\n'; return 0; }

    raw=$(printf '%s\n' "${raw_vals[@]}" | median)
    wrap=$(printf '%s\n' "${wrap_vals[@]}" | median)
    awk -v w="$wrap" -v r="$raw" 'BEGIN {d=w-r; if (d<0) d=0; printf "%.1f\n", d}'
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

total=$((warmup + runs))
bench_log "running $total iteration(s), the first $warmup discarded as warm-up"

runs_json="["
recorded=0
for i in $(seq 1 "$total"); do
    measuring=0
    ((i > warmup)) && measuring=1
    label="iteration $i/$total"
    ((measuring)) || label="$label (warm-up, discarded)"
    bench_log "$label: start"

    start_begin=$(bench_now_ms)
    govard env up >/dev/null 2>&1 || bench_die "govard env up failed on $label"
    wait_for_app || bench_die "storefront never answered on $label"
    start_end=$(bench_now_ms)
    start_ms=$((start_end - start_begin))

    bench_log "$label: ttfb"
    read -r ttfb_p50 ttfb_p95 <<<"$(sample_ttfb cached 50)"
    read -r ttfb_uncached_p50 ttfb_uncached_p95 <<<"$(sample_ttfb uncached 5)"

    bench_log "$label: static deploy"
    static_ms=$(time_ms bash -c "cd '$fixture' && '$binary' tool magento setup:static-content:deploy -f --no-interaction") ||
        bench_die "static-content:deploy failed on $label"

    bench_log "$label: di compile"
    di_ms=$(time_ms bash -c "cd '$fixture' && '$binary' tool magento setup:di:compile") ||
        bench_die "setup:di:compile failed on $label"

    mem_mb=$(peak_mem_mb)
    overhead_ms=$(cli_overhead_ms)

    bench_log "$label: stop"
    stop_begin=$(bench_now_ms)
    govard env down >/dev/null 2>&1 || bench_die "govard env down failed on $label"
    wait_for_stop || bench_die "containers did not stop on $label"
    stop_end=$(bench_now_ms)
    stop_ms=$((stop_end - stop_begin))

    if ((measuring)); then
        recorded=$((recorded + 1))
        [[ "$recorded" -gt 1 ]] && runs_json+=","
        runs_json+=$(printf '{"run":%d,"start_ms":%d,"stop_ms":%d,"ttfb_p50_ms":%s,"ttfb_p95_ms":%s,"ttfb_uncached_p50_ms":%s,"ttfb_uncached_p95_ms":%s,"static_deploy_ms":%d,"di_compile_ms":%d,"peak_mem_mb":%d,"cli_overhead_ms":%s}' \
            "$recorded" "$start_ms" "$stop_ms" "$ttfb_p50" "$ttfb_p95" "$ttfb_uncached_p50" "$ttfb_uncached_p95" "$static_ms" "$di_ms" "$mem_mb" "$overhead_ms")
    fi
done
runs_json+="]"

meta=$(bench_meta_json "$binary" "$REPO_ROOT" "$fixture")
printf '{"target":"govard","stack":"nginx, php, php-debug, varnish, redis, opensearch, mariadb","images_warm":true,"warmup_discarded":%d,"readiness_timeout_s":%d,"meta":%s,"runs":%s}\n' \
    "$warmup" "$readiness_timeout_s" "$meta" "$runs_json" >"$result"
