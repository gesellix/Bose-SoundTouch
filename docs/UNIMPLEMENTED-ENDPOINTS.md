# Unimplemented SoundTouch API Endpoints

**Last Updated:** January 2026  
**Source:** [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)  
**Current Implementation:** 35 endpoints (including preset & navigation management discovered via SoundTouch Plus Wiki)  
**Wiki Documentation:** 87 endpoints  
**Implementation Gap:** 52 endpoints

This document provides comprehensive information about SoundTouch API endpoints documented in the community wiki but not yet implemented in this Go library. All examples are based on real device responses and extensive community testing.

---

## Implementation Priority Matrix

### 🔥 Critical Priority (12 endpoints)
Essential user functionality that significantly impacts user experience.

### 🎯 High Priority (13 endpoints)
Smart home integration and advanced user features.

### 📊 Medium Priority (19 endpoints)
Professional features and system administration.

### 🔧 Low Priority (10 endpoints)
Specialized hardware-specific features.

**Note:** 6 critical priority endpoints have been implemented: preset management (storePreset, removePreset) and content navigation/station management (navigate, searchStation, addStation, removeStation). These endpoints were discovered through the comprehensive [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API), which documents additional functionality beyond the official API.

---

## Critical Priority Implementation Candidates

### ~~Preset Management~~ ✅ **IMPLEMENTED**
~~Essential for saving and managing favorite stations and playlists.~~

#### ~~POST /storePreset~~ ✅ **REVERSE-ENGINEERED & IMPLEMENTED**
~~Stores a preset to the device (maximum 6 presets).~~

**Status:** **COMPLETE** - Successfully implemented using endpoints documented in the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API) despite official API marking as "N/A"
- Client methods: `StorePreset()`, `StoreCurrentAsPreset()`, `RemovePreset()`
- CLI commands: `preset store`, `preset store-current`, `preset remove`
- Full content source support: Spotify, TuneIn, local music, etc.
- WebSocket events: Generates `presetsUpdated` notifications
- Production ready: Tested with SoundTouch 10 & 20

**Implementation Notes:** RESOLVED - Special thanks to the SoundTouch Plus community for documenting these working endpoints
- If preset ID exists, overlay existing preset
- If content matches existing preset, move to specified slot
- Maximum 6 presets per device
- Supports all presetable content types

#### ~~POST /removePreset~~ ✅ **IMPLEMENTED**
~~Removes an existing preset from the device.~~

**Status:** **COMPLETE** - Successfully implemented using endpoints from the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- Client method: `RemovePreset(id)`
- CLI command: `preset remove --slot <1-6>`
- Generates `presetsUpdated` WebSocket events
- Production ready and tested

#### ~~GET /selectPreset~~ ✅ **ALREADY AVAILABLE**
~~Selects and plays a preset by ID.~~

**Status:** **AVAILABLE** - Implemented via key commands
- Client method: `SelectPreset(id)` (uses key command approach)
- CLI command: `preset select --slot <1-6>`
- Alternative: Direct key commands (`SendKey("PRESET_1")` etc.)

### Music Service Management
Critical for streaming service integration.

#### POST /setMusicServiceAccount 🔥 **CRITICAL**
Adds a music service account to the sources list.

**Request Examples:**

Pandora Service:
```xml
<credentials source="PANDORA" displayName="Pandora Music Service">
  <user>YourPandoraUserId</user>
  <pass>YourPandoraPassword$1pd</pass>
</credentials>
```

Spotify Service:
```xml
<credentials source="SPOTIFY" displayName="Spotify Premium">
  <user>YourSpotifyUserId</user>
  <pass>YourSpotifyPassword</pass>
</credentials>
```

NAS Music Library:
```xml
<credentials source="STORED_MUSIC" displayName="My NAS Media Library">
  <user>d09708a1-5953-44bc-a413-123456789012/0</user>
  <pass />
</credentials>
```

**Response:**
```xml
<status>/setMusicServiceAccount</status>
```

**Implementation Notes:**
- UPnP media servers must be detected first (check `/listMediaServers`)
- Note the `/0` suffix for STORED_MUSIC user names
- Spotify requires PREMIUM account for most operations

#### POST /removeMusicServiceAccount 🔥 **CRITICAL**
Removes an existing music service account.

**Request Examples:**

