#!/usr/bin/env bash
# Local Teleport Application Access dogfood for examples/ingress-demo.
# Default: host Teleport + Go demo (like Vault dogfood). Optional: PADE_TELEPORT_MODE=compose.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DEMO_DIR="${ROOT}/examples/ingress-demo"
CONFIG_SRC="${DEMO_DIR}/teleport/teleport.yaml"
COMPOSE_FILE="${DEMO_DIR}/docker-compose.yml"
TOOLS_DIR="${ROOT}/.tools/teleport"
DATA_DIR="${PADE_TELEPORT_DATA:-${ROOT}/.tools/teleport-ingress/data}"
RUN_DIR="${PADE_TELEPORT_RUN:-${ROOT}/.tools/teleport-ingress/run}"
CONFIG_DST="${RUN_DIR}/teleport.yaml"
TELEPORT_VERSION="${TELEPORT_VERSION:-18.10.0}"
TELEPORT_BIN="${TELEPORT_BIN:-}"
TCTL_BIN="${TCTL_BIN:-}"
MODE="${PADE_TELEPORT_MODE:-host}"
USER_NAME="${PADE_TELEPORT_USER:-pade}"
APP_NAME="pade-ingress-demo"
RAW_URL="http://127.0.0.1:8080/"
PROXY_PING="https://127.0.0.1:3080/webapi/ping"
APP_URL="https://${APP_NAME}.localhost:3080/"
DEMO_PID=""
TELEPORT_PID=""
DEMO_LOG="${RUN_DIR}/ingress-demo.log"
TELEPORT_LOG="${RUN_DIR}/teleport.log"
PADE="${PADE:-${ROOT}/bin/pade}"
GO="${GO:-}"

export PADE_TELEPORT_DATA="${DATA_DIR}"

cleanup_host() {
  if [[ -n "${TELEPORT_PID}" ]] && kill -0 "${TELEPORT_PID}" 2>/dev/null; then
    kill "${TELEPORT_PID}" 2>/dev/null || true
    wait "${TELEPORT_PID}" 2>/dev/null || true
  fi
  if [[ -n "${DEMO_PID}" ]] && kill -0 "${DEMO_PID}" 2>/dev/null; then
    kill "${DEMO_PID}" 2>/dev/null || true
    wait "${DEMO_PID}" 2>/dev/null || true
  fi
  if [[ -f "${RUN_DIR}/teleport.pid" ]]; then
    local old
    old="$(cat "${RUN_DIR}/teleport.pid" 2>/dev/null || true)"
    if [[ -n "${old}" ]] && kill -0 "${old}" 2>/dev/null; then
      kill "${old}" 2>/dev/null || true
    fi
  fi
  if [[ -f "${RUN_DIR}/demo.pid" ]]; then
    local old
    old="$(cat "${RUN_DIR}/demo.pid" 2>/dev/null || true)"
    if [[ -n "${old}" ]] && kill -0 "${old}" 2>/dev/null; then
      kill "${old}" 2>/dev/null || true
    fi
  fi
}

die() {
  echo "error: $*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

resolve_go() {
  if [[ -n "${GO}" && -x "${GO}" ]]; then
    return
  fi
  if command -v go >/dev/null 2>&1; then
    GO="$(command -v go)"
    return
  fi
  if [[ -x "${ROOT}/.tools/go/bin/go" ]]; then
    GO="${ROOT}/.tools/go/bin/go"
    return
  fi
  die "need Go 1.22+ (mise install, or .tools/go fallback) to build/run ingress-demo"
}

detect_os_arch() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "${arch}" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported arch: ${arch}" ;;
  esac
  case "${os}" in
    linux|darwin) ;;
    *) die "unsupported os: ${os}" ;;
  esac
  echo "${os}-${arch}"
}

