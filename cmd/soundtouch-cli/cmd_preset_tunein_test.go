package main

import (
	"errors"
	"testing"
)

func TestTuneInGuideID(t *testing.T) {
	cases := map[string]string{
		"/v1/playback/station/s1217":     "s1217",
		"/v1/playback/episode/e789012":   "e789012",
		"/v1/playback/episodes/p123456":  "p123456",
		"/v1/playback/station/":          "",
		"/v1/playback/station/s1/extra":  "",
		"https://tunein.com/radio/s1217": "",
		"":                               "",
	}

	for location, want := range cases {
		if got := tuneInGuideID(location); got != want {
			t.Errorf("tuneInGuideID(%q) = %q, want %q", location, got, want)
		}
	}
}

// stubTuneInDescribe replaces the describe lookup for one test and records the
// guide IDs it was asked for.
func stubTuneInDescribe(t *testing.T, name, logo string, err error) *[]string {
	t.Helper()

	var calls []string

	original := tuneInDescribe
	tuneInDescribe = func(id string) (string, string, error) {
		calls = append(calls, id)

		return name, logo, err
	}

	t.Cleanup(func() { tuneInDescribe = original })

	return &calls
}

func TestResolveLocationAndMetadataLooksUpBareTuneInLocation(t *testing.T) {
	calls := stubTuneInDescribe(t, "RMF FM", "https://cdn-profiles.tunein.com/s1217/images/logoq.png", nil)

	params := &presetParams{source: "TUNEIN", location: "/v1/playback/station/s1217"}
	if err := resolveLocationAndMetadata(params); err != nil {
		t.Fatalf("resolveLocationAndMetadata: %v", err)
	}

	if len(*calls) != 1 || (*calls)[0] != "s1217" {
		t.Fatalf("describe lookups = %v, want [s1217]", *calls)
	}

	if params.name != "RMF FM" || params.artwork != "https://cdn-profiles.tunein.com/s1217/images/logoq.png" {
		t.Errorf("params name=%q artwork=%q, want the looked-up values", params.name, params.artwork)
	}
}

func TestResolveLocationAndMetadataKeepsExplicitValues(t *testing.T) {
	stubTuneInDescribe(t, "Looked Up", "https://example.com/looked-up.png", nil)

	params := &presetParams{source: "TUNEIN", location: "/v1/playback/station/s1217", name: "My Name"}
	if err := resolveLocationAndMetadata(params); err != nil {
		t.Fatalf("resolveLocationAndMetadata: %v", err)
	}

	if params.name != "My Name" {
		t.Errorf("name = %q, want the explicit value kept", params.name)
	}

	if params.artwork != "https://example.com/looked-up.png" {
		t.Errorf("artwork = %q, want the missing value filled in", params.artwork)
	}
}

func TestResolveLocationAndMetadataSkipsLookupWhenComplete(t *testing.T) {
	calls := stubTuneInDescribe(t, "unused", "unused", nil)

	params := &presetParams{source: "TUNEIN", location: "/v1/playback/station/s1217", name: "N", artwork: "A"}
	if err := resolveLocationAndMetadata(params); err != nil {
		t.Fatalf("resolveLocationAndMetadata: %v", err)
	}

	if len(*calls) != 0 {
		t.Errorf("describe lookups = %v, want none when name and artwork are given", *calls)
	}
}

func TestResolveLocationAndMetadataToleratesLookupFailure(t *testing.T) {
	stubTuneInDescribe(t, "", "", errors.New("tunein unreachable"))

	params := &presetParams{source: "TUNEIN", location: "/v1/playback/station/s1217"}
	if err := resolveLocationAndMetadata(params); err != nil {
		t.Fatalf("a failed lookup must not fail the preset store: %v", err)
	}

	if params.name != "" || params.artwork != "" {
		t.Errorf("params name=%q artwork=%q, want both left empty", params.name, params.artwork)
	}
}