Remove Pandora:
```xml
<credentials source="PANDORA" displayName="Pandora Music Service">
  <user>YourPandoraUserId</user>
  <pass />
</credentials>
```

Remove NAS Library:
```xml
<credentials source="STORED_MUSIC" displayName="My NAS Media Library">
  <user>d09708a1-5953-44bc-a413-123456789012/0</user>
  <pass />
</credentials>
```

### ~~Content Discovery and Navigation~~ ✅ **IMPLEMENTED**
~~Essential for browsing music libraries and discovering new content.~~

#### ~~POST /navigate~~ ✅ **IMPLEMENTED**
~~Retrieves child container items from music libraries.~~

**Status:** **COMPLETE** - Full navigation functionality implemented using endpoints documented in the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- Client methods: `Navigate()`, `NavigateWithMenu()`, `NavigateContainer()`
- Helper methods: `GetTuneInStations()`, `GetPandoraStations()`, `GetStoredMusicLibrary()`
- CLI commands: `browse content`, `browse menu`, `browse container`, `browse tunein`, `browse pandora`, `browse stored-music`
- Supports all sources: TUNEIN, PANDORA, SPOTIFY, STORED_MUSIC
- Pagination support with configurable page sizes
- Production ready and tested

#### POST /search 🔥 **CRITICAL**
Searches music library containers.

**Request Examples:**

Search for tracks containing "christmas":
```xml
<search source="STORED_MUSIC" sourceAccount="d09708a1-5953-44bc-a413-123456789012/0">
  <startItem>1</startItem>
  <numItems>1000</numItems>
  <searchTerm filter="track">christmas</searchTerm>
  <item>
    <name>All Music</name>
    <type>dir</type>
    <ContentItem source="STORED_MUSIC" location="4" sourceAccount="d09708a1-5953-44bc-a413-123456789012/0" isPresetable="true" />
  </item>
</search>
```

Search for artists containing "MercyMe":
```xml
<search source="STORED_MUSIC" sourceAccount="d09708a1-5953-44bc-a413-123456789012/0">
  <startItem>1</startItem>
  <numItems>1000</numItems>
  <searchTerm filter="artist">MercyMe</searchTerm>
  <item>
    <name>All Artists</name>
    <type>dir</type>
    <ContentItem source="STORED_MUSIC" location="6" sourceAccount="d09708a1-5953-44bc-a413-123456789012/0" isPresetable="true" />
  </item>
</search>
```

**Response Example:**
```xml
<searchResponse source="STORED_MUSIC" sourceAccount="d09708a1-5953-44bc-a413-123456789012/0">
  <totalItems>142</totalItems>
  <items>
    <item Playable="1">
      <name>Christmas Gift</name>
      <type>track</type>
      <ContentItem source="STORED_MUSIC" location="4-7678 TRACK" sourceAccount="d09708a1-5953-44bc-a413-123456789012/0" isPresetable="true">
        <itemName>Christmas Gift</itemName>
      </ContentItem>
      <artistName>NJS</artistName>
      <albumName>Sound of Night</albumName>
    </item>
  </items>
</searchResponse>
```

### ~~Station Management~~ ✅ **IMPLEMENTED**
~~Pandora and other music service station management.~~

#### ~~POST /searchStation~~ ✅ **IMPLEMENTED**
~~Searches music services for stations to add.~~

**Status:** **COMPLETE** - Full station search functionality implemented using endpoints documented in the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- Client methods: `SearchStation()`, `SearchTuneInStations()`, `SearchPandoraStations()`, `SearchSpotifyContent()`
- CLI commands: `station search`, `station search-tunein`, `station search-pandora`, `station search-spotify`
- Supports all major sources: TUNEIN, PANDORA, SPOTIFY
- Rich result categorization: songs, artists, stations
- Production ready and tested

#### ~~POST /addStation~~ ✅ **IMPLEMENTED**
~~Adds a station to music service collection.~~

**Status:** **COMPLETE** - Station addition and immediate playback implemented using endpoints documented in the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- Client method: `AddStation(source, sourceAccount, token, name)`
- CLI command: `station add --source <SOURCE> --token <TOKEN> --name <NAME>`
- Supports immediate playback after adding
- Works with tokens from search results
- Tested with TuneIn, Pandora, and Spotify

#### ~~POST /removeStation~~ ✅ **IMPLEMENTED**  
~~Removes a station from music service collection.~~

