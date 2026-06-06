package marge

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestReadSourcesWithEmptyDisplayName(t *testing.T) {
	tempBaseDir := "repro_sources_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	accountID := "1234567"
	deviceID := "001122334455"

	// Create device directory
	devDir := filepath.Join(tempBaseDir, "accounts", accountID, "devices", deviceID)
	err = os.MkdirAll(devDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create DeviceInfo.xml so ListAllDevices finds it
	devInfo := `<info deviceID="` + deviceID + `"><name>Test Device</name></info>`
	os.WriteFile(filepath.Join(devDir, "DeviceInfo.xml"), []byte(devInfo), 0644)

	// Create Sources.xml with some empty displayNames (as provided in the issue)
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source displayName="AUX IN" id="" secret="" secretType="">
        <sourceKey type="AUX" account="AUX"></sourceKey>
    </source>
    <source displayName="" id="" secret="" secretType="token">
        <sourceKey type="INTERNET_RADIO" account=""></sourceKey>
    </source>
    <source displayName="" id="" secret="S1" secretType="token">
        <sourceKey type="LOCAL_INTERNET_RADIO" account=""></sourceKey>
    </source>
    <source displayName="test-user+spotify@gmail.com" id="" secret="S2" secretType="token">
        <sourceKey type="SPOTIFY" account="test-user"></sourceKey>
    </source>
    <source displayName="" id="" secret="S3" secretType="token">
        <sourceKey type="TUNEIN" account=""></sourceKey>
    </source>
</sources>`
	os.WriteFile(filepath.Join(devDir, "Sources.xml"), []byte(sourcesXML), 0644)

	ds := datastore.NewDataStore(tempBaseDir)
	sources, err := ds.GetConfiguredSources(accountID, deviceID)
	if err != nil {
		t.Fatalf("Failed to get configured sources: %v", err)
	}

	if len(sources) != 5 {
		t.Errorf("Expected 5 sources, got %d", len(sources))
	}

	for i, s := range sources {
		t.Logf("Source %d: ID=%s, DisplayName=%s, Type=%s, SourceKeyType=%s, Account=%s", i, s.ID, s.DisplayName, s.Type, s.SourceKeyType, s.SourceKey.Account)
		if s.SourceKeyType == "" {
			t.Errorf("Source %d (%s) has empty SourceKeyType", i, s.DisplayName)
		}

		if s.Type == "SPOTIFY" && s.SourceKey.Account != "test-user" {
			t.Errorf("Source %d (%s) expected account 'test-user', got '%s'", i, s.DisplayName, s.SourceKey.Account)
		}

		label := constants.GetSourceLabel(s.Type)
		t.Logf("  Label: %s", label)
		if label == "" && s.Type != "" {
			t.Errorf("Source %d (%s) has empty label for type %s", i, s.DisplayName, s.Type)
		}
	}
}

func TestReproduceMissingName(t *testing.T) {
	tempBaseDir := "repro_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	accountID := "1234567"

	// Create device folders
	// AABBCCDDEE0A (has name)
	// 001122334455 (missing name in full_local.xml)

	// Device 1: AABBCCDDEE0A
	dev1Dir := filepath.Join(tempBaseDir, "accounts", accountID, "devices", "AABBCCDDEE0A")
	err = os.MkdirAll(dev1Dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	dev1Info := `<info deviceID="AABBCCDDEE0A">
    <name>Kitchen SoundTouch</name>
    <type>SoundTouch</type>
    <moduleType>20</moduleType>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
            <serialNumber>K4245112804625125000710</serialNumber>
        </component>
        <component>
            <componentCategory>PackagedProduct</componentCategory>
            <serialNumber>066802942560222AE</serialNumber>
        </component>
    </components>
</info>`
	os.WriteFile(filepath.Join(dev1Dir, "DeviceInfo.xml"), []byte(dev1Info), 0644)

	// Device 2: 001122334455 - MAC address ID in XML, name with special char or space?
	dev2Dir := filepath.Join(tempBaseDir, "accounts", accountID, "devices", "001122334455")
	err = os.MkdirAll(dev2Dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	dev2Info := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="001122334455">
    <name>Living Room SoundTouch</name>
    <type>SoundTouch</type>
    <moduleType>10 sm2</moduleType>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
            <serialNumber>I6332527703739342000020</serialNumber>
        </component>
        <component>
            <componentCategory>PackagedProduct</componentCategory>
            <serialNumber>069231P63364828AE</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <ipAddress>192.0.2.35</ipAddress>
        <macAddress>001122334455</macAddress>
    </networkInfo>
    <discoveryMethod>sync_full</discoveryMethod>
</info>`
	os.WriteFile(filepath.Join(dev2Dir, "DeviceInfo.xml"), []byte(dev2Info), 0644)

	// In the backup, there is NO default entry with empty name for this device's serial.
	// But let's see what happens if we use the EXACT content from the backup.
	// I'll also add a test case that unmarshals the exact backup file content.

	ds := datastore.NewDataStore(tempBaseDir)
	err = ds.Initialize()
	if err != nil {
		t.Fatal(err)
	}

	// Generate /full response XML
	data, err := AccountFullToXML(ds, accountID)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Resulting XML:\n%s\n", string(data))

	var resp models.AccountFullResponse
	err = xml.Unmarshal(data, &resp)
	if err != nil {
		t.Fatal(err)
	}

	// Now test name preservation during sync
	// Mock a response with empty name for 001122334455
	for i := range resp.Devices {
		if resp.Devices[i].DeviceID == "001122334455" {
			resp.Devices[i].Name = ""
		}
	}

	// Remove the account-specific device directory to force resolution to 'default'
	os.RemoveAll(filepath.Join(tempBaseDir, "accounts", accountID, "devices", "001122334455"))

	// Create a duplicate directory in another place (e.g. 'st-go/data/accounts/default') with the CORRECT name
	// This simulates a global entry that ds.ListAllDevices() should find
	globalDevDir := filepath.Join("st-go", "data", "accounts", "default", "devices", "001122334455")
	os.MkdirAll(globalDevDir, 0755)
	defer os.RemoveAll("st-go")
	globalDevInfo := `<info deviceID="001122334455"><name>Living Room SoundTouch</name><type>SoundTouch</type><moduleType>10 sm2</moduleType></info>`
	os.WriteFile(filepath.Join(globalDevDir, "DeviceInfo.xml"), []byte(globalDevInfo), 0644)

	// Create a directory in 'default' with EMPTY name (the one that GetDeviceInfo will pick up)
	defaultDevDir := filepath.Join(tempBaseDir, "default", "devices", "001122334455")
	os.MkdirAll(defaultDevDir, 0755)
	defaultDevInfo := `<info deviceID="001122334455"><name></name><type>SoundTouch</type><moduleType>10 sm2</moduleType></info>`
	os.WriteFile(filepath.Join(defaultDevDir, "DeviceInfo.xml"), []byte(defaultDevInfo), 0644)

	err = SyncFromAccountFull(ds, &resp)
	if err != nil {
		t.Fatal(err)
	}

	// Verify name was preserved
	info, err := ds.GetDeviceInfo(accountID, "001122334455")
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != "Living Room SoundTouch" {
		t.Errorf("Expected name 'Living Room SoundTouch' to be preserved, got '%s'", info.Name)
	}

	// Re-generate XML to see if it now uses the preserved name
	data, err = AccountFullToXML(ds, accountID)
	if err != nil {
		t.Fatal(err)
	}
	err = xml.Unmarshal(data, &resp)
	if err != nil {
		t.Fatal(err)
	}

	found08 := false
	foundA8 := false

	for _, d := range resp.Devices {
		t.Logf("Checking device in response: ID=%s, Name='%s'\n", d.DeviceID, d.Name)
		if d.DeviceID == "AABBCCDDEE0A" {
			found08 = true
			if d.Name == "" {
				t.Error("Device AABBCCDDEE0A name should not be empty")
			}
		}
		if d.DeviceID == "001122334455" || d.DeviceID == "I6332527703739342000020" {
			if d.Name != "" {
				foundA8 = true
			}
		}
	}

	if !found08 {
		t.Error("Device AABBCCDDEE0A not found in response")
	}
	if !foundA8 {
		t.Error("Device 001122334455 not found in response")
	}
}

func TestRecentItemsMissingSources(t *testing.T) {
	tempBaseDir := "repro_recents_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	accountID := "1234567"
	deviceID := "001122334455"

	// Create device directory
	devDir := filepath.Join(tempBaseDir, "accounts", accountID, "devices", deviceID)
	err = os.MkdirAll(devDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create Recents.xml with some entries that have missing sources or sparse data
	recentsXML := `<?xml version="1.0" encoding="UTF-8"?>
<recents>
    <recent id="1" deviceID="001122334455" utcTime="1771916458">
        <contentItem source="INTERNET_RADIO" type="tracklisturl" location="/some/loc" isPresetable="true">
            <itemName>For Your Darkest Days</itemName>
        </contentItem>
    </recent>
    <recent id="2" deviceID="001122334455" utcTime="1771916459">
        <contentItem source="SPOTIFY" type="tracklisturl" location="/spotify/loc" sourceAccount="test-user" isPresetable="true">
            <itemName>Spotify Item</itemName>
        </contentItem>
    </recent>
</recents>`
	os.WriteFile(filepath.Join(devDir, "Recents.xml"), []byte(recentsXML), 0644)

	// Create Sources.xml with matching sources
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source displayName="" id="ir_source" type="INTERNET_RADIO">
        <sourceKey type="INTERNET_RADIO" account=""></sourceKey>
    </source>
    <source displayName="test-user+spotify@gmail.com" id="spotify_source" type="SPOTIFY">
        <sourceKey type="SPOTIFY" account="test-user"></sourceKey>
    </source>
</sources>`
	os.WriteFile(filepath.Join(devDir, "Sources.xml"), []byte(sourcesXML), 0644)

	ds := datastore.NewDataStore(tempBaseDir)
	recents, err := ds.GetRecents(accountID, deviceID)
	if err != nil {
		t.Fatalf("Failed to get recents: %v", err)
	}

	if len(recents) != 2 {
		t.Errorf("Expected 2 recents, got %d", len(recents))
	}

	for _, r := range recents {
		t.Logf("Recent: ID=%s, Name=%s, Source=%s, SourceAccount=%s", r.ID, r.Name, r.Source, r.SourceAccount)
		if r.Name == "" {
			t.Errorf("Recent %s has empty Name", r.ID)
		}
		if r.Source == "" {
			t.Errorf("Recent %s has empty Source attribute", r.ID)
		}
	}
}

func TestSyncSourcesAttributes(t *testing.T) {
	tempBaseDir := "sync_test_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	ds := datastore.NewDataStore(tempBaseDir)
	err = ds.Initialize()
	if err != nil {
		t.Fatal(err)
	}

	xmlData := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<account id="1234567">
    <devices>
        <device deviceid="AABBCCDDEE0A">
            <presets>
                <preset buttonNumber="1">
                    <name>test-playlist</name>
                    <source id="10863533" type="Audio">
                        <name>test-user</name>
                        <sourceproviderid>15</sourceproviderid>
                        <sourcename>test-user+spotify@gmail.com</sourcename>
                        <username>test-user</username>
                    </source>
                </preset>
            </presets>
            <recents>
                <recent id="2538285498">
                    <contentItem source="SPOTIFY" type="tracklisturl" location="spotify:track:abc" sourceAccount="test-user" isPresetable="true">
                        <itemName>test-playlist</itemName>
                    </contentItem>
                    <source id="10863533" type="Audio">
                        <name>test-user</name>
                        <sourceproviderid>15</sourceproviderid>
                        <sourcename>test-user+spotify@gmail.com</sourcename>
                        <username>test-user</username>
                    </source>
                </recent>
            </recents>
        </device>
    </devices>
    <sources>
        <source id="10863533" type="Audio">
            <name>test-user</name>
            <sourceproviderid>15</sourceproviderid>
            <sourcename>test-user+spotify@gmail.com</sourcename>
            <username>test-user</username>
        </source>
    </sources>
</account>`

	var resp models.AccountFullResponse
	err = xml.Unmarshal([]byte(xmlData), &resp)
	if err != nil {
		t.Fatal(err)
	}

	// Verify unmarshaling of attributes
	if resp.ID != "1234567" {
		t.Errorf("Expected account ID 1234567, got %s", resp.ID)
	}

	if len(resp.Devices) == 0 {
		t.Fatal("No devices found in unmarshaled response")
	}

	dev := resp.Devices[0]
	if len(dev.Presets) == 0 {
		t.Fatal("No presets found in unmarshaled response")
	}

	p := dev.Presets[0]
	if p.Source.ID != "10863533" {
		t.Errorf("Preset source ID not unmarshaled: expected 10863533, got '%s'", p.Source.ID)
	}
	if p.Source.Type != "Audio" {
		t.Errorf("Preset source Type not unmarshaled: expected Audio, got '%s'", p.Source.Type)
	}

	err = SyncFromAccountFull(ds, &resp)
	if err != nil {
		t.Fatal(err)
	}

	// Check datastore
	presets, err := ds.GetPresets("1234567", "AABBCCDDEE0A")
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) == 0 {
		t.Fatal("No presets found in datastore after sync")
	}

	lp := presets[0]
	if lp.SourceID != "10863533" {
		t.Errorf("Synced preset source ID mismatch: expected 10863533, got '%s'", lp.SourceID)
	}

	recents, err := ds.GetRecents("1234567", "AABBCCDDEE0A")
	if err != nil {
		t.Fatal(err)
	}
	if len(recents) == 0 {
		t.Fatal("No recents found in datastore after sync")
	}
	lr := recents[0]
	if lr.SourceID != "10863533" {
		t.Errorf("Synced recent source ID mismatch: expected 10863533, got '%s'", lr.SourceID)
	}
}

func TestSyncSourcesAggregation(t *testing.T) {
	tempBaseDir := "sync_agg_test_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	ds := datastore.NewDataStore(tempBaseDir)
	err = ds.Initialize()
	if err != nil {
		t.Fatal(err)
	}

	// Sources include valid <sourceproviderid> values so that HasResolvableProviderID
	// accepts them on import. Sources without a resolvable providerid are filtered
	// before persistence (issue #334) — bare type="Audio" sources with no providerid
	// would be dropped, exactly as device-local transient slots are. Use real
	// providerids (LOCAL_INTERNET_RADIO=11, INTERNET_RADIO=2, TUNEIN=25) here so
	// the aggregation assertion covers the path that actually reaches SaveConfiguredSources.
	xmlData := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<account id="agg_account">
    <devices>
        <device deviceid="dev1">
            <presets>
                <preset buttonNumber="1">
                    <name>Preset Source</name>
                    <source id="src_preset" type="Audio">
                        <sourceproviderid>11</sourceproviderid>
                        <name>Preset Source Name</name>
                    </source>
                </preset>
            </presets>
            <recents>
                <recent id="rec1">
                    <contentItem source="TUNEIN" type="stationurl" location="tunein:station:s123" sourceAccount="" isPresetable="true">
                        <itemName>Recent Source</itemName>
                    </contentItem>
                    <source id="src_recent" type="Audio">
                        <sourceproviderid>25</sourceproviderid>
                        <name>Recent Source Name</name>
                    </source>
                </recent>
            </recents>
        </device>
    </devices>
    <sources>
        <source id="src_account" type="Audio">
            <sourceproviderid>2</sourceproviderid>
            <name>Account Source Name</name>
        </source>
    </sources>
</account>`

	var resp models.AccountFullResponse
	err = xml.Unmarshal([]byte(xmlData), &resp)
	if err != nil {
		t.Fatal(err)
	}

	err = SyncFromAccountFull(ds, &resp)
	if err != nil {
		t.Fatal(err)
	}

	// Check aggregated sources for dev1
	sources, err := ds.GetConfiguredSources("agg_account", "dev1")
	if err != nil {
		t.Fatal(err)
	}

	// We expect 3 sources: src_account, src_preset, src_recent
	if len(sources) != 3 {
		t.Errorf("Expected 3 aggregated sources, got %d", len(sources))
		for _, s := range sources {
			t.Logf("Found source: ID=%s, Name=%s", s.ID, s.Name)
		}
	}

	foundAccount := false
	foundPreset := false
	foundRecent := false

	for _, s := range sources {
		switch s.ID {
		case "src_account":
			foundAccount = true
		case "src_preset":
			foundPreset = true
		case "src_recent":
			foundRecent = true
		}
	}

	if !foundAccount {
		t.Error("Source 'src_account' not found in aggregated sources")
	}
	if !foundPreset {
		t.Error("Source 'src_preset' not found in aggregated sources")
	}
	if !foundRecent {
		t.Error("Source 'src_recent' not found in aggregated sources")
	}
}
