package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestRecentsCommands(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput []string
		expectError    bool
	}{
		{
			name: "recents list command",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "recents", "list"},
			expectedOutput: []string{
				"Getting recently played content",
				"Recent Items Summary:",
				"Recent Items",
			},
		},
		{
			name: "recents filter by source",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "recents", "filter", "--source", "SPOTIFY"},
			expectedOutput: []string{
				"Getting filtered recent content",
				"filtered by source: SPOTIFY",
			},
		},
		{
			name: "recents latest command",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "recents", "latest"},
			expectedOutput: []string{
				"Getting most recent item",
				"Most Recent Item:",
			},
		},
		{
			name: "recents stats command",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "recents", "stats"},
			expectedOutput: []string{
				"Getting recent items statistics",
				"Recent Items Statistics",
			},
		},
		{
			name:        "recents missing host",
			args:        []string{"soundtouch-cli", "recents", "list"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip actual execution for now - these would need mock HTTP servers
			// This test structure shows how the CLI commands would be tested
			t.Skip("Integration test - requires mock HTTP server setup")

			// Example of how you would set up the test:
			// app := createTestApp()
			//
			// var buf bytes.Buffer
			// app.Writer = &buf
			// app.ErrWriter = &buf
			//
			// err := app.Run(tt.args)
			//
			// if tt.expectError {
			//     if err == nil {
			//         t.Error("expected error, got nil")
			//     }
			//     return
			// }
			//
			// if err != nil {
			//     t.Fatalf("unexpected error: %v", err)
			// }
			//
			// output := buf.String()
			// for _, expected := range tt.expectedOutput {
			//     if !strings.Contains(output, expected) {
			//         t.Errorf("expected output to contain %q, got:\n%s", expected, output)
			//     }
			// }
		})
	}
}

