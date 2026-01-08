package models

import (
	"encoding/xml"
	"testing"
	"time"
)

func TestPlayStatus_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlInput string
		expected PlayStatus
	}{
		{
			name:     "playing state",
			xmlInput: `<playStatus>PLAY_STATE</playStatus>`,
			expected: PlayStatusPlaying,
		},
		{
			name:     "paused state",
			xmlInput: `<playStatus>PAUSE_STATE</playStatus>`,
			expected: PlayStatusPaused,
		},
		{
			name:     "stopped state",
			xmlInput: `<playStatus>STOP_STATE</playStatus>`,
			expected: PlayStatusStopped,
		},
		{
			name:     "buffering state",
			xmlInput: `<playStatus>BUFFERING_STATE</playStatus>`,
			expected: PlayStatusBuffering,
		},
		{
			name:     "unknown state defaults to stopped",
			xmlInput: `<playStatus>UNKNOWN_STATE</playStatus>`,
			expected: PlayStatusStopped,
		},
		{
			name:     "empty state defaults to stopped",
			xmlInput: `<playStatus></playStatus>`,
			expected: PlayStatusStopped,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status PlayStatus

			err := xml.Unmarshal([]byte(tt.xmlInput), &status)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if status != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, status)
			}
		})
	}
}

func TestPlayStatus_Methods(t *testing.T) {
	tests := []struct {
		status    PlayStatus
		isPlaying bool
		isPaused  bool
		isStopped bool
		toString  string
	}{
		{PlayStatusPlaying, true, false, false, "Playing"},
		{PlayStatusPaused, false, true, false, "Paused"},
		{PlayStatusStopped, false, false, true, "Stopped"},
		{PlayStatusBuffering, false, false, false, "Buffering"},
		{PlayStatusInvalidPlay, false, false, false, "Invalid"},
		{PlayStatus("UNKNOWN"), false, false, false, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.toString, func(t *testing.T) {
			if tt.status.IsPlaying() != tt.isPlaying {
				t.Errorf("IsPlaying() = %v, want %v", tt.status.IsPlaying(), tt.isPlaying)
			}
			if tt.status.IsPaused() != tt.isPaused {
				t.Errorf("IsPaused() = %v, want %v", tt.status.IsPaused(), tt.isPaused)
			}
			if tt.status.IsStopped() != tt.isStopped {
				t.Errorf("IsStopped() = %v, want %v", tt.status.IsStopped(), tt.isStopped)
			}
			if tt.status.String() != tt.toString {
				t.Errorf("String() = %v, want %v", tt.status.String(), tt.toString)
			}
		})
	}
}

func TestShuffleSetting_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlInput string
		expected ShuffleSetting
	}{
		{
			name:     "shuffle on",
			xmlInput: `<shuffleSetting>SHUFFLE_ON</shuffleSetting>`,
			expected: ShuffleOn,
		},
		{
			name:     "shuffle off",
			xmlInput: `<shuffleSetting>SHUFFLE_OFF</shuffleSetting>`,
			expected: ShuffleOff,
		},
		{
			name:     "unknown state defaults to off",
			xmlInput: `<shuffleSetting>UNKNOWN</shuffleSetting>`,
			expected: ShuffleOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var setting ShuffleSetting

			err := xml.Unmarshal([]byte(tt.xmlInput), &setting)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if setting != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, setting)
			}
		})
	}
}

func TestRepeatSetting_UnmarshalXML(t *testing.T) {
	tests := []struct {
		name     string
		xmlInput string
		expected RepeatSetting
	}{
		{
			name:     "repeat off",
			xmlInput: `<repeatSetting>REPEAT_OFF</repeatSetting>`,
			expected: RepeatOff,
		},
		{
			name:     "repeat one",
			xmlInput: `<repeatSetting>REPEAT_ONE</repeatSetting>`,
			expected: RepeatOne,
		},
		{
			name:     "repeat all",
			xmlInput: `<repeatSetting>REPEAT_ALL</repeatSetting>`,
			expected: RepeatAll,
		},
		{
			name:     "unknown state defaults to off",
			xmlInput: `<repeatSetting>UNKNOWN</repeatSetting>`,
			expected: RepeatOff,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var setting RepeatSetting

			err := xml.Unmarshal([]byte(tt.xmlInput), &setting)
			if err != nil {
				t.Fatalf("Failed to unmarshal XML: %v", err)
			}

			if setting != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, setting)
			}
		})
	}
}

