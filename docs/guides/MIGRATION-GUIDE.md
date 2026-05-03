# Migration Guide: From Bose Cloud to AfterTouch

This guide walks through the complete process of migrating your SoundTouch speakers from Bose's cloud services to **AfterTouch**, the local replacement provided by `soundtouch-service`. By the end, your speakers will work fully independently of Bose's servers.

For a shorter overview, see the [Survival Guide](SURVIVAL-GUIDE.md). For safety considerations and rollback options, see the [Migration & Safety Guide](MIGRATION-SAFETY.md).

---

## What you need

- A machine that is **always on** (Raspberry Pi, NAS, home server, or similar) to run the service
- A **USB drive** (FAT-formatted) to enable SSH on each speaker
- Your speakers must be on the **same network** as the service host
- About **15–30 minutes per speaker**

---

## Step 1: Install and start the service

Choose the option that fits your setup.

### Binary (go install)

```bash
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest
soundtouch-service
```

The service starts on port 8000. Open `http://localhost:8000` in your browser.

### Docker Compose (recommended for home servers and VMs)

The repository ships a `docker-compose.yml` ready for this use case. Clone or download it, copy the example config, then edit `.env` before starting:

```bash
cp .env.example .env
# Edit .env:
#   SOUNDTOUCH_HOSTNAME=192.168.1.100   ← your server's address
#   SOUNDTOUCH_VERSION=v0.70.0          ← pin to a release tag instead of 'latest'
docker compose up -d
```

`SOUNDTOUCH_HOSTNAME` is the address your speakers will use to reach the service — use a hostname or IP reachable from the speaker, not `localhost`.

On **Linux** (Debian, Proxmox VE, Raspberry Pi OS, etc.) you can enable host networking for automatic speaker discovery. Uncomment the `network_mode: host` line in `docker-compose.yml` and remove the `ports:` section (they conflict with host networking). Without host networking, add your speakers by IP address in Step 4 instead.

For local overrides (e.g. switching to `build: .` during development), create a `docker-compose.override.yml` — Docker Compose picks it up automatically and it is not tracked in version control.

> **Note on `docker-compose.ci.yml`**: this file contains mock services used only for automated integration tests. It is not needed for your own deployment.

### Docker run (Linux — with host networking for device discovery)

```bash
docker run -d \
  --name soundtouch-service \
  --network host \
  -v $(pwd)/data:/app/data \
  ghcr.io/gesellix/bose-soundtouch:latest
```

### Docker run (macOS / Windows — manual device IP required)

```bash
docker run -d \
  --name soundtouch-service \
  -p 8000:8000 -p 8443:8443 \
  -v $(pwd)/data:/app/data \
  --env SERVER_URL=http://soundtouch.local:8000 \
  --env HTTPS_SERVER_URL=https://soundtouch.local:8443 \
  ghcr.io/gesellix/bose-soundtouch:latest
```

On macOS/Windows, device discovery via mDNS won't work inside the container — you'll add devices by IP address in Step 4.

See [Raspberry Pi Setup](RASPBERRY-PI.md) and the [SoundTouch Service Guide](SOUNDTOUCH-SERVICE.md) for more deployment options.

---

## Step 2: Configure the service URL

Open `http://<server>:8000` and go to the **Settings** tab.

![AfterTouch Settings tab](../images/ui-settings.png)

Set the **Target Domain** to the address your speakers can reach — for example `https://soundtouch.fritz.box` or `http://192.168.1.100:8000`. This must be the host's address on your local network, not `localhost`.

If you plan to use DNS/DHCP redirect, enable the **DNS Discovery Server** and set the **DNS Bind Address** to `:53`. The upstream DNS should be your router's IP, not the service's own address.

> **Tip**: If you change settings and they don't seem to take effect, check `data/settings.json` — settings saved in the UI take precedence over environment variables.

---

## Step 3: Enable SSH on each speaker

The migration writes updated configuration to the speaker's filesystem, which requires SSH access. Enable it once per device:

1. Format a USB drive as FAT (FAT32). Some speakers require the **bootable flag** to be set on the partition — see [SoundCork issue #172](https://github.com/deborahgu/soundcork/issues/172) for details.
2. Create an empty file named **`remote_services`** (no extension) in the root of the drive.
3. Insert the drive into the speaker's USB port while it is powered on.
4. Power-cycle the speaker (unplug the power cable, wait 10 seconds, reconnect).
5. After boot, root SSH is available with no password: `ssh -oHostKeyAlgorithms=+ssh-rsa root@<SPEAKER-IP>`

You only need to do this once per speaker. SSH can remain enabled for future maintenance or be disabled after migration — your choice.

---

## Step 4: Add and sync your speaker

### Discover

The service scans for SoundTouch devices automatically every few minutes. Check the **Devices** tab in the web UI. If your speaker doesn't appear, click **Scan Again** to trigger an immediate scan, or enter the IP address manually and click **Add Device**.

![AfterTouch Devices tab showing discovered speakers](../images/ui-devices.png)

### Sync

Once the speaker appears, click **Sync Data**. This connects to the speaker and pulls its current presets, recently played items, and configured sources into the local service's datastore. It also creates an off-device backup of the speaker's configuration.

![Data Sync tab showing a successful sync](../images/ui-sync.png)

If the Bose cloud is still running, Sync also fetches your account data from Bose's servers. This is your preservation step — do it before the cloud shuts down.

---

## Step 5: Migrate

Click **Migrate** next to a device on the Devices tab to open the Migration tab. It shows SSH status, CA trust status, and connection test results before letting you apply the redirect.

![Migration tab showing HTTPS and DNS connection tests](../images/ui-migration.png)

Two redirect methods are available:

### XML redirect (recommended for first-time / testing)

Uploads a configuration file to the speaker via the SoundTouch Web API. This changes the application-level service URLs without touching the speaker's network configuration. It's the least invasive option.

The web UI guides you through:
1. Previewing the config change (current vs. planned XML)
2. Optionally installing the AfterTouch CA certificate on the speaker (requires SSH; needed for HTTPS)
3. Applying the XML redirect
4. Verifying the speaker can reach the local service

### DNS/DHCP redirect (recommended for permanent / all-device setup)

Configures the speaker to use a custom DNS server that resolves Bose cloud hostnames to the local service. This is the most robust method — it covers all Bose endpoints automatically and survives reboots.

Requirements:
- The AfterTouch DNS server must be running and bound to **port 53** on your network. Enable it in the **Settings** tab (`DNS Discovery` → enabled).
- HTTPS is required. The web UI walks you through trusting the CA certificate on the speaker (via SSH).

The web UI guides you through:
1. Verifying the DNS server is running and reachable
2. Installing the CA certificate on the speaker
3. Configuring the speaker to use the AfterTouch DNS server
4. Verifying DNS resolution and HTTPS connectivity

---

## Step 6: Reboot and verify

After migration, **power-cycle the speaker** (unplug and replug). This applies all configuration changes.

After reboot:
- The speaker should appear as **migrated** in the Devices tab
- Presets should load and play (served from the local service)
- TuneIn browsing should work
- Recently played items should appear

If something doesn't work, check the **Interactions** tab in the web UI for failed requests, and the **Troubleshooting** section in the [SoundTouch Service Guide](SOUNDTOUCH-SERVICE.md).

---

## Repeat for each speaker

Each speaker is migrated independently. You can run multiple migrations in parallel, but migrating one at a time makes it easier to diagnose issues.

---

## Rollback

If you need to undo a migration:

- **From the web UI**: Use the **Revert** action on the device — this restores the `.original` backup files created on the speaker during migration.
- **Via SSH**: The original config is backed up on the speaker with a `.original` suffix. Restore it manually if the UI is unreachable.
- **Factory reset**: As a last resort, perform a factory reset (see [Device Initial Setup](DEVICE-INITIAL-SETUP.md) for button sequences). This wipes all configuration and returns the speaker to out-of-box state.

---

## Post-migration

Once all speakers are migrated, the `data/` directory is the source of truth for your presets, recents, and device state. Back it up periodically. The web UI at `http://<server>:8000` is your management interface from this point on.

For the Bose cloud backup you created in Step 4, keep the `.tar.gz` archive in case you need to restore credentials or presets later.
