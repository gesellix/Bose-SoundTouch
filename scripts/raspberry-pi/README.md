# Raspberry Pi installers

Two installer scripts are available, one for each binary:

| Script           | Binary               | Role                                    | Default port |
|------------------|----------------------|-----------------------------------------|--------------|
| `install.sh`     | `soundtouch-service` | Cloud-replacement relay — must run 24/7 | 80 / 443     |
| `install-web.sh` | `soundtouch-web`     | Browser control panel — run on demand   | 8080         |

Both scripts auto-detect CPU architecture (armv7 / arm64 / amd64), create a systemd unit,
and are safe to re-run for updates.

---

# soundtouch-service

## Installation

```bash
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install.sh
sudo bash install.sh
```

Override defaults at install time:

```bash
sudo \
  VERSION=v0.97.0 \
  HOSTNAME_FQDN=soundtouch.local \
  HTTP_PORT=80 \
  HTTPS_PORT=443 \
  bash install.sh
```

## Configuration

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

## Service management

```bash
systemctl status soundtouch-service
sudo systemctl enable soundtouch-service   # start on boot
sudo systemctl disable soundtouch-service
sudo systemctl stop soundtouch-service
sudo systemctl start soundtouch-service
sudo systemctl restart soundtouch-service
```

## Logs

```bash
journalctl -u soundtouch-service -e --no-pager   # recent
journalctl -u soundtouch-service -f               # follow
journalctl -u soundtouch-service -b               # this boot
```

## Updates

```bash
sudo bash install.sh vX.Y.Z
```

The script self-updates, downloads the new binary, backs up the old one to `.old`, and
restarts the service. Your env file and data are preserved.

## Removal

```bash
sudo systemctl disable --now soundtouch-service
sudo rm /etc/systemd/system/soundtouch-service.service
sudo rm -rf /etc/soundtouch-service
sudo rm -rf /var/lib/soundtouch-service
sudo rm /usr/local/bin/soundtouch-service
sudo systemctl daemon-reload
```

---

# soundtouch-web

## Installation

```bash
curl -fsSL -o install-web.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-web.sh
sudo bash install-web.sh
```

Override defaults at install time:

```bash
sudo \
  VERSION=v0.97.0 \
  HTTP_PORT=8081 \
  bash install-web.sh
```

`soundtouch-web` is **stateless** — it holds no persistent data and can be stopped or
restarted at any time without data loss.

## Configuration

```
/etc/soundtouch-web/soundtouch-web.env
```

Example:

```bash
PORT=8080
BIND_ADDR=
DISCOVERY_INTERFACE=
SOUNDTOUCH_DEVICES=
```

`SOUNDTOUCH_DEVICES` accepts a comma-separated list of IP addresses for manual device
registration (useful when mDNS auto-discovery is unreliable on your network).

After editing the env file:

```bash
sudo systemctl restart soundtouch-web
```

## Port conflicts

Port 8080 is a common default for other services. To use a different port, either pass
`HTTP_PORT=<port>` to the installer, or edit the env file after installation:

```bash
sudo ss -tulpn | grep :8080   # check what's using the port
```

## Service management

```bash
systemctl status soundtouch-web
sudo systemctl enable soundtouch-web    # start on boot
sudo systemctl disable soundtouch-web
sudo systemctl stop soundtouch-web
sudo systemctl start soundtouch-web
sudo systemctl restart soundtouch-web
```

## Logs

```bash
journalctl -u soundtouch-web -e --no-pager
journalctl -u soundtouch-web -f
```

## Updates

```bash
sudo bash install-web.sh vX.Y.Z
```

## Removal

```bash
sudo systemctl disable --now soundtouch-web
sudo rm /etc/systemd/system/soundtouch-web.service
sudo rm -rf /etc/soundtouch-web
sudo rm /usr/local/bin/soundtouch-web
sudo systemctl daemon-reload
```

---

# Architecture auto-detection

Both installers detect the CPU and pick the matching release asset automatically:

| `uname -m`          | asset suffix  |
|---------------------|---------------|
| `aarch64`           | `linux-arm64` |
| `armv7l` / `armv6l` | `linux-armv7` |
| `x86_64`            | `linux-amd64` |

Override if needed:

```bash
sudo ARCH_ASSET=linux-arm64 bash install.sh
sudo ARCH_ASSET=linux-arm64 bash install-web.sh
```

---

# Security

Both services run as the `soundtouch` system user (no login shell, no home directory
ownership required for `soundtouch-web`). `soundtouch-service` additionally uses
`AmbientCapabilities=CAP_NET_BIND_SERVICE` to bind ports 80/443 without root.
