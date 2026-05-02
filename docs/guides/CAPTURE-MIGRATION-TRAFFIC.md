# Capture Speaker Migration Traffic

Runbook for migrating a SoundTouch speaker to `soundtouch-service` and capturing
all traffic (App→Service and Speaker→Service) to identify unimplemented endpoints.

**Goal:** obtain a complete picture of every cloud request a speaker and the Bose
app make after migration, so missing endpoint implementations can be tracked down.

**Pre-requisites:** the MITM pipeline is already set up and working. See
[CAPTURE-DEVICE-PAIRING.md](CAPTURE-DEVICE-PAIRING.md) for the one-time AVD setup
and the pairing capture runbook.

---

## Overview

```
Step 1  Start soundtouch-service locally (with interaction recording)
Step 2  Start a fresh mitmproxy + Frida session (captures App traffic)
Step 3  Discover or register the speaker in the service UI
Step 4  Migrate the speaker (modifies SoundTouchSdkPrivateCfg.xml via SSH)
Step 5  Operate the Bose app — everything now flows through local service
Step 6  Inspect captured interactions for unimplemented endpoints
Step 7  Revert (optional) / clean up
```

Traffic sources:
- **App → Service** — captured by mitmproxy + Frida (same as pairing capture)
- **Speaker → Service** — captured by the service's built-in interaction recorder

---

## Step 1 — Start soundtouch-service

Build and start the service. Recording is on by default; add `--server-url` so the
service knows its own public address (the speaker needs it for redirections).

```bash
# Determine Mac LAN IP first
MAC_IP=$(ipconfig getifaddr en0)
echo "Mac IP: ${MAC_IP}"

# Build + run with explicit server-url so the service embeds the correct address
make build-service
./build/soundtouch-service \
  --server-url "http://${MAC_IP}:8000" \
  --record-interactions \
  --log-bodies
```

Service listens on `:8000` by default. Web UI: `http://localhost:8000`

> To also enable mirror mode (forward unhandled requests to official Bose servers
> for comparison), add: `--mirror-enabled --mirror-endpoints /streaming/`

---

## Step 2 — Start mitmproxy + Frida (new capture)

In a separate terminal:

```bash
scripts/android/start-mitm-session.sh
```

The script prints ready-to-run commands for mitmweb and Frida. Run each in its own
terminal tab as instructed.

New capture file lands in `scripts/android/captures/`.

---

## Step 3 — Discover the Speaker

Open the service web UI at `http://localhost:8000`.

The service discovers speakers via mDNS automatically on startup. If the speaker
does not appear within ~30 s, add it manually:

```bash
# Via API (replace IP with speaker's current LAN IP)
curl -s -X POST http://localhost:8000/setup/devices \
  -H 'Content-Type: application/json' \
  -d '{"ip": "192.168.x.y"}'

# Confirm it's registered
curl -s http://localhost:8000/setup/devices | python3 -m json.tool
```

Note the `device_id` from the response — you need it for migration.

```bash
# List all known devices and their IDs
curl -s http://localhost:8000/setup/devices | python3 -m json.tool

# Extract device_id for the speaker by matching its IP
DEVICE_ID=$(curl -s http://localhost:8000/setup/devices \
  | python3 -c "import sys,json; devs=json.load(sys.stdin); \
    [print(d['device_id']) for d in devs if '35' in d.get('ip_address','')]")
echo "Device ID: ${DEVICE_ID}"
```

---

## Step 4 — Migrate the Speaker

The migration modifies `SoundTouchSdkPrivateCfg.xml` on the speaker via SSH,
redirecting `margeServerUrl` (and optionally other service URLs) to the local
service.

### 4.1 Review the Migration Plan

```bash
# Dry-run: see what will be changed
curl -s "http://localhost:8000/setup/summary/${DEVICE_ID}" | python3 -m json.tool
```

Key fields to check:
- `margeServerUrl` — should become `http://<MAC_IP>:8000/streaming`
- `remoteServicesEnabled` — must be `true` for the speaker to make cloud calls
- `is_migrated` — `false` before, `true` after

### 4.2 Run Migration

```bash
MAC_IP=$(ipconfig getifaddr en0)
TARGET_URL="http://${MAC_IP}:8000"

curl -s -X POST \
  "http://localhost:8000/setup/migrate/${DEVICE_ID}" \
  -G --data-urlencode "target_url=${TARGET_URL}" \
  | python3 -m json.tool
```

Expected response: `{"ok": true, "message": "Migration started", "output": "..."}`.
The output field contains the SSH transcript of the changes made.

### 4.3 Reboot the Speaker

A reboot applies the new config:

```bash
curl -s -X POST "http://localhost:8000/setup/reboot/${DEVICE_ID}"
```

