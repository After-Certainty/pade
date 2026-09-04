#!/usr/bin/env bash
# Milestone 9: resolve capabilities via keeper-secrets-manager (PADE_KSM_FAKE / CI).
# Secret values must not appear in PADE plan/capabilities output.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
MANIFEST="$ROOT/examples/demo-project/pade.yaml"
SHARED_BINDINGS="$ROOT/examples/demo-project/bindings.keeper-secrets-manager.example.yaml"
ALICE_BINDINGS="$ROOT/examples/demo-project/identities/alice.keeper-secrets-manager.bindings.yaml"
BOB_BINDINGS="$ROOT/examples/demo-project/identities/bob.keeper-secrets-manager.bindings.yaml"
WHOAMI="$ROOT/examples/demo-project/scripts/github-whoami"

export PADE_KSM_FAKE=1

die() {
  echo "error: $*" >&2
  exit 1
}

[[ -x "$PADE" ]] || die "pade binary not found at $PADE (run: mise run build)"
chmod +x "$WHOAMI"

assert_no_secret_leak() {
  local file="$1"
  if grep -E 'pade-demo-ksm-token|pade-demo-alice-ksm-token|pade-demo-bob-ksm-token' "$file" >/dev/null; then
    die "PADE output appears to contain KSM credential material: $file"
  fi
}

echo "=== Keeper Secrets Manager shared binding (PADE_KSM_FAKE=1) ==="
unset GITHUB_TOKEN || true
# Bootstrap must not be required in fake mode; ensure child strip path still works when set.
export KSM_CONFIG="fake-bootstrap-must-not-reach-child"

"$PADE" validate -f "$MANIFEST"
"$PADE" plan -f "$MANIFEST" --bindings "$SHARED_BINDINGS" --json >/tmp/pade-ksm-plan.json
assert_no_secret_leak /tmp/pade-ksm-plan.json

"$PADE" capabilities -f "$MANIFEST" --bindings "$SHARED_BINDINGS" --json >/tmp/pade-ksm-capabilities.json
assert_no_secret_leak /tmp/pade-ksm-capabilities.json
grep -Eq '"status"[[:space:]]*:[[:space:]]*"configured"' /tmp/pade-ksm-capabilities.json \
  || die "expected keeper-secrets-manager capability status configured"

shared_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$SHARED_BINDINGS" \
    --capability github.user.read \
    -- /bin/sh -c '
      if [ -n "${KSM_CONFIG:-}" ]; then echo "ksm-config-leaked-to-child"; exit 1; fi
      test "$GITHUB_TOKEN" = "pade-demo-ksm-token" || exit 1
      printf "ksm-ok:shared\n"
    '
)"
test "$shared_out" = "ksm-ok:shared" || die "shared ksm exec failed: $shared_out"

echo "=== KSM Alice/Bob identity refs ==="
"$PADE" plan -f "$MANIFEST" --bindings "$ALICE_BINDINGS" --json >/tmp/pade-ksm-alice-plan.json
"$PADE" plan -f "$MANIFEST" --bindings "$BOB_BINDINGS" --json >/tmp/pade-ksm-bob-plan.json
assert_no_secret_leak /tmp/pade-ksm-alice-plan.json
assert_no_secret_leak /tmp/pade-ksm-bob-plan.json

alice_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$ALICE_BINDINGS" \
    --capability github.user.read \
    -- /bin/sh -c '
      test "$GITHUB_TOKEN" = "pade-demo-alice-ksm-token" || exit 1
      printf "ksm-ok:alice\n"
    '
)"
bob_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$BOB_BINDINGS" \
    --capability github.user.read \
    -- /bin/sh -c '
      test "$GITHUB_TOKEN" = "pade-demo-bob-ksm-token" || exit 1
      printf "ksm-ok:bob\n"
    '
)"
test "$alice_out" = "ksm-ok:alice" || die "alice ksm exec failed: $alice_out"
test "$bob_out" = "ksm-ok:bob" || die "bob ksm exec failed: $bob_out"

echo "=== github-whoami stub via KSM ==="
"$PADE" exec \
  -f "$MANIFEST" \
  --bindings "$SHARED_BINDINGS" \
  --capability github.user.read \
  -- "$WHOAMI" | tee /tmp/pade-ksm-whoami.txt
grep -q 'stub-user' /tmp/pade-ksm-whoami.txt || die "expected stub whoami"

# Redaction: accidental print of the token must not appear in captured stdout.
redact_out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$SHARED_BINDINGS" \
    --capability github.user.read \
    --quiet \
    -- /bin/sh -c 'printf "token=%s\n" "$GITHUB_TOKEN"'
)"
echo "$redact_out" | grep -q 'pade-demo-ksm-token' && die "redaction failed: secret in output"
echo "$redact_out" | grep -q '\[REDACTED\]' || die "expected [REDACTED] placeholder"

echo "dogfood-ksm: ok"
