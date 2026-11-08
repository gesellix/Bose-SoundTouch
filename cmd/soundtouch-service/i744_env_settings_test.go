package main

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// Issue 744: a Settings value that an environment variable or flag also sets
// came back after a restart, with nothing saying why.

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	previous := log.Writer()

	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(previous) })

	return &buf
}

func TestSettingsControlledByEnvironmentNamesOnlyWhatIsSet_GH744(t *testing.T) {
	captureLog(t)

	got := settingsControlledByEnvironment(serviceConfig{
		spotifyRedirectURI: "https://env.example/callback",
		ttsVolume:          40,
	}, datastore.Settings{AmazonClientID: "only-saved"})

	want := map[string]string{
		"spotify_redirect_uri": "SPOTIFY_REDIRECT_URI",
		"tts_volume":           "TTS_VOLUME",
	}

	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}

	for key, envVar := range want {
		if got[key] != envVar {
			t.Errorf("%s = %q, want %q", key, got[key], envVar)
		}
	}
}

func TestSettingsControlledByEnvironmentWarnsOnlyOnADifference_GH744(t *testing.T) {
	tests := []struct {
		name      string
		saved     string
		wantLines int
	}{
		{name: "differs from saved", saved: "saved-secret", wantLines: 1},
		{name: "same as saved", saved: "env-secret", wantLines: 0},
		{name: "nothing saved", saved: "", wantLines: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs := captureLog(t)

			settingsControlledByEnvironment(
				serviceConfig{spotifyClientSecret: "env-secret"},
				datastore.Settings{SpotifyClientSecret: tt.saved},
			)

			if got := strings.Count(logs.String(), "SPOTIFY_CLIENT_SECRET"); got != tt.wantLines {
				t.Errorf("warnings = %d, want %d: %s", got, tt.wantLines, logs)
			}

			if strings.Contains(logs.String(), "env-secret") || strings.Contains(logs.String(), "saved-secret") {
				t.Errorf("log carries a value: %s", logs)
			}
		})
	}
}
