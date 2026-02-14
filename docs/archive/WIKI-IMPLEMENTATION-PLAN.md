# SoundTouch API Wiki Implementation Plan

**Date:** January 2026  
**Source:** [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)  
**Target:** Complete implementation of 64 additional endpoints from wiki documentation

## Project Overview

### Scope
Implement 64 additional API endpoints documented in the SoundTouch Plus Wiki to achieve comprehensive SoundTouch ecosystem coverage.

### Current Status
- ✅ **Implemented**: 23 endpoints (core functionality)
- 🎯 **Target**: 87 endpoints (comprehensive functionality)
- 📈 **Expansion**: 3.8x increase in API coverage

---

## Implementation Phases

## Phase 1: Essential User Features (4 weeks)
**Priority:** CRITICAL  
**Endpoints:** 20  
**User Impact:** HIGH

### 1.1 Preset Management (Week 1)
Essential for user experience - save and manage favorite stations/playlists.

#### Endpoints to Implement:
```go
// pkg/api/presets.go (new file)
func (c *Client) StorePreset(id int, content ContentItem) error
func (c *Client) RemovePreset(id int) error  
func (c *Client) SelectPreset(id int) error
```

#### XML Structures:
```xml
<!-- Store Preset Request -->
<preset id="3" createdOn="1701220500" updatedOn="1701220500">
  <ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s309605" sourceAccount="" isPresetable="true">
    <itemName>K-LOVE 90s</itemName>
    <containerArt>http://cdn-profiles.tunein.com/s309605/images/logog.png</containerArt>
  </ContentItem>
</preset>

<!-- Remove Preset Request -->
<preset id="4"></preset>
```

#### WebSocket Events:
- `presetsUpdated` - Triggered on store/remove operations

### 1.2 Music Service Management (Week 1-2)
Critical for streaming service integration - Spotify, Pandora, NAS libraries.

#### Endpoints to Implement:
```go
// pkg/api/music_services.go (new file)
func (c *Client) SetMusicServiceAccount(source, user, password, displayName string) error
func (c *Client) RemoveMusicServiceAccount(source, user string) error
func (c *Client) ListMediaServers() (*MediaServerList, error)
func (c *Client) GetServiceAvailability() (*ServiceAvailability, error)
```

#### Service Types:
```go
type MusicService string

const (
    ServicePandora     MusicService = "PANDORA"
    ServiceSpotify     MusicService = "SPOTIFY" 
    ServiceStoredMusic MusicService = "STORED_MUSIC"
    ServiceLocalMusic  MusicService = "LOCAL_MUSIC"
)

type MediaServer struct {
    ID           string `xml:"id,attr"`
    MAC          string `xml:"mac,attr"`
    IP           string `xml:"ip,attr"`
    Manufacturer string `xml:"manufacturer,attr"`
    ModelName    string `xml:"model_name,attr"`
    FriendlyName string `xml:"friendly_name,attr"`
    Location     string `xml:"location,attr"`
}
```

#### XML Examples:
```xml
<!-- Pandora Account Setup -->
<credentials source="PANDORA" displayName="Pandora Music Service">
  <user>YourPandoraUserId</user>
  <pass>YourPandoraPassword$1pd</pass>
</credentials>

<!-- NAS Library Setup -->
<credentials source="STORED_MUSIC" displayName="My NAS Media Library:">
  <user>d09708a1-5953-44bc-a413-123456789012/0</user>
  <pass />
</credentials>
```

### 1.3 Content Discovery (Week 2-3)
Essential for browsing music libraries and searching content.

#### Endpoints to Implement:
```go
// pkg/api/content.go (new file)
func (c *Client) Navigate(source, sourceAccount string, options NavigateOptions) (*NavigateResponse, error)
func (c *Client) Search(source, sourceAccount, searchTerm string, options SearchOptions) (*SearchResponse, error)
func (c *Client) GetRecents() (*RecentsResponse, error) // ✅ IMPLEMENTED
func (c *Client) Introspect(source, sourceAccount string) (*IntrospectResponse, error) // ✅ IMPLEMENTED
```

