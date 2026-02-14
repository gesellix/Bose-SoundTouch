# SoundTouch supportedURLs Endpoint Analysis

This document provides a comprehensive analysis of the `/supportedURLs` endpoint response from real Bose SoundTouch devices and compares it with our current implementation.

## Discovery Summary

**Test Devices:**
- Device 1: `192.168.178.28:8090` (deviceID: `08DF1F0BA325`)
- Device 2: `192.168.178.35:8090` (deviceID: `A81B6A536A98`)

**Key Findings:**
- Both devices return identical endpoint lists
- **103 total endpoints** discovered
- **~35 currently implemented** in this Go library (34%)
- **68 additional endpoints** available for future implementation

## Endpoint Categories

### ✅ Fully Implemented (Core Functionality)

**Device Information (5/5):**
- `/info` - Device information
- `/capabilities` - Device capabilities
- `/supportedURLs` - Supported endpoints list
- `/networkInfo` - Network configuration
- `/name` - Device name management

**Playback Control (3/3):**
- `/nowPlaying` - Current playback status
- `/now_playing` - Alternative current playback endpoint
- `/key` - Send key commands

**Volume & Audio (4/4):**
- `/volume` - Volume control
- `/bass` - Bass settings
- `/bassCapabilities` - Bass capability information
- `/balance` - Stereo balance

**Source Management (2/2):**
- `/sources` - Available sources
- `/select` - Select source/content

**Preset Management (1/2):**
- `/presets` - Get presets ✅ Complete
- `/storePreset` - Store/update presets ✅ Complete (reverse-engineered)
- `/removePreset` - Remove presets ✅ Complete (reverse-engineered)

**Zone/Multiroom (4/4):**
- `/getZone` - Get zone configuration
- `/setZone` - Set zone configuration
- `/addZoneSlave` - Add device to zone
- `/removeZoneSlave` - Remove device from zone

**Clock & Display (2/2):**
- `/clockDisplay` - Clock display settings
- `/clockTime` - Device time management

**Advanced Audio (3/3):**
- `/audiodspcontrols` - DSP settings (capability-dependent)
- `/audioproducttonecontrols` - Advanced tone controls (capability-dependent)
- `/audioproductlevelcontrols` - Speaker level controls (capability-dependent)

**System Info (3/3):**
- `/trackInfo` - Track information
- `/bluetoothInfo` - Bluetooth information
- `/recents` - Recently played content

### 🔶 Partially Implemented/Different Approach

**Zone Management:**
- `/addGroup` ⚠️ - We use `/setZone` for group management
- `/removeGroup` ⚠️ - We use `/setZone` for group management  
- `/getGroup` ⚠️ - We use `/getZone` for group information
- `/updateGroup` ⚠️ - We use `/setZone` for group updates

### ❌ Not Yet Implemented (High Priority)

**Enhanced Playback Control:**
- `/nowSelection` - Current selection details
- `/playbackRequest` - Advanced playback requests
- `/userPlayControl` - User play control interface
- `/userTrackControl` - User track control interface
- `/selectPreset` - Select preset by ID

**Source Enhancement:**
- `/sourceDiscoveryStatus` - Source discovery status
- `/nameSource` - Name/rename sources
- `/selectLastSource` - Select last used source
- `/selectLastWiFiSource` - Select last WiFi source
- `/selectLastSoundTouchSource` - Select last SoundTouch source
- `/selectLocalSource` - Select local source

**Music Services Integration:**
- `/setMusicServiceAccount` - Configure music service account
- `/setMusicServiceOAuthAccount` - OAuth account setup
- `/removeMusicServiceAccount` - Remove music service account
- `/serviceAvailability` - Check service availability

**Enhanced Presets:**
- `/storePreset` - Store new preset
- `/removePreset` - Remove existing preset
- `/bookmark` - Bookmark current content
- `/userRating` - User rating for content

**Station/Radio Management:**
- `/searchStation` - Search for stations
- `/addStation` - Add station to favorites
- `/removeStation` - Remove station from favorites
- `/genreStations` - Browse stations by genre
- `/stationInfo` - Station information

### ❌ Not Yet Implemented (Medium Priority)

