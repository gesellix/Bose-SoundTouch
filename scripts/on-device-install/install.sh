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
# AfterTouch binary is ~15.5 MB as of v0.130.0 and grows by roughly
# 1 MB per Go toolchain bump. /mnt/nv is ~31 MB in total, of which
# ~20 MB is free with AfterTouch installed, and it persists across
# reboots the same way /opt would.
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

# --- Preflight disk-space check ------------------------------------------
# /mnt/nv is small (tens of MB) and binaries keep growing (Go 1.27 alone
# added ~640KB to this binary via its own new stdlib defaults, unrelated to
# this project's code). A prior attempt on real hardware ran out of space
# mid-replace and left a truncated, non-executable binary in place: UBIFS is
# a log-structured flash filesystem, so space freed by overwriting the old
# binary isn't necessarily reusable by the time the new one needs to land.
# Check upfront, with a safety margin, instead of discovering this mid-write.
#
# The new binary's size comes from a HEAD request rather than a hardcoded
# threshold, so this doesn't go stale as binaries grow across releases.
NEW_BINARY_BYTES=$(curl -sSLI --fail "$BINARY_URL" 2>/dev/null \
  | tr -d '\r' \
  | awk 'tolower($1) == "content-length:" {v=$2} END {print v}') || true

AVAILABLE_KB=$(df -Pk "$INSTALL_DIR" | awk 'NR==2 {print $4}')

CURRENT_BINARY_KB=0
if [ -f "$INSTALL_DIR/aftertouch-service" ]; then
  CURRENT_BINARY_KB=$(du -k "$INSTALL_DIR/aftertouch-service" | awk '{print $1}')
fi

# Flat margin, not a percentage: covers UBIFS's own reserved/GC headroom on
# this log-structured flash filesystem plus general slack.
SAFETY_MARGIN_KB=5120 # 5 MB

SKIP_BACKUP=no

