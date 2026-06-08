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

Some firmware images only bind the AfterTouch HTTP port to loopback (see issue #196). The workaround is an SSH tunnel — your machine talks to its own local `:8000`, the SSH connection forwards to the speaker's `:8000` on loopback.

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
# 1. Environment variable (works when piping into sh)
VERSION=0.111.3 rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh

# 2. Command-line flag (pass args after `sh -s --`)
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh -s -- --version 0.111.3

# 3. Download first, then run with a flag
curl -sSLo install.sh https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh
sh install.sh --version 0.111.3
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
