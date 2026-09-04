#!/usr/bin/env bash
# Dogfood broker-side provider: exec (B–C), GitHub App (D–E), Google Analytics (F),
# and two-provider same-seam (G). Consumer uses provider: broker only; exec runs
# on the broker host via operator-owned server bindings.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
PADE="${PADE:-$ROOT/bin/pade}"
GO="${GO:-go}"
MODE="${1:-stub}" # stub | github | ga | two

die() {
  echo "error: $*" >&2
  exit 1
}

[[ -x "$PADE" ]] || die "pade binary not found at $PADE (run: mise run build)"

STUB_OUT="${ROOT}/bin/pade-provider-stub"
GH_OUT="${ROOT}/bin/pade-provider-github"
GA_OUT="${ROOT}/bin/pade-provider-google-analytics"
TMP="$(mktemp -d "${TMPDIR:-/tmp}/pade-exec-provider.XXXXXX")"
HELPER_LOG="$TMP/helper.log"
HELPER_PID=""

cleanup() {
  if [[ -n "${HELPER_PID:-}" ]] && kill -0 "$HELPER_PID" 2>/dev/null; then
    kill "$HELPER_PID" 2>/dev/null || true
    wait "$HELPER_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT
mkdir -p "${ROOT}/bin"

start_broker() {
  # Expects $TMP/broker-bindings.yaml already written with provider: exec.
  echo "=== starting broker with operator-side exec bindings ==="
  "$GO" run "$ROOT/scripts/broker-dogfood-helper" "$TMP" >"$HELPER_LOG" 2>&1 &
  HELPER_PID=$!

  ENV_FILE=""
  for _ in $(seq 1 50); do
    if [[ -f "$TMP/dogfood.env" ]]; then
      ENV_FILE="$TMP/dogfood.env"
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
}

run_two_provider_seam() {
  echo "=== Milestone G: two-provider same-seam architectural test (broker-side exec) ==="
  echo "=== github + google-analytics unit tests ==="
  "$GO" test ./examples/providers/github/ ./examples/providers/google-analytics/ -count=1
  "$GO" build -o "${GH_OUT}" ./examples/providers/github
  "$GO" build -o "${GA_OUT}" ./examples/providers/google-analytics
  export PADE_PROVIDER_FAKE=1

  cat >"${TMP}/pade.yaml" <<EOF
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: two-provider-seam-dogfood
spec:
  capabilities:
    github.repo.read:
      access: use
    google-analytics.read:
      access: read
EOF

  cat >"${TMP}/broker-bindings.yaml" <<EOF
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
          - After-Certainty/pade
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
        propertyId: properties/000000000
EOF

  start_broker

  echo "=== two-provider: validate ==="
  "$PADE" validate -f "${TMP}/pade.yaml"

  echo "=== two-provider: capabilities (Consumer sees provider: broker only) ==="
  out="$("$PADE" capabilities -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS")"
  echo "$out"
  echo "$out" | grep -q 'provider: broker' || die "expected provider: broker on Consumer"
  if echo "$out" | grep -q 'provider: exec'; then
    die "Consumer must not expose provider: exec"
  fi
  echo "$out" | grep -q 'github.repo.read' || die "missing github.repo.read"
  echo "$out" | grep -q 'google-analytics.read' || die "missing google-analytics.read"

  chmod +x \
    "${ROOT}/examples/demo-project/scripts/github-repo-meta" \
    "${ROOT}/examples/demo-project/scripts/ga-property-meta"

  echo "=== two-provider: resolve github.repo.read via broker ==="
  "$PADE" exec -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS" \
    --capability github.repo.read --quiet -- \
    env GITHUB_REPOSITORY=After-Certainty/pade "${ROOT}/examples/demo-project/scripts/github-repo-meta"

  echo "=== two-provider: resolve google-analytics.read via broker ==="
  "$PADE" exec -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS" \
    --capability google-analytics.read --quiet -- \
    "${ROOT}/examples/demo-project/scripts/ga-property-meta"

  echo "ok: two-provider same-seam dogfood (broker-side exec; no vendor fields in PADE core)"
}

