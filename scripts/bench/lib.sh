#!/usr/bin/env bash
# Shared helpers for the Govard benchmark harness.
#
# Everything here is deliberately plain bash: the harness has to run on a
# contributor's machine with nothing installed beyond govard, docker, curl and
# git, so a number in the published table can always be reproduced.

set -euo pipefail

# bench_now_ms prints the wall clock in milliseconds.
bench_now_ms() {
    date +%s%3N
}

# bench_die prints a message to stderr and exits non-zero.
bench_die() {
    printf 'bench: %s\n' "$*" >&2
    exit 1
}

# bench_log prints a progress line to stderr, keeping stdout for JSON.
bench_log() {
    printf 'bench: %s\n' "$*" >&2
}

# bench_require_binary builds bin/govard when it is missing and prints its path.
bench_require_binary() {
    local repo_root="$1"
    local binary="$repo_root/bin/govard"

    if [[ ! -x "$binary" ]]; then
        bench_log "bin/govard missing, building it with make build"
        (cd "$repo_root" && make build >/dev/null) || bench_die "make build failed"
    fi
    [[ -x "$binary" ]] || bench_die "bin/govard is not executable after make build"

    printf '%s\n' "$binary"
}

# bench_govard_version prints the version string of a built binary.
bench_govard_version() {
    local binary="$1"
    "$binary" version 2>/dev/null | grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+[^ ]*' | head -1 || true
}

# bench_cpu_model prints the CPU model name.
bench_cpu_model() {
    awk -F': ' '/model name/ {print $2; exit}' /proc/cpuinfo 2>/dev/null || sysctl -n machdep.cpu.brand_string 2>/dev/null || printf 'unknown\n'
}

# bench_ram_gb prints total memory in GiB.
bench_ram_gb() {
    local kb
    kb=$(awk '/MemTotal/ {print $2}' /proc/meminfo 2>/dev/null || true)
    if [[ -n "$kb" ]]; then
        awk -v kb="$kb" 'BEGIN {printf "%d", kb / 1024 / 1024}'
        return
    fi
    printf '0\n'
}

# bench_disk_type reports rot or ssd for the filesystem holding path.
bench_disk_type() {
    local path="$1"
    local device rotational
    device=$(df --output=source "$path" 2>/dev/null | tail -1 || true)
    device=$(basename "$device" 2>/dev/null | sed -E 's/p?[0-9]+$//')
    rotational=$(cat "/sys/block/$device/queue/rotational" 2>/dev/null || true)
    if [[ "$rotational" == "0" ]]; then
        printf 'ssd\n'
    elif [[ "$rotational" == "1" ]]; then
        printf 'hdd\n'
    else
        printf 'unknown\n'
    fi
}

# bench_domain_of prints the primary domain recorded in a fixture's config.
bench_domain_of() {
    local fixture="$1"
    awk -F': *' '/^domain:/ {print $2; exit}' "$fixture/.govard.yml" 2>/dev/null || true
}

# bench_meta_json prints the machine and tool metadata as one JSON object.
# Every published table carries this block, so a reader can tell what the
# numbers were measured on.
bench_meta_json() {
    local binary="$1" repo_root="$2" fixture="$3"
    local govard_version commit docker_version

    govard_version=$(bench_govard_version "$binary")
    commit=$(git -C "$repo_root" rev-parse --short HEAD 2>/dev/null || printf 'unknown')
    docker_version=$(docker version --format '{{.Server.Version}}' 2>/dev/null || printf 'unknown')

    printf '{"machine":{"cpu":"%s","cores":%s,"ram_gb":%s,"disk":"%s"},"os":"%s","docker_version":"%s","govard_version":"%s","commit":"%s","fixture":"%s","started_at":"%s"}' \
        "$(bench_cpu_model)" \
        "$(nproc 2>/dev/null || printf 0)" \
        "$(bench_ram_gb)" \
        "$(bench_disk_type "$repo_root")" \
        "$(uname -sr)" \
        "$docker_version" \
        "${govard_version:-unknown}" \
        "$commit" \
        "$fixture" \
        "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}
