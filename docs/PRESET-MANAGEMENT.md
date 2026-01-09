# Preset Management - Bose SoundTouch API

This document covers preset management functionality in the Bose SoundTouch API client.

## Overview

Bose SoundTouch devices support up to 6 presets that can store favorite music sources, playlists, radio stations, and other audio content. The API provides comprehensive **read access** to preset information, while **write access** (creating/updating presets) is officially not supported by the API.

## Current Implementation Status

### ✅ **Fully Supported - Read Operations**
- `GET /presets` - Retrieve all configured presets
- Comprehensive preset analysis and helper methods
- Integration with CLI tool for viewing presets

### ❌ **Not Supported - Write Operations**  
- `POST /presets` - Officially marked as "N/A" in Bose API documentation
- Preset clearing/deletion via API
- Direct preset creation from currently playing content

### ✅ **Supported Alternatives**
- Use official Bose SoundTouch mobile app (iOS/Android)
- Use physical preset buttons on the device (long-press while playing content)
- Changes made via these methods are visible through the API's GET endpoint

## Reading Presets

### CLI Usage

```bash
# Get all configured presets
soundtouch-cli -host 192.168.1.10 -presets
```

**Example Output:**
```
Configured Presets:
  Used Slots: 6/6
  Spotify Presets: 6

Preset 1: My Favorite Playlist
  Source: SPOTIFY (user@example.com)
  Type: tracklisturl
  Created: 2024-04-30 07:37:40
  Updated: 2024-04-30 07:37:40
  Artwork: https://i.scdn.co/image/...

Preset 2: Rock Hits Radio
  Source: TUNEIN
  Type: station
  Artwork: https://cdn-radiotime-logos.tunein.com/...

Available Slots: []
Most Recent: Preset 1 (My Favorite Playlist)
```

### Go Library Usage

```go
package main

import (
    "fmt"
    "github.com/gesellix/bose-soundtouch/pkg/client"
)

func main() {
    // Create client
    soundtouchClient := client.NewClientFromHost("192.168.1.10")
    
    // Get all presets
    presets, err := soundtouchClient.GetPresets()
    if err != nil {
        panic(err)
    }
    
    // Display preset information
    fmt.Printf("Total presets: %d\n", presets.GetPresetCount())
    fmt.Printf("Used slots: %d\n", len(presets.GetUsedPresetSlots()))
    fmt.Printf("Empty slots: %v\n", presets.GetEmptyPresetSlots())
    
    // Check for Spotify presets
    spotifyPresets := presets.GetSpotifyPresets()
    fmt.Printf("Spotify presets: %d\n", len(spotifyPresets))
    
    // Get specific preset
    preset1 := presets.GetPresetByID(1)
    if preset1 != nil && !preset1.IsEmpty() {
        fmt.Printf("Preset 1: %s\n", preset1.GetDisplayName())
        fmt.Printf("  Source: %s\n", preset1.GetSource())
        fmt.Printf("  Type: %s\n", preset1.GetContentType())
        
        if preset1.HasTimestamps() {
            fmt.Printf("  Created: %s\n", preset1.GetCreatedTime())
            fmt.Printf("  Updated: %s\n", preset1.GetUpdatedTime())
        }
    }
    
    // Find most recent preset
    if recent := presets.GetMostRecentPreset(); recent != nil {
        fmt.Printf("Most recent: Preset %d (%s)\n", 
            recent.ID, recent.GetDisplayName())
    }
}
```

## Preset Data Structure

### XML Response Format
```xml
<presets>
  <preset id="1" createdOn="1745991460" updatedOn="1745991460">
    <ContentItem source="SPOTIFY" type="tracklisturl" 
                 location="/playback/container/..." 
                 sourceAccount="user@example.com" 
                 isPresetable="true">
      <itemName>My Favorite Songs</itemName>
      <containerArt>https://i.scdn.co/image/...</containerArt>
    </ContentItem>
  </preset>
  <preset id="2">
    <ContentItem source="TUNEIN" type="station" 
                 location="s12345" isPresetable="true">
      <itemName>Classic Rock Radio</itemName>
      <containerArt>https://cdn-radiotime-logos.tunein.com/...</containerArt>
    </ContentItem>
  </preset>
</presets>
```

