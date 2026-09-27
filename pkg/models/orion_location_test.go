package models

import "testing"

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