case "$MODE" in
  two)
    run_two_provider_seam
    exit 0
    ;;
  stub)
    "$GO" build -o "${STUB_OUT}" ./examples/providers/stub
    COMMAND_PATH="${STUB_OUT}"
    CAP="demo.derived"
    CHECK_ENV="DEMO_TOKEN"
    EXPECT="stub-ok"
    CONFIG_YAML="        tokenEnv: DEMO_TOKEN
        value: stub-ok"
    unset PADE_PROVIDER_FAKE || true
    ;;
  github)
    echo "=== github provider unit tests (JWT + installation token httptest) ==="
    "$GO" test ./examples/providers/github/ -count=1
    "$GO" build -o "${GH_OUT}" ./examples/providers/github
    COMMAND_PATH="${GH_OUT}"
    CAP="github.repo.read"
    CHECK_ENV="GITHUB_TOKEN"
    EXPECT="ghs_pade_fake_installation_token"
    CONFIG_YAML="        tokenEnv: GITHUB_TOKEN
        repositories:
          - After-Certainty/pade
        permissions:
          metadata: read
          contents: read"
    export PADE_PROVIDER_FAKE=1
    ;;
  ga)
    echo "=== google-analytics provider unit tests (SA JWT + token exchange httptest) ==="
    "$GO" test ./examples/providers/google-analytics/ -count=1
    "$GO" build -o "${GA_OUT}" ./examples/providers/google-analytics
    COMMAND_PATH="${GA_OUT}"
    CAP="google-analytics.read"
    CHECK_ENV="GA_ACCESS_TOKEN"
    EXPECT="ya29.pade_fake_access_token"
    CONFIG_YAML="        tokenEnv: GA_ACCESS_TOKEN
        propertyId: properties/000000000"
    export PADE_PROVIDER_FAKE=1
    ;;
  *)
    die "usage: $0 [stub|github|ga|two]"
    ;;
esac

cat >"${TMP}/pade.yaml" <<EOF
apiVersion: pade.local/v1alpha1
kind: DevelopmentSession
metadata:
  name: exec-provider-dogfood
spec:
  capabilities:
    ${CAP}:
      access: use
EOF

cat >"${TMP}/broker-bindings.yaml" <<EOF
version: "0.1"
capabilities:
  ${CAP}:
    provider: exec
    exec:
      command:
        - ${COMMAND_PATH}
      config:
${CONFIG_YAML}
EOF

start_broker

echo "=== exec provider dogfood (${MODE}): validate ==="
"$PADE" validate -f "${TMP}/pade.yaml"

echo "=== exec provider dogfood (${MODE}): capabilities (broker; no local exec) ==="
caps_out="$("$PADE" capabilities -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS")"
echo "$caps_out"
echo "$caps_out" | grep -q 'provider: broker' || die "expected provider: broker"
if echo "$caps_out" | grep -q 'provider: exec'; then
  die "Consumer must not expose provider: exec"
fi

echo "=== assert development-side exec bindings are rejected ==="
if "$PADE" capabilities -f "${TMP}/pade.yaml" --bindings "${TMP}/broker-bindings.yaml" >/dev/null 2>"$TMP/reject.err"; then
  die "expected Consumer to reject provider: exec bindings"
fi
grep -q 'broker-side only' "$TMP/reject.err" || die "expected broker-side only error, got: $(cat "$TMP/reject.err")"

echo "=== exec provider dogfood (${MODE}): exec via broker (assert env; do not print secret) ==="
"$PADE" exec -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS" \
  --capability "${CAP}" --quiet -- \
  /bin/sh -c "test \"\$${CHECK_ENV}\" = '${EXPECT}'"

if [[ "$MODE" == "github" ]]; then
  REPO_META="${ROOT}/examples/demo-project/scripts/github-repo-meta"
  chmod +x "$REPO_META"
  echo "=== exec provider dogfood (github): repo-scoped validation (Milestone E; fake skips network) ==="
  "$PADE" exec -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS" \
    --capability "${CAP}" --quiet -- \
    env GITHUB_REPOSITORY=After-Certainty/pade "$REPO_META"
fi

if [[ "$MODE" == "ga" ]]; then
  GA_META="${ROOT}/examples/demo-project/scripts/ga-property-meta"
  chmod +x "$GA_META"
  echo "=== exec provider dogfood (ga): property validation (Milestone F; fake skips network) ==="
  "$PADE" exec -f "${TMP}/pade.yaml" --bindings "$PADE_BINDINGS" \
    --capability "${CAP}" --quiet -- \
    "$GA_META"
fi

echo "ok: exec provider dogfood (${MODE}) via broker-side exec"
