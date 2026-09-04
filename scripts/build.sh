#!/usr/bin/env bash
# Local build of bin/pade and bin/pade-broker with version ldflags.
# Uses go from PATH (mise task environment). Does not select or install a toolchain.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GO="${GO:-go}"
VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
BUILD_TIME="${BUILD_TIME:-$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo unknown)}"
LDFLAGS="-s -w \
  -X github.com/After-Certainty/pade/internal/version.Version=${VERSION} \
  -X github.com/After-Certainty/pade/internal/version.Commit=${COMMIT} \
  -X github.com/After-Certainty/pade/internal/version.BuildTime=${BUILD_TIME}"

mkdir -p bin
"$GO" build -ldflags "$LDFLAGS" -o bin/pade ./cmd/pade
"$GO" build -ldflags "$LDFLAGS" -o bin/pade-broker ./cmd/pade-broker
