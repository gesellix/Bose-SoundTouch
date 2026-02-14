# Preset Management Example

This example demonstrates comprehensive preset management functionality for Bose SoundTouch devices.

## Features Demonstrated

### Core Preset Operations
- **List Presets**: View all configured presets with details
- **Store Current Content**: Save what's currently playing as a preset
- **Store Specific Content**: Save Spotify playlists, radio stations, etc.
- **Select Presets**: Choose and play a specific preset
- **Remove Presets**: Delete unwanted presets
- **WebSocket Events**: Monitor real-time preset updates

### Content Types Supported
- **Spotify**: Playlists, albums, artists, tracks
- **Radio Stations**: TuneIn, local internet radio
- **Local Music**: NAS storage, local libraries
- **Other Sources**: Any presetable content source

## Prerequisites

1. **Go 1.21+** installed on your system
2. **SoundTouch Device** on your network
3. **Device IP Address** (use discovery to find it)

## Running the Example

### 1. Find Your Device IP

```bash
# From project root
go run ./cmd/soundtouch-cli discover devices
```

### 2. Run the Example

```bash
# Navigate to example directory
cd examples/preset-management

# Run with your device IP
go run . 192.168.1.100
```

## What the Example Does

### Step-by-Step Demonstration

1. **📋 Current Presets**: Lists all configured presets
2. **🔍 Content Check**: Analyzes what's currently playing
3. **💾 Store Current**: Saves current content as preset (if presetable)
4. **💿 Store Spotify**: Demonstrates storing a Spotify playlist
5. **📻 Store Radio**: Demonstrates storing a radio station
6. **📋 Updated List**: Shows presets after changes
7. **🎯 Select Preset**: Plays preset #1
8. **📡 WebSocket Demo**: Shows real-time preset events

### Example Output

```
🎵 SoundTouch Preset Management Example
📱 Device: 192.168.1.100:8090

📋 Step 1: Getting current presets...
  📻 Found 2 configured presets:
    1. Morning Jazz
       Source: SPOTIFY
       Location: spotify:playlist:37i9dQZF1DXcBWIGoYBM5M
       Created: 2024-01-15 08:30:00

    2. K-LOVE Radio
       Source: TUNEIN
       Location: /v1/playbook/station/s33828
       Created: 2024-01-15 09:15:00

  🆓 Available slots: [3 4 5 6]

🔍 Step 2: Checking current content...
  🎵 Now Playing: Bohemian Rhapsody
      Artist: Queen
      Source: SPOTIFY
      Presetable: true
      Location: spotify:track:17GmwQ9Q3MTAz05OokmNNB

💾 Step 3: Storing current content as preset...
  💾 Storing current content as preset 3...
  ✅ Successfully stored as preset 3

📡 Step 8: Demonstrating preset events...
  📡 Connecting to WebSocket for real-time events...
  ✅ WebSocket connected, listening for preset events...
  🔄 Making a preset change to trigger an event...
  💾 Storing test preset 4 to trigger event...
  ⏳ Waiting 3 seconds for WebSocket event...
  📡 Preset Update Event Received!
      Device: A81B6A536A98
      Presets count: 4
      - Preset 1: Morning Jazz (SPOTIFY)
      - Preset 2: K-LOVE Radio (TUNEIN)
      - Preset 3: Bohemian Rhapsody (SPOTIFY)
      - Preset 4: BBC Radio 1 (TUNEIN)

✅ Preset management demo completed!
```

## Understanding the Code

### Basic Preset Operations

```go
// Get all presets
presets, err := client.GetPresets()

// Check if current content can be saved
presetable, err := client.IsCurrentContentPresetable()

// Store current content
err = client.StoreCurrentAsPreset(slotNumber)

// Store specific content
contentItem := &models.ContentItem{
    Source:        "SPOTIFY",
    Type:          "uri",
    Location:      "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
    SourceAccount: "username",
    IsPresetable:  true,
    ItemName:      "Today's Top Hits",
}
err = client.StorePreset(slotNumber, contentItem)

// Select a preset
err = client.SelectPreset(1)

// Remove a preset
err = client.RemovePreset(6)
```

