# Navigation and Station Management Guide

## Overview

The Bose SoundTouch Go client provides comprehensive navigation and station management functionality that allows you to:

- **Browse content sources** (TuneIn, Pandora, Spotify, stored music)
- **Search for stations and content** across music services
- **Add stations and immediately play them**
- **Remove stations from collections**
- **Navigate directory structures** in music libraries

This guide provides complete examples and best practices for using these features.

## Table of Contents

- [Quick Start](#quick-start)
- [Content Navigation](#content-navigation)
- [Station Search](#station-search)
- [Station Management](#station-management)
- [Complete Workflows](#complete-workflows)
- [Error Handling](#error-handling)
- [Best Practices](#best-practices)
- [API Reference](#api-reference)

## Quick Start

### Basic Setup

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
)

func main() {
    // Create client
    config := &client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    }
    soundtouch := client.NewClient(config)
    
    // Your navigation code here...
}
```

### Simple Navigation Example

```go
// Browse TuneIn content
response, err := soundtouch.Navigate("TUNEIN", "", 1, 25)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d items\n", response.TotalItems)
for _, item := range response.Items {
    fmt.Printf("- %s (%s)\n", item.GetDisplayName(), item.Type)
}
```

## Content Navigation

### Browse Different Sources

```go
// Browse TuneIn radio stations
tuneInStations, err := soundtouch.GetTuneInStations("")
if err != nil {
    log.Printf("TuneIn not available: %v", err)
} else {
    fmt.Printf("TuneIn has %d items\n", tuneInStations.TotalItems)
}

// Browse Pandora stations (requires account)
pandoraStations, err := soundtouch.GetPandoraStations("your_pandora_account")
if err != nil {
    log.Printf("Pandora not available: %v", err)
} else {
    stations := pandoraStations.GetStations()
    fmt.Printf("Found %d Pandora stations\n", len(stations))
}

// Browse stored music library
musicLibrary, err := soundtouch.GetStoredMusicLibrary("device_account/0")
if err != nil {
    log.Printf("Stored music not available: %v", err)
} else {
    directories := musicLibrary.GetDirectories()
    tracks := musicLibrary.GetTracks()
    fmt.Printf("Music library: %d dirs, %d tracks\n", len(directories), len(tracks))
}
```

### Navigate Into Directories

```go
// First, get the root level
musicLibrary, err := soundtouch.GetStoredMusicLibrary("device_account/0")
if err != nil {
    log.Fatal(err)
}

// Find a directory to browse into
directories := musicLibrary.GetDirectories()
if len(directories) == 0 {
    fmt.Println("No directories found")
    return
}

// Navigate into the first directory
directory := directories[0]
fmt.Printf("Browsing into: %s\n", directory.GetDisplayName())

contents, err := soundtouch.NavigateContainer(
    "STORED_MUSIC",
    "device_account/0",
    1, 100,  // Get up to 100 items starting from position 1
    directory.ContentItem,
)
if err != nil {
    log.Fatal(err)
}

// Show what's inside
tracks := contents.GetTracks()
subdirs := contents.GetDirectories()
fmt.Printf("Found %d tracks and %d subdirectories\n", len(tracks), len(subdirs))

// List first few tracks
for i, track := range tracks[:min(5, len(tracks))] {
    fmt.Printf("%d. %s", i+1, track.GetDisplayName())
    if track.ArtistName != "" {
        fmt.Printf(" - %s", track.ArtistName)
    }
    if track.AlbumName != "" {
        fmt.Printf(" [%s]", track.AlbumName)
    }
    fmt.Println()
}
```

### Advanced Navigation with Pagination

```go
// Browse large collections with pagination
const pageSize = 50
startItem := 1

for {
    response, err := soundtouch.Navigate("STORED_MUSIC", "device/0", startItem, pageSize)
    if err != nil {
        log.Fatal(err)
    }
    
    if len(response.Items) == 0 {
        break // No more items
    }
    
    fmt.Printf("Page starting at %d: %d items\n", startItem, len(response.Items))
    
    // Process this page
    for _, item := range response.Items {
        fmt.Printf("  %s (%s)\n", item.GetDisplayName(), item.Type)
    }
    
    // Move to next page
    startItem += pageSize
    
    // Stop if we've seen all items
    if startItem > response.TotalItems {
        break
    }
}
```

## Station Search

### Basic Search

```go
// Search TuneIn for jazz stations
results, err := soundtouch.SearchTuneInStations("jazz")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Found %d total results for 'jazz'\n", results.GetResultCount())

// Show different types of results
songs := results.GetSongs()
artists := results.GetArtists()
stations := results.GetStations()

fmt.Printf("Songs: %d, Artists: %d, Stations: %d\n",
    len(songs), len(artists), len(stations))
```

### Service-Specific Search

```go
// Search Pandora (requires account)
pandoraResults, err := soundtouch.SearchPandoraStations("your_account", "Taylor Swift")
if err != nil {
    log.Fatal(err)
}

// Show artists found
artists := pandoraResults.GetArtists()
for _, artist := range artists {
    fmt.Printf("Artist: %s (Token: %s)\n", artist.Name, artist.Token)
    if artist.Logo != "" {
        fmt.Printf("  Artwork: %s\n", artist.GetArtworkURL())
    }
}

// Search Spotify content
spotifyResults, err := soundtouch.SearchSpotifyContent("your_spotify_account", "Queen")
if err != nil {
    log.Fatal(err)
}

songs := spotifyResults.GetSongs()
for _, song := range songs[:min(5, len(songs))] {
    fmt.Printf("Song: %s\n", song.GetFullTitle())
}
```

### Search Result Analysis

```go
results, err := soundtouch.SearchPandoraStations("account", "classic rock")
if err != nil {
    log.Fatal(err)
}

// Analyze all results
for _, result := range results.GetAllResults() {
    fmt.Printf("Name: %s, Token: %s\n", result.GetDisplayName(), result.Token)
    
    // Determine result type
    switch {
    case result.IsSong():
        fmt.Printf("  Type: Song by %s\n", result.Artist)
    case result.IsArtist():
        fmt.Printf("  Type: Artist\n")
    case result.IsStation():
        fmt.Printf("  Type: Station")
        if result.Description != "" {
            fmt.Printf(" - %s", result.Description)
        }
        fmt.Println()
    }
}
```

## Station Management

### Adding Stations (Immediate Playback)

```go
// Search for content first
results, err := soundtouch.SearchPandoraStations("your_account", "Led Zeppelin")
if err != nil {
    log.Fatal(err)
}

// Find an artist to create a station from
artists := results.GetArtists()
if len(artists) == 0 {
    fmt.Println("No artists found")
    return
}

artist := artists[0]
stationName := artist.Name + " Radio"

// Add station - this immediately starts playing it!
err = soundtouch.AddStation("PANDORA", "your_account", artist.Token, stationName)
if err != nil {
    log.Fatal(err)
}

fmt.Printf("✓ Added and now playing: %s\n", stationName)

// The station is now:
// 1. Added to your Pandora collection
// 2. Currently playing on the device
```

### Removing Stations

```go
// First, get existing stations
stations, err := soundtouch.GetPandoraStations("your_account")
if err != nil {
    log.Fatal(err)
}

// Show current stations
fmt.Printf("Current stations (%d):\n", len(stations.Items))
for i, station := range stations.Items {
    fmt.Printf("%d. %s\n", i+1, station.GetDisplayName())
}

// Remove a specific station (example: remove the first one)
if len(stations.Items) > 0 {
    stationToRemove := stations.Items[0]
    
    if stationToRemove.ContentItem != nil {
        fmt.Printf("Removing: %s\n", stationToRemove.GetDisplayName())
        
        err := soundtouch.RemoveStation(stationToRemove.ContentItem)
        if err != nil {
            log.Printf("Failed to remove station: %v", err)
        } else {
            fmt.Println("✓ Station removed successfully")
        }
    }
}
```

### Station Collection Management

```go
// Get current collection
currentStations, err := soundtouch.GetPandoraStations("your_account")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Current collection has %d stations\n", len(currentStations.Items))

// Search for new content
searchResults, err := soundtouch.SearchPandoraStations("your_account", "indie rock")
if err != nil {
    log.Fatal(err)
}

// Add top 3 artist stations
artists := searchResults.GetArtists()
for i, artist := range artists[:min(3, len(artists))] {
    stationName := fmt.Sprintf("%s Radio", artist.Name)
    
    fmt.Printf("Adding station %d: %s\n", i+1, stationName)
    
    err := soundtouch.AddStation("PANDORA", "your_account", artist.Token, stationName)
    if err != nil {
        log.Printf("Failed to add %s: %v", stationName, err)
        continue
    }
    
    fmt.Printf("✓ Added: %s\n", stationName)
    
    // Note: Each AddStation immediately starts playing that station
    // You might want to pause between additions in a real app
}

fmt.Println("Station collection updated!")
```

## Complete Workflows

### Discover and Play Workflow

```go
func discoverAndPlayWorkflow(soundtouch *client.Client) {
    fmt.Println("=== Discover and Play Workflow ===")
    
    // Step 1: Search for content
    searchTerm := "electronic music"
    fmt.Printf("🔍 Searching for '%s'...\n", searchTerm)
    
    results, err := soundtouch.SearchTuneInStations(searchTerm)
    if err != nil {
        log.Fatal(err)
    }
    
    if results.IsEmpty() {
        fmt.Println("❌ No results found")
        return
    }
    
    // Step 2: Show options
    stations := results.GetStations()
    fmt.Printf("📻 Found %d stations:\n", len(stations))
    
    for i, station := range stations[:min(5, len(stations))] {
        fmt.Printf("%d. %s", i+1, station.GetDisplayName())
        if station.Description != "" {
            fmt.Printf(" - %s", station.Description)
        }
        fmt.Println()
    }
    
    // Step 3: Select and play (example: select first one)
    if len(stations) > 0 {
        selectedStation := stations[0]
        fmt.Printf("🎵 Playing: %s\n", selectedStation.GetDisplayName())
        
        // For services that support it, add the station to play it
        if selectedStation.Token != "" {
            err := soundtouch.AddStation("TUNEIN", "", selectedStation.Token, selectedStation.Name)
            if err != nil {
                log.Printf("Could not add station: %v", err)
            } else {
                fmt.Println("✓ Station added and playing!")
            }
        }
    }
}
```

### Library Organization Workflow

```go
func organizeLibraryWorkflow(soundtouch *client.Client, deviceAccount string) {
    fmt.Println("=== Library Organization Workflow ===")
    
    // Step 1: Explore library structure
    fmt.Println("📂 Exploring music library...")
    
    library, err := soundtouch.GetStoredMusicLibrary(deviceAccount)
    if err != nil {
        log.Fatal(err)
    }
    
    directories := library.GetDirectories()
    tracks := library.GetTracks()
    
    fmt.Printf("📊 Library overview: %d directories, %d tracks\n", 
        len(directories), len(tracks))
    
    // Step 2: Navigate into each directory
    for _, dir := range directories[:min(3, len(directories))] {
        fmt.Printf("\n📁 Exploring: %s\n", dir.GetDisplayName())
        
        contents, err := soundtouch.NavigateContainer(
            "STORED_MUSIC", deviceAccount, 1, 20, dir.ContentItem)
        if err != nil {
            log.Printf("❌ Failed to explore %s: %v", dir.GetDisplayName(), err)
            continue
        }
        
        subTracks := contents.GetTracks()
        subDirs := contents.GetDirectories()
        
        fmt.Printf("   Contains: %d tracks, %d subdirectories\n", 
            len(subTracks), len(subDirs))
        
        // Show some tracks
        for i, track := range subTracks[:min(3, len(subTracks))] {
            fmt.Printf("   %d. %s", i+1, track.GetDisplayName())
            if track.ArtistName != "" {
                fmt.Printf(" - %s", track.ArtistName)
            }
            fmt.Println()
        }
    }
    
    fmt.Println("\n✓ Library exploration complete!")
}
```

### Multi-Service Content Discovery

```go
func multiServiceDiscovery(soundtouch *client.Client, accounts map[string]string) {
    searchTerm := "jazz"
    fmt.Printf("🔍 Searching '%s' across all services...\n", searchTerm)
    
    // Search TuneIn (no account needed)
    fmt.Println("\n📻 TuneIn Results:")
    tuneInResults, err := soundtouch.SearchTuneInStations(searchTerm)
    if err != nil {
        fmt.Printf("❌ TuneIn search failed: %v\n", err)
    } else {
        stations := tuneInResults.GetStations()
        fmt.Printf("✓ Found %d TuneIn stations\n", len(stations))
        for i, station := range stations[:min(3, len(stations))] {
            fmt.Printf("  %d. %s\n", i+1, station.GetDisplayName())
        }
    }
    
    // Search Pandora (if account available)
    if pandoraAccount, ok := accounts["PANDORA"]; ok {
        fmt.Println("\n🎵 Pandora Results:")
        pandoraResults, err := soundtouch.SearchPandoraStations(pandoraAccount, searchTerm)
        if err != nil {
            fmt.Printf("❌ Pandora search failed: %v\n", err)
        } else {
            artists := pandoraResults.GetArtists()
            stations := pandoraResults.GetStations()
            fmt.Printf("✓ Found %d artists, %d stations\n", len(artists), len(stations))
            
            for i, artist := range artists[:min(2, len(artists))] {
                fmt.Printf("  Artist: %s\n", artist.GetDisplayName())
            }
        }
    }
    
    // Search Spotify (if account available)
    if spotifyAccount, ok := accounts["SPOTIFY"]; ok {
        fmt.Println("\n🎼 Spotify Results:")
        spotifyResults, err := soundtouch.SearchSpotifyContent(spotifyAccount, searchTerm)
        if err != nil {
            fmt.Printf("❌ Spotify search failed: %v\n", err)
        } else {
            songs := spotifyResults.GetSongs()
            fmt.Printf("✓ Found %d songs\n", len(songs))
            
            for i, song := range songs[:min(2, len(songs))] {
                fmt.Printf("  Song: %s\n", song.GetFullTitle())
            }
        }
    }
    
    fmt.Println("\n✓ Multi-service discovery complete!")
}
```

## Error Handling

### Graceful Error Handling

```go
func robustNavigation(soundtouch *client.Client) error {
    // Try multiple sources gracefully
    sources := []string{"TUNEIN", "SPOTIFY", "STORED_MUSIC"}
    
    for _, source := range sources {
        fmt.Printf("Trying %s...\n", source)
        
        response, err := soundtouch.Navigate(source, "", 1, 10)
        if err != nil {
            fmt.Printf("❌ %s failed: %v\n", source, err)
            continue
        }
        
        if response.IsEmpty() {
            fmt.Printf("⚠️  %s has no content\n", source)
            continue
        }
        
        fmt.Printf("✓ %s available with %d items\n", source, response.TotalItems)
        return nil
    }
    
    return fmt.Errorf("no sources available")
}
```

### Retry Logic

```go
func searchWithRetry(soundtouch *client.Client, maxRetries int) (*models.SearchStationResponse, error) {
    var lastErr error
    
    for attempt := 1; attempt <= maxRetries; attempt++ {
        fmt.Printf("Search attempt %d/%d...\n", attempt, maxRetries)
        
        results, err := soundtouch.SearchTuneInStations("classical")
        if err == nil {
            return results, nil
        }
        
        lastErr = err
        fmt.Printf("❌ Attempt %d failed: %v\n", attempt, err)
        
        if attempt < maxRetries {
            time.Sleep(time.Duration(attempt) * time.Second)
        }
    }
    
    return nil, fmt.Errorf("search failed after %d attempts: %w", maxRetries, lastErr)
}
```

### Validation and Safety

```go
func safeStationManagement(soundtouch *client.Client, pandoraAccount string) {
    // Always validate inputs
    if pandoraAccount == "" {
        log.Fatal("Pandora account required")
    }
    
    // Search safely
    results, err := soundtouch.SearchPandoraStations(pandoraAccount, "blues")
    if err != nil {
        log.Fatal(err)
    }
    
    if results.IsEmpty() {
        fmt.Println("No results found")
        return
    }
    
    // Check what we have before adding stations
    artists := results.GetArtists()
    if len(artists) == 0 {
        fmt.Println("No artists found to create stations from")
        return
    }
    
    // Get current stations to avoid duplicates
    currentStations, err := soundtouch.GetPandoraStations(pandoraAccount)
    if err != nil {
        log.Printf("Warning: Could not get current stations: %v", err)
    }
    
    // Create a map of existing station names
    existingStations := make(map[string]bool)
    for _, station := range currentStations.Items {
        existingStations[station.GetDisplayName()] = true
    }
    
    // Add stations only if they don't exist
    for _, artist := range artists[:min(2, len(artists))] {
        stationName := artist.Name + " Radio"
        
        if existingStations[stationName] {
            fmt.Printf("⚠️  Station already exists: %s\n", stationName)
            continue
        }
        
        fmt.Printf("Adding new station: %s\n", stationName)
        err := soundtouch.AddStation("PANDORA", pandoraAccount, artist.Token, stationName)
        if err != nil {
            log.Printf("❌ Failed to add %s: %v", stationName, err)
        } else {
            fmt.Printf("✓ Added: %s\n", stationName)
        }
    }
}
```

## Best Practices

### 1. Check Source Availability

```go
// Always check what sources are available first
sources, err := soundtouch.GetSources()
if err != nil {
    return err
}

// Check if TuneIn is ready
for _, source := range sources.SourceItem {
    if source.Source == "TUNEIN" && source.Status.IsReady() {
        // TuneIn is available
        break
    }
}
```

### 2. Use Pagination for Large Collections

```go
// For large libraries, use pagination
const batchSize = 50

func processLargeLibrary(soundtouch *client.Client, sourceAccount string) {
    startItem := 1
    
    for {
        batch, err := soundtouch.Navigate("STORED_MUSIC", sourceAccount, startItem, batchSize)
        if err != nil {
            log.Printf("Error at position %d: %v", startItem, err)
            break
        }
        
        if len(batch.Items) == 0 {
            break // No more items
        }
        
        // Process this batch
        processBatch(batch.Items)
        
        startItem += batchSize
        
        // Prevent infinite loops
        if startItem > batch.TotalItems {
            break
        }
    }
}
```

### 3. Handle Service-Specific Behavior

```go
func handleServiceDifferences(soundtouch *client.Client) {
    // TuneIn: Usually no account needed
    tuneInStations, err := soundtouch.SearchTuneInStations("news")
    if err == nil {
        fmt.Printf("TuneIn: %d stations\n", len(tuneInStations.GetStations()))
    }
    
    // Pandora: Requires user account
    pandoraResults, err := soundtouch.SearchPandoraStations("user_account", "rock")
    if err == nil {
        // Pandora returns artists you can create stations from
        artists := pandoraResults.GetArtists()
        fmt.Printf("Pandora: %d artists\n", len(artists))
    }
    
    // Spotify: Requires user account, returns tracks/playlists
    spotifyResults, err := soundtouch.SearchSpotifyContent("spotify_user", "pop")
    if err == nil {
        songs := spotifyResults.GetSongs()
        fmt.Printf("Spotify: %d songs\n", len(songs))
    }
}
```

### 4. Implement User-Friendly Interfaces

```go
func userFriendlySearch(soundtouch *client.Client, searchTerm string) {
    fmt.Printf("🔍 Searching for '%s'...\n", searchTerm)
    
    results, err := soundtouch.SearchTuneInStations(searchTerm)
    if err != nil {
        fmt.Printf("❌ Search failed: %v\n", err)
        return
    }
    
    if results.IsEmpty() {
        fmt.Printf("😞 No results found for '%s'\n", searchTerm)
        fmt.Println("💡 Try different search terms like:")
        fmt.Println("   - Genre names: jazz, rock, classical")
        fmt.Println("   - Artist names: Beatles, Mozart")
        fmt.Println("   - Station types: news, talk, music")
        return
    }
    
    stations := results.GetStations()
    fmt.Printf("🎵 Found %d stations:\n", len(stations))
    
    for i, station := range stations {
        fmt.Printf("%d. 📻 %s", i+1, station.GetDisplayName())
        if station.Description != "" {
            fmt.Printf("\n   %s", station.Description)
        }
        if station.GetArtworkURL() != "" {
            fmt.Printf("\n   🎨 %s", station.GetArtworkURL())
        }
        fmt.Println()
    }
}
```

### 5. Performance Considerations

```go
func efficientBrowsing(soundtouch *client.Client) {
    // Use reasonable page sizes
    const optimalPageSize = 25 // Good balance of network efficiency and memory usage
    
    // Cache frequently accessed data
    var cachedSources *models.Sources
    
    getSources := func() (*models.Sources, error) {
        if cachedSources == nil {
            var err error
            cachedSources, err = soundtouch.GetSources()
            return cachedSources, err
        }
        return cachedSources, nil
    }
    
    // Use the cached sources
    sources, err := getSources()
    if err != nil {
        return
    }
    
    // Process efficiently
    for _, source := range sources.SourceItem {
        if source.Status.IsReady() {
            // Only browse ready sources
            procesReadySource(soundtouch, source.Source, source.SourceAccount)
        }
    }
}
```

## API Reference

### Navigation Methods

| Method | Description | Parameters | Returns |
|--------|-------------|------------|---------|
| `Navigate()` | Browse content source | source, account, start, count | NavigateResponse |
| `NavigateWithMenu()` | Browse with menu/sort | source, account, menu, sort, start, count | NavigateResponse |
| `NavigateContainer()` | Browse into directory | source, account, start, count, container | NavigateResponse |
| `GetTuneInStations()` | Convenience for TuneIn | account | NavigateResponse |
| `GetPandoraStations()` | Convenience for Pandora | account | NavigateResponse |
| `GetStoredMusicLibrary()` | Convenience for stored music | account | NavigateResponse |

### Search Methods

| Method | Description | Parameters | Returns |
|--------|-------------|------------|---------|
| `SearchStation()` | Generic station search | source, account, term | SearchStationResponse |
| `SearchTuneInStations()` | Search TuneIn | term | SearchStationResponse |
| `SearchPandoraStations()` | Search Pandora | account, term | SearchStationResponse |
| `SearchSpotifyContent()` | Search Spotify | account, term | SearchStationResponse |

### Station Management Methods

| Method | Description | Parameters | Returns |
|--------|-------------|------------|---------|
| `AddStation()` | Add station (plays immediately) | source, account, token, name | error |
| `RemoveStation()` | Remove station from collection | contentItem | error |

### Response Helper Methods

#### NavigateResponse Methods

- `GetPlayableItems()` - Filter playable items
- `GetDirectories()` - Filter directories
- `GetTracks()` - Filter music tracks  
- `GetStations()` - Filter radio stations
- `IsEmpty()` - Check if response has no items

#### SearchStationResponse Methods

- `GetSongs()` - Filter song results
- `GetArtists()` - Filter artist results
- `GetStations()` - Filter station results
- `GetAllResults()` - Get all results combined
- `GetResultCount()` - Count total results
- `HasResults()` - Check if any results found
- `IsEmpty()` - Check if no results

#### SearchResult Methods

- `IsSong()` - Check if result is a song
- `IsArtist()` - Check if result is an artist
- `IsStation()` - Check if result is a station
- `GetDisplayName()` - Get formatted name
- `GetFullTitle()` - Get name with artist (for songs)
- `GetArtworkURL()` - Get artwork/logo URL

### Common Source Types

| Source | Description | Account Required | Search Support |
|--------|-------------|------------------|----------------|
| `TUNEIN` | Internet radio stations | No | Yes |
| `PANDORA` | Pandora music service | Yes | Yes |
| `SPOTIFY` | Spotify music service | Yes | Yes |
| `STORED_MUSIC` | Local/network music | Device account | No |
| `BLUETOOTH` | Bluetooth audio input | No | No |
| `AUX` | Auxiliary input | No | No |

## Troubleshooting

### Common Issues

**"Source not available"**
- Check if the service is configured on your SoundTouch device
- Verify account credentials are set up properly
- Use `GetSources()` to see what's actually available

**"No results found"**
- Try broader search terms
- Check if the service is working (try via SoundTouch app)
- Verify account has access to content

**"AddStation failed"** 
- Ensure the token is valid (from search results)
- Check that the service supports adding stations
- Verify account permissions

**Navigation timeouts**
- Large libraries may take time to browse
- Use smaller page sizes for better performance
- Implement timeout handling in your code

### Getting Help

For additional help:
- Check the SoundTouch device logs
- Test functionality via the official SoundTouch app
- Review network connectivity between client and device
- Examine the raw XML responses for debugging

---

*This guide covers the complete navigation and station management functionality. For preset management, see [PRESET-MANAGEMENT.md](reference/PRESET-MANAGEMENT.md).*