**Status:** **COMPLETE** - Station removal functionality implemented using endpoints documented in the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- Client method: `RemoveStation(contentItem)`
- CLI command: `station remove --source <SOURCE> --location <LOCATION>`
- Handles playback interruption if removed station is playing
- Uses ContentItem from navigation/browse results
- Production ready and tested

### Enhanced Playback Control

#### POST /userPlayControl 🔥 **CRITICAL**
Sends user play control commands.

**Request Example:**
```xml
<PlayControl>PLAY_CONTROL</PlayControl>
```

**Valid Control Values:**
- `PAUSE_CONTROL` - Pause currently playing content
- `PLAY_CONTROL` - Play content that is paused or stopped  
- `PLAY_PAUSE_CONTROL` - Toggle play/pause
- `STOP_CONTROL` - Stop currently playing content

**Response:**
```xml
<status>/userPlayControl</status>
```

#### POST /userRating 🔥 **CRITICAL**
Rates currently playing media (Pandora only).

**Request Example:**
```xml
<Rating>UP</Rating>
```

**Valid Rating Values:**
- `UP` - Thumbs up rating
- `DOWN` - Thumbs down rating (stops current track)

**Response:**
```xml
<status>/userRating</status>
```

### System Information



#### GET /listMediaServers 🔥 **CRITICAL**
Returns detected UPnP/DLNA media servers.

**Response Example:**
```xml
<ListMediaServersResponse>
  <media_server id="2f402f80-da50-11e1-9b23-123456789012" mac="0017886e13fe" ip="192.168.1.4" manufacturer="Signify" model_name="Philips hue bridge 2015" friendly_name="Hue Bridge (192.168.1.4)" model_description="Philips hue Personal Wireless Lighting" location="http://192.168.1.4:80/description.xml" />
  <media_server id="d09708a1-5953-44bc-a413-123456789012" mac="S-1-5-21-240303764-901663538-1234567890-1001" ip="192.168.1.5" manufacturer="Microsoft Corporation" model_name="Windows Media Player Sharing" friendly_name="My NAS Media Library" model_description="" location="http://192.168.1.5:2869/upnphost/udhisapi.dll?content=uuid:d09708a1-5953-44bc-a413-123456789012" />
</ListMediaServersResponse>
```

#### GET /serviceAvailability ✅ **IMPLEMENTED**
Returns source service availability status.

**Implementation Status:** ✅ Complete - Available in `pkg/client/client.go` as `GetServiceAvailability()`

**Response Example:**
```xml
<serviceAvailability>
  <services>
    <service type="AIRPLAY" isAvailable="true" />
    <service type="ALEXA" isAvailable="false" />
    <service type="AMAZON" isAvailable="true" />
    <service type="BLUETOOTH" isAvailable="false" reason="INVALID_SOURCE_TYPE" />
    <service type="BMX" isAvailable="false" />
    <service type="DEEZER" isAvailable="true" />
    <service type="IHEART" isAvailable="true" />
    <service type="LOCAL_INTERNET_RADIO" isAvailable="true" />
    <service type="LOCAL_MUSIC" isAvailable="true" />
    <service type="NOTIFICATION" isAvailable="false" />
    <service type="PANDORA" isAvailable="true" />
    <service type="SPOTIFY" isAvailable="true" />
    <service type="TUNEIN" isAvailable="true" />
  </services>
</serviceAvailability>
```



### Power Management

#### GET /standby 🔥 **CRITICAL**
Places device into standby mode.

**Response:**
```xml
<status>/standby</status>
```

**WebSocket Event:** `nowPlayingUpdated` with source="STANDBY"

#### GET /powerManagement 🔥 **CRITICAL**  
Returns power state and battery capability.

**Response Example:**
```xml
<powerManagementResponse>
  <powerState>FullPower</powerState>
  <battery>
    <capable>false</capable>
  </battery>
</powerManagementResponse>
```

#### GET /lowPowerStandby 🔥 **CRITICAL**
Places device into low-power mode.

**Response:**
```xml
<status>/lowPowerStandby</status>
```

**Implementation Notes:**
- Device stops responding to API calls
- Must physically power on device to recover
- Use for complete power-down scenarios

---

## High Priority Implementation Candidates

### ~~Notification System (ST-10 Series Only)~~ ✅ **IMPLEMENTED**

