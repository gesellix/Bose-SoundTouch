---
title: "SoundTouch Troubleshooting Guide"
---
**Complete guide to diagnosing and fixing common SoundTouch Go client issues**

This guide helps you quickly identify and resolve problems with the SoundTouch Go client library. Issues are organized by category with step-by-step solutions.

## 🚨 **Quick Diagnostics**

### Test Your Setup
Run these commands to quickly diagnose your setup:

```bash
# 1. Test discovery
go run ./cmd/soundtouch-cli -discover

# 2. Test specific device connection
go run ./cmd/soundtouch-cli -host 192.0.2.100 -info

# 3. Test basic controls
go run ./cmd/soundtouch-cli -host 192.0.2.100 -volume

# 4. Test network connectivity
ping 192.0.2.100
```

---

## 🔍 **Discovery Issues**

### ❌ "No devices found"

**Symptoms:**
```
🔍 Discovering SoundTouch devices...
❌ No devices found on the network
```

**Causes & Solutions:**

#### 1. **Network Configuration**
```bash
# Check if devices are on same network
ip route show default  # Your gateway
arp -a | grep -i bose   # Look for Bose devices
```

**Solution:** Ensure both your computer and SoundTouch are on the same subnet.

#### 2. **Firewall Issues**
```bash
# Check if firewall is blocking UPnP
sudo ufw status                    # Ubuntu
netsh advfirewall show allprofiles # Windows
```

**Solution:** Allow UPnP traffic (port 1900 UDP) or temporarily disable firewall.

#### 3. **Device Not Ready**
- Power cycle your SoundTouch device
- Wait 30 seconds for full boot
- Check device is connected to network (solid white LED)

#### 4. **Discovery Timeout Too Short**
```go
discoverer := discovery.NewDiscoverer(discovery.Config{
    Timeout: 30 * time.Second,  // Increase timeout
})
```

#### 5. **Use Manual IP**
```go
// Bypass discovery entirely
client := client.NewClientFromHost("192.0.2.100")
```

### ❌ "Discovery timeout"

**Symptoms:**
```
🔍 Discovering SoundTouch devices (timeout: 5s)...
❌ Discovery failed: context deadline exceeded
```

**Solutions:**

1. **Increase timeout:**
```go
discoverer := discovery.NewDiscoverer(discovery.Config{
    Timeout: 15 * time.Second,
})
```

2. **Check network performance:**
```bash
# Test network latency
ping -c 4 192.0.2.1

# Check for network congestion
iperf3 -c 192.0.2.1  # If iperf server available
```

3. **Use wired connection if possible**

---

## 🌐 **Connection Issues**

### ❌ Every cloud source shows `status="UNAVAILABLE"` / can't stream anything

**Symptoms:**

- The speaker's `/sources` (or the soundtouch-cli `source availability` output) lists every cloud-backed source — Spotify, TuneIn, Internet Radio, AirPlay, Amazon, Alexa — as `status="UNAVAILABLE"`.
- Often only AUX shows `status="READY"`.
- The speaker can be reached on the LAN (`:8090/info` works) but no Internet streaming source can be selected.

