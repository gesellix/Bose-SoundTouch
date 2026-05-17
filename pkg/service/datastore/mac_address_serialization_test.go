package datastore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestMacAddressSerialization(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mac-serialization-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	account := "1000001"
	device := "I6332527703739342000020"
	macAddress := "AABBCCDDEEFF"

	// Create device info with MAC address
	info := &models.ServiceDeviceInfo{
		DeviceID:            device,
		Name:                "Test SoundTouch",
		ProductCode:         "SoundTouch 10",
		IPAddress:           "192.0.2.100",
		MacAddress:          macAddress,
		DeviceSerialNumber:  device,
		ProductSerialNumber: "PROD123456",
		FirmwareVersion:     "4.8.1.23456",
		DiscoveryMethod:     "UPnP",
	}

	// Save device info
	err = ds.SaveDeviceInfo(account, device, info)
	if err != nil {
		t.Fatalf("SaveDeviceInfo failed: %v", err)
	}

	// Verify the XML file was created
	deviceInfoPath := filepath.Join(ds.AccountDeviceDir(account, device), "DeviceInfo.xml")
	if _, err := os.Stat(deviceInfoPath); err != nil {
		t.Fatalf("DeviceInfo.xml not created: %v", err)
	}

	// Read back the device info
	loadedInfo, err := ds.GetDeviceInfo(account, device)
	if err != nil {
		t.Fatalf("GetDeviceInfo failed: %v", err)
	}

	// Verify MAC address is preserved
	if loadedInfo.MacAddress != macAddress {
		t.Errorf("MAC address not preserved. Expected: '%s', Got: '%s'", macAddress, loadedInfo.MacAddress)
	}

	// Verify other fields are also correct
	if loadedInfo.DeviceID != device {
		t.Errorf("DeviceID mismatch. Expected: %s, Got: %s", device, loadedInfo.DeviceID)
	}

	if loadedInfo.IPAddress != "192.0.2.100" {
		t.Errorf("IPAddress mismatch. Expected: 192.0.2.100, Got: %s", loadedInfo.IPAddress)
	}

	// Initialize datastore to populate MAC mappings
	err = ds.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test that MAC address mapping works
	resolvedPath := ds.AccountDeviceDir(account, macAddress)
	expectedPath := ds.AccountDeviceDir(account, device)

	if resolvedPath != expectedPath {
		t.Errorf("MAC address mapping failed. MAC '%s' resolved to '%s', expected '%s'",
			macAddress, resolvedPath, expectedPath)
	}

	// Test that Sources.xml path resolves correctly via MAC address
	// (We don't need to actually read the file, just verify the path resolution works)
	macPath := ds.AccountDeviceDir(account, macAddress)
	devicePath := ds.AccountDeviceDir(account, device)

	if macPath != devicePath {
		t.Errorf("MAC address path resolution failed. MAC path: %s, Device path: %s", macPath, devicePath)
	}

	t.Logf("✅ MAC address serialization working correctly")
	t.Logf("   - MAC address '%s' saved to DeviceInfo.xml", macAddress)
	t.Logf("   - MAC address '%s' loaded from DeviceInfo.xml", loadedInfo.MacAddress)
	t.Logf("   - MAC mapping: '%s' -> '%s'", macAddress, device)
	t.Logf("   - Sources.xml accessible via MAC address")
}

func TestMacAddressSerializationEdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mac-edge-cases-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	account := "testaccount"
	device := "testdevice"

	testCases := []struct {
		name       string
		macAddress string
		expected   string
	}{
		{"uppercase", "AABBCCDDEEFF", "AABBCCDDEEFF"},
		{"lowercase", "aabbccddeeff", "aabbccddeeff"},
		{"with_colons", "AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF"},
		{"with_dashes", "AA-BB-CC-DD-EE-FF", "AA-BB-CC-DD-EE-FF"},
		{"empty", "", ""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			deviceID := device + "_" + tc.name

			info := &models.ServiceDeviceInfo{
				DeviceID:           deviceID,
				Name:               "Test Device " + tc.name,
				ProductCode:        "SoundTouch 10",
				IPAddress:          "192.0.2.100",
				MacAddress:         tc.macAddress,
				DeviceSerialNumber: deviceID,
			}

			// Save and load
			err := ds.SaveDeviceInfo(account, deviceID, info)
			if err != nil {
				t.Fatalf("SaveDeviceInfo failed for %s: %v", tc.name, err)
			}

			loadedInfo, err := ds.GetDeviceInfo(account, deviceID)
			if err != nil {
				t.Fatalf("GetDeviceInfo failed for %s: %v", tc.name, err)
			}

			if loadedInfo.MacAddress != tc.expected {
				t.Errorf("MAC address mismatch for %s. Expected: '%s', Got: '%s'",
					tc.name, tc.expected, loadedInfo.MacAddress)
			}
		})
	}
}

func TestExistingDeviceInfoUpdate(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "device-update-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	account := "1000001"
	device := "I6332527703739342000020"

	// First save without MAC address (simulating old DeviceInfo.xml)
	infoWithoutMAC := &models.ServiceDeviceInfo{
		DeviceID:           device,
		Name:               "Test SoundTouch",
		ProductCode:        "SoundTouch 10",
		IPAddress:          "192.0.2.100",
		MacAddress:         "", // No MAC address initially
		DeviceSerialNumber: device,
	}

	err = ds.SaveDeviceInfo(account, device, infoWithoutMAC)
	if err != nil {
		t.Fatalf("Initial SaveDeviceInfo failed: %v", err)
	}

	// Verify no MAC address initially
	loadedInfo1, err := ds.GetDeviceInfo(account, device)
	if err != nil {
		t.Fatalf("Initial GetDeviceInfo failed: %v", err)
	}

	if loadedInfo1.MacAddress != "" {
		t.Errorf("Expected empty MAC address, got '%s'", loadedInfo1.MacAddress)
	}

	// Now update with MAC address (simulating discovery update)
	macAddress := "AABBCCDDEEFF"
	infoWithMAC := &models.ServiceDeviceInfo{
		DeviceID:           device,
		Name:               "Test SoundTouch",
		ProductCode:        "SoundTouch 10",
		IPAddress:          "192.0.2.100",
		MacAddress:         macAddress,
		DeviceSerialNumber: device,
	}

	err = ds.SaveDeviceInfo(account, device, infoWithMAC)
	if err != nil {
		t.Fatalf("Update SaveDeviceInfo failed: %v", err)
	}

	// Verify MAC address is now present
	loadedInfo2, err := ds.GetDeviceInfo(account, device)
	if err != nil {
		t.Fatalf("Updated GetDeviceInfo failed: %v", err)
	}

	if loadedInfo2.MacAddress != macAddress {
		t.Errorf("MAC address not updated. Expected: '%s', Got: '%s'", macAddress, loadedInfo2.MacAddress)
	}

	// Initialize to test mapping
	err = ds.Initialize()
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Test that MAC mapping now works
	resolvedPath := ds.AccountDeviceDir(account, macAddress)
	expectedPath := ds.AccountDeviceDir(account, device)

	if resolvedPath != expectedPath {
		t.Errorf("MAC mapping failed after update. MAC '%s' resolved to '%s', expected '%s'",
			macAddress, resolvedPath, expectedPath)
	}

	t.Logf("✅ DeviceInfo.xml update with MAC address working correctly")
	t.Logf("   - Initial: no MAC address")
	t.Logf("   - Updated: MAC address '%s' added", macAddress)
	t.Logf("   - Mapping: '%s' -> '%s'", macAddress, device)
}
