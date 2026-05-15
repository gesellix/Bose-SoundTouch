#!/bin/sh
# Uninstall AfterTouch on-device. Handles both the historical
# layout (/opt/aftertouch as a directory) and the post-#268 layout
# (/opt/aftertouch as a symlink into /mnt/nv/aftertouch).
set -eu

/etc/init.d/aftertouch stop || true
rm -f /etc/init.d/aftertouch
update-rc.d -f aftertouch remove

# If /opt/aftertouch is a symlink, resolve it and remove the target
# before unlinking, so we don't leave ~12 MB of orphan binary on
# /mnt/nv. Tolerate either layout — readlink -f returns the same
# path for a real directory, and rm -rf on a missing path with
# set -eu would abort.
target="$(readlink -f /opt/aftertouch 2>/dev/null || echo /opt/aftertouch)"
if [ -e "$target" ]; then
  rm -rf "$target"
fi
if [ -L /opt/aftertouch ] || [ -e /opt/aftertouch ]; then
  rm -rf /opt/aftertouch
fi
