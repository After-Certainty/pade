#!/usr/bin/env bash
# Local / Cloud Agent only: real Keeper Secrets Manager + real GitHub API.
# Not run in CI. Requires ambient KSM_CONFIG and a narrowly scoped KSM Application.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
MANIFEST="$ROOT/examples/demo-project/pade.yaml"
WHOAMI="$ROOT/examples/demo-project/scripts/github-whoami"
BINDINGS_OUT="${PADE_KSM_BINDINGS:-$ROOT/.pade/bindings.ksm.live.yaml}"
RECORD_UID="${KSM_RECORD_UID:-}"
NOTATION="${KSM_NOTATION:-}"

die() {
  echo "error: $*" >&2
  exit 1
}

print_setup() {
  cat >&2 <<EOF
Set up Keeper Secrets Manager + GitHub, then re-run:

  1. Create a classic GitHub PAT with at least read:user:
     https://github.com/settings/tokens

  2. Create a narrowly scoped Secrets Manager Application and shared folder
     containing only the Login record whose password field is that PAT.

  3. Create a KSM device config (bound), base64-encode it, and export:
     export KSM_CONFIG="\$(base64 -w0 ksm-config.json)"   # or Cursor Runtime Secret

  4. Set the record UID (or full notation):
     export KSM_RECORD_UID=<uid>
     # optional override:
     # export KSM_NOTATION="keeper://<uid>/field/password"

  5. mise run dogfood-ksm-live
EOF
}

[[ -x "$PADE" ]] || die "pade binary not found at $PADE (run: mise run build)"
chmod +x "$WHOAMI"

if [[ "${PADE_KSM_FAKE:-}" == "1" || "${PADE_KSM_FAKE:-}" == "true" ]]; then
  die "PADE_KSM_FAKE is set; refuse live dogfood (use mise run dogfood-ksm for fake path)"
fi

if [[ -z "${KSM_CONFIG:-}" ]]; then
  print_setup
  die "KSM_CONFIG is not set"
fi

if [[ -z "$NOTATION" ]]; then
  if [[ -z "$RECORD_UID" ]]; then
    print_setup
    die "KSM_RECORD_UID or KSM_NOTATION is required"
  fi
  NOTATION="keeper://${RECORD_UID}/field/password"
fi

mkdir -p "$(dirname "$BINDINGS_OUT")"
cat >"$BINDINGS_OUT" <<EOF
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "${NOTATION}"
EOF

echo "=== Keeper Secrets Manager live: plan/capabilities (no secret values) ==="
"$PADE" validate -f "$MANIFEST"
"$PADE" plan -f "$MANIFEST" --bindings "$BINDINGS_OUT" --json >/tmp/pade-ksm-live-plan.json
"$PADE" capabilities -f "$MANIFEST" --bindings "$BINDINGS_OUT" --json >/tmp/pade-ksm-live-capabilities.json
grep -Eq '"status"[[:space:]]*:[[:space:]]*"configured"' /tmp/pade-ksm-live-capabilities.json \
  || die "expected keeper-secrets-manager status configured (check KSM_CONFIG + notation)"

echo "=== pade exec → github-whoami (real GitHub API) ==="
unset GITHUB_TOKEN || true
"$PADE" exec \
  -f "$MANIFEST" \
  --bindings "$BINDINGS_OUT" \
  --capability github.user.read \
  -- "$WHOAMI" | tee /tmp/pade-ksm-live-whoami.txt

grep -q 'login:' /tmp/pade-ksm-live-whoami.txt || die "expected github login line"
grep -q 'stub-user' /tmp/pade-ksm-live-whoami.txt && die "got stub-user; expected real GitHub API"

echo "dogfood-ksm-live: ok"