ensure_teleport_bins() {
  if [[ -n "${TELEPORT_BIN}" && -x "${TELEPORT_BIN}" ]]; then
    :
  elif [[ -x "${TOOLS_DIR}/teleport" ]]; then
    TELEPORT_BIN="${TOOLS_DIR}/teleport"
  elif command -v teleport >/dev/null 2>&1; then
    TELEPORT_BIN="$(command -v teleport)"
  else
    TELEPORT_BIN=""
  fi

  if [[ -n "${TCTL_BIN}" && -x "${TCTL_BIN}" ]]; then
    :
  elif [[ -x "${TOOLS_DIR}/tctl" ]]; then
    TCTL_BIN="${TOOLS_DIR}/tctl"
  elif [[ -x "${TOOLS_DIR}/tctl.app/Contents/MacOS/tctl" ]]; then
    TCTL_BIN="${TOOLS_DIR}/tctl.app/Contents/MacOS/tctl"
  elif command -v tctl >/dev/null 2>&1; then
    TCTL_BIN="$(command -v tctl)"
  else
    TCTL_BIN=""
  fi

  if [[ -n "${TELEPORT_BIN}" && -n "${TCTL_BIN}" ]]; then
    return
  fi

  local platform url tar os
  platform="$(detect_os_arch)"
  os="${platform%%-*}"
  mkdir -p "${TOOLS_DIR}"
  tar="/tmp/teleport-v${TELEPORT_VERSION}-${platform}-bin.tar.gz"
  url="https://cdn.teleport.dev/teleport-v${TELEPORT_VERSION}-${platform}-bin.tar.gz"
  echo "Downloading Teleport ${TELEPORT_VERSION} (${platform})..."
  curl -fsSL -o "${tar}" "${url}"
  # macOS packages ship tctl/tsh as .app bundles; Linux still has flat binaries.
  if [[ "${os}" == "darwin" ]]; then
    tar -xzf "${tar}" -C "${TOOLS_DIR}" --strip-components=1 \
      teleport/teleport teleport/tctl.app
    TCTL_BIN="${TOOLS_DIR}/tctl.app/Contents/MacOS/tctl"
  else
    tar -xzf "${tar}" -C "${TOOLS_DIR}" --strip-components=1 \
      teleport/teleport teleport/tctl
    TCTL_BIN="${TOOLS_DIR}/tctl"
  fi
  chmod +x "${TOOLS_DIR}/teleport" "${TCTL_BIN}"
  TELEPORT_BIN="${TOOLS_DIR}/teleport"
  [[ -x "${TELEPORT_BIN}" && -x "${TCTL_BIN}" ]] || die "Teleport extract incomplete under ${TOOLS_DIR}"
}

wait_http() {
  local url="$1"
  local label="$2"
  local insecure="${3:-}"
  local i
  for i in $(seq 1 90); do
    if [[ -n "${insecure}" ]]; then
      if curl -fsSk --max-time 2 "${url}" >/dev/null 2>&1; then
        echo "ready: ${label}"
        return 0
      fi
    else
      if curl -fsS --max-time 2 "${url}" >/dev/null 2>&1; then
        echo "ready: ${label}"
        return 0
      fi
    fi
    sleep 1
  done
  die "timed out waiting for ${label} (${url})"
}

assert_acceptance() {
  local body app_headers app_body
  body="$(curl -fsS --max-time 5 "${RAW_URL}")"
  echo "${body}" | grep -q "PADE ingress demo" || die "raw app did not return expected HTML"
  echo "ok: raw app is reachable without Teleport auth (${RAW_URL})"

  app_headers="$(mktemp)"
  app_body="$(mktemp)"
  # shellcheck disable=SC2064
  trap "rm -f '${app_headers}' '${app_body}'" RETURN
  curl -sk --max-time 10 -D "${app_headers}" -o "${app_body}" "${APP_URL}" || true
  if grep -q "PADE ingress demo" "${app_body}"; then
    die "Teleport app URL returned demo HTML without login (expected gated access)"
  fi
  if ! grep -qiE '^(HTTP/|location:)' "${app_headers}"; then
    die "Teleport app URL did not return HTTP headers"
  fi
  echo "ok: Teleport app URL is gated without a session (${APP_URL})"
}

