// Package models provides data structures and types for music service account management
// on Bose SoundTouch devices.
package models

import (
	"encoding/xml"
	"fmt"
)

// MusicServiceCredentials represents credentials for music service account operations
type MusicServiceCredentials struct {
	XMLName     xml.Name `xml:"credentials"`
	Source      string   `xml:"source,attr"`
	DisplayName string   `xml:"displayName,attr,omitempty"`
	User        string   `xml:"user"`
	Pass        string   `xml:"pass"`
}

// NewMusicServiceCredentials creates new music service credentials
func NewMusicServiceCredentials(source, displayName, user, pass string) *MusicServiceCredentials {
	return &MusicServiceCredentials{
		Source:      source,
		DisplayName: displayName,
		User:        user,
		Pass:        pass,
	}
}

// NewSpotifyCredentials creates credentials for Spotify service
func NewSpotifyCredentials(user, pass string) *MusicServiceCredentials {
	return NewMusicServiceCredentials("SPOTIFY", "Spotify Premium", user, pass)
}

// NewPandoraCredentials creates credentials for Pandora service
func NewPandoraCredentials(user, pass string) *MusicServiceCredentials {
	return NewMusicServiceCredentials("PANDORA", "Pandora Music Service", user, pass)
}

// NewStoredMusicCredentials creates credentials for STORED_MUSIC (NAS/UPnP) service
func NewStoredMusicCredentials(user, displayName string) *MusicServiceCredentials {
	return NewMusicServiceCredentials("STORED_MUSIC", displayName, user, "")
}

// NewAmazonMusicCredentials creates credentials for Amazon Music service
func NewAmazonMusicCredentials(user, pass string) *MusicServiceCredentials {
	return NewMusicServiceCredentials("AMAZON", "Amazon Music", user, pass)
}

// NewDeezerCredentials creates credentials for Deezer service
func NewDeezerCredentials(user, pass string) *MusicServiceCredentials {
	return NewMusicServiceCredentials("DEEZER", "Deezer Premium", user, pass)
}

// NewIHeartRadioCredentials creates credentials for iHeartRadio service
func NewIHeartRadioCredentials(user, pass string) *MusicServiceCredentials {
	return NewMusicServiceCredentials("IHEART", "iHeartRadio", user, pass)
}

// Validate ensures the credentials have required fields
func (cred *MusicServiceCredentials) Validate() error {
	if cred.Source == "" {
		return fmt.Errorf("source cannot be empty")
	}

	if cred.User == "" {
		return fmt.Errorf("user cannot be empty")
	}

	// STORED_MUSIC typically doesn't require a password
	if cred.Source != "STORED_MUSIC" && cred.Pass == "" {
		return fmt.Errorf("password cannot be empty for %s", cred.Source)
	}

	return nil
}

// IsForRemoval returns true if these credentials are for removing an account (empty password)
func (cred *MusicServiceCredentials) IsForRemoval() bool {
	return cred.Pass == ""
}

// HasPassword returns true if credentials include a password
func (cred *MusicServiceCredentials) HasPassword() bool {
	return cred.Pass != ""
}

// GetDescription returns a human-readable description of the service
func (cred *MusicServiceCredentials) GetDescription() string {
	if cred.DisplayName != "" {
		return cred.DisplayName
	}

	switch cred.Source {
	case "SPOTIFY":
		return "Spotify Premium"
	case "PANDORA":
		return "Pandora Music Service"
	case "AMAZON":
		return "Amazon Music"
	case "DEEZER":
		return "Deezer Premium"
	case "IHEART":
		return "iHeartRadio"
	case "STORED_MUSIC":
		return "Network Music Library"
	case "LOCAL_MUSIC":
		return "Local Music Server"
	default:
		return cred.Source
	}
}

// OAuthCredentials represents the credentials sent to /setMusicServiceOAuthAccount
type OAuthCredentials struct {
	XMLName     xml.Name `xml:"OAuthCredentials"`
	Source      string   `xml:"source,attr"`
	DisplayName string   `xml:"displayName,attr,omitempty"`
	User        string   `xml:"user"`
	Code        string   `xml:"code"`
	Version     string   `xml:"version"`
}

// NewSpotifyOAuthCredentials creates OAuth credentials for Spotify
func NewSpotifyOAuthCredentials(user, code, displayName string) *OAuthCredentials {
	if displayName == "" {
		displayName = user
	}

	return &OAuthCredentials{
		Source:      "SPOTIFY",
		DisplayName: displayName,
		User:        user,
		Code:        code,
		Version:     "token_version_3",
	}
}

// MusicServiceAccountResponse represents the response from account management operations
type MusicServiceAccountResponse struct {
	XMLName xml.Name `xml:"status"`
	Status  string   `xml:",chardata"`
}

// SourcesUpdatedResponse represents the response from /notification
type SourcesUpdatedResponse struct {
	XMLName xml.Name `xml:"status"`
	Status  string   `xml:",chardata"`
}

// IsSuccess returns true if the account operation was successful
func (resp *MusicServiceAccountResponse) IsSuccess() bool {
	return resp.Status == "/setMusicServiceAccount" || resp.Status == "/removeMusicServiceAccount" || resp.Status == "/notification"
}
