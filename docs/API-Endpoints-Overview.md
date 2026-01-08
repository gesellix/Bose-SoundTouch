# Bose SoundTouch Web API - Endpoints Overview

This document provides a comprehensive overview of the available API endpoints of the Bose SoundTouch Web API based on the official specification.

## API Basics

- **Protocol**: HTTP REST-like
- **Data Format**: XML Request/Response
- **Standard Port**: 8090
- **Base URL**: `http://<device-ip>:8090/`
- **Authentication**: No complex authentication required
- **Real-time Updates**: WebSocket connection available

## Device Information

### GET /info
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

### GET /now_playing
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

### POST /key
Sends key commands to the device.

**Request XML:**
```xml
<key state="press" sender="Sender">KEY_NAME</key>
```

**Available Keys:**
- `PLAY`
- `PAUSE` 
- `STOP`
- `PREV_TRACK`
- `NEXT_TRACK`
- `THUMBS_UP`
- `THUMBS_DOWN`
- `BOOKMARK`
- `POWER`
- `MUTE`
- `VOLUME_UP`
- `VOLUME_DOWN`
- `PRESET_1` to `PRESET_6`
- `AUX_INPUT`
- `SHUFFLE_OFF`
- `SHUFFLE_ON`
- `REPEAT_OFF`
- `REPEAT_ONE`
- `REPEAT_ALL`

## Volume Control

### GET /volume
Retrieves the current volume.

**Response XML:**
```xml
<volume deviceID="...">
  <targetvolume>50</targetvolume>
  <actualvolume>50</actualvolume>
  <muteenabled>false</muteenabled>
</volume>
```

### POST /volume
Sets the volume.

**Request XML:**
```xml
<volume>50</volume>
```

## Bass Settings

### GET /bass
Retrieves the current bass settings.

**Response XML:**
```xml
<bass deviceID="...">
  <targetbass>0</targetbass>
  <actualbass>0</actualbass>
</bass>
```

### POST /bass
Sets the bass settings (-9 to +9).

**Request XML:**
```xml
<bass>0</bass>
```

## Source Management

### GET /sources
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

### POST /select
Selects an audio source.

**Request XML:**
```xml
<ContentItem source="SPOTIFY" sourceAccount="...">
  <itemName>Spotify</itemName>
</ContentItem>
```

## Preset Management

### GET /presets
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

### POST /presets
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

### GET /getZone
Retrieves multiroom zone information.

### POST /setZone
Configures multiroom zones.

### GET /balance
Retrieves balance settings (stereo devices).

### POST /balance
Sets balance settings.

### GET /clockTime
Retrieves the device time.

### POST /clockTime
Sets the device time.

### GET /clockDisplay
Retrieves clock display settings.

### POST /clockDisplay
Configures the clock display.

## WebSocket Connection

### WebSocket /
Establishes a persistent connection for live updates.

**Event Types:**
- `nowPlayingUpdated`
- `volumeUpdated`
- `connectionStateUpdated`
- `presetUpdated`

## Network and System

### GET /networkInfo
Retrieves network information.

### GET /capabilities
Retrieves device capabilities.

### POST /reboot
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