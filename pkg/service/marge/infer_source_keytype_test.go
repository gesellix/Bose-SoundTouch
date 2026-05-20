package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

// TestInferSourceKeyTypeFromLocation pins the diagnostic URL-pattern
// inference. This function is intentionally fuzzy and only feeds a log
// line — it must never become load-bearing for binding decisions, so
// the unknown-pattern case returns "" rather than guessing.
func TestInferSourceKeyTypeFromLocation(t *testing.T) {
	cases := []struct {
		location string
		want     string
	}{
		// TuneIn station ID (sNNN) and episode ID (tNNN) patterns.
		{"/v1/playback/station/s166521", constants.ProviderTunein},
		{"/v1/playback/station/s6634", constants.ProviderTunein},
		{"/v1/playback/episodes/t544562099?encoded_name=...", constants.ProviderTunein},

		// Spotify URI containers and tracks.
		{"/playback/container/c3BvdGlmeTphbGJ1bToxRjh5MmJnOVY5blJveTh6dXhvM0p0", constants.ProviderSpotify},
		{"/playback/track/c3BvdGlmeTp0cmFjazoxbWd1OEhmSlAxNHU1Y3p3UkpsR1Zw", constants.ProviderSpotify},

		// LocalInternetRadio via the BMX /custom/v1/playback/ proxy.
		{"http://192.168.178.68/custom/v1/playback/aHR0cHM6Ly9zdHJlYW0ubGF1dC5mbS9zbW9vdGgtamF6eg==", constants.ProviderLocalInternetRadio},
		{"http://soundtouch.fritz.box/custom/v1/playback/abc", constants.ProviderLocalInternetRadio},

		// Empty / unknown — must return "" so the diagnostic stays
		// silent rather than printing a guess.
		{"", ""},
		{"http://example.invalid/some/random/path.mp3", ""},
		{"19059", ""},
		{"some-random-string", ""},
	}

	for _, tc := range cases {
		t.Run(tc.location, func(t *testing.T) {
			if got := inferSourceKeyTypeFromLocation(tc.location); got != tc.want {
				t.Errorf("inferSourceKeyTypeFromLocation(%q) = %q, want %q", tc.location, got, tc.want)
			}
		})
	}
}
