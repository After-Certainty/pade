#!/usr/bin/env bash
# Stage B: real Cursor OIDC → local pade-broker → fake KSM → pade exec.
# Runs on a Cursor Cloud Agent VM (identity socket required).
# No PADE_BROKER_FAKE_JWT. No KSM_CONFIG on the agent (broker uses PADE_KSM_FAKE=1).
# Secret values must not appear in plan/capabilities.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
BROKER="${BROKER:-$ROOT/bin/pade-broker}"
MANIFEST="$ROOT/examples/demo-project/pade.yaml"
AUDIENCE="${PADE_BROKER_AUDIENCE:-https://pade-broker.local}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/pade-broker-stage-b.XXXXXX")"
BROKER_LOG="$WORK/broker.log"

cleanup() {
  if [[ -n "${BROKER_PID:-}" ]] && kill -0 "$BROKER_PID" 2>/dev/null; then
    kill "$BROKER_PID" 2>/dev/null || true
    wait "$BROKER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

die() {
  echo "error: $*" >&2
  exit 1
}

need_bin() {
  [[ -x "$1" ]] || die "$2 not found at $1 (run: mise run build)"
}

need_bin "$PADE" "pade"
need_bin "$BROKER" "pade-broker"

# Real Cursor identity only — never use the fake JWT path for Stage B.
unset PADE_BROKER_FAKE_JWT || true
unset KSM_CONFIG || true
unset GITHUB_TOKEN || true

SOCKET="${CURSOR_AGENT_SOCKET:-/run/cursor/api.sock}"
[[ -S "$SOCKET" ]] || die "Cursor identity socket not found at $SOCKET (Stage B requires a Cloud Agent VM)"

command -v python3 >/dev/null || die "python3 is required to parse identity JSON"

echo "=== Stage B: mint Cursor workload identity (safe claims) ==="
ID_JSON="$WORK/identity.json"
"$PADE" identity --audience "$AUDIENCE" --json >"$ID_JSON"
SUBJECT="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); s=(d.get("subject") or "").strip();
assert s, "empty subject"; print(s)' "$ID_JSON")"
REPO_URL="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print((d.get("repoUrl") or "").strip())' "$ID_JSON")"
REPO_URLS_LEN="$(python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(len(d.get("repoUrls") or []))' "$ID_JSON")"

echo "subject:       $SUBJECT"
echo "repo_url:      ${REPO_URL:-"(absent)"}"
echo "repo_urls len: $REPO_URLS_LEN"
echo "audience:      $AUDIENCE"
echo "note:          raw JWT not printed; token authenticates the Cloud Agent workload"

# Fail closed on requireRepoURLs when Cursor does not attest the complete set.
# Stage A on managed Cloud Agents observed repo_url without repo_urls — do not
# treat missing repo_urls as single-repo confinement, and do not authorize from
# repo_url alone. This dogfood therefore binds subject + capability only.
# Production policies should be static allowlists, not minted from the current token.
if [[ "${PADE_STAGE_B_SUBJECT:-}" != "" ]]; then
  [[ "$PADE_STAGE_B_SUBJECT" == "$SUBJECT" ]] \
    || die "attested subject $SUBJECT does not match PADE_STAGE_B_SUBJECT=$PADE_STAGE_B_SUBJECT"
fi

POLICY="$WORK/broker-policy.yaml"
BINDINGS_SRV="$WORK/broker-bindings.yaml"
BINDINGS_AGENT="$WORK/agent-bindings.yaml"

# Pick an ephemeral loopback port.
PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
ENDPOINT="http://127.0.0.1:${PORT}"

cat >"$POLICY" <<EOF
# Generated for Stage B dogfood (session-local). Not portable pade.yaml.
# requireRepoURLs intentionally false: complete repo_urls often absent on
# managed Cloud Agents (see docs/cursor-oidc-broker-dogfood.md Stage A/B).
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: ${AUDIENCE}
policies:
  - subject: "${SUBJECT}"
    requireRepoURLs: false
    capabilities:
      - github.user.read
EOF

cat >"$BINDINGS_SRV" <<'EOF'
# Server-side bindings (broker host). Fake KSM for Stage B local proof.
version: "0.1"
capabilities:
  github.user.read:
    provider: keeper-secrets-manager
    keeperSecretsManager:
      refs:
        GITHUB_TOKEN: "keeper://pade-demo-github/field/password"
EOF

cat >"$BINDINGS_AGENT" <<EOF
# Agent-side broker binding (runtime local). No KSM_CONFIG on the agent VM.
version: "0.1"
capabilities:
  github.user.read:
    provider: broker
    broker:
      endpoint: "${ENDPOINT}"
      audience: "${AUDIENCE}"
EOF

echo "=== Stage B: starting local pade-broker (PADE_KSM_FAKE=1, real JWKS) ==="
PADE_KSM_FAKE=1 "$BROKER" \
  -policy "$POLICY" \
  -bindings "$BINDINGS_SRV" \
  -listen "127.0.0.1:${PORT}" \
  >"$BROKER_LOG" 2>&1 &
BROKER_PID=$!

for _ in $(seq 1 50); do
  if curl -fsS "${ENDPOINT}/healthz" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$BROKER_PID" 2>/dev/null; then
    cat "$BROKER_LOG" >&2 || true
    die "pade-broker exited early"
  fi
  sleep 0.1
done
curl -fsS "${ENDPOINT}/healthz" >/dev/null || {
  cat "$BROKER_LOG" >&2 || true
  die "timed out waiting for broker healthz"
}

echo "=== plan/capabilities via real OIDC + broker (no secret values) ==="
"$PADE" validate -f "$MANIFEST"
"$PADE" plan -f "$MANIFEST" --bindings "$BINDINGS_AGENT" --json >"$WORK/plan.json"
"$PADE" capabilities -f "$MANIFEST" --bindings "$BINDINGS_AGENT" --json >"$WORK/capabilities.json"
if grep -E 'pade-demo-ksm-token|broker-secret|eyJ' "$WORK/plan.json" "$WORK/capabilities.json" >/dev/null; then
  die "plan/capabilities appear to contain secrets or JWTs"
fi
grep -Eq '"status"[[:space:]]*:[[:space:]]*"configured"' "$WORK/capabilities.json" \
  || die "expected broker capability configured"

echo "=== pade exec through broker (real Cursor OIDC) ==="
out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$BINDINGS_AGENT" \
    --capability github.user.read \
    --quiet \
    -- /bin/sh -c '
      if [ -n "${KSM_CONFIG:-}" ]; then echo ksm-leaked; exit 1; fi
      test "$GITHUB_TOKEN" = "pade-demo-ksm-token" || exit 2
      printf "stage-b-ok\n"
    '
)"
test "$out" = "stage-b-ok" || die "exec failed: $out"

echo "dogfood-broker-stage-b: ok"
