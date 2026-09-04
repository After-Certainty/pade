#!/usr/bin/env bash
# Install Keeper Commander (`keeper`) for local live dogfood (not used by CI fake-keeper).
# Order: existing PATH → Homebrew → official macOS .pkg (GUI) → repo-local Python venv.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VENV_DIR="$ROOT/.tools/keeper-venv"
VENV_KEEPER="$VENV_DIR/bin/keeper"
PKG_DIR="$ROOT/.tools/keeper"
# Keep in sync with https://github.com/Keeper-Security/Commander/releases
KEEPER_VERSION="${KEEPER_VERSION:-v18.1.0}"

die() {
  echo "error: $*" >&2
  exit 1
}

have_real_keeper() {
  local bin
  bin="$(command -v keeper 2>/dev/null || true)"
  [[ -n "$bin" ]] || return 1
  [[ "$bin" != *fake-keeper* ]] || return 1
  return 0
}

darwin_arch() {
  case "$(uname -m)" in
    arm64|aarch64) echo arm64 ;;
    x86_64|amd64) echo x86_64 ;;
    *) die "unsupported macOS arch: $(uname -m)" ;;
  esac
}

install_via_brew() {
  command -v brew >/dev/null 2>&1 || return 1
  if [[ ! -w "$(brew --cellar 2>/dev/null || echo /usr/local/Cellar)" ]]; then
    echo "Homebrew is present but not writable; skipping brew install."
    return 1
  fi
  echo "Installing Keeper Commander via Homebrew..."
  if brew install keeper-commander; then
    return 0
  fi
  echo "Homebrew install failed; trying next method."
  return 1
}

download_pkg() {
  local arch pkg_name url pkg_path
  arch="$(darwin_arch)"
  pkg_name="keeper-commander-mac-${arch}-${KEEPER_VERSION}.pkg"
  url="https://github.com/Keeper-Security/Commander/releases/download/${KEEPER_VERSION}/${pkg_name}"
  mkdir -p "$PKG_DIR"
  pkg_path="$PKG_DIR/$pkg_name"
  if [[ ! -f "$pkg_path" ]]; then
    echo "Downloading official Keeper Commander package (${pkg_name})..."
    curl -fsSL -A "pade-install-keeper-cli/${KEEPER_VERSION}" -o "$pkg_path" "$url"
  else
    echo "Using cached package: $pkg_path"
  fi
  echo "$pkg_path"
}

install_via_pkg() {
  [[ "$(uname -s)" == "Darwin" ]] || return 1
  local pkg_path
  pkg_path="$(download_pkg)"
  echo "Opening package installer (complete the GUI prompts if shown)..."
  open "$pkg_path"
  echo "Waiting for 'keeper' to appear on PATH (up to 3 minutes)..."
  local i
  for i in $(seq 1 36); do
    hash -r 2>/dev/null || true
    if have_real_keeper; then
      return 0
    fi
    sleep 5
  done
  echo "Package may still be installing. After it finishes, re-run: mise run install-keeper-cli"
  echo "Cached package: $pkg_path"
  return 1
}

install_via_venv() {
  command -v python3 >/dev/null 2>&1 || die "python3 not found (needed for pip fallback install)"
  echo "Creating Python venv at $VENV_DIR ..."
  rm -rf "$VENV_DIR"
  python3 -m venv "$VENV_DIR"
  "$VENV_DIR/bin/python" -m pip install --upgrade pip
  echo "Installing keepercommander (binary wheels only; no local Rust build)..."
  # Prefer wheels so we do not need a local Rust toolchain for cryptography/cbor2.
  if ! "$VENV_DIR/bin/python" -m pip install --only-binary=:all: 'keepercommander'; then
    echo "No complete binary wheel set available for this Python/platform."
    return 1
  fi
  [[ -x "$VENV_KEEPER" ]] || die "venv install finished but $VENV_KEEPER is missing"
  echo "Installed: $VENV_KEEPER"
  echo
  echo "Add it to PATH for this shell:"
  echo "  export PATH=\"$VENV_DIR/bin:\$PATH\""
}

print_next_steps() {
  cat <<EOF

Next (for mise run dogfood-keeper-live):
  1. Sign in (interactive):  keeper shell
     Then: login <you@example.com>
     Optional persistence: this-device persistent-login on && this-device register
  2. Store a GitHub PAT (read:user) in a Login record's password field
     (Keeper app or Commander). Note the record UID.
  3. Export:  export KEEPER_RECORD_UID=<uid>
  4. Run:     mise run dogfood-keeper-live

See docs/keeper-dogfood.md for details.
EOF
}

report_ok() {
  local bin="${1:-}"
  if [[ -n "$bin" ]]; then
    echo "ok: $bin"
  else
    echo "ok: $(command -v keeper)"
  fi
  if have_real_keeper; then
    keeper version 2>/dev/null || true
  elif [[ -n "$bin" && -x "$bin" ]]; then
    "$bin" version 2>/dev/null || true
  fi
  print_next_steps
}

main() {
  if have_real_keeper; then
    report_ok "$(command -v keeper)"
    exit 0
  fi

  if [[ -x "$VENV_KEEPER" ]]; then
    echo "ok: found $VENV_KEEPER"
    echo "Add it to PATH: export PATH=\"$VENV_DIR/bin:\$PATH\""
    print_next_steps
    exit 0
  fi

  if install_via_brew && have_real_keeper; then
    report_ok
    exit 0
  fi

  if install_via_pkg && have_real_keeper; then
    report_ok
    exit 0
  fi

  echo "Trying repo-local Python venv fallback..."
  if install_via_venv; then
    report_ok "$VENV_KEEPER"
    exit 0
  fi

  die "could not install Keeper Commander automatically.

On macOS, install the official package (already downloaded under .tools/keeper/ if present), then re-run mise run install-keeper-cli:
  open .tools/keeper/keeper-commander-mac-*.pkg

Docs: https://docs.keeper.io/keeperpam/commander-cli/commander-installation-setup/installation-on-mac"
}

main "$@"
