---
title: "On-Device Install Walkthrough"
---
A complete end-to-end runbook for installing AfterTouch directly on a
Bose SoundTouch speaker — from first SSH connection through verified
radio preset playback.

**Credit:** This guide is based on a step-by-step walkthrough contributed
by [weissigera](https://github.com/weissigera) in
[issue #329](https://github.com/gesellix/Bose-SoundTouch/issues/329#issuecomment-4521280831),
documenting a successful fresh installation on a SoundTouch 20 Series I.

---

## Prerequisites

- SSH enabled on the speaker (the usual "Stick with remote_services" procedure).
- Your machine can reach the speaker on the LAN.
- The speaker's LAN IP address — replace `192.0.2.1` throughout with the
  actual address shown in your router or `arp -a`.

> **Note on SSH host-key negotiation:** SoundTouch speakers only advertise
> legacy host-key algorithms (`ssh-rsa`, `ssh-dss`). Modern OpenSSH clients
> reject these by default. The `-oHostKeyAlgorithms=+ssh-rsa` flag below
> opts them back in. Without it you'll see
> `no matching host key type found`.

---

## Step 1 — Connect to the speaker via SSH

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa root@192.0.2.1
```

You should see a prompt such as `root@soundtouch-device:~#`.

---

## Step 2 — Check free space (and clean up if needed)

The persistent `/mnt/nv` partition typically has 20–40 MB free — enough for
the AfterTouch binary (~12 MB) plus one backup. Check first:

```bash
rw            # remount rootfs read-write
df -h /mnt/nv
```

If you have an older installation with multiple backup or artefact files left
behind by earlier upgrades, remove them:

```bash
# List what's there
ls -lh /mnt/nv/aftertouch/

# Remove specific stale files (adjust version numbers to what you see)
rm -f /mnt/nv/aftertouch/aftertouch-service.v0.80.1.backup
rm -f /mnt/nv/aftertouch/aftertouch-service.v0.86.0.backup
rm -f /mnt/nv/aftertouch/aftertouch-service.v0.86.0.old
rm -f /mnt/nv/aftertouch/aftertouch-service.new
rm -f /mnt/nv/soundtouch-cli        # cli binary if left there by hand
rm -f /mnt/nv/aftertouch/soundtouch-cli

df -h /mnt/nv   # confirm space recovered
```

> **From v0.89.0 onwards the installer prunes stale artefacts automatically**
> during every upgrade — manual cleanup should no longer be necessary on
> fresh installs.

---

## Step 3 — Install (or upgrade) AfterTouch

Run the canonical one-liner. It downloads the binary and init script,
creates `/mnt/nv/aftertouch/`, symlinks `/opt/aftertouch`, backs up the
currently running binary, and starts the service:

```bash
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh
```

To target a specific version instead of the default:

```bash
# Via environment variable (works with pipe-to-sh)
VERSION=0.111.3 rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh

# Via command-line flag (pass args after sh -s --)
curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh -s -- --version 0.111.3
```

Verify the installed version:

```bash
wget -qO- http://localhost:8000/health
```

The JSON response should include `"version":"v0.111.3"` (or whichever
version you installed).

---

## Step 4 — Reboot the speaker

```bash
sync
reboot
```

Wait 2–3 minutes for the speaker to come back up, then reconnect:

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa root@192.0.2.1
```

---

## Step 5 — Open an SSH tunnel and access the Admin UI

**Open a new terminal on your machine** (not inside the speaker's SSH
session — see [issue #250](https://github.com/gesellix/Bose-SoundTouch/issues/250)
for the port-forward-from-inside trap) and run:

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa -L 8000:localhost:8000 root@192.0.2.1
```

Keep this terminal open. Navigate to **http://localhost:8000** in your
browser.

> Skip this step if your speaker's firmware exposes port 8000 on the LAN
> directly — you can reach `http://192.0.2.1:8000` without a tunnel in that
> case.

---

## Step 6 — Run the Health QuickFix for empty `margeAccountUUID`

In the AfterTouch UI:

1. Open the **Health** tab.
2. Run or refresh the health checks.
3. Look for the warning:
   > *Speaker reports an empty `<margeAccountUUID>`*
4. Click the **QuickFix** button (labelled "Fix", "Pair account", or
   "Apply QuickFix" depending on the version) and confirm.

Then reboot again to let the pairing take effect:

```bash
sync
reboot
```

---

## Step 7 — Verify pairing and sources

After the reboot reconnect via SSH and check:

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa root@192.0.2.1

# margeAccountUUID must NOT be empty after the QuickFix
wget -qO- http://localhost:8090/info | grep margeAccountUUID

# Sources must include LOCAL_INTERNET_RADIO, TUNEIN, and RADIO_BROWSER
wget -qO- http://localhost:8090/sources
```

If `margeAccountUUID` is still empty, re-run the Health QuickFix (Step 6)
and reboot again.

---

## Step 8 — Download soundtouch-cli (optional, for preset setup)

If you want to program preset buttons from the command line, download the
CLI binary to the speaker's `/tmp` (tmpfs, so it survives only until the
next reboot — which is fine for a one-time setup run):

```bash
cd /tmp

curl -L --fail -o soundtouch-cli \
  https://github.com/gesellix/Bose-SoundTouch/releases/download/v0.111.3/soundtouch-cli-v0.111.3-linux-armv7
chmod +x soundtouch-cli

/tmp/soundtouch-cli --version
```

Replace `v0.111.3` with the version you installed.

---

## Step 9 — Store custom radio streams to preset buttons

Each station must be playing before it can be saved. The `sleep 5` gives
the speaker time to buffer and confirm the stream before storing.

> **Press preset buttons briefly.** A long press on the physical hardware
> overwrites the stored preset.

```bash
# Preset 1 — Hitradio OE3
/tmp/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "http://orf-live.ors-shoutcast.at/oe3-q2a" \
  --name "Hitradio OE3" \
  --service-url "http://localhost:8000"
sleep 5
/tmp/soundtouch-cli --host 127.0.0.1 preset store-current --slot 1

# Preset 2 — Lounge FM
/tmp/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "http://188.138.9.183/digital.mp3" \
  --name "Lounge FM" \
  --service-url "http://localhost:8000"
sleep 5
/tmp/soundtouch-cli --host 127.0.0.1 preset store-current --slot 2

# Preset 3 — Country Nonstop
/tmp/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "https://stream.laut.fm/country-nonstop" \
  --name "Country Nonstop" \
  --service-url "http://localhost:8000"
sleep 5
/tmp/soundtouch-cli --host 127.0.0.1 preset store-current --slot 3

# Preset 4 — Radio Piterpan
/tmp/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "https://klasse1.fluidstream.eu/piterpan.mp3?FLID=8" \
  --name "Radio Piterpan" \
  --service-url "http://localhost:8000"
sleep 5
/tmp/soundtouch-cli --host 127.0.0.1 preset store-current --slot 4

# Preset 5 — kronehit
/tmp/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "https://secureonair.krone.at/kronehit-hp.mp3" \
  --name "kronehit" \
  --service-url "http://localhost:8000"
sleep 5
/tmp/soundtouch-cli --host 127.0.0.1 preset store-current --slot 5

# Preset 6 — Radio Niederösterreich
/tmp/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "http://orf-live.ors-shoutcast.at/noe-q2a" \
  --name "Radio Niederoesterreich" \
  --service-url "http://localhost:8000"
sleep 5
/tmp/soundtouch-cli --host 127.0.0.1 preset store-current --slot 6
```

These are the stations from weissigera's setup (Austrian public and
internet radio). Replace any or all of them with your own streams — the
pattern is the same regardless of station.

---

## Step 10 — Verify presets and final reboot

```bash
wget -qO- http://localhost:8090/presets
```

You should see all six preset slots populated. Then do a final reboot and
test the physical buttons:

```bash
sync
reboot
```

After the speaker comes back up, press preset buttons 1–6 briefly — each
should start playing the corresponding stream.

---

## Troubleshooting

| Symptom                                              | First check                                         |
|------------------------------------------------------|-----------------------------------------------------|
| SSH "no matching host key type"                      | Add `-oHostKeyAlgorithms=+ssh-rsa`                  |
| Port 8000 not reachable from LAN                     | Use the SSH tunnel (Step 5)                         |
| `margeAccountUUID` still empty after reboot          | Re-run Health QuickFix, reboot again                |
| Radio source error 1005                              | `margeAccountUUID` is empty — complete Step 6 first |
| `http://localhost:8000` not responding after install | `logread \| grep aftertouch \| tail -20`            |
| No space left on device during install               | Run the cleanup in Step 2; check `df -h /mnt/nv`    |

For more detail see [TROUBLESHOOTING.md](TROUBLESHOOTING.md).

---

## Updating AfterTouch

Re-run the installer with the version you want. The script backs up the
running binary (named after its version), installs the new one, and prunes
older artefacts to keep `/mnt/nv` free:

```bash
# Update to latest release
rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh

# Update to a specific version — three equivalent forms
VERSION=0.111.3 rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh

rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh -s -- --version 0.111.3

curl -sSLo install.sh https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh
sh install.sh --version 0.111.3
```

**Rollback:** the installer keeps a `.backup` file alongside the binary:

```bash
ls /mnt/nv/aftertouch/aftertouch-service*.backup
cp /mnt/nv/aftertouch/aftertouch-service.<old-version>.backup \
   /mnt/nv/aftertouch/aftertouch-service
/etc/init.d/aftertouch restart
```

---

## Service management

```bash
/etc/init.d/aftertouch start
/etc/init.d/aftertouch stop
/etc/init.d/aftertouch restart
/etc/init.d/aftertouch status   # distinguishes "running + listener up" from "PID alive but listener down"
```

---

## Logs

The daemon writes to BusyBox syslog (tagged `aftertouch`). Disk usage stays
bounded — the syslog ring buffer is in memory:

```bash
logread        | grep aftertouch | tail -20   # recent entries
logread -f     | grep aftertouch              # live tail
```

If the service is running but port 8000 isn't responding, check the syslog
tail first — panics and startup errors appear there.

---

## Uninstalling

Before uninstalling, consider reverting the speaker migration from the
AfterTouch Admin UI so the speaker URL is set back to the Bose cloud (though
neither Bose nor AfterTouch will be reachable once both are removed).

```bash
curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/uninstall.sh | sh
```
