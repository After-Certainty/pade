#!/usr/bin/env bash
# Install the real 1Password CLI (`op`) for local dogfood (not used by CI fake-op).
# Prefers Homebrew when available; otherwise downloads a pinned release into .tools/op/.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TOOLS_DIR="$ROOT/.tools/op"
# Keep in sync with the Homebrew cask when bumping:
#   https://formulae.brew.sh/cask/1password-cli
OP_VERSION="${OP_VERSION:-2.38.1}"

die() {
  echo "error: $*" >&2
  exit 1
}

have_real_op() {
  local bin
  bin="$(command -v op 2>/dev/null || true)"
  [[ -n "$bin" ]] || return 1
  [[ "$bin" != *fake-op* ]] || return 1
  return 0
}

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    aarch64|arm64) arch="arm64" ;;
    *) die "unsupported arch: $arch" ;;
  esac
  case "$os" in
    linux|darwin) ;;
    *) die "unsupported os: $os (install manually: https://developer.1password.com/docs/cli/get-started/)" ;;
  esac
  echo "${os}_${arch}"
}

install_via_brew() {
  command -v brew >/dev/null 2>&1 || return 1
  # Writable Cellar is required; otherwise fall through to pinned download.
  if [[ ! -w "$(brew --cellar 2>/dev/null || echo /usr/local/Cellar)" ]]; then
    echo "Homebrew is present but not writable; skipping brew install."
    return 1
  fi
  echo "Installing 1Password CLI via Homebrew..."
  # Official docs use `brew install 1password-cli` (cask).
  brew install --cask 1password-cli
}

install_via_download() {
  local platform zip url
  platform="$(detect_platform)"
  mkdir -p "$TOOLS_DIR"
  zip="/tmp/op_${OP_VERSION}_${platform}.zip"
  url="https://cache.agilebits.com/dist/1P/op2/pkg/v${OP_VERSION}/op_${platform}_v${OP_VERSION}.zip"
  echo "Downloading 1Password CLI ${OP_VERSION} (${platform})..."
  # Some CDNs reject empty/default curl User-Agent with 403.
  curl -fsSL -A "pade-install-onepassword-cli/${OP_VERSION}" -o "$zip" "$url"
  if command -v unzip >/dev/null 2>&1; then
    unzip -o -q "$zip" op -d "$TOOLS_DIR"
  else
    python3 - "$zip" "$TOOLS_DIR" <<'PY'
import sys, zipfile
z = zipfile.ZipFile(sys.argv[1])
z.extract("op", sys.argv[2])
PY
  fi
  chmod +x "$TOOLS_DIR/op"
  echo "Installed: $TOOLS_DIR/op"
  echo
  echo "Add it to PATH for this shell:"
  echo "  export PATH=\"$TOOLS_DIR:\$PATH\""
}

print_next_steps() {
  cat <<EOF

Next (for mise run dogfood-onepassword-live):
  1. Sign in:  op signin
     (or enable app integration: 1Password → Settings → Developer → Integrate with 1Password CLI)
  2. Store a GitHub PAT (read:user) — see docs/onepassword-dogfood.md
  3. Run:     mise run dogfood-onepassword-live
EOF
}

main() {
  if have_real_op; then
    echo "ok: 1Password CLI already on PATH: $(command -v op) ($(op --version 2>/dev/null || echo version-unknown))"
    print_next_steps
    exit 0
  fi

  if [[ -x "$TOOLS_DIR/op" ]]; then
    echo "ok: found $TOOLS_DIR/op ($("$TOOLS_DIR/op" --version 2>/dev/null || echo version-unknown))"
    echo "Add it to PATH: export PATH=\"$TOOLS_DIR:\$PATH\""
    print_next_steps
    exit 0
  fi

  if install_via_brew; then
    if have_real_op; then
      echo "ok: $(command -v op) ($(op --version))"
      print_next_steps
      exit 0
    fi
    die "Homebrew reported success but 'op' is not on PATH"
  fi

  echo "Brew install unavailable; falling back to pinned download into .tools/op/"
  install_via_download
  print_next_steps
}

main "$@"
