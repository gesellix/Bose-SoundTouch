#!/bin/bash
set -eo pipefail

# Version to install. Left empty by default so the canonical one-liner
#   curl -sSL .../install.sh | sh
# resolves and installs the latest release automatically (see below).
#
# Pin a specific version via environment variable or the --version/-v flag:
#   VERSION=0.111.3 curl -sSL .../install.sh | sh
#   curl -sSL .../install.sh | sh -s -- --version 0.111.3
VERSION=${VERSION:-}

# Parse optional command-line arguments so the script can be invoked as:
#   install.sh --version 0.111.3
#   install.sh -v 0.111.3
while [ $# -gt 0 ]; do
  case "$1" in
    --version|-v)
      if [ -z "$2" ]; then
        echo "ERROR: --version requires an argument." >&2; exit 1
      fi
      VERSION="$2"; shift 2;;
    --) shift; break;;
    *) echo "Unknown argument: $1" >&2; exit 1;;
  esac
done

GH_REPO=${GH_REPO:-gesellix/Bose-SoundTouch}

# Used only when the latest-release lookup fails (offline / rate-limited /
# a curl without -w support).
FALLBACK_VERSION=${FALLBACK_VERSION:-0.111.3}

# Resolve the latest release when no explicit version was provided, by
# following the stable redirect https://github.com/<repo>/releases/latest
# -> .../releases/tag/vX.Y.Z and taking the tag from the final URL. The
# leading "v" is stripped because the URLs below add it back (v$VERSION).
if [ -z "$VERSION" ]; then
  LATEST_URL="https://github.com/$GH_REPO/releases/latest"
  echo "Resolving latest release via $LATEST_URL ..."
  EFFECTIVE=$(curl -sSLI -o /dev/null -w '%{url_effective}' "$LATEST_URL" 2>/dev/null) || true
  TAG=${EFFECTIVE##*/}
  VER=${TAG#v}
  case "$VER" in
    [0-9]*.[0-9]*) VERSION="$VER" ;;
    *)
      VERSION="$FALLBACK_VERSION"
      echo "WARNING: could not resolve latest release; using fallback $VERSION" >&2
      ;;
  esac
fi

