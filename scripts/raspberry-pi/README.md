Here is a `README.md` you can place next to your install script (or in your repo) to document installation, configuration, updates, and debugging.

---

# SoundTouch Service (systemd install)

This setup installs `soundtouch-service` from the official GitHub release and runs it as a hardened systemd service.

It supports:

* Automatic start on boot
* Binding to privileged ports (80 / 443) without running as root
* Config via environment file
* Clean updates
* Safe re-runs of the installer

---

# Installation

Run the installer script:

```bash
sudo bash install-soundtouch-service.sh
```

You can override defaults:

```bash
sudo \
  VERSION=v0.17.0 \
  HOSTNAME_FQDN=soundtouch.local \
  HTTP_PORT=80 \
  HTTPS_PORT=443 \
  bash install-soundtouch-service.sh
```

---

# Configuration

Configuration lives in:

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

---

# Important: Applying Configuration Changes

If you change the environment file, you must reload and restart the service.

Full roundtrip:

```bash
sudo systemctl daemon-reload
sudo systemctl restart soundtouch-service
```

Usually `daemon-reload` is only needed if the **unit file** changed.

If only the `.env` file changed:

```bash
sudo systemctl restart soundtouch-service
```

---

# Service Management

Check status:

```bash
systemctl status soundtouch-service
```

Enable at boot:

```bash
sudo systemctl enable soundtouch-service
```

Disable:

```bash
sudo systemctl disable soundtouch-service
```

Stop / start manually:

```bash
sudo systemctl stop soundtouch-service
sudo systemctl start soundtouch-service
```

---

# Logs & Debugging

View recent logs:

```bash
journalctl -u soundtouch-service -e --no-pager
```

Follow logs live:

```bash
journalctl -u soundtouch-service -f
```

Show logs from current boot:

```bash
journalctl -u soundtouch-service -b
```

If the service fails to start:

```bash
systemctl status soundtouch-service --no-pager
```

Look for:

* `bind: permission denied` → capability issue
* `address already in use` → port conflict
* permission errors in DATA_DIR → ownership issue

---

# Port Conflicts

Check if 80/443 are in use:

```bash
sudo ss -tulpn | grep -E ':80|:443'
```

If another service is using the port, either:

* stop/disable that service
* or change `PORT` / `HTTPS_PORT` in the env file

Then restart the service.

---

# Updating to a New Version

To upgrade, simply run the installer with the desired version as an argument:

```bash
sudo bash install.sh vX.Y.Z
```

The script will:

* Automatically fetch the latest version of the installer script for that release
* Download the new service binary
* Backup the old binary to `.old`
* Overwrite the binary and restart the service

No need to reconfigure anything; your existing `.env` file and data will be preserved.

---

# Reinstall / Reset

To fully reset:

```bash
sudo systemctl stop soundtouch-service
sudo rm -rf /var/lib/soundtouch-service/*
sudo systemctl start soundtouch-service
```

To completely remove:

```bash
sudo systemctl disable --now soundtouch-service
sudo rm /etc/systemd/system/soundtouch-service.service
sudo rm -rf /etc/soundtouch-service
sudo rm -rf /var/lib/soundtouch-service
sudo rm /usr/local/bin/soundtouch-service
sudo systemctl daemon-reload
```

---

# Architecture Auto-Detection

The installer auto-detects:

* `linux-armv7`
* `linux-arm64`
* `linux-amd64`

Override manually if needed:

```bash
sudo ARCH_ASSET=linux-arm64 bash install-soundtouch-service.sh
```

---

# Security Notes

The service:

* Runs as a dedicated `soundtouch` system user
* Uses `AmbientCapabilities=CAP_NET_BIND_SERVICE`
* Does not require `setcap`
* Does not run as root
* Uses systemd sandboxing (`ProtectSystem`, `PrivateTmp`, etc.)

---

# Quick Troubleshooting Checklist

If something does not work:

1. Check status:

   ```
   systemctl status soundtouch-service
   ```

2. Check logs:

   ```
   journalctl -u soundtouch-service -e
   ```

3. Confirm ports:

   ```
   ss -tulpn | grep -E ':80|:443'
   ```

4. Confirm env file:

   ```
   cat /etc/soundtouch-service/soundtouch-service.env
   ```

5. Restart cleanly:

   ```
   sudo systemctl restart soundtouch-service
   ```

---

If you’d like, I can also provide:

* A `make update` style wrapper
* A rollback mechanism
* Or a self-update script with checksum verification