func TestPrintRecentItem(t *testing.T) {
	tests := []struct {
		name     string
		item     *models.RecentsResponseItem
		detailed bool
		expected []string
	}{
		{
			name: "basic track item",
			item: &models.RecentsResponseItem{
				DeviceID: "device1",
				UTCTime:  1701200000,
				ContentItem: &models.ContentItem{
					Source:   "SPOTIFY",
					Type:     "track",
					ItemName: "Test Song",
				},
			},
			detailed: false,
			expected: []string{
				"🎵 Test Song",
				"Source: Spotify",
				"Type: track",
			},
		},
		{
			name: "detailed station item",
			item: &models.RecentsResponseItem{
				DeviceID: "device1",
				UTCTime:  1701200000,
				ID:       "station123",
				ContentItem: &models.ContentItem{
					Source:        "TUNEIN",
					Type:          "stationurl",
					ItemName:      "Rock FM",
					Location:      "tunein:station:s12345",
					SourceAccount: "tunein_account",
					IsPresetable:  true,
				},
			},
			detailed: true,
			expected: []string{
				"📻 Rock FM",
				"Source: TuneIn Radio",
				"ID: station123",
				"Can be saved as preset",
				"Location: tunein:station:s12345",
				"Classification: Streaming",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printRecentItem(1, tt.item, tt.detailed)

			// Restore stdout and read output
			w.Close()

			os.Stdout = oldStdout

			var buf bytes.Buffer

			_, err := buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("failed to read output: %v", err)
			}

			output := buf.String()

			// Check expected strings are present
			for _, expected := range tt.expected {
				if !bytes.Contains(buf.Bytes(), []byte(expected)) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

func TestGetContentTypeIcon(t *testing.T) {
	tests := []struct {
		name     string
		item     *models.RecentsResponseItem
		expected string
	}{
		{
			name: "track item",
			item: &models.RecentsResponseItem{
				ContentItem: &models.ContentItem{Type: "track"},
			},
			expected: "🎵",
		},
		{
			name: "station item",
			item: &models.RecentsResponseItem{
				ContentItem: &models.ContentItem{Type: "stationurl"},
			},
			expected: "📻",
		},
		{
			name: "playlist item",
			item: &models.RecentsResponseItem{
				ContentItem: &models.ContentItem{Type: "playlist"},
			},
			expected: "📋",
		},
		{
			name: "album item",
			item: &models.RecentsResponseItem{
				ContentItem: &models.ContentItem{Type: "album"},
			},
			expected: "💿",
		},
		{
			name: "container item",
			item: &models.RecentsResponseItem{
				ContentItem: &models.ContentItem{Type: "container"},
			},
			expected: "📁",
		},
		{
			name: "unknown type",
			item: &models.RecentsResponseItem{
				ContentItem: &models.ContentItem{Type: "unknown"},
			},
			expected: "🎶",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getContentTypeIcon(tt.item)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestFormatSourceForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{"Spotify", "SPOTIFY", "Spotify"},
		{"Local Music", "LOCAL_MUSIC", "Local Music"},
		{"Stored Music", "STORED_MUSIC", "Stored Music"},
		{"TuneIn", "TUNEIN", "TuneIn Radio"},
		{"Pandora", "PANDORA", "Pandora"},
		{"Amazon", "AMAZON", "Amazon Music"},
		{"Deezer", "DEEZER", "Deezer"},
		{"iHeart", "IHEART", "iHeartRadio"},
		{"Bluetooth", "BLUETOOTH", "Bluetooth"},
		{"AUX", "AUX", "AUX Input"},
		{"AirPlay", "AIRPLAY", "AirPlay"},
		{"Unknown", "UNKNOWN_SOURCE", "UNKNOWN_SOURCE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSourceForDisplay(tt.source)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		expected  string
	}{
		{
			name:      "short string",
			input:     "hello",
			maxLength: 10,
			expected:  "hello",
		},
		{
			name:      "exact length",
			input:     "hello",
			maxLength: 5,
			expected:  "hello",
		},
		{
			name:      "long string",
			input:     "this is a very long string that needs truncation",
			maxLength: 20,
			expected:  "this is a very lo...",
		},
		{
			name:      "very short max length",
			input:     "hello world",
			maxLength: 3,
			expected:  "...",
		},
		{
			name:      "zero length",
			input:     "hello",
			maxLength: 0,
			expected:  "...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLength)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test helper functions that would be used in full integration tests
func createTestRecentsResponse() *models.RecentsResponse {
	return &models.RecentsResponse{
		Items: []models.RecentsResponseItem{
			{
				DeviceID: "1004567890AA",
				UTCTime:  1701300000,
				ID:       "spotify1",
				ContentItem: &models.ContentItem{
					Source:        "SPOTIFY",
					Type:          "track",
					Location:      "spotify:track:4iV5W9uYEdYUVa79Axb7Rh",
					SourceAccount: "spotify_user",
					IsPresetable:  true,
					ItemName:      "Shape of You - Ed Sheeran",
					ContainerArt:  "https://i.scdn.co/image/ab67616d0000b273ba5db46f4b838ef6027e6f96",
				},
			},
			{
				DeviceID: "1004567890AA",
				UTCTime:  1701200000,
				ID:       "local1",
				ContentItem: &models.ContentItem{
					Source:       "LOCAL_MUSIC",
					Type:         "track",
					Location:     "/music/local_song.mp3",
					IsPresetable: false,
					ItemName:     "Local Song - Local Artist",
				},
			},
			{
				DeviceID: "1004567890AA",
				UTCTime:  1701100000,
				ID:       "tunein1",
				ContentItem: &models.ContentItem{
					Source:        "TUNEIN",
					Type:          "stationurl",
					Location:      "tunein:station:s24939",
					SourceAccount: "tunein",
					IsPresetable:  true,
					ItemName:      "BBC Radio 1",
				},
			},
		},
	}
}

func TestCreateTestRecentsResponse(t *testing.T) {
	response := createTestRecentsResponse()

	if response == nil {
		t.Fatal("expected response, got nil")
	}

	if response.GetItemCount() != 3 {
		t.Errorf("expected 3 items, got %d", response.GetItemCount())
	}

	if response.IsEmpty() {
		t.Error("expected response not to be empty")
	}

	// Test filtering
	spotifyItems := response.GetSpotifyItems()
	if len(spotifyItems) != 1 {
		t.Errorf("expected 1 Spotify item, got %d", len(spotifyItems))
	}

	localItems := response.GetLocalMusicItems()
	if len(localItems) != 1 {
		t.Errorf("expected 1 local music item, got %d", len(localItems))
	}

	tuneInItems := response.GetTuneInItems()
	if len(tuneInItems) != 1 {
		t.Errorf("expected 1 TuneIn item, got %d", len(tuneInItems))
	}

	tracks := response.GetTracks()
	if len(tracks) != 2 {
		t.Errorf("expected 2 tracks, got %d", len(tracks))
	}

	stations := response.GetStations()
	if len(stations) != 1 {
		t.Errorf("expected 1 station, got %d", len(stations))
	}

	presetableItems := response.GetPresetableItems()
	if len(presetableItems) != 2 {
		t.Errorf("expected 2 presetable items, got %d", len(presetableItems))
	}
}
