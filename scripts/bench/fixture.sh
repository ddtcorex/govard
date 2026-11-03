#!/usr/bin/env bash
# Create the benchmark fixture: one installed Mage-OS project with sample data.
#
# The fixture is created once and reused by every run. It is keyless (Mage-OS
# installs from repo.mage-os.org), so anyone can rebuild it.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# shellcheck source=scripts/bench/lib.sh
source "$SCRIPT_DIR/lib.sh"

dir=""
framework="mageos"

while [[ $# -gt 0 ]]; do
    case "$1" in
        --dir) [[ $# -ge 2 ]] || bench_die "--dir needs a value"; dir="$2"; shift 2 ;;
        --framework) [[ $# -ge 2 ]] || bench_die "--framework needs a value"; framework="$2"; shift 2 ;;
        -h | --help)
            printf 'usage: scripts/bench/fixture.sh --dir <path> [--framework mageos]\n'
            exit 0
            ;;
        *) bench_die "unknown argument: $1" ;;
    esac
done

[[ -n "$dir" ]] || bench_die "missing --dir"
dir=$(mkdir -p "$dir" && cd "$dir" && pwd)
if [[ -n "$(ls -A "$dir")" ]]; then
    bench_die "$dir is not empty; pick an empty directory for the fixture"
fi

binary=$(bench_require_binary "$REPO_ROOT")

bench_log "creating the fixture in $dir (about ten minutes with sample data)"
(
    cd "$dir"
    "$binary" bootstrap --fresh --framework "$framework" --include-sample --yes < /dev/null
) || bench_die "bootstrap failed; see the output above"

[[ -f "$dir/.govard.yml" ]] || bench_die "fixture is missing .govard.yml after bootstrap"

bench_log "fixture ready at $dir"
printf '%s\n' "$dir"