func TestNowPlaying_UnmarshalXML(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<nowPlaying deviceID="A81B6A536A98" source="SPOTIFY" sourceAccount="user@example.com">
    <ContentItem source="SPOTIFY" type="tracklisturl" location="/playbook/container/abc123" sourceAccount="user@example.com" isPresetable="true">
        <itemName>SYML</itemName>
        <containerArt>https://i.scdn.co/image/ab67616d0000b273ca6f00df62ef197fdc8af79c</containerArt>
    </ContentItem>
    <track>In Between Breaths - Paris Unplugged</track>
    <artist>SYML</artist>
    <album>Paris Unplugged</album>
    <art artImageStatus="IMAGE_PRESENT">https://i.scdn.co/image/ab67616d0000b273ca6f00df62ef197fdc8af79c</art>
    <time total="210">36</time>
    <skipEnabled />
    <favoriteEnabled />
    <playStatus>PLAY_STATE</playStatus>
    <shuffleSetting>SHUFFLE_OFF</shuffleSetting>
    <repeatSetting>REPEAT_OFF</repeatSetting>
    <skipPreviousEnabled />
    <seekSupported value="true" />
    <streamType>TRACK_ONDEMAND</streamType>
    <trackID>spotify:track:3LX0dk3YT8cUgp7XxUJgTB</trackID>
</nowPlaying>`

	var nowPlaying NowPlaying
	err := xml.Unmarshal([]byte(xmlData), &nowPlaying)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	// Test basic fields
	if nowPlaying.DeviceID != "A81B6A536A98" {
		t.Errorf("Expected DeviceID 'A81B6A536A98', got '%s'", nowPlaying.DeviceID)
	}

	if nowPlaying.Source != "SPOTIFY" {
		t.Errorf("Expected Source 'SPOTIFY', got '%s'", nowPlaying.Source)
	}

	if nowPlaying.Track != "In Between Breaths - Paris Unplugged" {
		t.Errorf("Expected Track 'In Between Breaths - Paris Unplugged', got '%s'", nowPlaying.Track)
	}

	if nowPlaying.Artist != "SYML" {
		t.Errorf("Expected Artist 'SYML', got '%s'", nowPlaying.Artist)
	}

	if nowPlaying.Album != "Paris Unplugged" {
		t.Errorf("Expected Album 'Paris Unplugged', got '%s'", nowPlaying.Album)
	}

	if nowPlaying.SourceAccount != "user@example.com" {
		t.Errorf("Expected SourceAccount 'user@example.com', got '%s'", nowPlaying.SourceAccount)
	}

	if nowPlaying.PlayStatus != PlayStatusPlaying {
		t.Errorf("Expected PlayStatus Playing, got %v", nowPlaying.PlayStatus)
	}

	if nowPlaying.ShuffleSetting != ShuffleOff {
		t.Errorf("Expected ShuffleSetting Off, got %v", nowPlaying.ShuffleSetting)
	}

	if nowPlaying.RepeatSetting != RepeatOff {
		t.Errorf("Expected RepeatSetting Off, got %v", nowPlaying.RepeatSetting)
	}

	// Test ContentItem
	if nowPlaying.ContentItem == nil {
		t.Fatal("Expected ContentItem to be present")
	}

	if nowPlaying.ContentItem.Source != "SPOTIFY" {
		t.Errorf("Expected ContentItem.Source 'SPOTIFY', got '%s'", nowPlaying.ContentItem.Source)
	}

	if nowPlaying.ContentItem.ItemName != "SYML" {
		t.Errorf("Expected ContentItem.ItemName 'SYML', got '%s'", nowPlaying.ContentItem.ItemName)
	}

	// Test Art
	if nowPlaying.Art == nil {
		t.Fatal("Expected Art to be present")
	}

	if nowPlaying.Art.ArtImageStatus != "IMAGE_PRESENT" {
		t.Errorf("Expected Art.ArtImageStatus 'IMAGE_PRESENT', got '%s'", nowPlaying.Art.ArtImageStatus)
	}

	// Test Time
	if nowPlaying.Time == nil {
		t.Fatal("Expected Time to be present")
	}

	if nowPlaying.Time.Position != 36 {
		t.Errorf("Expected Time.Position 36, got %d", nowPlaying.Time.Position)
	}

	if nowPlaying.Time.Total != 210 {
		t.Errorf("Expected Time.Total 210, got %d", nowPlaying.Time.Total)
	}

	// Test TrackID
	if nowPlaying.TrackID != "spotify:track:3LX0dk3YT8cUgp7XxUJgTB" {
		t.Errorf("Expected TrackID 'spotify:track:3LX0dk3YT8cUgp7XxUJgTB', got '%s'", nowPlaying.TrackID)
	}

	// Test Capabilities
	if nowPlaying.SkipEnabled == nil {
		t.Error("Expected SkipEnabled to be present")
	}

	if nowPlaying.FavoriteEnabled == nil {
		t.Error("Expected FavoriteEnabled to be present")
	}

	if nowPlaying.SkipPreviousEnabled == nil {
		t.Error("Expected SkipPreviousEnabled to be present")
	}

	if nowPlaying.SeekSupported == nil {
		t.Error("Expected SeekSupported to be present")
	} else if !nowPlaying.SeekSupported.Value {
		t.Error("Expected SeekSupported.Value to be true")
	}
}

func TestNowPlaying_RadioStation(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<nowPlaying deviceID="A81B6A536A98" source="TUNEIN">
    <stationName>Classic Rock 101.5</stationName>
    <description>The Best Classic Rock Hits</description>
    <stationLocation>New York, NY</stationLocation>
    <art artImageStatus="IMAGE_PRESENT">https://cdn-radiotime-logos.tunein.com/s123456q.png</art>
    <playStatus>PLAY_STATE</playStatus>
</nowPlaying>`

	var nowPlaying NowPlaying
	err := xml.Unmarshal([]byte(xmlData), &nowPlaying)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	if !nowPlaying.IsRadio() {
		t.Error("Expected IsRadio() to return true for TUNEIN source")
	}

	if nowPlaying.StationName != "Classic Rock 101.5" {
		t.Errorf("Expected StationName 'Classic Rock 101.5', got '%s'", nowPlaying.StationName)
	}

	if nowPlaying.Description != "The Best Classic Rock Hits" {
		t.Errorf("Expected Description 'The Best Classic Rock Hits', got '%s'", nowPlaying.Description)
	}

	if nowPlaying.StationLocation != "New York, NY" {
		t.Errorf("Expected StationLocation 'New York, NY', got '%s'", nowPlaying.StationLocation)
	}
}

