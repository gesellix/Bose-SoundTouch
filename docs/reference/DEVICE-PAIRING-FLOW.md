# SoundTouch Device WebSocket API — Pairing & Operation Flow

Reference document derived from mitmproxy captures of the Bose SoundTouch Android app
(`bose-pairing-20260502-155542`, `bose-pairing-20260502-165549`).

> **Why this matters:** With Bose cloud services shutting down on 2026-05-06, the original
> app may stop working for pairing and playback control. This document captures the exact
> WebSocket message sequences needed to replicate those flows independently.

---

## Connection

All interactions use the SoundTouch WebSocket API on the speaker's local IP, port **8090**
(the same port as the REST API). Connect with the `Gabbo` sub-protocol:

```
GET ws://192.168.x.y:8090/
Upgrade: websocket
Sec-WebSocket-Protocol: Gabbo
```

Upon connection the server immediately sends an identification banner:

```xml
<SoundTouchSdkInfo serverVersion="4" serverBuild="trunk r46330 v4 epdbuild hepdswbld04" />
```

---

## Message Envelope

All subsequent messages (except `selectLastWiFiSource`, see below) use this envelope:

**Client → Server request:**
```xml
<msg>
  <header deviceID="{device_id}" url="{endpoint}" method="{GET|POST}">
    <request requestID="{n}">
      <info type="new"/>                <!-- or type="update" -->
      <!-- optional: <sourceItem source="TUNEIN"/> -->
    </request>
  </header>
  <body>
    <!-- payload, may be empty -->
  </body>
</msg>
```

**Server → Client response:**
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<msg>
  <header deviceID="{device_id}" url="{endpoint}" method="{GET|POST}">
    <request requestID="{n}" msgType="RESPONSE">
      <info type="new"/>
    </request>
  </header>
  <body>
    <!-- response payload -->
  </body>
</msg>
```

**Server → Client push (unsolicited):**
```xml
<updates deviceID="{device_id}">
  <nowPlayingUpdated>...</nowPlayingUpdated>
</updates>
```

`requestID` is a monotonically increasing integer per connection (client-side sequence).
`{device_id}` is the speaker's MAC address with colons removed (e.g. `08DF1F0BA325`).

---

## Phase 1 — Discovery: Is the Speaker Already Paired?

```xml
<!-- C→S: fetch device info -->
<msg><header deviceID="{device_id}" url="info" method="GET">
  <request requestID="1"><info type="new"/></request>
</header></msg>

<!-- S→C: response -->
<info deviceID="{device_id}">
  <name>SoundTouch 10</name>
  <type>SoundTouch 10</type>
  <margeAccountUUID>1000002</margeAccountUUID>   <!-- empty = unpaired -->
  <margeURL>https://streaming.bose.com</margeURL>
  ...
</info>
```

- **Empty `margeAccountUUID`** → device is unpaired, proceed to Phase 2
- **Populated `margeAccountUUID`** → already paired with that account ID

---

## Phase 2 — Pairing a New Speaker

### 2.1 Setup State Machine

The pairing flow uses a setup state machine on the device. States must be sent in order.

```xml
<!-- 1. Start setup -->
<msg><header deviceID="{device_id}" url="setup" method="POST">
  <request requestID="21"></request>
</header><body><setupState state="SETUP_START"/></body></msg>

<!-- 2. Enter identify mode — device flashes/beeps; 300 000 ms timeout -->
<msg><header deviceID="{device_id}" url="setup" method="POST">
  <request requestID="22"></request>
</header><body><setupState state="SETUP_IDENTIFY_DEVICE_ENTER" timeout="300000"/></body></msg>

<!-- Server pushes: -->
<updates deviceID="{device_id}">
  <soundTouchConfigurationUpdated>
    <soundTouchConfigurationStatus status="SOUNDTOUCH_CONFIGURING"/>
  </soundTouchConfigurationUpdated>
</updates>

<!-- 3. Set language (3 = German; adjust as needed) -->
<msg><header deviceID="{device_id}" url="language" method="POST">
  <request requestID="23"></request>
</header><body><sysLanguage>3</sysLanguage></body></msg>

<!-- 4. Enter setup (user has confirmed identification) -->
<msg><header deviceID="{device_id}" url="setup" method="POST">
  <request requestID="24"></request>
</header><body><setupState state="SETUP_ENTER"/></body></msg>

<!-- 5. Leave identify mode -->
<msg><header deviceID="{device_id}" url="setup" method="POST">
  <request requestID="25"></request>
</header><body><setupState state="SETUP_IDENTIFY_DEVICE_LEAVE"/></body></msg>