BINARY_URL=${BINARY_URL:-https://github.com/$GH_REPO/releases/download/v$VERSION/soundtouch-service-v$VERSION-linux-armv7}
INIT_SCRIPT_URL=${INIT_SCRIPT_URL:-https://raw.githubusercontent.com/$GH_REPO/v$VERSION/scripts/on-device-install/aftertouch}

# Default install location is /mnt/nv/aftertouch (the persistent
# partition), not /opt/aftertouch on rootfs. Stock SoundTouch rootfs
# has ~4 MB free on devices like the ST20 (issue #268); the
# AfterTouch binary is ~12 MB. /mnt/nv typically has tens of MB
# free and persists across reboots the same way /opt would.
#
# /opt/aftertouch becomes a symlink into the install target so the
# init script's hardcoded DAEMON path keeps working unchanged.
#
# Power users can override with INSTALL_DIR=/some/other/path.
INSTALL_DIR=${INSTALL_DIR:-/mnt/nv/aftertouch}

# Scratch directory for the download. /media is tmpfs on most
# SoundTouch firmware, fine for transient files but unrelated to
# the persistent install target.
UPDATE_TMP_DIR=${UPDATE_TMP_DIR:-/media/aftertouch}

rm -rf "$UPDATE_TMP_DIR" || true
mkdir -p "$UPDATE_TMP_DIR"

echo "Installing AfterTouch $VERSION to $INSTALL_DIR ..."
mkdir -p "$INSTALL_DIR"

# Wire /opt/aftertouch -> $INSTALL_DIR so the init script
# (DAEMON=/opt/aftertouch/aftertouch-service) finds the binary
# regardless of which target we picked. Replace any prior
# /opt/aftertouch (directory or stale symlink) before re-creating.
if [ "$INSTALL_DIR" != "/opt/aftertouch" ]; then
  rm -rf /opt/aftertouch
  ln -sf "$INSTALL_DIR" /opt/aftertouch
fi

curl \
  -sSL \
  -o "$UPDATE_TMP_DIR/binary" \
  --fail \
  "$BINARY_URL"

# Back up the current binary before overwriting so a one-step rollback
# is always available.  The version string comes from the binary itself;
# if it is absent (very old build or corrupted) we fall back to a timestamp.
BACKUP_FILE=""
if [ -f "$INSTALL_DIR/aftertouch-service" ]; then
  current_version=$("$INSTALL_DIR/aftertouch-service" --version 2>/dev/null \
    | awk '{print $NF}') || true
  if [ -z "$current_version" ] || [ "$current_version" = "dev" ]; then
    current_version=$(date +%Y%m%d-%H%M%S)
  fi
  BACKUP_FILE="$INSTALL_DIR/aftertouch-service.${current_version}.backup"
  cp -p "$INSTALL_DIR/aftertouch-service" "$BACKUP_FILE"
  echo "Backed up current binary ($current_version) → $BACKUP_FILE"
fi

mv "$UPDATE_TMP_DIR/binary" "$INSTALL_DIR/aftertouch-service"
chmod +x "$INSTALL_DIR/aftertouch-service"

# Keep only the backup we just created; prune all older *.backup, *.old, and
# *.new artefacts left by earlier installs.  /mnt/nv is small (tens of MB),
# so accumulation quickly causes "no space left on device" during downloads.
if [ -n "$BACKUP_FILE" ]; then
  echo "Disk usage before GC:"; df -h "$INSTALL_DIR"
  for f in "$INSTALL_DIR/aftertouch-service".*.backup \
            "$INSTALL_DIR/aftertouch-service".*.old \
            "$INSTALL_DIR/aftertouch-service.new"; do
    [ -f "$f" ] || continue
    [ "$f" = "$BACKUP_FILE" ] && continue
    rm -f "$f"
    echo "Removed stale artefact: $f"
  done
  echo "Disk usage after GC:"; df -h "$INSTALL_DIR"
fi

echo "Creating init script..."
curl \
  -sSL \
  -o "$UPDATE_TMP_DIR/init-script" \
  --fail \
  "$INIT_SCRIPT_URL"

mv "$UPDATE_TMP_DIR/init-script" /etc/init.d/aftertouch
chmod +x /etc/init.d/aftertouch
update-rc.d aftertouch defaults

echo "Installation complete. Running initial startup..."
/etc/init.d/aftertouch start

/etc/init.d/aftertouch status

# Post-install verification: the init script's own poll loop only
# checks that the daemon registered a PID file; that's not enough
# evidence the listener is actually serving HTTP. Issue #250 shipped
# with a "running but unreachable" state where status was green and
# `curl :8000` got connection-refused. Re-check directly here and
# surface the recent syslog if it fails — the init script pipes the
# daemon's stdout/stderr through `logger -t aftertouch`, so panics
# land in busybox syslog and `logread` reads them out.
if curl -fsS --max-time 10 http://localhost:8000 >/dev/null 2>&1; then
  echo "Installation complete. AfterTouch $VERSION is now running on your device."
  echo "Connect to http://<your-device-ip>:8000 from another machine on the LAN."
  echo "If the device doesn't expose :8000 directly, port-forward via SSH:"
  echo "  ssh -L 8000:localhost:8000 root@<IP_ADDRESS_OF_SPEAKER>"
else
  echo "WARNING: the init script reports AfterTouch as running, but" >&2
  echo "  http://localhost:8000 isn't responding. The daemon may have" >&2
  echo "  panicked shortly after start. Recent aftertouch syslog:" >&2
  echo "" >&2
  logread 2>/dev/null | grep aftertouch | tail -20 >&2 || \
    echo "  (logread returned nothing for tag 'aftertouch'; the daemon" >&2
  echo "" >&2
  echo "  For a live view of the daemon's output, run:" >&2
  echo "    logread -f | grep aftertouch" >&2
  exit 1
fi