### Go Model
```go
type Presets struct {
    XMLName xml.Name `xml:"presets"`
    Preset  []Preset `xml:"preset"`
}

type Preset struct {
    XMLName     xml.Name     `xml:"preset"`
    ID          int          `xml:"id,attr"`
    CreatedOn   *int64       `xml:"createdOn,attr,omitempty"`
    UpdatedOn   *int64       `xml:"updatedOn,attr,omitempty"`
    ContentItem *ContentItem `xml:"ContentItem,omitempty"`
}
```

## Helper Methods

### Preset Analysis
```go
// Get preset by ID
preset := presets.GetPresetByID(3)

// Check if preset has content
if !preset.IsEmpty() {
    // Use preset
}

// Get presets by source type
spotifyPresets := presets.GetPresetsBySource("SPOTIFY")
tuneInPresets := presets.GetPresetsBySource("TUNEIN")

// Find available slots
emptySlots := presets.GetEmptyPresetSlots()  // Returns [4, 5] if slots 4-5 are empty
usedSlots := presets.GetUsedPresetSlots()    // Returns [1, 2, 3, 6] if those are used
```

### Preset Content Analysis
```go
// Get display information
name := preset.GetDisplayName()         // "My Playlist" or "Preset 1" fallback
source := preset.GetSource()           // "SPOTIFY", "TUNEIN", etc.
account := preset.GetSourceAccount()   // "user@example.com"
contentType := preset.GetContentType() // "playlist", "station", etc.
artwork := preset.GetArtworkURL()      // Album/station artwork URL

// Check preset characteristics
isSpotify := preset.IsSpotifyPreset()
isPresetable := preset.IsPresetable()

// Time information (if available)
if preset.HasTimestamps() {
    created := preset.GetCreatedTime()
    updated := preset.GetUpdatedTime()
}
```

### Content Type Examples

Common content types found in presets:

| Source | Type | Description | Example Location |
|--------|------|-------------|------------------|
| `SPOTIFY` | `tracklisturl` | Playlist/Album | `/playback/container/c3Bv...` |
| `SPOTIFY` | `track` | Single Track | `/playback/container/c3Bv...` |
| `TUNEIN` | `station` | Radio Station | `s12345` |
| `PANDORA` | `station` | Pandora Station | `TR:station:12345` |
| `AMAZON` | `playlist` | Amazon Playlist | `amzn1.dv.gti...` |

## Preset Selection

While you cannot create presets via API, you can select existing presets:

### Via Key Commands
```bash
# Select preset 1-6 using key commands
soundtouch-cli -host 192.168.1.10 -preset 1
soundtouch-cli -host 192.168.1.10 -key PRESET_3
```

### Via Go Library
```go
// Select preset using key command
err := soundtouchClient.SelectPreset(1)

// Or use direct key command
err := soundtouchClient.SendKey("PRESET_1")
```

## Limitations and Workarounds

### API Design Limitations
1. **No API-based preset creation** - `POST /presets` is officially marked as "N/A" in Bose documentation
2. **No preset deletion** - Cannot clear preset slots via API (by design)
3. **No preset modification** - Cannot update existing preset content via API (by design)
4. **Read-only access** - API intentionally provides comprehensive read access only

### Working Alternatives

#### 1. Official Bose SoundTouch App
- iOS/Android app allows full preset management
- Can create, update, and delete presets
- Changes sync automatically with device

#### 2. Physical Device Controls
- Use preset buttons (1-6) on the device
- Long-press while content is playing to save as preset
- Short-press to select saved preset

