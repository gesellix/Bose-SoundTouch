# soundtouch-backup

A standalone tool for backing up Bose SoundTouch data — both your **cloud account** (presets, devices, sources) and the **local filesystem** of each speaker — before the Bose cloud services shut down on May 6, 2026.

## Overview

| Subcommand | What it backs up                                                                                   |
|------------|----------------------------------------------------------------------------------------------------|
| `all`      | Cloud account **and** all paired speakers in one step — the recommended starting point             |
| `cloud`    | Bose account profile, paired devices, cloud presets, music service sources                         |
| `local`    | Speaker HTTP API data (presets, sources, volume, …) and optionally device filesystem files via SSH |

Output is a single `.tar.gz` archive (or `.zip`) with a dated root directory.

## Building

```bash
make build-backup
# binary: ./build/soundtouch-backup
```

Or install alongside the other tools:

```bash
make install
```

## Usage

### Combined backup (recommended)

The `all` command is the simplest way to capture everything: it authenticates with the Bose cloud, backs up your account data, then reads the IP addresses from `devices.xml` and backs up each reachable speaker over HTTP.

```bash
# Interactive — prompts for email and password
soundtouch-backup all

# Non-interactive
soundtouch-backup all --email you@example.com --password secret

# Include SSH filesystem backup for each speaker
soundtouch-backup all --ssh

# Environment variables
BOSE_EMAIL=you@example.com BOSE_PASSWORD=secret soundtouch-backup all --ssh
```

**Flags**

| Flag         | Short  | Default                               | Description                                            |
|--------------|--------|---------------------------------------|--------------------------------------------------------|
| `--email`    | `-e`   | —                                     | Bose account email (`$BOSE_EMAIL`)                     |
| `--password` | `--pw` | —                                     | Bose account password (`$BOSE_PASSWORD`)               |
| `--ssh`      |        | on                                    | Also capture filesystem files via SSH for each speaker |
| `--output`   | `-o`   | `soundtouch-backup-YYYY-MM-DD.tar.gz` | Output archive path                                    |
| `--format`   |        | `tar.gz`                              | Archive format: `tar.gz` or `zip`                      |

Speakers that are offline or unreachable at the time of backup are skipped with a `✗` warning; the cloud data is still saved.

---

### Cloud backup

Backs up data from your Bose account at `streaming.bose.com`. Credentials are prompted interactively if not supplied as flags.

```bash
# Interactive — prompts for email, masked password input
soundtouch-backup cloud

# Non-interactive
soundtouch-backup cloud --email you@example.com --password secret

# Environment variables (avoids secrets in shell history)
BOSE_EMAIL=you@example.com BOSE_PASSWORD=secret soundtouch-backup cloud

# Zip output
soundtouch-backup cloud --format zip --output my-bose-cloud.zip
```

**Flags**

| Flag         | Short  | Default                               | Description                                       |
|--------------|--------|---------------------------------------|---------------------------------------------------|
| `--email`    | `-e`   | —                                     | Bose account email (`$BOSE_EMAIL`)                |
| `--password` | `--pw` | —                                     | Bose account password (`$BOSE_PASSWORD`)          |
| `--output`   | `-o`   | `soundtouch-backup-YYYY-MM-DD.tar.gz` | Output archive path (`$SOUNDTOUCH_BACKUP_OUTPUT`) |
| `--format`   |        | `tar.gz`                              | Archive format: `tar.gz` or `zip`                 |

**What gets fetched**

| File in archive          | Source endpoint                                                                 |
|--------------------------|---------------------------------------------------------------------------------|
| `cloud/emailaddress.xml` | `GET /streaming/account/{id}/emailaddress`                                      |
| `cloud/devices.xml`      | `GET /streaming/account/{id}/devices`                                           |
| `cloud/sources.xml`      | `GET /streaming/account/{id}/sources`                                           |
| `cloud/presets.xml`      | `GET /streaming/account/{id}/presets/all`                                       |
| `cloud/full.xml`         | `GET /streaming/account/{id}/full` (may overlap with the above; skipped if 4xx) |

---

### Local backup

Backs up each speaker over its HTTP API on port 8090. With `--ssh`, also captures key filesystem files via SSH.

