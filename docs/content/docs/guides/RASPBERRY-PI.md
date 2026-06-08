---
title: "Raspberry Pi Installation Guide"
---
How to install and manage AfterTouch on a Raspberry Pi (or any always-on Linux
host) using the provided installer scripts.

Two scripts are available, one per binary:

| Script              | Binary               | Role                                | Default port |
|---------------------|----------------------|-------------------------------------|--------------|
| `install.sh`        | `soundtouch-service` | Cloud-replacement relay — always-on | 80 / 443     |
| `install-player.sh` | `soundtouch-player`  | Browser control panel               | 8080         |

Both auto-detect CPU architecture (armv7 / arm64 / amd64), create a `soundtouch`
system user, and install a systemd unit. They are safe to re-run for updates.
Each installer has a matching uninstaller (`uninstall.sh`, `uninstall-player.sh`).

> `install-web.sh` is the previous name for `install-player.sh`. It still works
> as a deprecated alias but will be removed in a future release. If you installed
> `soundtouch-web` before the rename, see
> [Migrating from soundtouch-web](#migrating-from-soundtouch-web).

For a complete install-through-migration walkthrough see
[EXTERNAL-HOST-WALKTHROUGH.md](EXTERNAL-HOST-WALKTHROUGH.md).
Not sure whether to use a Pi or run AfterTouch on the speaker itself? See
[DEPLOYMENT-OVERVIEW.md](DEPLOYMENT-OVERVIEW.md).

---

## soundtouch-service

### Installation

```bash
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install.sh
sudo bash install.sh
```

Install a specific version:

```bash
sudo bash install.sh v0.111.3
```

Override defaults at install time:

```bash
sudo \
  VERSION=v0.111.3 \
  HOSTNAME_FQDN=soundtouch.local \
  HTTP_PORT=80 \
  HTTPS_PORT=443 \
  bash install.sh
```

### Configuration

```
/etc/soundtouch-service/soundtouch-service.env
```

Example:

```bash
PORT=80
HTTPS_PORT=443
DATA_DIR=/var/lib/soundtouch-service

LOG_PROXY_BODY=false
REDACT_PROXY_LOGS=true
RECORD_INTERACTIONS=true
DISCOVERY_INTERVAL=5m

SERVER_URL=http://soundtouch.local
HTTPS_SERVER_URL=https://soundtouch.local
```

After editing the env file:

```bash
sudo systemctl restart soundtouch-service
```

### Service management

```bash
systemctl status soundtouch-service
sudo systemctl enable soundtouch-service   # start on boot
sudo systemctl disable soundtouch-service
sudo systemctl stop soundtouch-service
sudo systemctl start soundtouch-service
sudo systemctl restart soundtouch-service
```

### Logs

```bash
journalctl -u soundtouch-service -e --no-pager   # recent
journalctl -u soundtouch-service -f               # follow live
journalctl -u soundtouch-service -b               # this boot only
```

### Updates

```bash
sudo bash install.sh              # update to latest release
sudo bash install.sh v0.111.3     # update to a specific version
```

The script stops the service, downloads the new binary (backs up the old one to
`.old`), and restarts automatically. Your env file and data directory are preserved.

### Removal

Use the uninstaller, which stops and disables the service and removes the unit,
binary, and config. Your data directory is **preserved** by default:

```bash
curl -fsSL -o uninstall.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/uninstall.sh
sudo bash uninstall.sh              # keep /var/lib/soundtouch-service
sudo bash uninstall.sh --purge      # also delete the data directory
```

The `soundtouch:soundtouch` user/group is removed only once no other
`soundtouch-*` install remains on the host.

Prefer to do it by hand? The equivalent manual steps are:

```bash
sudo systemctl disable --now soundtouch-service
sudo rm /etc/systemd/system/soundtouch-service.service
sudo rm -rf /etc/soundtouch-service
sudo rm /usr/local/bin/soundtouch-service
sudo systemctl daemon-reload
# Datastore (presets, device registrations, certs) — delete only if you are
# sure you no longer need it:
sudo rm -rf /var/lib/soundtouch-service
```

---

## soundtouch-player

`soundtouch-player` is a stateless browser control panel — it holds no persistent
data and can be stopped or restarted at any time without data loss.

### Installation

```bash
curl -fsSL -o install-player.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-player.sh
sudo bash install-player.sh
```

Install a specific version:

```bash
sudo bash install-player.sh v0.111.3
```

Override defaults at install time:

```bash
sudo \
  VERSION=v0.111.3 \
  HTTP_PORT=8081 \
  bash install-player.sh
```

Once running, open **`http://<pi-ip>:8080`** in a browser.

### Configuration

```
/etc/soundtouch-player/soundtouch-player.env
```

Example:

```bash
PORT=8080
BIND_ADDR=
DISCOVERY_INTERFACE=
SOUNDTOUCH_DEVICES=
SERVICE_URL=
SERVICE_CA=
```

`SOUNDTOUCH_DEVICES` accepts a comma-separated list of IP addresses for manual
device registration — useful when mDNS auto-discovery is unreliable on your
network:

```bash
SOUNDTOUCH_DEVICES=192.0.2.1,192.0.2.2
```

`SERVICE_URL` links `soundtouch-player` to your `soundtouch-service` instance,
which is required for Text-to-Speech ("Speak"). When the service is served
over HTTPS with its own self-signed certificate (the default), also set
`SERVICE_CA` to that CA certificate, or the proxied TTS call fails with
`x509: certificate signed by unknown authority`. The CA is the service's
`<dataDir>/certs/ca.crt` (also downloadable from `GET /setup/ca.crt`). For
example:

```bash
SERVICE_URL=https://soundtouch.local
SERVICE_CA=/var/lib/soundtouch-service/certs/ca.crt
```

With a plain `http://` `SERVICE_URL`, `SERVICE_CA` is unused (no TLS) and can
be left empty.

After editing the env file:

```bash
sudo systemctl restart soundtouch-player
```

### Port conflicts

Port 8080 is a common default for other services. To check what is already
using it:

```bash
sudo ss -tulpn | grep :8080
```

To use a different port, pass `HTTP_PORT=<port>` to the installer, or edit
the env file after installation and restart the service.

### Service management

```bash
systemctl status soundtouch-player
sudo systemctl enable soundtouch-player    # start on boot
sudo systemctl disable soundtouch-player
sudo systemctl stop soundtouch-player
sudo systemctl start soundtouch-player
sudo systemctl restart soundtouch-player
```

### Logs

```bash
journalctl -u soundtouch-player -e --no-pager
journalctl -u soundtouch-player -f
```

### Updates

```bash
sudo bash install-player.sh              # update to latest release
sudo bash install-player.sh v0.111.3     # update to a specific version
```

### Removal

Use the uninstaller:

```bash
curl -fsSL -o uninstall-player.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/uninstall-player.sh
sudo bash uninstall-player.sh
```

The `soundtouch:soundtouch` user/group is removed only once no other
`soundtouch-*` install remains on the host.

Prefer to do it by hand? The equivalent manual steps are:

```bash
sudo systemctl disable --now soundtouch-player
sudo rm /etc/systemd/system/soundtouch-player.service
sudo rm -rf /etc/soundtouch-player
sudo rm /usr/local/bin/soundtouch-player
sudo systemctl daemon-reload
```

---

## Migrating from soundtouch-web

`soundtouch-web` was renamed to `soundtouch-player`. The old `install-web.sh`
installer and the `soundtouch-web` release asset still exist as deprecated
aliases and will be removed in a future release.

If you have an existing `soundtouch-web` install, remove it and switch to
`soundtouch-player`:

```bash
# 1. Remove the old soundtouch-web service:
curl -fsSL -o uninstall-web.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/uninstall-web.sh
sudo bash uninstall-web.sh

# 2. Install soundtouch-player (see the section above):
curl -fsSL -o install-player.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-player.sh
sudo bash install-player.sh
```

`uninstall-web.sh` stops and disables the `soundtouch-web` service and removes
its unit, binary, and `/etc/soundtouch-web` config. Both binaries are stateless,
so there is no data to migrate; re-create any per-host settings in
`/etc/soundtouch-player/soundtouch-player.env` (the variables are identical).

---

## Architecture auto-detection

Both installers detect the CPU and pick the matching release asset automatically:

| `uname -m`          | asset suffix  |
|---------------------|---------------|
| `aarch64`           | `linux-arm64` |
| `armv7l` / `armv6l` | `linux-armv7` |
| `x86_64`            | `linux-amd64` |

Override if needed:

```bash
sudo ARCH_ASSET=linux-arm64 bash install.sh
sudo ARCH_ASSET=linux-arm64 bash install-player.sh
```

---

## Security

Both services run as the `soundtouch` system user (no login shell, no home
directory). `soundtouch-service` additionally uses
`AmbientCapabilities=CAP_NET_BIND_SERVICE` to bind ports 80 / 443 without root.
