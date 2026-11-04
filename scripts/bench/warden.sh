#!/usr/bin/env bash
# Warden adapter for the benchmark harness.
#
# Warden's installer needs root: it chowns an SSH tunnel key, rewrites
# /etc/ssh/ssh_config and installs a CA into the system trust store. Rather than
# patching Warden or asking for sudo, this arm runs the untouched Warden CLI
# inside a container that mounts the Docker socket. The stack it starts is still
# the real one on this host: same daemon, same CPU, same memory.
#
# Two consequences, both stated on the benchmark page:
#   - the CLI runs through `docker exec` into that container, and the overhead of
#     doing so is measured and published instead of hidden;
#   - Warden's traefik publishes 80 and 443 with no way to move them, so the
#     Govard proxy has to be stopped for this arm. The page says so.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=scripts/bench/lib.sh
source "$SCRIPT_DIR/lib.sh"

result="${1:-}"
[[ -n "$result" ]] || bench_die "adapter needs an output file path"

fixture="${BENCH_FIXTURE:?BENCH_FIXTURE is not set}"
runs="${BENCH_RUNS:-1}"
warmup="${BENCH_WARMUP:-1}"

cli_image="${WARDEN_CLI_IMAGE:-warden-cli:bench}"
cli_container="${WARDEN_CLI_CONTAINER:-warden-cli-bench}"
project_dir="${WARDEN_PROJECT_DIR:?set WARDEN_PROJECT_DIR to the Warden project directory}"
warden_home="${WARDEN_HOME_DIR:-$(dirname "$project_dir")/home}"
mount_root="${WARDEN_MOUNT_ROOT:-$(dirname "$project_dir")}"

[[ -f "$fixture/.env" ]] || bench_die "$fixture has no Warden .env; run warden env-init there first"
docker image inspect "$cli_image" >/dev/null 2>&1 || bench_die "$cli_image is missing; build it with the Dockerfile the docs page links"

env_name=$(awk -F= '/^WARDEN_ENV_NAME=/ {print $2}' "$fixture/.env")
[[ -n "$env_name" ]] || bench_die "could not read WARDEN_ENV_NAME from $fixture/.env"

domain=$(awk -F= '/^TRAEFIK_DOMAIN=/ {print $2}' "$fixture/.env")
subdomain=$(awk -F= '/^TRAEFIK_SUBDOMAIN=/ {print $2}' "$fixture/.env")
host="${subdomain:+$subdomain.}$domain"
base_url="https://$host"

readiness_timeout_s=600

# ensure_cli keeps one long-lived container so each warden call costs a docker
# exec rather than a docker run.
ensure_cli() {
    if docker inspect "$cli_container" >/dev/null 2>&1; then
        return 0
    fi
    docker run -d --name "$cli_container" \
        -v /var/run/docker.sock:/var/run/docker.sock \
        -v "$mount_root:$mount_root" \
        -e WARDEN_HOME_DIR="$warden_home" \
        --entrypoint /bin/bash "$cli_image" -c 'sleep infinity' >/dev/null
}

warden() {
    docker exec "$cli_container" bash -lc "cd '$fixture' && warden $*"
}

app_status() {
    curl -sk -o /dev/null -w '%{http_code} %{time_total}' \
        --resolve "$host:443:127.0.0.1" "$base_url/" || printf '000 0'
}

varnish_ban() {
    local container
    container=$(docker ps --format '{{.Names}}' | grep -E "^${env_name}-varnish-[0-9]+$" | head -1)
    [[ -n "$container" ]] || return 0
    docker exec "$container" varnishadm "ban req.http.host ~ \"$host\"" >/dev/null 2>&1 || true
}

wait_for_app() {
    local deadline=$((SECONDS + readiness_timeout_s)) status
    while ((SECONDS < deadline)); do
        status=$(app_status | awk '{print $1}')
        [[ "$status" == "200" ]] && return 0
        sleep 1
    done
    return 1
}