<!-- 6. Set device name -->
<msg><header deviceID="{device_id}" url="name" method="POST">
  <request requestID="26"></request>
</header><body><name>My SoundTouch 10</name></body></msg>
```

### 2.2 Account Pairing — The Critical Step

```xml
<!-- C→S: pair device with account -->
<msg><header deviceID="{device_id}" url="setMargeAccount" method="POST">
  <request requestID="27"></request>
</header><body>
  <PairDeviceWithAccount>
    <accountId>{accountId}</accountId>
    <userAuthToken>Bearer {token}</userAuthToken>
  </PairDeviceWithAccount>
</body></msg>

<!-- S→C: device info response with margeAccountUUID now set -->
<info deviceID="{device_id}">
  ...
  <margeAccountUUID>{accountId}</margeAccountUUID>
  ...
</info>
```

The server also pushes several `sourcesUpdated` events after successful pairing.

**`{accountId}`** — the numeric Bose account ID (e.g. `1000002`), obtainable from
`GET /streaming/account/login` on soundtouch-service.

**`{token}`** — a Bearer token issued by Bose authentication (or soundtouch-service).
The full token from the captures:
```
Bearer NtJDRbNtY3hDhm5K8FC2JprRhRQNH3QdZjG6aR4ASwYQg4rvZMY6dPLc3Bm6zvWNciWzCpMWZ/dbITRQoVdClOdssgDO+Nlh4ZJWp2w3tZiGzB8Flho0c+ipXnT/0Yg5
```
(session-specific; obtain a fresh one from the service's account login flow)

### 2.3 Finish Setup and Telemetry

```xml
<!-- Leave setup state machine -->
<msg><header deviceID="{device_id}" url="setup" method="POST">
  <request requestID="28"></request>
</header><body><setupState state="SETUP_LEAVE"/></body></msg>

<!-- Trigger device to sync customer support info to Marge cloud -->
<msg><header deviceID="{device_id}" url="pushCustomerSupportInfoToMarge" method="GET">
  <request requestID="29"></request>
</header></msg>

<!-- S→C: -->
<status>/pushCustomerSupportInfoToMarge</status>
```

---

## Phase 3 — Unpairing

```xml
<!-- C→S: remove device from account -->
<msg><header deviceID="{device_id}" url="setMargeAccount" method="POST">
  <request requestID="24">
    <info mainNode="removeDevice" type="new"/>
    <sourceItem source="SETTINGS" sourceAccount="{device_id}"/>
  </request>
</header><body><UnPairDeviceWithAccount/></body></msg>

<!-- S→C: response with device info showing empty margeAccountUUID -->
<!-- Server also pushes: <updates><infoUpdated/></updates> -->
```

---

## Phase 4 — App Initialization (Bulk State Fetch)

When the app connects to an already-paired device it sends these in rapid parallel sequence:

```
info          (GET)  — device metadata, check pairing
sources       (GET)  — available input sources
presets       (GET)  — saved presets 1–6
swUpdateQuery (POST) — check if update is in progress
capabilities  (GET)  — hardware capabilities, network config
bassCapabilities (GET) — bass range and defaults
now_playing   (GET)  — current playback state
volume        (GET)  — current volume
getZone       (GET)  — multi-room zone membership
clockDisplay  (POST) — set clock timezone/format
```

Then a second wave:

```
swUpdateCheck   (POST) — check for new firmware
systemtimeout   (GET)  — power-saving timeout
rebroadcastlatencymode (GET) — zone latency mode
getGroup        (GET)  — stereo-pair group
language        (GET, sourceItem source="settings") — UI language
bass            (GET)  — current bass level
serviceAvailability (GET, sourceItem source="add_service" or "settings")
webserver/pingRequest (GET) — keepalive
pushCustomerSupportInfoToMarge (GET) — telemetry
netStats        (GET, sourceItem source="settings") — network statistics
introspect      (POST, sourceItem source="AIRPLAY") — AirPlay2 capabilities
```

`clockDisplay` example with timezone:
```xml
<clockDisplay>
  <clockConfig timezoneInfo="Europe/Berlin" timeFormat="TIME_FORMAT_12HOUR_ID"/>
</clockDisplay>
```

`serviceAvailability` response lists availability of all service types (PANDORA, AIRPLAY,
AMAZON, DEEZER, SPOTIFY, TUNEIN, SIRIUSXM_EVEREST, BLUETOOTH, etc.) with `isAvailable`
and optional `reason` attributes.

---

## Playback Control

### Start Playback via `playbackRequest` (preferred — bypasses source checks)

```xml
<msg><header deviceID="{device_id}" url="playbackRequest" method="POST">
  <request requestID="{n}"><info type="new"/></request>
