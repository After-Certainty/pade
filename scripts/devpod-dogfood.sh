#!/usr/bin/env bash
# DevPod dogfood helpers for examples/demo-project.
# PADE never owns workspace lifecycle; this script only orchestrates local proof steps.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DEMO_DIR="$ROOT/examples/demo-project"
WORKSPACE="${DEVPOD_WORKSPACE:-demo-project}"
BIN_LINUX="$ROOT/bin/pade-linux"

die() { echo "error: $*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || die "'$1' not found on PATH"; }

linux_goarch() {
  local arch
  arch="$(docker info --format '{{.Architecture}}' 2>/dev/null || true)"
  case "$arch" in
    aarch64|arm64) echo arm64 ;;
    *) echo amd64 ;;
  esac
}

go_bin() {
  # Prefer PATH go (mise-activated) over a leftover .tools/go unpack.
  if command -v go >/dev/null 2>&1; then
    command -v go
  elif [[ -x "$ROOT/.tools/go/bin/go" ]]; then
    echo "$ROOT/.tools/go/bin/go"
  else
    return 1
  fi
}

# Resolve the running Docker container for this DevPod workspace.
resolve_container_id() {
  local ws_dir result uid folder cid
  ws_dir="${HOME}/.devpod/contexts/default/workspaces/${WORKSPACE}"
  result="${ws_dir}/workspace_result.json"

  if [[ -f "$result" ]]; then
    cid="$(python3 - "$result" <<'PY'
import json, sys
doc = json.load(open(sys.argv[1]))
print(doc.get("ContainerDetails", {}).get("ID", "")[:12])
PY
)"
    if [[ -n "$cid" ]] && docker inspect "$cid" >/dev/null 2>&1; then
      echo "$cid"
      return 0
    fi
  fi

  if [[ -f "${ws_dir}/workspace.json" ]]; then
    uid="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["uid"])' "${ws_dir}/workspace.json")"
    folder="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source"]["localFolder"])' "${ws_dir}/workspace.json")"
    if [[ -n "$uid" ]]; then
      cid="$(docker ps -qf "label=dev.containers.id=${uid}" | head -n1)"
      if [[ -n "$cid" ]]; then
        echo "$cid"
        return 0
      fi
    fi
    # Fallback: container that bind-mounts the demo folder.
    if [[ -n "$folder" ]]; then
      while IFS= read -r id; do
        if docker inspect "$id" --format '{{range .Mounts}}{{.Source}}{{"\n"}}{{end}}' | grep -Fxq "$folder"; then
          echo "$id"
          return 0
        fi
      done < <(docker ps -q)
    fi
  fi

  return 1
}

cmd_check() {
  need docker
  need devpod
  docker info >/dev/null 2>&1 || die "docker daemon not reachable (start Docker Desktop / OrbStack)"
  echo "ok: docker + devpod available"
  echo "    docker arch: $(docker info --format '{{.Architecture}}' 2>/dev/null || echo unknown)"
  echo "    workspace:   $WORKSPACE"
}

cmd_provider() {
  cmd_check
  if ! devpod provider list 2>/dev/null | grep -q docker; then
    echo "adding docker provider..."
    devpod provider add docker
  fi
  echo "setting default provider to docker..."
  devpod provider use docker
  devpod provider list
}

cmd_up() {
  cmd_check
  [[ -d "$DEMO_DIR" ]] || die "missing $DEMO_DIR"
  echo "starting DevPod workspace from $DEMO_DIR ..."
  (cd "$DEMO_DIR" && devpod up . --ide none)
  devpod list
  devpod status "$WORKSPACE" 2>/dev/null || true
}

cmd_build_linux() {
  local goarch gobin
  need docker
  goarch="$(linux_goarch)"
  gobin="$(go_bin)"
  [[ -n "$gobin" ]] || die "go not found"
  echo "building linux/$goarch pade -> $BIN_LINUX"
  mkdir -p "$ROOT/bin"
  (cd "$ROOT" && GOOS=linux GOARCH="$goarch" CGO_ENABLED=0 "$gobin" build -o "$BIN_LINUX" ./cmd/pade)
  ls -la "$BIN_LINUX"
}