#### ~~POST /speaker~~ ✅ **IMPLEMENTED**
Plays TTS messages or URL content for notifications.

**CLI Usage:**
```bash
# TTS with multiple languages
soundtouch-cli speaker tts --text "Hello World" --app-key YOUR_KEY --language EN --volume 70

# URL content playback
soundtouch-cli speaker url --url "https://example.com/audio.mp3" --app-key YOUR_KEY --volume 60

# Simple notification beep
soundtouch-cli speaker beep
```

**Go Client Usage:**
```go
// Text-to-Speech
client.PlayTTS("Hello World", "your-app-key", "EN", 70)

// URL content
client.PlayURL("https://example.com/audio.mp3", "your-app-key", "Service", "Message", "Reason", 60)

// Notification beep
client.PlayNotificationBeep()
```

**Implementation Features:**
- ✅ Complete TTS support with multi-language (EN, DE, ES, FR, IT, NL, PT, RU, ZH, JA, etc.)
- ✅ URL content playback with custom metadata
- ✅ Volume control with automatic restoration
- ✅ Comprehensive CLI commands with help system
- ✅ Full validation and error handling
- ✅ Complete test suite and documentation

#### ~~GET /playNotification~~ ✅ **IMPLEMENTED**
Plays a notification beep sound.

**Implementation:**
- ✅ `PlayNotificationBeep()` method
- ✅ CLI command: `soundtouch-cli speaker beep`
- ✅ Proper error handling for unsupported devices

### WiFi Management

#### POST /performWirelessSiteSurvey 🎯 **HIGH**
Gets list of detectable wireless networks.

**Response Example:**
```xml
<PerformWirelessSiteSurveyResponse error="none">
  <items>
    <item ssid="my_wireless_ssid" signalStrength="-58" secure="true">
      <securityTypes>
        <type>wpa_or_wpa2</type>
      </securityTypes>
    </item>
    <item ssid="Imagine" signalStrength="-65" secure="true">
      <securityTypes>
        <type>wpa_or_wpa2</type>
      </securityTypes>
    </item>
  </items>
</PerformWirelessSiteSurveyResponse>
```

#### POST /addWirelessProfile 🎯 **HIGH**
Adds wireless profile configuration.

**Request Example:**
```xml
<addWirelessProfile timeout="30">
  <profile ssid="YourSSIDName" password="YourSSIDPassword" securityType="wpa_or_wpa2"></profile>
</addWirelessProfile>
```

**Security Types:**
- `none` - No security
- `wep` - WEP
- `wpatkip` - WPA/TKIP
- `wpaaes` - WPA/AES
- `wpa2tkip` - WPA2/TKIP
- `wpa2aes` - WPA2/AES
- `wpa_or_wpa2` - WPA/WPA2 (recommended)

**Response:**
```xml
<status>/addWirelessProfile</status>
```

**Setup Process:**
1. Connect to device WiFi (e.g., `Bose ST XX (XXXXXXXX)`)
2. Device has IP 192.0.2.1 during setup
3. Add wireless profile
4. End setup: POST to `/setup` with `<setupState state="SETUP_WIFI_LEAVE" />`

#### GET /getActiveWirelessProfile 🎯 **HIGH**  
Gets current wireless profile configuration.

**Response Example:**
```xml
<GetActiveWirelessProfileResponse>
  <ssid>my_wireless_ssid</ssid>
</GetActiveWirelessProfileResponse>
```

### Bluetooth Management

#### GET /enterBluetoothPairing 🎯 **HIGH**
Enters Bluetooth pairing mode.

**Response:**
```xml
<status>/enterBluetoothPairing</status>
```

**Implementation Notes:**
- Device waits for compatible device to pair
- Bluetooth indicator turns blue when in pairing mode
- Emits ascending tone when pairing completes
- Source immediately switches to BLUETOOTH
- Device name appears in Bluetooth settings within seconds

#### GET /clearBluetoothPaired 🎯 **HIGH**
Clears all Bluetooth pairings.

**Response Example:**
```xml
<BluetoothInfo BluetoothMACAddress="34:15:13:45:2f:93" />
```

**Implementation Notes:**
- All existing pairings are removed
- Previously paired devices can no longer connect
- Must re-pair each device after clearing
- Some devices emit descending tone when cleared

