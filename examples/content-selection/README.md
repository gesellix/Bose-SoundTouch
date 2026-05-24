# Content Selection Example

This example demonstrates the advanced content selection features of the Bose SoundTouch Go client, including support for LOCAL_INTERNET_RADIO with streamUrl format, LOCAL_MUSIC, and STORED_MUSIC content.

## Features Demonstrated

### 1. LOCAL_INTERNET_RADIO with streamUrl Format
- Uses proxy server format: `http://contentapi.gmuth.de/station.php?name=StationName&streamUrl=ActualStreamURL`
- Supports complex radio station metadata
- Artwork and station information

### 2. LOCAL_INTERNET_RADIO Direct Streams
- Direct HTTP/HTTPS stream URLs
- Simple internet radio playback
- MP3 and other audio format support

### 3. LOCAL_MUSIC Content
- SoundTouch App Media Server content
- Albums, tracks, artists, playlists
- Requires local SoundTouch Media Server running

### 4. STORED_MUSIC Content
- UPnP/DLNA media server content
- NAS libraries and Windows Media Player sharing
- Network-attached storage music libraries

### 5. Generic ContentItem Selection
- Direct ContentItem object creation
- Maximum flexibility for any content type
- All SoundTouch sources supported

## Prerequisites

- SoundTouch device on your network
- Device IP address
- Go 1.21+ installed

### Optional (for specific examples):
- **LOCAL_MUSIC**: SoundTouch App Media Server running on a computer
- **STORED_MUSIC**: UPnP/DLNA media server (Windows Media Player, NAS, etc.)

## Usage

```bash
# Build and run
go run main.go <device_ip>

# Example
go run main.go 192.0.2.100
```

## Example Output

```
🎵 SoundTouch Content Selection Example
📱 Device: 192.0.2.100:8090

📻 Step 1: Demonstrating LOCAL_INTERNET_RADIO with streamUrl format...
  📡 Using streamUrl format with proxy server...
      Station: Antenne Chillout
      Proxy URL: http://contentapi.gmuth.de/station.php?name=Antenne%20Chillout&streamUrl=https://stream.antenne.de/chillout/stream/aacp
  ✅ Successfully selected internet radio with streamUrl format

  🎵 Now Playing:
      Title: Antenne Chillout
      Source: LOCAL_INTERNET_RADIO
      Status: Playing
      Location: http://contentapi.gmuth.de/station.php?name=Antenne%20Chillout&streamUrl=https://stream.antenne.de/chillout/stream/aacp

📻 Step 2: Demonstrating LOCAL_INTERNET_RADIO with direct stream...
  📡 Using direct stream URL...
      Stream: Test Audio Stream
      URL: https://freetestdata.com/wp-content/uploads/2021/09/Free_Test_Data_1MB_MP3.mp3
  ✅ Successfully selected direct internet radio stream

💿 Step 3: Demonstrating LOCAL_MUSIC selection...
  ⚠️  LOCAL_MUSIC demo failed (this requires SoundTouch App Media Server): failed to select local music: HTTP 404 Not Found

💾 Step 4: Demonstrating STORED_MUSIC selection...
  ⚠️  STORED_MUSIC demo failed (this requires UPnP/DLNA media server): failed to select stored music: HTTP 404 Not Found

🎯 Step 5: Demonstrating generic ContentItem selection...
  🎯 Using generic ContentItem selection...
      Content: K-LOVE Radio
      Source: TUNEIN
      Location: /v1/playback/station/s33828
  ✅ Successfully selected content using ContentItem

✅ Content selection demo completed!
```

## API Methods Demonstrated

### SelectLocalInternetRadio
```go
err := client.SelectLocalInternetRadio(location, sourceAccount, itemName, containerArt)
```

### SelectLocalMusic
```go
err := client.SelectLocalMusic(location, sourceAccount, itemName, containerArt)
```

### SelectStoredMusic
```go
err := client.SelectStoredMusic(location, sourceAccount, itemName, containerArt)
```

### SelectContentItem (Advanced)
```go
contentItem := &models.ContentItem{
    Source:        "LOCAL_INTERNET_RADIO",
    Type:          "stationurl",
    Location:      "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio",
    SourceAccount: "",
    IsPresetable:  true,
    ItemName:      "My Radio Station",
    ContainerArt:  "https://example.com/art.png",
}
err := client.SelectContentItem(contentItem)
```

## CLI Usage Examples

These API methods are also available via the CLI:

```bash
# Internet radio with streamUrl format
soundtouch-cli --host 192.0.2.100 source internet-radio \
  --location "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio" \
  --name "My Station" \
  --artwork "https://example.com/art.png"

# Local music content
soundtouch-cli --host 192.0.2.100 source local-music \
  --location "album:983" \
  --account "3f205110-4a57-4e91-810a-123456789012" \
  --name "Welcome to the New"

# Stored music content
soundtouch-cli --host 192.0.2.100 source stored-music \
  --location "6_a2874b5d_4f83d999" \
  --account "d09708a1-5953-44bc-a413-123456789012/0" \
  --name "Christmas Album"

# Generic content selection (advanced)
soundtouch-cli --host 192.0.2.100 source content \
  --source LOCAL_INTERNET_RADIO \
  --location "https://stream.example.com/radio" \
  --name "My Stream" \
  --type stationurl \
  --presetable
```

## Implementation Notes

### streamUrl Format
The streamUrl format uses a proxy server that accepts the actual stream URL as a parameter. This allows for:
- Complex metadata handling
- Stream URL obfuscation
- Cross-origin request handling
- Additional processing capabilities

### ContentItem Structure
All content selection methods create a `ContentItem` with appropriate defaults:
- `Type` is automatically set based on source
- `IsPresetable` defaults to true
- `ItemName` gets a sensible default if not provided

### Error Handling
The example gracefully handles missing services:
- LOCAL_MUSIC requires SoundTouch App Media Server
- STORED_MUSIC requires UPnP/DLNA media server
- Some internet streams may be geo-restricted

## Related Documentation

- [SoundTouch WebServices API Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- [CLI Reference](../../docs/content/docs/guides/CLI-REFERENCE.md)
- [Navigation Guide](../../docs/content/docs/guides/SURVIVAL-GUIDE.md)
