package models

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"testing"
)

const testOrionData = "eyJuYW1lIjoiRG9jIFJhZGlvIn0%3D"

func TestRelativeOrionLocation(t *testing.T) {
	cases := []struct {
		name     string
		location string
		want     string
		ok       bool
	}{
		{"relative", "/station?data=" + testOrionData, "/station?data=" + testOrionData, true},
		{"relative, surrounding space", "  /station?data=" + testOrionData + " ", "/station?data=" + testOrionData, true},
		{"absolute, AfterTouch host", "http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + testOrionData, "/station?data=" + testOrionData, true},
		{"absolute, Bose cloud", "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + testOrionData, "/station?data=" + testOrionData, true},
		{"absolute, foreign host", "https://radio.example.org/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + testOrionData, "/station?data=" + testOrionData, true},
		{"absolute, query kept verbatim", "http://198.51.100.7/core02/svc-bmx-adapter-orion/prod/orion/station?data=a%2Bb%3D&x=1", "/station?data=a%2Bb%3D&x=1", true},
		{"radio browser relative", "/stations/byuuid/abc", "", false},
		{"tunein relative", "/v1/playback/station/s1234", "", false},
		{"orion but not a station", "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/v1/playback/station/s1234", "", false},
		{"raw stream url", "http://stream.example.org/station?data=x", "", false},
		{"empty", "", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := RelativeOrionLocation(tc.location)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("RelativeOrionLocation(%q) = (%q, %v), want (%q, %v)", tc.location, got, ok, tc.want, tc.ok)
			}

			if IsOrionStationLocation(tc.location) != tc.ok {
				t.Fatalf("IsOrionStationLocation(%q) = %v, want %v", tc.location, !tc.ok, tc.ok)
			}
		})
	}
}

func TestOrionLocationHost(t *testing.T) {
	if got := OrionLocationHost("http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=x"); got != "192.0.2.10:8000" {
		t.Errorf("absolute host = %q", got)
	}

	if got := OrionLocationHost("/station?data=x"); got != "" {
		t.Errorf("relative host = %q, want empty", got)
	}

	if got := OrionLocationHost("http://192.0.2.10/stream.mp3"); got != "" {
		t.Errorf("non-Orion host = %q, want empty", got)
	}
}

func TestCanonicalContentLocation(t *testing.T) {
	absolute := "http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + testOrionData
	relative := "/station?data=" + testOrionData

	if got := CanonicalContentLocation("LOCAL_INTERNET_RADIO", absolute); got != relative {
		t.Errorf("absolute Orion location canonicalised to %q, want %q", got, relative)
	}

	if got := CanonicalContentLocation("LOCAL_INTERNET_RADIO", relative); got != relative {
		t.Errorf("relative Orion location canonicalised to %q, want unchanged", got)
	}

	// Other sources keep their location, even if it happens to look like an
	// Orion one: only LOCAL_INTERNET_RADIO plays Orion stations.
	if got := CanonicalContentLocation("TUNEIN", absolute); got != absolute {
		t.Errorf("TUNEIN location changed to %q", got)
	}

	raw := "http://stream.example.org/live.mp3"
	if got := CanonicalContentLocation("LOCAL_INTERNET_RADIO", raw); got != raw {
		t.Errorf("raw stream location changed to %q", got)
	}
}

// orionLocation builds a relative Orion location the way bmx.BuildOrionLocation
// does (that package imports this one, so it can't be used here).
func orionLocation(t *testing.T, name, imageURL, streamURL string) string {
	t.Helper()

	data, err := json.Marshal(map[string]string{"name": name, "imageUrl": imageURL, "streamUrl": streamURL})
	if err != nil {
		t.Fatal(err)
	}

	return OrionStationPath + "?data=" + url.QueryEscape(base64.StdEncoding.EncodeToString(data))
}

// TestCanonicalContentLocationKeysStationsByStream: the relative and absolute
// Orion spellings and the older playback adapter all play one stream, and
// count as one station (issue 784), whatever name or artwork they carry.
func TestCanonicalContentLocationKeysStationsByStream(t *testing.T) {
	const stream = "https://stream.example.org/technikum.mp3"

	relative := orionLocation(t, "TechnikumCity", "", stream)
	withArt := orionLocation(t, "Technikum City", "https://img.example.org/logo.png", stream)
	absolute := "http://192.0.2.10:8000" + OrionBasePath + relative
	adapterStd := "http://192.0.2.10:8000/custom/v1/playback/" + base64.StdEncoding.EncodeToString([]byte(stream)) + "?name=TechnikumCity"
	adapterURLSafe := "http://192.0.2.10:8000/custom/v1/playback/" + base64.URLEncoding.EncodeToString([]byte(stream))
	adapterPlain := "http://192.0.2.10:8000/custom/v1/playback/" + url.PathEscape(stream)

	want := CanonicalContentLocation("LOCAL_INTERNET_RADIO", relative)
	if want == relative {
		t.Fatalf("canonical key for %q is the location itself, want the stream key", relative)
	}

	for name, location := range map[string]string{
		"other name and artwork": withArt,
		"absolute Orion":         absolute,
		"playback adapter":       adapterStd,
		"URL-safe base64":        adapterURLSafe,
		"plain escaped URL":      adapterPlain,
	} {
		if got := CanonicalContentLocation("LOCAL_INTERNET_RADIO", location); got != want {
			t.Errorf("%s: canonical %q, want %q", name, got, want)
		}
	}

	other := orionLocation(t, "TechnikumCity", "", "https://stream.example.org/other.mp3")
	if CanonicalContentLocation("LOCAL_INTERNET_RADIO", other) == want {
		t.Error("a different stream got the same key")
	}

	// A raw stream URL as location does not play (the speaker wants a station
	// description), so it stays its own entry rather than matching the
	// station that wraps it.
	if CanonicalContentLocation("LOCAL_INTERNET_RADIO", stream) == want {
		t.Error("a raw stream location matched the wrapped station")
	}

	if got := CanonicalContentLocation("TUNEIN", adapterStd); got != adapterStd {
		t.Errorf("TUNEIN location changed to %q", got)
	}
}

func TestStationStreamURLRejectsOtherLocations(t *testing.T) {
	for _, location := range []string{
		"",
		"http://stream.example.org/live.mp3",
		"/v1/playback/station/s12345",
		"http://192.0.2.10:8000/custom/v1/playback/",
		"http://192.0.2.10:8000/custom/v1/playback/bm90LWEtdXJs", // "not-a-url"
		"/station?data=" + testOrionData,                         // no streamUrl
	} {
		if stream, ok := StationStreamURL(location); ok {
			t.Errorf("StationStreamURL(%q) = %q, want none", location, stream)
		}
	}
}