#### GET /bluetoothInfo 🎯 **HIGH**
Returns current Bluetooth configuration.

**Response Example:**
```xml
<status>/clearBluetoothPaired</status>
```

### Language and System Configuration

#### GET /language 🎯 **HIGH**
Returns current device language.

**Response Example:**
```xml
<sysLanguage>3</sysLanguage>
```

**Language Codes:**
- 1 = Danish
- 2 = German  
- 3 = English
- 4 = Spanish
- 5 = French
- 6 = Italian
- 7 = Dutch
- 8 = Swedish
- 9 = Japanese
- 10 = Simplified Chinese
- 11 = Traditional Chinese
- 12 = Korean
- 13 = Thai
- 15 = Czech
- 16 = Finnish
- 17 = Greek
- 18 = Norwegian
- 19 = Polish
- 20 = Portuguese
- 21 = Romanian
- 22 = Russian
- 23 = Slovenian
- 24 = Turkish
- 25 = Hungarian

#### POST /language 🎯 **HIGH**
Sets device language.

**Request Example:**
```xml
<sysLanguage>3</sysLanguage>
```

**Response:**
```xml
<sysLanguage>3</sysLanguage>
```

#### GET /soundTouchConfigurationStatus 🎯 **HIGH**
Returns device configuration status.

**Response Example:**
```xml
<SoundTouchConfigurationStatus status="SOUNDTOUCH_CONFIGURED" />
```

**Valid Status Values:**
- `SOUNDTOUCH_CONFIGURED` - Device configuration complete
- `SOUNDTOUCH_NOT_CONFIGURED` - Device not configured
- `SOUNDTOUCH_CONFIGURING` - Configuration in progress

### Software Update Management

#### GET /swUpdateCheck 🎯 **HIGH**
Gets latest available software update information.

**Response Example:**
```xml
<swUpdateCheckResponse deviceID="1004567890AA" indexFileUrl="https://worldwide.bose.com/updates/soundtouch">
  <release revision="27.0.6.46330.5043500" />
</swUpdateCheckResponse>
```

#### GET /swUpdateQuery 🎯 **HIGH**
Gets status of software update process.

**Response Example:**
```xml
<swUpdateQueryResponse deviceID="1004567890AA">
  <state>IDLE</state>
  <percentComplete>0</percentComplete>
  <canAbort>false</canAbort>
</swUpdateQueryResponse>
```

**Update States:**
- `IDLE` - No update in progress
- `DOWNLOADING` - Downloading update
- `INSTALLING` - Installing update
- `ERROR` - Update failed

---

## Medium Priority Implementation Candidates

### Source Selection Shortcuts

#### GET /selectLastSource 📊 **MEDIUM**
Selects the last source that was active.

**Response:**
```xml
<status>/selectLastSource</status>
```

#### GET /selectLastSoundTouchSource 📊 **MEDIUM**  
Selects last SoundTouch source.

**Response:**
```xml
<status>/selectLastSoundTouchSource</status>
```

#### GET /selectLastWiFiSource 📊 **MEDIUM**
Selects last WiFi source.

**Response:**
```xml
<status>/selectLastWiFiSource</status>
```

#### GET /selectLocalSource 📊 **MEDIUM**
Selects LOCAL source (only way to select LOCAL on some devices).

**Response:**
```xml
<status>/selectLocalSource</status>
```

### Group Management (ST-10 Stereo Pairs Only)

#### GET /getGroup 📊 **MEDIUM**
Gets current stereo pair configuration.

**Response Example (paired):**
```xml
<group id="1115893">
  <name>Bose-ST10-1 + Bose-ST10-4</name>
  <masterDeviceId>9070658C9D4A</masterDeviceId>
  <roles>
    <groupRole>
      <deviceId>9070658C9D4A</deviceId>
      <role>LEFT</role>
      <ipAddress>192.168.1.131</ipAddress>
    </groupRole>
    <groupRole>
      <deviceId>F45EAB3115DA</deviceId>
      <role>RIGHT</role>
      <ipAddress>192.168.1.134</ipAddress>
    </groupRole>
  </roles>
  <senderIPAddress>192.168.1.131</senderIPAddress>
  <status>GROUP_OK</status>
</group>
```

**Response Example (not paired):**
```xml
<group />
```

#### POST /addGroup 📊 **MEDIUM**
Creates new stereo pair group.

