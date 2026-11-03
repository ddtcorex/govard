#!/usr/bin/env bash
# DDEV adapter for the benchmark harness.
#
# Measures the same metrics as the govard adapter, on its own copy of the same
# project and database. DDEV runs its own router on alternate ports (8080/8443)
# so it never fights the Govard proxy for 80/443.
#
# DDEV is never installed by this script: a missing binary is a clear error, not
# a download. The stack measured here is DDEV's Magento default (web and db);
# the govard arm runs the project's full default stack, and both JSON files
# record their stack so the published table can say so.

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
warmup="${BENCH_WARMUP:-1}"

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

# flush_page_cache empties Magento's full page cache. DDEV has no reverse proxy
# cache, so this is what "cold render" means on that arm.
flush_page_cache() {
    (cd "$fixture" && ddev exec bin/magento cache:flush full_page >/dev/null 2>&1) || true
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

median() {
    sort -n | awk '{a[NR]=$1} END {if (NR==0) {print 0; exit} if (NR%2) print a[(NR+1)/2]; else printf "%.1f\n", (a[NR/2]+a[NR/2+1])/2}'
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

sample_ttfb() {
    local mode="$1" count="$2" sample code seconds file
    file=$(mktemp)
    for _ in $(seq 1 "$count"); do
        [[ "$mode" == "uncached" ]] && flush_page_cache
        sample=$(app_status)
        code=${sample%% *}
        seconds=${sample##* }
        [[ "$code" == "200" ]] || bench_die "ttfb $mode sample returned $code, refusing to record it"
        awk -v s="$seconds" 'BEGIN {printf "%.3f\n", s * 1000}' >>"$file"
    done
    printf '%s %s\n' "$(percentile "$file" 0.50)" "$(percentile "$file" 0.95)"
    rm -f "$file"
}

# cli_overhead_ms: the same trivial command through `ddev exec` and through a raw
# `docker exec` into the web container.
cli_overhead_ms() {
    local container raw_vals=() wrap_vals=() raw wrap value
    container="ddev-${project}-web"
    docker inspect "$container" >/dev/null 2>&1 || { printf '0\n'; return 0; }

    for _ in 1 2 3; do
        if value=$(time_ms docker exec "$container" php -r 'echo 1;'); then
            raw_vals+=("$value")
        fi
        if value=$(time_ms bash -c "cd '$fixture' && ddev exec php -r 'echo 1;'"); then
            wrap_vals+=("$value")
        fi
    done
    [[ ${#raw_vals[@]} -gt 0 && ${#wrap_vals[@]} -gt 0 ]] || { printf '0\n'; return 0; }

    raw=$(printf '%s\n' "${raw_vals[@]}" | median)
    wrap=$(printf '%s\n' "${wrap_vals[@]}" | median)
    awk -v w="$wrap" -v r="$raw" 'BEGIN {d=w-r; if (d<0) d=0; printf "%.1f\n", d}'
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
    ddev_cmd start >/dev/null 2>&1 || bench_die "ddev start failed on $label"
    wait_for_app || bench_die "ddev storefront never answered on $label"
    start_end=$(bench_now_ms)
    start_ms=$((start_end - start_begin))

    bench_log "$label: ttfb"
    read -r ttfb_p50 ttfb_p95 <<<"$(sample_ttfb cached 50)"
    read -r ttfb_uncached_p50 ttfb_uncached_p95 <<<"$(sample_ttfb uncached 5)"

    bench_log "$label: static deploy"
    static_ms=$(time_ms bash -c "cd '$fixture' && ddev exec bin/magento setup:static-content:deploy -f --no-interaction") ||
        bench_die "static-content:deploy failed on $label"

    bench_log "$label: di compile"
    di_ms=$(time_ms bash -c "cd '$fixture' && ddev exec bin/magento setup:di:compile") ||
        bench_die "setup:di:compile failed on $label"

    mem_mb=$(peak_mem_mb)
    overhead_ms=$(cli_overhead_ms)

    bench_log "$label: stop"
    stop_begin=$(bench_now_ms)
    ddev_cmd stop >/dev/null 2>&1 || bench_die "ddev stop failed on $label"
    wait_for_stop || bench_die "ddev containers did not stop on $label"
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

meta=$(bench_meta_json "${BENCH_BINARY:-/bin/true}" "$REPO_ROOT" "$fixture")
printf '{"target":"ddev","stack":"web (nginx + php-fpm), database; ddev magento default, no varnish, redis or search","images_warm":true,"warmup_discarded":%d,"readiness_timeout_s":%d,"meta":%s,"runs":%s}\n' \
    "$warmup" "$readiness_timeout_s" "$meta" "$runs_json" >"$result"
