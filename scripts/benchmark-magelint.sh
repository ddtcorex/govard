#!/usr/bin/env bash
#
# Cold/warm benchmark for the Govard-owned Magento lint backend.
#
# It drives the real `govard audit run` CLI against the checked-in
# tests/integration/projects/magento2/audit-module fixture, once cold and then
# twice warm, for each of the three audit target modes, and emits one JSON object
# per pass (JSONL) with these fields:
#
#   host arch image_digest target_mode php_versions cache_state
#   prepare_ms analyser_ms total_ms peak_disk_bytes cache_bytes
#
# prepare_ms sums the runner's own "validate" and "prepare" phases and
# analyser_ms sums its "phpcs", "compat", and "phpstan" phases, across every PHP
# version in the pass, so the split reports what the runner actually measured
# rather than a wall-clock guess. total_ms is the report's own duration.
# peak_disk_bytes is sampled from the isolated Govard home while the pass runs,
# so it is a real peak rather than a post-hoc size. cache_bytes is the size of
# the target's reusable cache generation once the pass finished.
#
# Everything runs against a private GOVARD_HOME_DIR and a private copy of the
# fixture, so a benchmark never touches ~/.govard and never mutates the repo.
#
# Usage:
#   bash scripts/benchmark-magelint.sh [options]
#
# Options:
#   --output FILE      Write JSONL here instead of standard output.
#   --govard BINARY    Use this govard binary instead of building one.
#   --fixture DIR      Use this fixture tree instead of the checked-in one.
#   --php VERSION      Active PHP for the project and module passes (default 8.3).
#   --warm-passes N    Warm passes per mode (default 2).
#   --keep-workdir     Keep the temporary Govard home and fixture copies.
#   --help             Print this help.
#
# Environment:
#   GOVARD_BENCHMARK_HOST  Overrides the reported host name.
#
# Requirements: docker, python3, and either a govard binary or a Go toolchain.
#
# Copyright (c) Govard contributors.
# Distributed under the terms of the repository LICENSE file.

set -euo pipefail

SCRIPTS_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(dirname -- "$SCRIPTS_DIR")

OUTPUT=""
GOVARD_BIN=""
FIXTURE="$REPO_ROOT/tests/integration/projects/magento2/audit-module"
PROJECT_PHP="8.3"
WARM_PASSES=2
KEEP_WORKDIR=0

# The runner phases that make up each half of a pass. Keeping them here means the
# JSONL split stays in one place if the runner ever grows a phase.
PREPARE_PHASES="validate,prepare"
ANALYSER_PHASES="phpcs,compat,phpstan"

# The documented warm-module budget: a warm module-in-project pass must reach the
# analyzers within ten seconds, and must not have repeated Composer resolution to
# get there.
WARM_MODULE_PREPARE_BUDGET_MS=10000

usage() {
    sed -n '2,42p' "$0" | sed 's/^# \{0,1\}//'
}

fail() {
    printf 'benchmark-magelint: %s\n' "$1" >&2
    exit 1
}

note() {
    printf 'benchmark-magelint: %s\n' "$1" >&2
}

while [ "$#" -gt 0 ]; do
    case "$1" in
        --output) OUTPUT=${2:-}; shift 2 ;;
        --govard) GOVARD_BIN=${2:-}; shift 2 ;;
        --fixture) FIXTURE=${2:-}; shift 2 ;;
        --php) PROJECT_PHP=${2:-}; shift 2 ;;
        --warm-passes) WARM_PASSES=${2:-}; shift 2 ;;
        --keep-workdir) KEEP_WORKDIR=1; shift ;;
        --help|-h) usage; exit 0 ;;
        *) fail "unknown option $1; run --help for usage" ;;
    esac
done

command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required to read lint reports"
docker version >/dev/null 2>&1 || fail "the Docker daemon is unreachable"
[ -d "$FIXTURE" ] || fail "fixture directory $FIXTURE does not exist"
case "$WARM_PASSES" in
    ''|*[!0-9]*) fail "--warm-passes must be a non-negative integer" ;;
esac

if [ -z "$GOVARD_BIN" ]; then
    command -v go >/dev/null 2>&1 || fail "no --govard binary given and no Go toolchain to build one"
    GOVARD_BIN="$REPO_ROOT/bin/govard-benchmark"
    note "building $GOVARD_BIN"
    (cd "$REPO_ROOT" && go build -o "$GOVARD_BIN" ./cmd/govard/main.go)
fi
[ -x "$GOVARD_BIN" ] || fail "govard binary $GOVARD_BIN is not executable"

HOST=${GOVARD_BENCHMARK_HOST:-$(uname -n)}
ARCH=$(uname -m)

WORKDIR=$(mktemp -d "${TMPDIR:-/tmp}/govard-magelint-benchmark.XXXXXX")
GOVARD_HOME="$WORKDIR/govard-home"
mkdir -p "$GOVARD_HOME"
export GOVARD_HOME_DIR="$GOVARD_HOME"

