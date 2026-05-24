---
title: "Capture Device Pairing Traffic"
---

# Capture Device Pairing Traffic

Step-by-step runbook for factory-resetting a SoundTouch speaker, pairing it to a Bose cloud account, and capturing every cloud request via mitmproxy. Tested on Apple Silicon Mac.

**Goal:** obtain a full `.mitm` recording of the account-pairing flow (streaming.bose.com) triggered by the official Android app.

---

## Overview

```
Phase 0  Pre-flight checks
Phase 1  Factory reset speaker
Phase 2  Provision speaker Wi-Fi (AP mode, console)
Phase 3  Start mitmproxy + emulator + Frida
Phase 4  Pair speaker via Bose app (adb-driven)
Phase 5  Save & inspect recording
```

The Android emulator does not have Bluetooth, so the standard BLE setup path is unavailable. Instead:

1. Provision Wi-Fi directly over the speaker's AP web server (Phase 2).
2. Once the speaker is on the LAN, the Bose app discovers it via mDNS — no BLE needed.

---

## Phase 0 — Pre-flight

The emulator setup is fully scripted. Run once per machine:

```bash
# Place the Bose APK at scripts/android/bose.apk first (see BOSE-APP-ADB-Emulator.md § 1)
# Run mitmweb once to generate the CA: mitmweb --listen-port 8080 (Ctrl-C after it starts)

scripts/android/setup-mitm-avd.sh
```

This installs the system image, creates an AVD named `bose-mitm`, installs the
mitmproxy cert and Bose APK, and saves an emulator snapshot `mitm-ready` — so
subsequent sessions never repeat the cert/reboot cycle.

For subsequent sessions, Phase 3 below is replaced by a single command:

```bash
scripts/android/start-mitm-session.sh
```

Manual steps are only needed if you want to understand the internals; see
[BOSE-APP-ADB-Emulator.md](../analysis/BOSE-APP-ADB-Emulator.md) for the
full manual walkthrough.

---

## Phase 1 — Factory Reset Speaker

Perform the reset for your model (see
[DEVICE-INITIAL-SETUP.md § 5](DEVICE-INITIAL-SETUP.md) for full table):

| Model                       | Sequence                                                                |
|-----------------------------|-------------------------------------------------------------------------|
| SoundTouch 10               | Power on; hold **Preset 1** + **Vol −** ~10 s → solid amber Wi-Fi LED   |
| SoundTouch 20               | Power on; hold **Preset 1** + **Vol −** ~10 s → lights blink L→R, amber |
| SoundTouch 20/30 Series III | Hold **Preset 1** + **Preset 6** ~10 s                                  |
| SoundTouch 300              | Hold **Vol −** ~15 s until light bar blinks                             |

Wait until the white LED sweep / restart animation completes (~30 s). The speaker
is now in setup mode and broadcasting its own Wi-Fi AP.

---

## Phase 2 — Provision Speaker Wi-Fi (AP Mode)

### 2.1 Connect Mac to Speaker AP

```bash
# Find the SSID — use System Settings → Wi-Fi or:
#   sudo wdutil info   (macOS Sequoia+, airport command removed)

# Connect (replace SSID with actual value)
SPEAKER_SSID="Bose SoundTouch XXXX"
networksetup -setairportnetwork en0 "$SPEAKER_SSID"

# Confirm: speaker web UI reachable at 192.0.2.1 (client gets 192.0.2.2)
curl -s --connect-timeout 5 http://192.0.2.1/ | head -3
```

### 2.2 Push Home Wi-Fi Credentials

The setup UI at `http://192.0.2.1/` uses the SoundTouch API on **port 8090**. Push credentials directly:

