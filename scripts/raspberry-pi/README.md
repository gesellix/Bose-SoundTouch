# Raspberry Pi installers

Full documentation — installation, configuration, service management, updates,
and removal for both `soundtouch-service` and `soundtouch-player` — lives in the
project docs:

**[docs/content/docs/guides/RASPBERRY-PI.md](../../docs/content/docs/guides/RASPBERRY-PI.md)**

---

## Quick start

**soundtouch-service** (cloud-replacement relay):

```bash
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install.sh
sudo bash install.sh
```

**soundtouch-player** (browser control panel):

```bash
curl -fsSL -o install-player.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-player.sh
sudo bash install-player.sh
```

Pass a version tag as the first argument to pin a specific release:

```bash
sudo bash install.sh v0.107.0
sudo bash install-player.sh v0.107.0
```
