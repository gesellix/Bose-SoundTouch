#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Bose-SoundTouch soundtouch-service installer (systemd, headless)
#
# Usage:
#   sudo bash install.sh [vX.Y.Z]
#
# Examples (override defaults via env vars):
#
#   sudo \
#     VERSION=v0.17.0 \
#     HOSTNAME_FQDN=soundtouch.local \
#     HTTP_PORT=80 \
#     HTTPS_PORT=443 \
#     DATA_DIR=/var/lib/soundtouch-service \
#     bash install.sh
#
# Or with a version argument to perform an update:
#   sudo bash install.sh v0.18.1
#
# Notes:
# - This script downloads a release binary for your CPU (auto-detects armv7/arm64/amd64).
# - It installs a systemd unit that can bind privileged ports (80/443) using:
#     AmbientCapabilities=CAP_NET_BIND_SERVICE
#   so you do NOT need setcap and do NOT need to run as root.
# - Safe to re-run; it will update binary/config/unit and restart the service.
# ==============================================================================

VERSION="${1:-${VERSION:-v0.17.0}}"
# Normalize version prefix
if [[ ! "$VERSION" =~ ^v ]]; then
  VERSION="v${VERSION}"
fi
SERVICE_NAME="${SERVICE_NAME:-soundtouch-service}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/soundtouch-service}"

CONFIG_DIR="${CONFIG_DIR:-/etc/soundtouch-service}"
ENV_FILE="${ENV_FILE:-$CONFIG_DIR/soundtouch-service.env}"
DATA_DIR="${DATA_DIR:-/var/lib/soundtouch-service}"

SERVICE_USER="${SERVICE_USER:-soundtouch}"
SERVICE_GROUP="${SERVICE_GROUP:-soundtouch}"

# Ports
HTTP_PORT="${HTTP_PORT:-80}"
HTTPS_PORT="${HTTPS_PORT:-443}"

# URLs (default uses current hostname + .local)
HOSTNAME_FQDN="${HOSTNAME_FQDN:-$(hostname).local}"
SERVER_URL="${SERVER_URL:-http://${HOSTNAME_FQDN}}"
HTTPS_SERVER_URL="${HTTPS_SERVER_URL:-https://${HOSTNAME_FQDN}}"

# Additional env vars (mirrors the project's docker-compose.yml)
LOG_PROXY_BODY="${LOG_PROXY_BODY:-false}"
REDACT_PROXY_LOGS="${REDACT_PROXY_LOGS:-true}"
RECORD_INTERACTIONS="${RECORD_INTERACTIONS:-true}"
DISCOVERY_INTERVAL="${DISCOVERY_INTERVAL:-5m}"

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
  # Upstream release naming expects: linux-armv7, linux-arm64, linux-amd64
  # Map uname -m to those.
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
  # Release asset pattern used by you earlier:
  # soundtouch-service-v0.17.0-linux-armv7
  echo "https://github.com/gesellix/Bose-SoundTouch/releases/download/${VERSION}/soundtouch-service-${VERSION}-${asset}"
}

ensure_user_group() {
  log "Ensuring service user/group exist: ${SERVICE_USER}:${SERVICE_GROUP}"
  if ! getent group "${SERVICE_GROUP}" >/dev/null; then
    groupadd --system "${SERVICE_GROUP}"
  fi
  if ! id -u "${SERVICE_USER}" >/dev/null 2>&1; then
    useradd --system \
      --home "${DATA_DIR}" \
      --create-home \
      --shell /usr/sbin/nologin \
      --gid "${SERVICE_GROUP}" \
      "${SERVICE_USER}"
  fi
}

ensure_dirs() {
  log "Creating directories"
  mkdir -p "${CONFIG_DIR}" "${DATA_DIR}"

  # Optimized ownership check: only chown if not already owned by service user
  if [[ "$(stat -c '%U:%G' "${DATA_DIR}")" != "${SERVICE_USER}:${SERVICE_GROUP}" ]]; then
    log "Adjusting ownership of ${DATA_DIR} to ${SERVICE_USER}:${SERVICE_GROUP}"
    chown -R "${SERVICE_USER}:${SERVICE_GROUP}" "${DATA_DIR}"
  fi

  chmod 0755 "${CONFIG_DIR}" "${DATA_DIR}"
}

