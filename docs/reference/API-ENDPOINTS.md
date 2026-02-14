# Bose SoundTouch Web API - Endpoints Overview

This document provides a comprehensive overview of the available API endpoints verified against the official Bose SoundTouch Web API v1.0 specification (January 7, 2026).

**Acknowledgment**: Additional endpoints beyond the official API were discovered through the comprehensive [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API) maintained by the SoundTouch Plus community. Special thanks to @thlucas1 and contributors for documenting these working endpoints that enable full preset management and content navigation functionality.

## Implementation Status Legend
- ✅ **Implemented** - Fully implemented with tests and real device validation
- 🔍 **Extra** - Implemented but not in official API v1.0 (may be newer version or undocumented)
- ⚠️ **Different** - Implemented with different approach than official API
- ℹ️ **N/A** - Documented but officially unsupported or non-functional on real hardware

## API Basics

- **Protocol**: HTTP REST-like
- **Data Format**: XML Request/Response
- **Standard Port**: 8090
- **Base URL**: `http://<device-ip>:8090/`
- **Authentication**: No complex authentication required
- **Real-time Updates**: WebSocket connection available

## Device Information

### GET /info ✅ **Implemented**
Retrieves basic device information.

**Response XML Structure:**
```xml
<info deviceID="..." type="..." name="..." ...>
  <name>Device Name</name>
  <type>Device Type</type>
  <margeAccountUUID>UUID</margeAccountUUID>
  <components>...</components>
</info>
```

## Playback Control

### GET /now_playing ✅ **Implemented**
Retrieves information about the currently playing music.

**Response XML Structure:**
```xml
<nowPlaying deviceID="..." source="...">
  <ContentItem source="..." type="..." location="..." sourceAccount="...">
    <itemName>Track Name</itemName>
    <containerArt>Album Art URL</containerArt>
  </ContentItem>
  <track>Track Name</track>
  <artist>Artist Name</artist>
  <album>Album Name</album>
  <stationName>Station Name</stationName>
  <art artImageStatus="...">Art URL</art>
  <playStatus>PLAY_STATE</playStatus>
  <shuffleSetting>...</shuffleSetting>
  <repeatSetting>...</repeatSetting>
</nowPlaying>
```

### POST /key ✅ **Implemented**
Sends key commands to the device.

**IMPORTANT - Key values, state, and sender attributes are CaSe-SeNsItIvE!**

**Important**: Proper key simulation requires sending both press and release states:

**Request XML (Press + Release):**
```xml
<key state="press" sender="Gabbo">KEY_NAME</key>
<key state="release" sender="Gabbo">KEY_NAME</key>
```

**Response XML:**
```xml
<status>/key</status>
```

**Available Keys:**

**Playback Controls:**
- `PLAY` - Start playback
- `PAUSE` - Pause current playback
- `STOP` - Stop current playback
- `PREV_TRACK` - Go to previous track
- `NEXT_TRACK` - Go to next track
- `PLAY_PAUSE` - Toggles between play and pause for currently playing media

**Rating and Bookmark Controls:**
- `THUMBS_UP` - Rate current content positively (Pandora, Spotify, etc.)
- `THUMBS_DOWN` - Rate current content negatively (Pandora, Spotify, etc.)
- `BOOKMARK` - Bookmark current content
- `ADD_FAVORITE` - Adds currently playing media to device favorites (Pandora, Spotify, etc.)
- `REMOVE_FAVORITE` - Removes currently playing media from device favorites (Pandora, Spotify, etc.)

**Power and System Controls:**
- `POWER` - Toggle device power state
- `MUTE` - Toggle mute state

**Volume Controls:**
- `VOLUME_UP` - Increase volume
- `VOLUME_DOWN` - Decrease volume

**Preset Controls:**
- `PRESET_1` to `PRESET_6` - Select preset 1-6

**Input Controls:**
- `AUX_INPUT` - Switch to auxiliary input

**Shuffle Controls:**
- `SHUFFLE_OFF` - Turn shuffle mode off
- `SHUFFLE_ON` - Turn shuffle mode on

**Repeat Controls:**
- `REPEAT_OFF` - Turn repeat mode off
- `REPEAT_ONE` - Repeat current track
- `REPEAT_ALL` - Repeat all tracks in playlist

