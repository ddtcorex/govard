#!/usr/bin/env bash
# Proves the Docker-free core contract inside an environment that has neither a
# docker binary nor a docker socket. The only input is the govard binary path.
#
# Contract (see the Docker-free core design):
#   1. A host with no container runtime can run every requirement-free command.
#   2. A command whose requirement is unmet fails fast with exit 3 and a
#      CAPABILITY_MISSING envelope instead of leaking a raw runtime error.
#   3. `govard doctor` reports without failing; `--strict` is the hard gate.
#   4. The container-free analysis check runs here, and the container-backed
#      check fails with the capability contract instead of a raw runtime error.
set -euo pipefail

BIN="${1:-./govard}"
if [ ! -x "$BIN" ]; then
  echo "core-contract: binary not executable: $BIN" >&2
  exit 1
fi
if command -v docker >/dev/null 2>&1; then
  echo "core-contract: docker is present; this environment is not Docker-free" >&2
  exit 1
fi
if [ -S /var/run/docker.sock ]; then
  echo "core-contract: docker socket is mounted; this environment is not Docker-free" >&2
  exit 1
fi
echo "core-contract: environment is Docker-free"

failures=0

# 1. Diagnostics must report on a host without a container runtime, and
#    --strict must remain the explicit hard gate.
if ! "$BIN" doctor >/dev/null 2>&1; then
  echo "core-contract: FAIL govard doctor exited non-zero without docker" >&2
  failures=$((failures + 1))
fi
if "$BIN" doctor --strict >/dev/null 2>&1; then
  echo "core-contract: FAIL govard doctor --strict must fail without docker" >&2
  failures=$((failures + 1))
fi

# 2. The manifest must exist and the core set must be derived from it.
if ! "$BIN" capabilities | grep -q "$(printf '^govard version\tnone\t')"; then
  echo "core-contract: FAIL govard version is not a requirement-free command" >&2
  failures=$((failures + 1))
fi
if ! "$BIN" capabilities --json | grep -q '"schema_version": 1'; then
  echo "core-contract: FAIL govard capabilities --json is not versioned" >&2
  failures=$((failures + 1))
fi

# 3. Every requirement-free command must run, and no command may leak a raw
#    container-runtime error: the gate must own that message.
while IFS=$'\t' read -r command requires status; do
  [ -n "${command:-}" ] || continue
  args="${command#govard }"
  if [ "$requires" = "none" ]; then
    # A requirement-free command must work on this host. Commands that forward
    # their arguments cannot be probed with --help, so only their invocation
    # (below, via the gate branch) is asserted.
    if ! "$BIN" $args --help >/dev/null 2>&1; then
      echo "core-contract: FAIL $command --help exited non-zero" >&2
      failures=$((failures + 1))
    fi
    continue
  fi
  set +e
  out="$("$BIN" $args --error-json 2>&1)"
  code=$?
  set -e
  case "$code" in
    0|2|3) ;;
    *)
      echo "core-contract: FAIL $command exited $code, want 0, 2 or 3" >&2
      failures=$((failures + 1))
      continue
      ;;
  esac
  if printf '%s' "$out" | grep -q "Cannot connect to the Docker daemon" &&
     ! printf '%s' "$out" | grep -q "missing capability"; then
    echo "core-contract: FAIL $command leaked a raw container-runtime error" >&2
    failures=$((failures + 1))
  fi
done < <("$BIN" capabilities)

# 4. Representative gate checks: one command per requirement that can be
#    invoked without arguments and without side effects.
check_gate() {
  local command="$1" expected="$2"
  set +e
  out="$("$BIN" $command --error-json 2>&1)"
  code=$?
  set -e
  if [ "$code" -ne 3 ]; then
    echo "core-contract: FAIL govard $command exited $code, want 3" >&2
    failures=$((failures + 1))
    return
  fi
  if ! printf '%s' "$out" | grep -q '"schema_version"'; then
    echo "core-contract: FAIL govard $command did not emit the error envelope" >&2
    failures=$((failures + 1))
    return
  fi
  if ! printf '%s' "$out" | grep -q "\"capability\": \"$expected\""; then
    echo "core-contract: FAIL govard $command did not name capability $expected" >&2
    failures=$((failures + 1))
  fi
}

