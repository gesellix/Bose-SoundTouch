#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Bose-SoundTouch soundtouch-web installer (systemd, headless)
#
# Usage:
#   sudo bash install-web.sh [vX.Y.Z]
#
# Examples (override defaults via env vars):
#
#   sudo \
#     VERSION=v0.95.0 \
#     HTTP_PORT=8081 \
#     bash install-web.sh
#
# Or with a version argument to perform an update:
#   sudo bash install-web.sh v0.95.0
#
# Notes:
# - This script downloads a release binary for your CPU (auto-detects armv7/arm64/amd64).
# - soundtouch-web is stateless (no data directory) — it is safe to stop/restart freely.
# - Default port is 8080 (unprivileged — no special capabilities needed).
# - If soundtouch-service is already installed, soundtouch-web reuses the
#   existing soundtouch:soundtouch user/group.
# - Safe to re-run; it will update the binary, env file, and unit and restart.
# ==============================================================================

VERSION="${1:-${VERSION:-v0.97.0}}"
# Normalize version prefix
if [[ ! "$VERSION" =~ ^v ]]; then
  VERSION="v${VERSION}"
fi
SERVICE_NAME="${SERVICE_NAME:-soundtouch-web}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/soundtouch-web}"

CONFIG_DIR="${CONFIG_DIR:-/etc/soundtouch-web}"
ENV_FILE="${ENV_FILE:-$CONFIG_DIR/soundtouch-web.env}"

SERVICE_USER="${SERVICE_USER:-soundtouch}"
SERVICE_GROUP="${SERVICE_GROUP:-soundtouch}"

# Port (unprivileged — no CAP_NET_BIND_SERVICE needed)
HTTP_PORT="${HTTP_PORT:-8080}"

# Optional discovery / device config
BIND_ADDR="${BIND_ADDR:-}"
DISCOVERY_INTERFACE="${DISCOVERY_INTERFACE:-}"
SOUNDTOUCH_DEVICES="${SOUNDTOUCH_DEVICES:-}"

# Override if you want to force a specific asset suffix:
#   ARCH_ASSET=linux-armv7|linux-arm64|linux-amd64
ARCH_ASSET="${ARCH_ASSET:-}"

# Internal variables
SCRIPT_PATH="$(realpath "$0" 2>/dev/null || echo "$0")"
IS_SELF_UPDATE="${IS_SELF_UPDATE:-false}"

log() { printf "\n==> %s\n" "$*"; }
die() { echo "ERROR: $*" >&2; exit 1; }

need_root() {
  [[ "${EUID}" -eq 0 ]] || die "Please run as root (e.g. sudo bash $0)."
}

ensure_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "Missing required command: $1"
}

apt_install_if_missing() {
  log "Installing dependencies: $*"
  apt-get update -y
  apt-get install -y --no-install-recommends "$@"
}

detect_arch_asset() {
  local m
  m="$(uname -m)"

  case "$m" in
    armv7l|armv6l)
      echo "linux-armv7"
      ;;
    aarch64)
      echo "linux-arm64"
      ;;
    x86_64|amd64)
      echo "linux-amd64"
      ;;
    *)
      die "Unsupported architecture from uname -m: $m (set ARCH_ASSET manually)"
      ;;
  esac
}

download_url_for() {
  local asset="$1"
  echo "https://github.com/gesellix/Bose-SoundTouch/releases/download/${VERSION}/soundtouch-web-${VERSION}-${asset}"
}

ensure_user_group() {
  log "Ensuring service user/group exist: ${SERVICE_USER}:${SERVICE_GROUP}"
  if ! getent group "${SERVICE_GROUP}" >/dev/null; then
    groupadd --system "${SERVICE_GROUP}"
  fi
  if ! id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    useradd --system \
      --no-create-home \
      --shell /usr/sbin/nologin \
      --gid "${SERVICE_GROUP}" \
      "${SERVICE_USER}"
  fi
}

ensure_dirs() {
  log "Creating config directory"
  mkdir -p "${CONFIG_DIR}"
  chmod 0755 "${CONFIG_DIR}"
}

download_binary() {
  local asset url tmp=""
  asset="${ARCH_ASSET:-$(detect_arch_asset)}"
  url="$(download_url_for "$asset")"

  log "Downloading binary for ${asset}: ${url}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' EXIT

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "${tmp}/soundtouch-web" "${url}"
  else
    wget -qO "${tmp}/soundtouch-web" "${url}"
  fi

  chmod +x "${tmp}/soundtouch-web"

  if [[ -f "${BIN_PATH}" ]]; then
    log "Backing up existing binary to ${BIN_PATH}.old"
    cp -p "${BIN_PATH}" "${BIN_PATH}.old"
  fi

  install -m 0755 "${tmp}/soundtouch-web" "${BIN_PATH}"
  log "Installed binary to ${BIN_PATH}"
}

