# Recents Endpoint Example

This example demonstrates how to use the `/recents` endpoint to retrieve and analyze recently played content from your SoundTouch device.

## What is the Recents Endpoint?

The recents endpoint provides access to the device's recently played content history, including:

- **Recently played tracks** from various music services
- **Radio stations** that were recently listened to
- **Playlists and albums** that were recently accessed
- **Local music** files that were recently played
- **Metadata** including play timestamps, content types, and source information
- **Filtering capabilities** by source type and content type

## Usage

```bash
# Basic usage - show last 10 items
go run main.go -host 192.168.1.100

# Show detailed information for all items
go run main.go -host 192.168.1.100 -detailed -limit 0

# Filter by source (show only Spotify items)
go run main.go -host 192.168.1.100 -source SPOTIFY

# Filter by content type (show only tracks)
go run main.go -host 192.168.1.100 -type track

# Show statistics only
go run main.go -host 192.168.1.100 -stats

# Combined filters with custom limit
go run main.go -host 192.168.1.100 -source LOCAL_MUSIC -type track -limit 5 -detailed
```

## Command Line Options

- `-host` - **Required**: SoundTouch device IP address
- `-detailed` - Show detailed information for each item (default: false)
- `-limit` - Maximum number of items to display, 0 for all (default: 10)
- `-source` - Filter by source (SPOTIFY, LOCAL_MUSIC, TUNEIN, etc.)
- `-type` - Filter by content type (track, station, playlist, album, presetable)
- `-stats` - Show statistics only (default: false)
- `-timeout` - Request timeout duration (default: 10s)

## Example Output

### Basic Listing
```
Getting recent items from 192.168.1.100

📊 Recent Items Summary:
   Showing: 5 items (of 15 total)
   By Source: Spotify: 3, Local: 1, TuneIn: 1

=== Recent Items ===
1. 🎵 Shape of You - Ed Sheeran
   Source: Spotify | Type: Track
   Played: 2023-12-14 15:30:22 (2 hours ago)

2. 📻 BBC Radio 1
   Source: TuneIn Radio | Type: Stationurl
   Played: 2023-12-14 13:15:45 (4 hours ago)

3. 🎵 Local Song.mp3
   Source: Local Music | Type: Track
   Played: 2023-12-14 10:45:12 (7 hours ago)

💡 Showing 3 of 15 total items
   Use -limit 0 to show all items
```

### Detailed Information
```
1. 🎵 Shape of You - Ed Sheeran
   Source: Spotify | Type: Track
   Played: 2023-12-14 15:30:22 (2 hours ago)
   ID: spotify123
   ⭐ Can be saved as preset
   🎨 Has artwork
   📍 Location: spotify:track:4iV5W9uYEdYUVa79Axb7Rh
   👤 Account: spotify_user
   🏷️  Type: Streaming
```

### Statistics View
```
📊 Recent Items Statistics

Overall Statistics:
  Total Items: 25
  Last Played: 2023-12-14 15:30:22

📍 By Source:
  Spotify         15 items ( 60.0%)
  Local Music      6 items ( 24.0%)
  TuneIn           3 items ( 12.0%)
  Pandora          1 items (  4.0%)

🎼 By Content Type:
  Tracks          20 items ( 80.0%)
  Stations         4 items ( 16.0%)
  Playlists/Albums 1 items (  4.0%)

⭐ Special Categories:
  Presetable      18 items ( 72.0%)

📡 Source Analysis:
  Streaming       19 items ( 76.0%)
  Local            6 items ( 24.0%)

🕐 Time Analysis:
  Today           12 items
  Yesterday        8 items
  This Week        3 items
  Older            2 items
```

## Supported Sources

- **SPOTIFY** - Spotify streaming service
- **LOCAL_MUSIC** - Local music files
- **STORED_MUSIC** - Stored music library
- **TUNEIN** - TuneIn radio stations
- **PANDORA** - Pandora music service
- **AMAZON** - Amazon Music
- **DEEZER** - Deezer streaming
- **IHEART** - iHeartRadio
- **BLUETOOTH** - Bluetooth input
- **AUX** - AUX input
- **AIRPLAY** - AirPlay