check_gate "env up" docker
check_gate "tunnel status" cloudflared
# The toolchain commands inspect, pull, and build a Docker image. They live
# under `audit`, which declares no requirement so the container-free integrity
# check stays runnable here, so the group re-declares docker itself.
check_gate "audit toolchain status" docker
check_gate "audit toolchain pull" docker
check_gate "audit toolchain build" docker

# 5. Container-free analysis. The integrity check reads the checkout directly;
#    the container-backed lint check must refuse with exit 3 and point here.
fixture_dir="/tmp/govard-integrity-fixture"
govard_home="/tmp/govard-contract-home"
rm -rf "$fixture_dir" "$govard_home"
mkdir -p "$fixture_dir/app/code/Acme/Demo/etc" "$fixture_dir/bin"
cat > "$fixture_dir/composer.json" <<'JSON'
{"name":"acme/demo","require":{"php":">=8.1 <8.4","magento/product-community-edition":"2.4.7"}}
JSON
cat > "$fixture_dir/composer.lock" <<'JSON'
{"content-hash":"fixture","packages":[{"name":"magento/product-community-edition","version":"2.4.7"}],"packages-dev":[]}
JSON
printf '#!/usr/bin/env php\n<?php\n' > "$fixture_dir/bin/magento"
chmod +x "$fixture_dir/bin/magento"
cat > "$fixture_dir/app/code/Acme/Demo/registration.php" <<'PHP'
<?php
\Magento\Framework\Component\ComponentRegistrar::register(\Magento\Framework\Component\ComponentRegistrar::MODULE, 'Acme_Demo', __DIR__);
PHP
cat > "$fixture_dir/app/code/Acme/Demo/etc/module.xml" <<'XML'
<?xml version="1.0"?>
<config>
    <module name="Acme_Demo"/>
</config>
XML
cat > "$fixture_dir/.govard.yml" <<'YAML'
project_name: integrity-fixture
domain: integrity-fixture.test
framework: magento2
stack:
  php_version: "8.3"
YAML

set +e
integrity_out="$(cd "$fixture_dir" && GOVARD_HOME_DIR="$govard_home" "$BIN" audit run --checks integrity --format json 2>&1)"
integrity_code=$?
set -e
case "$integrity_code" in
  0|1) ;;
  *)
    echo "core-contract: FAIL govard audit run --checks integrity exited $integrity_code" >&2
    failures=$((failures + 1))
    ;;
esac
if printf '%s' "$integrity_out" | grep -q "Cannot connect to the Docker daemon"; then
  echo "core-contract: FAIL the integrity check touched the container runtime" >&2
  failures=$((failures + 1))
fi
if ! printf '%s' "$integrity_out" | grep -q '"id":"integrity"'; then
  echo "core-contract: FAIL the integrity job did not run:" >&2
  printf '%s\n' "$integrity_out" >&2
  failures=$((failures + 1))
fi

set +e
lint_out="$(cd "$fixture_dir" && GOVARD_HOME_DIR="$govard_home" "$BIN" audit run --checks lint 2>&1)"
lint_code=$?
set -e
if [ "$lint_code" -ne 3 ]; then
  echo "core-contract: FAIL govard audit run --checks lint exited $lint_code, want 3" >&2
  failures=$((failures + 1))
fi
if ! printf '%s' "$lint_out" | grep -q -- "--checks integrity"; then
  echo "core-contract: FAIL the lint gate did not point at the container-free check" >&2
  failures=$((failures + 1))
fi

if [ "$failures" -ne 0 ]; then
  echo "core-contract: $failures failure(s)" >&2
  exit 1
fi
echo "core-contract: OK"
