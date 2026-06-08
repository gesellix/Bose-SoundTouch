#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Bose-SoundTouch soundtouch-web uninstaller (systemd, headless)
#
# soundtouch-web was renamed to soundtouch-player. This uninstaller removes a
# leftover soundtouch-web install created by the (now deprecated) install-web.sh:
# it stops and disables the systemd service, removes the unit, binary, and config
# directory.
#
# Usage:
#   sudo bash uninstall-web.sh
#
# Notes:
# - soundtouch-web is stateless (no data directory) — nothing to preserve.
# - The shared soundtouch:soundtouch user/group is removed only when no other
#   soundtouch-{service,player,web} install remains on this host.
# - Safe to re-run; every step tolerates already-missing pieces.
# ==============================================================================

SERVICE_NAME="${SERVICE_NAME:-soundtouch-web}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/soundtouch-web}"
CONFIG_DIR="${CONFIG_DIR:-/etc/soundtouch-web}"
SERVICE_USER="${SERVICE_USER:-soundtouch}"
SERVICE_GROUP="${SERVICE_GROUP:-soundtouch}"

log() { printf "\n==> %s\n" "$*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

need_root() {
  [[ "${EUID}" -eq 0 ]] || die "Please run as root (e.g. sudo bash $0)."
}

ensure_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

stop_remove_service() {
  log "Stopping and disabling ${SERVICE_NAME}.service"
  systemctl disable --now "${SERVICE_NAME}.service" 2>/dev/null || true

  local unit="/etc/systemd/system/${SERVICE_NAME}.service"
  if [[ -f "${unit}" ]]; then
    log "Removing systemd unit: ${unit}"
    rm -f "${unit}"
  fi
  systemctl daemon-reload
  systemctl reset-failed "${SERVICE_NAME}.service" 2>/dev/null || true
}

remove_binary() {
  if [[ -e "${BIN_PATH}" || -e "${BIN_PATH}.old" ]]; then
    log "Removing binary: ${BIN_PATH} (and ${BIN_PATH}.old)"
    rm -f "${BIN_PATH}" "${BIN_PATH}.old"
  fi
}

remove_config() {
  if [[ -d "${CONFIG_DIR}" ]]; then
    log "Removing config directory: ${CONFIG_DIR}"
    rm -rf "${CONFIG_DIR}"
  fi
}

# Remove the shared soundtouch:soundtouch user/group only when no other
# soundtouch-{service,player,web} install remains on this host.
remove_user_group_if_unused() {
  local n
  for n in service player web; do
    if [[ -f "/etc/systemd/system/soundtouch-${n}.service" ]] || \
       [[ -e "/usr/local/bin/soundtouch-${n}" ]]; then
      log "Keeping ${SERVICE_USER}:${SERVICE_GROUP} — still used by soundtouch-${n}."
      return
    fi
  done

  if id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    log "No other soundtouch installs remain; removing user ${SERVICE_USER}"
    userdel "${SERVICE_USER}" 2>/dev/null || true
  fi
  if getent group "${SERVICE_GROUP}" >/dev/null 2>&1; then
    groupdel "${SERVICE_GROUP}" 2>/dev/null || true
  fi
}

main() {
  need_root
  ensure_cmd systemctl

  stop_remove_service
  remove_binary
  remove_config
  remove_user_group_if_unused

  log "✅ soundtouch-web has been removed."
  cat <<'EOF'

soundtouch-web is the old name for soundtouch-player. To install the current
control panel instead:

  curl -fsSL -o install-player.sh \
    https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-player.sh
  sudo bash install-player.sh
EOF
}

main "$@"
