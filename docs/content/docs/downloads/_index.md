---
title: "Downloads"
weight: 1
sidebar:
  open: true
---

# Downloads

Everything AfterTouch ships is on the
**[GitHub releases page](https://github.com/gesellix/Bose-SoundTouch/releases/latest)**.
This page helps you pick the right file: choose **which tool** you need,
then **which build** matches your computer.

## 1. Which tool do I need?

AfterTouch is a small set of separate programs. Most people run one or
two of them.

| Tool                 | What it does                                                                                   | You want this if…                                        |
|----------------------|-----------------------------------------------------------------------------------------------|----------------------------------------------------------|
| `soundtouch-service` | The local cloud replacement ("AfterTouch"). Runs always-on and takes over from the Bose cloud. | You are migrating speakers off the Bose cloud.           |
| `soundtouch-player`  | A browser control panel (radio browsing, device control).                                      | You want a web UI to browse radio and control speakers.  |
| `soundtouch-cli`     | Command-line control and setup (status, play, presets, groups, **migration**, …).              | You want to script things, or run a migration by hand.   |
| `soundtouch-backup`  | Backs up your Bose cloud account and each speaker's local state.                                | You are preparing before a shutdown / factory reset.     |

> Running a migration from the command line (for example the telnet
> re-migration in the
> [troubleshooting guide](../guides/TROUBLESHOOTING.md#radio-sources-after-migration))
> uses **`soundtouch-cli`**.

## 2. Which build matches my computer?

Release assets are named:

```
soundtouch-<tool>-v<VERSION>-<os>-<arch>[.exe]
```

Pick the `<os>-<arch>` suffix for your system:

| Your system                          | `<os>-<arch>` suffix |
|--------------------------------------|----------------------|
| Raspberry Pi (64-bit) / ARM64 Linux  | `linux-arm64`        |
| Raspberry Pi (32-bit) / ARMv7        | `linux-armv7`        |
| Linux (64-bit PC)                    | `linux-amd64`        |
| macOS (Apple Silicon: M1/M2/M3/…)    | `darwin-arm64`       |
| macOS (Intel)                        | `darwin-amd64`       |
| Windows (64-bit)                     | `windows-amd64.exe`  |
| FreeBSD (64-bit)                     | `freebsd-amd64`      |

**Example.** To control speakers from a Raspberry Pi 4, download the CLI
build `soundtouch-cli-vX.Y.Z-linux-arm64`. On an Apple Silicon Mac you
would take `soundtouch-cli-vX.Y.Z-darwin-arm64` instead.

The download is a single executable, ready to run (no archive to extract).
Each asset ships with `.sha256` and `.sha512` checksum files, and every
release also has combined `checksums.sha256` / `checksums.sha512` if you
want to verify the download.

> **macOS / Windows note:** because these binaries are not code-signed,
> the OS may warn on first launch (Gatekeeper on macOS, SmartScreen on
> Windows). Approve it in the security prompt, or use the Docker or
> install-script routes below.

## 3. Other ways to install

### Install scripts (Linux / Raspberry Pi)

These download the latest release for you and set up a background service.

- **Service** (`soundtouch-service`):

  ```bash
  curl -fsSL -o install.sh \
    https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install.sh
  sudo bash install.sh
  ```

- **Player** (`soundtouch-player`):

  ```bash
  curl -fsSL -o install-player.sh \
    https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-player.sh
  sudo bash install-player.sh
  ```

There is also an **on-device** installer that runs AfterTouch directly on
the speaker; see the
[On-Device Install Walkthrough](../guides/ON-DEVICE-INSTALL-WALKTHROUGH.md).

### Docker

```bash
# AfterTouch service
docker pull ghcr.io/gesellix/bose-soundtouch:latest

# Web player
docker pull ghcr.io/gesellix/bose-soundtouch-player:latest
```

Both images are multi-arch (`linux/amd64`, `linux/arm64`, `linux/arm/v7`).
See the [Deployment Guide](../guides/DEPLOYMENT.md) for Docker Compose
examples.

### Go toolchain

If you have Go installed you can build from source:

```bash
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-cli@latest
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-player@latest
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-backup@latest
```

## 4. Not sure how to deploy?

The [Deployment Overview](../guides/DEPLOYMENT-OVERVIEW.md) compares
running AfterTouch on a Raspberry Pi / always-on host against running it
directly on the speaker, with step-by-step walkthroughs for each path.
For the full migration story, start with the
[Migration Guide](../guides/MIGRATION-GUIDE.md).