```bash
# Auto-discover all speakers on the local network
soundtouch-backup local

# Specific speaker
soundtouch-backup local --host 192.0.2.11

# Multiple speakers
soundtouch-backup local --host 192.0.2.11 --host 192.0.2.10

# Include SSH filesystem backup
soundtouch-backup local --ssh

# Longer discovery window on busy networks
soundtouch-backup local --discover-timeout 10s
```

**Flags**

| Flag                 | Short | Default                               | Description                                      |
|----------------------|-------|---------------------------------------|--------------------------------------------------|
| `--host`             | `-H`  | —                                     | Speaker host/IP, repeatable (`$SOUNDTOUCH_HOST`) |
| `--port`             | `-p`  | `8090`                                | Speaker HTTP port (`$SOUNDTOUCH_PORT`)           |
| `--discover`         | `-d`  | auto                                  | Force mDNS/UPnP discovery                        |
| `--discover-timeout` |       | `5s`                                  | Discovery timeout                                |
| `--ssh`              |       | on                                    | Also capture filesystem files via SSH            |
| `--output`           | `-o`  | `soundtouch-backup-YYYY-MM-DD.tar.gz` | Output archive path                              |
| `--format`           |       | `tar.gz`                              | Archive format: `tar.gz` or `zip`                |

**What gets fetched via HTTP**

| File                | Device endpoint |
|---------------------|-----------------|
| `info.xml`          | `/info`         |
| `name.xml`          | `/name`         |
| `presets.xml`       | `/presets`      |
| `sources.xml`       | `/sources`      |
| `now_playing.xml`   | `/now_playing`  |
| `volume.xml`        | `/volume`       |
| `bass.xml`          | `/bass`         |
| `balance.xml`       | `/balance`      |
| `capabilities.xml`  | `/capabilities` |
| `network_info.xml`  | `/networkInfo`  |
| `clock_display.xml` | `/clockDisplay` |
| `zone.xml`          | `/getZone`      |

Endpoints that return HTTP 4xx (not supported on the device model) are silently skipped.

**What gets fetched via SSH** (`--ssh`)

SSH connects as `root@<host>:22` with an empty password, which is the default for SoundTouch firmware.

Individual files:

| Remote path               | Notes                                      |
|---------------------------|--------------------------------------------|
| `/etc/hosts`              | DNS redirect state                         |
| `/etc/resolv.conf`        | DNS resolver configuration                 |
| `/etc/remote_services`    | Service registration (post-migration only) |
| `/mnt/nv/remote_services` | Alternative location for remote services   |

Directories (all regular files recursively):

| Remote path                      | Contents                                                                   |
|----------------------------------|----------------------------------------------------------------------------|
| `/opt/Bose/etc/`                 | Full Bose configuration directory, including `SoundTouchSdkPrivateCfg.xml` |
| `/mnt/nv/BoseApp-Persistence/1/` | Persisted app state                                                        |

Missing files and directories are silently skipped with a `⚠` warning.

---

## Archive structure

Both subcommands write into a single dated archive:

```
soundtouch-backup-2026-05-02/
├── cloud/
│   ├── emailaddress.xml
│   ├── devices.xml
│   ├── sources.xml
│   └── presets.xml
└── local/
    ├── A_Sound_Machine/
    │   ├── info.xml
    │   ├── presets.xml
    │   ├── sources.xml
    │   ├── volume.xml
    │   ├── …
    │   └── ssh/
    │       ├── etc/
    │       │   ├── hosts
    │       │   └── resolv.conf
    │       ├── opt/Bose/etc/
    │       │   └── SoundTouchSdkPrivateCfg.xml
    │       └── mnt/nv/BoseApp-Persistence/1/
    └── Sound_Machinechen/
        └── …
```

Running `cloud` and `local` separately produces two archives. To combine them, use the same `--output` path for both invocations — each adds its own subdirectory so they won't collide (`.tar.gz` does not support appending; use `--format zip` if you need a single archive from two runs, or just keep them separate).

## See also

- [Cloud Shutdown Survival Guide](../../docs/content/docs/guides/SURVIVAL-GUIDE.md) — full migration context
- [`soundtouch-cli`](../soundtouch-cli/) — live device control
- [`soundtouch-service`](../soundtouch-service/) — local cloud replacement
