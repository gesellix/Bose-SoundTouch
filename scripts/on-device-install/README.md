# On-Device Installer

Allows to run AfterTouch on SoundTouch devices directly, eliminating the need to run and maintain a separate server on the local network.

For a complete step-by-step walkthrough — from first SSH connection through verified radio preset playback — see
[docs/guides/ON-DEVICE-INSTALL-WALKTHROUGH.md](../../docs/content/docs/guides/ON-DEVICE-INSTALL-WALKTHROUGH.md).

## Disclaimer

### Invasiveness

AfterTouch usually normally migrates the SoundTouch devices very noninvasive, by changing the configuration of the device. Running AfterTouch on the device itself is slightly more invasive, because it needs to create a script that starts AfterTouch on boot.

### AfterTouch Availability

Some devices will expose the AfterTouch port, some won't. We currently (May 2026) suspect that the newer generation devices (those with Bluetooth) will expose the port, while the older ones won't. We're still investigating how to expose AfterTouch on all devices.

If your device doesn't expose the port, you can still use the on-device installer, but you'll need to run AfterTouch on each one of your speakers individually and may only access AfterTouch via ssh port forwarding. This will also make OAuth authentication a little more tricky, but should also work via SSH port forwarding.

### Space Limitation

The storage space on the SoundTouch devices is very limited — stock rootfs typically has only a few MB free (e.g. ~4 MB on the ST20, see issue #268), well below the AfterTouch binary's ~12 MB. To work around this, the installer puts everything on `/mnt/nv/aftertouch` by default (the persistent partition, typically ~30 MB free) and points `/opt/aftertouch` at it via a symlink so the init script and runtime paths stay unchanged. Override the install target with `INSTALL_DIR=/some/path` if you've got room elsewhere.

The space limitation also means we are currently unsure on how to update the system, because two binaries are already too large. We are currently working on this - both by checking how we can make the binaries smaller, but also on how we can extend the storage space (e.g. by running AfterTouch from a USB drive).

### Logs

The daemon writes to BusyBox syslog (tagged `aftertouch`) rather than to a file. Disk usage stays bounded — the syslog ring buffer is in memory — and the same `logread` recipe used elsewhere in this project works:

```sh
logread        | grep aftertouch | tail -20   # recent entries
logread -f     | grep aftertouch              # live tail
```

If the install command reports "running but :8000 not responding" or `aftertouch status` reports the listener is down, the syslog tail is the first place to look.

## Installation

Enable SSH on your SoundTouch device using the usual "Stick with remote_services" method. Connect with the following command.

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa root@<IP_ADDRESS_OF_SPEAKER>
```

Then, run the following command to install AfterTouch on the device.

```bash
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh
```

This installs the **latest release** by default (the script resolves it from
GitHub's `releases/latest` redirect). To pin a specific version, see
[Updating AfterTouch](#updating-aftertouch) below.

After the installation check if you can access AfterTouch from your local device by navigating to `http://<IP_ADDRESS_OF_SPEAKER>:8000`. If you can access the AfterTouch UI, you're good to go!

### If `http://<IP_ADDRESS_OF_SPEAKER>:8000` fails: SSH port forwarding

On some device models AfterTouch's port is reachable from other machines on
your LAN out of the box. On others (see issue #196) it isn't, and (unlike
the phrasing this README used to have) that's not AfterTouch or its
firewall configuration choosing to bind loopback-only. AfterTouch itself
binds `0.0.0.0` (all interfaces) correctly, confirmed by inspecting the
running device directly, and there's no firewall rule (`iptables`,
`nftables`, or otherwise) blocking it either.

**Current knowledge (2026-08-16), confirmed on real hardware via a
decrypted firmware backup plus simultaneous packet captures on both the
speaker and a client machine:** some SoundTouch models built around a
"combo" WiFi/Bluetooth co-processor (used for AirPlay) route LAN traffic
through that co-processor before it reaches the main application
processor where AfterTouch actually runs. That co-processor only relays a
fixed set of the device's own original service ports (the same ones the
stock SoundTouch app and companion services always used), a list that,
as far as we can tell, is compiled into the co-processor's own firmware.
AfterTouch's ports were never part of that original design, so they never
got included. This isn't a bug in AfterTouch, a router/firewall setting,
or WiFi client isolation; all three were separately ruled out.

**The installer works around this automatically.** On an affected speaker
it redirects one of the ports the co-processor *does* relay to AfterTouch,
so the UI is reachable from the LAN without any tunnel:

```
http://<IP_ADDRESS_OF_SPEAKER>:17008
```

Port `17008` is Bose's software-update listener; that cloud service no
longer exists, so taking over its inbound traffic costs nothing. Only
traffic from other machines is affected; anything running on the speaker
still reaches AfterTouch on `:8000` as before. Change or disable this with
`AFTERTOUCH_LAN_PORT` (`auto` / `none` / a port number) in
`/opt/aftertouch/aftertouch.conf`, or pass it at install time:

