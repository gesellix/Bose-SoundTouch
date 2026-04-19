# Bose SoundTouch Traffic Interception Runbook

Intercept HTTPS/WebSocket traffic from the Bose SoundTouch Android app using an Android emulator, mitmproxy, and Frida. Tested on Apple Silicon (ARM64) Mac.

## Prerequisites

- Android Studio installed (for SDK tools and emulator)
- Docker installed
- mitmproxy installed (`pip install mitmproxy` or via your preferred method)
- The Bose SoundTouch APK (extracted from a real device, see below)

Add Android SDK tools to your PATH (add to `~/.zshrc`):

```bash
export PATH=$PATH:~/Library/Android/sdk/emulator
export PATH=$PATH:~/Library/Android/sdk/platform-tools
```

---

## 1. Extract APK from Real Device

Connect your Android device via USB with USB debugging enabled.

```bash
adb devices
# note your device ID, e.g. "ABC123"

adb -s ABC123 shell pm path com.bose.soundtouch
# output e.g.: package:/data/app/~~xyz/com.bose.soundtouch-abc/base.apk

adb -s ABC123 pull /data/app/~~xyz/com.bose.soundtouch-abc/base.apk bose.apk
```

---

## 2. Create Android Emulator (ARM64, API 33)

On Apple Silicon you need an ARM64 image. Use the `avdmanager` and `sdkmanager` CLI tools.

```bash
# Install the system image
~/Library/Android/sdk/cmdline-tools/latest/bin/sdkmanager \
  "system-images;android-33;google_apis;arm64-v8a"

# Create the AVD
~/Library/Android/sdk/cmdline-tools/latest/bin/avdmanager create avd \
  -n Pixel_6_API33 \
  -k "system-images;android-33;google_apis;arm64-v8a" \
  -d "pixel_6"
```

Alternatively create the AVD via Android Studio Device Manager (choose "Google APIs", arm64-v8a, API 33).

---

## 3. Start Emulator with Writable System

```bash
# List available AVDs
~/Library/Android/sdk/emulator/emulator -list-avds

# Start with writable system partition
~/Library/Android/sdk/emulator/emulator -avd Pixel_6_API33 -writable-system
```

Wait until the emulator has fully booted, then:

```bash
adb -s emulator-5554 root
adb -s emulator-5554 shell avbctl disable-verification
adb -s emulator-5554 reboot

# After reboot:
adb -s emulator-5554 root
```

---

## 4. Install Bose APK

```bash
adb -s emulator-5554 install bose.apk
```

---

## 5. Set Up mitmproxy

```bash
# Start mitmproxy (generates CA cert on first run)
mitmweb --port 8080 --mode regular -w bose_traffic.mitm
```

Extract the CA certificate (without private key):

```bash
openssl x509 -in ~/.mitmproxy/mitmproxy-ca.pem -out ~/.mitmproxy/mitmproxy-ca-cert.pem

# Verify it's the mitmproxy cert, not another cert:
openssl x509 -in ~/.mitmproxy/mitmproxy-ca-cert.pem -noout -issuer
# should show: issuer= /CN=mitmproxy/O=mitmproxy
```

---

## 6. Install mitmproxy CA Certificate in Emulator

```bash
HASH=$(openssl x509 -inform PEM -subject_hash_old \
  -in ~/.mitmproxy/mitmproxy-ca-cert.pem | head -1)

adb -s emulator-5554 push ~/.mitmproxy/mitmproxy-ca-cert.pem /data/local/tmp/mitmproxy.pem

adb -s emulator-5554 shell su 0 mkdir -p /data/misc/user/0/cacerts-added

adb -s emulator-5554 shell su 0 \
  cp /data/local/tmp/mitmproxy.pem /data/misc/user/0/cacerts-added/${HASH}.0

adb -s emulator-5554 shell su 0 \
  chmod 644 /data/misc/user/0/cacerts-added/${HASH}.0
```

---

## 7. Set System Proxy in Emulator

Find your Mac's local IP:

```bash
ipconfig getifaddr en0
# e.g. 192.168.1.123
```

Set the proxy:

```bash
adb -s emulator-5554 shell settings put global http_proxy 192.168.1.123:8080
```

---

## 8. Set Up Frida (via Python venv)

```bash
python3 -m venv /tmp/frida-venv
/tmp/frida-venv/bin/pip install frida==17.9.1 frida-tools==14.8.1
```

Download the frida-server binary for ARM64 Android:

```bash
FRIDA_VERSION=17.9.1

curl -L "https://github.com/frida/frida/releases/download/${FRIDA_VERSION}/frida-server-${FRIDA_VERSION}-android-arm64.xz" \
  -o /tmp/frida-server.xz

unxz /tmp/frida-server.xz
mv /tmp/frida-server-${FRIDA_VERSION}-android-arm64 /tmp/frida-server
```

Push to emulator and start:

```bash
adb -s emulator-5554 push /tmp/frida-server /data/local/tmp/frida-server
adb -s emulator-5554 shell su 0 chmod 755 /data/local/tmp/frida-server
adb -s emulator-5554 shell su 0 /data/local/tmp/frida-server &
```

---

## 9. Download SSL Bypass Scripts