print_success() {
  cat <<EOF

dogfood-ingress-teleport: ok

Next (browser):
  1. Open Proxy UI:  https://localhost:3080/
     (accept the self-signed cert warning for local dogfood)
  2. Sign in as user "${USER_NAME}" (invite URL printed above if newly created)
  3. Launch application "${APP_NAME}" or open:
       ${APP_URL}

Contrast:
  Open (no auth):     ${RAW_URL}
  Gated via Teleport: ${APP_URL}

Teardown:
  mise run dogfood-ingress-teleport-down

Docs: docs/teleport-ingress.md
EOF
}

ensure_user_host() {
  local listing
  listing="$("${TCTL_BIN}" --config="${CONFIG_DST}" users ls 2>/dev/null || true)"
  if echo "${listing}" | grep -qw "${USER_NAME}"; then
    echo "Teleport user already exists: ${USER_NAME}"
    echo "Reset password if needed: ${TCTL_BIN} --config=${CONFIG_DST} users reset ${USER_NAME}"
    return 0
  fi
  echo "Creating Teleport local user: ${USER_NAME}"
  "${TCTL_BIN}" --config="${CONFIG_DST}" users add "${USER_NAME}" --roles=access,editor || {
    echo "warn: tctl users add failed; run: ${TCTL_BIN} --config=${CONFIG_DST} users add ${USER_NAME} --roles=access,editor" >&2
  }
}

prepare_config() {
  mkdir -p "${DATA_DIR}" "${RUN_DIR}"
  # Rewrite data_dir to the gitignored local path (config ships with container default).
  sed "s|data_dir: /var/lib/teleport|data_dir: ${DATA_DIR}|" "${CONFIG_SRC}" >"${CONFIG_DST}"
}

start_demo_host() {
  resolve_go
  # Free port if a previous demo is still bound.
  if curl -fsS --max-time 1 "${RAW_URL}healthz" >/dev/null 2>&1; then
    echo "ingress-demo already responding on :8080 (reusing)"
    return 0
  fi
  (
    cd "${DEMO_DIR}"
    "${GO}" run . >"${DEMO_LOG}" 2>&1
  ) &
  DEMO_PID=$!
  echo "${DEMO_PID}" >"${RUN_DIR}/demo.pid"
}

start_teleport_host() {
  ensure_teleport_bins
  prepare_config
  if curl -fsSk --max-time 2 "${PROXY_PING}" >/dev/null 2>&1; then
    echo "Teleport Proxy already responding on :3080 (reusing)"
    return 0
  fi
  "${TELEPORT_BIN}" start --config="${CONFIG_DST}" >"${TELEPORT_LOG}" 2>&1 &
  TELEPORT_PID=$!
  echo "${TELEPORT_PID}" >"${RUN_DIR}/teleport.pid"
}

cmd_up_host() {
  need_cmd curl
  [[ -f "${CONFIG_SRC}" ]] || die "missing ${CONFIG_SRC}"
  [[ -x "${PADE}" ]] || die "pade binary not found at ${PADE} (run: mise run build)"

  # On successful up, leave host processes running; only tear down on failure.
  trap 'rc=$?; if [[ $rc -ne 0 ]]; then cleanup_host; fi' EXIT

  "${PADE}" validate -f "${DEMO_DIR}/pade.yaml"

  echo "Starting Teleport ingress dogfood (host mode; data: ${DATA_DIR})"
  start_demo_host
  wait_http "${RAW_URL}healthz" "ingress-demo on :8080"
  start_teleport_host
  wait_http "${PROXY_PING}" "Teleport Proxy on :3080" insecure

  assert_acceptance
  ensure_user_host
  print_success
}

compose() {
  if docker compose version >/dev/null 2>&1; then
    docker compose -f "${COMPOSE_FILE}" --project-directory "${DEMO_DIR}" "$@"
  elif command -v docker-compose >/dev/null 2>&1; then
    docker-compose -f "${COMPOSE_FILE}" --project-directory "${DEMO_DIR}" "$@"
  else
    die "docker compose (or docker-compose) is required for PADE_TELEPORT_MODE=compose"
  fi
}