cleanup() {
    if [ "$KEEP_WORKDIR" -eq 1 ]; then
        note "keeping work directory $WORKDIR"
        return
    fi
    rm -rf "$WORKDIR"
}
trap cleanup EXIT

emit() {
    if [ -n "$OUTPUT" ]; then
        printf '%s\n' "$1" >>"$OUTPUT"
    else
        printf '%s\n' "$1"
    fi
}

if [ -n "$OUTPUT" ]; then
    : >"$OUTPUT"
fi

# ---------------------------------------------------------------------------
# Image identity
# ---------------------------------------------------------------------------

# The toolchain image is resolved once, before any timed pass, so no pass pays
# for a first-time build and every row reports the same image identity.
note "resolving the lint toolchain image"
TOOLCHAIN_JSON=$("$GOVARD_BIN" audit toolchain build --format json) ||
    fail "could not resolve the lint toolchain image"
IMAGE_DIGEST=$(printf '%s' "$TOOLCHAIN_JSON" | python3 -c 'import json,sys; print(json.load(sys.stdin)["image_digest"])')
[ -n "$IMAGE_DIGEST" ] || fail "the resolved lint toolchain reported no image digest"
note "lint toolchain image digest $IMAGE_DIGEST"

# ---------------------------------------------------------------------------
# Pass execution
# ---------------------------------------------------------------------------

# sample_disk_peak polls the isolated Govard home until its sentinel file is
# removed, recording the largest size it observed. Sampling rather than measuring
# once at the end is what makes peak_disk_bytes an actual peak: a pass can stage
# an analyzer worktree and then discard it.
sample_disk_peak() {
    local sentinel=$1 destination=$2 peak=0 current
    while [ -e "$sentinel" ]; do
        current=$(du -sb "$GOVARD_HOME" 2>/dev/null | cut -f1)
        if [ -n "${current:-}" ] && [ "$current" -gt "$peak" ]; then
            peak=$current
        fi
        sleep 0.2
    done
    printf '%s' "$peak" >"$destination"
}

directory_bytes() {
    local path=$1
    if [ -d "$path" ]; then
        du -sb "$path" 2>/dev/null | cut -f1
    else
        printf '0'
    fi
}

# run_pass MODE WORKDIR CACHE_STATE_LABEL [extra audit args...]
# Runs one audit, emits its JSONL row, and leaves the decoded metrics in the
# PASS_* variables so the caller can assert on them.
PASS_PREPARE_MS=0
PASS_CACHE_STATE=""
PASS_TARGET_ID=""

run_pass() {
    local mode=$1 pass_dir=$2
    shift 2

    local sentinel="$WORKDIR/pass-running" peak_file="$WORKDIR/pass-peak"
    : >"$sentinel"
    : >"$peak_file"
    sample_disk_peak "$sentinel" "$peak_file" &
    local sampler_pid=$!

    local stdout_file="$WORKDIR/pass-stdout.json"
    local status=0
    ( cd "$pass_dir" && "$GOVARD_BIN" audit run --format json "$@" ) >"$stdout_file" 2>"$WORKDIR/pass-stderr.log" || status=$?

    rm -f "$sentinel"
    wait "$sampler_pid" 2>/dev/null || true
    local peak_disk
    peak_disk=$(cat "$peak_file")
    [ -n "$peak_disk" ] || peak_disk=0

    if [ "$status" -ne 0 ]; then
        note "audit run in $pass_dir failed with exit code $status"
        sed -n '1,20p' "$WORKDIR/pass-stderr.log" >&2
        sed -n '1,20p' "$stdout_file" >&2
        fail "a benchmark pass must succeed; a failing lint run has no comparable timing"
    fi

    local identity
    identity=$(python3 -c '
import json, sys
with open(sys.argv[1]) as handle:
    result = json.load(handle)
print(result["project_id"], result["session_id"], result["run_id"])
' "$stdout_file") || fail "could not decode the audit run result"
    # shellcheck disable=SC2086
    set -- $identity
    local report="$GOVARD_HOME/audit/$1/sessions/$2/runs/$3/report.json"
    [ -f "$report" ] || fail "the run published no report at $report"

    local metrics
    metrics=$(python3 -c '
import json, sys

report_path, prepare_names, analyser_names = sys.argv[1], sys.argv[2], sys.argv[3]
prepare = set(prepare_names.split(","))
analyser = set(analyser_names.split(","))

with open(report_path) as handle:
    report = json.load(handle)

prepare_ms = analyser_ms = 0
states = []
for result in report["php_results"]:
    states.append(result["cache"]["state"])
    for phase in result["phases"]:
        if phase["name"] in prepare:
            prepare_ms += int(phase["duration_ms"])
        elif phase["name"] in analyser:
            analyser_ms += int(phase["duration_ms"])

unique = sorted(set(states))
# A pass reports one cache state only when every PHP version agrees; a mixed
# pass says so explicitly instead of hiding it behind whichever came first.
state = unique[0] if len(unique) == 1 else "mixed:" + "+".join(unique)

print(state)
print(prepare_ms)
print(analyser_ms)
print(int(report["duration_ms"]))
print(",".join(report["selected_php_versions"]))
print(report["target_id"])
' "$report" "$PREPARE_PHASES" "$ANALYSER_PHASES") || fail "could not decode the lint report at $report"

    local cache_state prepare_ms analyser_ms total_ms php_versions target_id
    {
        read -r cache_state
        read -r prepare_ms
        read -r analyser_ms
        read -r total_ms
        read -r php_versions
        read -r target_id
    } <<<"$metrics"

    local cache_bytes
    cache_bytes=$(directory_bytes "$GOVARD_HOME/cache/audit/lint/$target_id")

    PASS_PREPARE_MS=$prepare_ms
    PASS_CACHE_STATE=$cache_state
    PASS_TARGET_ID=$target_id

    emit "$(python3 -c '
import json, sys
keys = ["host", "arch", "image_digest", "target_mode", "php_versions", "cache_state",
        "prepare_ms", "analyser_ms", "total_ms", "peak_disk_bytes", "cache_bytes"]
values = sys.argv[1:]
row = dict(zip(keys, values))
for numeric in ("prepare_ms", "analyser_ms", "total_ms", "peak_disk_bytes", "cache_bytes"):
    row[numeric] = int(row[numeric])
print(json.dumps(row, separators=(",", ":")))
' "$HOST" "$ARCH" "$IMAGE_DIGEST" "$mode" "$php_versions" "$cache_state" \
        "$prepare_ms" "$analyser_ms" "$total_ms" "$peak_disk" "$cache_bytes")"
}

# prepare_mode_tree copies the fixture for one mode and prints the directory the
# audit must be started from, which is what selects the target mode.
prepare_mode_tree() {
    local mode=$1
    local destination="$WORKDIR/$mode"
    rm -rf "$destination"
    cp -a "$FIXTURE" "$destination"
    case "$mode" in
        project)
            printf 'stack:\n  php_version: "%s"\n' "$PROJECT_PHP" >"$destination/project/.govard.local.yml"
            printf '%s' "$destination/project"
            ;;
        module_in_project)
            printf 'stack:\n  php_version: "%s"\n' "$PROJECT_PHP" >"$destination/project/.govard.local.yml"
            printf '%s' "$destination/project/app/code/Govard/AuditSample"
            ;;
        standalone)
            printf '%s' "$destination/standalone"
            ;;
        *) fail "unknown target mode $mode" ;;
    esac
}

