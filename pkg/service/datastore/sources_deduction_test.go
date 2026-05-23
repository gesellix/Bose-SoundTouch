package datastore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfiguredSources_DeduceIDs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	account := "test-account"
	device := "test-device"

	// Create recents with specific source IDs for provider IDs
	// Let's create a manual Recents.xml and Presets.xml in the temp directory to simulate the state.
	deviceDir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	// Provider 11 = LOCAL_INTERNET_RADIO (active, present in initial sources)
	recentsXML := `<?xml version="1.0" encoding="UTF-8" ?>
<recents>
    <recent id="2184615630">
        <contentItemType></contentItemType>
        <createdOn>2017-02-07T11:22:00.000+00:00</createdOn>
        <lastplayedat>2017-05-17T13:18:57.000+00:00</lastplayedat>
        <location>52349</location>
        <name>Lounge FM Digital</name>
        <source id="9330201" type="Audio">
            <createdOn>2019-01-24T08:18:37.000+00:00</createdOn>
            <credential type="token"></credential>
            <name>9330201</name>
            <sourceproviderid>11</sourceproviderid>
            <sourcename></sourcename>
            <sourceSettings/>
            <updatedOn>2019-02-03T18:35:45.000+00:00</updatedOn>
            <username></username>
        </source>
        <sourceid>9330201</sourceid>
        <updatedOn>2017-05-17T17:18:58.000+00:00</updatedOn>
        <username>Lounge FM Digital</username>
    </recent>
</recents>`

	if err := os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte(recentsXML), 0644); err != nil {
		t.Fatalf("Failed to write Recents.xml: %v", err)
	}

	// Now call GetConfiguredSources and expect it to have "9330201" for provider ID "11"
	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	foundDeducted := false
	for _, s := range sources {
		if s.SourceProviderID == "11" {
			if s.ID == "9330201" {
				foundDeducted = true
			} else {
				t.Errorf("Expected source ID 9330201 for provider 11, got %s", s.ID)
			}
		}
	}

	if !foundDeducted {
		t.Errorf("Did not find source with provider ID 11 and deducted ID 9330201")
	}
}

func TestGetConfiguredSources_DeduceIDs_AllProviders(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datastore-test-all-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	account := "test-account"
	device := "test-device"

	deviceDir := ds.AccountDeviceDir(account, device)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("Failed to create device dir: %v", err)
	}

	// 9: AUX
	// 11: LOCAL_INTERNET_RADIO
	// 25: TUNEIN
	// INTERNET_RADIO (provider 2) is intentionally excluded from initial sources
	// and therefore not expected in the deduction result.
	presetsXML := `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
    <preset id="1">
        <contentItem source="AUX" sourceAccount="AUX" isPresetable="true" type="station" itemName="Station 9">
            <containerArt>http://example.com/art9.png</containerArt>
        </contentItem>
        <source id="ID9" type="Audio" sourceproviderid="9" />
        <sourceid>ID9</sourceid>
    </preset>
    <preset id="2">
        <contentItem source="LOCAL_INTERNET_RADIO" sourceAccount="" isPresetable="true" type="station" itemName="Station 11">
            <containerArt>http://example.com/art11.png</containerArt>
        </contentItem>
        <source id="ID11" type="Audio" sourceproviderid="11" />
        <sourceid>ID11</sourceid>
    </preset>
    <preset id="3">
        <contentItem source="TUNEIN" sourceAccount="" isPresetable="true" type="station" itemName="Station 25">
            <containerArt>http://example.com/art25.png</containerArt>
        </contentItem>
        <source id="ID25" type="Audio" sourceproviderid="25" />
        <sourceid>ID25</sourceid>
    </preset>
</presets>`

	if err := os.WriteFile(filepath.Join(deviceDir, "Presets.xml"), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("Failed to write Presets.xml: %v", err)
	}

	sources, err := ds.GetConfiguredSources(account, device)
	if err != nil {
		t.Fatalf("GetConfiguredSources failed: %v", err)
	}

	expected := map[string]string{
		"9":  "ID9",
		"11": "ID11",
		"25": "ID25",
	}

	found := make(map[string]bool)
	for _, s := range sources {
		if expID, ok := expected[s.SourceProviderID]; ok {
			if s.ID != expID {
				t.Errorf("Expected source ID %s for provider %s, got %s", expID, s.SourceProviderID, s.ID)
			}
			found[s.SourceProviderID] = true
		} else if s.SourceKeyType == "AUX" && s.SourceProviderID == "" {
			// Special case for AUX if it doesn't have provider ID 9 by default
			if expID, ok := expected["9"]; ok {
				if s.ID != expID {
					t.Errorf("Expected source ID %s for AUX, got %s", expID, s.ID)
				}
				found["9"] = true
			}
		}
	}

	for pid := range expected {
		if !found[pid] {
			t.Errorf("Did not find source with provider ID %s", pid)
		}
	}
}