cmd_install() {
  local cid
  cmd_check
  cmd_build_linux
  [[ -f "$BIN_LINUX" ]] || die "missing $BIN_LINUX"

  cid="$(resolve_container_id)" || die "could not find running container for workspace '$WORKSPACE' (is it up? try: mise run dogfood-devpod-up)"
  echo "installing pade into container ${cid} via docker cp..."
  docker exec -u root "$cid" mkdir -p /home/vscode/bin
  docker cp "$BIN_LINUX" "${cid}:/home/vscode/bin/pade"
  docker exec -u root "$cid" chown vscode:vscode /home/vscode/bin/pade
  docker exec -u root "$cid" chmod +x /home/vscode/bin/pade
  echo "verifying..."
  docker exec -u vscode "$cid" /home/vscode/bin/pade --help >/dev/null
  echo "ok: pade installed at /home/vscode/bin/pade in $WORKSPACE ($cid)"
}

cmd_smoke() {
  local cid
  cmd_check
  cid="$(resolve_container_id)" || die "could not find running container for workspace '$WORKSPACE' (is it up? try: mise run dogfood-devpod-up)"
  echo "running PADE smoke inside container ${cid}..."
  docker exec -u vscode -w /workspaces/demo-project \
    -e HOME=/home/vscode \
    -e PATH="/home/vscode/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin" \
    "$cid" \
    bash -lc '
set -euo pipefail
command -v pade >/dev/null || { echo "pade not installed; run: mise run dogfood-devpod-install" >&2; exit 1; }
chmod +x ./scripts/github-whoami 2>/dev/null || true
pade validate
pade plan --bindings bindings.example.yaml >/tmp/pade-plan.txt
GITHUB_TOKEN=pade-demo-env-token \
pade exec \
  --bindings bindings.example.yaml \
  --capability github.user.read \
  -- ./scripts/github-whoami
echo "ok: DevPod + PADE smoke succeeded"
'
}

cmd_down() {
  need devpod
  echo "stopping workspace '$WORKSPACE' (DevPod owns lifecycle)..."
  devpod stop "$WORKSPACE" || true
}

cmd_delete() {
  need devpod
  echo "deleting workspace '$WORKSPACE'..."
  devpod delete "$WORKSPACE" --force || true
}

cmd_all() {
  cmd_provider
  cmd_up
  cmd_install
  cmd_smoke
  echo
  echo "DevPod dogfood complete."
  echo "  SSH:    devpod ssh $WORKSPACE"
  echo "  Stop:   mise run dogfood-devpod-down"
  echo "  Delete: mise run dogfood-devpod-delete"
}

# Non-interactive CI entrypoint: full dogfood then best-effort delete.
cmd_ci() {
  cmd_all
  echo "cleaning up DevPod workspace for CI..."
  cmd_delete
}

usage() {
  cat <<EOF
usage: $(basename "$0") <command>

commands:
  check      Verify docker + devpod are available
  provider   Add/use the docker provider
  up         devpod up examples/demo-project
  build      Cross-compile pade for the Docker VM arch (linux)
  install    Copy linux pade into the running workspace (docker cp)
  smoke      validate + exec inside the workspace (docker exec)
  down       devpod stop
  delete     devpod delete
  all        provider + up + install + smoke
  ci         all + delete (for GitHub Actions)

env:
  DEVPOD_WORKSPACE  workspace name (default: demo-project)
EOF
}

main() {
  local cmd="${1:-}"
  shift || true
  case "$cmd" in
    check) cmd_check "$@" ;;
    provider) cmd_provider "$@" ;;
    up) cmd_up "$@" ;;
    build) cmd_build_linux "$@" ;;
    install) cmd_install "$@" ;;
    smoke) cmd_smoke "$@" ;;
    down) cmd_down "$@" ;;
    delete) cmd_delete "$@" ;;
    all) cmd_all "$@" ;;
    ci) cmd_ci "$@" ;;
    ""|-h|--help|help) usage ;;
    *) die "unknown command: $cmd (try: help)" ;;
  esac
}

main "$@"
