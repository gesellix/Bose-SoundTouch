#!/usr/bin/env bash
#
# spotify-prime-speaker — Prime a Bose SoundTouch speaker for Spotify playback
#
# Activates Spotify on a SoundTouch speaker by sending an access token
# via the Spotify Connect ZeroConf endpoint (port 8200). This is the
# same mechanism the Spotify desktop app uses internally.
#
# Works standalone — no soundtouch-service, ueberboese, or other server required.
#
# Requirements: curl, jq
#
# Usage:
#   ./spotify-prime-speaker SPEAKER_IP ACCESS_TOKEN
#
# Example:
#   ./spotify-prime-speaker 192.168.1.143 BQDj...your_token...
#
# How to get an access token:
#   - Spotify Developer Console: https://developer.spotify.com
#     (create an app, use the "Get Token" button)
#   - Via soundtouch-service management API: POST /mgmt/spotify/auth/init
#   - Via ueberboese management API: POST /mgmt/spotify/init
#   - Any Spotify OAuth Authorization Code flow with user-read-email scope
#
# Notes:
#   - Access tokens expire after 1 hour (3600 seconds)
#   - The speaker must be on the same network and reachable on port 8200
#   - After priming, Spotify presets on the speaker should work immediately
#   - Re-run after each speaker reboot (or use a server like soundtouch-service
#     to automate this)
#
# How it works:
#   The Bose SoundTouch speaker exposes a Spotify Connect ZeroConf API
#   on port 8200. By sending an addUser request with a valid Spotify
#   access token, the speaker activates its built-in Spotify Connect
#   client. No encryption is needed — the token is sent as plain text,
#   exactly like the Spotify desktop app does it.
#
# Related:
#   - https://github.com/gesellix/Bose-SoundTouch (comprehensive toolkit)
set -euo pipefail

# --- Argument parsing ---
if [ $# -lt 2 ]; then
  echo "Usage: $0 SPEAKER_IP ACCESS_TOKEN"
  echo ""
  echo "Prime a Bose SoundTouch speaker for Spotify playback."
  echo ""
  echo "Arguments:"
  echo "  SPEAKER_IP     IP address of the SoundTouch speaker"
  echo "  ACCESS_TOKEN   Spotify access token (starts with BQ...)"
  echo ""
  echo "Get a token at https://developer.spotify.com or via a server's OAuth flow."
  exit 1
fi

SPEAKER_IP="$1"
TOKEN="$2"
ZC_URL="http://${SPEAKER_IP}:8200/zc"

# --- Dependency check ---
for cmd in curl jq; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "Error: $cmd is required but not installed." >&2
    exit 1
  fi
done

# --- Step 1: Discover Spotify username from token ---
echo "Discovering Spotify user from token..."
ME_RESPONSE=$(curl -sf -H "Authorization: Bearer ${TOKEN}" \
  https://api.spotify.com/v1/me 2>&1) || {
  echo "Error: Failed to call Spotify /me API. Is the token valid?" >&2
  echo "  (tokens expire after 1 hour)" >&2
  exit 1
}

USER=$(echo "$ME_RESPONSE" | jq -r '.id // empty')
if [ -z "$USER" ]; then
  echo "Error: Could not extract user ID from Spotify response." >&2
  echo "$ME_RESPONSE" >&2
  exit 1
fi
echo "  Spotify user: $USER"

# --- Step 2: Check current speaker status ---
echo "Checking speaker at ${SPEAKER_IP}:8200..."
INFO=$(curl -sf "${ZC_URL}?action=getInfo" 2>&1) || {
  echo "Error: Could not reach speaker at ${SPEAKER_IP}:8200." >&2
  echo "  Is the speaker on and on the same network?" >&2
  exit 1
}

ACTIVE=$(echo "$INFO" | jq -r '.activeUser // empty')
DEVICE_NAME=$(echo "$INFO" | jq -r '.remoteName // empty')

if [ -n "$DEVICE_NAME" ]; then
  echo "  Speaker: $DEVICE_NAME"
fi

if [ -n "$ACTIVE" ]; then
  echo "  Already primed (activeUser=$ACTIVE)"
  echo "Done — speaker is ready for Spotify playback."
  exit 0
fi

echo "  No active Spotify user — priming now..."

# --- Step 3: Send addUser ---
RESULT=$(curl -sf -X POST "${ZC_URL}" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "action=addUser&userName=${USER}&blob=${TOKEN}&clientKey=&tokenType=accesstoken" \
  2>&1) || {
  echo "Error: addUser request failed." >&2
  exit 1
}

STATUS=$(echo "$RESULT" | jq -r '.status // -1')
STATUS_STR=$(echo "$RESULT" | jq -r '.statusString // empty')

if [ "$STATUS" != "101" ]; then
  echo "Error: Speaker returned status $STATUS ($STATUS_STR)" >&2
  echo "$RESULT" | jq . >&2
  exit 1
fi

echo "  Speaker accepted the token (status 101)."

# --- Step 4: Verify ---
echo "  Verifying (waiting 2 seconds)..."
sleep 2
ACTIVE=$(curl -sf "${ZC_URL}?action=getInfo" | jq -r '.activeUser // empty')

if [ -n "$ACTIVE" ]; then
  echo "Done — speaker primed for Spotify (activeUser=$ACTIVE)"
else
  echo "Warning: Speaker returned 101 but activeUser is still empty."
  echo "  The speaker may need more time. Try pressing a Spotify preset."
  exit 1
fi
