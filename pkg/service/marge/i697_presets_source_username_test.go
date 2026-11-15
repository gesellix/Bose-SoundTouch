package marge

import (
	"encoding/xml"
	"strconv"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// TestPresetsServeTheSourceAccountForEveryProvider is a regression test for
// issue 697. A speaker applies /presets as it comes: with an empty source
// <username> it drops the preset's account, and recalling the preset fails
// with INVALID_SOURCE. Seen on hardware for Spotify presets as well as for
// media-server ones, after a presetsUpdated notification made the speaker
// fetch /presets alone. /presets has to carry the account the way /full
// does; radio sources, which have none, stay empty.
func TestPresetsServeTheSourceAccountForEveryProvider(t *testing.T) {
	const (
		account     = "0000000"
		device      = "DEVICEID01"
		spotifyUser = "spotify-user"
	)

	spotify := models.ServicePreset{ButtonNumber: "2", ID: "2"}
	spotify.Source = constants.ProviderSpotify
	spotify.SourceAccount = spotifyUser
	spotify.ContentItemType = "tracklisturl"
	spotify.Location = "/playback/container/c3BvdGlmeTphbGJ1bTpURVNU"
	spotify.Name = "Album"

	tunein := models.ServicePreset{ButtonNumber: "1", ID: "1"}
	tunein.Source = constants.ProviderTunein
	tunein.ContentItemType = "stationurl"
	tunein.Location = "/v1/playback/station/s0000"
	tunein.Name = "Station"

	ds := presetSyncFixture(t, account, map[string][]models.ServicePreset{
		device: {tunein, spotify},
	})

	if _, err := AddSource(ds, account, spotifyUser, strconv.Itoa(constants.SpotifyProviderID), "bs-handle", constants.CredentialTypeTokenV3, ""); err != nil {
		t.Fatalf("add source: %v", err)
	}

	xmlData, err := PresetsToXML(ds, account, device)
	if err != nil {
		t.Fatalf("presets XML: %v", err)
	}

	var served struct {
		Presets []struct {
			ButtonNumber string `xml:"buttonNumber,attr"`
			Source       *struct {
				ProviderID string `xml:"sourceproviderid"`
				Username   string `xml:"username"`
			} `xml:"source"`
		} `xml:"preset"`
	}

	if err := xml.Unmarshal(xmlData, &served); err != nil {
		t.Fatalf("parse /presets: %v\n%s", err, xmlData)
	}

	var sawSpotify bool

	for _, p := range served.Presets {
		switch p.ButtonNumber {
		case "2":
			sawSpotify = true

			if p.Source == nil {
				t.Fatalf("Spotify preset served without a source block:\n%s", xmlData)
			}

			if p.Source.Username != spotifyUser {
				t.Errorf("Spotify preset source <username> = %q, want %q:\n%s", p.Source.Username, spotifyUser, xmlData)
			}
		case "1":
			if p.Source != nil && p.Source.Username != "" {
				t.Errorf("TuneIn preset source <username> = %q, want empty", p.Source.Username)
			}
		}
	}

	if !sawSpotify {
		t.Fatalf("Spotify preset missing from /presets:\n%s", xmlData)
	}
}
