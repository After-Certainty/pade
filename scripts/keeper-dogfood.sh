#!/usr/bin/env bash
# Milestone 7: resolve capabilities via the keeper provider (fake-keeper / CI).
# Secret values must not appear in PADE plan/capabilities output.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
MANIFEST="$ROOT/examples/demo-project/pade.yaml"
SHARED_BINDINGS="$ROOT/examples/demo-project/bindings.keeper.example.yaml"
ALICE_BINDINGS="$ROOT/examples/demo-project/identities/alice.keeper.bindings.yaml"
BOB_BINDINGS="$ROOT/examples/demo-project/identities/bob.keeper.bindings.yaml"
FAKE_KEEPER="${PADE_KEEPER_BIN:-$ROOT/scripts/fake-keeper.sh}"

export PADE_KEEPER_BIN="$FAKE_KEEPER"

die() {
  echo "error: $*" >&2
  exit 1
}

[[ -x "$PADE" ]] || die "pade binary not found at $PADE (run: mise run build)"
chmod +x "$FAKE_KEEPER"

assert_no_secret_leak() {
  local file="$1"
  if grep -E 'pade-demo-keeper-token|pade-demo-alice-keeper-token|pade-demo-bob-keeper-token' "$file" >/dev/null; then
    die "PADE output appears to contain Keeper credential material: $file"
  fi
}

echo "=== Keeper shared binding (via PADE_KEEPER_BIN) ==="
unset GITHUB_TOKEN || true

"$PADE" validate -f "$MANIFEST"
"$PADE" plan -f "$MANIFEST" --bindings "$SHARED_BINDINGS" --json >/tmp/pade-keeper-plan.json
assert_no_secret_leak /tmp/pade-keeper-plan.json

"$PADE" capabilities -f "$MANIFEST" --bindings "$SHARED_BINDINGS" --json >/tmp/pade-keeper-capabilities.json
assert_no_secret_leak /tmp/pade-keeper-capabilities.json
grep -Eq '"status"[[:space:]]*:[[:space:]]*"configured"' /tmp/pade-keeper-capabilities.json \
  || die "expected keeper capability status configured"

shared_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$SHARED_BINDINGS" \
    --capability github.user.read \
    -- /bin/sh -c '
      test "$GITHUB_TOKEN" = "pade-demo-keeper-token" || exit 1
      printf "keeper-ok:shared\n"
    '
)"
test "$shared_out" = "keeper-ok:shared" || die "shared keeper exec failed: $shared_out"

echo "=== Keeper Alice/Bob identity refs ==="
"$PADE" plan -f "$MANIFEST" --bindings "$ALICE_BINDINGS" --json >/tmp/pade-keeper-alice-plan.json
"$PADE" plan -f "$MANIFEST" --bindings "$BOB_BINDINGS" --json >/tmp/pade-keeper-bob-plan.json
assert_no_secret_leak /tmp/pade-keeper-alice-plan.json
assert_no_secret_leak /tmp/pade-keeper-bob-plan.json

alice_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$ALICE_BINDINGS" \
    --capability github.user.read \
    -- /bin/sh -c '
      test "$GITHUB_TOKEN" = "pade-demo-alice-keeper-token" || exit 1
      printf "keeper-ok:alice\n"
    '
)"
bob_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$BOB_BINDINGS" \
    --capability github.user.read \
    -- /bin/sh -c '
      test "$GITHUB_TOKEN" = "pade-demo-bob-keeper-token" || exit 1
      printf "keeper-ok:bob\n"
    '
)"

test "$alice_out" = "keeper-ok:alice" || die "alice keeper exec failed: $alice_out"
test "$bob_out" = "keeper-ok:bob" || die "bob keeper exec failed: $bob_out"
test "$alice_out" != "$bob_out"

echo "dogfood-keeper: ok"
