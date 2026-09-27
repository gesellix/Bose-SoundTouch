package main

import (
	"strings"
	"testing"
)

func TestIsOrionLocationRecognisesBothForms(t *testing.T) {
	cases := map[string]bool{
		"/station?data=eyJ9": true,
		"http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJ9":      true,
		"https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJ9": true,
		"http://stream.example.org/live.mp3":                                                    false,
		"/stations/byuuid/abc":                                                                  false,
		"":                                                                                      false,
	}

	for location, want := range cases {
		if got := isOrionLocation(location); got != want {
			t.Errorf("isOrionLocation(%q) = %v, want %v", location, got, want)
		}
	}
}

func TestResolveLocationAndMetadataWrapsStreamRelative(t *testing.T) {
	// No --service-url: the stream is still wrapped, and relative, so the
	// preset follows AfterTouch to a new address (issue 769).
	params := &presetParams{
		source:   "LOCAL_INTERNET_RADIO",
		location: "http://stream.example.org/live.mp3",
		name:     "Doc Radio",
		artwork:  "http://192.0.2.10/art.png",
	}
	if err := resolveLocationAndMetadata(params); err != nil {
		t.Fatalf("resolveLocationAndMetadata: %v", err)
	}

	if !strings.HasPrefix(params.location, "/station?data=") {
		t.Fatalf("location = %q, want relative /station?data=...", params.location)
	}
}

func TestResolveLocationAndMetadataIgnoresServiceURL(t *testing.T) {
	withURL := &presetParams{
		source:     "LOCAL_INTERNET_RADIO",
		location:   "http://stream.example.org/live.mp3",
		name:       "Doc Radio",
		artwork:    "http://192.0.2.10/art.png",
		serviceURL: "http://192.0.2.10:8000",
	}
	without := *withURL
	without.serviceURL = ""

	if err := resolveLocationAndMetadata(withURL); err != nil {
		t.Fatalf("resolveLocationAndMetadata: %v", err)
	}

	if err := resolveLocationAndMetadata(&without); err != nil {
		t.Fatalf("resolveLocationAndMetadata: %v", err)
	}

	if withURL.location != without.location || strings.Contains(withURL.location, "192.0.2.10:8000") {
		t.Fatalf("--service-url changed the location: %q vs %q", withURL.location, without.location)
	}
}

func TestResolveLocationAndMetadataKeepsOrionLocations(t *testing.T) {
	for _, location := range []string{
		"/station?data=eyJ9",
		"http://198.51.100.7/core02/svc-bmx-adapter-orion/prod/orion/station?data=eyJ9",
	} {
		params := &presetParams{source: "LOCAL_INTERNET_RADIO", location: location}
		if err := resolveLocationAndMetadata(params); err != nil {
			t.Fatalf("resolveLocationAndMetadata: %v", err)
		}

		if params.location != location {
			t.Errorf("Orion location %q rewritten to %q", location, params.location)
		}
	}
}
