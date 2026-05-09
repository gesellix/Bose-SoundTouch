#!/bin/bash
set -eo pipefail

VERSION=${VERSION:-0.71.2}
GH_REPO=${GH_REPO:-gesellix/Bose-SoundTouch}
BINARY_URL=${BINARY_URL:-https://github.com/$GH_REPO/releases/download/v$VERSION/soundtouch-service-v$VERSION-linux-armv7}
INIT_SCRIPT_URL=${INIT_SCRIPT_URL:-https://raw.githubusercontent.com/$GH_REPO/v$VERSION/scripts/on-device-install/aftertouch}

echo "Installing Aftertouch $VERSION for ARMv7..."
mkdir -p /opt/aftertouch
cd /opt/aftertouch
curl \
  -sSL \
  -O \
  "$BINARY_URL"

mv soundtouch-service-v$VERSION-linux-armv7 aftertouch-service
chmod +x aftertouch-service

echo "Creating init script..."
cd /etc/init.d
curl \
  -sSL \
  -O \
  "$INIT_SCRIPT_URL"

chmod +x aftertouch
update-rc.d aftertouch defaults

echo "Installation complete. Running initial startup to accelerate future startups..."
/etc/init.d/aftertouch start

/etc/init.d/aftertouch status

echo "Installation complete. Aftertouch $VERSION is now running on your device."
echo "You can try to connect to at http://<your-device-ip>:8000 ."
echo "If the connection fails, reconnect ssh with port forwarding like:"
echo "ssh -L 8000:localhost:8000 root@<IP_ADDRESS_OF_SPEAKER>"
