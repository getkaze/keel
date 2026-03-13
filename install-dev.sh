#!/usr/bin/env bash
# keel dev installer — instala bin/keel local para testes
# Usage: sudo bash install-dev.sh
set -euo pipefail

BINARY_NAME="keel"
INSTALL_DIR="/usr/local/bin"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_PATH="${SCRIPT_DIR}/bin/${BINARY_NAME}"

# Data directory: Linux uses /var/lib/keel, macOS uses ~/.keel
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
if [ "$OS" = "darwin" ]; then
  REAL_USER="${SUDO_USER:-$(whoami)}"
  REAL_HOME=$(eval echo "~${REAL_USER}")
  KEEL_DIR="${REAL_HOME}/.keel"
else
  KEEL_DIR="/var/lib/keel"
fi

bold=$(tput bold 2>/dev/null || true)
reset=$(tput sgr0 2>/dev/null || true)
green=$(tput setaf 2 2>/dev/null || true)
red=$(tput setaf 1 2>/dev/null || true)

info() { echo "${bold}==>${reset} $*"; }
ok()   { echo "${green}  ✓${reset} $*"; }
fail() { echo "${red}error:${reset} $*" >&2; exit 1; }

[ "$(id -u)" = "0" ] || fail "rode com sudo: sudo bash install-dev.sh"
[ -f "$BIN_PATH" ] || fail "bin/keel não encontrado — rode 'make build' primeiro"

# ── instalar binário ───────────────────────────────────────────────────────────
info "instalando ${BIN_PATH} -> ${INSTALL_DIR}/${BINARY_NAME}"
cp "$BIN_PATH" "${INSTALL_DIR}/${BINARY_NAME}"
chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
ok "binário instalado"

# ── criar diretórios de dados ─────────────────────────────────────────────────
info "configurando ${KEEL_DIR}"
mkdir -p "${KEEL_DIR}/data/services"
mkdir -p "${KEEL_DIR}/data/seeders"
mkdir -p "${KEEL_DIR}/data/config/traefik"
mkdir -p "${KEEL_DIR}/state"
ok "diretórios criados"

# Initialize config.json if not present
if [ ! -f "${KEEL_DIR}/data/config.json" ]; then
  cat > "${KEEL_DIR}/data/config.json" <<'JSON'
{
  "network": "keel-net",
  "network_subnet": "172.20.1.0/24"
}
JSON
  ok "config.json criado"
fi

if [ ! -f "${KEEL_DIR}/state/target" ]; then
  echo "local" > "${KEEL_DIR}/state/target"
  ok "target padrão: local"
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
  ok "targets.json criado"
fi

# ── ghcr setup ────────────────────────────────────────────────────────────────
setup_ghcr() {
  if [ -s "${KEEL_DIR}/state/ghcr-user" ] && [ -s "${KEEL_DIR}/state/ghcr-pat" ]; then
    ok "credenciais ghcr já configuradas"
    return
  fi
  echo ""
  echo "${bold}GitHub Container Registry (ghcr.io)${reset}"
  echo "  Necessário para imagens de packages privados do GitHub."
  echo ""
  printf "  Usar ghcr.io? [s/N] "
  read -r use_ghcr < /dev/tty
  case "$use_ghcr" in
    [sS]|[sS][iI][mM])
      echo ""
      printf "  Usuário do GitHub: "
      read -r ghcr_user < /dev/tty
      [ -n "$ghcr_user" ] || { echo "${red}  usuário não pode ser vazio${reset}"; return; }

      printf "  Personal Access Token (PAT): "
      read -rs ghcr_pat < /dev/tty
      echo ""
      [ -n "$ghcr_pat" ] || { echo "${red}  PAT não pode ser vazio${reset}"; return; }

      printf '%s' "$ghcr_user" > "${KEEL_DIR}/state/ghcr-user"
      printf '%s' "$ghcr_pat"  > "${KEEL_DIR}/state/ghcr-pat"
      chmod 600 "${KEEL_DIR}/state/ghcr-user" "${KEEL_DIR}/state/ghcr-pat"
      ok "credenciais ghcr salvas"
      ;;
    *)
      ok "ghcr ignorado"
      ;;
  esac
}
setup_ghcr

# ── ownership ─────────────────────────────────────────────────────────────────
REAL_USER="${REAL_USER:-${SUDO_USER:-}}"
if [ -n "$REAL_USER" ]; then
  chown -R "${REAL_USER}" "${KEEL_DIR}"
  ok "ownership de ${KEEL_DIR} para ${REAL_USER}"
fi

# ── done ──────────────────────────────────────────────────────────────────────
VERSION="$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null || echo "dev")"
echo ""
echo "${bold}keel ${VERSION} instalado (dev build)${reset}"
echo ""
echo "  keel            # dashboard (porta 60000)"
echo "  keel target     # target ativo"
echo "  keel help       # todos os comandos"
echo ""