**State Values:**
- `press` - Indicates the key is pressed
- `release` - Indicates the key is released
- `repeat` - Indicates the key is repeated

**Sender Values:**
- `Gabbo` - Default value for standard SoundTouch remote control device
- `IrRemote` - IR remote control device
- `Console` - Console device
- `LightswitchRemote` - Lightswitch remote device
- `BoselinkRemote` - Boselink remote device
- `Etap` - Etap device

## Volume Control

### GET /volume ✅ **Implemented**
Retrieves the current volume.

**Response XML:**
```xml
<volume deviceID="...">
  <targetvolume>50</targetvolume>
  <actualvolume>50</actualvolume>
  <muteenabled>false</muteenabled>
</volume>
```

### POST /volume ✅ **Implemented**
Sets the volume.

**Request XML:**
```xml
<volume>50</volume>
```

## Bass Settings

### GET /bass ✅ **Implemented**
Retrieves the current bass settings.

**Response XML:**
```xml
<bass deviceID="...">
  <targetbass>0</targetbass>
  <actualbass>0</actualbass>
</bass>
```

### POST /bass ✅ **Implemented**
Sets the bass settings. Range varies by device - check `/bassCapabilities` for supported range.

**Request XML:**
```xml
<bass>0</bass>
```

**Note**: Value must be within the range specified by `bassMin` and `bassMax` from `/bassCapabilities` service.

## Source Management

### GET /sources ✅ **Implemented**
Retrieves the available audio sources.

**Response XML:**
```xml
<sources deviceID="...">
  <sourceItem source="SPOTIFY" sourceAccount="..." status="READY" multiroomallowed="true">
    <itemName>Spotify</itemName>
  </sourceItem>
  <sourceItem source="BLUETOOTH" status="READY" multiroomallowed="false">
    <itemName>Bluetooth</itemName>
  </sourceItem>
  <!-- Additional sources -->
</sources>
```

**Typical Sources:**
- `SPOTIFY`
- `AMAZON`
- `PANDORA`
- `IHEARTRADIO`
- `TUNEIN`
- `BLUETOOTH`
- `AUX`
- `STORED_MUSIC`

### POST /select ✅ **Implemented**
Selects an audio source.

**Request XML:**
```xml
<ContentItem source="SPOTIFY" sourceAccount="...">
  <itemName>Spotify</itemName>
</ContentItem>
```

## Preset Management

### GET /presets ✅ **Implemented**
Retrieves the configured presets.

**Response XML:**
```xml
<presets deviceID="...">
  <preset id="1" createdOn="..." updatedOn="...">
    <ContentItem source="..." sourceAccount="..." location="...">
      <itemName>Preset Name</itemName>
      <containerArt>Art URL</containerArt>
    </ContentItem>
  </preset>
  <!-- Additional presets -->
</presets>
```

### POST /storePreset ✅ **IMPLEMENTED**
Creates or updates a preset.

**Status**: While the official Bose SoundTouch API documentation marks POST `/presets` as "N/A", we discovered and implemented the actual working endpoint `/storePreset` through the comprehensive [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API). This enables full preset management functionality.

**Implementation**: 
- Client method: `StorePreset(id, contentItem)`, `StoreCurrentAsPreset(id)`  
- CLI: `preset store`, `preset store-current`
- Supports all content sources: Spotify, TuneIn, local music, etc.

**XML Request**:
```xml
<preset id="1" createdOn="1640995200" updatedOn="1640995200">
  <ContentItem source="SPOTIFY" type="uri" location="spotify:playlist:123" isPresetable="true">
    <itemName>My Playlist</itemName>
  </ContentItem>
</preset>
```

**Response**: Updated preset configuration

### POST /removePreset ✅ **IMPLEMENTED**
Removes/clears a preset slot.

**Implementation**:
- Client method: `RemovePreset(id)`
- CLI: `preset remove --slot <1-6>`
- WebSocket events: Triggers `presetsUpdated` notifications

**XML Request**:
```xml
<preset id="3"/>
```

