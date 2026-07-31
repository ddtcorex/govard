#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: scripts/build-macos-pkg.sh <version|tag> <arch> [output_dir]

Arguments:
  version|tag   Release version (for example: v1.9.0 or 1.9.0)
  arch          Target architecture: amd64 | arm64
  output_dir    Output directory (default: dist)
EOF
}

if [[ $# -lt 2 || $# -gt 3 ]]; then
  usage
  exit 1
fi

RAW_VERSION="$1"
ARCH="$2"
OUT_DIR="${3:-dist}"

case "$ARCH" in
  amd64|arm64) ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION="${RAW_VERSION#v}"
if [[ -z "$VERSION" ]]; then
  echo "Invalid version: $RAW_VERSION" >&2
  exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

STAGE_ROOT="$TMP_DIR/stage"
BIN_DIR="$STAGE_ROOT/usr/local/bin"
PKG_NAME="govard_${VERSION}_Darwin_${ARCH}.pkg"
PKG_PATH="$OUT_DIR/$PKG_NAME"

mkdir -p "$BIN_DIR" "$OUT_DIR"

LDFLAGS="-s -w -X govard/internal/cmd.Version=${VERSION} -X govard/internal/desktop.Version=${VERSION}"

pushd "$ROOT_DIR" >/dev/null
CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" go build -ldflags "$LDFLAGS" -o "$BIN_DIR/govard" ./cmd/govard/main.go
CGO_ENABLED=0 GOOS=darwin GOARCH="$ARCH" go build -tags desktop -ldflags "$LDFLAGS" -o "$BIN_DIR/govard-desktop" ./cmd/govard-desktop
popd >/dev/null

pkgbuild \
  --root "$STAGE_ROOT" \
  --identifier "com.ddtcorex.govard" \
  --version "$VERSION" \
  --install-location "/" \
  "$PKG_PATH"

echo "Created: $PKG_PATH"

# `govard self-update` refreshes govard-desktop by downloading
# govard-desktop_<version>_Darwin_<arch>.tar.gz (same naming as the Linux
# archive GoReleaser produces) — ship it here since GoReleaser only
# cross-builds govard-desktop for linux.
DESKTOP_ARCHIVE_NAME="govard-desktop_${VERSION}_Darwin_${ARCH}.tar.gz"
DESKTOP_ARCHIVE_PATH="$OUT_DIR/$DESKTOP_ARCHIVE_NAME"
tar -czf "$DESKTOP_ARCHIVE_PATH" -C "$BIN_DIR" govard-desktop

echo "Created: $DESKTOP_ARCHIVE_PATH"