self_update() {
  if [[ "$IS_SELF_UPDATE" == "true" ]]; then
    return
  fi

  local url="https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/${VERSION}/scripts/raspberry-pi/install-web.sh"
  local tmp_script="/tmp/soundtouch-web-install-${VERSION}.sh"

  log "Checking for installer updates for ${VERSION}..."
  log "URL: ${url}"

  if command -v curl >/dev/null 2>&1; then
    if ! curl -fsSL -o "${tmp_script}" "${url}"; then
      log "⚠️ Could not fetch installer for ${VERSION}, continuing with current script."
      return
    fi
  else
    if ! wget -qO "${tmp_script}" "${url}"; then
      log "⚠️ Could not fetch installer for ${VERSION}, continuing with current script."
      return
    fi
  fi

  if diff -q "${SCRIPT_PATH}" "${tmp_script}" >/dev/null 2>&1; then
    log "Installer is already up to date."
    rm -f "${tmp_script}"
    return
  fi

  log "Newer installer found for ${VERSION}. Updating ${SCRIPT_PATH} and re-executing..."
  install -m 0755 "${tmp_script}" "${SCRIPT_PATH}"
  rm -f "${tmp_script}"

  export IS_SELF_UPDATE="true"
  export VERSION HTTP_PORT BIND_ADDR DISCOVERY_INTERFACE SOUNDTOUCH_DEVICES
  export BIN_PATH CONFIG_DIR ENV_FILE SERVICE_USER SERVICE_GROUP

  exec "${SCRIPT_PATH}" "$@"
}

write_env_file() {
  log "Updating env file: ${ENV_FILE}"

  local vars=(
    "PORT=${HTTP_PORT}"
    "BIND_ADDR=${BIND_ADDR}"
    "DISCOVERY_INTERFACE=${DISCOVERY_INTERFACE}"
    "SOUNDTOUCH_DEVICES=${SOUNDTOUCH_DEVICES}"
  )

  if [[ ! -f "${ENV_FILE}" ]]; then
    for entry in "${vars[@]}"; do
      echo "${entry}" >> "${ENV_FILE}"
    done
  else
    for entry in "${vars[@]}"; do
      local key="${entry%%=*}"
      local val="${entry#*=}"
      if ! grep -q "^${key}=" "${ENV_FILE}"; then
        echo "${key}=${val}" >> "${ENV_FILE}"
      fi
    done
  fi

  chmod 0640 "${ENV_FILE}"
  chown root:"${SERVICE_GROUP}" "${ENV_FILE}" || true
}

write_systemd_unit() {
  log "Writing systemd unit: /etc/systemd/system/${SERVICE_NAME}.service"
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=Bose SoundTouch Web UI
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN_PATH}
Restart=on-failure
RestartSec=2

PrivateTmp=true
ProtectSystem=strict
ProtectHome=true

[Install]
WantedBy=multi-user.target
EOF
}

reload_enable_start() {
  log "Reloading systemd, enabling and starting service"
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}.service"
  systemctl restart "${SERVICE_NAME}.service"

  log "Verifying service health..."
  local health_url="http://localhost:${HTTP_PORT}/health"
  local max_retries=5
  local count=0
  local success=false

  while [[ $count -lt $max_retries ]]; do
    if curl -fs "$health_url" >/dev/null 2>&1; then
      success=true
      break
    fi
    echo "Waiting for service to respond at $health_url... ($((count+1))/$max_retries)"
    sleep 2
    count=$((count+1))
  done

  if [[ "$success" = true ]]; then
    log "✅ soundtouch-web is healthy and responding!"
  else
    log "⚠️ Service started but did not respond at $health_url within timeout."
    log "Check logs with: journalctl -u ${SERVICE_NAME}.service -n 50"
  fi
}

show_status() {
  log "Service status"
  systemctl --no-pager --full status "${SERVICE_NAME}.service" || true

  log "Listening socket (:${HTTP_PORT})"
  ss -tulpn | grep -E ":${HTTP_PORT}\b" || true

  if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
    log "Firewall check (UFW is active)"
    if ! ufw status | grep -qE "${HTTP_PORT}.*ALLOW"; then
      log "⚠️ UFW is active but port ${HTTP_PORT} might be blocked."
      log "Run: sudo ufw allow ${HTTP_PORT}/tcp"
    else
      log "✅ UFW rule for port ${HTTP_PORT} appears to be in place."
    fi
  fi

  cat <<EOF

Open in your browser:
  http://<pi-ip>:${HTTP_PORT}/

soundtouch-web is a control panel — you can stop it when not in use:
  sudo systemctl stop ${SERVICE_NAME}
  sudo systemctl start ${SERVICE_NAME}

Logs:
  journalctl -u ${SERVICE_NAME}.service -e --no-pager
EOF
}

main() {
  need_root
  ensure_cmd systemctl
  ensure_cmd ss

  if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
    apt_install_if_missing curl
  fi

  self_update "$@"

  ensure_user_group
  ensure_dirs
  download_binary
  write_env_file
  write_systemd_unit
  reload_enable_start
  show_status
}

main "$@"
