package models

import (
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
)

// Error constants for speaker validation
var (
	ErrInvalidURL     = errors.New("URL cannot be empty")
	ErrInvalidAppKey  = errors.New("app key cannot be empty")
	ErrInvalidService = errors.New("service cannot be empty")
	ErrInvalidVolume  = errors.New("volume must be between 0 and 100")
)

// PlayInfo represents the request body for the /speaker endpoint to play TTS or URL content
type PlayInfo struct {
	XMLName xml.Name `xml:"play_info"`
	URL     string   `xml:"url"`
	AppKey  string   `xml:"app_key"`
	Service string   `xml:"service"`
	Message string   `xml:"message"`
	Reason  string   `xml:"reason"`
	Volume  *int     `xml:"volume,omitempty"`
}

// SpeakerResponse represents the response from the /speaker endpoint
type SpeakerResponse struct {
	XMLName xml.Name `xml:"status"`
	Value   string   `xml:",chardata"`
}

// SpeakerPlayStatus represents the status during speaker playback
type SpeakerPlayStatus struct {
	Service string `json:"service"`
	Message string `json:"message"`
	Reason  string `json:"reason"`
	Volume  int    `json:"volume,omitempty"`
}

// NewPlayInfo creates a new PlayInfo instance for TTS or URL playback
func NewPlayInfo(url, appKey, service, message, reason string) *PlayInfo {
	return &PlayInfo{
		XMLName: xml.Name{Local: "play_info"},
		URL:     url,
		AppKey:  appKey,
		Service: service,
		Message: message,
		Reason:  reason,
	}
}

// SetVolume sets the volume level for playback
func (p *PlayInfo) SetVolume(volume int) *PlayInfo {
	p.Volume = &volume
	return p
}

// NewTTSPlayInfo creates a PlayInfo for Google TTS playback
func NewTTSPlayInfo(text, appKey, language string, volume ...int) *PlayInfo {
	// URL encode the text for Google TTS
	url := fmt.Sprintf("http://translate.google.com/translate_tts?ie=UTF-8&tl=%s&client=tw-ob&q=%s", language, url.QueryEscape(text))

	playInfo := &PlayInfo{
		XMLName: xml.Name{Local: "play_info"},
		URL:     url,
		AppKey:  appKey,
		Service: "TTS Notification",
		Message: "Google TTS",
		Reason:  text,
	}

	if len(volume) > 0 {
		playInfo.Volume = &volume[0]
	}

	return playInfo
}

// NewURLPlayInfo creates a PlayInfo for URL content playback
func NewURLPlayInfo(url, appKey, service, message, reason string, volume ...int) *PlayInfo {
	playInfo := &PlayInfo{
		XMLName: xml.Name{Local: "play_info"},
		URL:     url,
		AppKey:  appKey,
		Service: service,
		Message: message,
		Reason:  reason,
	}

	if len(volume) > 0 {
		playInfo.Volume = &volume[0]
	}

	return playInfo
}

// Validate validates the PlayInfo request
func (p *PlayInfo) Validate() error {
	if p.URL == "" {
		return ErrInvalidURL
	}

	if p.AppKey == "" {
		return ErrInvalidAppKey
	}

	if p.Service == "" {
		return ErrInvalidService
	}

	if p.Volume != nil && (*p.Volume < 0 || *p.Volume > 100) {
		return ErrInvalidVolume
	}

	return nil
}

// String returns a string representation of the PlayInfo
func (p *PlayInfo) String() string {
	volumeStr := "current"
	if p.Volume != nil {
		volumeStr = string(rune(*p.Volume))
	}

	return "Service: " + p.Service + ", Message: " + p.Message + ", Volume: " + volumeStr
}
