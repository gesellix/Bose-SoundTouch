#!/usr/bin/env bash
# Emits a "Quick downloads" markdown section with real, direct download
# links for soundtouch-service and soundtouch-cli, one row per platform.
# Asset URLs are deterministic (<binary>-<tag>-<os>-<arch>[.exe]), so this
# needs no GitHub API call to build them.
#
# Usage: quick-downloads.sh <tag-name> <owner/repo>
# Output goes to stdout, wrapped in <!-- quick-downloads:start/end -->
# markers so callers can find-and-replace a previously inserted block.

set -euo pipefail

TAG_NAME="$1"
REPOSITORY="$2"
BASE_URL="https://github.com/${REPOSITORY}/releases/download/${TAG_NAME}"

# suffix|human label, same order as docs/content/docs/downloads/_index.md
PLATFORMS=(
  "linux-arm64|Raspberry Pi (64-bit) / ARM64 Linux"
  "linux-armv7|Raspberry Pi (32-bit) / ARMv7"
  "linux-amd64|Linux (64-bit PC)"
  "darwin-arm64|macOS (Apple Silicon)"
  "darwin-amd64|macOS (Intel)"
  "windows-amd64.exe|Windows (64-bit)"
  "freebsd-amd64|FreeBSD (64-bit)"
)

build_table() {
  local BINARY_NAME=$1
  echo "| Platform | Download | Checksum |"
  echo "|---|---|---|"
  for ENTRY in "${PLATFORMS[@]}"; do
    local SUFFIX="${ENTRY%%|*}"
    local LABEL="${ENTRY##*|}"
    local FILENAME="${BINARY_NAME}-${TAG_NAME}-${SUFFIX}"
    echo "| ${LABEL} | [${FILENAME}](${BASE_URL}/${FILENAME}) | [sha256](${BASE_URL}/${FILENAME}.sha256) |"
  done
}

SERVICE_TABLE="$(build_table soundtouch-service)"
CLI_TABLE="$(build_table soundtouch-cli)"

cat << EOF
<!-- quick-downloads:start -->
## Quick downloads

Most people only need one of these two:

**soundtouch-service** — the local server that replaces the Bose cloud. Point your speaker at it and you keep full control; the built-in web UI on port 8000 handles setup.

$SERVICE_TABLE

**soundtouch-cli** — command-line control of any device: playback, presets, sources, multiroom zones, discovery, and migration. Good for scripting and home automation.

$CLI_TABLE

Everything else (soundtouch-player, soundtouch-backup, other platforms, Docker, install scripts): [Downloads page](https://gesellix.github.io/Bose-SoundTouch/docs/downloads/).
<!-- quick-downloads:end -->
EOF
