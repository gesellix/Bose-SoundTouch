---
title: "External Host Walkthrough"
---
A step-by-step guide to running AfterTouch on a Raspberry Pi (or any always-on
computer) and migrating your Bose SoundTouch speakers to use it.

By the end you'll have AfterTouch running on your host, your speaker(s) pointed
at it, and your radio presets working again.

For a comparison with the on-device install option see
[DEPLOYMENT-OVERVIEW.md](DEPLOYMENT-OVERVIEW.md).

---

## What you need

- A **Raspberry Pi** (Zero 2W, 3, or 4), a NAS, or any always-on Linux/macOS/Windows
  machine on your home network.
- Your **Bose SoundTouch speaker** on the same network. It doesn't need SSH enabled.
- A **browser** on any device on the same network.
- The **speaker's LAN IP address** — find it in your router's device list, or run
  `arp -a` on your computer. Replace `192.0.2.1` throughout with the real IP.

---

## Step 1 — Install AfterTouch on your host

### Raspberry Pi (recommended for always-on use)

```bash
curl -fsSL -o install.sh \
  https://raw.githubusercontent.com/gesellix/bose-soundtouch/main/scripts/raspberry-pi/install.sh
sudo bash install.sh
```

The installer detects your Pi's architecture (armv7, arm64, or amd64), downloads
the binary, creates a `soundtouch` system user, and registers a systemd unit that
starts on boot.

To install a specific version:

```bash
sudo bash install.sh v0.111.3
```

Check that the service is running:

```bash
systemctl status soundtouch-service
```

The Admin UI is now at **`http://<pi-ip>`** (port 80: the Raspberry Pi
installer defaults to port 80, not 8000) — open it in a browser.

### Other Linux hosts (systemd)

