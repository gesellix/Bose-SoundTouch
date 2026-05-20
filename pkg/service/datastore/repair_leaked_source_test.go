package datastore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetPresets_RepairsAudioLeakViaSourceID exercises the on-read repair:
// a persisted preset whose Source is the protocol-level "Audio" leak gets
// resolved to the speaker-perspective SourceKeyType through the device's
// Sources.xml. Speaker is the source of truth; this just un-rots data the
// legacy syncPresets path wrote.
func TestGetPresets_RepairsAudioLeakViaSourceID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-repair-leak-*")
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

	// Sources.xml claims id=14774275 is a TUNEIN source.
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="14774275">
        <sourceKey type="TUNEIN" account=""/>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	// Presets.xml carries the leak: source="Audio" + sourceid=14774275.
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="4" createdOn="0" updatedOn="0">
        <contentItem source="Audio" type="stationurl" location="/v1/playback/station/s166521" sourceAccount="" isPresetable="true">
            <itemName>SMOOTH JAZZ</itemName>
            <containerArt></containerArt>
        </contentItem>
        <sourceid>14774275</sourceid>
    </preset>
</presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}

	ds := NewDataStore(tempDir)

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}

	if presets[0].Source != "TUNEIN" {
		t.Errorf("expected repaired Source=TUNEIN, got %q", presets[0].Source)
	}
}

// TestGetPresets_PreservesNonLeakedSource is the GH-343 protection: a
// preset persisted with Source=TUNEIN must stay TUNEIN even when
// Sources.xml has been re-classified to RADIOPLAYER. The speaker's
// previously-stored intent wins over a stale current Sources.xml entry.
func TestGetPresets_PreservesNonLeakedSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-preserve-source-*")
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

	// Sources.xml *currently* says id=14774275 is RADIOPLAYER (stale /
	// drifted classification). The preset was stored earlier when the
	// same id was understood as TUNEIN.
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="14774275">
        <sourceKey type="RADIOPLAYER" account=""/>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="4" createdOn="0" updatedOn="0">
        <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s166521" sourceAccount="" isPresetable="true">
            <itemName>SMOOTH JAZZ</itemName>
        </contentItem>
        <sourceid>14774275</sourceid>
    </preset>
</presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}

	ds := NewDataStore(tempDir)

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if presets[0].Source != "TUNEIN" {
		t.Errorf("expected speaker's previously-stored Source=TUNEIN to be preserved, got %q (GH-343 silent rewrite would substitute RADIOPLAYER)", presets[0].Source)
	}
}

// TestGetRecents_RepairsAudioLeakViaSourceID applies the same load-time
// repair to recents — symmetric protection for the recents pipeline.
func TestGetRecents_RepairsAudioLeakViaSourceID(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-repair-recent-*")
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

	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source id="9330201">
        <sourceKey type="INTERNET_RADIO" account=""/>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("write Sources.xml: %v", err)
	}

	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="rec-1">
        <contentItem source="Audio" type="stationurl" location="19059" sourceAccount="" isPresetable="true">
            <itemName>Russkoe Radio Ukraine</itemName>
        </contentItem>
        <sourceid>9330201</sourceid>
    </recent>
</recents>`
	if err := os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644); err != nil {
		t.Fatalf("write Recents.xml: %v", err)
	}

	ds := NewDataStore(tempDir)

	recents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("GetRecents: %v", err)
	}

	if len(recents) != 1 {
		t.Fatalf("expected 1 recent, got %d", len(recents))
	}

	if recents[0].Source != "INTERNET_RADIO" {
		t.Errorf("expected repaired Source=INTERNET_RADIO, got %q", recents[0].Source)
	}
}