**Alternative Methods**:
- Use the official Bose SoundTouch mobile app
- Use physical preset buttons on the device (long-press while content is playing)
- Changes made via these methods will be visible through the GET endpoint

## Advanced Features

### GET /getZone ✅ **Implemented**
Retrieves multiroom zone information.

### POST /setZone ✅ **Implemented**
Configures multiroom zones.

### GET /balance ✅ **Implemented**
Retrieves balance settings (stereo devices). Only works if device is configured as part of a stereo pair.

**Response XML:**
```xml
<balance deviceID="...">
  <balanceAvailable>true</balanceAvailable>
  <balanceMin>-7</balanceMin>
  <balanceMax>7</balanceMax>
  <balanceDefault>0</balanceDefault>
  <targetBalance>0</targetBalance>
  <actualBalance>0</actualBalance>
</balance>
```

### POST /balance ✅ **Implemented**
Sets balance settings. Value must be within the range specified by `balanceMin` and `balanceMax`.

**Request XML:**
```xml
<balance>
  <targetBalance>0</targetBalance>
</balance>
```

**Range Examples:**
- `-7` = left speaker
- `0` = centered
- `7` = right speaker

### GET /clockTime ✅ **Implemented**
Retrieves the device time.

**Response XML:**
```xml
<clockTime utcTime="1701824606" cueMusic="0" timeFormat="TIME_FORMAT_12HOUR_ID" brightness="70" clockError="0" utcSyncTime="1701820350">
  <localTime year="2023" month="11" dayOfMonth="5" dayOfWeek="2" hour="19" minute="3" second="26" />
</clockTime>
```

### POST /clockTime ✅ **Implemented**
Sets the device time.

### GET /clockDisplay ✅ **Implemented**
Retrieves clock display settings.

**Response XML:**
```xml
<clockDisplay>
  <clockConfig timezoneInfo="America/Chicago" userEnable="false" timeFormat="TIME_FORMAT_12HOUR_ID" userOffsetMinute="0" brightnessLevel="70" userUtcTime="0" />
</clockDisplay>
```

### POST /clockDisplay ✅ **Implemented**
Configures the clock display.

### POST /speaker ✅ **Implemented**
Plays TTS messages or URL content for notifications (ST-10 Series only).

**TTS Request XML:**
```xml
<play_info>
  <url>http://translate.google.com/translate_tts?ie=UTF-8&amp;tl=EN&amp;client=tw-ob&amp;q=Hello%20World</url>
  <app_key>YOUR_APPLICATION_KEY</app_key>
  <service>TTS Notification</service>
  <message>Google TTS</message>
  <reason>Hello World</reason>
  <volume>70</volume>
</play_info>
```

**URL Content Request XML:**
```xml
<play_info>
  <url>https://example.com/audio.mp3</url>
  <app_key>YOUR_APPLICATION_KEY</app_key>
  <service>Music Service</service>
  <message>Song Title</message>
  <reason>Artist Name</reason>
  <volume>60</volume>
</play_info>
```

**Response XML:**
```xml
<status>/speaker</status>
```

**Implementation Features:**
- Multi-language TTS support (EN, DE, ES, FR, IT, NL, PT, RU, ZH, JA, etc.)
- Volume control with automatic restoration
- Custom metadata for NowPlaying display
- Pauses current content, plays notification, then resumes

### GET /playNotification ✅ **Implemented**
Plays a notification beep sound (ST-10 Series only).

**Response XML:**
```xml
<status>/playNotification</status>
```

**Implementation:**
- Simple double beep sound
- Pauses current media during beep
- Available via `PlayNotificationBeep()` method

## WebSocket Connection

### WebSocket / ✅ **Implemented**
Establishes a persistent connection for live updates.

**Event Types:**
- `nowPlayingUpdated`
- `volumeUpdated`
- `connectionStateUpdated`
- `presetUpdated`

## Network and System

### GET /networkInfo ✅ **Implemented**
Retrieves network information.

**Response XML:**
```xml
<networkInfo wifiProfileCount="1">
  <interfaces>
    <interface type="WIFI_INTERFACE" name="wlan0" macAddress="..." ipAddress="192.168.1.131" ssid="network_name" frequencyKHz="2452000" state="NETWORK_WIFI_CONNECTED" signal="MARGINAL_SIGNAL" mode="STATION" />
    <interface type="WIFI_INTERFACE" name="wlan1" macAddress="..." state="NETWORK_WIFI_DISCONNECTED" />
  </interfaces>
</networkInfo>
```

