package client

import (
	"os"
	"testing"
	"time"
)

func TestClient_GetRecents_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host := os.Getenv("SOUNDTOUCH_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_HOST not set, skipping integration test")
	}

	config := &Config{
		Host:    host,
		Timeout: 10 * time.Second,
	}
	client := NewClient(config)

	t.Run("get recents", func(t *testing.T) {
		response, err := client.GetRecents()
		if err != nil {
			t.Fatalf("failed to get recents: %v", err)
		}

		if response == nil {
			t.Fatal("expected response, got nil")
		}

		t.Logf("Recent items count: %d", response.GetItemCount())

		if response.IsEmpty() {
			t.Log("No recent items found - this is normal if device hasn't played anything recently")
			return
		}

		// Test basic functionality
		t.Logf("Recent items found: %d", response.GetItemCount())

		// Get most recent item
		mostRecent := response.GetMostRecent()
		if mostRecent != nil {
			t.Logf("Most recent item: %s (Source: %s, Time: %d)",
				mostRecent.GetDisplayName(),
				mostRecent.GetSource(),
				mostRecent.GetUTCTime())

			if mostRecent.HasArtwork() {
				t.Logf("  Has artwork: %s", mostRecent.GetArtwork())
			}

			if mostRecent.IsPresetable() {
				t.Log("  Can be saved as preset")
			}

			// Test content type detection
			if mostRecent.IsTrack() {
				t.Log("  Content type: Track")
			} else if mostRecent.IsStation() {
				t.Log("  Content type: Radio Station")
			} else if mostRecent.IsPlaylist() {
				t.Log("  Content type: Playlist")
			} else if mostRecent.IsAlbum() {
				t.Log("  Content type: Album")
			} else if mostRecent.IsContainer() {
				t.Log("  Content type: Container")
			}

			// Test source type detection
			if mostRecent.IsSpotifyContent() {
				t.Log("  Source type: Spotify")
			} else if mostRecent.IsLocalContent() {
				t.Log("  Source type: Local")
			} else if mostRecent.IsStreamingContent() {
				t.Log("  Source type: Streaming service")
			}
		}

		// Test filtering methods
		spotifyItems := response.GetSpotifyItems()
		if len(spotifyItems) > 0 {
			t.Logf("Spotify items: %d", len(spotifyItems))

			for i, item := range spotifyItems {
				if i < 3 { // Show first 3
					t.Logf("  - %s", item.GetDisplayName())
				}
			}
		}

		localItems := response.GetLocalMusicItems()
		if len(localItems) > 0 {
			t.Logf("Local music items: %d", len(localItems))
		}

		storedItems := response.GetStoredMusicItems()
		if len(storedItems) > 0 {
			t.Logf("Stored music items: %d", len(storedItems))
		}

		tuneInItems := response.GetTuneInItems()
		if len(tuneInItems) > 0 {
			t.Logf("TuneIn items: %d", len(tuneInItems))
		}

		pandoraItems := response.GetPandoraItems()
		if len(pandoraItems) > 0 {
			t.Logf("Pandora items: %d", len(pandoraItems))
		}

		// Test content type filters
		tracks := response.GetTracks()
		if len(tracks) > 0 {
			t.Logf("Track items: %d", len(tracks))
		}

		stations := response.GetStations()
		if len(stations) > 0 {
			t.Logf("Station items: %d", len(stations))
		}

		playlistsAndAlbums := response.GetPlaylistsAndAlbums()
		if len(playlistsAndAlbums) > 0 {
			t.Logf("Playlist/Album items: %d", len(playlistsAndAlbums))
		}

		presetableItems := response.GetPresetableItems()
		if len(presetableItems) > 0 {
			t.Logf("Presetable items: %d", len(presetableItems))
		}

		// Show all items with details
		t.Log("\nAll recent items:")

		for i, item := range response.Items {
			if i >= 10 { // Limit to first 10 items to avoid spam
				t.Logf("  ... and %d more items", len(response.Items)-i)
				break
			}

			displayName := item.GetDisplayName()
			source := item.GetSource()
			contentType := item.GetContentType()
			utcTime := item.GetUTCTime()

			timeStr := ""

			if utcTime > 0 {
				playTime := time.Unix(utcTime, 0)
				timeStr = playTime.Format("2006-01-02 15:04:05")
			}

			t.Logf("  %d. %s (%s/%s) - %s", i+1, displayName, source, contentType, timeStr)

			if item.HasID() {
				t.Logf("     ID: %s", item.GetID())
			}
		}
	})
}

