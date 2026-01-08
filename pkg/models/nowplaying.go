package models

import (
	"encoding/xml"
	"fmt"
	"time"
)

// NowPlaying represents the current playback information from /now_playing endpoint
type NowPlaying struct {
	XMLName             xml.Name             `xml:"nowPlaying"`
	DeviceID            string               `xml:"deviceID,attr"`
	Source              string               `xml:"source,attr"`
	SourceAccount       string               `xml:"sourceAccount,attr,omitempty"`
	ContentItem         *ContentItem         `xml:"ContentItem,omitempty"`
	Track               string               `xml:"track,omitempty"`
	Artist              string               `xml:"artist,omitempty"`
	Album               string               `xml:"album,omitempty"`
	StationName         string               `xml:"stationName,omitempty"`
	Art                 *Art                 `xml:"art,omitempty"`
	Time                *Time                `xml:"time,omitempty"`
	SkipEnabled         *SkipEnabled         `xml:"skipEnabled,omitempty"`
	FavoriteEnabled     *FavoriteEnabled     `xml:"favoriteEnabled,omitempty"`
	PlayStatus          PlayStatus           `xml:"playStatus,omitempty"`
	ShuffleSetting      ShuffleSetting       `xml:"shuffleSetting,omitempty"`
	RepeatSetting       RepeatSetting        `xml:"repeatSetting,omitempty"`
	SkipPreviousEnabled *SkipPreviousEnabled `xml:"skipPreviousEnabled,omitempty"`
	SeekSupported       *SeekSupported       `xml:"seekSupported,omitempty"`
	StreamType          string               `xml:"streamType,omitempty"`
	TrackID             string               `xml:"trackID,omitempty"`
	Position            *Position            `xml:"position,omitempty"`
	Description         string               `xml:"description,omitempty"`
	StationLocation     string               `xml:"stationLocation,omitempty"`
}

// ContentItem represents metadata about the currently playing content
type ContentItem struct {
	Source        string `xml:"source,attr"`
	Type          string `xml:"type,attr"`
	Location      string `xml:"location,attr"`
	SourceAccount string `xml:"sourceAccount,attr"`
	IsPresetable  bool   `xml:"isPresetable,attr"`
	ItemName      string `xml:"itemName,omitempty"`
	ContainerArt  string `xml:"containerArt,omitempty"`
}

// Art represents album artwork information
type Art struct {
	ArtImageStatus string `xml:"artImageStatus,attr"`
	URL            string `xml:",chardata"`
}

// Time represents playback time information with total duration and current position
type Time struct {
	Total    int `xml:"total,attr"` // Total duration in seconds
	Position int `xml:",chardata"`  // Current position in seconds
}

// SkipEnabled indicates if skip functionality is enabled
type SkipEnabled struct{}

// FavoriteEnabled indicates if favorite functionality is enabled
type FavoriteEnabled struct{}

// SkipPreviousEnabled indicates if skip previous functionality is enabled
type SkipPreviousEnabled struct{}

// SeekSupported indicates if seek functionality is supported
type SeekSupported struct {
	Value bool `xml:"value,attr"`
}

// Position represents playback position information (legacy field)
type Position struct {
	Position int `xml:",chardata"` // Position in seconds
}

// PlayStatus represents the current playback state
type PlayStatus string

const (
	PlayStatusPlaying     PlayStatus = "PLAY_STATE"
	PlayStatusPaused      PlayStatus = "PAUSE_STATE"
	PlayStatusStopped     PlayStatus = "STOP_STATE"
	PlayStatusBuffering   PlayStatus = "BUFFERING_STATE"
	PlayStatusInvalidPlay PlayStatus = "INVALID_PLAY_STATE"
	PlayStatusStandby     PlayStatus = "STANDBY"
)

// IsPlaying returns true if the device is currently playing
func (ps PlayStatus) IsPlaying() bool {
	return ps == PlayStatusPlaying
}

// IsPaused returns true if the device is paused
func (ps PlayStatus) IsPaused() bool {
	return ps == PlayStatusPaused
}

// IsStopped returns true if the device is stopped
func (ps PlayStatus) IsStopped() bool {
	return ps == PlayStatusStopped
}

// String returns a human-readable string representation
func (ps PlayStatus) String() string {
	switch ps {
	case PlayStatusPlaying:
		return "Playing"
	case PlayStatusPaused:
		return "Paused"
	case PlayStatusStopped:
		return "Stopped"
	case PlayStatusBuffering:
		return "Buffering"
	case PlayStatusInvalidPlay:
		return "Invalid"
	default:
		return "Unknown"
	}
}

// UnmarshalXML implements custom XML unmarshaling with validation
func (ps *PlayStatus) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}

	switch s {
	case string(PlayStatusPlaying), string(PlayStatusPaused), string(PlayStatusStopped),
		string(PlayStatusBuffering), string(PlayStatusInvalidPlay):
		*ps = PlayStatus(s)
	default:
		*ps = PlayStatusStopped // Default fallback for unknown states
	}
	return nil
}

// ShuffleSetting represents shuffle mode state
type ShuffleSetting string

const (
	ShuffleOff ShuffleSetting = "SHUFFLE_OFF"
	ShuffleOn  ShuffleSetting = "SHUFFLE_ON"
)

// IsEnabled returns true if shuffle is enabled
func (ss ShuffleSetting) IsEnabled() bool {
	return ss == ShuffleOn
}