func TestNowPlaying_EmptyState(t *testing.T) {
	xmlData := `<?xml version="1.0" encoding="UTF-8" ?>
<nowPlaying deviceID="A81B6A536A98" source="">
    <playStatus>STOP_STATE</playStatus>
</nowPlaying>`

	var nowPlaying NowPlaying
	err := xml.Unmarshal([]byte(xmlData), &nowPlaying)
	if err != nil {
		t.Fatalf("Failed to unmarshal XML: %v", err)
	}

	if !nowPlaying.IsEmpty() {
		t.Error("Expected IsEmpty() to return true for empty state")
	}

	if nowPlaying.HasTrackInfo() {
		t.Error("Expected HasTrackInfo() to return false for empty state")
	}

	if nowPlaying.PlayStatus != PlayStatusStopped {
		t.Errorf("Expected PlayStatus Stopped, got %v", nowPlaying.PlayStatus)
	}
}

func TestNowPlaying_HelperMethods(t *testing.T) {
	// Test with track info
	nowPlaying := NowPlaying{
		Track:    "Test Track",
		Artist:   "Test Artist",
		Album:    "Test Album",
		Position: &Position{Position: 125},
	}

	if nowPlaying.GetDisplayTitle() != "Test Track" {
		t.Errorf("Expected GetDisplayTitle() 'Test Track', got '%s'", nowPlaying.GetDisplayTitle())
	}

	if nowPlaying.GetDisplayArtist() != "Test Artist" {
		t.Errorf("Expected GetDisplayArtist() 'Test Artist', got '%s'", nowPlaying.GetDisplayArtist())
	}

	if !nowPlaying.HasTrackInfo() {
		t.Error("Expected HasTrackInfo() to return true")
	}

	if nowPlaying.IsEmpty() {
		t.Error("Expected IsEmpty() to return false")
	}

	// Test position formatting
	expectedPosition := "2:05"
	if nowPlaying.FormatPosition() != expectedPosition {
		t.Errorf("Expected FormatPosition() '%s', got '%s'", expectedPosition, nowPlaying.FormatPosition())
	}

	// Test position duration
	expectedDuration := 125 * time.Second
	if nowPlaying.GetPositionDuration() != expectedDuration {
		t.Errorf("Expected GetPositionDuration() %v, got %v", expectedDuration, nowPlaying.GetPositionDuration())
	}

	// Test with station name but no track
	radioNowPlaying := NowPlaying{
		StationName: "Test Station",
		Source:      "TUNEIN",
	}

	if radioNowPlaying.GetDisplayTitle() != "Test Station" {
		t.Errorf("Expected GetDisplayTitle() 'Test Station', got '%s'", radioNowPlaying.GetDisplayTitle())
	}

	if !radioNowPlaying.IsRadio() {
		t.Error("Expected IsRadio() to return true for TUNEIN source")
	}

	// Test with ContentItem fallback
	contentNowPlaying := NowPlaying{
		ContentItem: &ContentItem{ItemName: "Content Item Name"},
	}

	if contentNowPlaying.GetDisplayTitle() != "Content Item Name" {
		t.Errorf("Expected GetDisplayTitle() 'Content Item Name', got '%s'", contentNowPlaying.GetDisplayTitle())
	}

	// Test artwork URL
	artNowPlaying := NowPlaying{
		Art: &Art{URL: "https://example.com/art.jpg"},
	}

	if artNowPlaying.GetArtworkURL() != "https://example.com/art.jpg" {
		t.Errorf("Expected GetArtworkURL() 'https://example.com/art.jpg', got '%s'", artNowPlaying.GetArtworkURL())
	}

	// Test ContentItem artwork fallback
	contentArtNowPlaying := NowPlaying{
		ContentItem: &ContentItem{ContainerArt: "https://example.com/container.jpg"},
	}

	if contentArtNowPlaying.GetArtworkURL() != "https://example.com/container.jpg" {
		t.Errorf("Expected GetArtworkURL() 'https://example.com/container.jpg', got '%s'", contentArtNowPlaying.GetArtworkURL())
	}
}