#### Data Structures:
```go
type NavigateOptions struct {
    StartItem int            `xml:"startItem"`
    NumItems  int            `xml:"numItems"`
    Item      *ContentItem   `xml:"item,omitempty"`
    Sort      string         `xml:"sort,attr,omitempty"`
    Menu      string         `xml:"menu,attr,omitempty"`
}

type NavigateResponse struct {
    Source       string        `xml:"source,attr"`
    SourceAccount string       `xml:"sourceAccount,attr"`
    TotalItems   int           `xml:"totalItems"`
    Items        []ContentItem `xml:"items>item"`
}

type SearchOptions struct {
    StartItem int    `xml:"startItem"`
    NumItems  int    `xml:"numItems"`
    Filter    string `xml:"searchTerm,attr,omitempty"`  // "track", "artist", "album"
}
```

### 1.4 Station Management (Week 3)
Pandora and other music service station management.

#### Endpoints to Implement:
```go
// pkg/api/stations.go (new file)
func (c *Client) SearchStations(source, sourceAccount, searchTerm string) (*StationSearchResponse, error)
func (c *Client) AddStation(source, sourceAccount, token, name string) error
func (c *Client) RemoveStation(content ContentItem) error
```

### 1.5 Enhanced Playback Control (Week 4)
Advanced playback and rating controls.

#### Endpoints to Implement:
```go
// pkg/api/playback.go (extend existing)
func (c *Client) SendPlayControl(action PlayControlAction) error
func (c *Client) RateCurrentTrack(rating RatingValue) error
```

#### Enums:
```go
type PlayControlAction string
const (
    PlayControlPause     PlayControlAction = "PAUSE_CONTROL"
    PlayControlPlay      PlayControlAction = "PLAY_CONTROL" 
    PlayControlPlayPause PlayControlAction = "PLAY_PAUSE_CONTROL"
    PlayControlStop      PlayControlAction = "STOP_CONTROL"
)

type RatingValue string
const (
    RatingUp   RatingValue = "UP"
    RatingDown RatingValue = "DOWN"
)
```

### 1.6 Power Management (Week 4)
Essential for smart home integration.

#### Endpoints to Implement:
```go
// pkg/api/power.go (new file)
func (c *Client) Standby() error
func (c *Client) GetPowerState() (*PowerState, error)
func (c *Client) SetLowPowerStandby() error
```

---

## Phase 2: Smart Home Integration (3 weeks)
**Priority:** HIGH  
**Endpoints:** 15  
**User Impact:** MEDIUM-HIGH

### 2.1 Notification System (Week 1)
Text-to-speech and URL playback for smart home notifications.

#### Endpoints to Implement:
```go
// pkg/api/notifications.go (new file)
func (c *Client) PlayTTSMessage(message string, options TTSOptions) error
func (c *Client) PlayURL(url string, options PlayOptions) error
func (c *Client) PlayNotificationBeep() error
```

#### Data Structures:
```go
type TTSOptions struct {
    VolumeLevel int    `xml:"volume,omitempty"`
    Language    string `xml:"tl,omitempty"`     // "EN", "DE", etc.
    AppKey      string `xml:"app_key"`
    Service     string `xml:"service"`
    Message     string `xml:"message"`
    Reason      string `xml:"reason"`
}

type PlayOptions struct {
    VolumeLevel int    `xml:"volume,omitempty"`
    AppKey      string `xml:"app_key"`
    Service     string `xml:"service"`
    Message     string `xml:"message"`
    Reason      string `xml:"reason"`
}
```

#### XML Examples:
```xml
<!-- TTS Message -->
<play_info>
  <url>http://translate.google.com/translate_tts?ie=UTF-8&amp;tl=EN&amp;client=tw-ob&amp;q=There%20is%20activity%20at%20the%20front%20door.</url>
  <app_key>YourAppKey</app_key>
  <service>TTS Notification</service>
  <message>Google TTS</message>
  <reason>There is activity at the front door.</reason>
  <volume>70</volume>
</play_info>
```

### 2.2 Network Management (Week 2)
WiFi configuration and network information.

#### Endpoints to Implement:
```go
// pkg/api/network.go (extend existing)
func (c *Client) PerformWiFiSurvey() (*WiFiSurveyResponse, error)
func (c *Client) AddWiFiProfile(ssid, password string, securityType SecurityType) error
func (c *Client) GetActiveWiFiProfile() (*WiFiProfile, error)
func (c *Client) GetNetworkStats() (*NetworkStats, error)
```