**Request Example:**
```xml
<group>
  <name>Bose-ST10-1 + Bose-ST10-4</name>
  <masterDeviceId>9070658C9D4A</masterDeviceId>
  <roles>
    <groupRole>
      <deviceId>9070658C9D4A</deviceId>
      <role>LEFT</role>
      <ipAddress>192.168.1.131</ipAddress>
    </groupRole>
    <groupRole>
      <deviceId>F45EAB3115DA</deviceId>
      <role>RIGHT</role>
      <ipAddress>192.168.1.134</ipAddress>
    </groupRole>
  </roles>
</group>
```

**Response:** Same as GET /getGroup  
**WebSocket Event:** `groupUpdated` sent to both devices

#### GET /removeGroup 📊 **MEDIUM**
Removes existing stereo pair group.

**Response:**
```xml
<group />
```

**WebSocket Event:** `groupUpdated` sent to both devices

#### POST /updateGroup 📊 **MEDIUM**
Updates stereo pair group name.

**Request Example:**
```xml
<group id="1116267">
  <name>Bose-ST10-1 + Bose-ST10-4 Group</name>
  <masterDeviceId>9070658C9D4A</masterDeviceId>
  <roles>
    <groupRole>
      <deviceId>9070658C9D4A</deviceId>
      <role>LEFT</role>
      <ipAddress>192.168.1.131</ipAddress>
    </groupRole>
    <groupRole>
      <deviceId>F45EAB3115DA</deviceId>
      <role>RIGHT</role>
      <ipAddress>192.168.1.134</ipAddress>
    </groupRole>
  </roles>
</group>
```

### Advanced System Information

#### GET /systemtimeout 📊 **MEDIUM**
Gets current system timeout configuration.

**Response Example:**
```xml
<systemtimeout>
  <powersaving_enabled>true</powersaving_enabled>
</systemtimeout>
```

#### GET /rebroadcastlatencymode 📊 **MEDIUM**
Gets current rebroadcast latency mode.

**Response Example:**
```xml
<rebroadcastlatencymode mode="SYNC_TO_ZONE" controllable="true" />
```

#### GET /DSPMonoStereo 📊 **MEDIUM**
Gets digital signal processor configuration.

**Response Example:**
```xml
<DSPMonoStereo deviceID="1004567890AA">
  <mono enable="false" />
</DSPMonoStereo>
```

#### GET /netStats 📊 **MEDIUM**
Returns network status configuration.

**Response Example:**
```xml
<network-data>
  <devices>
    <device deviceID="1004567890AA">
      <deviceSerialNumber>P7277179802731234567890</deviceSerialNumber>
      <interfaces>
        <interface>
          <name>eth0</name>
          <mac-addr>1004567890AA</mac-addr>
          <bindings>
            <ipv4address>192.168.1.131</ipv4address>
          </bindings>
          <running>true</running>
          <kind>Wireless</kind>
          <ssid>my_network_ssid</ssid>
          <rssi>Good</rssi>
          <frequencyKHz>2452000</frequencyKHz>
        </interface>
      </interfaces>
    </device>
  </devices>
</network-data>
```

---

## Low Priority / Specialized Endpoints

### Advanced Audio Features (ST-300 Hardware-Specific)

#### GET /audiospeakerattributeandsetting 🔧 **LOW**
Returns speaker attribute configuration.

**Response Example:**
```xml
<audiospeakerattributeandsetting>
  <rear available="false" active="false" wireless="false" controllable="true" />
  <subwoofer01 available="true" active="true" wireless="true" controllable="true" />
</audiospeakerattributeandsetting>
```

#### GET /productcechdmicontrol 🔧 **LOW**
Gets HDMI CEC control configuration (ST-300 only).

#### POST /productcechdmicontrol 🔧 **LOW**
Sets HDMI CEC control configuration (ST-300 only).

#### GET /producthdmiassignmentcontrols 🔧 **LOW**
Gets HDMI assignment controls configuration (ST-300 only).

#### POST /producthdmiassignmentcontrols 🔧 **LOW**
Sets HDMI assignment controls configuration (ST-300 only).

### System Administration Features

#### POST /swUpdateStart 🔧 **LOW**
Starts software update process.

**Response:**
```xml
<status>/swUpdateStart</status>
```

#### POST /swUpdateAbort 🔧 **LOW**
Aborts software update process.

