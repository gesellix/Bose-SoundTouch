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

- SSH enabled on the speaker — either the usual "USB stick with
  `remote_services`" procedure, or `soundtouch-cli setup enable-ssh`
  (no stick needed, see Step 1).
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

If SSH isn't enabled yet, you don't need a USB stick: `soundtouch-cli` can
bootstrap it purely over the network (#471), using the speaker's
telnet:17000 diagnostic shell (open by default on most firmware) to inject
the SSH-enable command:

```bash
soundtouch-cli --host 192.0.2.1 setup enable-ssh
```

This waits for `:22` to come up and persists it (survives a reboot) by
default. The USB-stick method (format FAT32, create an empty
`remote_services` file in its root, insert, power-cycle) still works as a
fallback if telnet:17000 is closed or the injection doesn't take on your
model.

Either way, connect the same way:

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

By default this installs the **latest release** — the script resolves it from
GitHub's `releases/latest` redirect. To target a specific version instead:

```bash
# Via environment variable (works with pipe-to-sh)
VERSION=0.123.0 rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh

# Via command-line flag (pass args after sh -s --)
curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh -s -- --version 0.123.0
```

Verify the installed version:

```bash
wget -qO- http://localhost:8000/health
```

The JSON response should include `"version":"v0.123.0"` (or whichever
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

## Step 6 — Migrate (point the speaker at itself)

The speaker isn't pointed at the AfterTouch instance you just installed yet
— this step does that. On-device, the speaker and the AfterTouch instance
are the same machine, so **loopback is the correct and recommended Target
Domain value**: `http://localhost:8000`. This is the one case where the
general migration guide's "must not be `localhost`" warning does not
apply — that warning is about the external-host/cloud scenarios, where
`localhost` would resolve on the wrong machine (the service host, not the
speaker). Here there is no wrong machine to resolve on.

**Via the Admin UI:**

1. Go to **Settings**, set **Target Domain** to `http://localhost:8000`.
2. Go to **Devices**, find your speaker (it self-discovers on its own LAN
   IP), click **Migrate**.
3. Accept the suggested plan and let it apply.
4. Reboot to apply the change:
   ```bash
   sync
   reboot
   ```

**Or via the CLI** (equivalent, no browser needed — grab `soundtouch-cli`
from Step 9 below first if you want this path):

```bash
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 setup migrate \
  --service-url http://localhost:8000 --method telnet
sync
reboot
```

---

## Step 7 — Run the Health QuickFix for empty `margeAccountUUID`

In the AfterTouch UI:

1. Open the **Health** tab.
2. Run or refresh the health checks.
3. Look for the warning:
   > *Speaker reports an empty `<margeAccountUUID>`*
4. Click the **QuickFix** button (labelled "Fix", "Pair account", or
   "Apply QuickFix" depending on the version) and confirm.

Or via the CLI (same underlying pairing call, `--mode=bare` matches what
the QuickFix does — see Step 9 to grab `soundtouch-cli` first):

```bash
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 setup pair \
  --mode=bare --account=1111111 --service-url http://localhost:8000
```

Then reboot again to let the pairing take effect:

```bash
sync
reboot
```

---

## Step 8 — Verify pairing and sources

After the reboot reconnect via SSH and check:

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa root@192.0.2.1

# margeAccountUUID must NOT be empty after the QuickFix
wget -qO- http://localhost:8090/info | grep margeAccountUUID

# Sources must include LOCAL_INTERNET_RADIO, TUNEIN, and RADIO_BROWSER
wget -qO- http://localhost:8090/sources
```

If `margeAccountUUID` is still empty, re-run the Health QuickFix (Step 7)
and reboot again.

---

## Step 9 — Download soundtouch-cli (optional, for preset setup)

If you want to program preset buttons from the command line, download the
CLI binary to `/mnt/nv/aftertouch` (the same persistent partition
AfterTouch itself lives on) rather than `/tmp`: `/tmp` is tmpfs and gets
wiped on every reboot, and if you used the CLI alternatives in Steps 6/7
above, it needs to survive those steps' reboots too, not just the final
one:

```bash
cd /mnt/nv/aftertouch

curl -L --fail -o soundtouch-cli \
  https://github.com/gesellix/Bose-SoundTouch/releases/download/v0.123.0/soundtouch-cli-v0.123.0-linux-armv7
chmod +x soundtouch-cli

/mnt/nv/aftertouch/soundtouch-cli --version
```

Replace `v0.123.0` with the version you installed. If you want the CLI
alternatives in Steps 6/7, download it here first, before doing those
steps — it'll be in place and already persistent either way.

---

## Step 10 — Store custom radio streams to preset buttons

Each station must be playing before it can be saved. The `sleep 5` gives
the speaker time to buffer and confirm the stream before storing.

> **Press preset buttons briefly.** A long press on the physical hardware
> overwrites the stored preset.

```bash
# Preset 1 — Hitradio OE3
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "http://orf-live.ors-shoutcast.at/oe3-q2a" \
  --name "Hitradio OE3" \
  --service-url "http://localhost:8000"
sleep 5
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 preset store-current --slot 1

# Preset 2 — Lounge FM
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "http://188.138.9.183/digital.mp3" \
  --name "Lounge FM" \
  --service-url "http://localhost:8000"
sleep 5
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 preset store-current --slot 2

# Preset 3 — Country Nonstop
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "https://stream.laut.fm/country-nonstop" \
  --name "Country Nonstop" \
  --service-url "http://localhost:8000"
sleep 5
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 preset store-current --slot 3

# Preset 4 — Radio Piterpan
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "https://klasse1.fluidstream.eu/piterpan.mp3?FLID=8" \
  --name "Radio Piterpan" \
  --service-url "http://localhost:8000"
sleep 5
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 preset store-current --slot 4

# Preset 5 — kronehit
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "https://secureonair.krone.at/kronehit-hp.mp3" \
  --name "kronehit" \
  --service-url "http://localhost:8000"
sleep 5
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 preset store-current --slot 5

# Preset 6 — Radio Niederösterreich
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 source custom-radio \
  --url "http://orf-live.ors-shoutcast.at/noe-q2a" \
  --name "Radio Niederoesterreich" \
  --service-url "http://localhost:8000"
sleep 5
/mnt/nv/aftertouch/soundtouch-cli --host 127.0.0.1 preset store-current --slot 6
```

These are the stations from weissigera's setup (Austrian public and
internet radio). Replace any or all of them with your own streams — the
pattern is the same regardless of station.

---

## Step 11 — Verify presets and final reboot

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
| Radio source error 1005                              | `margeAccountUUID` is empty — complete Step 7 first |
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
VERSION=0.123.0 rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh

rw && curl -sSL https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh | sh -s -- --version 0.123.0

curl -sSLo install.sh https://raw.githubusercontent.com/gesellix/Bose-SoundTouch/main/scripts/on-device-install/install.sh
sh install.sh --version 0.123.0
```

**Rollback:** the installer keeps a `.backup` file alongside the binary:

```bash
ls /mnt/nv/aftertouch/aftertouch-service*.backup
cp /mnt/nv/aftertouch/aftertouch-service.<old-version>.backup \
   /mnt/nv/aftertouch/aftertouch-service
/etc/init.d/aftertouch restart
```

**Testing a pre-release build (from `main`, not yet tagged):** `install.sh`
only ever downloads from GitHub Releases, so there's no one-line installer
for an unreleased commit. Cross-compile and swap the binary manually
instead — this is a direct extension of the rollback procedure above:

```bash
# On your own machine, from a checkout of the branch/commit you want:
make build-linux-armv7   # builds build/soundtouch-service-linux-armv7,
                          # build/soundtouch-cli-linux-armv7, and
                          # build/soundtouch-backup-linux-armv7

scp build/soundtouch-service-linux-armv7 root@192.0.2.1:/mnt/nv/aftertouch/aftertouch-service.new
ssh -oHostKeyAlgorithms=+ssh-rsa root@192.0.2.1

rw
/etc/init.d/aftertouch stop
cp /mnt/nv/aftertouch/aftertouch-service /mnt/nv/aftertouch/aftertouch-service.pre-test.backup
mv /mnt/nv/aftertouch/aftertouch-service.new /mnt/nv/aftertouch/aftertouch-service
chmod +x /mnt/nv/aftertouch/aftertouch-service
/etc/init.d/aftertouch start
```

If you're testing an unreleased `soundtouch-cli` change (not just the
service), swap that binary too — same idea, and it lands in the same
`/mnt/nv/aftertouch` directory Step 9 above uses:

```bash
scp build/soundtouch-cli-linux-armv7 root@192.0.2.1:/mnt/nv/aftertouch/soundtouch-cli
ssh -oHostKeyAlgorithms=+ssh-rsa root@192.0.2.1 chmod +x /mnt/nv/aftertouch/soundtouch-cli
```

Roll back the same way as above, using the `.pre-test.backup` file.

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
