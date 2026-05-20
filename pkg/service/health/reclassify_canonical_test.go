package health

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestReclassifyCanonicalSourceIDs_GH343 reproduces the situation from
// issue #343: a device's Sources.xml has built-in radio sources at
// non-canonical "fallback" IDs (2000003/2000004/2000008), and Presets.xml
// has presets bound to those IDs by <sourceid>. The QuickFix rewrites
// the source IDs to canonical values and updates the references in
// Presets.xml/Recents.xml atomically; running it twice is a no-op.
func TestReclassifyCanonicalSourceIDs_GH343(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "health-reclassify-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "1111111"
	device := "AABBCCDDEEFF"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Mirrors og-gh's Sources.xml from issue #343: built-in radio
	// sources at fallback IDs.
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="2000003" secret="" secretType="token" type="Audio" sourceproviderid="2">
        <credential type=""></credential>
        <sourceKey type="INTERNET_RADIO" account=""></sourceKey>
    </source>
    <source id="2000004" secret="aaa" secretType="token" type="Audio" sourceproviderid="11">
        <credential type="token">aaa</credential>
        <sourceKey type="LOCAL_INTERNET_RADIO" account=""></sourceKey>
    </source>
    <source id="2000008" secret="bbb" secretType="token" type="Audio" sourceproviderid="25">
        <credential type="token">bbb</credential>
        <sourceKey type="TUNEIN" account=""></sourceKey>
    </source>
    <source id="10005" secret="" secretType="token" type="Audio" sourceproviderid="39">
        <credential type=""></credential>
        <sourceKey type="RADIO_BROWSER" account=""></sourceKey>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	// Presets bound by ID to the non-canonical TuneIn (2000008) and
	// LocalInternetRadio (2000004) sources.
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1" createdOn="0" updatedOn="0">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s6634">
            <itemName>MDR JUMP</itemName>
        </contentItem>
        <sourceid>2000008</sourceid>
    </preset>
    <preset id="5" createdOn="0" updatedOn="0">
        <contentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://example/custom/v1/playback/abc">
            <itemName>laut.fm</itemName>
        </contentItem>
        <sourceid>2000004</sourceid>
    </preset>
</presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}

	// And a recent bound to TuneIn by ID.
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="rec-1">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s6634">
            <itemName>MDR JUMP</itemName>
        </contentItem>
        <sourceid>2000008</sourceid>
    </recent>
</recents>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644); err != nil {
		t.Fatalf("write Recents.xml: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	msg, err := reclassifyCanonicalSourceIDs(ds, Target{Account: account, Device: device})
	if err != nil {
		t.Fatalf("reclassifyCanonicalSourceIDs: %v", err)
	}

	if !strings.Contains(msg, "Re-classified 3") {
		t.Errorf("success message should report 3 re-classifications, got %q", msg)
	}

	// Sources.xml: IDs rewritten to canonical.
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("re-read Sources.xml: %v", err)
	}

	want := map[string]string{
		"INTERNET_RADIO":       "10002",
		"LOCAL_INTERNET_RADIO": "10003",
		"TUNEIN":               "10004",
		"RADIO_BROWSER":        "10005",
	}

	for _, s := range sources {
		if expected, ok := want[s.SourceKeyType]; ok && s.ID != expected {
			t.Errorf("%s: expected id %s, got %s", s.SourceKeyType, expected, s.ID)
		}
	}

	// Presets.xml: <sourceid> references rewritten.
	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("re-read Presets.xml: %v", err)
	}

	for _, p := range presets {
		switch p.Source {
		case "TUNEIN":
			if p.SourceID != "10004" {
				t.Errorf("TUNEIN preset slot %s: expected sourceid=10004, got %q", p.ButtonNumber, p.SourceID)
			}
		case "LOCAL_INTERNET_RADIO":
			if p.SourceID != "10003" {
				t.Errorf("LIR preset slot %s: expected sourceid=10003, got %q", p.ButtonNumber, p.SourceID)
			}
		}
	}

	// Recents.xml: same.
	recents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("re-read Recents.xml: %v", err)
	}

	for _, r := range recents {
		if r.Source == "TUNEIN" && r.SourceID != "10004" {
			t.Errorf("TUNEIN recent: expected sourceid=10004, got %q", r.SourceID)
		}
	}

	// Idempotency: a second run should be a no-op.
	msg2, err := reclassifyCanonicalSourceIDs(ds, Target{Account: account, Device: device})
	if err != nil {
		t.Fatalf("second reclassify: %v", err)
	}

	if !strings.Contains(msg2, "Nothing to do") {
		t.Errorf("second run should be a no-op, got %q", msg2)
	}
}

// TestReclassifyCanonicalSourceIDs_SkipsCollision verifies the
// collision-avoidance guard: if the canonical ID is already in use by
// another source (e.g. operator hand-edited and now has two TUNEIN
// entries, one at 10004 and one at 2000008), we leave both alone
// rather than create an ID conflict.
func TestReclassifyCanonicalSourceIDs_SkipsCollision(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "health-reclassify-collision-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "1111111"
	device := "AABBCCDDEEFF"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="10004" secret="aaa" secretType="token" type="Audio" sourceproviderid="25">
        <credential type="token">aaa</credential>
        <sourceKey type="TUNEIN" account="acct-A"></sourceKey>
    </source>
    <source id="2000008" secret="bbb" secretType="token" type="Audio" sourceproviderid="25">
        <credential type="token">bbb</credential>
        <sourceKey type="TUNEIN" account="acct-B"></sourceKey>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	msg, err := reclassifyCanonicalSourceIDs(ds, Target{Account: account, Device: device})
	if err != nil {
		t.Fatalf("reclassifyCanonicalSourceIDs: %v", err)
	}

	if !strings.Contains(msg, "Nothing to do") {
		t.Errorf("expected no-op when canonical collides; got %q", msg)
	}
}