func TestClient_GetRecents_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping performance test")
	}

	host := os.Getenv("SOUNDTOUCH_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_HOST not set, skipping integration test")
	}

	config := &Config{
		Host:    host,
		Timeout: 5 * time.Second,
	}
	client := NewClient(config)

	// Measure response time
	start := time.Now()
	response, err := client.GetRecents()
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("failed to get recents: %v", err)
	}

	t.Logf("GetRecents() took %v", duration)

	if duration > 2*time.Second {
		t.Logf("Warning: GetRecents() took longer than expected: %v", duration)
	}

	if response != nil {
		t.Logf("Retrieved %d recent items", response.GetItemCount())
	}
}

func TestClient_GetRecents_ErrorConditions(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Test with invalid host
	t.Run("invalid host", func(t *testing.T) {
		config := &Config{
			Host:    "192.168.255.255", // Non-existent IP
			Timeout: 2 * time.Second,   // Short timeout
		}
		client := NewClient(config)

		response, err := client.GetRecents()
		if err == nil {
			t.Error("expected error for invalid host, got nil")
		}

		if response != nil {
			t.Error("expected nil response for invalid host, got non-nil")
		}

		t.Logf("Expected error for invalid host: %v", err)
	})

	// Test with very short timeout
	t.Run("timeout", func(t *testing.T) {
		host := os.Getenv("SOUNDTOUCH_HOST")
		if host == "" {
			t.Skip("SOUNDTOUCH_HOST not set")
		}

		config := &Config{
			Host:    host,
			Timeout: 1 * time.Nanosecond, // Impossibly short timeout
		}
		client := NewClient(config)

		response, err := client.GetRecents()
		if err == nil {
			t.Log("Warning: expected timeout error, but request succeeded")
		}

		if response != nil && err != nil {
			t.Error("got both response and error")
		}

		t.Logf("Timeout test result - error: %v, response nil: %t", err, response == nil)
	})
}

// ExampleClient_GetRecents demonstrates how to use the GetRecents method
func ExampleClient_GetRecents() {
	config := &Config{
		Host: "192.0.2.100",
		Port: 8090,
	}
	client := NewClient(config)

	// Get recent items
	response, err := client.GetRecents()
	if err != nil {
		panic(err)
	}

	if response.IsEmpty() {
		println("No recent items found")
		return
	}

	// Show most recent item
	mostRecent := response.GetMostRecent()
	if mostRecent != nil {
		println("Most recent:", mostRecent.GetDisplayName())
		println("Source:", mostRecent.GetSource())

		if mostRecent.IsPresetable() {
			println("Can be saved as preset")
		}
	}

	// Show Spotify items
	spotifyItems := response.GetSpotifyItems()
	if len(spotifyItems) > 0 {
		println("Recent Spotify tracks:")

		for _, item := range spotifyItems {
			println("-", item.GetDisplayName())
		}
	}

	// Show only tracks (no stations or playlists)
	tracks := response.GetTracks()
	println("Total tracks in recent items:", len(tracks))
}

// ExampleRecentsResponse_filtering demonstrates filtering recent items
func ExampleRecentsResponse_filtering() {
	config := &Config{
		Host: "192.0.2.100",
		Port: 8090,
	}
	client := NewClient(config)

	response, err := client.GetRecents()
	if err != nil {
		panic(err)
	}

	// Filter by source
	println("Spotify items:", len(response.GetSpotifyItems()))
	println("Local music items:", len(response.GetLocalMusicItems()))
	println("TuneIn items:", len(response.GetTuneInItems()))

	// Filter by type
	println("Tracks:", len(response.GetTracks()))
	println("Stations:", len(response.GetStations()))
	println("Playlists/Albums:", len(response.GetPlaylistsAndAlbums()))

	// Filter by capability
	println("Presetable items:", len(response.GetPresetableItems()))

	// Get items from streaming services only
	streamingItems := 0

	for _, item := range response.Items {
		if item.IsStreamingContent() {
			streamingItems++
		}
	}

	println("Streaming service items:", streamingItems)
}
