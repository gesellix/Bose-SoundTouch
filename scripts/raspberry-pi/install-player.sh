#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# Bose-SoundTouch soundtouch-player installer (systemd, headless)
#
# Usage:
#   sudo bash install-player.sh [vX.Y.Z]
#
# Examples (override defaults via env vars):
#
#   sudo \
#     VERSION=v0.111.3 \
#     HTTP_PORT=8081 \
#     bash install-player.sh
#
#   # With an AfterTouch service link for TTS (HTTPS + self-signed CA):
#   sudo \
#     SERVICE_URL=https://soundtouch.local \
#     SERVICE_CA=/var/lib/soundtouch-service/certs/ca.crt \
#     bash install-player.sh
#
# Or with a version argument to perform an update:
#   sudo bash install-player.sh v0.111.3
#
# Notes:
# - This script downloads a release binary for your CPU (auto-detects armv7/arm64/amd64).
# - soundtouch-player is stateless (no data directory) — it is safe to stop/restart freely.
# - Default port is 8080 (unprivileged — no special capabilities needed).
# - If soundtouch-service is already installed, soundtouch-player reuses the
#   existing soundtouch:soundtouch user/group.
# - Safe to re-run; it will update the binary, env file, and unit and restart.
# ==============================================================================

# Release to install. Empty means "resolve the latest release" (see
# resolve_version). Pass a tag/number as $1 or VERSION=... to pin a release.
VERSION="${1:-${VERSION:-}}"
# Normalize version prefix for an explicitly provided version.
if [[ -n "$VERSION" && ! "$VERSION" =~ ^v ]]; then
  VERSION="v${VERSION}"
fi
GH_REPO="${GH_REPO:-gesellix/Bose-SoundTouch}"
# Used only when the latest-release lookup fails (offline / rate-limited).
FALLBACK_VERSION="${FALLBACK_VERSION:-v0.111.3}"
SERVICE_NAME="${SERVICE_NAME:-soundtouch-player}"
BIN_PATH="${BIN_PATH:-/usr/local/bin/soundtouch-player}"

CONFIG_DIR="${CONFIG_DIR:-/etc/soundtouch-player}"
ENV_FILE="${ENV_FILE:-$CONFIG_DIR/soundtouch-player.env}"

SERVICE_USER="${SERVICE_USER:-soundtouch}"
SERVICE_GROUP="${SERVICE_GROUP:-soundtouch}"

# Port (unprivileged — no CAP_NET_BIND_SERVICE needed)
HTTP_PORT="${HTTP_PORT:-8080}"

# Optional discovery / device config
BIND_ADDR="${BIND_ADDR:-}"
DISCOVERY_INTERFACE="${DISCOVERY_INTERFACE:-}"
SOUNDTOUCH_DEVICES="${SOUNDTOUCH_DEVICES:-}"

# Optional AfterTouch service link (needed for TTS / "Speak").
# SERVICE_URL: base URL of soundtouch-service, e.g. https://soundtouch.local
# SERVICE_CA:  path to the service CA cert when it serves HTTPS with its own
#              self-signed certificate, e.g. /var/lib/soundtouch-service/certs/ca.crt
SERVICE_URL="${SERVICE_URL:-}"
SERVICE_CA="${SERVICE_CA:-}"

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
  echo "https://github.com/gesellix/Bose-SoundTouch/releases/download/${VERSION}/soundtouch-player-${VERSION}-${asset}"
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
    curl -fsSL -o "${tmp}/soundtouch-player" "${url}"
  else
    wget -qO "${tmp}/soundtouch-player" "${url}"
  fi

  chmod +x "${tmp}/soundtouch-player"

  if [[ -f "${BIN_PATH}" ]]; then
    log "Backing up existing binary to ${BIN_PATH}.old"
    cp -p "${BIN_PATH}" "${BIN_PATH}.old"
  fi

  install -m 0755 "${tmp}/soundtouch-player" "${BIN_PATH}"
  log "Installed binary to ${BIN_PATH}"
}

resolve_version() {
  # When no explicit version was given, resolve the latest release tag by
  # following the documented stable redirect:
  #   https://github.com/<owner>/<repo>/releases/latest
  # which 302-redirects to .../releases/tag/vX.Y.Z. We read the final URL and
  # take the tag from it. Falls back to FALLBACK_VERSION on any failure
  # (offline, rate-limited, no usable curl/wget).
  if [[ -n "$VERSION" ]]; then
    return
  fi

  local latest_url="https://github.com/${GH_REPO}/releases/latest"
  log "Resolving latest release via ${latest_url}"

  local effective="" tag=""
  if command -v curl >/dev/null 2>&1; then
    effective="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "$latest_url" 2>/dev/null)" || true
  else
    # wget: don't follow the redirect, read the Location header instead.
    effective="$(wget -S --max-redirect=0 -O /dev/null "$latest_url" 2>&1 \
      | awk 'tolower($1) ~ /location:/ {print $2}' | tr -d '\r' | tail -1)" || true
  fi
  tag="${effective##*/}"

  if [[ "$tag" =~ ^v?[0-9]+\.[0-9]+ ]]; then
    [[ "$tag" =~ ^v ]] || tag="v${tag}"
    VERSION="$tag"
    log "Latest release is ${VERSION}"
  else
    VERSION="$FALLBACK_VERSION"
    log "⚠️ Could not resolve latest release; falling back to ${VERSION}"
  fi
}

self_update() {
  if [[ "$IS_SELF_UPDATE" == "true" ]]; then
    return
  fi

  local url="https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/${VERSION}/scripts/raspberry-pi/install-player.sh"
  local tmp_script="/tmp/soundtouch-player-install-${VERSION}.sh"

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
  export SERVICE_URL SERVICE_CA
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
    "SERVICE_URL=${SERVICE_URL}"
    "SERVICE_CA=${SERVICE_CA}"
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
    log "✅ soundtouch-player is healthy and responding!"
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

soundtouch-player is a control panel — you can stop it when not in use:
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

  resolve_version
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