WARM_MODULE_PREPARE_MS=""
WARM_MODULE_COMPOSER_BYTES=""

for mode in project module_in_project standalone; do
    pass_dir=$(prepare_mode_tree "$mode")

    # A cold pass needs an empty reusable cache. Only the lint cache is removed:
    # the toolchain image cache stays, because rebuilding the image is not part of
    # what a cold analyzer cache is meant to measure.
    rm -rf "$GOVARD_HOME/cache/audit/lint"
    note "$mode: cold pass"
    run_pass "$mode" "$pass_dir"
    if [ "$PASS_CACHE_STATE" != "cold" ]; then
        fail "$mode cold pass reported cache state $PASS_CACHE_STATE"
    fi

    pass=1
    while [ "$pass" -le "$WARM_PASSES" ]; do
        note "$mode: warm pass $pass"
        run_pass "$mode" "$pass_dir"
        if [ "$PASS_CACHE_STATE" != "warm" ]; then
            fail "$mode warm pass $pass reported cache state $PASS_CACHE_STATE"
        fi
        if [ "$mode" = "module_in_project" ]; then
            WARM_MODULE_PREPARE_MS=$PASS_PREPARE_MS
            WARM_MODULE_COMPOSER_BYTES=$(directory_bytes "$GOVARD_HOME/cache/audit/lint/$PASS_TARGET_ID")
            # A module-in-project target is analyzed straight off the read-only
            # source mount, so no Composer download cache may ever appear for it.
            for generation in "$GOVARD_HOME/cache/audit/lint/$PASS_TARGET_ID"/*; do
                if [ -d "$generation/composer" ]; then
                    fail "a warm module-in-project pass repeated Composer resolution: $generation/composer exists"
                fi
            done
        fi
        pass=$((pass + 1))
    done
done

# ---------------------------------------------------------------------------
# Documented warm-module budget
# ---------------------------------------------------------------------------

if [ -n "$WARM_MODULE_PREPARE_MS" ]; then
    note "warm module-in-project preparation: ${WARM_MODULE_PREPARE_MS}ms (budget ${WARM_MODULE_PREPARE_BUDGET_MS}ms), cache ${WARM_MODULE_COMPOSER_BYTES} bytes, no Composer resolution"
    if [ "$WARM_MODULE_PREPARE_MS" -gt "$WARM_MODULE_PREPARE_BUDGET_MS" ]; then
        fail "a warm module-in-project pass must reach the analyzers within ${WARM_MODULE_PREPARE_BUDGET_MS}ms, took ${WARM_MODULE_PREPARE_MS}ms"
    fi
fi

note "done on $HOST ($ARCH)"
