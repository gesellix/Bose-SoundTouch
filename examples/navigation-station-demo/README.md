# Navigation & Station Management Demo

This example demonstrates the comprehensive content navigation and station management capabilities of the Bose SoundTouch API client.

## Features Demonstrated

### Content Navigation
- **Browse TuneIn Stations**: Discover available radio stations
- **Content Pagination**: Navigate through large content collections
- **Source-Specific Browsing**: Browse different content sources (TuneIn, Pandora, Spotify, local music)
- **Container Navigation**: Browse into directories and folders

### Station Search & Discovery
- **TuneIn Search**: Find radio stations by genre, name, or description
- **Multi-Source Search**: Search across TuneIn, Pandora, and Spotify
- **Rich Results**: Get songs, artists, and stations with metadata
- **Token Extraction**: Get station tokens for immediate playback

### Station Management
- **Add & Play**: Add stations and start playing immediately
- **Station Removal**: Remove stations from collections
- **Real-time Playback**: Immediate feedback on what's playing

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

### 2. Run the Demo

```bash
# Navigate to example directory
cd examples/navigation-station-demo

# Run with your device IP
go run . 192.168.1.100
```

## What the Demo Does

### Step-by-Step Demonstration

1. **📻 Browse TuneIn**: Lists available radio stations
2. **🔍 Search Jazz**: Searches TuneIn for jazz-related content  
3. **➕ Add Station**: Adds a station from search results and plays it
4. **🎵 Pandora Demo**: Shows how Pandora search would work (requires account)
5. **💿 Stored Music**: Shows how to browse local music libraries
6. **🎧 Spotify Demo**: Shows how Spotify search would work (requires account)

### Example Output

```
🎵 SoundTouch Navigation & Station Management Demo
📱 Device: 192.168.1.100:8090

📻 Step 1: Browsing TuneIn stations...
  📡 Getting TuneIn stations (first 10)...
  📻 Found 2847 total TuneIn stations
  🎵 Sample stations:
    1. BBC Radio 1
       ▶️  Playable
    2. Classic FM
       ▶️  Playable
    3. Jazz FM
       ▶️  Playable

🔍 Step 2: Searching for jazz stations...
  🎷 Searching TuneIn for 'jazz'...
  📊 Search results: 25 total
  📻 Stations (18):
    1. Jazz FM (Token: c121508)
    2. Smooth Jazz 24/7 (Token: c456789)
    3. NYC Jazz Radio (Token: c789123)

➕ Step 3: Adding and playing a station...
  ➕ Adding station: Jazz FM
  🎯 Token: c121508
  ✅ Successfully added and started playing: Jazz FM
  🎵 Checking what's now playing...
      Now Playing: Blue Moon
      Source: TUNEIN

✅ Navigation and station management demo completed!
```

## Understanding the Code

### Basic Navigation Operations

```go
// Browse TuneIn stations with pagination
response, err := client.Navigate("TUNEIN", "", 1, 10)

// Browse with menu navigation (for Pandora)
response, err := client.NavigateWithMenu("PANDORA", account, "radioStations", "dateCreated", 1, 20)

// Browse into a container/directory
containerItem := &models.ContentItem{
    Source:   "STORED_MUSIC",
    Location: "album:983",
    Type:     "dir",
}
response, err := client.NavigateContainer("STORED_MUSIC", deviceID, 1, 50, containerItem)
```

### Station Search Operations

```go
// Search TuneIn for content
searchResults, err := client.SearchTuneInStations("jazz")

// Search Pandora stations (requires account)
searchResults, err := client.SearchPandoraStations("pandora_account", "rock")

// Search Spotify content (requires account)
searchResults, err := client.SearchSpotifyContent("spotify_username", "workout")

// Process search results
songs := searchResults.GetSongs()
artists := searchResults.GetArtists()
stations := searchResults.GetStations()
```

### Station Management Operations

```go
// Add station and play immediately
err := client.AddStation("TUNEIN", "", "c121508", "Jazz FM")

// Remove station from collection
contentItem := &models.ContentItem{
    Source:   "TUNEIN",
    Location: "/v1/playbook/station/s33828",
}
err := client.RemoveStation(contentItem)
```

## Content Source Requirements

