#!/bin/bash
#
# spotify-boot-primer — Self-contained Spotify primer for Bose SoundTouch speakers
#
# Runs at boot (via /mnt/nv/rc.local), waits for the ZeroConf endpoint to
# come up, fetches a fresh Spotify token from a soundtouch-service server, and
# primes the speaker. No Spotify credentials stored on the device.
#
# Only needs: curl, grep, sed (all available on the speaker via busybox).
#
# Install:
#   1. mkdir -p /mnt/nv/soundtouch-service
#   2. Copy this script to /mnt/nv/soundtouch-service/spotify-boot-primer
#   3. Create /mnt/nv/soundtouch-service/spotify-primer.conf
#   4. Create /mnt/nv/rc.local that backgrounds this script
#   5. chmod +x /mnt/nv/rc.local /mnt/nv/soundtouch-service/spotify-boot-primer
#
# Config file format (/mnt/nv/soundtouch-service/spotify-primer.conf):
#   SOUNDTOUCH_URL=https://soundtouch.example.com
#   SOUNDTOUCH_USER=admin
#   SOUNDTOUCH_PASS=secret
#
# Related:
#   https://github.com/gesellix/Bose-SoundTouch
#
set -uo pipefail

CONF="/mnt/nv/soundtouch-service/spotify-primer.conf"
LOG_TAG="spotify-primer[$$]"
ZC_URL="http://localhost:8200/zc"
MAX_WAIT=120    # max seconds to wait for port 8200
RETRY_DELAY=3   # seconds between retries

# --- Logging ---
log() {
    logger -s -t "$LOG_TAG" -p "$1" "$2"
}

# --- JSON parsing without jq ---
# Extract a string value: echo '{"key":"val"}' | json_str key
json_str() {
    grep -o "\"$1\" *: *\"[^\"]*\"" | sed "s/\"$1\" *: *\"//;s/\"$//"
}

# Extract a numeric value: echo '{"key":123}' | json_num key
json_num() {
    grep -o "\"$1\" *: *[0-9]*" | sed "s/\"$1\" *: *//"
}

# --- Load config ---
if [ ! -f "$CONF" ]; then
    log err "Config not found: $CONF"
    exit 1
fi

. "$CONF"

for var in SOUNDTOUCH_URL SOUNDTOUCH_USER SOUNDTOUCH_PASS; do
    if [ -z "${!var:-}" ]; then
        log err "Missing $var in $CONF"
        exit 1
    fi
done

log info "Config loaded (server=${SOUNDTOUCH_URL})"

# --- Wait for ZeroConf endpoint (port 8200) ---
log info "Waiting for ZeroConf endpoint (max ${MAX_WAIT}s)..."
waited=0
while true; do
    if curl -sf --max-time 2 "${ZC_URL}?action=getInfo" >/dev/null 2>&1; then
        break
    fi
    waited=$((waited + RETRY_DELAY))
    if [ $waited -ge $MAX_WAIT ]; then
        log err "ZeroConf endpoint not available after ${MAX_WAIT}s — giving up"
        exit 1
    fi
    sleep $RETRY_DELAY
done
log info "ZeroConf endpoint is up (waited ${waited}s)"

# --- Check if already primed ---
info=$(curl -sf --max-time 5 "${ZC_URL}?action=getInfo" 2>/dev/null)
active_user=$(echo "$info" | json_str activeUser)
device_name=$(echo "$info" | json_str remoteName)

if [ -n "$active_user" ]; then
    log info "Already primed (device=$device_name, activeUser=$active_user) — nothing to do"
    exit 0
fi

log info "Speaker '$device_name' has no active Spotify user — priming..."

# --- Get token from soundtouch-service server ---
log info "Requesting Spotify token from soundtouch-service..."
token_response=$(curl -sf --max-time 15 \
    -u "${SOUNDTOUCH_USER}:${SOUNDTOUCH_PASS}" \
    "${SOUNDTOUCH_URL}/mgmt/spotify/token" \
    2>&1)

if [ $? -ne 0 ] || [ -z "$token_response" ]; then
    log err "Failed to get token from soundtouch-service (is the server reachable?)"
    exit 1
fi

access_token=$(echo "$token_response" | json_str accessToken)
user=$(echo "$token_response" | json_str username)

if [ -z "$access_token" ] || [ -z "$user" ]; then
    error_msg=$(echo "$token_response" | json_str detail)
    log err "soundtouch-service returned error: ${error_msg:-no token/username in response}"
    exit 1
fi

log info "Got token for user $user (${access_token:0:10}...)"

# --- Prime the speaker ---
result=$(curl -sf --max-time 10 -X POST "$ZC_URL" \
    -H "Content-Type: application/x-www-form-urlencoded" \
    -d "action=addUser&userName=${user}&blob=${access_token}&clientKey=&tokenType=accesstoken" \
    2>&1)

status=$(echo "$result" | json_num status)
status_str=$(echo "$result" | json_str statusString)

if [ "$status" != "101" ]; then
    log err "addUser failed: status=$status ($status_str)"
    exit 1
fi

# --- Verify (retry — speaker needs a few seconds after cold boot) ---
log info "addUser accepted (status 101) — verifying..."
for i in 1 2 3 4 5; do
    sleep $((i * 2))
    active_user=$(curl -sf --max-time 5 "${ZC_URL}?action=getInfo" | json_str activeUser)
    if [ -n "$active_user" ]; then
        log info "Speaker primed successfully (activeUser=$active_user)"
        exit 0
    fi
done

log warning "Speaker accepted addUser but activeUser still empty after 30s"
exit 1