</header><body>
  <playbackRequest source="TUNEIN" sourceAccount="">
    <container type="stationurl"
               location="/v1/playback/station/s25260"
               isPresetable="true"
               source="TUNEIN"
               sourceAccount="">
      <itemName>1LIVE</itemName>
    </container>
  </playbackRequest>
</body></msg>

<!-- S→C response: -->
<playbackResponse source="TUNEIN" sourceAccount=""/>

<!-- S→C pushes: nowPlayingUpdated, recentsUpdated -->
```

For a TuneIn podcast episode, use `type="tracklisturl"` and
`location="/v1/playback/episodes/{id}?encoded_name={base64}"`.

### Select Content via `select` (triggers preset/recents UI highlight)

```xml
<msg><header deviceID="{device_id}" url="select" method="POST">
  <request requestID="{n}"><info type="new"/></request>
</header><body>
  <ContentItem source="TUNEIN"
               type="stationurl"
               location="/v1/playback/station/s25260"
               sourceAccount="TUNEIN"
               isPresetable="true">
    <itemName>1LIVE</itemName>
  </ContentItem>
</body></msg>
```

Note: `select` with a TUNEIN item that the device can't resolve directly may return
`error value="1005" name="UNKNOWN_SOURCE_ERROR"`. Use `playbackRequest` instead for
reliable playback.

### Special: Select Last Wi-Fi Source

A plain-text (non-XML) client message:
```
selectLastWiFiSource
```

Server responds with plain text:
```
<?xml version="1.0" encoding="UTF-8" ?><status>/selectLastWiFiSource</status>
```

### Key Presses

```xml
<!-- press -->
<msg><header deviceID="{device_id}" url="key" method="POST">
  <request requestID="{n}"><info mainNode="keyPress" type="new"/><sourceItem source="TUNEIN"/></request>
</header><body><key state="press" sender="Gabbo">{KEY}</key></body></msg>

<!-- release (required for POWER — not for STOP/PAUSE) -->
<msg><header deviceID="{device_id}" url="key" method="POST">
  <request requestID="{n}"><info mainNode="keyRelease" type="new"/><sourceItem source="TUNEIN"/></request>
</header><body><key state="release" sender="Gabbo">{KEY}</key></body></msg>
```

Key names observed: `POWER`, `STOP`, `PAUSE`, `ADD_FAVORITE`

`sender="Gabbo"` is the app identifier string used by all Bose mobile apps.

### Volume

```xml
<!-- Set volume (0–100) -->
<msg><header deviceID="{device_id}" url="volume" method="POST">
  <request requestID="{n}"><info mainNode="volume" type="new"/><sourceItem source="TUNEIN"/></request>
</header><body><volume>30</volume></body></msg>

<!-- S→C push: -->
<updates deviceID="{device_id}">
  <volumeUpdated>
    <volume><targetvolume>30</targetvolume><actualvolume>30</actualvolume><muteenabled>false</muteenabled></volume>
  </volumeUpdated>
</updates>
```

### Bass

```xml
<!-- Get -->
<msg><header deviceID="{device_id}" url="bass" method="GET">
  <request requestID="{n}"><info type="new"/></request>
</header></msg>

<!-- Set (range: bassMin to bassMax from bassCapabilities, typically -9 to 0) -->
<msg><header deviceID="{device_id}" url="bass" method="POST">
  <request requestID="{n}"><info mainNode="bassSet" type="new"/><sourceItem source="SETTINGS"/></request>
</header><body><bass>-2</bass></body></msg>

<!-- S→C push: <updates><bassUpdated/></updates> -->
```

---

## Browse & Navigate

```xml
<!-- Open recents menu -->
<msg><header deviceID="{device_id}" url="navigate" method="POST">
  <request requestID="{n}"><info mainNode="navigateMenu" type="new"/><sourceItem source="RECENTS"/></request>
</header><body><navigate menu="recents"/></body></msg>

<!-- S→C response: -->
<navigateResponse menu="recents">
  <totalItems>4</totalItems>
  <items>
    <item type="stationurl" source="TUNEIN" location="/v1/playback/station/s25260"
          sourceAccount="TUNEIN" isPresetable="true" id="0">
      <itemName>1LIVE</itemName>
    </item>
    ...
  </items>
</navigateResponse>
```

Use `type="update"` on `<info>` for subsequent refresh calls on the same menu.

---

## Settings

### System Timeout (Power-Saving)

```xml
<!-- Read -->
<msg><header deviceID="{device_id}" url="systemtimeout" method="GET">
  <request requestID="{n}"><info type="new"/></request>
</header></msg>