// String returns a human-readable string representation
func (ss ShuffleSetting) String() string {
	switch ss {
	case ShuffleOn:
		return "On"
	case ShuffleOff:
		return "Off"
	default:
		return "Unknown"
	}
}

// UnmarshalXML implements custom XML unmarshaling
func (ss *ShuffleSetting) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}

	switch s {
	case string(ShuffleOn), string(ShuffleOff):
		*ss = ShuffleSetting(s)
	default:
		*ss = ShuffleOff // Default fallback
	}
	return nil
}

// RepeatSetting represents repeat mode state
type RepeatSetting string

const (
	RepeatOff RepeatSetting = "REPEAT_OFF"
	RepeatOne RepeatSetting = "REPEAT_ONE"
	RepeatAll RepeatSetting = "REPEAT_ALL"
)

// IsEnabled returns true if any repeat mode is enabled
func (rs RepeatSetting) IsEnabled() bool {
	return rs != RepeatOff
}

// String returns a human-readable string representation
func (rs RepeatSetting) String() string {
	switch rs {
	case RepeatOff:
		return "Off"
	case RepeatOne:
		return "One"
	case RepeatAll:
		return "All"
	default:
		return "Unknown"
	}
}

// UnmarshalXML implements custom XML unmarshaling
func (rs *RepeatSetting) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}

	switch s {
	case string(RepeatOff), string(RepeatOne), string(RepeatAll):
		*rs = RepeatSetting(s)
	default:
		*rs = RepeatOff // Default fallback
	}
	return nil
}

// IsEmpty returns true if no content is currently playing
func (np *NowPlaying) IsEmpty() bool {
	return np.Track == "" && np.Artist == "" && np.Album == "" && np.StationName == ""
}

// HasTrackInfo returns true if the playing content has track metadata
func (np *NowPlaying) HasTrackInfo() bool {
	return np.Track != "" || np.Artist != "" || np.Album != ""
}

// IsRadio returns true if the current source appears to be radio/streaming
func (np *NowPlaying) IsRadio() bool {
	return np.StationName != "" ||
		np.Source == "TUNEIN" ||
		np.Source == "IHEARTRADIO" ||
		np.Source == "PANDORA"
}

// GetDisplayTitle returns the best available title for display
func (np *NowPlaying) GetDisplayTitle() string {
	if np.Track != "" {
		return np.Track
	}
	if np.StationName != "" {
		return np.StationName
	}
	if np.ContentItem != nil && np.ContentItem.ItemName != "" {
		return np.ContentItem.ItemName
	}
	return "Unknown"
}

// GetDisplayArtist returns the best available artist for display
func (np *NowPlaying) GetDisplayArtist() string {
	if np.Artist != "" {
		return np.Artist
	}
	if np.Description != "" {
		return np.Description
	}
	return ""
}

// GetArtworkURL returns the artwork URL if available
func (np *NowPlaying) GetArtworkURL() string {
	if np.Art != nil && np.Art.URL != "" {
		return np.Art.URL
	}
	if np.ContentItem != nil && np.ContentItem.ContainerArt != "" {
		return np.ContentItem.ContainerArt
	}
	return ""
}

// GetPositionDuration returns position as a time.Duration
func (np *NowPlaying) GetPositionDuration() time.Duration {
	if np.Time != nil {
		return time.Duration(np.Time.Position) * time.Second
	}
	if np.Position != nil {
		return time.Duration(np.Position.Position) * time.Second
	}
	return 0
}

// GetTotalDuration returns total duration as a time.Duration
func (np *NowPlaying) GetTotalDuration() time.Duration {
	if np.Time != nil {
		return time.Duration(np.Time.Total) * time.Second
	}
	return 0
}

// FormatPosition returns a formatted position string (MM:SS)
func (np *NowPlaying) FormatPosition() string {
	if np.Time == nil && np.Position == nil {
		return ""
	}

	duration := np.GetPositionDuration()
	minutes := int(duration.Minutes())
	seconds := int(duration.Seconds()) % 60

	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// FormatDuration returns a formatted duration string (MM:SS) including total time
func (np *NowPlaying) FormatDuration() string {
	position := np.FormatPosition()
	if position == "" {
		return ""
	}

	totalDuration := np.GetTotalDuration()
	if totalDuration == 0 {
		return position
	}

	totalMinutes := int(totalDuration.Minutes())
	totalSeconds := int(totalDuration.Seconds()) % 60

	return fmt.Sprintf("%s / %d:%02d", position, totalMinutes, totalSeconds)
}

// HasTimeInfo returns true if time/duration information is available
func (np *NowPlaying) HasTimeInfo() bool {
	return np.Time != nil || np.Position != nil
}

// IsSeekSupported returns true if seeking is supported
func (np *NowPlaying) IsSeekSupported() bool {
	return np.SeekSupported != nil && np.SeekSupported.Value
}

// CanSkip returns true if skip functionality is available
func (np *NowPlaying) CanSkip() bool {
	return np.SkipEnabled != nil
}

// CanSkipPrevious returns true if skip previous functionality is available
func (np *NowPlaying) CanSkipPrevious() bool {
	return np.SkipPreviousEnabled != nil
}

// CanFavorite returns true if favorite functionality is available
func (np *NowPlaying) CanFavorite() bool {
	return np.FavoriteEnabled != nil
}
