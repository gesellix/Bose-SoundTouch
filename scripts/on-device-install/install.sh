#!/bin/bash
set -eo pipefail

# Version to install. Left empty by default so the canonical one-liner
#   curl -sSL .../install.sh | sh
# resolves and installs the latest release automatically (see below).
#
# Pin a specific version via environment variable or the --version/-v flag.
# The env var goes on `sh`, not `curl`: in a pipe, each command is its own
# process, so `VERSION=X curl ... | sh` silently does NOT set it for `sh`.
#   curl -sSL .../install.sh | VERSION=0.123.0 sh
#   curl -sSL .../install.sh | sh -s -- --version 0.123.0
VERSION=${VERSION:-}

# Parse optional command-line arguments so the script can be invoked as:
#   install.sh --version 0.123.0
#   install.sh -v 0.123.0
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
FALLBACK_VERSION=${FALLBACK_VERSION:-0.123.0}

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

# Prune any *.backup/*.old/*.new artefacts left behind by an earlier install
# attempt, before doing anything else that needs disk space. /mnt/nv is small
# (tens of MB), and if a previous run died between creating its backup and
# reaching the GC step below (e.g. "no space left on device" during the
# download that follows), that backup would otherwise never get cleaned up --
# and low free space is exactly what makes the next attempt likely to die the
# same way. Pruning up front makes cleanup idempotent regardless of where a
# prior run was interrupted.
echo "Disk usage before pre-install GC:"; df -h "$INSTALL_DIR"
for f in "$INSTALL_DIR/aftertouch-service".*.backup \
          "$INSTALL_DIR/aftertouch-service".*.backup.gz \
          "$INSTALL_DIR/aftertouch-service".*.old \
          "$INSTALL_DIR/aftertouch-service.new"; do
  [ -f "$f" ] || continue
  rm -f "$f"
  echo "Removed stale artefact: $f"
done
echo "Disk usage after pre-install GC:"; df -h "$INSTALL_DIR"

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
  # Binaries are tens of MB and only growing (see #614 investigation into
  # Go 1.27's default binary-size increase), while /mnt/nv is small (tens of
  # MB total). Stream straight into the compressed file rather than cp-then-
  # gzip: at this point in the script the old binary is still live AND the
  # newly-downloaded one is already sitting in $UPDATE_TMP_DIR, so an
  # intermediate uncompressed backup copy would briefly need all three full
  # copies on disk at once -- exactly the kind of moment that has already
  # caused "no space left on device" failures here. Best effort: if gzip is
  # missing, or the stream fails partway (e.g. disk fills mid-compress),
  # fall back to a plain uncompressed copy exactly as before.
  BACKUP_FILE="$INSTALL_DIR/aftertouch-service.${current_version}.backup"
  if command -v gzip >/dev/null 2>&1 \
      && gzip -c < "$INSTALL_DIR/aftertouch-service" > "$BACKUP_FILE.gz"; then
    BACKUP_FILE="$BACKUP_FILE.gz"
  else
    rm -f "$BACKUP_FILE.gz"
    cp -p "$INSTALL_DIR/aftertouch-service" "$BACKUP_FILE"
  fi
  echo "Backed up current binary ($current_version) → $BACKUP_FILE"
fi

mv "$UPDATE_TMP_DIR/binary" "$INSTALL_DIR/aftertouch-service"
chmod +x "$INSTALL_DIR/aftertouch-service"

# Keep only the backup we just created; prune all older *.backup, *.old, and
# *.new artefacts left by earlier installs. This is a second, defensive pass:
# it only matters if something wrote a stray artefact between the pre-install
# GC above and here (e.g. a concurrent install run).
if [ -n "$BACKUP_FILE" ]; then
  echo "Disk usage before post-install GC:"; df -h "$INSTALL_DIR"
  for f in "$INSTALL_DIR/aftertouch-service".*.backup \
            "$INSTALL_DIR/aftertouch-service".*.backup.gz \
            "$INSTALL_DIR/aftertouch-service".*.old \
            "$INSTALL_DIR/aftertouch-service.new"; do
    [ -f "$f" ] || continue
    [ "$f" = "$BACKUP_FILE" ] && continue
    rm -f "$f"
    echo "Removed stale artefact: $f"
  done
  echo "Disk usage after post-install GC:"; df -h "$INSTALL_DIR"