**System Configuration:**
- `/powerManagement` - Power management settings
- `/standby` - Standby mode control
- `/lowPowerStandby` - Low power standby mode
- `/systemtimeout` - System timeout settings
- `/powersaving` - Power saving configuration
- `/language` - Language settings
- `/speaker` - Speaker configuration

**Network & Connectivity:**
- `/performWirelessSiteSurvey` - WiFi site survey
- `/addWirelessProfile` - Add WiFi profile
- `/getActiveWirelessProfile` - Get active WiFi profile
- `/setWiFiRadio` - WiFi radio control

**Bluetooth Enhancement:**
- `/enterBluetoothPairing` - Enter Bluetooth pairing mode
- `/clearBluetoothPaired` - Clear Bluetooth pairings

**Content Discovery:**
- `/search` - Content search
- `/navigate` - Content navigation
- `/listMediaServers` - List available media servers

### ❌ Not Yet Implemented (Low Priority)

**Pairing & Setup:**
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

**System Utilities:**
- `/userActivity` - User activity tracking
- `/requestToken` - Token management
- `/notification` - Notification management
- `/playNotification` - Play notification sound
- `/introspect` - System introspection
- `/test` - System test interface

**Internal/Advanced:**
- `/pdo` - Internal PDO operations
- `/slaveMsg` - Slave device messaging
- `/masterMsg` - Master device messaging
- `/factoryDefault` - Factory reset
- `/criticalError` - Critical error handling
- `/netStats` - Network statistics
- `/rebroadcastlatencymode` - Rebroadcast latency mode
- `/getBCOReset` - Get BCO reset status
- `/setBCOReset` - Set BCO reset

**Product Management:**
- `/setProductSerialNumber` - Set product serial number
- `/setProductSoftwareVersion` - Set software version
- `/setComponentSoftwareVersion` - Set component versions

**Cloud Integration (EOL May 2026):**
- `/marge` - Marge service integration
- `/setMargeAccount` - Set Marge account
- `/pushCustomerSupportInfoToMarge` - Push support info to cloud

**Enhanced DSP (Device Dependent):**
- `/DSPMonoStereo` - DSP mono/stereo settings

## Implementation Recommendations

### Phase 1: High-Value User Features
1. **Enhanced Source Selection** - `/selectLast*` endpoints for better UX
2. **Preset Management** - `/storePreset`, `/removePreset`, `/selectPreset`
3. **Station Management** - Radio/streaming station operations
4. **Music Service Integration** - Account management endpoints

### Phase 2: System Enhancement
1. **Power Management** - Standby and power saving controls
2. **Network Management** - WiFi profile and radio control
3. **Content Discovery** - Search and navigation capabilities
4. **Bluetooth Enhancement** - Pairing management

### Phase 3: Advanced Features
1. **System Diagnostics** - Network stats, introspection
2. **Update Management** - Software update control
3. **Notification System** - Notification management
4. **Advanced Setup** - Pairing and configuration tools

## Notes

1. **Device Consistency**: Both test devices expose identical endpoint lists, suggesting consistent firmware behavior across SoundTouch models.

2. **Official vs. Real**: The device exposes **84 additional endpoints** beyond the 19 documented in the official API v1.0, indicating significant undocumented functionality.

3. **Cloud Dependency**: Some endpoints (especially `/marge*`) may become non-functional after the May 2026 SoundTouch cloud EOL.

4. **Implementation Strategy**: Focus on user-facing functionality first, then system management, finally internal/diagnostic features.

5. **Testing Required**: Each new endpoint implementation should be tested against real hardware to verify functionality and response formats.

6. **Documentation Gap**: Many endpoints lack official documentation, requiring reverse engineering through testing.

## Raw Device Response

**Device Count:** 103 unique endpoints
**Response Format:** XML with URL location attributes
**Common Pattern:** Most endpoints support both GET (query) and POST (modify) operations

**Example Response Structure:**
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<supportedURLs deviceID="08DF1F0BA325">
    <URL location="/info" />
    <URL location="/capabilities" />
    <!-- ... 101 additional endpoints ... -->
</supportedURLs>
```

This analysis provides a roadmap for expanding the Go library's API coverage from 34% to potentially 100% of available device functionality.