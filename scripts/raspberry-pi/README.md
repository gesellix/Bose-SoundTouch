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
sudo bash install.sh v0.111.3
sudo bash install-player.sh v0.111.3
```

> `install-web.sh` is the old name for `install-player.sh` and still works as a
> deprecated alias. If you installed `soundtouch-web` previously, remove it with
> `uninstall-web.sh` (see below) and switch to `install-player.sh`.

## Removal

Matching uninstallers reverse each installer (stop and disable the service,
remove the unit, binary, and config). You can also remove things by hand — see
the [full guide](../../docs/content/docs/guides/RASPBERRY-PI.md) for the manual
commands.

```bash
# soundtouch-service (keeps the data directory unless you pass --purge):
curl -fsSL -o uninstall.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/uninstall.sh
sudo bash uninstall.sh

# soundtouch-player:
curl -fsSL -o uninstall-player.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/uninstall-player.sh
sudo bash uninstall-player.sh

# soundtouch-web (deprecated; switch to soundtouch-player afterwards):
curl -fsSL -o uninstall-web.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/uninstall-web.sh
sudo bash uninstall-web.sh
```

The shared `soundtouch:soundtouch` user/group is removed only once no other
`soundtouch-*` install remains on the host.
