#!/usr/bin/env bash
# Stage B exec: real Cursor OIDC → local pade-broker → broker-side exec providers
# (github.repo.read + google-analytics.read) → pade exec.
# Runs on a Cursor Cloud Agent VM (identity socket required).
# No PADE_BROKER_FAKE_JWT. No vendor credentials on the agent VM.
#
# Provider fake mode (PADE_PROVIDER_FAKE=1) is the default for offline proof.
# Unset PADE_PROVIDER_FAKE and configure broker-side GITHUB_APP_* /
# GOOGLE_APPLICATION_CREDENTIALS for live API validation.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PADE="${PADE:-$ROOT/bin/pade}"
BROKER="${BROKER:-$ROOT/bin/pade-broker}"
GH_OUT="${ROOT}/bin/pade-provider-github"
GA_OUT="${ROOT}/bin/pade-provider-google-analytics"
AUDIENCE="${PADE_BROKER_AUDIENCE:-https://pade-broker.local}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/pade-broker-stage-b-exec.XXXXXX")"
BROKER_LOG="$WORK/broker.log"
MANIFEST="$WORK/pade.yaml"
REPO="${PADE_DOGFOOD_REPO:-After-Certainty/pade}"
GA_PROPERTY="${GA_PROPERTY_ID:-properties/000000000}"

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
  [[ -x "$1" ]] || die "$2 not found at $1 (run: make build && go build providers)"
}

need_bin "$PADE" "pade"
need_bin "$BROKER" "pade-broker"
need_bin "$GH_OUT" "pade-provider-github"
need_bin "$GA_OUT" "pade-provider-google-analytics"

unset PADE_BROKER_FAKE_JWT || true
unset KSM_CONFIG || true
unset GITHUB_TOKEN || true
unset GA_ACCESS_TOKEN || true

SOCKET="${CURSOR_AGENT_SOCKET:-/run/cursor/api.sock}"
[[ -S "$SOCKET" ]] || die "Cursor identity socket not found at $SOCKET (Stage B exec requires a Cloud Agent VM)"

command -v python3 >/dev/null || die "python3 is required to parse identity JSON"

echo "=== Stage B exec: mint Cursor workload identity (safe claims) ==="
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

if [[ "${PADE_STAGE_B_SUBJECT:-}" != "" ]]; then
  [[ "$PADE_STAGE_B_SUBJECT" == "$SUBJECT" ]] \
    || die "attested subject $SUBJECT does not match PADE_STAGE_B_SUBJECT=$PADE_STAGE_B_SUBJECT"
fi

POLICY="$WORK/broker-policy.yaml"
BINDINGS_SRV="$WORK/broker-bindings.yaml"
BINDINGS_AGENT="$WORK/agent-bindings.yaml"

PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()')"
ENDPOINT="http://127.0.0.1:${PORT}"

cat >"$MANIFEST" <<EOF
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: stage-b-exec-dogfood
spec:
  capabilities:
    github.repo.read:
      access: use
    google-analytics.read:
      access: read
EOF

cat >"$POLICY" <<EOF
# Generated for Stage B exec dogfood (session-local). Not portable pade.yaml.
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: ${AUDIENCE}
policies:
  - subject: "${SUBJECT}"
    requireRepoURLs: false
    capabilities:
      - github.repo.read
      - google-analytics.read
EOF

cat >"$BINDINGS_SRV" <<EOF
# Server-side exec bindings (broker host). Provider credentials stay broker-side.
version: "0.1"
capabilities:
  github.repo.read:
    provider: exec
    exec:
      command:
        - ${GH_OUT}
      config:
        tokenEnv: GITHUB_TOKEN
        repositories:
          - ${REPO}
        permissions:
          metadata: read
          contents: read
  google-analytics.read:
    provider: exec
    exec:
      command:
        - ${GA_OUT}
      config:
        tokenEnv: GA_ACCESS_TOKEN
        propertyId: ${GA_PROPERTY}
EOF

cat >"$BINDINGS_AGENT" <<EOF
# Agent-side broker binding (runtime local). No vendor secrets on the agent VM.
version: "0.1"
capabilities:
  github.repo.read:
    provider: broker
    broker:
      endpoint: "${ENDPOINT}"
      audience: "${AUDIENCE}"
  google-analytics.read:
    provider: broker
    broker:
      endpoint: "${ENDPOINT}"
      audience: "${AUDIENCE}"
EOF

PROVIDER_FAKE="${PADE_PROVIDER_FAKE:-1}"
if [[ "$PROVIDER_FAKE" == "1" ]]; then
  echo "=== Stage B exec: starting broker (PADE_PROVIDER_FAKE=1; offline provider proof) ==="
else
  echo "=== Stage B exec: starting broker (live provider credentials on broker process) ==="
fi

PADE_PROVIDER_FAKE="$PROVIDER_FAKE" "$BROKER" \
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
if grep -E 'ghs_|ya29\.|eyJ|private_key|GITHUB_APP' "$WORK/plan.json" "$WORK/capabilities.json" >/dev/null; then
  die "plan/capabilities appear to contain secrets or JWTs"
fi
grep -Eq '"status"[[:space:]]*:[[:space:]]*"configured"' "$WORK/capabilities.json" \
  || die "expected broker capabilities configured"

caps_out="$("$PADE" capabilities -f "$MANIFEST" --bindings "$BINDINGS_AGENT")"
echo "$caps_out"
echo "$caps_out" | grep -q 'provider: broker' || die "expected provider: broker on Consumer"
if echo "$caps_out" | grep -q 'provider: exec'; then
  die "Consumer must not expose provider: exec"
fi
echo "$caps_out" | grep -q 'github.repo.read' || die "missing github.repo.read"
echo "$caps_out" | grep -q 'google-analytics.read' || die "missing google-analytics.read"

chmod +x \
  "${ROOT}/examples/demo-project/scripts/github-repo-meta" \
  "${ROOT}/examples/demo-project/scripts/ga-property-meta"

echo "=== pade exec: github.repo.read through broker (real Cursor OIDC) ==="
"$PADE" exec -f "$MANIFEST" --bindings "$BINDINGS_AGENT" \
  --capability github.repo.read --quiet -- \
  env GITHUB_REPOSITORY="$REPO" "${ROOT}/examples/demo-project/scripts/github-repo-meta"

echo "=== pade exec: google-analytics.read through broker (real Cursor OIDC) ==="
"$PADE" exec -f "$MANIFEST" --bindings "$BINDINGS_AGENT" \
  --capability google-analytics.read --quiet -- \
  "${ROOT}/examples/demo-project/scripts/ga-property-meta"

echo "dogfood-broker-stage-b-exec: ok"