This is a different failure mode from the [`Curl 7` case below](#-speaker-logs-curl-7-http-0-and-aftertouch-sees-no-http-requests): the speaker can reach AfterTouch but doesn't have the account state to authenticate any cloud surface, so every cloud handler 401s itself out.

**Three-step diagnostic checklist** (in order — the cause is almost always one of these):

#### 1. Is `:443` reachable on AfterTouch?

The AfterTouch Settings tab now ships a preflight that flips ✅ / ❌ for whether the speaker can open a TLS handshake to AfterTouch's HTTPS listener. If `:443` is ❌, follow the steps in [HTTPS-SETUP.md → Binding to port 443](HTTPS-SETUP.md#binding-to-port-443).

A failing preflight at this layer typically presents as `Curl 7, http 0` in the speaker's syslog (see the [`Curl 7` entry below](#-speaker-logs-curl-7-http-0-and-aftertouch-sees-no-http-requests) for the focused walkthrough).

#### 2. Does the speaker have a `margeAccountUUID`?

```bash
curl -s http://<speaker-ip>:8090/info | xmllint --xpath '/info/margeAccountUUID/text()' -
```

If the element is empty (or you get no output), the speaker has no account token — every cloud surface that requires authentication will 401 itself out. The Migration tab in AfterTouch detects this and renders:

> **Current: ❌ Not paired (factory-reset or never paired) — set an ID to pair as part of Apply**

The Devices list also shows a `⚠ Not paired — re-pair` badge. To resolve, **open the Migration tab**, pick a previous account ID from the dropdown (or click **Generate**), and click **Apply** — same flow as the [factory-reset recovery](#-presets-flash-then-revert-to-select-a-preset-after-a-factory-reset) section below.

#### 3. What does `logread` say while you trigger a failing source?

SSH into the speaker (see [DEVICE-LOGGING.md](../appendix/DEVICE-LOGGING.md#1-accessing-system-logs-requires-root)) and capture:

```bash
logread -f | grep -v '127.0.0.1:'
```

…while you select a failing source in the SoundTouch app or via `soundtouch-cli`. The lines around the failed attempt usually name the failing host + protocol — TLS handshake error, token fetch 401, missing route, etc. — and that's enough to file an actionable issue.

**Common outcomes:**

- ❌ `:443` → fix HTTPS routing, sources transition to READY on the next refresh.
- ❌ `margeAccountUUID` empty → run Migration → Apply, sources reappear after `<sourcesUpdated/>` triggers a `/sources` re-sync.
- Everything looks right but sources still UNAVAILABLE → the `logread` snippet is the next signal; open an issue with it attached.

> **Note on the firmware-internal placeholder sources.** The `<sourceItem source="SPOTIFY" sourceAccount="SpotifyConnectUserName" ...>`, `SpotifyAlexaUserName`, `UPNP/UPnPUserName`, `STORED_MUSIC_MEDIA_RENDERER/StoredMusicUserName`, and `QPLAY/QPlay{1,2}UserName` entries that appear in `/sources` even on a broken or unpaired speaker are *firmware-synthesized*. They show up regardless of AfterTouch's source list — their `status="UNAVAILABLE"` does not indicate an AfterTouch problem. Use the three checks above to diagnose the actual cause.

### ❌ Speaker logs `Curl 7, http 0` and AfterTouch sees no HTTP requests

**Symptoms:**

In the speaker's log (see [DEVICE-LOGGING.md](../appendix/DEVICE-LOGGING.md#1-accessing-system-logs-requires-root) for the SSH/`logread` setup — the filtered command `logread -f | grep -v '127.0.0.1'` is what you want here):

```
SimpleURLFetcher: retry needed, Curl 7, http 0
```

In the AfterTouch service log: plenty of `[DNS] Intercepted query …` lines but **zero** HTTP requests after each DNS lookup.

**Cause:** speakers connect to Bose hostnames over implicit HTTPS, i.e. port **443**. AfterTouch's built-in HTTPS listener defaults to **8443** because 443 is privileged. The speaker resolves the right IP, dials `:443`, and gets connection refused — which is what `Curl 7` reports.

**Verify:**

```bash
curl -ksS -o /dev/null -w "443=%{http_code}\n"  https://localhost:443/
curl -ksS -o /dev/null -w "8443=%{http_code}\n" https://localhost:8443/
```

Expected when the misconfiguration is present: `443=000` plus a `curl: (7) Failed to connect …` line, `8443=200` (or any 3-digit code).

**Fix:** route `:443` to AfterTouch's HTTPS listener — see [HTTPS-SETUP.md → Binding to port 443](HTTPS-SETUP.md#binding-to-port-443). The AfterTouch settings page shows a ✅ / ❌ indicator for `:443` reachability once the routing is in place.

### ❌ Presets flash then revert to "Select a preset" after a factory reset

**Symptoms:**

- You factory-reset a SoundTouch (Wave / 10 / 20 / 30 / …) that was previously migrated.
- After reconnecting it to Wi-Fi, AfterTouch sees the speaker again, but pressing a preset on the device or in the app makes the display briefly show the preset name and then revert to *"Select a preset or explore music in the SoundTouch App"*.
- Spotify presets show the same revert unless Spotify Connect is started from the mobile app first.
- The speaker's `/sources` is missing TUNEIN / LOCAL_INTERNET_RADIO / DEEZER / your linked Spotify account — only AUX, BLUETOOTH, AIRPLAY, the SpotifyConnectUserName placeholder, NOTIFICATION, and QPLAY appear.

**Cause:**

A factory reset wipes `/mnt/nv/BoseApp-Persistence/1/Marge.xml` — the file that carries the speaker's auth token for the AfterTouch (or Bose) cloud service. The migrated URL configuration is preserved (it lives in `envswitch`), so the speaker keeps talking to AfterTouch, but with no token it can't authenticate for preset playback. Separately, the device's `/sources` cache is reduced until it receives a `<sourcesUpdated/>` notification.

**Fix:**

1. **Re-open the Migration tab** in the AfterTouch UI. The wizard reads `/info`, sees `margeAccountUUID` is empty, and renders:

   > **Current: ❌ Not paired (factory-reset or never paired) — set an ID to pair as part of Apply**

   The devices list now also shows a `⚠ Not paired — re-pair` badge next to such speakers, so you don't have to remember to open the Migration tab cold.

2. **Pick the previously-used account ID** from the "pick from datastore" dropdown (if AfterTouch remembers it), or click **Generate** for a fresh one.

3. **Click Apply.** The wizard runs `pair-account` along with the rest, recreating `Marge.xml` on the device with the chosen ID.

4. **Click Data Sync** (Tab 3). AfterTouch persists the speaker's presets/recents/sources and posts a `<sourcesUpdated/>` notification to the device — the missing TUNEIN / LOCAL_INTERNET_RADIO / DEEZER / linked Spotify entries reappear in `/sources` automatically.

5. Press a preset. It should play normally.

If presets still won't play after step 5, capture `logread -f | grep -v '127.0.0.1:'` on the speaker (see [DEVICE-LOGGING.md](../appendix/DEVICE-LOGGING.md#1-accessing-system-logs-requires-root)) while pressing the preset and file an issue with the snippet — the lines around the failed playback name the deeper cause.

### ❌ "Connection refused"

**Symptoms:**
```go
Failed to connect: dial tcp 192.0.2.100:8090: connection refused
```

**Diagnostic Steps:**

#### 1. **Verify IP and Port**
```bash
# Test if port 8090 is open
telnet 192.0.2.100 8090
# OR
nc -zv 192.0.2.100 8090

# Scan for open ports
nmap -p 8080-8100 192.0.2.100
```

#### 2. **Check Device Status**
- Device LED should be solid white (connected)
- Blinking white = connecting
- Red = error state

#### 3. **Router/Network Issues**
```bash
# Check routing
traceroute 192.0.2.100

# Test basic connectivity
ping -c 4 192.0.2.100
```

### ❌ "Timeout" / "Context deadline exceeded"

**Symptoms:**
```go
Failed to get device info: context deadline exceeded
```

**Solutions:**

#### 1. **Increase Client Timeout**
```go
config := client.ClientConfig{
    Host:    "192.0.2.100",
    Port:    8090,
    Timeout: 30 * time.Second,  // Increase from default 10s
}
```

#### 2. **Check Network Latency**
```bash
# Test response time
ping -c 10 192.0.2.100

# Should be < 100ms typically
```

#### 3. **Device Performance Issues**
- Device may be overloaded
- Try power cycling the device
- Check for firmware updates via Bose app

### ❌ "No such host"

**Symptoms:**
```go
Failed to connect: dial tcp: lookup soundtouch.local: no such host
```

**Solutions:**

1. **Use IP instead of hostname:**
```go
client := client.NewClientFromHost("192.0.2.100")  // Not "soundtouch.local"
```

2. **Fix DNS/mDNS:**
```bash
# Test hostname resolution
nslookup soundtouch.local
dig soundtouch.local

# Install mDNS tools if needed (Linux)
sudo apt-get install avahi-utils
avahi-resolve -n soundtouch.local
```

---

## 🎵 **Playback Control Issues**

### ❌ "Play/Pause not working"

**Symptoms:**
- Commands succeed but no audio change
- Device shows wrong status

**Diagnostic Steps:**

#### 1. **Check Current Status**
```go
nowPlaying, err := client.GetNowPlaying()
if err == nil {
    fmt.Printf("Status: %s, Source: %s\n", 
        nowPlaying.PlayStatus, nowPlaying.Source)
}
```

#### 2. **Verify Source Selection**
```go
sources, err := client.GetSources()
if err == nil {
    for _, source := range sources.Sources {
        fmt.Printf("Source: %s, Status: %s\n", 
            source.Source, source.Status)
    }
}
```

**Solutions:**

1. **Select active source first:**
```go
client.SelectSpotify()
time.Sleep(2 * time.Second)  // Wait for source change
client.Play()
```

2. **Use key commands instead:**
```go
client.SendKey("PLAY")   // Instead of client.Play()
client.SendKey("PAUSE")  // Instead of client.Pause()
```

3. **Check device isn't in setup mode**

### ❌ "Source selection fails"

**Symptoms:**
```go
Failed to select source: API request failed with status 500
```

**Solutions:**

1. **Check source availability:**
```go
sources, _ := client.GetSources()
for _, source := range sources.Sources {
    if source.Source == "SPOTIFY" && source.Status == "READY" {
        // Source is available
        client.SelectSource("SPOTIFY", source.SourceAccount)
    }
}
```

2. **Account-specific sources:**
```go
// For streaming services, include account
client.SelectSource("SPOTIFY", "your_account_id")
```

3. **Use convenience methods:**
```go
client.SelectSpotify()    // Handles account automatically
client.SelectBluetooth()
client.SelectAux()
```

---

## 🎶 **Music Service & Preset Issues**

### ❌ Spotify preset fails with "Current content cannot be saved as preset"

**Symptoms:**

You push playback to the speaker via Spotify Connect from the Spotify mobile/desktop app. Audio plays fine. You try to store it as a preset and the CLI reports:

```
$ soundtouch-cli preset store-current --slot 2
Storing current content as preset 2 from 192.168.x.y:8090...
✗ Current content cannot be saved as preset
  Content: <track name>
  Source: SPOTIFY
2026/05/16 09:13:10 current content cannot be preset
```

…and `soundtouch-cli play now` shows `Source Account: SpotifyConnectUserName`.

**Cause:**

The speaker firmware marks Spotify-Connect-pushed content as **non-presetable** at the NowPlaying layer:

```xml
<ContentItem source="SPOTIFY" type="DO_NOT_RESUME" ...
             sourceAccount="SpotifyConnectUserName" isPresetable="false">
```

That `isPresetable="false"` means the firmware can't independently re-fetch the stream later — it only knows about the session token your phone pushed via the Spotify Connect protocol, which is ephemeral. The speaker refuses the preset *locally*, before any storePreset request reaches AfterTouch's marge.

**Why an OAuth-linked Spotify account changes the answer:**

When AfterTouch has a Spotify OAuth account linked (see [MUSIC-SERVICES.md](MUSIC-SERVICES.md)), the speaker has a *persistent* Spotify source it can use to resolve the content URI later — typically an album/playlist container. With that source available, the firmware rewrites the content item from `DO_NOT_RESUME` to `tracklisturl` at save time, flips `isPresetable` to `true`, and the preset goes through. The recall path then routes through AfterTouch's `/oauth/.../cs3` token broker, which returns a Spotify access token for your linked account.

**Fix:**

1. Set up Spotify OAuth in AfterTouch following [MUSIC-SERVICES.md](MUSIC-SERVICES.md). The high-level model (Spotify Connect vs the OAuth-intercept path, the `streamingoauth.bose.com` DNS rewrite, the token lifecycle) is in [spotify-overview.md](../concepts/spotify-overview.md).
2. Make sure you're on **v0.84.0 or later** — earlier versions had a custom-OAuth-client bug that caused playback to hang at "Buffering".
3. Re-prime the speaker (Migration tab → **Prime Spotify**, or wait for the watchdog), then retry the preset save with Connect-pushed playback.

**What this won't fix:**

A Connect-only setup with no OAuth account linked in AfterTouch — that's a firmware-level constraint we can't route around from the server side. The speaker simply doesn't have credentials it can use to replay the content later, so it refuses to preset.

### ❌ TuneIn (or Internet Radio) missing from `/sources` after a factory reset

**Symptoms:**

- The speaker is happily migrated and reachable; most cloud sources work.
- `curl http://<speaker-ip>:8090/sources` lists AUX, Bluetooth, Spotify Connect placeholders, etc. — but **no `TUNEIN` entry**.
- `soundtouch-cli source content --source TUNEIN --type stationurl --location /v1/playback/station/<id> --name '<name>'` fails with `1005` (or playing a TuneIn preset silently does nothing).
- Other devices on the same setup have `TUNEIN` in `/sources` and work fine.

**Cause:**

TuneIn is **not a default source** on a freshly factory-reset SoundTouch. The speaker only adds `TUNEIN` to its `Sources.xml` after the source has been played at least once. Until then, source-selection requests for `TUNEIN` are rejected as invalid.

This is firmware behaviour — independent of AfterTouch — and is why one device can have `TUNEIN` and a sibling device (just reset) can be missing it. The same applies to `LOCAL_INTERNET_RADIO` if the speaker was reset before any LIR content was played.

**Fix:**

Play any TuneIn station once to register the source. Two equivalent paths:

1. **Via the SoundTouch app** — open the app, pick TuneIn, play any station. The source appears in `/sources` after a few seconds.
2. **Via `soundtouch-cli`** on a device that *does* still have TuneIn registered, or by first registering it with a known-working station:

   ```bash
   soundtouch-cli --host <speaker-ip> source content \
     --source TUNEIN --type stationurl \
     --location /v1/playback/station/s166521 \
     --name 'SMOOTH JAZZ'
   ```

   (Station `s166521` is one that works for AfterTouch testing; any valid TuneIn station ID works.)

Once the source plays once, it gets persisted to `/mnt/nv/BoseApp-Persistence/1/Sources.xml` and subsequent TuneIn requests succeed without needing the app.

**For speakers without SSH:**

If `soundtouch-cli source content --source TUNEIN ...` returns `1005` on a reset device that has never had TuneIn, the speaker is refusing because the source isn't registered yet — chicken-and-egg. The SoundTouch app is then the only practical path to register it; we can't write `Sources.xml` directly over telnet on most models.

## 🔊 **Volume & Audio Issues**

### ❌ "Volume control not working"

**Symptoms:**
- Volume commands succeed but no change
- "Permission denied" errors

**Diagnostic Steps:**

#### 1. **Check Zone Status**
```go
zoneStatus, err := client.GetZoneStatus()
if err == nil {
    fmt.Printf("Zone Status: %s\n", zoneStatus)
}
```

**Solutions:**

1. **Zone Member Issue:**
```go
// Only zone master can control volume
if zoneStatus == "MEMBER" {
    fmt.Println("Device is zone member - only master controls volume")
    
    // Find and use master device
    zone, _ := client.GetZone()
    // Connect to master device using zone.Master ID
}
```

2. **Use Safe Volume Methods:**
```go
client.SetVolumeSafe(50)     // Clamps to valid range
client.IncreaseVolume(5)     // Incremental control
client.DecreaseVolume(5)
```

3. **Check Current Volume:**
```go
volume, _ := client.GetVolume()
fmt.Printf("Target: %d, Actual: %d, Muted: %t\n", 
    volume.TargetVolume, volume.ActualVolume, volume.Muted)
```

### ❌ "Bass/Balance control not supported"

**Symptoms:**
```go
Failed to set bass: API request failed with status 404
```

**Solutions:**

1. **Check device capabilities:**
```go
caps, err := client.GetCapabilities()
if err == nil {
    fmt.Printf("Bass capable: %t\n", caps.BassCapable)
}
```

2. **Use safe methods:**
```go
client.SetBassSafe(-5)       // Won't fail on unsupported devices
client.SetBalanceSafe(10)    // Falls back gracefully
```

3. **Device-specific features:**
- SoundTouch 10: Basic bass only
- SoundTouch 20/30: Full bass and balance
- Soundbar models: Advanced audio controls

---

## 🔔 **Speaker Notification Issues**

### ❌ "speaker beep" command fails with status 400

**Symptoms:**
```bash
$ go run ./cmd/soundtouch-cli --host 192.0.2.10 sp beep
Playing notification beep from 192.0.2.10:8090...
✗ Failed to play notification beep: API request failed with status 400
```

**Cause:**
This was a bug in earlier versions where the Go client incorrectly used POST instead of GET for the `/playNotification` endpoint.

**Solution:**
Update to the latest version. The fix changed the `PlayNotificationBeep()` method to use GET requests:

```go
// Fixed implementation (v2025.02+)
func (c *Client) PlayNotificationBeep() error {
    var status models.StationResponse
    return c.get("/playNotification", &status)
}
```

**Verification:**
Both commands should now work identically:
```bash
# CLI command
go run ./cmd/soundtouch-cli --host 192.0.2.10 sp beep

# Direct curl (for comparison)
curl http://192.0.2.10:8090/playNotification
```

### ❌ "speaker" commands not supported

**Symptoms:**
```
✗ Failed to play notification: endpoint not supported
```

**Causes & Solutions:**

#### 1. **Device Model Compatibility**
- ✅ **Supported**: SoundTouch 10 (ST-10), SoundTouch 20 (ST-20)  
- ❌ **Not Supported**: SoundTouch 300 (ST-300), older models

**Solution:** Verify device model with:
```bash
soundtouch-cli --host <device> info
```

#### 2. **Missing App Key (TTS/URL only)**
TTS and URL playback require an app key, but beep does not:
```bash
# Beep - no app key needed
soundtouch-cli --host <device> speaker beep

# TTS - app key required
soundtouch-cli --host <device> speaker tts --text "Hello" --app-key "your-key"
```

### ❌ "Device is busy" during notifications

**Symptoms:**
```
✗ Failed to play notification: device is busy
```

**Solutions:**

#### 1. **Wait for Current Notification to Complete**
Only one notification can play at a time. Wait a few seconds and retry.

#### 2. **Check Current Playback Status**
```go
nowPlaying, _ := client.GetNowPlaying()
fmt.Printf("Current source: %s, status: %s\n", 
    nowPlaying.Source, nowPlaying.PlayStatus)
```

### ❌ soundtouch-web TTS fails with `certificate signed by unknown authority`

**Symptoms:**
```
TTS service request failed: Post "https://soundtouch.fritz.box/setup/tts/speak":
tls: failed to verify certificate: x509: certificate signed by unknown authority
```

**Cause:** TTS synthesis and the Bose app key live in `soundtouch-service`,
so `soundtouch-web` proxies the "Speak" action to the service. When the
service is served over HTTPS with its own self-signed certificate (the
default — see `GET /setup/ca.crt`), `soundtouch-web` doesn't trust that CA out
of the box, so the proxied call fails verification.

**Solution:** start `soundtouch-web` with `--service-ca` pointing at the
service's CA certificate (its `<dataDir>/certs/ca.crt`, or the file served at
`/setup/ca.crt`):

```bash
soundtouch-web \
  --service-url https://soundtouch.fritz.box \
  --service-ca /path/to/certs/ca.crt
```

`SERVICE_CA` is the equivalent environment variable. The CA is appended to the
system trust store, so a service URL that uses a publicly trusted certificate
needs no flag.

### ❌ soundtouch-web TTS returns `host ... is not a known device`

**Symptoms:**
```
TTS service returned 400: {"error":"host http://192.0.2.10:8090 is not a known device"}
```

**Cause:** the service only plays TTS on speakers it knows (an SSRF guard:
the target is matched against the service's device datastore, never taken
verbatim from the request).

**Solution:** make sure the target speaker is known to `soundtouch-service`
(discovered or manually added, and migrated to AfterTouch), not only to
`soundtouch-web`'s own discovery. Check with `GET /setup/devices` on the
service. (Recent `soundtouch-web` versions identify the speaker by its device
ID and a bare IP, so this error otherwise indicates the speaker simply isn't
registered with the service.)

---

## 📡 **WebSocket Issues**

### ❌ "WebSocket connection failed"

**Symptoms:**
```go
Failed to connect WebSocket: dial ws://192.0.2.100:8080/: connection refused
```

**Solutions:**

#### 1. **Verify WebSocket Port (8080)**
```bash
# WebSocket uses port 8080, not 8090
nc -zv 192.0.2.100 8080
```

#### 2. **Check Protocol Specification**
```go
// WebSocket client should auto-handle this
wsClient := client.NewWebSocketClient(nil)

// Manual connection (if needed)
url := "ws://192.0.2.100:8080/"
headers := http.Header{}
headers.Set("Sec-WebSocket-Protocol", "gabbo")
```

#### 3. **Connection Conflicts**
- Only one WebSocket connection per device
- Close other apps using SoundTouch
- Restart SoundTouch device if needed

### ❌ "WebSocket disconnects frequently"

**Symptoms:**
- Connection drops every few minutes
- Constant reconnection messages

**Solutions:**

1. **Increase ping interval:**
```go
config := client.DefaultWebSocketConfig()
config.PingInterval = 60 * time.Second    // Increase from 30s
config.PongTimeout = 20 * time.Second     // Increase timeout

wsClient := client.NewWebSocketClient(config)
```

2. **Check network stability:**
```bash
# Test for packet loss
ping -c 100 192.0.2.100 | grep loss
```

3. **Power management issues:**
```bash
# Disable WiFi power saving (Linux)
sudo iwconfig wlan0 power off

# Check Windows power management
powercfg -devicequery wake_armed
```

### ❌ "Events not received"

**Symptoms:**
- WebSocket connects but no events
- Missing volume/playback updates

**Solutions:**

1. **Verify event handlers:**
```go
wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
    fmt.Printf("Volume event received: %d\n", event.Volume.TargetVolume)
})

// Test by manually changing volume on device
```

2. **Check event parsing:**
```go
wsClient.OnUnknownEvent(func(event *models.WebSocketEvent) {
    fmt.Printf("Unknown event: %+v\n", event)
})
```

3. **Device activity required:**
- Events only sent when device state changes
- Try manual volume/source changes
- Check device isn't in standby

---

## 👥 **Multiroom Issues**

### ❌ "Zone creation fails"

**Symptoms:**
```go
Failed to create zone: API request failed with status 400
```

**Solutions:**

#### 1. **Check Device Compatibility**
```go
// Get device capabilities
caps, _ := client.GetCapabilities()
// Look for multiroom support

// Verify devices are on same network
for _, client := range clients {
    network, _ := client.GetNetworkInfo()
    fmt.Printf("Device IP: %s\n", network.GetConnectedInterface().IPAddress)
}
```

#### 2. **Correct Device IDs**
```go
// Get exact device IDs
info, _ := client.GetDeviceInfo()
masterID := info.DeviceID  // Use this, not MAC address

// Create zone with proper IDs
client.CreateZone(masterID, []string{member1ID, member2ID})
```

#### 3. **Sequential Zone Operations**
```go
// Don't create multiple zones simultaneously
client1.CreateZone(master1, []string{member1})
time.Sleep(2 * time.Second)
client2.CreateZone(master2, []string{member2})
```

### ❌ "Device won't join zone"

**Symptoms:**
- Zone creation succeeds but member doesn't join
- Member device shows as standalone

**Solutions:**

1. **Check device status:**
```go
status, _ := memberClient.GetZoneStatus()
fmt.Printf("Member status: %s\n", status)

if status == "STANDALONE" {
    // Device didn't join - check network/permissions
}
```

2. **Firmware compatibility:**
- Ensure all devices have recent firmware
- Update via Bose SoundTouch app
- Some very old devices don't support multiroom

3. **Network subnet issues:**
```bash
# Verify devices can reach each other
ping -c 4 member_device_ip
```

---

## 🔧 **Development & Debugging**

### Enable Detailed Logging

```go
import "log"

// Enable verbose HTTP logging
log.SetFlags(log.LstdFlags | log.Lshortfile)

// Custom HTTP client with debug
transport := &http.Transport{
    // Add debug transport if needed
}

config := client.ClientConfig{
    Host:    "192.0.2.100",
    Port:    8090,
    Timeout: 10 * time.Second,
}
```

### Debug WebSocket Events

```go
wsClient.OnUnknownEvent(func(event *models.WebSocketEvent) {
    log.Printf("Raw event: %+v", event)
})

// Enable WebSocket debug logging
config := client.DefaultWebSocketConfig()
config.Logger = &client.DefaultLogger{}  // Or custom logger
```

### Network Debugging Tools

```bash
# Capture SoundTouch traffic
sudo tcpdump -i any host 192.0.2.100 and port 8090

# Monitor WebSocket traffic  
sudo tcpdump -i any host 192.0.2.100 and port 8080

# HTTP debugging with curl
curl -v http://192.0.2.100:8090/info
curl -v http://192.0.2.100:8090/volume
```

---

## 📊 **Performance Issues**

### High Memory Usage

**Symptoms:**
- Go process memory keeps growing
- Out of memory errors in long-running apps

**Solutions:**

1. **Connection cleanup:**
```go
// Always close WebSocket connections
defer wsClient.Disconnect()

// Use connection pools for multiple devices
pool := NewConnectionPool(10, 5*time.Minute)
defer pool.Close()
```

2. **Goroutine leaks:**
```go
// Use context for cancellation
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Monitor goroutines
go func() {
    for {
        fmt.Printf("Goroutines: %d\n", runtime.NumGoroutine())
        time.Sleep(10 * time.Second)
    }
}()
```

### Slow Response Times

**Solutions:**

1. **Increase timeouts appropriately:**
```go
config := client.ClientConfig{
    Timeout: 15 * time.Second,  // Reasonable for network ops
}
```

2. **Use connection pooling:**
```go
// Reuse connections instead of creating new ones
pool := NewConnectionPool(5, 5*time.Minute)
client := pool.GetClient(host, port)
```

3. **Concurrent operations:**
```go
// Process multiple devices concurrently
var wg sync.WaitGroup
for _, client := range clients {
    wg.Add(1)
    go func(c *client.Client) {
        defer wg.Done()
        // Process device
    }(client)
}
wg.Wait()
```

---

## 🚨 **Emergency Procedures**

### Device Becomes Unresponsive

1. **Power cycle device:**
   - Unplug for 10 seconds
   - Reconnect and wait 30 seconds for boot

2. **Network reset:**
   - Hold Bluetooth and Volume Down for 10 seconds
   - Device will reset network settings

3. **Factory reset (last resort):**
   - Hold Power for 10 seconds while plugged in
   - Will lose all presets and settings

### Multiple Devices Acting Strange

1. **Check router:**
   - Restart router/access point
   - Check for firmware updates
   - Verify DHCP/IP assignment

2. **Network interference:**
   - Check for 2.4GHz interference
   - Try 5GHz WiFi if available
   - Check for microwave/Bluetooth interference

### App Crashes or Hangs

1. **Graceful shutdown:**
```go
// Always use context for cancellation
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Cleanup resources
defer func() {
    if wsClient != nil {
        wsClient.Disconnect()
    }
}()
```

2. **Resource monitoring:**
```go
// Monitor resource usage
go func() {
    var m runtime.MemStats
    for {
        runtime.ReadMemStats(&m)
        log.Printf("Alloc = %d KB, Sys = %d KB", m.Alloc/1024, m.Sys/1024)
        time.Sleep(30 * time.Second)
    }
}()
```

---

## 📋 **Diagnostic Checklist**

Use this checklist to systematically troubleshoot issues:

### Network Connectivity
- [ ] Device power LED is solid white
- [ ] Both devices on same network subnet  
- [ ] Firewall allows ports 8090 (HTTP) and 8080 (WebSocket)
- [ ] Can ping device IP address
- [ ] Can telnet to ports 8090 and 8080

### Device Status
- [ ] Device not in setup mode (solid white LED)
- [ ] Recent firmware version (check Bose app)
- [ ] Device responds to Bose app
- [ ] No other apps connected to device

### Code Configuration
- [ ] Correct IP address and ports
- [ ] Reasonable timeouts (10-30 seconds)
- [ ] Proper error handling
- [ ] Resource cleanup (defer statements)

### Multiroom Specific  
- [ ] All devices support multiroom
- [ ] Device IDs are correct (from GetDeviceInfo)
- [ ] Devices on same network subnet
- [ ] No existing zone conflicts

---

## 🆔 **Device Identification & Mapping Issues**

### ❌ "File not found" errors with MAC addresses

**Symptoms:**
```
GET /streaming/account/1000001/device/AABBCCDDEEFF/presets
→ 500 Internal Server Error
→ Log: "open .../devices/AABBCCDDEEFF/Presets.xml: no such file or directory"
```

**Cause:** The service uses MAC addresses in API requests but stores files using device serial numbers. A mapping system resolves MAC addresses to serial numbers automatically.

**Quick Solutions:**

1. **Restart the service** (mappings are created at startup):
```bash
sudo systemctl restart soundtouch-service
```

2. **Check device directory structure**:
```bash
# Files should be stored by serial number, not MAC
ls data/accounts/1000001/devices/
# Should show: I6332527703739342000020/ (not AABBCCDDEEFF/)
```

3. **Verify DeviceInfo.xml contains MAC address**:
```bash
cat data/accounts/1000001/devices/*/DeviceInfo.xml | grep macAddress
```

**For detailed diagnosis and solutions**, see: [**MAC Address Mapping Guide**](MAC-ADDRESS-MAPPING.md)

---

## 🌐 **Cross-subnet / VLAN isolation** {#cross-subnet}

### ❌ AfterTouch unreachable when speaker and server are on different subnets

**Symptoms:**

- AfterTouch is running and reachable from your computer, but the speaker cannot
  connect to it after migration.
- `logread | grep aftertouch` on the speaker shows no outgoing requests, or shows
  `Curl 7, http 0` for the AfterTouch host.
- Everything works when speaker and server are on the same `/24` subnet, but fails
  when they are on different VLANs (e.g. IoT VLAN `192.168.20.x` vs server VLAN
  `192.168.10.x`).

**Cause:**

Since a 2018 firmware update, SoundTouch devices apply an iptables policy that
blocks incoming traffic from subnets other than their own. The policy lives in
`/etc/init.d/Firewalls/update_iptables` inside the `block_remote_traffic()`
function.

**Fix A — Add an explicit ACCEPT rule for your AfterTouch subnet (targeted):**

SSH into the speaker and edit the file:

```bash
ssh -oHostKeyAlgorithms=+ssh-rsa root@<speaker-ip>
rw
cd /etc/init.d/Firewalls/
vi update_iptables
```

In `vi`, find `block_remote_traffic()`. Press `i` to enter insert mode. After the
first `done` line in that function, add:

```
echo -A INPUT -i $IFACE -s 192.0.2.0/24 -j ACCEPT
```

Replace `192.0.2.0/24` with the subnet your AfterTouch host is on. Press `Esc`,
type `:wq`, press `Enter`, then reboot:

```bash
reboot
```

To allow **all** subnets, use a wider CIDR range such as `192.168.0.0/16` or
`0.0.0.0/0`.

**Fix B — Comment out the DROP rule (simpler, allows all inbound traffic):**

Instead of adding an ACCEPT rule, find the `DROP` line inside
`block_remote_traffic()` and comment it out by prepending `#`:

```bash
# Before:
echo -A INPUT -i $IFACE ! -s $ADDR/$CIDR -j DROP
# After:
# echo -A INPUT -i $IFACE ! -s $ADDR/$CIDR -j DROP
```

This is less targeted than Fix A but simpler if you run the speaker in an already
firewalled network.

**Related: ST20 Series I also blocks outbound connections to non-standard ports**

Some older firmware images (observed on SoundTouch 20 Series I) also apply an
outbound iptables policy that blocks connections to ports other than 80 and 443.
If AfterTouch is running on a non-standard port (e.g. 8000, 8080) and the speaker
simply never reaches it, this policy may be the cause. Fix A and Fix B apply
equally — inspect `update_iptables` for matching DROP rules on the OUTPUT chain.

*This behaviour was first documented in
[Discussion #354](https://github.com/gesellix/Bose-SoundTouch/discussions/354).*

---

## 🌐 **Hostname Resolution** {#hostname-resolution}

### Why the service resolves the hostname from the device

When you migrate a speaker using the resolv.conf method, the service needs to write a raw IP address into the speaker's network configuration. That IP must be the address the *speaker itself* can reach — which is not necessarily the same address your computer resolves.

In environments with NAT, split-horizon DNS, or Docker/container networking, `soundtouch.local` (or whatever you set as `SERVER_URL`) may resolve to a different IP depending on who is asking. The service therefore resolves the hostname by running `ping -c 1 <hostname>` over SSH on the speaker and extracting the IP from the output. This is the authoritative result: it is exactly what the speaker would use.

If that SSH ping fails, migration is aborted. Writing an unresolvable or incorrectly resolved hostname into `aftertouch.resolv.conf` would silently break the speaker's DNS config and prevent it from reaching the service after reboot.

**The XML migration method is different.** It writes the full URL (e.g. `http://soundtouch.local:8000`) into `SoundTouchSdkPrivateCfg.xml`. The speaker resolves the hostname at connect time, not at migration time. This means migration can proceed even if the hostname is not yet reachable — for example, when the service will be deployed under that hostname but is not running yet. A warning is still shown in the UI so you are aware, but the Confirm Migration button remains enabled.

### ❌ "Cannot resolve target hostname for migration"

**Symptoms** (migration log or web UI warning):
```
cannot resolve target hostname for migration: cannot resolve "soundtouch.local":
SSH ping from device failed and service-side DNS lookup also failed
```
or:
```
resolved "soundtouch.local" to 192.0.2.100 from service, not from device —
result may be wrong if NAT or split-DNS is in use
```

**What this means:**

The service could not confirm the IP by running `ping` on the speaker via SSH. Either:
- the `ping` binary is not available or not in `$PATH` on this firmware, or
- the hostname is not resolvable from the speaker's network context.

**Diagnosis — run manually over SSH:**

```bash
# SSH into the speaker
ssh root@<speaker-ip>

# Try to resolve the service hostname
ping -c 1 soundtouch.local
# or use the IP directly to verify connectivity
ping -c 1 192.0.2.100

# Check the speaker's current DNS config
cat /etc/resolv.conf

# Check if ping is available
which ping
busybox ping --help
```

**Solutions:**

#### 1. Use an IP address as SERVER_URL

The most reliable fix. If the hostname cannot be resolved from the device, use a raw IP instead. Resolution is skipped entirely when `SERVER_URL` contains an IP.

```bash
# In your .env
SERVER_URL=http://192.0.2.100:8000
HTTPS_SERVER_URL=https://192.0.2.100:8443
```

HTTPS works correctly with IP addresses — the service certificate includes the IP as a Subject Alternative Name (SAN).

#### 2. Ensure the hostname resolves on the speaker's network segment

If you use `soundtouch.local`, verify mDNS is working from another device on the same subnet:

```bash
avahi-resolve -n soundtouch.local    # Linux
dns-sd -G v4 soundtouch.local        # macOS
```

#### 3. Use the XML migration method

Select the XML method in the migration UI. It writes the full URL and the speaker resolves it at connect time, so hostname resolution is not required during migration. This also allows migrating to a hostname that is not yet live.

---

## 🛟 **Getting More Help**

### Information to Gather

When reporting issues, include:

```go
// Device information
info, _ := client.GetDeviceInfo()
fmt.Printf("Device: %s %s (ID: %s)\n", info.Type, info.Name, info.DeviceID)

// Network information
network, _ := client.GetNetworkInfo()
fmt.Printf("Network: %+v\n", network)

// Go version and OS
fmt.Printf("Go version: %s\n", runtime.Version())
fmt.Printf("OS: %s/%s\n", runtime.GOOS, runtime.GOARCH)
```

### Useful Commands

```bash
# System information
go version
uname -a  # Linux/macOS
systeminfo # Windows

# Network debugging
ip addr show        # Linux
ifconfig           # macOS
ipconfig /all      # Windows

# SoundTouch specific
go run ./cmd/soundtouch-cli -host <ip> -info
go run ./cmd/soundtouch-cli -host <ip> -network-info
```

### Support Resources

- **GitHub Issues**: Create detailed issue with logs and system info
- **Documentation**: Check `/docs` directory for specific topics
- **Examples**: Review `/examples` for working code patterns
- **CLI Tool**: Use built-in CLI for testing and debugging

Remember: Most issues are network-related. Start with basic connectivity testing before investigating code issues.