**Response:**
```xml
<status>/swUpdateAbort</status>
```

#### GET /criticalError 🔧 **LOW**
Gets critical error information.

#### POST /factoryDefault 🔧 **LOW**
Performs factory reset of device.

**Warning:** This completely resets the device to factory defaults.

---

## Implementation Guidelines

### Device Compatibility Matrix

| Endpoint | ST-10 | ST-300 | ST-20 | ST-520 | Notes |
|----------|-------|--------|-------|--------|-------|
| `/playNotification` | ✅ | ❌ | ❌ | ❌ | ST-10 III series only |
| `/speaker` | ✅ | ❌ | ❌ | ❌ | ST-10 III series only |
| `/audiodspcontrols` | ❌ | ✅ | ❌ | ✅ | Soundbar products |
| `/audioproducttonecontrols` | ❌ | ✅ | ❌ | ✅ | Advanced audio devices |
| `/getGroup` | ✅ | ❌ | ❌ | ❌ | Stereo pair support |
| `/productcechdmicontrol` | ❌ | ✅ | ❌ | ❌ | HDMI-enabled devices |

### Error Handling Best Practices

#### Capability Checking
```go
// Always check capabilities before calling advanced features
capabilities, err := client.GetCapabilities()
if err != nil {
    return fmt.Errorf("failed to get capabilities: %w", err)
}

if !capabilities.SupportsFeature("audiodspcontrols") {
    return ErrFeatureNotSupported
}
```

#### Timeout Handling
```go
// Some endpoints timeout on unsupported devices
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := client.makeRequestWithTimeout(ctx, endpoint); err != nil {
    if errors.Is(err, context.DeadlineExceeded) {
        return ErrEndpointNotSupported
    }
    return err
}
```

#### Graceful Degradation
```go
// Provide fallback functionality when advanced features unavailable
if err := client.PlayNotification(); err != nil {
    if errors.Is(err, ErrFeatureNotSupported) {
        // Fallback to volume beep or other notification method
        return client.SendKeyPress("VOLUME_UP")
    }
    return err
}
```

### WebSocket Events Generated

Many POST operations generate corresponding WebSocket events:

| Operation | WebSocket Event | Content |
|-----------|-----------------|---------|
| ✅ `storePreset` | ✅ `presetsUpdated` | Updated preset list (IMPLEMENTED) |
| ✅ `removePreset` | ✅ `presetsUpdated` | Updated preset list (IMPLEMENTED) |
| `addGroup` | `groupUpdated` | Stereo pair configuration |
| `removeGroup` | `groupUpdated` | Stereo pair configuration |
| `userPlayControl` | `nowPlayingUpdated` | Playback state changes |
| ✅ `addStation` | None | Station immediately plays (IMPLEMENTED) |
| ✅ `removeStation` | `nowPlayingUpdated` | If removed station was playing (IMPLEMENTED) |

### Security and Authentication

#### App Key Requirements
```xml
<!-- TTS and URL playback require app_key -->
<play_info>
  <url>...</url>
  <app_key>YourApplicationKey</app_key>
  <!-- other fields -->
</play_info>
```

#### Bearer Token Usage
```go
// Use existing token system for authenticated requests
token, err := client.RequestToken()
if err != nil {
    return err
}
client.SetAuthToken(token.Value)
```

### Music Service Specifics

#### Pandora Integration
- ✅ **Station Management**: Search, add, remove stations
- ✅ **Ratings**: Thumbs up/down support
- ✅ **Navigation**: Browse station collections
- ⚠️ **Account Setup**: Requires valid Pandora credentials

#### Spotify Integration  
- ✅ **Premium Required**: Most operations require Spotify Premium
- ✅ **URI Support**: Full spotify:// URI support
- ✅ **Playlists**: Access to user playlists and saved music
- ⚠️ **Account Setup**: OAuth flow recommended

#### NAS/DLNA Libraries
- ✅ **UPnP Discovery**: Automatic media server detection  
- ✅ **Navigation**: Full folder/album/artist browsing
- ✅ **Search**: Track, artist, album search within libraries
- ⚠️ **Setup Required**: Windows Media Player sharing or UPnP server

### Testing Strategy

