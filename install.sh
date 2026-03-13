#!/usr/bin/env bash
# keel installer — https://getkaze.dev/keel/install.sh
# Usage: curl -fsSL https://getkaze.dev/keel/install.sh | sudo bash
set -euo pipefail

BINARY_NAME="keel"
INSTALL_DIR="/usr/local/bin"
REPO="getkaze/keel"
API_BASE="https://api.github.com/repos/${REPO}"
RELEASES_BASE="https://github.com/${REPO}/releases"

# ── color helpers ──────────────────────────────────────────────────────────────
bold=$(tput bold 2>/dev/null || true)
reset=$(tput sgr0 2>/dev/null || true)
green=$(tput setaf 2 2>/dev/null || true)
cyan=$(tput setaf 6 2>/dev/null || true)
yellow=$(tput setaf 3 2>/dev/null || true)
red=$(tput setaf 1 2>/dev/null || true)
dim=$(tput dim 2>/dev/null || true)

info()  { echo "${bold}==>${reset} $*"; }
ok()    { echo "${green}  ✓${reset} $*"; }
fail()  { echo "${red}error:${reset} $*" >&2; exit 1; }

# ── sanity checks ─────────────────────────────────────────────────────────────
[ "$(id -u)" = "0" ] || fail "please run with sudo:\n\n  curl -fsSL https://getkaze.dev/keel/install.sh | sudo bash"

command -v curl &>/dev/null || command -v wget &>/dev/null || fail "curl or wget is required"

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

# Data directory: Linux uses /var/lib/keel, macOS uses ~/.keel
if [ "$OS" = "darwin" ]; then
  REAL_USER="${SUDO_USER:-$(whoami)}"
  REAL_HOME=$(eval echo "~${REAL_USER}")
  KEEL_DIR="${REAL_HOME}/.keel"
else
  KEEL_DIR="/var/lib/keel"
fi

# ── http helper ───────────────────────────────────────────────────────────────
http_get() {
  if command -v curl &>/dev/null; then
    curl -fsSL "$1"
  else
    wget -qO- "$1"
  fi
}

# ── version picker TUI ────────────────────────────────────────────────────────
pick_version() {
  # If KEEL_VERSION is set in env, skip TUI
  if [ -n "${KEEL_VERSION:-}" ]; then
    VERSION="$KEEL_VERSION"
    return
  fi

  info "fetching available releases"

  # Pull tag_name list from GitHub API (no jq needed)
  local releases_json
  releases_json="$(http_get "${API_BASE}/releases" 2>/dev/null)" \
    || fail "could not reach GitHub API — check your internet connection"

  # Extract tag names preserving order (latest first)
  local versions=()
  while IFS= read -r line; do
    versions+=("$line")
  done < <(printf '%s' "$releases_json" | grep '"tag_name"' | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')

  [ "${#versions[@]}" -gt 0 ] || fail "no releases found for ${REPO}"

  # ── draw menu ────────────────────────────────────────────────────────────────
  clear 2>/dev/null || true
  echo ""
  echo "${bold}${cyan}  keel installer${reset}"
  echo "${dim}  ${OS}/${ARCH}${reset}"
  echo ""
  echo "${bold}  Select a version to install:${reset}"
  echo ""

  local i=1
  for v in "${versions[@]}"; do
    if [ "$i" = "1" ]; then
      printf "  ${green}%2d)${reset}  %s  ${dim}(latest)${reset}\n" "$i" "$v"
    else
      printf "  ${yellow}%2d)${reset}  %s\n" "$i" "$v"
    fi
    i=$((i + 1))
  done

  echo ""
  printf "  ${bold}Choice [1]:${reset} "
  local choice
  read -r choice < /dev/tty
  choice="${choice:-1}"

  # Validate input is a number in range
  case "$choice" in
    ''|*[!0-9]*) fail "invalid choice: '${choice}'" ;;
  esac
  [ "$choice" -ge 1 ] && [ "$choice" -le "${#versions[@]}" ] \
    || fail "choice out of range (1–${#versions[@]})"

  VERSION="${versions[$((choice - 1))]}"
  echo ""
}

pick_version

DOWNLOAD_URL="${RELEASES_BASE}/download/${VERSION}/${BINARY_NAME}-${OS}-${ARCH}"

# ── download binary ────────────────────────────────────────────────────────────
info "installing keel ${VERSION} (${OS}/${ARCH})"

TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

if command -v curl &>/dev/null; then
  curl -fsSL "$DOWNLOAD_URL" -o "$TMP"
else
  wget -qO "$TMP" "$DOWNLOAD_URL"
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
# Give the calling (non-root) user ownership of the data directory
# so that `keel target` can be run without sudo.
REAL_USER="${REAL_USER:-${SUDO_USER:-}}"
if [ -n "$REAL_USER" ]; then
  chown -R "${REAL_USER}" "${KEEL_DIR}"
  ok "ownership of ${KEEL_DIR} set to ${REAL_USER}"
fi

# ── done ───────────────────────────────────────────────────────────────────────
echo ""
echo "${bold}keel ${VERSION} installed successfully!${reset}"
echo ""
echo "Quick start:"
echo "  keel target              # show active target"
echo "  keel target ec2          # switch to ec2 target"
echo "  keel start               # start all services"
echo "  keel stop                # stop all services"
echo "  keel reset --all         # recreate all containers"
echo "  keel                     # open the web dashboard (port 60000)"
echo ""
echo "Docs: https://getkaze.dev/docs"