### GET /capabilities ✅ **Implemented**
Retrieves device capabilities.

### GET /name 🔍 **Extra**
Retrieves the device name.

**Response XML:**
```xml
<name>SoundTouch 10</name>
```

**Note**: Official API only documents `POST /name` for setting device name. Our GET implementation appears to be an undocumented extension.

### POST /name ✅ **Implemented**
Sets the device name via `SetName()` method. If name is changed, the change will be detected immediately via ZeroConf services.

**Request XML:**
```xml
<name>SoundTouch Living Room</name>
```

**Response**: Returns same structure as `/info` endpoint with updated name.

### GET /bassCapabilities ✅ **Implemented**
Checks if bass customization is supported on the device.

**Official Response Format:**
```xml
<bassCapabilities deviceID="$MACADDR">
    <bassAvailable>$BOOL</bassAvailable>
    <bassMin>$INT</bassMin>
    <bassMax>$INT</bassMax>
    <bassDefault>$INT</bassDefault>
</bassCapabilities>
```

### GET /trackInfo ✅ **Implemented**
Gets extended track information for currently playing music service media.

**Response XML:**
```xml
<trackInfo deviceID="...">Track Name;extended details;separated by semicolons;</trackInfo>
```

**Important Notes:**
- Only returns information if currently playing content is from a music service (PANDORA, SPOTIFY, etc.)
- If playing non-music-service content (AIRPLAY, STORED_MUSIC, etc.), service becomes unresponsive for ~30 seconds until timeout
- Extended details are delimited by semicolons (e.g., "Who You Are To Me (feat. Lady A);vocal duets;upbeat lyrics;")
- Times out on some SoundTouch models - use `/now_playing` as reliable alternative

**Implementation**: Available via `GetTrackInfo()` method. Consider using `GetNowPlaying()` method for guaranteed compatibility.

### Zone Slave Management ✅ **Implemented**
Both official low-level endpoints and high-level zone management are available:

#### POST /addZoneSlave ✅ **Implemented**
Add individual device to existing zone using official API format.

**Implementation**: Available via `AddZoneSlave()` and `AddZoneSlaveByDeviceID()` methods

#### POST /removeZoneSlave ✅ **Implemented** 
Remove individual device from existing zone using official API format.

**Implementation**: Available via `RemoveZoneSlave()` and `RemoveZoneSlaveByDeviceID()` methods

#### High-Level Zone API ✅ **Enhanced**
- **Enhanced**: `CreateZone()`, `AddToZone()`, `RemoveFromZone()` methods via `/setZone`
- **Status**: Provides both official low-level API and enhanced high-level operations

### Advanced Audio Controls ✅ **Conditionally Available**
Professional/high-end device features (only available on devices that list these capabilities):

#### `/audiodspcontrols` - GET/POST ✅ **Implemented**
Access DSP settings including audio modes and video sync delay.

**Availability**: Only available if `audiodspcontrols` is listed in the reply to `GET /capabilities`

**Implementation**: Available via `GetAudioDSPControls()`, `SetAudioDSPControls()`, `SetAudioMode()`, `SetVideoSyncAudioDelay()` methods with automatic capability checking

#### `/audioproducttonecontrols` - GET/POST ✅ **Implemented**
Advanced bass and treble controls (beyond basic `/bass` endpoint).

**Availability**: Only available if `audioproducttonecontrols` is listed in the reply to `GET /capabilities`

**Implementation**: Available via `GetAudioProductToneControls()`, `SetAudioProductToneControls()`, `SetAdvancedBass()`, `SetAdvancedTreble()` methods with automatic capability checking

#### `/audioproductlevelcontrols` - GET/POST ✅ **Implemented**
Speaker level controls for front-center and rear-surround speakers.

**Availability**: Only available if `audioproductlevelcontrols` is listed in the reply to `GET /capabilities`

