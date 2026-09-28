#!/usr/bin/env bash
# Fail when a govulncheck report names a vulnerability govard has not triaged.
#
# Why this exists: govulncheck has no exclude flag, and its exit status cannot
# tell "a new finding appeared" apart from "the two upstream-unfixed transitive
# Moby findings are still there". Left alone, the scheduled job is red forever
# and stops being a signal. This reads the text report — where a
# `Vulnerability #N:` block exists only for a finding whose code path reaches
# this repository, while the imported-but-uncalled ones are merely counted at the
# end — subtracts scripts/govulncheck-allowlist.txt, and fails on everything else.
#
# Usage: scripts/govulncheck-gate.sh <report.txt> [allowlist]
set -euo pipefail

report=${1:?usage: govulncheck-gate.sh <report.txt> [allowlist]}
allowlist=${2:-"$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/govulncheck-allowlist.txt"}

[[ -f "$report" ]] || { echo "govulncheck gate: no such report: $report" >&2; exit 2; }
[[ -f "$allowlist" ]] || { echo "govulncheck gate: no such allowlist: $allowlist" >&2; exit 2; }

mapfile -t affecting < <(grep -oE '^Vulnerability #[0-9]+: GO-[0-9]{4}-[0-9]+' "$report" |
    grep -oE 'GO-[0-9]{4}-[0-9]+' | sort -u)
mapfile -t allowed < <(grep -vE '^[[:space:]]*(#|$)' "$allowlist" | awk '{print $1}' | sort -u)

untriaged=()
printf 'govulncheck gate: %d finding(s) reach this repository\n' "${#affecting[@]}"
for id in ${affecting[@]+"${affecting[@]}"}; do
    if printf '%s\n' ${allowed[@]+"${allowed[@]}"} | grep -qx "$id"; then
        printf '  allowed   %s\n' "$id"
    else
        printf '  UNTRIAGED %s\n' "$id"
        untriaged+=("$id")
    fi
done

# An allowlisted finding that stopped being reported means the entry is stale.
for id in ${allowed[@]+"${allowed[@]}"}; do
    if ! printf '%s\n' ${affecting[@]+"${affecting[@]}"} | grep -qx "$id"; then
        printf '  note: %s is allowlisted but no longer reported — remove it from %s\n' \
            "$id" "$(basename "$allowlist")"
    fi
done

if ((${#untriaged[@]} > 0)); then
    {
        echo
        echo "govulncheck gate FAILED: ${#untriaged[@]} untriaged finding(s): ${untriaged[*]}"
        echo "Fix the dependency, or add the id to $(basename "$allowlist") with a reason."
    } >&2
    exit 1
fi

echo "govulncheck gate passed"