#### Real Device Testing
```go
var deviceTests = []struct {
    model     string
    endpoint  string
    supported bool
}{
    {"ST-10", "/playNotification", true},
    {"ST-300", "/playNotification", false},
    {"ST-300", "/audiodspcontrols", true},
    {"ST-10", "/audiodspcontrols", false},
}

func TestDeviceCompatibility(t *testing.T) {
    for _, tt := range deviceTests {
        t.Run(fmt.Sprintf("%s_%s", tt.model, tt.endpoint), func(t *testing.T) {
            // Test endpoint on specific device model
        })
    }
}
```

#### Integration Testing
- Unit tests for XML marshaling/unmarshaling
- Real device validation for each endpoint
- WebSocket event verification  
- Error scenario testing

---

## Implementation Priority Recommendations

### Phase 1: Essential Features (4 weeks)
1. ✅ **Preset Management**: ~~`storePreset`, `removePreset`, `selectPreset`~~ (IMPLEMENTED)
2. **Music Services**: `setMusicServiceAccount`, `removeMusicServiceAccount`  
3. ✅ **Content Discovery**: ~~`navigate`, `search`~~ (IMPLEMENTED), `recents`
4. ✅ **Station Management**: ~~`searchStation`, `addStation`, `removeStation`~~ (IMPLEMENTED)
5. **Enhanced Controls**: `userPlayControl`, `userRating`

### Phase 2: Smart Home Integration (3 weeks)
1. **Power Management**: `standby`, `powerManagement`, `lowPowerStandby`
2. **Notifications**: `speaker`, `playNotification` 
3. **Network Management**: `performWirelessSiteSurvey`, `addWirelessProfile`
4. **System Info**: ~~`serviceAvailability`~~ (✅ implemented), `listMediaServers`, `language`

### Phase 3: Advanced Features (3 weeks)
1. **Bluetooth**: `enterBluetoothPairing`, `clearBluetoothPaired`
2. **Software Updates**: `swUpdateCheck`, `swUpdateQuery`
3. **Stereo Pairs**: `getGroup`, `addGroup`, `removeGroup`, `updateGroup`
4. **Source Shortcuts**: `selectLastSource`, `selectLastSoundTouchSource`

### Phase 4: Specialized Features (2 weeks)
1. **HDMI Controls**: `productcechdmicontrol`, `producthdmiassignmentcontrols`
2. **System Administration**: `factoryDefault`, `criticalError`
3. **Audio Processing**: `audiospeakerattributeandsetting`, `DSPMonoStereo`

---

## Success Metrics

### Functionality Coverage
- ✅ **87 total endpoints** (from 23 current → 87 wiki documented)
- ✅ **Complete music service integration** (Pandora, Spotify, NAS)
- ✅ **Smart home automation ready** (power, notifications, network)
- ✅ **Professional audio features** (advanced controls, HDMI)

### Quality Assurance
- ✅ **Real device testing** on multiple SoundTouch models
- ✅ **Comprehensive error handling** with graceful degradation
- ✅ **Complete documentation** with XML examples
- ✅ **WebSocket event integration** for real-time updates

### Developer Experience
- ✅ **Type-safe Go implementations** for all endpoints
- ✅ **Device capability checking** before endpoint calls
- ✅ **Production-ready examples** from community wiki
- ✅ **Backward compatibility** with existing implementations

---

## Conclusion

The SoundTouch Plus Wiki provides comprehensive documentation for **64 additional endpoints** that can transform this Go library from basic device control to complete SoundTouch ecosystem management.

### Key Benefits:
- 🎯 **3.8x API Coverage**: From 23 to 87 endpoints
- 🏠 **Complete Smart Home Integration**: Power, notifications, network management  
- 🎵 **Full Music Service Support**: Spotify, Pandora, NAS libraries
- ✅ **Production-Ready**: Real-world tested XML examples
- 📚 **Comprehensive Documentation**: Device compatibility matrix and examples

### Implementation Path:
1. **Start with high-impact user features** (presets, music services)
2. **Add smart home integration** (power, notifications, network)
3. **Include advanced features** (stereo pairs, updates, system admin)
4. **Maintain quality** through real device testing and comprehensive error handling

This documentation provides the complete foundation for implementing all endpoints from the SoundTouch Plus Wiki, enabling this Go library to become the definitive SoundTouch integration solution for everything from basic home automation to professional audio installations.

*All examples and XML structures are verified against real SoundTouch hardware and extensively tested by the SoundTouch Plus community.*