#### Security Types:
```go
type SecurityType string
const (
    SecurityNone       SecurityType = "none"
    SecurityWEP        SecurityType = "wep"
    SecurityWPATKIP    SecurityType = "wpatkip"
    SecurityWPAAES     SecurityType = "wpaaes"
    SecurityWPA2TKIP   SecurityType = "wpa2tkip" 
    SecurityWPA2AES    SecurityType = "wpa2aes"
    SecurityWPAOrWPA2  SecurityType = "wpa_or_wpa2"  // Recommended
)
```

### 2.3 Bluetooth Management (Week 2)
Bluetooth pairing and connection management.

#### Endpoints to Implement:
```go
// pkg/api/bluetooth.go (new file)
func (c *Client) EnterBluetoothPairing() error
func (c *Client) ClearBluetoothPairings() error
func (c *Client) GetBluetoothInfo() (*BluetoothInfo, error)
```

### 2.4 Language and System Configuration (Week 3)
Device language and system settings.

#### Endpoints to Implement:
```go
// pkg/api/system.go (new file)
func (c *Client) GetLanguage() (LanguageCode, error)
func (c *Client) SetLanguage(lang LanguageCode) error
func (c *Client) GetConfigurationStatus() (*ConfigurationStatus, error)
func (c *Client) GetSystemTimeout() (*SystemTimeout, error)
```

#### Language Codes:
```go
type LanguageCode int
const (
    LangDanish              LanguageCode = 1
    LangGerman              LanguageCode = 2
    LangEnglish             LanguageCode = 3
    LangSpanish             LanguageCode = 4
    LangFrench              LanguageCode = 5
    LangItalian             LanguageCode = 6
    LangDutch               LanguageCode = 7
    LangSwedish             LanguageCode = 8
    LangJapanese            LanguageCode = 9
    LangSimplifiedChinese   LanguageCode = 10
    LangTraditionalChinese  LanguageCode = 11
    LangKorean              LanguageCode = 12
)
```

---

## Phase 3: Advanced Features (4 weeks)
**Priority:** MEDIUM  
**Endpoints:** 19  
**User Impact:** MEDIUM

### 3.1 Stereo Pair Management (Week 1)
ST-10 specific left/right speaker pairing.

#### Endpoints to Implement:
```go
// pkg/api/groups.go (new file)
func (c *Client) GetStereoPairStatus() (*StereoPair, error)
func (c *Client) CreateStereoPair(leftDeviceID, rightDeviceID string, name string) (*StereoPair, error)
func (c *Client) RemoveStereoPair() error
func (c *Client) UpdateStereoPairName(groupID, newName string) (*StereoPair, error)
```

#### Data Structures:
```go
type StereoPair struct {
    ID              string      `xml:"id,attr"`
    Name            string      `xml:"name"`
    MasterDeviceID  string      `xml:"masterDeviceId"`
    Roles           []GroupRole `xml:"roles>groupRole"`
    SenderIPAddress string      `xml:"senderIPAddress"`
    Status          string      `xml:"status"`
}

type GroupRole struct {
    DeviceID  string `xml:"deviceId"`
    Role      string `xml:"role"`      // "LEFT", "RIGHT"
    IPAddress string `xml:"ipAddress"`
}
```

### 3.2 Software Update Management (Week 2)
Firmware update checking and management.

#### Endpoints to Implement:
```go
// pkg/api/updates.go (new file)
func (c *Client) CheckSoftwareUpdate() (*UpdateInfo, error)
func (c *Client) GetUpdateStatus() (*UpdateStatus, error)
func (c *Client) StartSoftwareUpdate() error
func (c *Client) AbortSoftwareUpdate() error
```

### 3.3 Advanced Audio Features (Week 3)
Advanced DSP and speaker configuration.

#### Endpoints to Implement:
```go
// pkg/api/audio_advanced.go (new file)
func (c *Client) GetDSPMonoStereo() (*DSPMonoStereoConfig, error)
func (c *Client) SetDSPMonoStereo(enabled bool) error
func (c *Client) GetAudioSpeakerAttributes() (*SpeakerAttributes, error)
func (c *Client) GetRebroadcastLatencyMode() (*LatencyMode, error)
```

### 3.4 Source Selection Shortcuts (Week 4)
Quick source switching utilities.

