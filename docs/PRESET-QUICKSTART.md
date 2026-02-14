# Preset Management Quick Start Guide

**Save your favorite music, radio stations, and playlists as 1-6 presets for instant access.**

## Overview

SoundTouch devices support 6 preset slots that can store your favorite content for instant access. This guide shows you how to manage presets using both the CLI and Go library.

## Quick CLI Usage

### 1. See Current Presets
```bash
soundtouch-cli --host 192.168.1.100 preset list
```

### 2. Store What's Currently Playing
```bash
# Store current song/station as preset 1
soundtouch-cli --host 192.168.1.100 preset store-current --slot 1
```

### 3. Store Specific Content

#### Spotify Playlist
```bash
soundtouch-cli --host 192.168.1.100 preset store \
  --slot 2 \
  --source SPOTIFY \
  --location "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M" \
  --name "Today's Top Hits"
```

#### Radio Station
```bash
soundtouch-cli --host 192.168.1.100 preset store \
  --slot 3 \
  --source TUNEIN \
  --location "/v1/playbook/station/s33828" \
  --name "K-LOVE Radio"
```

### 4. Use Your Presets
```bash
# Play preset 1
soundtouch-cli --host 192.168.1.100 preset select --slot 1

# Play preset 2
soundtouch-cli --host 192.168.1.100 preset select --slot 2
```

### 5. Remove Presets
```bash
# Remove preset 6
soundtouch-cli --host 192.168.1.100 preset remove --slot 6
```

## Getting Content Locations

To store specific content, you need the `location` parameter. Here's how to get it:

### Method 1: From Currently Playing Content
```bash
# Play the content you want to save, then:
soundtouch-cli --host 192.168.1.100 play now
```

**Example output:**
```
Now Playing:
  Track: Bohemian Rhapsody
  Artist: Queen
  Source: SPOTIFY

Content Details:
  Location: spotify:track:17GmwQ9Q3MTAz05OokmNNB  ← Use this!
```

### Method 2: Convert Spotify URLs
If you have a Spotify web URL, convert it to a URI:

- **URL**: `https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M`
- **URI**: `spotify:playlist:37i9dQZF1DXcBWIGoYBM5M`

Just replace `https://open.spotify.com/` with `spotify:` and `/` with `:`.

## Common Content Types

### Spotify Content
```bash
# Playlist
--source SPOTIFY --location "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M"

# Album
--source SPOTIFY --location "spotify:album:4aawyAB9vmqN3uQ7FjRGTy"

# Artist
--source SPOTIFY --location "spotify:artist:6APm8EjxOHSYM5B4i3vT3q"

# Track
--source SPOTIFY --location "spotify:track:17GmwQ9Q3MTAz05OokmNNB"
```

### Radio Stations
```bash
# TuneIn Radio
--source TUNEIN --location "/v1/playbook/station/s33828"

# Internet Radio Stream
--source LOCAL_INTERNET_RADIO --location "https://stream.example.com/jazz"
```

### Local Music (NAS/USB)
```bash
# Album from local storage
--source STORED_MUSIC --location "album:983"

# Track from local storage  
--source STORED_MUSIC --location "track:2579"
```

## Go Library Usage

### Basic Operations
```go
package main

import (
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    // Create client
    c := client.NewClient(&client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    })
    
    // List current presets
    presets, err := c.GetPresets()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found %d presets\n", len(presets.Preset))
    
    // Store current content as preset 1
    err = c.StoreCurrentAsPreset(1)
    if err != nil {
        log.Fatal(err)
    }
    
    // Store Spotify playlist as preset 2
    content := &models.ContentItem{
        Source:        "SPOTIFY",
        Type:          "uri",
        Location:      "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
        SourceAccount: "username",
        IsPresetable:  true,
        ItemName:      "My Favorites",
    }
    err = c.StorePreset(2, content)
    if err != nil {
        log.Fatal(err)
    }
    
    // Select preset 1
    err = c.SelectPreset(1)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Smart Preset Management
```go
// Find next available slot automatically
nextSlot, err := c.GetNextAvailablePresetSlot()
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Next available slot: %d\n", nextSlot)

// Check if current content can be saved
presetable, err := c.IsCurrentContentPresetable()
if err != nil {
    log.Fatal(err)
}
if presetable {
    c.StoreCurrentAsPreset(nextSlot)
}

