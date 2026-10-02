#!/usr/bin/env bash
# Create a project fixture for sandbox live tests, once.
#
# A fresh install takes minutes (about 6 on a warm machine), and every sandbox
# rehearsal needs an installed project. Build one with this script, then reuse it
# with `env start` or `snapshot restore` instead of bootstrapping again.
#
# Usage: scripts/sandbox-fixture.sh <dir> [--framework <name>] [--framework-version <v>]
#
# GOVARD_BIN selects the binary (default: govard on PATH). Call a built binary by
# path when testing a branch, never a bare `govard` that may resolve to another copy.
set -euo pipefail

usage() {
    echo "usage: sandbox-fixture.sh <dir> [--framework <name>] [--framework-version <v>]" >&2
    exit 2
}

dir=""
framework="magento2"
version=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --framework) [[ $# -ge 2 ]] || usage; framework="$2"; shift 2 ;;
        --framework-version) [[ $# -ge 2 ]] || usage; version="$2"; shift 2 ;;
        -h | --help) usage ;;
        -*) usage ;;
        *) [[ -z "$dir" ]] || usage; dir="$1"; shift ;;
    esac
done
[[ -n "$dir" ]] || usage

govard="${GOVARD_BIN:-govard}"

if [[ -d "$dir" ]] && [[ -n "$(ls -A "$dir")" ]]; then
    echo "sandbox-fixture: $dir is not empty; a fixture starts from an empty directory" >&2
    exit 2
fi
mkdir -p "$dir"
cd "$dir"

init_args=(init --framework "$framework")
[[ -n "$version" ]] && init_args+=(--framework-version "$version")
init_args+=(-y)

git init -q
"$govard" "${init_args[@]}"
"$govard" bootstrap --fresh -y

# .govard.yml is gitignored in a real project, and the deploy build reads the
# committed tree: force-add the files the sandbox loop needs. A transient identity
# keeps the commit working on a machine with no git identity configured.
tracked=()
for file in .govard.yml composer.json composer.lock app/etc/config.php; do
    [[ -e "$file" ]] && tracked+=("$file")
done
git add -f -- "${tracked[@]}"
git -c user.name=govard-fixture -c user.email=fixture@govard.invalid commit -q -m "fixture: installed $framework project"

cat <<HINT

Fixture ready in $dir. Reuse it instead of bootstrapping again:

  cd $dir
  $govard snapshot create          # once, to restore a clean state later
  $govard env start                # a stopped fixture comes back in seconds
  $govard sandbox up --profile full --php <series>
  $govard deploy build sandbox --revision "\$(git rev-parse HEAD)" --output ./artifact-out --force
  $govard deploy --remote sandbox --build artifact --artifact-dir ./artifact-out --revision "\$(git rev-parse HEAD)" --yes

The sandbox keeps its database between runs: pass --reseed to sandbox up to
refresh it from the origin, or sandbox down --purge to discard it.
HINT