#### 3. Web Interface (if available)
- Some devices may have a web interface
- Access via `http://device-ip:8090` in browser
- May provide preset management controls

## Checking Presetability

Before attempting to save content as a preset (via app/hardware), you can check if the current content supports preset saving:

```go
// Check if currently playing content can be saved as preset
nowPlaying, err := client.GetNowPlaying()
if err != nil {
    return err
}

if nowPlaying.ContentItem != nil && nowPlaying.ContentItem.IsPresetable {
    fmt.Println("✓ Current content can be saved as a preset")
    fmt.Printf("  Content: %s\n", nowPlaying.ContentItem.ItemName)
    fmt.Printf("  Source: %s\n", nowPlaying.ContentItem.Source)
    fmt.Printf("  Type: %s\n", nowPlaying.ContentItem.Type)
} else {
    fmt.Println("✗ Current content cannot be saved as a preset")
}
```

Or use the convenience method:
```go
// Simple presetability check
presetable, err := client.IsCurrentContentPresetable()
if err != nil {
    return err
}

if presetable {
    fmt.Println("✓ Content is presetable - use app or device buttons to save")
} else {
    fmt.Println("✗ Content cannot be saved as preset")
}
```

## Best Practices

### 1. Check Available Slots
```go
presets, err := client.GetPresets()
if err != nil {
    return err
}

emptySlots := presets.GetEmptyPresetSlots()
if len(emptySlots) == 0 {
    fmt.Println("All preset slots are occupied")
    // Consider which preset to overwrite
} else {
    fmt.Printf("Available preset slots: %v\n", emptySlots)
}
```

### 2. Analyze Current Presets
```go
// Get summary statistics
summary := presets.GetPresetsSummary()
fmt.Printf("Total: %d, Used: %d, Empty: %d\n", 
    summary["total"], summary["used"], summary["empty"])

// Check source distribution
if summary["SPOTIFY"] > 0 {
    fmt.Printf("Spotify presets: %d\n", summary["SPOTIFY"])
}
if summary["TUNEIN"] > 0 {
    fmt.Printf("TuneIn presets: %d\n", summary["TUNEIN"])
}
```

### 3. Handle Preset History
```go
// Find recently used presets
if recent := presets.GetMostRecentPreset(); recent != nil {
    fmt.Printf("Most recently updated: Preset %d (%s)\n", 
        recent.ID, recent.GetDisplayName())
}

if oldest := presets.GetOldestPreset(); oldest != nil {
    fmt.Printf("Oldest preset: Preset %d (%s)\n", 
        oldest.ID, oldest.GetDisplayName())
}
```

## Future Development

### API Design Decision
Based on the official Bose SoundTouch API documentation, preset creation via API is intentionally not supported. This is likely a design decision to:
1. **Maintain user control** - Presets are personal configurations best managed by the user
2. **Prevent accidental overrides** - Avoid third-party apps accidentally modifying user presets
3. **Ensure UI consistency** - Keep preset management in official interfaces
4. **Security considerations** - Limit configuration changes to authenticated official apps

### No Further Investigation Needed
The preset creation limitation is **not a bug or missing feature** - it's the intended API design. The comprehensive read access provides everything needed for applications to work with existing user configurations.

## Related Documentation

- [API Endpoints Overview](API-Endpoints-Overview.md) - Complete API reference
- [Volume Controls](VOLUME-CONTROLS.md) - Volume management
- [Key Controls](KEY-CONTROLS.md) - Media control commands
- [Source Selection](SOURCE-SELECTION.md) - Audio source management

## Summary

Preset management in the Bose SoundTouch API is **intentionally read-only** by design. The API provides excellent capabilities for analyzing and understanding preset configurations, but preset creation must be done through official channels (app or device). This is a deliberate design decision that respects user control over their personal preset configurations.

For most use cases, reading preset information is sufficient for building applications that work with existing user configurations. For preset creation, guide users to use the official app or device controls, which provide the proper user experience and validation.