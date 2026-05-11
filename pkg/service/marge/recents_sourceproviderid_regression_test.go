package marge

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestAccountFullToXML_RecentWithPoisonedSourceProviderID is a regression test
// for the production failure where the speaker's BoseApp rejected the
// /streaming/account/.../full response with:
//
//	protobuf::FatalException - CHECK failed: IsInitialized():
//	  Message of type "MargePB.account" is missing required fields:
//	    devices.device[1].recents.recent[0].source.sourceproviderid
//
// Trigger sequence reproduced here:
//
//  1. The device POSTs a "laut.fm" recent (location "/custom/v1/playback/...")
//     against an account that has no Sources.xml yet.
//  2. classifyLearnedSource fails to recognise /custom/v1/playback/ and the
//     numeric source id "10003", and historically wrote sourceKey type="INVALID"
//     with an empty sourceproviderid.
//  3. The persisted Sources.xml then re-appears in /full with an empty
//     <sourceproviderid> element inside recents>recent>source, which the
//     post-marshal cleanup stripped entirely — making the speaker's protobuf
//     decode fail on a required field.
//
// The fix combines three things, all exercised below:
//
//   - classifyLearnedSource recognises LocalInternetRadio via the
//     /custom/v1/playback/ URL pattern and via sourceProviderID == "11".
//   - mapToFullResponseSource falls back to the canonical SourceProviderID
//     keyed by source ID, so already-poisoned data on disk still renders a
//     non-empty providerid.
//   - AccountFullToXML no longer strips empty <sourceproviderid> elements
//     inside recents/preset source blocks.
func TestAccountFullToXML_RecentWithPoisonedSourceProviderID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-recent-provid-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "1234567"
	device := "ABCDEF012345"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(deviceDir, "DeviceInfo.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="ABCDEF012345">
    <name>Kitchen</name>
    <type>SoundTouch</type>
    <moduleType>10 sm2</moduleType>
</info>`), 0644); err != nil {
		t.Fatalf("Failed to write DeviceInfo.xml: %v", err)
	}

	// Sources.xml reproduces the poisoned entry observed in the user's
	// backup (May 11): id="10003" with sourceKey type="INVALID" and no
	// sourceproviderid attribute. Older repair paths (applyCanonicalDefaults,
	// ensureSourceProviderID) all key off sourceKey.type, so the entry stays
	// broken at load time.
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="10003" secret="" secretType="">
        <credential type=""></credential>
        <sourceKey type="INVALID" account=""></sourceKey>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("Failed to write Sources.xml: %v", err)
	}

	// Recents.xml references the poisoned source via <sourceid>10003</sourceid>.
	// The location is a laut.fm stream proxied through /custom/v1/playback/ —
	// exactly the URL pattern the old classifier failed to recognise.
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent deviceID="ABCDEF012345" utcTime="1778014606" id="260505002">
        <contentItem source="INVALID" type="stationurl" location="http://192.168.123.123/custom/v1/playback/aHR0cHM6Ly9zdHJlYW0ubGF1dC5mbS9zbW9vdGgtamF6eg==" sourceAccount="" isPresetable="true">
            <itemName>Smooth Jazz Instrumental 24/7</itemName>
        </contentItem>
        <createdOn>2026-05-05T20:56:49.305+00:00</createdOn>
        <updatedOn>2026-05-05T20:56:49.305+00:00</updatedOn>
        <sourceid>10003</sourceid>
    </recent>
</recents>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644); err != nil {
		t.Fatalf("Failed to write Recents.xml: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	fullXML, err := AccountFullToXML(ds, account)
	if err != nil {
		t.Fatalf("AccountFullToXML failed: %v", err)
	}

	body := string(fullXML)

	// Locate the recents block and assert every <source> inside it carries a
	// non-empty <sourceproviderid>. Without the fix, the post-marshal
	// strip-empty step deletes the empty element and the speaker rejects
	// the message with "missing required field".
	recentsRE := regexp.MustCompile(`(?s)<recents>(.*?)</recents>`)
	matches := recentsRE.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		t.Fatalf("Expected at least one <recents> block; body:\n%s", body)
	}

	sourceInRecentRE := regexp.MustCompile(`(?s)<source(?:\s[^>]*)?>(.*?)</source>`)

	for _, recentsBlock := range matches {
		for _, src := range sourceInRecentRE.FindAllStringSubmatch(recentsBlock[1], -1) {
			inner := src[1]
			if !strings.Contains(inner, "<sourceproviderid>") {
				t.Errorf("<source> inside <recents> has no <sourceproviderid> element; block:\n%s", src[0])
				continue
			}

			if strings.Contains(inner, "<sourceproviderid></sourceproviderid>") {
				t.Errorf("<source> inside <recents> has empty <sourceproviderid>; block:\n%s", src[0])
			}
		}
	}

	// And spot-check the canonical fallback fired for the laut.fm recent.
	if !strings.Contains(body, "<sourceproviderid>11</sourceproviderid>") {
		t.Errorf("Expected <sourceproviderid>11</sourceproviderid> (LocalInternetRadio) in /full; body:\n%s", body)
	}
}

// TestClassifyLearnedSource_LocalInternetRadioCustomPlayback locks in the
// classifier behaviour: a recent POSTed with a /custom/v1/playback/ URL must
// classify as LocalInternetRadio. Previously this fell into the "INVALID"
// default and poisoned Sources.xml — see the regression test above.
func TestClassifyLearnedSource_LocalInternetRadioCustomPlayback(t *testing.T) {
	cases := []struct {
		name             string
		sourceID         string
		location         string
		sourceProviderID string
	}{
		{
			name:     "laut.fm /custom/v1/playback URL",
			sourceID: "10003",
			location: "http://192.168.123.123/custom/v1/playback/aHR0cHM6Ly9zdHJlYW0ubGF1dC5mbS9zbW9vdGgtamF6eg==",
		},
		{
			name:             "sourceProviderID==11 alone",
			sourceID:         "999999",
			location:         "http://example.invalid/whatever",
			sourceProviderID: "11",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := createLearnedSource(tc.sourceID, tc.location, "", "", tc.sourceProviderID, "", "")

			if src.SourceKey.Type == "INVALID" || src.SourceKeyType == "INVALID" {
				t.Errorf("classifier wrote INVALID for %s; src=%+v", tc.name, src)
			}

			if src.SourceKey.Type != "LOCAL_INTERNET_RADIO" {
				t.Errorf("expected SourceKey.Type=LOCAL_INTERNET_RADIO, got %q", src.SourceKey.Type)
			}
		})
	}
}

// TestClassifyLearnedSource_UnknownLeavesKeyEmpty verifies the new default
// branch leaves SourceKey.Type empty instead of writing the literal "INVALID"
// sentinel that locks the source out of every downstream repair path.
func TestClassifyLearnedSource_UnknownLeavesKeyEmpty(t *testing.T) {
	src := createLearnedSource("SOMETHING_UNKNOWN", "http://example.invalid/nothing", "", "", "", "", "")

	if src.SourceKey.Type == "INVALID" || src.SourceKeyType == "INVALID" {
		t.Errorf("classifier still writes INVALID sentinel; src=%+v", src)
	}

	if src.SourceKey.Type != "" {
		t.Errorf("expected SourceKey.Type empty for an unrecognised source, got %q", src.SourceKey.Type)
	}
}
