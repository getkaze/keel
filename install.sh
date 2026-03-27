#!/usr/bin/env bash
# keel installer — https://getkaze.dev/keel/install.sh
# Usage: curl -fsSL https://getkaze.dev/keel/install.sh | sudo bash
set -euo pipefail

BINARY_NAME="keel"
RELEASES_BASE="https://github.com/getkaze/keel/releases"

# Install directory: always ~/.local/bin (user-writable, enables self-update without sudo).
REAL_USER="${SUDO_USER:-$(whoami)}"
REAL_HOME=$(eval echo "~${REAL_USER}")
if [ "$(id -u)" = "0" ]; then
  INSTALL_DIR="${REAL_HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
  chown "$REAL_USER" "$INSTALL_DIR"
else
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

# ── color helpers ──────────────────────────────────────────────────────────────
bold=$(tput bold 2>/dev/null || true)
reset=$(tput sgr0 2>/dev/null || true)
green=$(tput setaf 2 2>/dev/null || true)
red=$(tput setaf 1 2>/dev/null || true)

info()  { echo "${bold}==>${reset} $*"; }
ok()    { echo "${green}  ✓${reset} $*"; }
fail()  { echo "${red}error:${reset} $*" >&2; exit 1; }

# ── sanity checks ─────────────────────────────────────────────────────────────
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  ;;
  darwin) ;;
  *)      fail "unsupported OS: $OS (supported: linux, darwin)" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64)  ARCH="amd64" ;;
  aarch64) ARCH="arm64" ;;
  arm64)   ARCH="arm64" ;;
  *)       fail "unsupported architecture: $ARCH" ;;
esac

# Data directory: Linux uses /var/lib/keel (with sudo) or ~/.keel, macOS uses ~/.keel
if [ "$OS" = "darwin" ]; then
  KEEL_DIR="${REAL_HOME}/.keel"
elif [ "$(id -u)" = "0" ]; then
  KEEL_DIR="/var/lib/keel"
else
  KEEL_DIR="${HOME}/.keel"
fi

# Detect KEEL_VERSION (optional — defaults to latest)
VERSION="${KEEL_VERSION:-}"
if [ -z "$VERSION" ]; then
  info "fetching latest version"
  VERSION="$(curl -fsSL -o /dev/null -w '%{url_effective}' "${RELEASES_BASE}/latest" 2>/dev/null || true)"
  VERSION="${VERSION##*/}"
  [ -n "$VERSION" ] && [ "$VERSION" != "latest" ] || fail "could not fetch latest version from ${RELEASES_BASE}/latest"
  VERSION="$(echo "$VERSION" | tr -d '[:space:]')"
fi

DOWNLOAD_URL="${RELEASES_BASE}/download/${VERSION}/${BINARY_NAME}-${OS}-${ARCH}"

# ── download binary ────────────────────────────────────────────────────────────
info "installing keel ${VERSION} (${OS}/${ARCH})"

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

if command -v curl &>/dev/null; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TMP"
elif command -v wget &>/dev/null; then
  wget -qO "$TMP" "$DOWNLOAD_URL"
else
  fail "curl or wget is required"
fi

chmod +x "$TMP"
mv "$TMP" "${INSTALL_DIR}/${BINARY_NAME}"
ok "binary installed to ${INSTALL_DIR}/${BINARY_NAME}"

# ── create data directories ────────────────────────────────────────────────────
info "setting up data directory (${KEEL_DIR})"

mkdir -p "${KEEL_DIR}/data/services"
mkdir -p "${KEEL_DIR}/data/seeders"
mkdir -p "${KEEL_DIR}/data/config/traefik"
mkdir -p "${KEEL_DIR}/state"
ok "directories created"

# Initialize config.json if not present
if [ ! -f "${KEEL_DIR}/data/config.json" ]; then
  cat > "${KEEL_DIR}/data/config.json" <<'JSON'
{
  "network": "keel-net",
  "network_subnet": "172.20.1.0/24"
}
JSON
  ok "created default config.json"
fi

# Initialize state file (active target) if not present
if [ ! -f "${KEEL_DIR}/state/target" ]; then
  echo "local" > "${KEEL_DIR}/state/target"
  ok "default target set to: local"
fi

# Initialize targets.json with a local target if not present
if [ ! -f "${KEEL_DIR}/data/targets.json" ]; then
  cat > "${KEEL_DIR}/data/targets.json" <<'JSON'
{
  "targets": {
    "local": {
      "host": "127.0.0.1",
      "ssh_user": null,
      "port_bind": "127.0.0.1",
      "description": "Docker local"
    }
  },
  "default": "local"
}
JSON
  ok "created default targets.json"
fi

# ── ghcr setup ────────────────────────────────────────────────────────────────
setup_ghcr() {
  if [ -s "${KEEL_DIR}/state/ghcr-user" ] && [ -s "${KEEL_DIR}/state/ghcr-pat" ]; then
    ok "ghcr credentials already configured"
    return
  fi
  echo ""
  echo "${bold}GitHub Container Registry (ghcr.io)${reset}"
  echo "  Required to pull images from private GitHub packages."
  echo ""
  printf "  Use ghcr.io? [s/N] "
  read -r use_ghcr < /dev/tty
  case "$use_ghcr" in
    [sS]|[sS][iI][mM])
      echo ""
      printf "  GitHub username: "
      read -r ghcr_user < /dev/tty
      [ -n "$ghcr_user" ] || { echo "${red}  username cannot be empty${reset}"; return; }

      printf "  Personal Access Token (PAT): "
      read -rs ghcr_pat < /dev/tty
      echo ""
      [ -n "$ghcr_pat" ] || { echo "${red}  PAT cannot be empty${reset}"; return; }

      printf '%s' "$ghcr_user" > "${KEEL_DIR}/state/ghcr-user"
      printf '%s' "$ghcr_pat"  > "${KEEL_DIR}/state/ghcr-pat"
      chmod 600 "${KEEL_DIR}/state/ghcr-user" "${KEEL_DIR}/state/ghcr-pat"
      ok "ghcr credentials saved"
      ;;
    *)
      ok "skipping ghcr setup"
      ;;
  esac
}
setup_ghcr

# ── ownership ──────────────────────────────────────────────────────────────────
# Give the calling (non-root) user ownership so keel can self-update without sudo.
if [ "$(id -u)" = "0" ] && [ -n "${SUDO_USER:-}" ]; then
  chown -R "${SUDO_USER}" "${KEEL_DIR}"
  chown "${SUDO_USER}" "${INSTALL_DIR}/${BINARY_NAME}"
  ok "ownership set to ${SUDO_USER} (self-update enabled)"
fi

# ── done ───────────────────────────────────────────────────────────────────────
echo ""
echo "${bold}keel ${VERSION} installed successfully!${reset}"

# Check if install dir is in PATH
case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    echo ""
    echo "${bold}Note:${reset} ${INSTALL_DIR} is not in your PATH."
    echo "  Add it to your shell profile:"
    echo ""
    echo "    export PATH=\"${INSTALL_DIR}:\$PATH\""
    echo ""
    ;;
esac

echo ""
echo "Quick start:"
echo "  keel target              # show active target"
echo "  keel target ec2          # switch to ec2 target"
echo "  keel start               # start all services"
echo "  keel stop                # stop all services"
echo "  keel reset --all         # recreate all containers"
echo "  keel                     # open the web dashboard (port 60000)"
echo ""
echo "Docs: https://getkaze.dev/keel/docs"
