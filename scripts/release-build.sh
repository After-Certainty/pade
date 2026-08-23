#!/usr/bin/env bash
# Build Milestone I release artifacts: pade + pade-broker for linux/amd64,
# linux/arm64, and darwin/arm64. Writes dist/<version>/ archives and SHA-256 checksums.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="${VERSION:?VERSION is required (e.g. v0.1.0)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
GO="${GO:-go}"
DIST="$ROOT/dist/${VERSION}"

case "$VERSION" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *)
    echo "error: VERSION must look like v0.1.0 (got: $VERSION)" >&2
    exit 1
    ;;
esac

LDFLAGS="-s -w \
  -X github.com/After-Certainty/pade/internal/version.Version=${VERSION} \
  -X github.com/After-Certainty/pade/internal/version.Commit=${COMMIT} \
  -X github.com/After-Certainty/pade/internal/version.BuildTime=${BUILD_TIME}"

rm -rf "$DIST"
mkdir -p "$DIST"

build_one() {
  local goos="$1" goarch="$2"
  local out="$DIST/${goos}-${goarch}"
  mkdir -p "$out"
  echo "=== building ${goos}/${goarch} ==="
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    "$GO" build -ldflags "$LDFLAGS" -o "$out/pade" ./cmd/pade
  GOOS="$goos" GOARCH="$goarch" CGO_ENABLED=0 \
    "$GO" build -ldflags "$LDFLAGS" -o "$out/pade-broker" ./cmd/pade-broker
  tar -C "$out" -czf "$DIST/pade-${VERSION}-${goos}-${goarch}.tar.gz" pade pade-broker
}

build_one linux amd64
build_one linux arm64
build_one darwin arm64

(
  cd "$DIST"
  sha256sum pade-"${VERSION}"-*.tar.gz > SHA256SUMS
)

echo "ok: release artifacts in $DIST"
ls -la "$DIST"