**Implementation**: Available via `GetAudioProductLevelControls()`, `SetAudioProductLevelControls()`, `SetFrontCenterSpeakerLevel()`, `SetRearSurroundSpeakersLevel()` methods with automatic capability checking

### Clock and Network Endpoints 🔍 **Extra**
These endpoints work with real hardware but are NOT in official API v1.0:
- `GET/POST /clockTime` ✅ **Implemented** - Device time management
- `GET/POST /clockDisplay` ✅ **Implemented** - Clock display settings  
- `GET /networkInfo` ✅ **Implemented** - Network information

### Balance Control 🔍 **Extra**
- `GET/POST /balance` ✅ **Implemented** - Stereo balance adjustment

**Note**: Not documented in official API v1.0 but works with real devices.

### Token Management ✅ **Implemented**

#### GET /requestToken ✅ **Implemented**
Generates a new bearer token from the device for authentication purposes.

**Response XML:**
```xml
<bearertoken value="Bearer vUApzBVT6Lh0nw1xVu/plr1UDRNdMYMEpe0cStm4wCH5mWSjrrtORnGGirMn3pspkJ8mNR1MFh/J4OcsbEikMplcDGJVeuZOnDPAskQALvDBCF0PW74qXRms2k1AfLJ/" />
```

**Usage:**
- Tokens are generated per request and may have expiration times
- Use for HTTP Authorization headers: `Authorization: Bearer <token>`
- Store tokens securely and treat as passwords
- Request new tokens when needed rather than reusing old ones

**Implementation**: Available via `RequestToken()` method

**Testing**: Integration tests available - run with `SOUNDTOUCH_TEST_HOST=<device-ip> go test ./pkg/client -run TestRequestToken_Integration` to validate real device token generation without exposing token values

## Coverage Summary

### Official API Coverage: 100%
- **Total Official Endpoints**: 19
- **Implemented**: 19 (100%)
- **Conditionally Available**: 3 (16%) - Advanced audio endpoints require device support
- **Device-Dependent**: 1 (5%) - GET /trackInfo times out on some models
- **Excluded**: 1 endpoint (POST /presets officially N/A)

### Real Device Discovery: 103 Endpoints Found
- **Total Discovered Endpoints**: 103 (from /supportedURLs)
- **Currently Implemented**: ~35 (34%)
- **Core Functionality**: 100% implemented
- **Extended Features**: Many undocumented endpoints available
- **Implementation Focus**: User-facing and essential system endpoints prioritized

### Feature Coverage: 100%
- ✅ All available user functionality implemented
- ✅ All functional device operations supported  
- ✅ Complete WebSocket event system
- ✅ Full multiroom capabilities
- ✅ Complete advanced audio controls (where supported by device)
- 🔍 Additional features beyond official specification
- 🔍 68 additional undocumented endpoints discovered but not yet implemented


## Error Handling

The API uses standard HTTP status codes:
- `200 OK` - Successful request
- `400 Bad Request` - Invalid request
- `404 Not Found` - Endpoint or resource not found
- `500 Internal Server Error` - Internal device error

## Example Implementation

```go
// Example for a GET request
func GetNowPlaying(deviceIP string) (*NowPlaying, error) {
    url := fmt.Sprintf("http://%s:8090/now_playing", deviceIP)
    resp, err := http.Get(url)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var nowPlaying NowPlaying
    err = xml.NewDecoder(resp.Body).Decode(&nowPlaying)
    return &nowPlaying, err
}

// Example for a POST request
func SendKey(deviceIP string, key string) error {
    url := fmt.Sprintf("http://%s:8090/key", deviceIP)
    xmlData := fmt.Sprintf(`<key state="press" sender="GoClient">%s</key>`, key)
    
    resp, err := http.Post(url, "application/xml", strings.NewReader(xmlData))
    if err != nil {
        return err
    }
    resp.Body.Close()
    return nil
}
```

## Notes

1. **XML Namespace**: Most responses use no explicit XML namespace
2. **Encoding**: UTF-8 is used for all XML documents
3. **Timeouts**: Recommended timeout for HTTP requests: 10 seconds
4. **Rate Limiting**: No explicit limits documented, but moderate usage recommended
5. **Device Discovery**: Devices can be found via UPnP on the local network

## Comprehensive Endpoint Discovery

