#!/usr/bin/env bash
# Milestone / Phase 2: fake Cursor OIDC + pade-broker + fake KSM + pade exec.
# No real Cursor or Keeper. Secret values must not appear in plan/capabilities.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
MANIFEST="$ROOT/examples/demo-project/pade.yaml"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/pade-broker-dogfood.XXXXXX")"
HELPER_LOG="$WORK/helper.log"

cleanup() {
  if [[ -n "${HELPER_PID:-}" ]] && kill -0 "$HELPER_PID" 2>/dev/null; then
    kill "$HELPER_PID" 2>/dev/null || true
    wait "$HELPER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

die() {
  echo "error: $*" >&2
  exit 1
}

[[ -x "$PADE" ]] || die "pade binary not found at $PADE (run: mise run build)"

echo "=== broker dogfood: starting fake JWKS + pade-broker ==="
go run "$ROOT/scripts/broker-dogfood-helper" "$WORK" >"$HELPER_LOG" 2>&1 &
HELPER_PID=$!

ENV_FILE=""
for _ in $(seq 1 50); do
  if [[ -f "$WORK/dogfood.env" ]]; then
    ENV_FILE="$WORK/dogfood.env"
    break
  fi
  if ! kill -0 "$HELPER_PID" 2>/dev/null; then
    cat "$HELPER_LOG" >&2 || true
    die "broker dogfood helper exited early"
  fi
  sleep 0.1
done
[[ -n "$ENV_FILE" ]] || die "timed out waiting for dogfood.env"

# shellcheck disable=SC1090
source "$ENV_FILE"
[[ -n "${PADE_BROKER_FAKE_JWT:-}" ]] || die "PADE_BROKER_FAKE_JWT missing"
[[ -n "${PADE_BINDINGS:-}" ]] || die "PADE_BINDINGS missing"
export PADE_BROKER_FAKE_JWT PADE_BINDINGS

# Agent VM must not need KSM_CONFIG in broker mode.
unset KSM_CONFIG || true
unset GITHUB_TOKEN || true

echo "=== plan/capabilities via broker (no secret values) ==="
"$PADE" validate -f "$MANIFEST"
"$PADE" plan -f "$MANIFEST" --bindings "$PADE_BINDINGS" --json >"$WORK/plan.json"
"$PADE" capabilities -f "$MANIFEST" --bindings "$PADE_BINDINGS" --json >"$WORK/capabilities.json"
if grep -E 'pade-demo-ksm-token|broker-secret|eyJ' "$WORK/plan.json" "$WORK/capabilities.json" >/dev/null; then
  die "plan/capabilities appear to contain secrets or JWTs"
fi
grep -Eq '"status"[[:space:]]*:[[:space:]]*"configured"' "$WORK/capabilities.json" \
  || die "expected broker capability configured"

echo "=== pade exec through broker ==="
out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$PADE_BINDINGS" \
    --capability github.user.read \
    --quiet \
    -- /bin/sh -c '
      if [ -n "${KSM_CONFIG:-}" ]; then echo ksm-leaked; exit 1; fi
      test "$GITHUB_TOKEN" = "pade-demo-ksm-token" || exit 2
      printf "broker-ok\n"
    '
)"
test "$out" = "broker-ok" || die "exec failed: $out"

echo "dogfood-broker: ok"
