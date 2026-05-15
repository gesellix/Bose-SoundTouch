package setup

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/service/testing/fakespeaker"
)

// TestIssue235_SpotifyConnectNowPlayingReportsNotPresetable documents
// the device-side signal behind issue #235:
//
//	https://github.com/gesellix/Bose-SoundTouch/issues/235
//
// When music is streamed to a SoundTouch via Spotify Connect (the
// Spotify mobile/desktop app sends audio to the speaker, as opposed
// to the speaker's own Spotify integration), the speaker's
// /now_playing response carries:
//
//   - source = SPOTIFY
//   - sourceAccount = SpotifyConnectUserName (the magic placeholder)
//   - ContentItem.location = a base64-encoded Spotify URI that *does*
//     look replayable (e.g. spotify:playlist:... once decoded)
//   - **ContentItem.isPresetable = false**
//
// The CLI's storeCurrentPreset (cmd/soundtouch-cli/cmd_preset.go:41)
// keys on `IsPresetable` and bails out with the documented error
// "current content cannot be preset" — exactly what the reporter
// sees. The contradiction at the heart of the bug: the location is a
// perfectly resolvable Spotify URI, but the speaker still refuses
// to expose it as presetable.
//
// What this test locks in:
//
//   - The /now_playing payload AfterTouch reads from a Spotify
//     Connect session has IsPresetable=false, despite a non-empty
//     location.
//   - The location field, base64-URL-decoded, yields a recognisable
//     `spotify:` URI. The contradiction is preserved verbatim so we
//     don't accidentally "fix" the test by stripping the location.
//
// When AfterTouch grows logic to override the IsPresetable signal
// for Spotify Connect (e.g. a CLI --force flag, or service-side
// resolution to the device's own Spotify integration), the assertion
// here stays sound — it tests what the device emits, not what the
// CLI decides — but a sibling test should assert the new fallback
// path produces a successful preset.
//
// Pattern mirrors pkg/service/setup/issue218_regression_test.go.
func TestIssue235_SpotifyConnectNowPlayingReportsNotPresetable(t *testing.T) {
	npXML, err := os.ReadFile(filepath.Join("testdata", "issue235", "now_playing.xml"))
	if err != nil {
		t.Fatalf("read issue235 now_playing fixture: %v", err)
	}

	// Fixture sanity: the SpotifyConnectUserName marker and
	// isPresetable=false are the load-bearing parts.
	if !strings.Contains(string(npXML), "SpotifyConnectUserName") {
		t.Fatalf("fixture missing SpotifyConnectUserName marker; got:\n%s", npXML)
	}

	if !strings.Contains(string(npXML), `isPresetable="false"`) {
		t.Fatalf("fixture missing isPresetable=\"false\"; got:\n%s", npXML)
	}

	s, err := fakespeaker.Start(fakespeaker.Config{
		FixtureOverrides: map[string][]byte{
			"/now_playing": npXML,
		},
	})
	if err != nil {
		t.Fatalf("start fakespeaker: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	// fakespeaker.HTTPAddr() returns "127.0.0.1:<port>"; client.NewClientFromHost
	// accepts the host:port form directly and routes /now_playing to it.
	c := client.NewClientFromHost(s.HTTPAddr())

	now, err := c.GetNowPlaying()
	if err != nil {
		t.Fatalf("GetNowPlaying: %v", err)
	}

	if now.ContentItem == nil {
		t.Fatalf("ContentItem is nil; full now_playing:\n%+v", now)
	}

	// The bug's defining signal: false despite a non-empty location.
	if now.ContentItem.IsPresetable {
		t.Errorf("ContentItem.IsPresetable = true, want false — the Spotify Connect contradiction was 'fixed' on the device side; review whether the CLI's storeCurrentPreset still needs the IsPresetable gate")
	}

	if now.ContentItem.Location == "" {
		t.Errorf("ContentItem.Location is empty, want a Spotify URI — fixture has drifted from the issue payload")
	}

	if now.Source != "SPOTIFY" {
		t.Errorf("Source = %q, want SPOTIFY", now.Source)
	}

	if now.SourceAccount != "SpotifyConnectUserName" {
		t.Errorf("SourceAccount = %q, want SpotifyConnectUserName (the Spotify Connect marker)", now.SourceAccount)
	}

	// Surface the contradiction: the location decodes to a real Spotify URI,
	// so the IsPresetable=false is purely a device-side policy. Decode the
	// path-segment that follows `/playback/container/`.
	const containerPrefix = "/playback/container/"

	segment := strings.TrimPrefix(now.ContentItem.Location, containerPrefix)
	if segment == now.ContentItem.Location {
		t.Logf("note: location does not match /playback/container/<base64> shape (was %q); not decoding", now.ContentItem.Location)
		return
	}

	decoded, err := base64.URLEncoding.DecodeString(segment)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(segment, "="))
		if err != nil {
			t.Logf("note: location segment %q is not base64-URL-decodable: %v", segment, err)
			return
		}
	}

	if !strings.HasPrefix(string(decoded), "spotify:") {
		t.Errorf("decoded location %q does not look like a spotify: URI; fixture may have drifted", decoded)
	}
}