if [ -n "$NEW_BINARY_BYTES" ]; then
  NEW_BINARY_KB=$((NEW_BINARY_BYTES / 1024))

  # What an upgrade costs is the DIFFERENCE between the new binary and the one
  # it replaces, not the new binary's full size: the new file is written over
  # the existing path (the `mv` further down), so the old file's blocks are
  # released by the same operation that consumes new ones. Charging for the
  # full size counts the installed binary twice and aborts upgrades on devices
  # that have ample room -- 72KB short on a 31.6MB /mnt/nv in
  # https://github.com/gesellix/Bose-SoundTouch/issues/693, where the speaker
  # was replacing a 14.1MB binary with a 15.5MB one.
  #
  # A fresh install still needs the full size, which is exactly what
  # CURRENT_BINARY_KB=0 yields here. A downgrade frees space rather than
  # consuming it, so the difference is floored at zero.
  #
  # The difference is also the only figure that stays honest on a compressing
  # filesystem: `df` reports what UBIFS actually stores, while both sizes here
  # are logical (HEAD Content-Length and `du`, which reports the uncompressed
  # size on UBIFS). Old and new compress at the same ratio, so the unknown
  # ratio cancels out of the difference -- it does not cancel out of a
  # full-size comparison.
  REPLACE_COST_KB=$((NEW_BINARY_KB - CURRENT_BINARY_KB))
  if [ "$REPLACE_COST_KB" -lt 0 ]; then
    REPLACE_COST_KB=0
  fi

  # Deliberately conservative: measured on an ST20 (2026-09-12), a 15.5MB
  # armv7 binary gzipped to 5.9MB, i.e. 38%, and the v0.131.0 -> v0.132.0
  # upgrade kept its backup with room to spare. 70% is kept as the estimate
  # because the real ratio is unknown until compression actually runs, and a
  # future binary carrying less compressible content (embedded assets, an
  # already-compressed payload) moves it up rather than down. Over-estimating
  # only costs an unnecessary "continue without a backup?" prompt on a volume
  # tighter than any seen so far; under-estimating promises space that is not
  # there, which is what this whole check exists to prevent.
  #
  # Unlike the binary itself this is a genuine addition to what is stored, so
  # it is charged in full.
  BACKUP_ESTIMATE_KB=$((CURRENT_BINARY_KB * 7 / 10))

  NEEDED_WITH_BACKUP_KB=$((REPLACE_COST_KB + BACKUP_ESTIMATE_KB + SAFETY_MARGIN_KB))
  NEEDED_NO_BACKUP_KB=$((REPLACE_COST_KB + SAFETY_MARGIN_KB))

  if [ "$AVAILABLE_KB" -ge "$NEEDED_WITH_BACKUP_KB" ]; then
    : # plenty of room; proceed normally, with a backup
  elif [ "$AVAILABLE_KB" -ge "$NEEDED_NO_BACKUP_KB" ]; then
    echo "WARNING: not enough free space on $INSTALL_DIR to keep a rollback" >&2
    echo "backup this time (${AVAILABLE_KB}KB available; ~${NEEDED_WITH_BACKUP_KB}KB" >&2
    echo "wanted with a backup, ~${NEEDED_NO_BACKUP_KB}KB without one)." >&2
    echo "Continuing will replace the current binary with NO way to" >&2
    echo "automatically undo it if something goes wrong." >&2
    if [ -n "${AFTERTOUCH_FORCE_NO_BACKUP:-}" ]; then
      echo "Proceeding without a backup (AFTERTOUCH_FORCE_NO_BACKUP is set)." >&2
      SKIP_BACKUP=yes
    elif [ -r /dev/tty ] && [ -w /dev/tty ]; then
      printf 'Continue without a backup? [y/N] ' > /dev/tty
      REPLY=""
      read -r REPLY < /dev/tty || true
      case "$REPLY" in
        [Yy]*) SKIP_BACKUP=yes ;;
        *)
          echo "Aborting: refusing to proceed without a backup. Free up space" >&2
          echo "on $INSTALL_DIR and try again, or set AFTERTOUCH_FORCE_NO_BACKUP=yes" >&2
          echo "to proceed without one non-interactively." >&2
          exit 1
          ;;
      esac
    else
      echo "No interactive terminal available to confirm; aborting." >&2
      echo "Set AFTERTOUCH_FORCE_NO_BACKUP=yes to proceed without a backup" >&2
      echo "non-interactively." >&2
      exit 1
    fi
  else
    echo "ERROR: not enough free space on $INSTALL_DIR to install AfterTouch" >&2
    echo "$VERSION safely (${AVAILABLE_KB}KB available, ~${NEEDED_NO_BACKUP_KB}KB" >&2
    echo "needed for a ${NEW_BINARY_KB}KB binary replacing a ${CURRENT_BINARY_KB}KB one)." >&2
    echo "" >&2
    echo "The installer already pruned its own leftovers, so there may be" >&2
    echo "nothing left for you to delete. Options:" >&2
    echo "  * install an older, smaller release, e.g.:" >&2
    echo "      curl -sSL https://raw.githubusercontent.com/$GH_REPO/main/scripts/on-device-install/install.sh | sh -s -- --version $FALLBACK_VERSION" >&2
    echo "  * free space elsewhere on $INSTALL_DIR (ls -lh $INSTALL_DIR)" >&2
    echo "  * see docs: guides/ON-DEVICE-INSTALL-WALKTHROUGH.md, Troubleshooting" >&2
    exit 1
  fi
else
  echo "WARNING: could not determine the new binary's size ahead of time" >&2
  echo "(HEAD request to $BINARY_URL failed); skipping the preflight" >&2
  echo "disk-space check." >&2
fi

curl \
  -sSL \
  -o "$UPDATE_TMP_DIR/binary" \
  --fail \
  "$BINARY_URL"

# Verify the download against the checksum published alongside it before it is
# allowed anywhere near the installed binary. `curl --fail` already catches
# HTTP errors, and a body short of its Content-Length, so this is not about
# truncation: it is about the download being intact but wrong (a corrupted
# proxy cache, a tampered mirror).
#
# Best effort by design: releases before the checksum assets existed, a
# firmware without sha256sum, or an offline mirror serving only the binary all
# skip the check with a warning rather than blocking an install that would
# otherwise have worked. A checksum that is present and does NOT match is
# always fatal.
CHECKSUM_URL=${CHECKSUM_URL:-$BINARY_URL.sha256}
if ! command -v sha256sum >/dev/null 2>&1; then
  echo "NOTE: sha256sum is not available; skipping checksum verification." >&2
elif ! EXPECTED_SHA=$(curl -sSL --fail "$CHECKSUM_URL" 2>/dev/null | awk 'NR==1 {print $1}') \
    || [ -z "$EXPECTED_SHA" ]; then
  echo "NOTE: no checksum published at $CHECKSUM_URL; skipping verification." >&2
