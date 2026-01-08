package models

import (
	"encoding/xml"
	"strconv"
	"time"
)

// Presets represents the response from /presets endpoint
type Presets struct {
	XMLName xml.Name `xml:"presets"`
	Preset  []Preset `xml:"preset"`
}

// Preset represents an individual preset
type Preset struct {
	ID          int          `xml:"id,attr"`
	CreatedOn   *int64       `xml:"createdOn,attr,omitempty"`
	UpdatedOn   *int64       `xml:"updatedOn,attr,omitempty"`
	ContentItem *ContentItem `xml:"ContentItem,omitempty"`
}

// GetCreatedTime returns the creation time as a time.Time
func (p *Preset) GetCreatedTime() time.Time {
	if p.CreatedOn != nil {
		return time.Unix(*p.CreatedOn, 0)
	}
	return time.Time{}
}

// GetUpdatedTime returns the last updated time as a time.Time
func (p *Preset) GetUpdatedTime() time.Time {
	if p.UpdatedOn != nil {
		return time.Unix(*p.UpdatedOn, 0)
	}
	return time.Time{}
}

// HasTimestamps returns true if the preset has creation/update timestamps
func (p *Preset) HasTimestamps() bool {
	return p.CreatedOn != nil || p.UpdatedOn != nil
}

// GetDisplayName returns the best available display name for the preset
func (p *Preset) GetDisplayName() string {
	if p.ContentItem != nil && p.ContentItem.ItemName != "" {
		return p.ContentItem.ItemName
	}
	return "Preset " + strconv.Itoa(p.ID)
}

// GetArtworkURL returns the artwork URL if available
func (p *Preset) GetArtworkURL() string {
	if p.ContentItem != nil && p.ContentItem.ContainerArt != "" {
		return p.ContentItem.ContainerArt
	}
	return ""
}

// IsSpotifyPreset returns true if this is a Spotify preset
func (p *Preset) IsSpotifyPreset() bool {
	return p.ContentItem != nil && p.ContentItem.Source == "SPOTIFY"
}

// IsEmpty returns true if the preset has no content
func (p *Preset) IsEmpty() bool {
	return p.ContentItem == nil
}

// GetSource returns the source of the preset content
func (p *Preset) GetSource() string {
	if p.ContentItem != nil {
		return p.ContentItem.Source
	}
	return ""
}

// GetSourceAccount returns the source account of the preset content
func (p *Preset) GetSourceAccount() string {
	if p.ContentItem != nil {
		return p.ContentItem.SourceAccount
	}
	return ""
}

// GetContentType returns the content type of the preset
func (p *Preset) GetContentType() string {
	if p.ContentItem != nil {
		return p.ContentItem.Type
	}
	return ""
}

// GetLocation returns the content location/URL
func (p *Preset) GetLocation() string {
	if p.ContentItem != nil {
		return p.ContentItem.Location
	}
	return ""
}

// IsPresetable returns true if the content can be saved as a preset
func (p *Preset) IsPresetable() bool {
	return p.ContentItem != nil && p.ContentItem.IsPresetable
}

// GetPresetCount returns the total number of presets
func (ps *Presets) GetPresetCount() int {
	return len(ps.Preset)
}

// GetPresetByID returns a preset by its ID
func (ps *Presets) GetPresetByID(id int) *Preset {
	for _, preset := range ps.Preset {
		if preset.ID == id {
			return &preset
		}
	}
	return nil
}

// GetSpotifyPresets returns all Spotify presets
func (ps *Presets) GetSpotifyPresets() []Preset {
	var spotify []Preset
	for _, preset := range ps.Preset {
		if preset.IsSpotifyPreset() {
			spotify = append(spotify, preset)
		}
	}
	return spotify
}

// GetPresetsBySource returns presets filtered by source
func (ps *Presets) GetPresetsBySource(source string) []Preset {
	var filtered []Preset
	for _, preset := range ps.Preset {
		if preset.GetSource() == source {
			filtered = append(filtered, preset)
		}
	}
	return filtered
}

// GetEmptyPresetSlots returns preset IDs that are empty (1-6)
func (ps *Presets) GetEmptyPresetSlots() []int {
	var empty []int
	used := make(map[int]bool)

	// Mark used slots
	for _, preset := range ps.Preset {
		if !preset.IsEmpty() {
			used[preset.ID] = true
		}
	}

	// Find empty slots (1-6 are typical preset slots)
	for i := 1; i <= 6; i++ {
		if !used[i] {
			empty = append(empty, i)
		}
	}

	return empty
}

// HasPresets returns true if there are any presets configured
func (ps *Presets) HasPresets() bool {
	for _, preset := range ps.Preset {
		if !preset.IsEmpty() {
			return true
		}
	}
	return false
}

// GetUsedPresetSlots returns preset IDs that have content
func (ps *Presets) GetUsedPresetSlots() []int {
	var used []int
	for _, preset := range ps.Preset {
		if !preset.IsEmpty() {
			used = append(used, preset.ID)
		}
	}
	return used
}

// GetMostRecentPreset returns the most recently updated preset
func (ps *Presets) GetMostRecentPreset() *Preset {
	var mostRecent *Preset
	var latestTime int64

	for _, preset := range ps.Preset {
		if preset.UpdatedOn != nil && *preset.UpdatedOn > latestTime {
			latestTime = *preset.UpdatedOn
			mostRecent = &preset
		} else if preset.CreatedOn != nil && preset.UpdatedOn == nil && *preset.CreatedOn > latestTime {
			latestTime = *preset.CreatedOn
			mostRecent = &preset
		}
	}

	return mostRecent
}

// GetOldestPreset returns the oldest preset
func (ps *Presets) GetOldestPreset() *Preset {
	var oldest *Preset
	var earliestTime int64 = 9223372036854775807 // max int64

	for _, preset := range ps.Preset {
		if preset.CreatedOn != nil && *preset.CreatedOn < earliestTime {
			earliestTime = *preset.CreatedOn
			oldest = &preset
		}
	}

	return oldest
}

// GetPresetsSummary returns a summary of preset usage
func (ps *Presets) GetPresetsSummary() map[string]int {
	summary := map[string]int{
		"total":   ps.GetPresetCount(),
		"used":    len(ps.GetUsedPresetSlots()),
		"empty":   len(ps.GetEmptyPresetSlots()),
		"spotify": len(ps.GetSpotifyPresets()),
	}

	// Count by source
	sources := make(map[string]int)
	for _, preset := range ps.Preset {
		if !preset.IsEmpty() {
			source := preset.GetSource()
			sources[source]++
		}
	}

	// Add source counts to summary
	for source, count := range sources {
		summary[source] = count
	}

	return summary
}
