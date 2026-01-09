# Bose SoundTouch Web API - Endpoints Overview

This document provides a comprehensive overview of the available API endpoints of the Bose SoundTouch Web API based on the official specification.

## Implementation Status Legend
- ✅ **Implemented** - Fully implemented with tests and real device validation
- 🔄 **Planned** - Not yet implemented, planned for future development
- 📝 **Documented** - API documented but not implemented

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

**Important**: Proper key simulation requires sending both press and release states:

**Request XML (Press + Release):**
```xml
<key state="press" sender="Gabbo">KEY_NAME</key>
<key state="release" sender="Gabbo">KEY_NAME</key>
```

**Available Keys:**

**Playback Controls:**
- `PLAY` - Start playback
- `PAUSE` - Pause current playback
- `STOP` - Stop current playback
- `PREV_TRACK` - Go to previous track
- `NEXT_TRACK` - Go to next track

**Rating and Bookmark Controls:**
- `THUMBS_UP` - Rate current content positively (Pandora, etc.)
- `THUMBS_DOWN` - Rate current content negatively
- `BOOKMARK` - Bookmark current content

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

### GET /bass 🔄 **Planned**
Retrieves the current bass settings.

**Response XML:**
```xml
<bass deviceID="...">
  <targetbass>0</targetbass>
  <actualbass>0</actualbass>
</bass>
```

### POST /bass 🔄 **Planned**
Sets the bass settings (-9 to +9).

**Request XML:**
```xml
<bass>0</bass>
```

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

### POST /select 🔄 **Planned**
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

### POST /presets 🔄 **Planned**
Creates or updates a preset.

**Request XML:**
```xml
<preset id="1">
  <ContentItem source="..." sourceAccount="..." location="...">
    <itemName>Preset Name</itemName>
  </ContentItem>
</preset>
```

## Advanced Features

### GET /getZone 🔄 **Planned**
Retrieves multiroom zone information.

### POST /setZone 🔄 **Planned**
Configures multiroom zones.

### GET /balance 🔄 **Planned**
Retrieves balance settings (stereo devices).

### POST /balance 🔄 **Planned**
Sets balance settings.

### GET /clockTime 🔄 **Planned**
Retrieves the device time.

### POST /clockTime 🔄 **Planned**
Sets the device time.

### GET /clockDisplay 🔄 **Planned**
Retrieves clock display settings.

### POST /clockDisplay 🔄 **Planned**
Configures the clock display.

## WebSocket Connection

### WebSocket / 🔄 **Planned**
Establishes a persistent connection for live updates.

**Event Types:**
- `nowPlayingUpdated`
- `volumeUpdated`
- `connectionStateUpdated`
- `presetUpdated`

## Network and System

### GET /networkInfo 🔄 **Planned**
Retrieves network information.

### GET /capabilities ✅ **Implemented**
Retrieves device capabilities.

### GET /name ✅ **Implemented**
Retrieves the device name.

### POST /reboot 🔄 **Planned**
Restarts the device.

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

## Reference

Based on the official Bose SoundTouch Web API documentation:
https://assets.bosecreative.com/m/496577402d128874/original/SoundTouch-Web-API.pdf