package handlers

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestIssue218_OrionStationResolvesPresetStreamURL closes the loop on
// the issue #218 regression: it takes the exact preset location URL the
// reporter pasted, follows it against the real router, and asserts the
// returned BmxPlaybackResponse exposes the speaker-playable streamUrl.
//
// Pairs with pkg/service/setup/issue218_regression_test.go, which
// verifies the preset survives device sync verbatim. Together they
// prove that:
//
//  1. The sync step preserves the cloud URL embedded in
//     LOCAL_INTERNET_RADIO presets.
//  2. Hitting that URL against AfterTouch's router resolves it to the
//     speaker-playable stream — no rewrite required on the persisted
//     preset itself.
//
// Before commit f3a4658, this test would have 404'd: the orion routes
// were wrongly nested under `/bmx/` while the BMX registry advertises
// the un-prefixed path. See the matching doc-comment on
// HandleOrionPlayback for the protocol detail.
func TestIssue218_OrionStationResolvesPresetStreamURL(t *testing.T) {
	// Verbatim from pkg/service/setup/testdata/issue218/presets.xml's
	// ContentItem `location` attribute (issue #218 body). Decoded
	// query payload is:
	//   {"name":"OPB","imageUrl":"","streamUrl":"http://ais-sa3.cdnstream1.com/2440_128.aac"}
	const presetLocation = "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJuYW1lIjoiT1BCIiwiaW1hZ2VVcmwiOiIiLCJzdHJlYW1VcmwiOiJodHRwOi8vYWlzLXNhMy5jZG5zdHJlYW0xLmNvbS8yNDQwXzEyOC5hYWMifQ%3D%3D"

	const wantStreamURL = "http://ais-sa3.cdnstream1.com/2440_128.aac"

	// Sanity: the base64 payload really does encode wantStreamURL.
	// If the fixture ever diverges from this expectation the test
	// would silently keep passing on whatever the new payload says;
	// pin it explicitly.
	parsedLocation, err := url.Parse(presetLocation)
	if err != nil {
		t.Fatalf("parse preset location: %v", err)
	}

	data := parsedLocation.Query().Get("data")
	if data == "" {
		t.Fatalf("preset location has no `data` query param: %s", presetLocation)
	}

	decoded, err := base64.URLEncoding.DecodeString(data)
	if err != nil {
		// Some captures use RawURLEncoding (no padding); fall back.
		decoded, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(data, "="))
		if err != nil {
			t.Fatalf("decode data blob: %v", err)
		}
	}

	if !strings.Contains(string(decoded), wantStreamURL) {
		t.Fatalf("fixture data does not encode the expected streamUrl.\ndecoded:\n%s\nwant substring:\n%s",
			decoded, wantStreamURL)
	}

	// Drive the real router. Use only the path+query from the preset
	// URL — host is what DNS interception or URL-flip would have
	// substituted at runtime, not what the test server bound to.
	r, _ := setupRouter("http://localhost:8001", nil)
	ts := httptest.NewServer(r)

	t.Cleanup(ts.Close)

	resolved := ts.URL + parsedLocation.RequestURI()

	// Real speakers retrieve an orion token from
	// POST /core02/svc-bmx-adapter-orion/prod/orion/token before they
	// ever follow a LOCAL_INTERNET_RADIO preset; the playback handler
	// rejects an empty Authorization header for parity with the other
	// BMX playback routes. Use a sentinel Bearer token to match that
	// shape — HandleOrionPlayback doesn't validate the token contents,
	// only its presence.
	req, _ := http.NewRequest("GET", resolved, nil)
	req.Header.Set("Authorization", "Bearer mock-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", resolved, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("GET %s → %d, want 200; body:\n%s", resolved, resp.StatusCode, body)
	}

	var got models.BmxPlaybackResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Audio.StreamUrl != wantStreamURL {
		t.Errorf("audio.streamUrl = %q, want %q", got.Audio.StreamUrl, wantStreamURL)
	}

	if got.Name != "OPB" {
		t.Errorf("name = %q, want %q", got.Name, "OPB")
	}

	if got.StreamType != "liveRadio" {
		t.Errorf("streamType = %q, want %q", got.StreamType, "liveRadio")
	}

	// The streams array should mirror the top-level streamUrl —
	// PlayCustomStream sets both for parity with what real Bose emits.
	if len(got.Audio.Streams) == 0 {
		t.Errorf("audio.streams empty, want at least one entry with streamUrl=%q", wantStreamURL)
	} else if got.Audio.Streams[0].StreamUrl != wantStreamURL {
		t.Errorf("audio.streams[0].streamUrl = %q, want %q", got.Audio.Streams[0].StreamUrl, wantStreamURL)
	}
}