download_binary() {
  local asset url tmp
  asset="${ARCH_ASSET:-$(detect_arch_asset)}"
  url="$(download_url_for "$asset")"

  log "Downloading binary for ${asset}: ${url}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "${tmp}"' EXIT

  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -o "${tmp}/soundtouch-service" "${url}"
  else
    wget -qO "${tmp}/soundtouch-service" "${url}"
  fi

  chmod +x "${tmp}/soundtouch-service"

  # Backup existing binary if it exists
  if [[ -f "${BIN_PATH}" ]]; then
    log "Backing up existing binary to ${BIN_PATH}.old"
    cp -p "${BIN_PATH}" "${BIN_PATH}.old"
  fi

  install -m 0755 "${tmp}/soundtouch-service" "${BIN_PATH}"
  log "Installed binary to ${BIN_PATH}"
}

self_update() {
  # If we are already a self-update re-exec, don't do it again
  if [[ "$IS_SELF_UPDATE" == "true" ]]; then
    return
  fi

  local url="https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/${VERSION}/scripts/raspberry-pi/install.sh"
  local tmp_script="/tmp/soundtouch-install-${VERSION}.sh"

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

  # Compare scripts to see if we actually need to re-exec
  if diff -q "${SCRIPT_PATH}" "${tmp_script}" >/dev/null 2>&1; then
    log "Installer is already up to date."
    rm -f "${tmp_script}"
    return
  fi

  log "Newer installer found for ${VERSION}. Re-executing..."
  chmod +x "${tmp_script}"

  # Export current env vars to the new script
  export IS_SELF_UPDATE="true"
  export VERSION HOSTNAME_FQDN HTTP_PORT HTTPS_PORT DATA_DIR BIN_PATH CONFIG_DIR ENV_FILE SERVICE_USER SERVICE_GROUP

  exec "${tmp_script}" "$@"
}

write_env_file() {
  log "Writing env file: ${ENV_FILE}"
  cat > "${ENV_FILE}" <<EOF
PORT=${HTTP_PORT}
HTTPS_PORT=${HTTPS_PORT}
DATA_DIR=${DATA_DIR}

LOG_PROXY_BODY=${LOG_PROXY_BODY}
REDACT_PROXY_LOGS=${REDACT_PROXY_LOGS}
RECORD_INTERACTIONS=${RECORD_INTERACTIONS}
DISCOVERY_INTERVAL=${DISCOVERY_INTERVAL}

SERVER_URL=${SERVER_URL}
HTTPS_SERVER_URL=${HTTPS_SERVER_URL}
EOF
  chmod 0640 "${ENV_FILE}"
  # group-readable so you can add yourself to the group if desired
  chown root:"${SERVICE_GROUP}" "${ENV_FILE}" || true
}

write_systemd_unit() {
  log "Writing systemd unit: /etc/systemd/system/${SERVICE_NAME}.service"
  cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=Bose SoundTouch Service
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=${SERVICE_GROUP}
EnvironmentFile=${ENV_FILE}
WorkingDirectory=${DATA_DIR}
ExecStart=${BIN_PATH}

# Allow binding to privileged ports (80/443) without running as root
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE

Restart=on-failure
RestartSec=2

# Sensible hardening (compatible with privileged-port binding)
PrivateTmp=true
ProtectSystem=full
ProtectHome=true
ReadWritePaths=${DATA_DIR}

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
    log "✅ Service is healthy and responding!"
  else
    log "⚠️ Service started but did not respond to health check at $health_url within timeout."
    log "Check logs with: journalctl -u ${SERVICE_NAME}.service -n 50"
  fi
}

show_status() {
  log "Service status"
  systemctl --no-pager --full status "${SERVICE_NAME}.service" || true

  log "Listening sockets (${HTTP_PORT}/${HTTPS_PORT})"
  ss -tulpn | grep -E ":((${HTTP_PORT})|(${HTTPS_PORT}))\b" || true

  if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "Status: active"; then
    log "Firewall check (UFW is active)"
    if ! ufw status | grep -qE "${HTTP_PORT}.*ALLOW|${HTTPS_PORT}.*ALLOW"; then
      log "⚠️ UFW is active but ports ${HTTP_PORT}/${HTTPS_PORT} might be blocked."
      log "Run: sudo ufw allow ${HTTP_PORT}/tcp && sudo ufw allow ${HTTPS_PORT}/tcp"
    else
      log "✅ UFW rules for service ports appear to be in place."
    fi
  fi

  cat <<EOF

Try from another machine:
  ${SERVER_URL}
  ${HTTPS_SERVER_URL}

If mDNS doesn't work, use the Pi's IP:
  http://<pi-ip>/
  https://<pi-ip>/

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