```bash
HOME_SSID="MyHomeNetwork"
HOME_PASS="MyPassword"

# Optional: trigger a site survey first so the speaker finds your SSID
curl -s -X POST http://192.0.2.1:8090/performWirelessSiteSurvey \
  -H 'Content-Type: text/xml' \
  --data-raw '<PerformWirelessSiteSurvey timeout="5"/>'

curl -s -X POST http://192.0.2.1:8090/addWirelessProfile \
  -H 'Content-Type: text/xml' \
  --data-raw "<AddWirelessProfile><profile ssid=\"${HOME_SSID}\" password=\"${HOME_PASS}\" securityType=\"wpa_or_wpa2\" /></AddWirelessProfile>"
```

Expected response: `<AddWirelessProfileResponse />`

### 2.3 Reconnect Mac to Home Network

```bash
HOME_SSID="MyHomeNetwork"
HOME_PASS="MyPassword"

networksetup -setairportnetwork en0 "$HOME_SSID" "$HOME_PASS"
```

### 2.4 Wait for Speaker to Join LAN

```bash
# Poll mDNS until the speaker appears (~15-30 s)
echo "Waiting for speaker on LAN..."
until dns-sd -B _soundtouch._tcp local 2>&1 | grep -m1 "Add"; do sleep 2; done
echo "Speaker is online"

# Resolve its IP
dns-sd -L "$(dns-sd -B _soundtouch._tcp local 2>&1 | grep Add | awk '{print $7}')" \
  _soundtouch._tcp local 2>&1 | grep -o '[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}\.[0-9]\{1,3\}'
```

Or just scan for open port 8090:

```bash
# Quick scan of your /24 subnet for port 8090
SUBNET="192.168.1"   # adjust to your local subnet
for i in $(seq 1 254); do
  (ping -c1 -W1 ${SUBNET}.$i &>/dev/null && \
   nc -z -w1 ${SUBNET}.$i 8090 2>/dev/null && \
   echo "${SUBNET}.$i") &
done
wait
```

---

## Phase 3 — Start mitmproxy, Emulator, Frida

Run each block in a separate terminal tab.

### 3.1 Start mitmweb (native macOS app)

> **Note:** Docker mitmproxy does not work here — its NAT layer prevents the emulator from reaching it. Use the native macOS app instead (download: `https://downloads.mitmproxy.org/12.2.2/mitmproxy-12.2.2-macos-arm64.tar.gz`).

```bash
CAPTURE="bose-pairing-$(date +%Y%m%d-%H%M%S).mitm"
/Applications/mitmproxy.app/Contents/MacOS/mitmweb \
  --web-host 0.0.0.0 --listen-port 8080 --mode regular \
  --set web_password=bose \
  -w "scripts/android/captures/${CAPTURE}"
# Captures → scripts/android/captures/
# Web UI   → http://127.0.0.1:8081/?token=bose
```

### 3.2 Start Emulator

```bash
~/Library/Android/sdk/emulator/emulator -avd Pixel_6_API33 -writable-system &
echo "Waiting for emulator boot..."
adb wait-for-device
adb -s emulator-5554 wait-for-device shell 'while [[ -z $(getprop sys.boot_completed) ]]; do sleep 1; done'
echo "Emulator ready"
```

Enable root and install certificate (only needed once per emulator session):

```bash
adb -s emulator-5554 root
adb -s emulator-5554 shell avbctl disable-verification
adb -s emulator-5554 reboot
adb -s emulator-5554 wait-for-device
adb -s emulator-5554 root

HASH=$(openssl x509 -inform PEM -subject_hash_old \
  -in ~/.mitmproxy/mitmproxy-ca-cert.pem | head -1)

adb -s emulator-5554 push ~/.mitmproxy/mitmproxy-ca-cert.pem /data/local/tmp/mitmproxy.pem
adb -s emulator-5554 shell su 0 mkdir -p /data/misc/user/0/cacerts-added
adb -s emulator-5554 shell su 0 \
  cp /data/local/tmp/mitmproxy.pem /data/misc/user/0/cacerts-added/${HASH}.0
adb -s emulator-5554 shell su 0 \
  chmod 644 /data/misc/user/0/cacerts-added/${HASH}.0
```

