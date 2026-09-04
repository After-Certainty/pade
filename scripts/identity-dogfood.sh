#!/usr/bin/env bash
# Milestone 5: same portable pade.yaml, two simulated identities, different material.
# Secret values must not appear in PADE plan/capabilities output; the child only
# prints identity-ok:<name> labels after a private match.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
MANIFEST="$ROOT/examples/demo-project/pade.yaml"
ALICE_BINDINGS="$ROOT/examples/demo-project/identities/alice.bindings.yaml"
BOB_BINDINGS="$ROOT/examples/demo-project/identities/bob.bindings.yaml"

if [[ ! -x "$PADE" ]]; then
  echo "error: pade binary not found at $PADE (run: mise run build)" >&2
  exit 1
fi

"$PADE" validate -f "$MANIFEST"

"$PADE" plan -f "$MANIFEST" --bindings "$ALICE_BINDINGS" --json >/tmp/pade-identity-alice-plan.json
"$PADE" plan -f "$MANIFEST" --bindings "$BOB_BINDINGS" --json >/tmp/pade-identity-bob-plan.json

for f in /tmp/pade-identity-alice-plan.json /tmp/pade-identity-bob-plan.json; do
  if grep -E 'pade-demo-alice-token|pade-demo-bob-token' "$f" >/dev/null; then
    echo "error: plan output appears to contain credential material: $f" >&2
    exit 1
  fi
done

run_identity() {
  local name="$1"
  local bindings="$2"
  local token="$3"

  GITHUB_TOKEN="$token" \
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$bindings" \
    --capability github.user.read \
    -- /bin/sh -c '
      expected_token="$1"
      label="$2"
      if [ "$GITHUB_TOKEN" != "$expected_token" ]; then
        echo "identity-mismatch:token" >&2
        exit 1
      fi
      printf "identity-ok:%s\n" "$label"
    ' sh "$token" "$name"
}

alice_out="$(run_identity alice "$ALICE_BINDINGS" "pade-demo-alice-token")"
bob_out="$(run_identity bob "$BOB_BINDINGS" "pade-demo-bob-token")"

test "$alice_out" = "identity-ok:alice"
test "$bob_out" = "identity-ok:bob"
test "$alice_out" != "$bob_out"

echo "dogfood-identity: ok"