ensure_user_compose() {
  local listing
  listing="$(compose exec -T teleport tctl users ls 2>/dev/null || true)"
  if echo "${listing}" | grep -qw "${USER_NAME}"; then
    echo "Teleport user already exists: ${USER_NAME}"
    return 0
  fi
  echo "Creating Teleport local user: ${USER_NAME}"
  compose exec -T teleport tctl users add "${USER_NAME}" --roles=access,editor || true
}

cmd_up_compose() {
  need_cmd docker
  need_cmd curl
  [[ -x "${PADE}" ]] || die "pade binary not found at ${PADE} (run: mise run build)"
  mkdir -p "${DATA_DIR}"
  "${PADE}" validate -f "${DEMO_DIR}/pade.yaml"
  echo "Starting Teleport ingress dogfood (compose mode; data: ${DATA_DIR})"
  compose up -d --build
  wait_http "${RAW_URL}healthz" "ingress-demo on :8080"
  wait_http "${PROXY_PING}" "Teleport Proxy on :3080" insecure
  assert_acceptance
  ensure_user_compose
  print_success
}

cmd_up() {
  case "${MODE}" in
    host) cmd_up_host ;;
    compose) cmd_up_compose ;;
    *) die "unknown PADE_TELEPORT_MODE=${MODE} (use host or compose)" ;;
  esac
}

cmd_down() {
  case "${MODE}" in
    compose)
      need_cmd docker
      mkdir -p "${DATA_DIR}"
      compose down --remove-orphans || true
      ;;
    host|*)
      cleanup_host
      # Also stop recorded pids from a previous up.
      if [[ -f "${RUN_DIR}/teleport.pid" ]] || [[ -f "${RUN_DIR}/demo.pid" ]]; then
        cleanup_host
        rm -f "${RUN_DIR}/teleport.pid" "${RUN_DIR}/demo.pid"
      fi
      # Best-effort: stop listeners on dogfood ports if we own them.
      if command -v lsof >/dev/null 2>&1; then
        local p
        for p in $(lsof -tiTCP:3080 -sTCP:LISTEN 2>/dev/null || true); do
          if ps -p "${p}" -o command= 2>/dev/null | grep -q teleport; then
            kill "${p}" 2>/dev/null || true
          fi
        done
        for p in $(lsof -tiTCP:8080 -sTCP:LISTEN 2>/dev/null || true); do
          if ps -p "${p}" -o command= 2>/dev/null | grep -Eq 'ingress-demo|go-build|/go-run'; then
            kill "${p}" 2>/dev/null || true
          fi
        done
      fi
      ;;
  esac
  echo "dogfood-ingress-teleport-down: ok"
}

cmd_status() {
  case "${MODE}" in
    compose) compose ps ;;
    *)
      echo "mode: host"
      echo "data: ${DATA_DIR}"
      if [[ -f "${RUN_DIR}/teleport.pid" ]]; then
        echo "teleport.pid: $(cat "${RUN_DIR}/teleport.pid")"
      fi
      if [[ -f "${RUN_DIR}/demo.pid" ]]; then
        echo "demo.pid: $(cat "${RUN_DIR}/demo.pid")"
      fi
      curl -fsS --max-time 2 "${RAW_URL}healthz" >/dev/null && echo "demo: up" || echo "demo: down"
      curl -fsSk --max-time 2 "${PROXY_PING}" >/dev/null && echo "teleport: up" || echo "teleport: down"
      ;;
  esac
}

usage() {
  cat <<EOF
Usage: $(basename "$0") <up|down|status>

  up      Start demo + Teleport, validate open vs gated access
  down    Stop the dogfood stack
  status  Show status

Env:
  PADE_TELEPORT_MODE   host (default) or compose
  PADE_TELEPORT_DATA   Data dir (default: <repo>/.tools/teleport-ingress/data)
  PADE_TELEPORT_USER   Local Teleport user (default: pade)
  TELEPORT_VERSION     Pinned Community Edition (default: ${TELEPORT_VERSION})
EOF
}

main() {
  local cmd="${1:-up}"
  case "${cmd}" in
    up) cmd_up ;;
    down) cmd_down ;;
    status) cmd_status ;;
    -h|--help|help) usage ;;
    *) usage >&2; exit 2 ;;
  esac
}

main "$@"
