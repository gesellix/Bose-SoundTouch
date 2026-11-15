package datastore

import (
	"os"
	"testing"
)

// TestGetPresetsAndRecentsRepairNumericSource covers presets and recents
// stored while Sources.xml held a media server under type "7" (issue 697).
// Saving copied that type into the row, so the stored preset said source "7"
// where the speaker says "STORED_MUSIC", and the player reported the slot as
// different although it holds the same content. Reading must name the
// provider again; an unknown number stays as it is.
func TestGetPresetsAndRecentsRepairNumericSource(t *testing.T) {
	const (
		account = "0000000"
		device  = "DEVICEID01"
		udn     = "00000000-0000-0000-0000-0000000000aa/0"
	)

	ds := NewDataStore(t.TempDir())

	dir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="4" createdOn="0" updatedOn="0">
        <contentItem source="7" location="1" sourceAccount="` + udn + `" isPresetable="true">
            <itemName>Library item</itemName>
        </contentItem>
        <sourceid>SRC_media</sourceid>
    </preset>
    <preset id="5" createdOn="0" updatedOn="0">
        <contentItem source="999" location="x" sourceAccount="someone" isPresetable="true">
            <itemName>Unknown provider</itemName>
        </contentItem>
    </preset>
</presets>`

	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="1">
        <contentItem source="7" location="1" sourceAccount="` + udn + `" isPresetable="true">
            <itemName>Library item</itemName>
        </contentItem>
        <sourceid>SRC_media</sourceid>
    </recent>
</recents>`

	for name, body := range map[string]string{"Presets.xml": presetsXML, "Recents.xml": recentsXML} {
		if err := os.WriteFile(dir+"/"+name, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	got := map[string]string{}
	for i := range presets {
		got[presets[i].ID] = presets[i].Source
	}

	if got["4"] != "STORED_MUSIC" {
		t.Errorf("preset 4 source = %q, want STORED_MUSIC", got["4"])
	}

	if got["5"] != "999" {
		t.Errorf("preset 5 source = %q, want 999 (unknown provider stays)", got["5"])
	}

	recents, err := ds.GetRecents(account, device)
	if err != nil {
		t.Fatalf("GetRecents: %v", err)
	}

	if len(recents) != 1 || recents[0].Source != "STORED_MUSIC" {
		t.Errorf("recent source = %+v, want one recent with STORED_MUSIC", recents)
	}
}