```bash
BASE=https://raw.githubusercontent.com/httptoolkit/frida-interception-and-unpinning/main

curl -L "${BASE}/config.js" -o /tmp/config.js
curl -L "${BASE}/android/android-system-certificate-injection.js" \
  -o /tmp/android-system-certificate-injection.js
curl -L "${BASE}/android/android-proxy-override.js" \
  -o /tmp/android-proxy-override.js
curl -L "${BASE}/android/android-certificate-unpinning.js" \
  -o /tmp/android-certificate-unpinning.js
curl -L "${BASE}/android/android-certificate-unpinning-fallback.js" \
  -o /tmp/android-certificate-unpinning-fallback.js
```

---

## 10. Configure config.js

Edit `/tmp/config.js` and set:

```javascript
const CERT_PEM = `<contents of ~/.mitmproxy/mitmproxy-ca-cert.pem>`;

const PROXY_HOST = '192.168.1.123';  // your Mac IP
const PROXY_PORT = 8080;
```

Insert the full PEM content (from `-----BEGIN CERTIFICATE-----` to `-----END CERTIFICATE-----`) between the backticks.

Quick check that the right cert is in place:

```bash
# The issuer inside config.js should be mitmproxy, not SoundTouch
grep -A3 "CERT_PEM" /tmp/config.js | head -5
```

---

## 11. Start Interception

Make sure mitmweb is running, then:

```bash
/tmp/frida-venv/bin/frida \
  -U \
  -f com.bose.soundtouch \
  -l /tmp/config.js \
  -l /tmp/android-system-certificate-injection.js \
  -l /tmp/android-proxy-override.js \
  -l /tmp/android-certificate-unpinning.js \
  -l /tmp/android-certificate-unpinning-fallback.js
```

Expected output in the Frida REPL:

```
== System certificate trust injected ==
== Proxy system configuration overridden to 192.168.1.123:8080 ==
== Proxy configuration overridden to 192.168.1.123:8080 ==
== Certificate unpinning completed ==
== Unpinning fallback auto-patcher installed ==
```

Open mitmweb at `http://127.0.0.1:8081` to observe traffic live.

---

## 12. Save & Replay Recordings

Traffic is saved to `bose_traffic.mitm` (set via `-w` flag in step 5).

```bash
# Replay/analyse a saved recording:
mitmweb -r bose_traffic.mitm
```

---

## Cleanup

```bash
# Remove proxy setting from emulator
adb -s emulator-5554 shell settings delete global http_proxy

# Remove venv
rm -rf /tmp/frida-venv /tmp/frida-server /tmp/frida-server.xz
rm /tmp/config.js /tmp/android-*.js

# Stop emulator
adb -s emulator-5554 emu kill
```

---

## Troubleshooting

| Symptom                                 | Cause                                            | Fix                                                                            |
|-----------------------------------------|--------------------------------------------------|--------------------------------------------------------------------------------|
| `remount failed`                        | ARM64 emulator doesn't support overlayfs remount | Use `/data/misc/user/0/cacerts-added/` method instead                          |
| `TLS: Trust anchor not found`           | Wrong certificate in config.js                   | Check issuer: must be mitmproxy, not SoundTouch                                |
| `Chain validation failed`               | Private key included in cert                     | Re-extract with `openssl x509 -in mitmproxy-ca.pem -out mitmproxy-ca-cert.pem` |
| `frida-server: connection refused`      | frida-server not running                         | Re-run `adb shell su 0 /data/local/tmp/frida-server &`                         |
| frida and frida-server version mismatch | Versions must be identical                       | Pin both to same version (e.g. `17.9.1`)                                       |
| `emulator: multiple AVDs` error         | Emulator already running                         | Kill first: `adb emu kill`, then restart with `-writable-system`               |

---

## App Automation Options

For most traffic-recording purposes, manually operating the app while mitmproxy captures is sufficient. If you need to automate specific interactions (e.g. to repeatably capture the requests triggered by startup or a particular action), the following tools are available.

### Starting the App

```bash
# Via app drawer: swipe up on the home screen and tap "Bose SoundTouch"

# Via adb monkey (simplest)
adb -s emulator-5554 shell monkey -p com.bose.soundtouch 1

# Via explicit intent (if the activity name is known)
adb -s emulator-5554 shell am start -n com.bose.soundtouch/.MainActivity

# Look up all activities if the name is unknown
adb -s emulator-5554 shell dumpsys package com.bose.soundtouch | grep Activity
```

### adb — sufficient for simple cases

```bash
# Tap at screen coordinates
adb shell input tap 540 960

# Swipe
adb shell input swipe 540 1500 540 500

# Type text
adb shell input text "mytext"

# Take a screenshot
adb shell screencap /sdcard/screen.png && adb pull /sdcard/screen.png
```

### UIAutomator2 — inspect UI elements

```bash
# Dump the current UI hierarchy to find element IDs
adb shell uiautomator dump /sdcard/ui.xml
adb pull /sdcard/ui.xml
```

Open `ui.xml` to find element resource IDs, then target them precisely in scripts.

### Appium — full scripted automation

```python
from appium import webdriver

driver = webdriver.Remote('http://localhost:4723/wd/hub', {
    'platformName': 'Android',
    'appPackage': 'com.bose.soundtouch',
    'appActivity': '.MainActivity',
})

# Find an element by resource ID and tap it
driver.find_element('id', 'com.bose.soundtouch:id/play_button').click()
```

> **Note:** `monkey` is a stress-test tool that sends random events — use it only to launch the app, not to drive specific interactions.