else
  ACTUAL_SHA=$(sha256sum < "$UPDATE_TMP_DIR/binary" | awk '{print $1}')
  if [ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]; then
    echo "ERROR: the downloaded binary does not match its published checksum." >&2
    echo "  expected: $EXPECTED_SHA" >&2
    echo "  actual:   $ACTUAL_SHA" >&2
    echo "Nothing was installed. Retry; if it keeps failing, download the" >&2
    echo "binary on another machine and copy it over instead." >&2
    rm -f "$UPDATE_TMP_DIR/binary"
    exit 1
  fi
  echo "Checksum verified (sha256 $(echo "$ACTUAL_SHA" | cut -c1-8)...)."
fi

# Put the backed-up binary back in place. Used on every path that can leave a
# broken or incomplete binary installed: a failed write (disk full mid-copy is
# the documented failure mode on this filesystem) and a service that does not
# answer after the install. Without this the installer exits leaving the
# speaker with no working AfterTouch and the rollback left to the operator --
# who, on a headless device reached over SSH, may not get a second chance.
#
# Returns non-zero when there is nothing to restore (fresh install, or the
# operator chose to proceed without a backup), so callers can say so.
restore_backup() {
  if [ -z "$BACKUP_FILE" ] || [ ! -f "$BACKUP_FILE" ]; then
    return 1
  fi

  echo "Restoring the previous binary from $BACKUP_FILE ..." >&2
  case "$BACKUP_FILE" in
    *.gz)
      if ! gunzip -c "$BACKUP_FILE" > "$INSTALL_DIR/aftertouch-service"; then
        echo "ERROR: could not restore from $BACKUP_FILE." >&2
        return 1
      fi
      ;;
    *)
      if ! cp -p "$BACKUP_FILE" "$INSTALL_DIR/aftertouch-service"; then
        echo "ERROR: could not restore from $BACKUP_FILE." >&2
        return 1
      fi
      ;;
  esac

  chmod +x "$INSTALL_DIR/aftertouch-service"
  return 0
}

# Back up the current binary before overwriting so a one-step rollback
# is always available.  The version string comes from the binary itself;
# if it is absent (very old build or corrupted) we fall back to a timestamp.
# Skipped entirely when the preflight check above decided (with the
# operator's explicit confirmation, or AFTERTOUCH_FORCE_NO_BACKUP) that
# there isn't room for one.
BACKUP_FILE=""
if [ -f "$INSTALL_DIR/aftertouch-service" ] && [ "$SKIP_BACKUP" != "yes" ]; then
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

# `set -e` would abort here on a failed write, leaving a truncated binary in
# place and the backup untouched on disk -- the exact state the preflight check
# above exists to prevent, but it cannot rule out (the filesystem may still
# refuse a write it was predicted to accept). Handle it instead of aborting.
if ! mv "$UPDATE_TMP_DIR/binary" "$INSTALL_DIR/aftertouch-service"; then
  echo "" >&2
  echo "ERROR: writing the new binary to $INSTALL_DIR failed (out of space?)." >&2
  if restore_backup; then
    echo "The previous binary is back in place; AfterTouch is unchanged." >&2
    /etc/init.d/aftertouch restart || true
  else
    echo "No rollback backup is available, so the installed binary may be" >&2
    echo "incomplete. Re-run this installer to replace it, optionally with" >&2
    echo "an older --version." >&2
  fi
  exit 1
fi
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
  logread 2>/dev/null | grep aftertouch | tail -20 >&2 || {
    echo "  (logread returned nothing for tag 'aftertouch': the daemon may" >&2
    echo "   have died before it could log anything, or syslogd is not" >&2
    echo "   running on this firmware. Try '/etc/init.d/aftertouch status'" >&2
    echo "   and 'logread | tail -50'.)" >&2
  }
  echo "" >&2
  echo "  For a live view of the daemon's output, run:" >&2
  echo "    logread -f | grep aftertouch" >&2

  # A new binary that does not answer is worse than the old one that did, and
  # the operator is typically on a single SSH session with no local console to
  # fall back to. Put the previous binary back and restart, so the speaker is
  # left in the state it was in before this install rather than in a broken
  # one. The install still reports failure -- this is a rollback, not a
  # success path.
  echo "" >&2
  if restore_backup; then
    /etc/init.d/aftertouch restart || true
    if curl -fsS --max-time 10 http://localhost:8000 >/dev/null 2>&1; then
      echo "  Rolled back to the previous binary, which is answering again." >&2
      echo "  AfterTouch $VERSION was NOT installed." >&2
    else
      echo "  Rolled back to the previous binary, but it is not answering" >&2
      echo "  either -- the problem is unlikely to be this release." >&2
    fi
  else
    echo "  No rollback backup was kept, so the new binary is still in place." >&2
  fi

  exit 1
fi