```bash
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | AFTERTOUCH_LAN_PORT=none sh
```

**`aftertouch.conf` isn't limited to `AFTERTOUCH_LAN_PORT`.** The init
script exports every assignment in this file into the daemon's own
environment, so any env var `soundtouch-service` reads (see the
[configuration table](../../docs/content/docs/guides/SOUNDTOUCH-SERVICE.md#configuration-options))
can be set the same way — for example, to change the admin credentials:

```
MGMT_USERNAME=admin
MGMT_PASSWORD=change-me
```

Edit `/opt/aftertouch/aftertouch.conf` over SSH, then
`/etc/init.d/aftertouch restart` to apply. `DEPLOYMENT_MODE=on-device` is
already set by the init script itself — it never needs to be added here.
The auto-export behavior described here needs a build including the fix
for issue #546; older installs (before `aftertouch.conf` even existed, or
between then and that fix) need to reinstall/update first.

Which models need this, and how to report one that isn't listed yet, is
tracked in
[MODEL-SUPPORT-MATRIX.md](../../docs/content/docs/reference/MODEL-SUPPORT-MATRIX.md).
The SSH tunnel below still works, and remains the better route for
**linking music-service accounts**: Spotify only accepts `https://` or
loopback OAuth redirect URIs, so `http://localhost:8000` through a tunnel
succeeds where a plain LAN address is rejected.

**Open a fresh terminal on your own machine** (Linux/macOS/Windows — NOT another shell inside the speaker's SSH session — see issue #250 for the trap that catches everyone here) and run:

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa -L 8000:localhost:8000 root@<IP_ADDRESS_OF_SPEAKER>
```

The `-oHostKeyAlgorithms=+ssh-rsa` flag is required: SoundTouch speakers offer only legacy SSH host-key algorithms (`ssh-rsa`, `ssh-dss`) that modern OpenSSH clients refuse by default. Without it you'll see `Unable to negotiate with <ip> port 22: no matching host key type found`.

Leave that terminal open while you use AfterTouch. With the tunnel up, navigate to **`http://localhost:8000`** in your browser (`localhost`, not the speaker's IP).

### If the tunnel is open but `http://localhost:8000` still fails

You should see `ERR_CONNECTION_RESET` in the browser and `channel N: open failed: connect failed: Connection refused` in the SSH terminal — that means the tunnel itself works, but the AfterTouch daemon isn't listening on the speaker. Inside the SSH session, check:

```bash
netstat -tlnp 2>/dev/null | grep 8000     # is anything listening?
ps | grep -i aftertouch                   # is the daemon running at all?
logread | grep aftertouch | tail -20      # recent daemon output (panics, errors)
```

If the daemon isn't running, restart it:

```bash
/etc/init.d/aftertouch start
/etc/init.d/aftertouch status
```

The init script's `status` now distinguishes "running with listener up" from "PID alive but listener silently died" — if you get the latter, the syslog tail above will tell you why.

## Updating AfterTouch

Run the installer again with the version you want to install. The script backs up the currently-running binary (named after its version), installs the new one, and prunes older leftover artefacts to keep `/mnt/nv` free.

**Install (or upgrade to) a specific version** — three equivalent ways:

```bash
# 1. Environment variable — goes on `sh`, not `curl`: in a pipe, each
#    command is a separate process, so `VERSION=X curl ... | sh` silently
#    does NOT set it for `sh` (the one that actually reads $VERSION).
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | VERSION=0.123.0 sh

# 2. Command-line flag (pass args after `sh -s --`)
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh -s -- --version 0.123.0

# 3. Download first, then run with a flag
curl -sSLo install.sh https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh
sh install.sh --version 0.123.0
```

Running **without** a version override installs the latest release: the script
follows GitHub's `https://github.com/gesellix/Bose-SoundTouch/releases/latest`
redirect to discover the newest tag. If that lookup fails (offline, or a `curl`
build without `-w` support), it falls back to a pinned version baked into the
script.

> **Tip — rollback:** if the new binary misbehaves, the installer left a `.backup` file alongside it:
> ```bash
> ls /mnt/nv/aftertouch/aftertouch-service*.backup
> cp /mnt/nv/aftertouch/aftertouch-service.<old-version>.backup \
>    /mnt/nv/aftertouch/aftertouch-service
> /etc/init.d/aftertouch restart
> ```

## Uninstallation

Before uninstall, you might want to revert the migration, especially the changes to the server URLs (even though having configured an unresponsive local server probably is about as bad as having configured unresponsive Bose servers). To uninstall AfterTouch, run the following command on the speaker.

```bash
curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/uninstall.sh | sh
```