Set proxy to Mac IP:

```bash
MAC_IP=$(ipconfig getifaddr en0)
adb -s emulator-5554 shell settings put global http_proxy "${MAC_IP}:8080"
echo "Proxy set to ${MAC_IP}:8080"
```

Confirm emulator can reach the speaker:

```bash
SPEAKER_IP=192.0.2.50   # adjust to your speaker's LAN IP
adb -s emulator-5554 shell ping -c 3 "$SPEAKER_IP"
```

### 3.3 Start frida-server

```bash
adb -s emulator-5554 push scripts/android/frida-server /data/local/tmp/frida-server
adb -s emulator-5554 shell su 0 chmod 755 /data/local/tmp/frida-server
adb -s emulator-5554 shell su 0 "nohup /data/local/tmp/frida-server > /dev/null 2>&1 &"
sleep 2
echo "frida-server running"
```

### 3.4 Configure config.js

`start-mitm-session.sh` patches `scripts/android/frida/config.js` automatically with the current Mac IP and mitmproxy cert. No manual step needed.

### 3.5 Launch App with SSL Unpinning

```bash
scripts/android/frida-venv/bin/frida \
  -U \
  -f com.bose.soundtouch \
  -l scripts/android/frida/config.js \
  -l scripts/android/frida/native-connect-hook.js \
  -l scripts/android/frida/android/android-system-certificate-injection.js \
  -l scripts/android/frida/android/android-proxy-override.js \
  -l scripts/android/frida/android/android-certificate-unpinning.js \
  -l scripts/android/frida/android/android-certificate-unpinning-fallback.js
```

> `native-connect-hook.js` is required — the Bose app uses native networking that bypasses Java proxy settings.

Expected Frida output:

```
== System certificate trust injected ==
== Proxy system configuration overridden to <IP>:8080 ==
== Proxy configuration overridden to <IP>:8080 ==
== Certificate unpinning completed ==
== Unpinning fallback auto-patcher installed ==
```

---

## Phase 4 — Pair Speaker via App

The Bose app should now be running in the emulator with all traffic going through mitmproxy.

### 4.1 Inspect UI to Find Interactive Elements

```bash
# Dump current screen
adb -s emulator-5554 shell uiautomator dump /sdcard/ui.xml
adb -s emulator-5554 pull /sdcard/ui.xml /tmp/ui.xml

# Helper: list all clickable elements with their text + bounds
grep -o 'text="[^"]*" resource-id="[^"]*" \.\.\. clickable="true"[^/]*' /tmp/ui.xml \
  || python3 -c "
import xml.etree.ElementTree as ET
tree = ET.parse('/tmp/ui.xml')
for n in tree.iter('node'):
    if n.get('clickable') == 'true' and n.get('text'):
        print(n.get('bounds'), n.get('resource-id'), repr(n.get('text')))
"
```

### 4.2 Navigate Setup Flow (adb)

```bash
# Take a screenshot at any point to see current state
adb -s emulator-5554 shell screencap /sdcard/screen.png
adb -s emulator-5554 pull /sdcard/screen.png /tmp/screen.png
open /tmp/screen.png

# Tap by resource-id (find IDs from ui.xml dump)
adb -s emulator-5554 shell uiautomator runtest ... # or input tap

# Tap by screen coordinates
adb -s emulator-5554 shell input tap X Y

# Type into the focused field
adb -s emulator-5554 shell input text "your@email.com"

# Press Enter / Next
adb -s emulator-5554 shell input keyevent 66
```

### 4.3 Expected Setup Steps in the App

Follow the on-screen flow; mitmproxy captures everything automatically.

1. **Sign in** — enter email + password → triggers `POST /streaming/account/login`
2. **Add speaker** — tap "Set Up a New Speaker" or equivalent
3. **App discovers speaker** via mDNS on LAN (no BLE required)
4. **Wi-Fi already configured** — app skips the Wi-Fi step since speaker is online
5. **Name speaker** — type a name → WebSocket `name` message to speaker port 8080
6. **Pairing** — app sends `setMargeAccount` WebSocket to speaker → speaker POSTs to `streaming.bose.com/{accountId}/devices`

