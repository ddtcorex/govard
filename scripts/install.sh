#!/usr/bin/env bash
set -euo pipefail

BINARY_NAME="${BINARY_NAME:-govard}"
INSTALL_DIR_DEFAULT="/usr/local/bin"
INSTALL_DIR="${INSTALL_DIR:-$INSTALL_DIR_DEFAULT}"
MIN_GO_MAJOR=1
MIN_GO_MINOR=24

usage() {
  cat <<'EOF'
Quick installer for Govard.

Usage:
  ./scripts/install.sh [--local] [--dir <path>]

Options:
  --local       Install to ~/.local/bin (no sudo needed)
  --dir <path>  Install to a custom directory
  -h, --help    Show this help

Environment variables:
  BINARY_NAME   Name of the installed binary (default: govard)
  INSTALL_DIR   Installation directory (default: /usr/local/bin)
  BUILD_VERSION Embedded version string (default: git describe output)
EOF
}

can_write_dir() {
  local dir="$1"
  if [[ -d "$dir" ]]; then
    [[ -w "$dir" ]]
    return
  fi
  local parent
  parent="$(dirname "$dir")"
  [[ -d "$parent" && -w "$parent" ]]
}

resolve_build_version() {
  if command -v git >/dev/null 2>&1 &&
    git -C "$REPO_ROOT" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
    git -C "$REPO_ROOT" describe --tags --dirty --always 2>/dev/null || echo "1.0.0"
    return
  fi
  echo "1.0.0"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --local)
      INSTALL_DIR="${HOME}/.local/bin"
      shift
      ;;
    --dir)
      if [[ $# -lt 2 ]]; then
        echo "Error: --dir requires a path."
        usage
        exit 1
      fi
      INSTALL_DIR="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Error: unknown option '$1'"
      usage
      exit 1
      ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "${SCRIPT_DIR}/go.mod" && -f "${SCRIPT_DIR}/cmd/govard/main.go" ]]; then
  REPO_ROOT="$SCRIPT_DIR"
elif [[ -f "${SCRIPT_DIR}/../go.mod" && -f "${SCRIPT_DIR}/../cmd/govard/main.go" ]]; then
  REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
else
  echo "Error: installer must be run from inside the Govard repository."
  exit 1
fi
cd "$REPO_ROOT"

if [[ ! -f "go.mod" || ! -f "cmd/govard/main.go" ]]; then
  echo "Error: installer must be run from the Govard repository root."
  exit 1
fi

if ! command -v go >/dev/null 2>&1; then
  cat <<'EOF'
Error: Go is not installed or not in PATH.
Install Go 1.24+ first: https://go.dev/doc/install
EOF
  exit 1
fi

raw_go_version="$(go env GOVERSION 2>/dev/null || true)"
if [[ -z "$raw_go_version" ]]; then
  raw_go_version="$(go version | awk '{print $3}')"
fi
raw_go_version="${raw_go_version#go}"

go_major="${raw_go_version%%.*}"
go_minor_part="${raw_go_version#*.}"
go_minor="${go_minor_part%%.*}"
go_minor="${go_minor%%[^0-9]*}"

if [[ -z "$go_major" || -z "$go_minor" ]]; then
  echo "Error: unable to parse Go version from '${raw_go_version}'."
  exit 1
fi

if (( go_major < MIN_GO_MAJOR || (go_major == MIN_GO_MAJOR && go_minor < MIN_GO_MINOR) )); then
  echo "Error: Go ${raw_go_version} detected, but Go ${MIN_GO_MAJOR}.${MIN_GO_MINOR}+ is required."
  exit 1
fi

tmp_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmp_dir"
}
trap cleanup EXIT

build_version="${BUILD_VERSION:-$(resolve_build_version)}"
if [[ "${build_version}" == v* ]]; then
  build_version="${build_version#v}"
fi
ldflags="-s -w -X govard/internal/cmd.Version=${build_version}"

echo "Building ${BINARY_NAME}..."
go build -ldflags "${ldflags}" -o "${tmp_dir}/${BINARY_NAME}" ./cmd/govard/main.go

target_dir="$INSTALL_DIR"
target_path="${target_dir%/}/${BINARY_NAME}"

if can_write_dir "$target_dir"; then
  mkdir -p "$target_dir"
  install -m 0755 "${tmp_dir}/${BINARY_NAME}" "$target_path"
elif command -v sudo >/dev/null 2>&1; then
  sudo mkdir -p "$target_dir"
  sudo install -m 0755 "${tmp_dir}/${BINARY_NAME}" "$target_path"
else
  target_dir="${HOME}/.local/bin"
  target_path="${target_dir%/}/${BINARY_NAME}"
  mkdir -p "$target_dir"
  install -m 0755 "${tmp_dir}/${BINARY_NAME}" "$target_path"
  echo "Warning: no write access to '${INSTALL_DIR}' and sudo was not found."
fi

echo "Installed ${BINARY_NAME} to ${target_path}"
echo "Embedded version: ${build_version}"
if ! command -v "${BINARY_NAME}" >/dev/null 2>&1; then
  echo "Add '${target_dir}' to your PATH if needed:"
  echo "  export PATH=\"${target_dir}:\$PATH\""
fi
echo "Run '${BINARY_NAME} --help' to verify."