<!-- Write: disable auto power-off -->
<msg><header deviceID="{device_id}" url="systemtimeout" method="POST">
  <request requestID="{n}"><info mainNode="systemtimeout" type="new"/><sourceItem source="SETTINGS"/></request>
</header><body><systemtimeout><powersaving_enabled>false</powersaving_enabled></systemtimeout></body></msg>
```

### Clock Display

```xml
<msg><header deviceID="{device_id}" url="clockDisplay" method="POST">
  <request requestID="{n}"><info mainNode="clockDisplayBypass" type="new"/></request>
</header><body>
  <clockDisplay>
    <clockConfig timezoneInfo="Europe/Berlin" timeFormat="TIME_FORMAT_12HOUR_ID"/>
  </clockDisplay>
</body></msg>
```

---

## Keepalive

The app sends a ping roughly every 30 seconds:

```xml
<!-- C→S -->
<msg><header deviceID="{device_id}" url="webserver/pingRequest" method="GET">
  <request requestID="{n}"><info type="new"/></request>
</header></msg>

<!-- S→C -->
<pingRequest pong="true"/>
```

---

## Server Push Events (Unsolicited)

The server wraps push events in `<updates deviceID="{device_id}">`:

| Event element                    | Trigger                                                                                             |
|----------------------------------|-----------------------------------------------------------------------------------------------------|
| `nowPlayingUpdated`              | Source/track changed, playback state changed                                                        |
| `nowSelectionUpdated`            | Preset slot highlighted (UI selection changed)                                                      |
| `recentsUpdated`                 | Recents list changed                                                                                |
| `presetsUpdated`                 | Preset saved or modified                                                                            |
| `volumeUpdated`                  | Volume changed (any source)                                                                         |
| `bassUpdated`                    | Bass level changed                                                                                  |
| `connectionStateUpdated`         | Wi-Fi signal strength changed (`EXCELLENT_SIGNAL`, `GOOD_SIGNAL`, `MARGINAL_SIGNAL`, `POOR_SIGNAL`) |
| `soundTouchConfigurationUpdated` | Setup state changed (e.g. `SOUNDTOUCH_CONFIGURING`)                                                 |
| `infoUpdated`                    | Device info changed (e.g. after un-pairing)                                                         |
| `sourcesUpdated`                 | Available sources list changed                                                                      |

Separate push (not inside `<updates>`):
```xml
<userActivityUpdate deviceID="{device_id}"/>
```
Sent after any physical or app-initiated user action.

---

## Notification (Client → Device Push)

Used by the app to notify the device of data that has changed on the service side
(e.g. after syncing presets from cloud). Header uses `propagate="false"`:

```xml
<msg>
  <header deviceID="{device_id}" url="notification" method="POST" propagate="false">
    <request requestID="{n}"><info mainNode="presetsUpdated" type="new"/></request>
  </header>
  <body>
    <updates deviceID="{device_id}"><presetsUpdated/></updates>
  </body>
</msg>

<!-- S→C response: -->
<status>/notification</status>
```

---

## Complete Pairing Sequence (Minimal)

To pair a freshly factory-reset speaker to a Bose account (soundtouch-service must be
running and authenticated):

```
1.  Connect WebSocket to ws://{speakerIP}:8090/
2.  Receive: <SoundTouchSdkInfo .../>
3.  GET info  → confirm margeAccountUUID is empty
4.  POST setup SETUP_START
5.  POST setup SETUP_IDENTIFY_DEVICE_ENTER (timeout=300000)
    (user physically presses button on speaker to confirm identity)
6.  POST language <sysLanguage>3</sysLanguage>
7.  POST setup SETUP_ENTER
8.  POST setup SETUP_IDENTIFY_DEVICE_LEAVE
9.  POST name  <name>{desired name}</name>
10. POST setMargeAccount <PairDeviceWithAccount>
      <accountId>{accountId}</accountId>
      <userAuthToken>Bearer {token}</userAuthToken>
    </PairDeviceWithAccount>
    → device responds with info, margeAccountUUID is now set
11. POST setup SETUP_LEAVE
12. GET  pushCustomerSupportInfoToMarge   (telemetry, safe to skip)
```

---

## Source References

- `bose-pairing-20260502-155542` — Session 1: initial pairing of SoundTouch 10 to account 1000002
- `bose-pairing-20260502-165549` — Session 2: re-pairing and full operation (TuneIn, Spotify, presets)
- Raw WebSocket files: `scripts/android/mitm/{session}/mirror/{n}-websocket/*.txt`
- Companion HTTP upgrade files: `scripts/android/mitm/{session}/mirror/{n}-*.http`