### GET /supportedURLs ✅ **Implemented**
Retrieves all supported endpoints for the specific device with comprehensive feature mapping.

**Client Method**: `GetSupportedURLs() (*models.SupportedURLsResponse, error)`
**CLI Commands**: 
- `soundtouch-cli supported-urls [--features] [--verbose]` - Show endpoint-to-feature mapping
- `soundtouch-cli analyze` - Comprehensive device capability analysis with recommendations

**Response XML Structure:**
```xml
<supportedURLs deviceID="...">
  <URL location="/info" />
  <URL location="/capabilities" />
  <!-- ... additional endpoints ... -->
</supportedURLs>
```

**Feature Mapping System**: The implementation includes a comprehensive endpoint-to-feature mapping system that:
- Maps 103+ discovered endpoints to 15+ functional features
- Categorizes features by type (Core, Audio, Playback, Sources, Content, etc.)
- Identifies essential vs. optional features for device classification
- Provides feature completeness scoring (0-100%)
- Shows CLI command mappings for each supported feature
- Detects partial implementations and missing capabilities
- Offers personalized usage recommendations

**Complete Endpoint List** (103 endpoints discovered from real devices):

**Core Device Information:**
- `/info` ✅ - Device information
- `/capabilities` ✅ - Device capabilities 
- `/supportedURLs` ✅ - This endpoint (self-reference) - **FULLY IMPLEMENTED with Feature Mapping**
- `/networkInfo` ✅ - Network configuration
- `/name` ✅ - Device name management
- `/netStats` - Network statistics
- `/powerManagement` - Power state and battery information
- `/soundTouchConfigurationStatus` - Device configuration status

**Playback and Media Control:**
- `/nowPlaying` ✅ - Current playback status
- `/now_playing` ✅ - Alternative current playback endpoint
- `/nowSelection` - Current selection details
- `/key` ✅ - Send key commands
- `/select` ✅ - Select source/content
- `/playbackRequest` - Advanced playback requests
- `/userPlayControl` - User play control interface (PAUSE_CONTROL, PLAY_CONTROL, etc.)
- `/userTrackControl` - User track control interface
- `/userRating` - User rating interface (UP/DOWN for Pandora, etc.)

**Volume and Audio:**
- `/volume` ✅ - Volume control
- `/bass` ✅ - Bass settings
- `/bassCapabilities` ✅ - Bass capability info
- `/balance` ✅ - Stereo balance
- `/DSPMonoStereo` - DSP mono/stereo settings

**Sources and Content:**
- `/sources` ✅ - Available sources
- `/sourceDiscoveryStatus` - Source discovery status
- `/nameSource` - Name/rename sources
- `/selectLastSource` - Select last used source
- `/selectLastWiFiSource` - Select last WiFi source
- `/selectLastSoundTouchSource` - Select last SoundTouch source
- `/selectLocalSource` - Select local source

**Presets and Favorites:**
- `/presets` ✅ - Preset management
- `/storePreset` - Store new preset (max 6 presets)
- `/removePreset` - Remove existing preset  
- `/selectPreset` - Select preset by ID
- `/recents` ✅ - Recently played content
- `/bookmark` - Bookmark current content

**Music Services:**
- `/setMusicServiceAccount` - Configure music service account (Pandora, Spotify, etc.)
- `/setMusicServiceOAuthAccount` - OAuth account setup
- `/removeMusicServiceAccount` - Remove music service account
- `/serviceAvailability` ✅ **Implemented** - Check service availability
- `/introspect` ✅ **Implemented** - Get introspect data for specific sources

**Station Management (Radio/Streaming):**
- `/searchStation` - Search for stations (tested with Pandora)
- `/addStation` - Add station to favorites (tested with Pandora)
- `/removeStation` - Remove station from favorites (tested with Pandora)
- `/genreStations` - Browse stations by genre
- `/stationInfo` - Station information
- `/trackInfo` ✅ - Extended track information with semicolon-delimited details

**Zone and Multiroom:**
- `/getZone` ✅ - Get zone configuration
- `/setZone` ✅ - Set zone configuration  
- `/addZoneSlave` ✅ - Add device to zone
- `/removeZoneSlave` ✅ - Remove device from zone
- `/addGroup` - Add to speaker group
- `/removeGroup` - Remove from speaker group
- `/getGroup` - Get group configuration
- `/updateGroup` - Update group settings