### TuneIn Radio
- ✅ **No account required** for basic browsing and search
- ✅ **Public content** - works immediately
- 🎯 **Best for**: Radio stations, podcasts, news

### Pandora
- ⚠️ **Account required** - need valid Pandora username
- 🔐 **Account-specific content** - shows user's personalized stations
- 🎯 **Best for**: Personalized radio stations, music discovery

### Spotify
- ⚠️ **Account required** - need valid Spotify username
- 🔐 **Account-specific content** - shows user's playlists and saved content
- 🎯 **Best for**: Playlists, albums, tracks, artists

### Stored Music
- ⚠️ **Device ID required** - need SoundTouch device identifier
- 💾 **Local content** - music stored on NAS or USB drives
- 🎯 **Best for**: Personal music collections, local libraries

## CLI Command Equivalents

This example shows programmatic usage. For command-line usage:

```bash
# Browse TuneIn stations
go run ./cmd/soundtouch-cli --host 192.168.1.100 browse tunein

# Search for jazz stations
go run ./cmd/soundtouch-cli --host 192.168.1.100 station search-tunein --query "jazz"

# Add a station from search results
go run ./cmd/soundtouch-cli --host 192.168.1.100 station add \
  --source TUNEIN \
  --token "c121508" \
  --name "Jazz FM"

# Browse Pandora stations (requires account)
go run ./cmd/soundtouch-cli --host 192.168.1.100 browse pandora \
  --source-account "your_pandora_username"

# Search Spotify content (requires account)
go run ./cmd/soundtouch-cli --host 192.168.1.100 station search-spotify \
  --source-account "your_spotify_username" \
  --query "workout playlist"
```

## Workflow Patterns

### Discover → Search → Play Workflow

```go
// 1. Browse available content
tuneInStations, _ := client.Navigate("TUNEIN", "", 1, 20)

// 2. Search for specific content
jazzResults, _ := client.SearchTuneInStations("smooth jazz")

// 3. Add and play immediately
stations := jazzResults.GetStations()
if len(stations) > 0 {
    station := stations[0]
    client.AddStation("TUNEIN", "", station.Token, station.Name)
}
```

### Pagination Pattern

```go
// Browse large collections with pagination
start := 1
limit := 20
totalShown := 0

for {
    response, err := client.Navigate("TUNEIN", "", start, limit)
    if err != nil || len(response.Items) == 0 {
        break
    }
    
    // Process current page
    for _, item := range response.Items {
        fmt.Printf("%s\n", item.GetDisplayName())
    }
    
    totalShown += len(response.Items)
    if totalShown >= response.TotalItems {
        break
    }
    
    start += limit
}
```

## Error Scenarios

The demo handles common error cases:

- **Account Required**: Shows placeholder behavior for Pandora/Spotify without accounts
- **No Search Results**: Continues demo even if searches return empty
- **Station Add Failure**: Shows error message but continues with demo
- **Device Unavailable**: Fails gracefully with meaningful error messages

## Troubleshooting

### "No stations found"
- TuneIn might be temporarily unavailable
- Network connectivity issues
- Try searching for more common terms like "rock" or "news"

### "Account required" for Pandora/Spotify
- These services require valid user accounts
- Replace placeholder account names with real usernames
- Ensure accounts are properly configured on your SoundTouch device

### "Device not responding"
```bash
# Test basic connectivity first
go run ./cmd/soundtouch-cli --host 192.168.1.100 info
```

### "Search returns no results"
- Try broader search terms
- Check if the service is available in your region
- Ensure your SoundTouch device has internet connectivity

## Related Documentation

- [CLI Reference](../../docs/guides/CLI-REFERENCE.md) - Browse and station commands
- [Navigation Guide](../../docs/guides/SURVIVAL-GUIDE.md) - Comprehensive navigation documentation
- [Navigation API Reference](../../docs/API-NAVIGATION-REFERENCE.md) - Technical API details
- [WebSocket Events](../../docs/reference/WEBSOCKET-EVENTS.md) - Real-time event handling

## Use Cases

This example demonstrates patterns for:

- **Music Discovery**: Find new radio stations and content
- **Direct Playback**: Play content without storing as presets first
- **Content Exploration**: Browse large music libraries efficiently
- **Smart Home Integration**: Programmatically start specific content
- **Personalized Experiences**: Access account-specific content from streaming services