### WebSocket Event Handling

```go
// Create WebSocket client
wsClient := client.NewWebSocketClient(nil)

// Handle preset events
wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
    fmt.Printf("Presets updated on device %s\n", event.DeviceID)
    for _, preset := range event.Presets.Preset {
        if !preset.IsEmpty() {
            fmt.Printf("Preset %d: %s\n", preset.ID, preset.GetDisplayName())
        }
    }
})

// Connect and listen
err := wsClient.Connect()
defer wsClient.Close()
```

## Content Location Examples

### Spotify Content

```go
// Playlist
Location: "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M"

// Album  
Location: "spotify:album:4aawyAB9vmqN3uQ7FjRGTy"

// Artist
Location: "spotify:artist:6APm8EjxOHSYM5B4i3vT3q"

// Track
Location: "spotify:track:17GmwQ9Q3MTAz05OokmNNB"
```

### Radio Stations

```go
// TuneIn
Location: "/v1/playbook/station/s33828"

// Internet Radio
Location: "https://stream.example.com/radio"
```

## Getting Content Locations

### Method 1: From Currently Playing

```bash
# Show current content details (includes location)
go run ./cmd/soundtouch-cli --host 192.168.1.100 play now
```

### Method 2: From Spotify URLs

Convert Spotify web URLs to URIs:
- URL: `https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M`
- URI: `spotify:playlist:37i9dQZF1DXcBWIGoYBM5M`

## Error Scenarios

The example handles common error cases:

- **No Content Playing**: Gracefully handles empty now playing
- **Non-Presetable Content**: Shows when content can't be saved
- **Full Preset Slots**: Finds available slots or handles full device
- **WebSocket Issues**: Proper connection handling and cleanup

## Integration with CLI

This example shows programmatic usage. For command-line usage:

```bash
# List presets
go run ./cmd/soundtouch-cli --host 192.168.1.100 preset list

# Store current content
go run ./cmd/soundtouch-cli --host 192.168.1.100 preset store-current --slot 1

# Store specific content
go run ./cmd/soundtouch-cli --host 192.168.1.100 preset store \
  --slot 2 \
  --source SPOTIFY \
  --location "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M" \
  --name "My Playlist"

# Select preset
go run ./cmd/soundtouch-cli --host 192.168.1.100 preset select --slot 1

# Remove preset
go run ./cmd/soundtouch-cli --host 192.168.1.100 preset remove --slot 6
```

## Troubleshooting

### Device Not Found
```
Error: Failed to connect to device: connection refused
```
**Solution**: Verify device IP and ensure device is powered on

### Preset Store Failed
```
Error: Failed to store preset: content is not presetable
```
**Solution**: Not all content can be saved as presets (e.g., Bluetooth, some radio streams)

### No Available Slots
```
Error: All preset slots are occupied
```
**Solution**: Remove an existing preset first or use a specific slot number

## Related Documentation

- [CLI Reference](../../docs/guides/CLI-REFERENCE.md) - Command-line usage
- [Preset Implementation Guide](../../docs/preset-store.md) - Technical details
- [WebSocket Events](../../docs/reference/WEBSOCKET-EVENTS.md) - Real-time event handling
- [API Reference](../../docs/reference/API-ENDPOINTS.md) - Complete API documentation

## Use Cases

This example demonstrates patterns for:

- **Smart Home Automation**: Trigger presets based on time/events
- **Music Management**: Organize favorite content into quick-access presets
- **Family Scenarios**: Each person gets their own preset slots
- **Party Mode**: Pre-configure playlists for different moods
- **Radio Favorites**: Save frequently listened radio stations