wait_for_stop() {
    local deadline=$((SECONDS + 300))
    while ((SECONDS < deadline)); do
        if ! docker ps --format '{{.Names}}' | grep -q "^${env_name}-"; then
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
        awk -v prefix="${env_name}-" '
            index($1, prefix) == 1 {
                v=$2
                if (v ~ /GiB$/) { sub(/GiB$/,"",v); total += v*1024 }
                else if (v ~ /MiB$/) { sub(/MiB$/,"",v); total += v }
                else if (v ~ /KiB$/) { sub(/KiB$/, "", v); total += v/1024 }
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

# cli_overhead_ms: the same trivial command through the CLI container and through
# a raw docker exec into the php-fpm container.
cli_overhead_ms() {
    local php raw_vals=() wrap_vals=() raw wrap value
    php=$(docker ps --format '{{.Names}}' | grep -E "^${env_name}-php-fpm-[0-9]+$" | head -1)
    [[ -n "$php" ]] || { printf '0\n'; return 0; }

    for _ in 1 2 3; do
        if value=$(time_ms docker exec "$php" php -r 'echo 1;'); then raw_vals+=("$value"); fi
        if value=$(time_ms docker exec "$cli_container" bash -lc "cd '$fixture' && warden env exec php-fpm php -r 'echo 1;'"); then
            wrap_vals+=("$value")
        fi
    done
    [[ ${#raw_vals[@]} -gt 0 && ${#wrap_vals[@]} -gt 0 ]] || { printf '0\n'; return 0; }

    raw=$(printf '%s\n' "${raw_vals[@]}" | median)
    wrap=$(printf '%s\n' "${wrap_vals[@]}" | median)
    awk -v w="$wrap" -v r="$raw" 'BEGIN {d=w-r; if (d<0) d=0; printf "%.1f\n", d}'
}

bench_log "ensuring the Warden CLI container"
ensure_cli

bench_log "stopping any running warden environment"
warden env down >/dev/null 2>&1 || true
wait_for_stop || bench_die "could not stop the warden environment before measuring"

bench_log "warming images"
warden env up >/dev/null 2>&1 || true
wait_for_app || bench_die "warden storefront never answered before the timed runs"
warden env down >/dev/null 2>&1 || true
wait_for_stop || bench_die "could not stop the warden environment after warming"

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
    warden env up >/dev/null 2>&1 || bench_die "warden env up failed on $label"
    wait_for_app || bench_die "warden storefront never answered on $label"
    start_end=$(bench_now_ms)
    start_ms=$((start_end - start_begin))

    bench_log "$label: ttfb"
    read -r ttfb_p50 ttfb_p95 <<<"$(sample_ttfb cached 50)"
    read -r ttfb_uncached_p50 ttfb_uncached_p95 <<<"$(sample_ttfb uncached 5)"

    bench_log "$label: static deploy"
    static_ms=$(time_ms docker exec "$cli_container" bash -lc "cd '$fixture' && warden env exec php-fpm bin/magento setup:static-content:deploy -f --no-interaction") ||
        bench_die "static-content:deploy failed on $label"

    bench_log "$label: di compile"
    di_ms=$(time_ms docker exec "$cli_container" bash -lc "cd '$fixture' && warden env exec php-fpm bin/magento setup:di:compile") ||
        bench_die "setup:di:compile failed on $label"

    mem_mb=$(peak_mem_mb)
    overhead_ms=$(cli_overhead_ms)

    bench_log "$label: stop"
    stop_begin=$(bench_now_ms)
    warden env down >/dev/null 2>&1 || bench_die "warden env down failed on $label"
    wait_for_stop || bench_die "warden containers did not stop on $label"
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
printf '{"target":"warden","stack":"traefik, nginx, php-fpm, php-debug, varnish, redis, opensearch, rabbitmq, mariadb; cli in a container","images_warm":true,"warmup_discarded":%d,"readiness_timeout_s":%d,"meta":%s,"runs":%s}\n' \
    "$warmup" "$readiness_timeout_s" "$meta" "$runs_json" >"$result"