#### Endpoints to Implement:
```go
// pkg/api/sources.go (extend existing)
func (c *Client) SelectLastSource() error
func (c *Client) SelectLastSoundTouchSource() error
func (c *Client) SelectLastWiFiSource() error
func (c *Client) SelectLocalSource() error
```

---

## Phase 4: Professional Features (2 weeks)
**Priority:** LOW  
**Endpoints:** 10  
**User Impact:** LOW

### 4.1 HDMI and Product Controls
ST-300 specific HDMI and product controls.

#### Endpoints to Implement:
```go
// pkg/api/product.go (new file)
func (c *Client) GetProductCECHDMIControl() (*CECHDMIControl, error)
func (c *Client) SetProductCECHDMIControl(config CECHDMIControl) error
func (c *Client) GetProductHDMIAssignmentControls() (*HDMIAssignmentControls, error)
func (c *Client) SetProductHDMIAssignmentControls(config HDMIAssignmentControls) error
```

### 4.2 System Administration
Advanced system configuration and diagnostics.

#### Endpoints to Implement:
```go
// pkg/api/admin.go (new file)
func (c *Client) GetCriticalErrors() (*CriticalErrors, error)
func (c *Client) PerformFactoryDefault() error
func (c *Client) GetBCOReset() (*BCOResetStatus, error)
func (c *Client) SetBCOReset(enabled bool) error
```

---

## Implementation Guidelines

### File Structure
```
pkg/
├── api/
│   ├── presets.go          (Phase 1.1)
│   ├── music_services.go   (Phase 1.2)
│   ├── content.go          (Phase 1.3)
│   ├── stations.go         (Phase 1.4)
│   ├── playback.go         (Phase 1.5 - extend existing)
│   ├── power.go            (Phase 1.6)
│   ├── notifications.go    (Phase 2.1)
│   ├── network.go          (Phase 2.2 - extend existing)
│   ├── bluetooth.go        (Phase 2.3)
│   ├── system.go           (Phase 2.4)
│   ├── groups.go           (Phase 3.1)
│   ├── updates.go          (Phase 3.2)
│   ├── audio_advanced.go   (Phase 3.3)
│   ├── sources.go          (Phase 3.4 - extend existing)
│   ├── product.go          (Phase 4.1)
│   └── admin.go            (Phase 4.2)
├── types/
│   ├── presets.go
│   ├── music_services.go
│   ├── content.go
│   ├── notifications.go
│   ├── network.go
│   ├── bluetooth.go
│   ├── system.go
│   ├── groups.go
│   ├── updates.go
│   └── product.go
└── websocket/
    └── events.go           (extend with new event types)
```

### Error Handling Strategy

#### Device Capability Checking
```go
// Always check capabilities before calling advanced features
func (c *Client) callAdvancedEndpoint() error {
    capabilities, err := c.GetCapabilities()
    if err != nil {
        return fmt.Errorf("failed to get capabilities: %w", err)
    }
    
    if !capabilities.SupportsFeature("targetFeature") {
        return ErrFeatureNotSupported
    }
    
    // Proceed with endpoint call
}
```

#### Timeout Handling
```go
// Some endpoints timeout on unsupported devices
func (c *Client) callWithTimeout(endpoint string) error {
    ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
    defer cancel()
    
    // Make request with context
    if err := c.makeRequest(ctx, endpoint); err != nil {
        if errors.Is(err, context.DeadlineExceeded) {
            return ErrEndpointNotSupported
        }
        return err
    }
    return nil
}
```

### Testing Strategy

#### Unit Tests
- XML marshaling/unmarshaling for all new types
- Error handling scenarios
- Input validation

#### Integration Tests  
- Real device testing for each endpoint
- Device compatibility matrix validation
- WebSocket event verification

#### Device Matrix Testing
```go
var deviceTests = []struct {
    model     string
    endpoints []string
    supported bool
}{
    {"ST-10", []string{"/playNotification", "/getGroup"}, true},
    {"ST-300", []string{"/audiodspcontrols", "/productcechdmicontrol"}, true},
    {"ST-10", []string{"/audiodspcontrols"}, false},
}
```

### WebSocket Event Integration

