# On-Device Installer

Allows to run AfterTouch on SoundTouch devices directly, eliminating the need to run and maintain a separate server on the local network.

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

After the installation check if you can access AfterTouch from your local device by navigating to `http://<IP_ADDRESS_OF_SPEAKER>:8000`. If you can access the AfterTouch UI, you're good to go! If not, you may need to run AfterTouch on the speaker via SSH port forwarding.

```bash
ssh -L 8000:localhost:8000 root@<IP_ADDRESS_OF_SPEAKER>
```

## Updating AfterTouch

To update AfterTouch, simply run the installation command again. The installer will check if there's a new version available and update it if necessary.

## Uninstallation

Before uninstall, you might want to revert the migration, especially the changes to the server URLs (even though having configured an unresponsive local server probably is about as bad as having configured unresponsive Bose servers). To uninstall AfterTouch, run the following command on the speaker.

```bash
curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/uninstall.sh | sh
```