**Clock and Display:**
- `/clockDisplay` ✅ - Clock display settings
- `/clockTime` ✅ - Device time management

**System and Configuration:**
- `/powerManagement` - Power management settings
- `/standby` - Standby mode control
- `/lowPowerStandby` - Low power standby mode
- `/systemtimeout` - System timeout settings
- `/powersaving` - Power saving configuration
- `/userActivity` - User activity tracking
- `/language` - Language settings
- `/speaker` - Speaker configuration

**Network and Connectivity:**
- `/performWirelessSiteSurvey` - WiFi site survey (returns detected networks with signal strength)
- `/addWirelessProfile` - Add WiFi profile (supports various security types)
- `/getActiveWirelessProfile` - Get active WiFi profile
- `/setWiFiRadio` - WiFi radio control

**Bluetooth:**
- `/bluetoothInfo` ✅ - Bluetooth information and pairing status
- `/enterBluetoothPairing` - Enter Bluetooth pairing mode (switches to BLUETOOTH source)
- `/clearBluetoothPaired` - Clear all Bluetooth pairings (emits descending tone)

**Pairing and Setup:**
- `/pairLightswitch` - Pair with lightswitch accessory
- `/cancelPairLightswitch` - Cancel lightswitch pairing
- `/clearPairedList` - Clear all pairings
- `/enterPairingMode` - Enter general pairing mode
- `/setPairedStatus` - Set pairing status
- `/setPairingStatus` - Update pairing status
- `/soundTouchConfigurationStatus` - Configuration status
- `/setup` - Device setup interface

**Software Updates:**
- `/swUpdateStart` - Start software update
- `/swUpdateAbort` - Abort software update
- `/swUpdateQuery` - Query update status
- `/swUpdateCheck` - Check for updates

**Advanced Features:**
- `/search` - Content search (music libraries with filter support)
- `/navigate` - Content navigation (traverse music library containers)
- `/listMediaServers` - List available UPnP/DLNA media servers
- `/requestToken` ✅ - Bearer token generation
- `/notification` - Notification management
- `/playNotification` - Play notification beep (ST-10 series only)
- `/speaker` - Play TTS messages or URL content (ST-10 series only)
- `/test` - System test interface

**Internal/System:**
- `/pdo` - Internal PDO operations  
- `/slaveMsg` - Slave device messaging
- `/masterMsg` - Master device messaging
- `/factoryDefault` - Factory reset
- `/criticalError` - Critical error handling
- `/netStats` - Network statistics and device interface details
- `/rebroadcastlatencymode` - Rebroadcast latency mode configuration
- `/systemtimeout` - System timeout settings
- `/powersaving` - Power saving configuration

**Product Information:**
- `/setProductSerialNumber` - Set product serial number
- `/setProductSoftwareVersion` - Set software version
- `/setComponentSoftwareVersion` - Set component versions

**Marge Integration (Bose Cloud Services):**
- `/marge` - Marge service integration (Bose cloud services, EOL May 2026)
- `/setMargeAccount` - Set Marge account (EOL May 2026)
- `/pushCustomerSupportInfoToMarge` - Push support info to cloud (EOL May 2026)

**Reset and Control:**
- `/getBCOReset` - Get BCO reset status
- `/setBCOReset` - Set BCO reset

**Notes on Endpoint Discovery:**
- Total discovered endpoints: **103**
- Both test devices (192.168.178.28 and 192.168.178.35) support identical endpoint lists
- Many endpoints are undocumented in official API v1.0 but functional on real hardware
- Some endpoints may require specific device types or firmware versions
- Endpoints marked ✅ are currently implemented in this Go library

**Implementation Priority:**
1. **High**: Core functionality endpoints already implemented
2. **Medium**: Music service integration, advanced zone management  
3. **Low**: Internal/diagnostic endpoints, factory operations

## Reference

Based on the official Bose SoundTouch Web API documentation:
https://assets.bosecreative.com/m/496577402d128874/original/SoundTouch-Web-API.pdf