All cloud requests (steps 1, 6) will appear in mitmweb at `http://127.0.0.1:8081`.

---

## Phase 5 — Save & Inspect Recording

```bash
# Stop mitmweb (Ctrl-C in its terminal) — file is already written continuously

# Inspect offline in the web UI
mitmweb -r "$CAPTURE"

# Filter to streaming.bose.com only
mitmdump -r "$CAPTURE" --flow-filter '~u streaming.bose.com' -w bose-cloud-only.mitm

# Quick text summary
mitmdump -r "$CAPTURE" --flow-filter '~u streaming.bose.com' 2>/dev/null \
  | grep -E "POST|GET" | head -30
```

### Convert to .http files (IntelliJ-compatible)

Use `scripts/convert_mitm_script.py` to extract each flow as a `.http` file, organized by path:

```bash
NAME=$(basename "$CAPTURE" .mitm)
OUT="scripts/android/mitm/${NAME}"

/Applications/mitmproxy.app/Contents/MacOS/mitmdump \
  -n -r "$CAPTURE" \
  -s scripts/convert_mitm_script.py \
  --set out_dir="${OUT}"
```

Output lands in `scripts/android/mitm/<name>/mirror/` as numbered `.http` files
plus `*-websocket/` subdirectories for WebSocket frames. The directory is gitignored.

---

## Cleanup

```bash
# Remove proxy from emulator
adb -s emulator-5554 shell settings delete global http_proxy

# Kill emulator
adb -s emulator-5554 emu kill
```

Frida artefacts live in `scripts/android/` (gitignored) and persist between sessions — no cleanup needed unless you want to force a fresh setup.

---

## Troubleshooting

| Symptom                             | Likely cause                                     | Fix                                                                               |
|-------------------------------------|--------------------------------------------------|-----------------------------------------------------------------------------------|
| `curl http://192.0.2.1` times out   | Mac not on speaker AP                            | Re-run `networksetup -setairportnetwork`; speaker AP gateway is `192.0.2.1`       |
| `addWirelessProfile` returns error  | Speaker not in AP mode or wrong IP               | Confirm speaker AP is active; use `http://192.0.2.1:8090/addWirelessProfile`      |
| Speaker not found via mDNS          | Speaker still on AP (not home LAN yet)           | Wait ~30 s, retry; check router DHCP leases                                       |
| Emulator can't ping speaker         | Different subnet or emulator proxy misconfigured | `adb shell ping` the Mac IP first; check proxy setting                            |
| App shows "No speakers found"       | App not detecting mDNS                           | Ensure emulator is on same `/24` as speaker; disable emulator Wi-Fi and re-enable |
| No traffic in mitmweb               | Frida not running or cert mismatch               | Check Frida output for `== Certificate unpinning ==`; verify issuer in config.js  |

## See Also

- [BOSE-APP-ADB-Emulator.md](../analysis/BOSE-APP-ADB-Emulator.md) — full MITM + Frida setup
- [DEVICE-INITIAL-SETUP.md](DEVICE-INITIAL-SETUP.md) — factory reset sequences + AP mode detail
- [DEVICE-SETUP.md](../appendix/DEVICE-SETUP.md) — WebSocket and cloud pairing protocol reference

---

## Session Trace (2026-05-02, ST10)

Raw log of the first interactive run. To be cleaned up into the runbook above.

### Setup
- Ran `scripts/android/setup-mitm-avd.sh` — all 9 steps completed:
  - Steps 1–5 were already done from a prior session (idempotent skips)
  - Step 6: Docker image built, `frida-server` extracted to `scripts/android/frida-server`
  - Steps 7–9: Emulator started fresh (`-no-snapshot-load`), AVB disabled, rebooted, cert + APK + frida-server installed, snapshot `mitm-ready` saved
