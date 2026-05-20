package marge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestUpdatePreset_AutoAddsCanonicalSource is a regression test for GH-314 /
// GH-253: when a speaker long-presses to store a preset whose source is a
// well-known built-in (TuneIn / InternetRadio / LocalInternetRadio /
// RadioBrowser) that AfterTouch's per-device Sources.xml doesn't list yet
// (typical after a factory reset where the speaker locally knows TuneIn but
// AfterTouch hasn't seen it played yet), UpdatePreset used to return
// "invalid account/source" with a 500. The speaker's long-press appeared to
// succeed but the preset was never persisted; the next /full sync wiped the
// speaker's local copy.
//
// The fix auto-adds the canonical source from the same template post-pair
// would have used, then lets the preset land normally.
func TestUpdatePreset_AutoAddsCanonicalSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-update-preset-autoadd-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "1234567"
	device := "AABBCCDDEEFF"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Empty Sources.xml — represents the post-factory-reset state where
	// AfterTouch's per-device source list doesn't yet contain TuneIn.
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"),
		[]byte(`<?xml version="1.0" encoding="UTF-8"?><sources></sources>`), 0644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	// PUT body the speaker would send for a long-pressed TuneIn preset.
	putXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<preset>
    <name>SMOOTH JAZZ</name>
    <sourceid>10004</sourceid>
    <location>/v1/playback/station/s166521</location>
    <contentItemType>stationurl</contentItemType>
    <containerArt>https://cdn-profiles.tunein.com/s166521/images/logod.png</containerArt>
</preset>`)

	resp, err := UpdatePreset(ds, account, device, 3, putXML)
	if err != nil {
		t.Fatalf("UpdatePreset returned error: %v (expected auto-add to succeed)", err)
	}

	if !strings.Contains(string(resp), `buttonNumber="3"`) {
		t.Errorf("response XML missing buttonNumber=3:\n%s", string(resp))
	}

	// The canonical TuneIn source should now be in the device's sources.
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources after UpdatePreset: %v", err)
	}

	var foundTunein bool
	for _, s := range sources {
		if s.ID == "10004" {
			foundTunein = true

			if s.SourceProviderID != "25" {
				t.Errorf("auto-added TuneIn: expected sourceproviderid=25, got %q", s.SourceProviderID)
			}

			break
		}
	}

	if !foundTunein {
		t.Errorf("expected canonical TuneIn (id=10004) auto-added to configured sources after UpdatePreset; got %d sources", len(sources))
	}

	// And the preset itself should be on disk.
	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(presets) < 3 {
		t.Fatalf("expected at least 3 presets stored after UpdatePreset(slot 3), got %d", len(presets))
	}

	if presets[2].Name != "SMOOTH JAZZ" {
		t.Errorf("preset 3: expected name SMOOTH JAZZ, got %q", presets[2].Name)
	}
}

// TestUpdatePreset_RejectsNonCanonicalUnknownSource verifies the auto-add is
// scoped to the built-in canonical IDs. Account-bound sources (Spotify,
// Amazon) and arbitrary numeric IDs can't be fabricated without losing
// per-account state (credentials), so the request must still fail visibly
// instead of silently writing a broken source.
func TestUpdatePreset_RejectsNonCanonicalUnknownSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-update-preset-reject-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "1234567"
	device := "AABBCCDDEEFF"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"),
		[]byte(`<?xml version="1.0" encoding="UTF-8"?><sources></sources>`), 0644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	// A Spotify-style account-bound source id we cannot reconstruct.
	putXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<preset>
    <name>A Playlist</name>
    <sourceid>100004</sourceid>
    <location>/playback/container/abc</location>
    <contentItemType>tracklisturl</contentItemType>
</preset>`)

	_, err = UpdatePreset(ds, account, device, 2, putXML)
	if err == nil {
		t.Fatalf("expected UpdatePreset to reject sourceid=100004 (non-canonical, account-bound), got no error")
	}

	if !strings.Contains(err.Error(), "invalid account/source") {
		t.Errorf("expected 'invalid account/source' error, got: %v", err)
	}
}

// TestUpdatePreset_AcceptsStockholmUsername covers the Stockholm app's PUT
// shape: the mobile app sends the preset's human-readable name in
// <username> rather than <name>. soundcork documents the same divergence.
// The fix accepts both and prefers <name>.
func TestUpdatePreset_AcceptsStockholmUsername(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "marge-update-preset-stockholm-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "1234567"
	device := "AABBCCDDEEFF"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	putXML := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<preset>
    <username>Stockholm-styled Name</username>
    <sourceid>10004</sourceid>
    <location>/v1/playback/station/s166521</location>
    <contentItemType>stationurl</contentItemType>
</preset>`)

	if _, err := UpdatePreset(ds, account, device, 1, putXML); err != nil {
		t.Fatalf("UpdatePreset returned error: %v", err)
	}

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(presets) == 0 {
		t.Fatalf("expected one preset stored, got 0")
	}

	if presets[0].Name != "Stockholm-styled Name" {
		t.Errorf("expected preset name from <username>, got %q", presets[0].Name)
	}
}