Wait ~30 s for the speaker to come back online. Verify it's back:

```bash
dns-sd -B _soundtouch._tcp local 2>&1 | grep Add
# or
curl -s http://192.168.x.y:8090/info | head -5
```

### 4.4 Verify Migration

```bash
# Check migration summary again — is_migrated should now be true
curl -s "http://localhost:8000/setup/summary/${DEVICE_ID}" \
  | python3 -c "import sys,json; s=json.load(sys.stdin); print('migrated:', s.get('is_migrated'))"
```

You should also see incoming connections from the speaker in the service logs once
it resumes normal operation.

---

## Step 5 — Operate the Bose App

With the speaker migrated and Frida running, every app action triggers traffic
through the service:

1. **Sign in** — `POST /streaming/account/login`
2. **Speaker shows as linked** — speaker has called the service to register/sync
3. **Play music** — BMX registry lookup, playback control
4. **Set presets** — `POST /streaming/account/{id}/device/{id}/presets/{n}`
5. **Adjust volume, switch source** — direct speaker API (port 8090, not cloud)
6. **Check "Now Playing"** — speaker WebSocket events + marge sync

For each action, both mitmweb and the service's recorder capture the request.

---

## Step 6 — Inspect Captured Interactions

### 6.1 Service Interaction Recorder

The service records all incoming requests to `data/interactions/` (configurable via
`--data-dir`). Browse them via:

```bash
# List recorded sessions
curl -s http://localhost:8000/setup/interactions | python3 -m json.tool

# Download a session as HAR
curl -s "http://localhost:8000/setup/interactions/sessions/<session>/download" \
  -o session.har

# Find 404/500 responses (unimplemented endpoints)
curl -s "http://localhost:8000/setup/interaction-content" \
  | python3 -c "
import sys, json
for entry in json.load(sys.stdin).get('entries', []):
    status = entry.get('response', {}).get('status', 0)
    if status >= 400:
        print(status, entry.get('request', {}).get('method'), entry.get('request', {}).get('url'))
"
```

### 6.2 mitmproxy Recording

```bash
# Inspect app→service traffic offline
CAPTURE="scripts/android/captures/<filename>.mitm"
mitmweb -r "${CAPTURE}"

# Filter to local service only
mitmdump -r "${CAPTURE}" \
  --flow-filter "~u ${MAC_IP}:8000" \
  2>/dev/null | grep -E "POST|GET"
```

### 6.3 Identify Unimplemented Endpoints

Endpoints the service doesn't handle return `404 Not Found`. Check:

```bash
# From service stats
curl -s http://localhost:8000/setup/interaction-stats | python3 -m json.tool

# List parity mismatches (local vs upstream divergence, if mirror enabled)
curl -s http://localhost:8000/setup/parity-mismatches | python3 -m json.tool
```

---

## Step 7 — Revert Migration (Optional)

To restore the speaker to its original config (pointing back to Bose cloud):

```bash
curl -s -X POST "http://localhost:8000/setup/revert/${DEVICE_ID}" | python3 -m json.tool
```

Then reboot the speaker:

```bash
curl -s -X POST "http://localhost:8000/setup/reboot/${DEVICE_ID}"
```

---

## Cleanup

```bash
# Stop mitmweb (Ctrl-C in its terminal)
# Stop Frida (Ctrl-C in its terminal)
# Stop soundtouch-service (Ctrl-C in its terminal)

# Remove emulator proxy (if not running another session)
adb -s emulator-5554 shell settings delete global http_proxy
```

---

## Troubleshooting

| Symptom                                   | Cause                                      | Fix                                                                       |
|-------------------------------------------|--------------------------------------------|---------------------------------------------------------------------------|
| Speaker not in service device list        | mDNS discovery hasn't fired yet            | Trigger manually: `POST /setup/discover` or add via `POST /setup/devices` |
| Migration fails with SSH error            | Speaker SSH key not trusted                | Run `POST /setup/trust-ca/{deviceId}` first, or check SSH connectivity    |
| Speaker can't reach service after reboot  | Firewall blocking port 8000 from LAN       | Allow inbound TCP 8000 on Mac firewall                                    |
| `is_migrated: false` after migration      | Wrong `target_url` or config not written   | Check SSH output in migration response; re-run with `--method xml`        |
| Service logs show no speaker requests     | `remote_services` not enabled on speaker   | Run `POST /setup/ensure-remote-services/{deviceId}` and reboot            |
| App shows speaker offline after migration | Speaker config not pointing to correct URL | Check `margeServerUrl` via `GET /setup/summary/{deviceId}`                |

---

## Session Trace (2026-05-02, ST10)

Raw log of the first interactive migration run.

### Service Configuration

Settings applied in the web UI before migration:

| Setting             | Value                                                               |
|---------------------|---------------------------------------------------------------------|
| Target Domain       | `soundtouch.local` (resolvable from speaker to `192.168.x.z`)       |
| DNS Discovery       | enabled                                                             |
| Upstream DNS        | home Wi-Fi gateway                                                  |
| Mirroring           | enabled (for tracing while Bose cloud is still up)                  |
| Mirrored endpoints  | `/bmx/*`, `/streaming/*`, `/accounts/*`, `/v1/scmudc/*`, `/oauth/*` |
| Proxy logging       | enabled, including bodies                                           |
| Record interactions | enabled                                                             |
| Skip recording      | `/setup/*`, `/web/*`                                                |

Settings saved and service restarted.

### Navigation Flow

1. **Tab 1 — Settings**: entered all settings above, clicked **Save Settings**, restarted service
2. **Tab 2 — Devices**: speaker appeared via mDNS discovery; clicked **Sync Data**
3. **Tab 3 — Data Sync**: clicked **Start Sync** to pull account/device data from Bose cloud
4. **Tab 2 — Devices**: clicked **Migrate** on the speaker entry
5. In the Migrate panel: selected **Migration Method → `/etc/resolv.conf`**
6. Ran pre-migration checks (see below)
7. Ran migration steps (see below)
8. Rebooted speaker
9. Paired and configured speaker via the Bose app

### Pre-Migration Checks

All tests run from the **Devices → Migrate** panel after selecting the speaker (`192.168.x.y`, SoundTouch 10):

- **HTTPS test (explicit CA.crt)**: ✅ passed (result not recorded in detail)
- **HTTPS test (shared trust store)**: ✅ passed
  - Speaker connected to `soundtouch.local:443` → `192.168.x.z`
  - TLS: TLSv1.2 / ECDHE-RSA-AES128-GCM-SHA256, cert issued by `SoundTouch Local Root CA`
  - CA already in speaker's system trust store (`/etc/pki/tls/certs/ca-bundle.crt`)
- **Preliminary DNS Test**: ✅ passed
  - Raw DNS query for `aftertouch.test` returned `192.168.x.z` via the service DNS at `192.168.x.z:53`
- **Planned `/etc/resolv.conf`**:
  ```
  # Created by Aftertouch/SoundTouch-Service
  # Priority nameserver for Bose service redirection
  nameserver 192.168.x.z
  ```

### Migration Steps

1. **Enable Persistent Remote Services** → `Successfully ensured remote services for SoundTouch 10 (192.168.x.y)`
   - Note: `touch /etc/remote_services (with rw): sh: rw: command not found` — safe to ignore, `touch` succeeded
2. Reloaded migration view by deselecting and reselecting the speaker in the dropdown
3. **Backup Config Now** → `✅ Found .original config at /opt/Bose/etc/SoundTouchSdkPrivateCfg.xml.original`
4. **Confirm Migration** → `Successfully started migration for SoundTouch 10 (192.168.x.y). Please reboot the device to activate the changes.`

   Command output:
   - Off-device backup created ✅
   - Write access verified ✅
   - `soundtouch.local` resolved to `192.168.x.z` ✅
   - `/mnt/nv/soundtouch-service/aftertouch.resolv.conf` uploaded ✅
   - `rc.local` already contains Aftertouch hook logic ✅
   - `(rw || mount -o remount,rw /): sh: rw: command not found` — safe to ignore (same shell quirk as above)
   - `/etc/udhcpc.d/50default` patched and verified ✅
   - `/opt/Bose/udhcpc.script` patched and verified ✅
   - CA certificate already trusted, skipping injection ✅

5. **Reboot Speaker** → speaker came back online after ~30 s

### Post-Migration

- Paired speaker to Bose account via app — succeeded ✅
- Set presets via app — worked ✅
- Mirroring active and functional during session ✅
- No visible errors in app behaviour; service logs and interaction recordings not yet reviewed in detail

### Known Shell Warning (safe to ignore)

Two commands produced `sh: rw: command not found`. This occurs because the service wraps commands with `(rw || ...)` as a fallback pattern, but the shell on the ST10 interprets `rw` as a bare command rather than a shell variable/flag. The primary command (`touch`, `mount`) still succeeds. This is a known cosmetic issue in the migration output.

---

## See Also

- [CAPTURE-DEVICE-PAIRING.md](CAPTURE-DEVICE-PAIRING.md) — MITM setup and pairing capture
- [MIGRATION-GUIDE.md](MIGRATION-GUIDE.md) — full migration reference
- [SOUNDTOUCH-SERVICE.md](SOUNDTOUCH-SERVICE.md) — service architecture and configuration
- [BOSE-APP-ADB-Emulator.md](../analysis/BOSE-APP-ADB-Emulator.md) — Frida + mitmproxy setup