- Emulator running at `emulator-5554`

### Factory Reset (ST10)
- Correct sequence confirmed from official Bose guide (`firmware/FirmwareUpdateGuide/`):
  **Power on → hold Preset 1 + Volume − for 10 s → Wi-Fi LED glows solid amber**
- Note: original docs said "Vol− + Mute" — corrected in DEVICE-INITIAL-SETUP.md and this file

### Pre-reset note
- User inserted USB device containing `remote_services` before rebooting — device needs to be on home Wi-Fi before the USB config takes effect

### Wi-Fi Provisioning (AP mode)
- `airport` command not available on this macOS version (removed in recent releases)
- Connect Mac to speaker AP via **System Settings → Wi-Fi** (SSID: "Bose SoundTouch XXXX")
- Speaker AP gateway confirmed: `192.0.2.1` (client gets `192.0.2.2`), not `192.0.2.1` as previously assumed
- `/gabbo_wifi` endpoint was hallucinated — actual endpoint verified from browser network capture (`_/device-reset/wifi-setup.txt`):
  - Site survey: `POST http://192.0.2.1:8090/performWirelessSiteSurvey` with `<PerformWirelessSiteSurvey timeout="5"/>`
  - Add profile: `POST http://192.0.2.1:8090/addWirelessProfile` with XML body, `securityType="wpa_or_wpa2"`
  - Both use the standard SoundTouch API port 8090, same as normal device operation
  - Response: `<AddWirelessProfileResponse />`
- Reconnect Mac to home Wi-Fi; emulator stays running throughout (routes through Mac interface)

### Wi-Fi Provisioning Result
- `POST http://192.0.2.1:8090/addWirelessProfile` succeeded → `<AddWirelessProfileResponse />`
- Speaker joined home LAN at `192.168.x.y`
- SSH access confirmed: `ssh -oHostKeyAlgorithms=ssh-rsa root@192.168.x.y`
- Device name: `Bose SoundTouch XXXXXX`
- Network interfaces on device:
  - `wlan0` → `192.168.x.y` (home LAN)
  - `wlan1` → `192.0.2.1` (AP mode interface, stays up after provisioning)
  - `usb0` → `203.0.113.1` (USB gadget — remote_services USB device inserted before reboot)

### MITM Session Start
- `start-mitm-session.sh` run: snapshot restored, proxy set, frida-server started, config.js patched
- Issues encountered and fixed:
  - frida-server start command hung (`adb shell su 0 ... &`) → fixed with `nohup ... > /dev/null 2>&1 &`
  - Frida SSL scripts download via `curl` from GitHub timed out → moved to Dockerfile (extracted alongside frida-server)
  - Python heredoc cert injection syntax error (multiline cert in string) → fixed using env vars + single-quoted `'PYEOF'` heredoc
  - `mitmweb --port` deprecated → `--listen-port`
  - frida script paths missing `android/` subdirectory → fixed in session script
- Docker mitmproxy (`-p 8080:8080`) received no traffic from emulator — Docker NAT layer prevented the emulator reaching it
- **Fix: use native macOS mitmproxy app** (`/Users/gesellix/Downloads/mitmproxy.app`) — binds directly to Mac's real network interfaces, traffic flows immediately
- Android system traffic visible (connectivity checks to gstatic.com, www.google.com) — TLS failures for system processes expected since only Bose app has Frida cert injection

### Capture Working
- Full flow confirmed working with:
  - Native mitmproxy app (`~/Downloads/mitmproxy.app`, v12.2.2)
  - `native-connect-hook.js` added to Frida launch (required for Bose app's native networking)
  - All 5 Frida scripts loaded: config.js, native-connect-hook.js, android-system-certificate-injection.js, android-proxy-override.js, android-certificate-unpinning.js, android-certificate-unpinning-fallback.js
- Bose app traffic visible in mitmweb — pairing flow captured successfully