func TestNowPlaying_EdgeCases(t *testing.T) {
	// Test with nil time and position
	nowPlaying := NowPlaying{}

	if nowPlaying.FormatPosition() != "" {
		t.Errorf("Expected FormatPosition() to return empty string for nil time/position, got '%s'", nowPlaying.FormatPosition())
	}

	if nowPlaying.GetPositionDuration() != 0 {
		t.Errorf("Expected GetPositionDuration() to return 0 for nil time/position, got %v", nowPlaying.GetPositionDuration())
	}

	if nowPlaying.GetTotalDuration() != 0 {
		t.Errorf("Expected GetTotalDuration() to return 0 for nil time, got %v", nowPlaying.GetTotalDuration())
	}

	// Test fallback to "Unknown" title
	emptyNowPlaying := NowPlaying{}
	if emptyNowPlaying.GetDisplayTitle() != "Unknown" {
		t.Errorf("Expected GetDisplayTitle() 'Unknown' for empty NowPlaying, got '%s'", emptyNowPlaying.GetDisplayTitle())
	}

	// Test description fallback for artist
	descNowPlaying := NowPlaying{
		Description: "Test Description",
	}

	if descNowPlaying.GetDisplayArtist() != "Test Description" {
		t.Errorf("Expected GetDisplayArtist() 'Test Description', got '%s'", descNowPlaying.GetDisplayArtist())
	}
}

func TestNowPlaying_NewFields(t *testing.T) {
	// Test Time field
	nowPlaying := NowPlaying{
		Time: &Time{
			Total:    180,
			Position: 65,
		},
	}

	if !nowPlaying.HasTimeInfo() {
		t.Error("Expected HasTimeInfo() to return true for Time field")
	}

	expectedDuration := "1:05 / 3:00"
	if nowPlaying.FormatDuration() != expectedDuration {
		t.Errorf("Expected FormatDuration() '%s', got '%s'", expectedDuration, nowPlaying.FormatDuration())
	}

	// Test capabilities
	capableNowPlaying := NowPlaying{
		SkipEnabled:         &SkipEnabled{},
		FavoriteEnabled:     &FavoriteEnabled{},
		SkipPreviousEnabled: &SkipPreviousEnabled{},
		SeekSupported:       &SeekSupported{Value: true},
	}

	if !capableNowPlaying.CanSkip() {
		t.Error("Expected CanSkip() to return true")
	}

	if !capableNowPlaying.CanFavorite() {
		t.Error("Expected CanFavorite() to return true")
	}

	if !capableNowPlaying.CanSkipPrevious() {
		t.Error("Expected CanSkipPrevious() to return true")
	}

	if !capableNowPlaying.IsSeekSupported() {
		t.Error("Expected IsSeekSupported() to return true")
	}

	// Test seek not supported
	noSeekNowPlaying := NowPlaying{
		SeekSupported: &SeekSupported{Value: false},
	}

	if noSeekNowPlaying.IsSeekSupported() {
		t.Error("Expected IsSeekSupported() to return false")
	}
}