fi

# Settings file sourced by the init script. Written before the service is
# (re)started so the very first start already sees it.
#
# An existing file is left alone on upgrade -- it may carry the operator's own
# choices -- unless AFTERTOUCH_LAN_PORT was passed to this script explicitly.
CONF_FILE="$INSTALL_DIR/aftertouch.conf"
if [ -n "${AFTERTOUCH_LAN_PORT:-}" ] || [ ! -f "$CONF_FILE" ]; then
  cat > "$CONF_FILE" <<CONFEOF
# AfterTouch on-device settings. Sourced by /etc/init.d/aftertouch, which
# exports every assignment here into the daemon's own environment -- so any
# env var soundtouch-service reads (see docs: guides/SOUNDTOUCH-SERVICE.md,
# "Configuration Options") can be set by adding a line below and running
# \`/etc/init.d/aftertouch restart\`, e.g.:
#   MGMT_USERNAME=admin
#   MGMT_PASSWORD=change-me
#
# AFTERTOUCH_LAN_PORT: how AfterTouch is reached from other machines.
#   auto   (default) redirect a spare Bose port to AfterTouch, but only on
#          speakers whose Wi-Fi co-processor refuses to pass :8000 through.
#   none   never redirect; use an SSH tunnel instead.
#   <port> always redirect this inbound port to AfterTouch.
# See docs: reference/MODEL-SUPPORT-MATRIX.md
AFTERTOUCH_LAN_PORT=${AFTERTOUCH_LAN_PORT:-auto}
CONFEOF
  echo "Wrote settings to $CONF_FILE (AFTERTOUCH_LAN_PORT=${AFTERTOUCH_LAN_PORT:-auto})"
else
  echo "Keeping existing settings in $CONF_FILE"
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

echo "Installation complete. (Re)starting the service..."
# Use `restart`, not `start`: if AfterTouch is already running (the normal
# case for an in-place upgrade or downgrade), `start` calls start-stop-daemon
# with a pidfile that still points at a live PID. start-stop-daemon then
# refuses to launch a second instance and exits non-zero -- but this script
# has no `set -e` here and never checked that exit status, so the old
# process kept running untouched while the new binary sat unused on disk.
# The post-install curl check below couldn't catch it either, since the old
# process kept answering on :8000 throughout. `restart` stops the old
# process first (a no-op if nothing was running yet, e.g. on a fresh
# install), guaranteeing the newly-installed binary is the one that starts.
/etc/init.d/aftertouch restart

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
  # We are running ON the speaker, so print the address people actually need
  # rather than a <your-device-ip> placeholder they have to resolve themselves.
  LAN_IP=$(ip -4 addr show scope global 2>/dev/null \
    | awk '/inet /{sub(/\/.*/,"",$2); print $2; exit}')
  [ -n "$LAN_IP" ] || LAN_IP="<your-device-ip>"

  # If the init script installed a LAN entry-port redirect, that port -- not
  # 8000 -- is the one reachable from other machines.
  LAN_PORT=$(iptables -t nat -S PREROUTING 2>/dev/null \
    | grep -- '-j REDIRECT' \
    | sed -n 's/.*--dport \([0-9][0-9]*\).*--to-ports 8000.*/\1/p' \
    | head -1)

  echo ""
  echo "Installation complete. AfterTouch $VERSION is now running on your device."
  echo ""
  if [ -n "$LAN_PORT" ]; then
    echo "  Open  http://$LAN_IP:$LAN_PORT  from any machine on your network."
    echo ""
    echo "  (This speaker's Wi-Fi co-processor does not pass port 8000 through to"
    echo "   AfterTouch, so port $LAN_PORT is redirected to it instead. Set"
    echo "   AFTERTOUCH_LAN_PORT in $CONF_FILE to change or disable this.)"
  else
    echo "  Open  http://$LAN_IP:8000  from any machine on your network."
  fi
  echo ""
  echo "If that doesn't load, reach it through an SSH tunnel instead:"
  echo "  ssh -oHostKeyAlgorithms=+ssh-rsa -L 8000:localhost:8000 root@$LAN_IP"
  echo "then open http://localhost:8000"
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
