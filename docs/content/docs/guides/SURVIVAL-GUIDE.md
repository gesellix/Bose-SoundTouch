---
title: "Keeping Your Speakers Alive After the Bose Cloud Shutdown"
---
Bose shut down SoundTouch cloud services on **May 6, 2026**. Per the [official end-of-life page](https://www.bose.com/soundtouch-end-of-life), the following no longer work:

- **Presets** — preset buttons on the product and in the app
- **Music service browsing and playback** from the SoundTouch app (TuneIn, Spotify, etc.)
- **Stereo pairing** for SoundTouch 10
- **Alexa voice commands**
- Software update checks

What **continues to work** regardless:
- The official SoundTouch app for local control (play/pause/volume/source selection)
- Local playback controls via `soundtouch-cli`, `soundtouch-web`, or any app that uses the local Web API
- Bluetooth, AUX, and AirPlay inputs
- Multiroom zones (local, peer-to-peer)

**AfterTouch** — the `soundtouch-service` — restores the first three:

- **Presets** — full preset management including long-press assignment and recently-played sync; music service presets (Spotify, TuneIn, etc.) work once the service is linked (see [Connecting Music Services](MUSIC-SERVICES.md))
- **Music browsing and playback** — TuneIn, Internet Radio, and RadioBrowser via `soundtouch-web`; direct station/URL playback via `soundtouch-cli`; Spotify via Spotify Connect (speaker-native) or AfterTouch's OAuth integration; Amazon Music OAuth infrastructure is in place but streaming is not yet verified
- **Stereo pairing** — via `soundtouch-cli`

Alexa voice commands are not currently supported.

---

## How it works

The service emulates the Bose cloud endpoints that speakers call for music service browsing, device registration, preset sync, and update checks. Once a speaker is redirected to point at the local service instead of Bose's servers, it operates independently. The built-in web UI at `http://<server>:8000` handles all setup steps.

---

## Prerequisites

### 1. A machine that's always on

The service must run on a host that's available whenever your speakers are in use — a Raspberry Pi, NAS, home server, or similar. The host needs a stable local address (e.g. `soundtouch.fritz.box` or a fixed IP) reachable from your speakers.

See [Raspberry Pi Setup](RASPBERRY-PI.md) and the [SoundTouch Service Guide](SOUNDTOUCH-SERVICE.md) for deployment options, including Docker.

### 2. SSH access on your speakers (for migration)

Redirecting a speaker's service URLs requires writing to its configuration. This is done via SSH. Enable it once per device:

1. Create a file named `remote_services` on a FAT-formatted USB drive. The drive may need its bootable flag set — see [SoundCork issue #172](https://github.com/deborahgu/soundcork/issues/172) for details.
2. Insert the drive into the speaker's USB port while it's powered on.
3. Power-cycle the speaker (unplug and replug). After boot, root SSH is available with no password.

You can leave SSH enabled for future maintenance, or disable it once migration is complete.

---

## Scenario A: You migrated before May 6 (or have a backup)

If you ran `soundtouch-backup` and pointed your speakers at AfterTouch before the shutdown, your presets and listening history are already preserved in AfterTouch's datastore. You're done — just keep AfterTouch running.

If you have a backup but haven't migrated yet, start AfterTouch and restore the backup before following the steps below.

**Step 1 — Start the service and open the web UI** at `http://<server>:8000`.

**Step 2 — Configure the server URL.**
In the Settings tab, set the server URL to the address your speakers can reach (e.g. `http://soundtouch.fritz.box:8000`). If you plan to use DNS/DHCP redirect, also configure the HTTPS server URL.

**Step 3 — Add your speaker.**
The service discovers devices on your network automatically. If a speaker doesn't appear, add it manually by IP address.

**Step 4 — Migrate.**
**Step 5 — Reboot the speaker.**
Power-cycle the speaker to apply the changes. After reboot it contacts the local service instead of Bose's cloud.

---

## Scenario B: Set up after the shutdown (or after a factory reset)

If the Bose cloud is gone, or you've factory-reset a speaker, there's no existing account to migrate from. You start fresh with a local account.

**Step 1 — Set up DNS/DHCP redirect first** (recommended).
Configure your network's DNS to resolve the Bose cloud hostnames to the local service's address before the speaker tries to register. This way, when the speaker boots and attempts to register, it reaches AfterTouch automatically instead of failing to reach Bose.

See the [SoundTouch Service Guide](SOUNDTOUCH-SERVICE.md) for the built-in DNS server configuration and the list of hostnames to redirect.

**Step 2 — Connect the speaker to Wi-Fi.**
Use the speaker's built-in AP mode or BLE setup flow. See [Device Initial Setup](DEVICE-INITIAL-SETUP.md) for factory reset button sequences and Wi-Fi provisioning.

**Step 3 — Start the service and open the web UI** at `http://<server>:8000`.

**Step 4 — Add the speaker.**
After connecting to Wi-Fi, the speaker should appear in the web UI automatically (or add it manually by IP). If DNS redirect is already in place, the speaker is already communicating with AfterTouch.

**Step 5 — Migrate** (if not already using DNS redirect).
If you didn't set up DNS first, use the XML redirect method from the web UI to update the speaker's service URLs. The web UI walks you through the steps including CA certificate setup.

**Step 6 — Reboot the speaker.**
Power-cycle to ensure all changes take effect.

---

## After migration

Once migrated, your speaker uses the local service for music browsing, preset sync, and device registration. The web UI at `http://<server>:8000` is your management interface going forward. Back up the `data/` directory periodically in case you need to restore.

For the complete step-by-step walkthrough with commands and troubleshooting, see the [Migration Guide](MIGRATION-GUIDE.md). For safety measures and rollback options, see the [Migration & Safety Guide](MIGRATION-SAFETY.md).
