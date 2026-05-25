---
title: "SoundTouch `/storePreset` Implementation Guide"
sidebar:
  exclude: true
---
## Overview

This document analyzes the feasibility and implementation approach for adding `/storePreset` functionality to the Bose SoundTouch API client, based on [GitHub Issue #14](https://github.com/gesellix/Bose-SoundTouch/issues/14) and endpoints discovered through the comprehensive [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API).

## Current Implementation Status

### ✅ Already Implemented
- `GetPresets()` - Read presets from device
- `SelectPreset()` - Select preset by number (1-6)  
- `GetNextAvailablePresetSlot()` - Find next available preset slot
- `IsCurrentContentPresetable()` - Check if current content can be saved as preset
- Complete data models (`models.Preset`, `models.ContentItem`)
- WebSocket events for preset updates

### ❌ Missing Functionality
- `StorePreset()` - Save content as preset
- `RemovePreset()` - Delete existing preset

## API Capabilities

According to the comprehensive [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API#preset-store), `/storePreset` supports:

1. **Radio Stations** (TUNEIN, LOCAL_INTERNET_RADIO)
2. **Spotify Content** (Playlists, Albums, Artists, Tracks)
3. **Local Music** (STORED_MUSIC, LOCAL_MUSIC)  
4. **Maximum 6 Presets** per device
5. **Automatic Timestamps** (createdOn, updatedOn)
6. **WebSocket Events** (`presetsUpdated`)

## Implementation Examples

### Core Client Methods

```go
// StorePreset saves content as a preset on the SoundTouch device
func (c *Client) StorePreset(id int, contentItem *models.ContentItem) error {
    now := time.Now().Unix()
    preset := &models.Preset{
        ID:          id,
        CreatedOn:   &now,
        UpdatedOn:   &now,
        ContentItem: contentItem,
    }
    
    var response models.Presets
    return c.post("/storePreset", preset, &response)
}

// RemovePreset deletes a preset from the SoundTouch device  
func (c *Client) RemovePreset(id int) error {
    preset := &models.Preset{ID: id}
    var response models.Presets
    return c.post("/removePreset", preset, &response)
}

// StoreCurrentAsPreset saves currently playing content as preset
func (c *Client) StoreCurrentAsPreset(id int) error {
    nowPlaying, err := c.GetNowPlaying()
    if err != nil {
        return fmt.Errorf("failed to get current content: %w", err)
    }
    
    if !nowPlaying.ContentItem.IsPresetable {
        return fmt.Errorf("current content is not presetable")
    }
    
    return c.StorePreset(id, nowPlaying.ContentItem)
}
```

### CLI Commands

```bash
# Store currently playing content as preset
soundtouch-cli --host 192.0.2.100 preset store-current --slot 3

# Store specific content as preset
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 1 \
  --source SPOTIFY \
  --location "spotify:playlist:37i9dQZF1DX0XUsuxWHRQd" \
  --source-account "yourusername" \
  --name "My Worship Mix"

# Store radio station as preset
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 2 \
  --source TUNEIN \
  --location "/v1/playback/station/s33828" \
  --name "K-LOVE Radio"

# Store radio station using TuneIn URL (Name and Artwork are automatically fetched)
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 6 \
  --location "https://tunein.com/radio/WDR-2-Rheinland-1004-s213886/"

# Store Spotify album using URL (Name and Artwork are automatically fetched)
soundtouch-cli --host 192.0.2.100 preset store \
  --slot 1 \
  --location "https://open.spotify.com/album/6rT8yer84xoh0t17poLsmn?si=XqxdZazpTLC1ceoC8EeCuA" \
  --source-account "yourusername"

# Remove preset
soundtouch-cli --host 192.0.2.100 preset remove --slot 3

# Show current content details (including location URI for all sources)
soundtouch-cli --host 192.0.2.100 play now

# Show detailed content information 
soundtouch-cli --host 192.0.2.100 play now --verbose
```

## Spotify Integration Examples

### 1. Spotify Playlist
```go
contentItem := &models.ContentItem{
    Source:        "SPOTIFY",
    Type:          "uri",
    Location:      "spotify:playlist:37i9dQZF1DX0XUsuxWHRQd",
    SourceAccount: "yourspotifyusername",
    IsPresetable:  true,
    ItemName:      "My Worship Mix",
    ContainerArt:  "https://i.scdn.co/image/ab67706c0000da84820d2514932c9e2ea40f6473",
}
```

### 2. Spotify Album
```go
contentItem := &models.ContentItem{
    Source:        "SPOTIFY",
    Type:          "uri", 
    Location:      "spotify:album:6vc9OTcyd3hyzabCmsdnwE",
    SourceAccount: "yourspotifyusername",
    IsPresetable:  true,
    ItemName:      "Welcome to the New",
    ContainerArt:  "https://i.scdn.co/image/ab67616d0000b27316c019c87a927829804caf0b",
}
```

### 3. Spotify Artist
```go
contentItem := &models.ContentItem{
    Source:        "SPOTIFY",
    Type:          "uri",
    Location:      "spotify:artist:6APm8EjxOHSYM5B4i3vT3q",
    SourceAccount: "yourspotifyusername", 
    IsPresetable:  true,
    ItemName:      "MercyMe",
    ContainerArt:  "https://i.scdn.co/image/ab6761610000e5eb16c019c87a927829804caf0b",
}
```

## Getting Spotify URIs (Location Values)

### Method 1: From Spotify App
1. Right-click on playlist/album/song in Spotify app
2. "Share" → "Copy link to playlist"
3. Convert URL to URI:
   - URL: `https://open.spotify.com/playlist/37i9dQZF1DX0XUsuxWHRQd`
   - URI: `spotify:playlist:37i9dQZF1DX0XUsuxWHRQd`

### Method 2: From Currently Playing Content (All Sources)
```go
func getCurrentContentLocation(client *soundtouch.Client) (string, string, error) {
    nowPlaying, err := client.GetNowPlaying()
    if err != nil {
        return "", "", err
    }
    
    if nowPlaying.ContentItem == nil || nowPlaying.ContentItem.Location == "" {
        return "", "", fmt.Errorf("no content location available")
    }
    
    return nowPlaying.ContentItem.Location, nowPlaying.ContentItem.Source, nil
}
```

### Method 3: URL to URI Converter
```go
func SpotifyURLToURI(url string) (string, error) {
    re := regexp.MustCompile(`https://open\.spotify\.com/(playlist|album|artist|track|episode|show)/([a-zA-Z0-9]+)`)
    matches := re.FindStringSubmatch(url)
    
    if len(matches) != 3 {
        return "", fmt.Errorf("invalid Spotify URL format")
    }
    
    contentType := matches[1]
    contentID := matches[2]
    
    return fmt.Sprintf("spotify:%s:%s", contentType, contentID), nil
}
```

## XML Request Format

The actual XML request sent to the SoundTouch API:

```xml
<preset id="3" createdOn="1701220500" updatedOn="1701220500">
  <ContentItem source="SPOTIFY" type="uri" location="spotify:playlist:37i9dQZF1DX0XUsuxWHRQd" sourceAccount="yourusername" isPresetable="true">
    <itemName>My Worship Mix</itemName>
    <containerArt>https://i.scdn.co/image/ab67706c0000da84820d2514932c9e2ea40f6473</containerArt>
  </ContentItem>
</preset>
```

## Radio Station Examples

### TUNEIN Radio
```go
contentItem := &models.ContentItem{
    Source:       "TUNEIN",
    Type:         "stationurl",
    Location:     "/v1/playback/station/s33828",
    SourceAccount: "",
    IsPresetable: true,
    ItemName:     "K-LOVE Radio",
    ContainerArt: "http://cdn-profiles.tunein.com/s33828/images/logog.png",
}
```

### Local Internet Radio
```go
contentItem := &models.ContentItem{
    Source:       "LOCAL_INTERNET_RADIO",
    Type:         "stationurl", 
    Location:     "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJ...",
    SourceAccount: "",
    IsPresetable: true,
    ItemName:     "Custom Radio Station",
    ContainerArt: "",
}
```

## Implementation Roadmap

### Phase 1: Core Functionality
1. Add `StorePreset()` method to client
2. Add `RemovePreset()` method to client
3. Add basic CLI commands
4. Add unit tests

### Phase 2: Enhanced CLI
1. Add `store-current` command
2. Add Spotify URL-to-URI conversion
3. Add content validation
4. Add batch import functionality

### Phase 3: Advanced Features
1. Add preset management utilities
2. Add content discovery helpers
3. Add preset backup/restore
4. Integration with Spotify Web API for search

## Technical Requirements

### Prerequisites
- Existing HTTP client infrastructure ✅
- XML marshaling/unmarshaling ✅
- WebSocket event system ✅
- CLI framework ✅
- Data models ✅

### Implementation Effort
- **Client methods**: ~50-100 lines of code
- **CLI commands**: ~100-150 lines of code
- **Tests**: ~200-300 lines of code
- **Documentation**: This document + API docs

## WebSocket Events

When presets are stored or removed, the device generates `presetsUpdated` events:

```xml
<updates deviceID="1004567890AA">
  <presetsUpdated>
    <presets>
      <preset id="1" createdOn="1700536011" updatedOn="1700536011">
        <ContentItem source="SPOTIFY" type="uri" location="spotify:playlist:37i9dQZF1DX0XUsuxWHRQd" sourceAccount="username" isPresetable="true">
          <itemName>My Worship Mix</itemName>
          <containerArt>https://i.scdn.co/image/ab67706c0000da84820d2514932c9e2ea40f6473</containerArt>
        </ContentItem>
      </preset>
    </presets>
  </presetsUpdated>
</updates>
```

## CLI Command Updates

The CLI now automatically shows location details for **all sources** when using `play now`:

### Automatic Location Display
```bash
# Location automatically shown for any source with location data
go run ./cmd/soundtouch-cli --host 192.0.2.100 play now
```

**Example outputs:**

**TUNEIN Radio:**
```
Now Playing:
  Source: TUNEIN
  Track: K-LOVE Radio
  
Content Details:
  Location: /v1/playback/station/s33828
```

**LOCAL_INTERNET_RADIO:**
```
Now Playing:
  Source: LOCAL_INTERNET_RADIO  
  Track: Custom Radio Station
  
Content Details:
  Location: https://stream.example.com/radio
```

**STORED_MUSIC (NAS):**
```
Now Playing:
  Source: STORED_MUSIC
  Track: Welcome Home
  Artist: MercyMe
  
Content Details:
  Location: 6_a2874b5d_4f83d999
```

### Verbose Mode for Complete Details
```bash
go run ./cmd/soundtouch-cli --host 192.0.2.100 play now --verbose
```

Shows additional information:
```
Content Details:
  Location: /v1/playback/station/s33828
  Content Type: stationurl
  Item Name: K-LOVE Radio
  Presetable: true
```

## Use Cases

1. **Quick Access to Favorite Playlists**: Store frequently used Spotify playlists as presets 1-6
2. **Radio Station Shortcuts**: Save favorite TUNEIN and internet radio stations for instant access
3. **NAS Music Collections**: Store favorite albums from your network storage as presets
4. **Pandora Stations**: Save your custom Pandora radio stations for quick access
5. **Mood-based Presets**: Organize content by activity (workout, relaxation, work)
6. **Family-friendly Setup**: Each family member gets their own preset slots
7. **Smart Home Integration**: Trigger specific music for different scenarios

## Spotify URI Reference

## Location Reference for All Sources

| Source | Location Format | Example |
|--------|-----------------|---------|
| **Spotify Playlist** | `spotify:playlist:ID` | `spotify:playlist:37i9dQZF1DX0XUsuxWHRQd` |
| **Spotify Album** | `spotify:album:ID` | `spotify:album:4aawyAB9vmqN3uQ7FjRGTy` |
| **Spotify Artist** | `spotify:artist:ID` | `spotify:artist:6APm8EjxOHSYM5B4i3vT3q` |
| **Spotify Track** | `spotify:track:ID` | `spotify:track:17GmwQ9Q3MTAz05OokmNNB` |
| **TUNEIN Radio** | `/v1/playback/station/ID` | `/v1/playback/station/s33828` |
| **Internet Radio** | `URL or encoded URL` | `https://stream.example.com/radio` |
| **STORED_MUSIC** | `Container ID` | `6_a2874b5d_4f83d999` |
| **LOCAL_MUSIC** | `album:ID` or `track:ID` | `album:983`, `track:2579` |
| **PANDORA Station** | `Station ID` | `126740707481236361` |

## Conclusion

The `/storePreset` feature is **highly feasible** and would add significant value to the SoundTouch API client. The existing infrastructure provides a solid foundation, and the implementation would be straightforward.

Key benefits:
- ✅ **User-friendly**: Simple CLI commands for preset management with automatic location detection
- ✅ **Universal**: Supports ALL content sources (Spotify, TUNEIN, Internet Radio, NAS Music, Pandora, Local Music)
- ✅ **Well-documented**: Complete API specification available via [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- ✅ **Event-driven**: WebSocket integration for real-time updates
- ✅ **Low complexity**: Leverages existing code patterns and infrastructure
- ✅ **Enhanced CLI**: Automatic location display makes it easy to capture preset data

This feature would enable SoundTouch users to fully utilize their device's preset capabilities programmatically, making it easier to manage and access their favorite content from any supported source. **Special thanks to the SoundTouch Plus community for documenting these working endpoints that weren't included in the official API documentation.**
