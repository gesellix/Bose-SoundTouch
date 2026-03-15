package marge

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestReproduceMissingName(t *testing.T) {
	tempBaseDir := "repro_data"
	err := os.MkdirAll(tempBaseDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempBaseDir)

	accountID := "3230304"

	// Create device folders
	// 08DF1F0BA325 (has name)
	// A81B6A536A98 (missing name in full_local.xml)

	// Device 1: 08DF1F0BA325
	dev1Dir := filepath.Join(tempBaseDir, "accounts", accountID, "devices", "08DF1F0BA325")
	err = os.MkdirAll(dev1Dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	dev1Info := `<info deviceID="08DF1F0BA325">
    <name>A Sound Machine</name>
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

	// Device 2: A81B6A536A98 - MAC address ID in XML, name with special char or space?
	dev2Dir := filepath.Join(tempBaseDir, "accounts", accountID, "devices", "A81B6A536A98")
	err = os.MkdirAll(dev2Dir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	dev2Info := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="A81B6A536A98">
    <name>Sound Machinechen</name>
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
        <ipAddress>192.168.178.35</ipAddress>
        <macAddress>A81B6A536A98</macAddress>
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
	// Mock a response with empty name for A81B6A536A98
	for i := range resp.Devices {
		if resp.Devices[i].DeviceID == "A81B6A536A98" {
			resp.Devices[i].Name = ""
		}
	}

	// Remove the account-specific device directory to force resolution to 'default'
	os.RemoveAll(filepath.Join(tempBaseDir, "accounts", accountID, "devices", "A81B6A536A98"))

	// Create a duplicate directory in another place (e.g. 'st-go/data/accounts/default') with the CORRECT name
	// This simulates a global entry that ds.ListAllDevices() should find
	globalDevDir := filepath.Join("st-go", "data", "accounts", "default", "devices", "A81B6A536A98")
	os.MkdirAll(globalDevDir, 0755)
	defer os.RemoveAll("st-go")
	globalDevInfo := `<info deviceID="A81B6A536A98"><name>Sound Machinechen</name><type>SoundTouch</type><moduleType>10 sm2</moduleType></info>`
	os.WriteFile(filepath.Join(globalDevDir, "DeviceInfo.xml"), []byte(globalDevInfo), 0644)

	// Create a directory in 'default' with EMPTY name (the one that GetDeviceInfo will pick up)
	defaultDevDir := filepath.Join(tempBaseDir, "default", "devices", "A81B6A536A98")
	os.MkdirAll(defaultDevDir, 0755)
	defaultDevInfo := `<info deviceID="A81B6A536A98"><name></name><type>SoundTouch</type><moduleType>10 sm2</moduleType></info>`
	os.WriteFile(filepath.Join(defaultDevDir, "DeviceInfo.xml"), []byte(defaultDevInfo), 0644)

	err = SyncFromAccountFull(ds, &resp)
	if err != nil {
		t.Fatal(err)
	}

	// Verify name was preserved
	info, err := ds.GetDeviceInfo(accountID, "A81B6A536A98")
	if err != nil {
		t.Fatal(err)
	}

	if info.Name != "Sound Machinechen" {
		t.Errorf("Expected name 'Sound Machinechen' to be preserved, got '%s'", info.Name)
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
		if d.DeviceID == "08DF1F0BA325" {
			found08 = true
			if d.Name == "" {
				t.Error("Device 08DF1F0BA325 name should not be empty")
			}
		}
		if d.DeviceID == "A81B6A536A98" || d.DeviceID == "I6332527703739342000020" {
			if d.Name != "" {
				foundA8 = true
			}
		}
	}

	if !found08 {
		t.Error("Device 08DF1F0BA325 not found in response")
	}
	if !foundA8 {
		t.Error("Device A81B6A536A98 not found in response")
	}
}
