package models

import (
	"encoding/xml"
	"strings"
)

// Sources represents the response from /sources endpoint
type Sources struct {
	XMLName    xml.Name     `xml:"sources"`
	DeviceID   string       `xml:"deviceID,attr"`
	SourceItem []SourceItem `xml:"sourceItem"`
}

// SourceItem represents an individual audio source
type SourceItem struct {
	Source           string       `xml:"source,attr"`
	SourceAccount    string       `xml:"sourceAccount,attr,omitempty"`
	Status           SourceStatus `xml:"status,attr"`
	IsLocal          bool         `xml:"isLocal,attr"`
	MultiroomAllowed bool         `xml:"multiroomallowed,attr"`
	DisplayName      string       `xml:",chardata"`
}

// SourceStatus represents the availability status of a source
type SourceStatus string

const (
	SourceStatusReady       SourceStatus = "READY"
	SourceStatusUnavailable SourceStatus = "UNAVAILABLE"
	SourceStatusError       SourceStatus = "ERROR"
)

// IsReady returns true if the source is ready for use
func (ss SourceStatus) IsReady() bool {
	return ss == SourceStatusReady
}

// IsUnavailable returns true if the source is unavailable
func (ss SourceStatus) IsUnavailable() bool {
	return ss == SourceStatusUnavailable
}

// String returns a human-readable string representation
func (ss SourceStatus) String() string {
	switch ss {
	case SourceStatusReady:
		return "Ready"
	case SourceStatusUnavailable:
		return "Unavailable"
	case SourceStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// UnmarshalXML implements custom XML unmarshaling with validation
func (ss *SourceStatus) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var s string
	if err := d.DecodeElement(&s, &start); err != nil {
		return err
	}

	switch s {
	case string(SourceStatusReady), string(SourceStatusUnavailable), string(SourceStatusError):
		*ss = SourceStatus(s)
	default:
		*ss = SourceStatusUnavailable // Default fallback for unknown states
	}
	return nil
}

// GetDisplayName returns the best available display name for the source
func (si *SourceItem) GetDisplayName() string {
	if si.DisplayName != "" {
		return si.DisplayName
	}
	if si.SourceAccount != "" && si.SourceAccount != si.Source {
		return si.SourceAccount
	}
	return strings.Title(strings.ToLower(si.Source))
}

// IsSpotify returns true if this is a Spotify source
func (si *SourceItem) IsSpotify() bool {
	return si.Source == "SPOTIFY"
}

// IsBluetoothSource returns true if this is a Bluetooth source
func (si *SourceItem) IsBluetoothSource() bool {
	return si.Source == "BLUETOOTH"
}

// IsAuxSource returns true if this is an AUX input source
func (si *SourceItem) IsAuxSource() bool {
	return si.Source == "AUX"
}

// IsStreamingService returns true if this is an online streaming service
func (si *SourceItem) IsStreamingService() bool {
	streamingSources := []string{"SPOTIFY", "PANDORA", "TUNEIN", "IHEARTRADIO", "AMAZON", "LOCAL_INTERNET_RADIO"}
	for _, source := range streamingSources {
		if si.Source == source {
			return true
		}
	}
	return false
}

// IsLocalSource returns true if this is a local input source
func (si *SourceItem) IsLocalSource() bool {
	return si.IsLocal
}

// SupportsMultiroom returns true if this source supports multiroom playback
func (si *SourceItem) SupportsMultiroom() bool {
	return si.MultiroomAllowed
}

// GetAvailableSources returns only sources that are ready for use
func (s *Sources) GetAvailableSources() []SourceItem {
	var available []SourceItem
	for _, source := range s.SourceItem {
		if source.Status.IsReady() {
			available = append(available, source)
		}
	}
	return available
}

// GetSourcesByType returns sources filtered by source type
func (s *Sources) GetSourcesByType(sourceType string) []SourceItem {
	var filtered []SourceItem
	for _, source := range s.SourceItem {
		if source.Source == sourceType {
			filtered = append(filtered, source)
		}
	}
	return filtered
}

// GetSpotifySources returns all Spotify sources (there can be multiple accounts)
func (s *Sources) GetSpotifySources() []SourceItem {
	return s.GetSourcesByType("SPOTIFY")
}

// GetReadySpotifySources returns only ready Spotify sources
func (s *Sources) GetReadySpotifySources() []SourceItem {
	var ready []SourceItem
	for _, source := range s.GetSpotifySources() {
		if source.Status.IsReady() {
			ready = append(ready, source)
		}
	}
	return ready
}

// GetStreamingSources returns all streaming service sources
func (s *Sources) GetStreamingSources() []SourceItem {
	var streaming []SourceItem
	for _, source := range s.SourceItem {
		if source.IsStreamingService() {
			streaming = append(streaming, source)
		}
	}
	return streaming
}

// GetLocalSources returns all local input sources
func (s *Sources) GetLocalSources() []SourceItem {
	var local []SourceItem
	for _, source := range s.SourceItem {
		if source.IsLocalSource() {
			local = append(local, source)
		}
	}
	return local
}

// GetMultiroomSources returns sources that support multiroom playback
func (s *Sources) GetMultiroomSources() []SourceItem {
	var multiroom []SourceItem
	for _, source := range s.SourceItem {
		if source.SupportsMultiroom() {
			multiroom = append(multiroom, source)
		}
	}
	return multiroom
}

// HasSource returns true if the specified source type is available
func (s *Sources) HasSource(sourceType string) bool {
	sources := s.GetSourcesByType(sourceType)
	for _, source := range sources {
		if source.Status.IsReady() {
			return true
		}
	}
	return false
}

// HasSpotify returns true if any Spotify source is ready
func (s *Sources) HasSpotify() bool {
	return s.HasSource("SPOTIFY")
}

// HasBluetooth returns true if Bluetooth source is ready
func (s *Sources) HasBluetooth() bool {
	return s.HasSource("BLUETOOTH")
}

// HasAux returns true if AUX input is ready
func (s *Sources) HasAux() bool {
	return s.HasSource("AUX")
}

// GetSourceCount returns the total number of sources
func (s *Sources) GetSourceCount() int {
	return len(s.SourceItem)
}

// GetReadySourceCount returns the number of ready sources
func (s *Sources) GetReadySourceCount() int {
	return len(s.GetAvailableSources())
}
