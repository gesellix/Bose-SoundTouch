#!/bin/bash
set -eo pipefail

VERSION=${VERSION:-0.79.0}
GH_REPO=${GH_REPO:-gesellix/Bose-SoundTouch}
BINARY_URL=${BINARY_URL:-https://github.com/$GH_REPO/releases/download/v$VERSION/soundtouch-service-v$VERSION-linux-armv7}
INIT_SCRIPT_URL=${INIT_SCRIPT_URL:-https://raw.githubusercontent.com/$GH_REPO/v$VERSION/scripts/on-device-install/aftertouch}
UPDATE_TMP_DIR=${UPDATE_TMP_DIR:-/media/aftertouch}


rm -rf "$UPDATE_TMP_DIR" || true
mkdir -p "$UPDATE_TMP_DIR"

echo "Installing Aftertouch $VERSION ..."
mkdir -p /opt/aftertouch
curl \
  -sSL \
  -o "$UPDATE_TMP_DIR/binary" \
  --fail \
  "$BINARY_URL"

mv "$UPDATE_TMP_DIR/binary" /opt/aftertouch/aftertouch-service
chmod +x /opt/aftertouch/aftertouch-service

echo "Creating init script..."
curl \
  -sSL \
  -o "$UPDATE_TMP_DIR/init-script" \
  --fail \
  "$INIT_SCRIPT_URL"

mv "$UPDATE_TMP_DIR/init-script" /etc/init.d/aftertouch
chmod +x /etc/init.d/aftertouch
update-rc.d aftertouch defaults

echo "Installation complete. Running initial startup to accelerate future startups..."
/etc/init.d/aftertouch start

/etc/init.d/aftertouch status

echo "Installation complete. Aftertouch $VERSION is now running on your device."
echo "You can try to connect to at http://<your-device-ip>:8000 ."
echo "If the connection fails, reconnect ssh with port forwarding like:"
echo "ssh -L 8000:localhost:8000 root@<IP_ADDRESS_OF_SPEAKER>"