Download the binary for your architecture from the
[Releases page](https://github.com/gesellix/Bose-SoundTouch/releases), then
install it as a systemd service — see [DEPLOYMENT.md](DEPLOYMENT.md) for the
unit file template.

### Docker

```bash
docker run -d \
  --name aftertouch \
  --network host \
  -e SERVER_URL=http://192.0.2.10:8000 \
  -v aftertouch-data:/data \
  ghcr.io/gesellix/bose-soundtouch:latest
```

Replace `192.0.2.10` with the host machine's LAN IP. The `--network host` flag
is required so AfterTouch can reach the speakers and respond to mDNS discovery.

---

## Step 2 — Note your host's LAN IP and open the Admin UI

Your host's LAN IP is the address your speakers will use to reach AfterTouch.
Find it with:

```bash
# Linux / macOS
ip addr show     # or: hostname -I
# Windows
ipconfig
```

Open **`http://<host-ip>:8000`** in a browser. You should see the AfterTouch
Admin UI. If it's not reachable, check that port 8000 is not blocked by a
firewall on the host.

---

## Step 3 — Discover your speaker

In the Admin UI:

1. Go to the **Devices** tab. AfterTouch runs mDNS discovery automatically —
   your speaker should appear within a minute or two.
2. If it doesn't appear, click **Trigger Discovery**. If it still doesn't appear,
   add it manually: enter `192.0.2.1` (your speaker's IP) and click **Add**.

You should now see your speaker listed with its name and model.

---

## Step 4 — Migrate the speaker

This step tells the speaker to use your AfterTouch instance instead of the
defunct Bose cloud. **The speaker gets a new server URL written to it; this is
reversible via the "Revert" button.**

1. Click your speaker in the Devices list.
2. Click **Migrate** (or open the Migration wizard from the speaker's detail page).
3. The wizard shows a summary of what will change. Review it and click **Confirm**.
4. AfterTouch rewrites the speaker's server-URL config and reboots it.
5. Wait 2–3 minutes for the speaker to come back up.

After the reboot the speaker reconnects to AfterTouch. You should see its status
turn green in the Devices tab.

> **If the migration wizard asks for your AfterTouch URL**, enter
> `http://<host-ip>:8000` (the same URL you used to open the Admin UI).

---

## Step 5 — Check the Health tab and run QuickFixes

1. Click your speaker in the Devices list, then open the **Health** tab.
2. Click **Run health checks** (or wait for them to run automatically).
3. Look for any warnings. The most common after a fresh migration:

   | Warning                                         | QuickFix action                                                                                                            |
   |-------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------|
   | *Speaker reports an empty `<margeAccountUUID>`* | Click **Pair account** / **Apply QuickFix** and confirm. The speaker will reboot.                                          |
   | *INTERNET_RADIO source is a stale stub*         | Click **Remove INTERNET_RADIO source**.                                                                                    |
   | *TuneIn / Radio Browser missing from sources*   | These appear automatically once the speaker has paired; if still missing after a QuickFix reboot, trigger discovery again. |

4. After any QuickFix that reboots the speaker, re-run the health checks to
   confirm the warning is gone.

---

## Step 6 — Verify pairing and sources

After the health checks are green, confirm from your browser:

- In the **Devices** tab, click the speaker. Its **Account** field should show
  a non-empty UUID, not `default`.
- In the **Sources** tab (if present), or via the health checks, confirm
  `LOCAL_INTERNET_RADIO`, `TUNEIN`, and `RADIO_BROWSER` are listed.

You can also check directly from any machine on the LAN:

```bash
# Replace 192.0.2.1 with your speaker's IP
curl -s http://192.0.2.1:8090/info | grep -i marge
curl -s http://192.0.2.1:8090/sources
```

`margeAccountUUID` must not be empty. Sources should include
`LOCAL_INTERNET_RADIO`, `TUNEIN`, and `RADIO_BROWSER`.

---

## Step 7 — Set up preset buttons (optional)

### Via soundtouch-player

The Radio Browser, TuneIn tabs, and preset saving live in
**soundtouch-player**, a separate binary from the service. Once running,
open **`http://<host-ip>:8080`** in your browser (default port 8080).

### Installing soundtouch-player on a Raspberry Pi

`install.sh` only installs `soundtouch-service`. Use the dedicated
`install-player.sh` script to add soundtouch-player:

```bash
curl -fsSL -o install-player.sh \
  https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/raspberry-pi/install-player.sh
sudo bash install-player.sh
```

For configuration, service management, updates, and removal see the
[Raspberry Pi guide → soundtouch-player](RASPBERRY-PI.md#soundtouch-player).

### Installing soundtouch-player on other hosts

Download the binary for your OS and architecture from the
[Releases page](https://github.com/gesellix/Bose-SoundTouch/releases)
and run it directly:

```bash
./soundtouch-player --port 8080
```

Or install it as a systemd service following the same unit-file pattern
described in [DEPLOYMENT.md](DEPLOYMENT.md).

soundtouch-player provides two ways to save what's currently playing to a
preset slot:

**★ Star button in the Now Playing card**

1. Open **`http://<host-ip>:8080`** and navigate to your speaker.
2. Use the **Radio Browser** or **TuneIn** tab to find a station and click
   it to play.
3. A semi-transparent **★** appears in the top-right corner of the
   **Now Playing** card.  Click it — a slot picker (1–6) opens.  Select
   the target preset number.  The star turns gold once the content is
   mapped to at least one slot.
4. Repeat for each station you want to save.

**+ button on the preset grid**

Alternatively, while something is playing you can hover over any of the six
preset tiles in the **Presets** row.  A small **+** button appears in the
corner of each tile; clicking it saves the current content directly to that
slot.

### Alternatively — storing presets via soundtouch-cli (any machine on the LAN)

Download the CLI for your machine from the
[Releases page](https://github.com/gesellix/Bose-SoundTouch/releases), then:

```bash
# Play a custom radio stream on the speaker
soundtouch-cli --host 192.0.2.1 source custom-radio \
  --url "https://stream.laut.fm/country-nonstop" \
  --name "Country Nonstop" \
  --service-url "http://<host-ip>:8000"
sleep 5

# Store it to preset slot 1
soundtouch-cli --host 192.0.2.1 preset store-current --slot 1
```

Repeat for each slot. See the
[On-Device Install Walkthrough](ON-DEVICE-INSTALL-WALKTHROUGH.md#step-9--store-custom-radio-streams-to-preset-buttons)
for six worked examples with Austrian internet radio stations.

---

## Step 8 — Verify and enjoy

Press a preset button on the speaker briefly (a long press overwrites the preset).
Each slot should play the corresponding stream.

To check what's stored:

```bash
curl -s http://192.0.2.1:8090/presets
```

---

## Updating AfterTouch

### Raspberry Pi

```bash
sudo bash install.sh              # updates to latest release
sudo bash install.sh v0.111.3     # updates to a specific version
```

The installer stops the service, downloads the new binary, and restarts
automatically.

### Checking current version

```bash
systemctl status soundtouch-service   # shows the running version in the log
curl -s http://<host-ip>:8000/health | grep version
```

---

## Troubleshooting

| Symptom                                                | First check                                                                                                                       |
|--------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| Speaker not appearing in Devices                       | Click **Trigger Discovery**; try adding the IP manually                                                                           |
| Migration fails                                        | Confirm the speaker can reach `http://<host-ip>:8000` — try `curl http://<host-ip>:8000` from the speaker's SSH shell             |
| `margeAccountUUID` still empty after QuickFix + reboot | Re-run Health QuickFix, reboot again                                                                                              |
| Radio source error 1005                                | `margeAccountUUID` is empty — complete Step 5 first                                                                               |
| Speaker reverts to Bose cloud after router restart     | Your router's DNS is overriding AfterTouch's server URL — see [MIGRATION-GUIDE.md](MIGRATION-GUIDE.md) for DNS-interception setup |
| Admin UI not reachable                                 | Check `systemctl status soundtouch-service` and firewall rules for port 8000                                                      |

For more detail see [TROUBLESHOOTING.md](TROUBLESHOOTING.md) and
[MIGRATION-GUIDE.md](MIGRATION-GUIDE.md).