## Content Types

- **track** - Individual songs/tracks
- **station** - Radio stations
- **playlist** - Music playlists
- **album** - Music albums
- **container** - Folders/collections
- **presetable** - Items that can be saved as presets

## Use Cases

### 1. Recently Played Music Discovery
```bash
# Find recently played Spotify tracks
go run main.go -host 192.168.1.100 -source SPOTIFY -type track -detailed
```

### 2. Radio Station History
```bash
# See what radio stations were recently played
go run main.go -host 192.168.1.100 -type station -detailed
```

### 3. Content Analytics
```bash
# Get detailed listening statistics
go run main.go -host 192.168.1.100 -stats
```

### 4. Preset Candidates
```bash
# Find content that can be saved as presets
go run main.go -host 192.168.1.100 -type presetable -limit 6
```

### 5. Local vs Streaming Analysis
```bash
# Compare local vs streaming content usage
go run main.go -host 192.168.1.100 -stats
```

## API Integration

The example demonstrates several key API patterns:

### Basic Retrieval
```go
response, err := client.GetRecents()
if err != nil {
    log.Fatal(err)
}

if response.IsEmpty() {
    fmt.Println("No recent items found")
    return
}
```

### Filtering by Source
```go
spotifyItems := response.GetSpotifyItems()
localItems := response.GetLocalMusicItems()
tuneInItems := response.GetTuneInItems()
```

### Filtering by Type
```go
tracks := response.GetTracks()
stations := response.GetStations()
presetableItems := response.GetPresetableItems()
```

### Item Analysis
```go
for _, item := range response.Items {
    if item.IsSpotifyContent() {
        fmt.Printf("Spotify track: %s\n", item.GetDisplayName())
    }
    
    if item.IsPresetable() {
        fmt.Printf("Can be saved as preset: %s\n", item.GetDisplayName())
    }
    
    if item.HasArtwork() {
        fmt.Printf("Artwork URL: %s\n", item.GetArtwork())
    }
}
```

## Error Handling

The example includes comprehensive error handling:

```bash
# Test with invalid host
go run main.go -host 192.168.255.255
# Output: Failed to get recent items: connection timeout

# Test with unknown source
go run main.go -host 192.168.1.100 -source UNKNOWN
# Output: 📭 No items found for source: UNKNOWN
#         💡 Available sources: SPOTIFY, LOCAL_MUSIC, TUNEIN

# Test with unknown type
go run main.go -host 192.168.1.100 -type unknown
# Output: ❌ Unknown type filter: unknown
#         💡 Available types: track, station, playlist, album, presetable
```

## Performance Considerations

- The recents endpoint typically returns up to 20-50 items depending on device configuration
- Response times are usually under 500ms for typical recent lists
- Use filtering to reduce processing time for large recent lists
- Consider caching results if calling frequently in applications

## Integration with Other Examples

This recents data is useful for:
- [Preset Management](../preset-management/) - Finding presetable content to save
- [Content Selection](../../docs/reference/SOURCE-SELECTION.md) - Understanding usage patterns
- [Navigation](../../docs/guides/SURVIVAL-GUIDE.md) - Quickly accessing recently played content

## Related CLI Commands

```bash
# List recent items using CLI
soundtouch-cli --host 192.168.1.100 recents list

# Filter recent items by source
soundtouch-cli --host 192.168.1.100 recents filter --source SPOTIFY

# Get recent items statistics
soundtouch-cli --host 192.168.1.100 recents stats

# Show most recent item only
soundtouch-cli --host 192.168.1.100 recents latest
```

## API Documentation

For complete API documentation, see:
- [API Reference](../../docs/reference/API-ENDPOINTS.md)
- [CLI Reference](../../docs/guides/CLI-REFERENCE.md)
- [Recents Models](../../pkg/models/recents.go)
