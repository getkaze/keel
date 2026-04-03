#!/usr/bin/env bash
# keel dev installer — installs bin/keel locally for testing
# Usage: sudo bash install-dev.sh
set -euo pipefail

BINARY_NAME="keel"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="${SCRIPT_DIR}/bin/${BINARY_NAME}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
REAL_USER="${SUDO_USER:-$(whoami)}"
REAL_HOME=$(eval echo "~${REAL_USER}")

# Install to user-writable directory
if [ "$(id -u)" = "0" ]; then
  INSTALL_DIR="${REAL_HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
  chown "$REAL_USER" "$INSTALL_DIR"
else
  INSTALL_DIR="${HOME}/.local/bin"
  mkdir -p "$INSTALL_DIR"
fi

# Data directory
if [ "$OS" = "darwin" ]; then
  KEEL_DIR="${REAL_HOME}/.keel"
elif [ "$(id -u)" = "0" ]; then
  KEEL_DIR="/var/lib/keel"
else
  KEEL_DIR="${HOME}/.keel"
fi

bold=$(tput bold 2>/dev/null || true)
reset=$(tput sgr0 2>/dev/null || true)
green=$(tput setaf 2 2>/dev/null || true)
red=$(tput setaf 1 2>/dev/null || true)

info() { echo "${bold}==>${reset} $*"; }
ok()   { echo "${green}  ✓${reset} $*"; }
fail() { echo "${red}error:${reset} $*" >&2; exit 1; }

[ -f "$BIN_PATH" ] || fail "bin/keel not found — run 'make build' first"

# ── install binary ─────────────────────────────────────────────────────────────
info "installing ${BIN_PATH} -> ${INSTALL_DIR}/${BINARY_NAME}"
cp "$BIN_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
ok "binary installed"

# ── create data directories ────────────────────────────────────────────────────
info "configuring ${KEEL_DIR}"
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
  ok "config.json created"
fi

if [ ! -f "${KEEL_DIR}/state/target" ]; then
  echo "local" > "${KEEL_DIR}/state/target"
  ok "default target: local"
fi

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
  ok "targets.json created"
fi

# ── ghcr setup ────────────────────────────────────────────────────────────────
setup_ghcr() {
  if [ -s "${KEEL_DIR}/state/ghcr-user" ] && [ -s "${KEEL_DIR}/state/ghcr-pat" ]; then
    ok "ghcr credentials already configured"
    return
  fi
  echo ""
  echo "${bold}GitHub Container Registry (ghcr.io)${reset}"
  echo "  Required for private GitHub package images."
  echo ""
  printf "  Use ghcr.io? [y/N] "
  read -r use_ghcr < /dev/tty
  case "$use_ghcr" in
    [yY]|[yY][eE][sS])
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
      ok "ghcr skipped"
      ;;
  esac
}
setup_ghcr

# ── ownership ─────────────────────────────────────────────────────────────────
if [ "$(id -u)" = "0" ] && [ -n "${SUDO_USER:-}" ]; then
  chown -R "${SUDO_USER}" "${KEEL_DIR}"
  chown "${SUDO_USER}" "${INSTALL_DIR}/${BINARY_NAME}"
  ok "ownership set to ${SUDO_USER} (self-update enabled)"
fi

# ── PATH setup ────────────────────────────────────────────────────────────────
patch_shell_profile() {
  local export_line="export PATH=\"${INSTALL_DIR}:\$PATH\""
  local profile=""

  local user_shell
  user_shell="$(basename "${SHELL:-}")"
  case "$user_shell" in
    zsh)   profile="${REAL_HOME}/.zshrc" ;;
    bash)
      if [ "$OS" = "darwin" ]; then
        profile="${REAL_HOME}/.bash_profile"
      else
        profile="${REAL_HOME}/.bashrc"
      fi
      ;;
    fish)  profile="${REAL_HOME}/.config/fish/config.fish"
           export_line="fish_add_path ${INSTALL_DIR}" ;;
    *)     profile="" ;;
  esac

  if [ -z "$profile" ]; then
    echo ""
    echo "${bold}Note:${reset} add ${INSTALL_DIR} to your PATH manually:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    return
  fi

  if [ -f "$profile" ] && grep -qF "${INSTALL_DIR}" "$profile" 2>/dev/null; then
    return
  fi

  printf '\n# Added by keel installer\n%s\n' "$export_line" >> "$profile"
  ok "added ${INSTALL_DIR} to PATH in ${profile}"
  echo "  Run: ${bold}source ${profile}${reset}  (or open a new terminal)"
}

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) patch_shell_profile ;;
esac

# ── done ──────────────────────────────────────────────────────────────────────
VERSION="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null || echo "dev")"
echo ""
echo "${bold}keel ${VERSION} installed (dev build)${reset}"
echo ""
echo "  keel            # dashboard (port 60000)"
echo "  keel target     # active target"
echo "  keel help       # all commands"
echo ""
