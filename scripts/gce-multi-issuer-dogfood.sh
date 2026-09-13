#!/usr/bin/env bash
# Manual Experiment 005C: real GCE metadata identity → multi-issuer pade-broker.
# Not CI. Requires GCE metadata. Never prints JWTs.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

PADE="${PADE:-$ROOT/bin/pade}"
BROKER="${BROKER:-$ROOT/bin/pade-broker}"
AUDIENCE="${PADE_BROKER_AUDIENCE:-https://pade-broker.local}"
LISTEN="${PADE_BROKER_LISTEN:-127.0.0.1:8787}"
CAPABILITY="${PADE_GCE_CAPABILITY:-experiment.gce.identity}"

die() { echo "error: $*" >&2; exit 1; }

if [[ ! -x "$PADE" || ! -x "$BROKER" ]]; then
  echo "building pade and pade-broker..."
  mise run build
fi
[[ -x "$PADE" && -x "$BROKER" ]] || die "binaries missing after build"

# Probe metadata without printing the token.
meta_url='http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/identity'
aud_q="$(python3 -c 'import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1], safe=""))' "$AUDIENCE")"
meta_code="$(curl -sS -o /tmp/pade-gce-id.jwt -w '%{http_code}' \
  -H 'Metadata-Flavor: Google' \
  "${meta_url}?audience=${aud_q}&format=full" \
  || true)"
if [[ "$meta_code" != "200" ]]; then
  rm -f /tmp/pade-gce-id.jwt
  die "GCE metadata identity unavailable (http ${meta_code:-curl-failed}). Run on a GCE VM."
fi

GCE_SUB="$(python3 - <<'PY'
import base64, json, sys
raw = open("/tmp/pade-gce-id.jwt", "rb").read().decode().strip()
parts = raw.split(".")
if len(parts) != 3:
    sys.exit("malformed jwt from metadata")
pad = "=" * ((4 - len(parts[1]) % 4) % 4)
payload = json.loads(base64.urlsafe_b64decode(parts[1] + pad))
sub = (payload.get("sub") or "").strip()
if not sub:
    sys.exit("metadata token missing sub")
print(sub)
PY
)"
rm -f /tmp/pade-gce-id.jwt
echo "GCE metadata identity available (sub redacted; length=${#GCE_SUB})"

WORKDIR="$(mktemp -d "${TMPDIR:-/tmp}/pade-gce-multi-XXXXXX")"
cleanup() {
  if [[ -n "${BROKER_PID:-}" ]] && kill -0 "$BROKER_PID" 2>/dev/null; then
    kill "$BROKER_PID" 2>/dev/null || true
    wait "$BROKER_PID" 2>/dev/null || true
  fi
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

POLICY="$WORKDIR/policy.yaml"
BINDINGS_SERVER="$WORKDIR/server-bindings.yaml"
BINDINGS_CONSUMER="$WORKDIR/consumer-bindings.yaml"
MANIFEST="$WORKDIR/pade.yaml"

cat >"$POLICY" <<EOF
version: "0.1"
oidc:
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: ${AUDIENCE}
      jwksURL: https://api.cursor.com/keys
    google:
      issuer: https://accounts.google.com
      audience: ${AUDIENCE}
      jwksURL: https://www.googleapis.com/oauth2/v3/certs
policies:
  - issuer: cursor
    subject: "user:dogfood-cursor-placeholder"
    requireRepoURLs: false
    capabilities:
      - github.user.read
  - issuer: google
    subject: "${GCE_SUB}"
    requireRepoURLs: false
    capabilities:
      - ${CAPABILITY}
EOF

cat >"$BINDINGS_SERVER" <<EOF
version: "0.1"
capabilities:
  ${CAPABILITY}:
    provider: env
    env:
      - PADE_GCE_DOGFOOD
EOF

cat >"$BINDINGS_CONSUMER" <<EOF
version: "0.1"
capabilities:
  ${CAPABILITY}:
    provider: broker
    broker:
      endpoint: http://${LISTEN}
      audience: ${AUDIENCE}
      identity: gce
EOF

cat >"$MANIFEST" <<EOF
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: gce-multi-issuer-dogfood
spec:
  capabilities:
    ${CAPABILITY}:
      access: read
EOF

echo "starting multi-issuer broker on ${LISTEN}..."
# Nontrivial material value. The child must verify it in-process; do not echo it
# (pade exec redacts exact resolved material values from stdout).
export PADE_GCE_DOGFOOD=dogfood-pass
"$BROKER" -policy "$POLICY" -bindings "$BINDINGS_SERVER" -listen "$LISTEN" \
  >"$WORKDIR/broker.log" 2>&1 &
BROKER_PID=$!

for _ in $(seq 1 50); do
  if curl -sf "http://${LISTEN}/healthz" >/dev/null; then
    break
  fi
  if ! kill -0 "$BROKER_PID" 2>/dev/null; then
    echo "broker exited early:" >&2
    cat "$WORKDIR/broker.log" >&2 || true
    exit 1
  fi
  sleep 0.1
done
curl -sf "http://${LISTEN}/healthz" >/dev/null || die "broker healthz not ready"

echo "resolving ${CAPABILITY} via broker.identity=gce..."
# Verify material internally; print only a non-secret success marker (never the
# material value — redaction would turn it into [REDACTED] and break assertions).
out="$(
  "$PADE" exec \
    -f "$MANIFEST" \
    --bindings "$BINDINGS_CONSUMER" \
    --capability "$CAPABILITY" \
    --quiet \
    -- /bin/sh -c '
      if [ -z "${PADE_GCE_DOGFOOD:-}" ]; then
        echo "gce-multi-issuer-dogfood: missing material" >&2
        exit 1
      fi
      if [ "$PADE_GCE_DOGFOOD" != "dogfood-pass" ]; then
        echo "gce-multi-issuer-dogfood: unexpected material" >&2
        exit 1
      fi
      printf "gce-multi-issuer-dogfood: success\n"
    '
)"
test "$out" = "gce-multi-issuer-dogfood: success" || die "exec failed: $out"

echo "Experiment 005C dogfood succeeded (JWT never printed)."
