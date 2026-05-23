package datastore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGetPresets_CapitalCContentItem checks that Presets.xml files written by
// older AfterTouch versions (or verbatim speaker XML) using <ContentItem>
// (capital C) are parsed correctly. encoding/xml is case-sensitive, so without
// the normalisation in GetPresets the source and location fields would be empty
// and /full would silently skip all presets for the device (i218 diagnostic).
func TestGetPresets_CapitalCContentItem(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-capital-c-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "7961999"
	device := "304511B46CBC"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Verbatim format from the i218 diagnostic: capital-C ContentItem, no
	// <sourceid> child element, only the first preset has createdOn/updatedOn.
	presetsXML := `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
    <preset id="1" createdOn="1778969808" updatedOn="1778969808">
        <ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://192.168.1.11/OPB.json" sourceAccount="" isPresetable="true">
            <itemName>Internet Radio</itemName>
        </ContentItem>
    </preset>
    <preset id="2">
        <ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://192.168.1.11/AllClassicalPortland.json" sourceAccount="" isPresetable="true">
            <itemName>Internet Radio</itemName>
        </ContentItem>
    </preset>
    <preset id="3">
        <ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://192.168.1.11/AncientFM.json" sourceAccount="" isPresetable="true">
            <itemName>Internet Radio</itemName>
        </ContentItem>
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

	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d (capital-C ContentItem not parsed)", len(presets))
	}

	for i, p := range presets {
		if p.Source != "LOCAL_INTERNET_RADIO" {
			t.Errorf("preset %d: expected Source=LOCAL_INTERNET_RADIO, got %q", i+1, p.Source)
		}
		if p.Location == "" {
			t.Errorf("preset %d: Location is empty — ContentItem attributes not parsed", i+1)
		}
		if p.Name != "Internet Radio" {
			t.Errorf("preset %d: expected Name=Internet Radio, got %q", i+1, p.Name)
		}
	}

	if presets[0].CreatedOn != "1778969808" {
		t.Errorf("preset 1: expected CreatedOn=1778969808, got %q", presets[0].CreatedOn)
	}
}

// TestGetPresets_CapitalCRewritesFile checks that after GetPresets detects the
// legacy <ContentItem> format it rewrites Presets.xml in canonical lowercase
// form, so subsequent reads are clean without needing the compat shim.
func TestGetPresets_CapitalCRewritesFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-capital-c-rewrite-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	account := "7961999"
	device := "304511B46CBC"

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	presetsPath := filepath.Join(deviceDir, "Presets.xml")
	presetsXML := `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
    <preset id="1" createdOn="1778969808" updatedOn="1778969808">
        <ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://192.168.1.11/OPB.json" sourceAccount="" isPresetable="true">
            <itemName>Internet Radio</itemName>
        </ContentItem>
    </preset>
</presets>`
	if err := os.WriteFile(presetsPath, []byte(presetsXML), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}

	ds := NewDataStore(tempDir)

	if _, err := ds.GetPresets(account, device); err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	rewritten, err := os.ReadFile(presetsPath)
	if err != nil {
		t.Fatalf("read rewritten Presets.xml: %v", err)
	}

	if strings.Contains(string(rewritten), "<ContentItem") {
		t.Errorf("Presets.xml still contains <ContentItem> after auto-rewrite:\n%s", string(rewritten))
	}

	if !strings.Contains(string(rewritten), "<contentItem") {
		t.Errorf("Presets.xml missing canonical <contentItem> after auto-rewrite:\n%s", string(rewritten))
	}
}