// Get preset by ID
presets, _ := c.GetPresets()
preset := presets.GetPresetByID(1)
if preset != nil && !preset.IsEmpty() {
    fmt.Printf("Preset 1: %s\n", preset.GetDisplayName())
}
```

## Real-Time Preset Events

Monitor preset changes in real-time using WebSocket events:

```go
// Create WebSocket client
wsClient := c.NewWebSocketClient(nil)

// Handle preset updates
wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
    fmt.Printf("Presets updated on device %s\n", event.DeviceID)
    for _, preset := range event.Presets.Preset {
        if !preset.IsEmpty() {
            fmt.Printf("  Preset %d: %s (%s)\n", 
                preset.ID, preset.GetDisplayName(), preset.GetSource())
        }
    }
})

// Connect and listen
err := wsClient.Connect()
if err != nil {
    log.Fatal(err)
}
defer wsClient.Close()

// Keep listening for events
select {} // Run forever
```

## Practical Examples

### Family Setup
```bash
# Dad's morning playlist
soundtouch-cli --host 192.168.1.100 preset store \
  --slot 1 --source SPOTIFY \
  --location "spotify:playlist:morning-energy" \
  --name "Dad's Morning Mix"

# Mom's cooking music
soundtouch-cli --host 192.168.1.100 preset store \
  --slot 2 --source SPOTIFY \
  --location "spotify:playlist:cooking-vibes" \
  --name "Kitchen Tunes"

# Kids' bedtime stories
soundtouch-cli --host 192.168.1.100 preset store \
  --slot 3 --source TUNEIN \
  --location "/v1/playbook/station/bedtime-stories" \
  --name "Bedtime Stories"
```

### Party Mode
```bash
# Upbeat party playlist
soundtouch-cli --host 192.168.1.100 preset store-current --slot 1

# Chill background music  
soundtouch-cli --host 192.168.1.100 preset store-current --slot 2

# Dance music
soundtouch-cli --host 192.168.1.100 preset store-current --slot 3
```

### Smart Home Integration
```bash
# Morning routine (preset 1) - triggered by smart home at 7 AM
soundtouch-cli --host 192.168.1.100 preset select --slot 1

# Evening routine (preset 2) - triggered at sunset
soundtouch-cli --host 192.168.1.100 preset select --slot 2
```

## Troubleshooting

### "Content is not presetable"
Not all content can be saved as presets:
- ✅ **Works**: Spotify, TuneIn, Internet Radio, Local Music
- ❌ **Doesn't work**: Bluetooth, AUX, AirPlay (live sources)

**Solution**: Switch to a supported source first.

### "All preset slots are occupied"
```bash
# See which presets you have
soundtouch-cli --host 192.168.1.100 preset list

# Remove one you don't need
soundtouch-cli --host 192.168.1.100 preset remove --slot 6

# Or overwrite an existing one
soundtouch-cli --host 192.168.1.100 preset store-current --slot 6
```

### Getting Spotify URIs
If you can't find Spotify URIs:

1. **Play the content** in Spotify on your SoundTouch
2. **Check what's playing**: `soundtouch-cli --host 192.168.1.100 play now`
3. **Copy the location** from the output

### Device Connection Issues
```bash
# Test connection first
soundtouch-cli --host 192.168.1.100 info

# If that fails, check:
# - Device IP address is correct
# - Device is powered on
# - Network connectivity
```

## Best Practices

### Preset Organization
- **Slot 1-2**: Daily favorites (morning playlist, news)
- **Slot 3-4**: Mood music (workout, relaxation)  
- **Slot 5-6**: Special content (party music, kids' content)

### Content Management
- Use descriptive `--name` parameters for easy identification
- Store both individual tracks and playlists for variety
- Keep at least one slot free for temporary content

### Automation Ideas
- Create shell scripts for common preset operations
- Use with smart home systems for scheduled music
- Integrate with calendar events (work music during work hours)

## Next Steps

- 📖 [Complete CLI Reference](guides/CLI-REFERENCE.md)
- 🔧 [Full Implementation Guide](reference/PRESET-MANAGEMENT.md)
- 📡 [WebSocket Events Documentation](reference/WEBSOCKET-EVENTS.md)
- 💻 [Preset Management Example](../examples/preset-management/)
- 📚 [API Endpoints Overview](reference/API-ENDPOINTS.md)

## Need Help?

- 🐛 **Bug Reports**: [Create an issue](https://github.com/gesellix/bose-soundtouch/issues)
- 💡 **Feature Requests**: [Start a discussion](https://github.com/gesellix/bose-soundtouch/discussions)
- ❓ **Questions**: [Browse discussions](https://github.com/gesellix/bose-soundtouch/discussions)
