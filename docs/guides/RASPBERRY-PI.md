# Raspberry Pi Installation Guide

This guide explains how to install the `soundtouch-service` as a persistent systemd service on a Raspberry Pi (tested on Raspberry Pi Zero 2W, 3, and 4).

## Automated Installer

We provide a specialized installer script located in the `scripts/raspberry-pi/` directory of the repository.

### Features
*   **Automatic start on boot**: Installs a systemd unit.
*   **Non-root operation**: Uses `AmbientCapabilities` to bind to ports 80/443 without root privileges.
*   **Arch Detection**: Automatically selects the correct binary for `armv7`, `arm64`, or `amd64`.
*   **Easy Updates**: Re-running the script updates the binary to the latest version.

### Installation Steps

1.  **Download the installer**:
    ```bash
    curl -fsSL -o install.sh https://raw.githubusercontent.com/gesellix/bose-soundtouch/main/scripts/raspberry-pi/install.sh
    ```

2.  **Run with sudo**:
    ```bash
    sudo bash install.sh
    ```

### Overriding Defaults

You can customize the installation using environment variables:

```bash
sudo \
  VERSION=v0.17.0 \
  HOSTNAME_FQDN=soundtouch.local \
  HTTP_PORT=80 \
  HTTPS_PORT=443 \
  bash install.sh
```

## Management

Once installed, use standard `systemctl` commands to manage the service:

```bash
# Check status
systemctl status soundtouch-service

# Follow logs
journalctl -u soundtouch-service -f

# Restart
sudo systemctl restart soundtouch-service
```

## Configuration

Configuration is stored in `/etc/soundtouch-service/soundtouch-service.env`. Note that settings saved via the Web UI (in `settings.json`) will take precedence over these environment variables once the service is running.

For more details, see the [scripts/raspberry-pi/README.md](../../scripts/raspberry-pi/README.md) in the repository.