#### Extend Existing Event System
```go
// pkg/websocket/events.go (extend existing)
type WebSocketEvent struct {
    // Existing events...
    VolumeUpdated      *VolumeUpdate      `xml:"volumeUpdated"`
    NowPlayingUpdated  *NowPlayingUpdate  `xml:"nowPlayingUpdated"`
    
    // New events from wiki
    PresetsUpdated     *PresetsUpdate     `xml:"presetsUpdated"`
    GroupUpdated       *GroupUpdate       `xml:"groupUpdated"`
    AudioDSPUpdated    *AudioDSPUpdate    `xml:"audiodspcontrols"`
    ToneControlsUpdated *ToneUpdate       `xml:"audioproducttonecontrols"`
    LevelControlsUpdated *LevelUpdate     `xml:"audioproductlevelcontrols"`
}
```

### Documentation Integration

#### Wiki Examples in Go Docs
```go
// StorePreset saves a preset to the device (maximum 6 presets).
//
// Example from SoundTouch Plus Wiki:
//   preset := PresetData{
//       ID: 3,
//       ContentItem: ContentItem{
//           Source:      "TUNEIN",
//           Type:        "stationurl", 
//           Location:    "/v1/playback/station/s309605",
//           IsPresetable: true,
//           ItemName:    "K-LOVE 90s",
//           ContainerArt: "http://cdn-profiles.tunein.com/s309605/images/logog.png",
//       },
//   }
//   err := client.StorePreset(preset.ID, preset.ContentItem)
//
// This generates a presetsUpdated WebSocket event.
func (c *Client) StorePreset(id int, content ContentItem) error
```

---

## Success Metrics

### Phase 1 Completion Criteria
- [ ] All 20 endpoints implemented with full XML support
- [ ] Comprehensive unit test coverage (>90%)
- [ ] Real device testing on ST-10 and ST-300
- [ ] Documentation with wiki examples
- [ ] WebSocket event integration

### Phase 2 Completion Criteria  
- [ ] Smart home integration examples
- [ ] Network management automation
- [ ] Notification system with TTS
- [ ] Bluetooth management
- [ ] Language configuration

### Phase 3 Completion Criteria
- [ ] Stereo pair management
- [ ] Software update automation
- [ ] Advanced audio features
- [ ] Source switching utilities

### Phase 4 Completion Criteria
- [ ] Professional HDMI controls
- [ ] System administration features
- [ ] Complete device capability matrix
- [ ] Production deployment guide

### Overall Success Metrics
- ✅ 87+ total endpoints implemented
- ✅ Complete SoundTouch ecosystem coverage
- ✅ Production-ready error handling
- ✅ Comprehensive documentation
- ✅ Real-world testing validation
- ✅ Community collaboration with SoundTouch Plus project

---

## Risk Mitigation

### Technical Risks
1. **Device Compatibility**: Test each endpoint on multiple device models
2. **Timeout Issues**: Implement capability checking before endpoint calls
3. **XML Complexity**: Thorough marshaling/unmarshaling tests
4. **WebSocket Events**: Validate event generation for all POST operations

### Schedule Risks
1. **Resource Availability**: Prioritize high-impact endpoints first
2. **Device Access**: Arrange access to multiple SoundTouch models
3. **Complexity Underestimation**: Buffer time in each phase
4. **Integration Issues**: Continuous integration testing

### Quality Risks
1. **Incomplete Testing**: Mandate real device validation
2. **Poor Documentation**: Use wiki examples in all documentation
3. **Breaking Changes**: Maintain backward compatibility
4. **Performance**: Benchmark all new endpoints

---

## Conclusion

This implementation plan leverages the comprehensive SoundTouch Plus Wiki to transform our library from basic device control to complete ecosystem management. The phased approach prioritizes user-facing features while ensuring quality and maintainability.

**Key Benefits:**
- 🎯 **3.8x API Coverage Expansion**: From 23 to 87+ endpoints
- 🏠 **Complete Smart Home Integration**: Power, notifications, network management
- 🎵 **Full Music Service Support**: Spotify, Pandora, NAS libraries
- ✅ **Production-Ready Implementation**: Real-world tested examples
- 📚 **Comprehensive Documentation**: Wiki integration and examples

**Timeline:** 13 weeks total for complete implementation
**Resources:** 1-2 developers with access to multiple SoundTouch devices
**Outcome:** Industry-leading SoundTouch API library with complete ecosystem support

*This plan transforms our library into the definitive Go implementation for SoundTouch integration, suitable for everything from basic home automation to professional audio installations.*