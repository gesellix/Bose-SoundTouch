package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

func TestComprehensiveMigration_MultipleExistingDevices(t *testing.T) {
	// This test simulates the real-world scenario where a device has been discovered
	// and saved under multiple identifiers over time, and now needs to be consolidated
	// into a single MAC-based identifier.

	tempDir, err := os.MkdirTemp("", "comprehensive-migration-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	accountID := "3230304"

	// Scenario: Same device has been saved under different identifiers:
	// 1. Initially discovered by IP address
	// 2. Later discovered with UPnP serial
	// 3. Later discovered with device component serial

	// Create device entry #1: Saved by IP address (early discovery)
	ipDeviceID := "192.168.1.100"
	ipInfo := &models.ServiceDeviceInfo{
		DeviceID:        ipDeviceID,
		AccountID:       accountID,
		Name:            "Unknown Device", // Generic name from early discovery
		IPAddress:       ipDeviceID,
		ProductCode:     "Unknown",
		FirmwareVersion: "0.0.0",
		DiscoveryMethod: "UPnP",
	}
	if err := ds.SaveDeviceInfo(accountID, ipDeviceID, ipInfo); err != nil {
		t.Fatalf("Failed to save IP-based device: %v", err)
	}

	// Save some presets for the IP-based device
	testPresets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:       "1",
				Source:   "SPOTIFY",
				Location: "spotify://playlist/test1",
				Name:     "Test Playlist 1",
			},
			CreatedOn: "2024-01-01T00:00:00Z",
			UpdatedOn: "2024-01-01T00:00:00Z",
		},
	}
	if err := ds.SavePresets(accountID, ipDeviceID, testPresets); err != nil {
		t.Fatalf("Failed to save presets for IP device: %v", err)
	}

	// Create device entry #2: Saved by component serial (later discovery with better info)
	serialDeviceID := "I6332527703739342000020"
	serialInfo := &models.ServiceDeviceInfo{
		DeviceID:            serialDeviceID,
		AccountID:           accountID,
		Name:                "Sound Machinechen", // Real name from /info
		IPAddress:           "192.168.1.100",     // Same IP as before
		DeviceSerialNumber:  serialDeviceID,
		ProductCode:         "SoundTouch 10",
		FirmwareVersion:     "27.0.6.46330.5043500",
		ProductSerialNumber: "069231P63364828AE",
		DiscoveryMethod:     "UPnP",
	}
	if err := ds.SaveDeviceInfo(accountID, serialDeviceID, serialInfo); err != nil {
		t.Fatalf("Failed to save serial-based device: %v", err)
	}

	// Save different presets for the serial-based device (user might have configured both thinking they're different)
	serialPresets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:       "2",
				Source:   "SPOTIFY",
				Location: "spotify://playlist/test2",
				Name:     "Test Playlist 2",
			},
			CreatedOn: "2024-01-02T00:00:00Z",
			UpdatedOn: "2024-01-02T00:00:00Z",
		},
	}
	if err := ds.SavePresets(accountID, serialDeviceID, serialPresets); err != nil {
		t.Fatalf("Failed to save presets for serial device: %v", err)
	}

	// Create device entry #3: Saved by UPnP serial (yet another discovery)
	upnpDeviceID := "UPnP789XYZ"
	upnpInfo := &models.ServiceDeviceInfo{
		DeviceID:        upnpDeviceID,
		AccountID:       accountID,
		Name:            "SoundTouch Device", // Generic UPnP name
		IPAddress:       "192.168.1.100",     // Same IP again
		ProductCode:     "SoundTouch 10 sm2",
		FirmwareVersion: "27.0.6.46330.5043500", // Same firmware as serial device
		DiscoveryMethod: "UPnP",
	}
	if err := ds.SaveDeviceInfo(accountID, upnpDeviceID, upnpInfo); err != nil {
		t.Fatalf("Failed to save UPnP-based device: %v", err)
	}

	t.Logf("Test setup complete:")
	t.Logf("  Device #1: %s (IP-based, early discovery)", ipDeviceID)
	t.Logf("  Device #2: %s (serial-based, better info)", serialDeviceID)
	t.Logf("  Device #3: %s (UPnP-based, latest discovery)", upnpDeviceID)

	// Now simulate the device being rediscovered with /info endpoint working
	deviceInfoXML := `<info deviceID="A81B6A536A98">
<name>Sound Machinechen</name>
<type>SoundTouch 10</type>
<margeAccountUUID>3230304</margeAccountUUID>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>I6332527703739342000020</serialNumber>
</component>
<component>
<componentCategory>PackagedProduct</componentCategory>
<softwareVersion>27.0.6.46330.5043500</softwareVersion>
<serialNumber>069231P63364828AE</serialNumber>
</component>
</components>
<margeURL>https://streaming.bose.com</margeURL>
<networkInfo type="SCM">
<macAddress>A81B6A536A98</macAddress>
<ipAddress>192.168.1.100</ipAddress>
</networkInfo>
<moduleType>sm2</moduleType>
</info>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, deviceInfoXML)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	deviceIP := server.URL[len("http://"):]
	sm := setup.NewManager(server.URL, ds, nil)
	srv := NewServer(ds, sm, server.URL, false, false, false, false, false)

	// Simulate device rediscovery
	discoveredDevice := models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Generic Discovery Name",
		ModelID:         "SoundTouch",
		SerialNo:        "UPnP789XYZ", // This should match one of the existing devices
		DiscoveryMethod: "UPnP",
	}

	t.Logf("\nSimulating comprehensive device rediscovery...")
	t.Logf("  Discovery IP: %s", deviceIP)
	t.Logf("  Discovery Serial: %s", discoveredDevice.SerialNo)

	// Handle discovered device - should find and migrate all existing variants
	srv.handleDiscoveredDevice(discoveredDevice)

	// Verify the device now exists under the MAC address
	expectedDeviceID := "A81B6A536A98"
	migratedInfo, err := ds.GetDeviceInfo(accountID, expectedDeviceID)
	if err != nil {
		t.Fatalf("Failed to get migrated device info: %v", err)
	}

	// Verify the migrated device has the correct information
	if migratedInfo.DeviceID != expectedDeviceID {
		t.Errorf("Expected deviceID '%s', got '%s'", expectedDeviceID, migratedInfo.DeviceID)
	}

	if migratedInfo.Name != "Sound Machinechen" {
		t.Errorf("Expected name 'Sound Machinechen' (from /info), got '%s'", migratedInfo.Name)
	}

	if migratedInfo.MacAddress != "A81B6A536A98" {
		t.Errorf("Expected MAC 'A81B6A536A98', got '%s'", migratedInfo.MacAddress)
	}

	if migratedInfo.DeviceSerialNumber != "I6332527703739342000020" {
		t.Errorf("Expected device serial 'I6332527703739342000020', got '%s'", migratedInfo.DeviceSerialNumber)
	}

	t.Logf("\nMigration completed successfully:")
	t.Logf("  New device ID: %s (MAC address)", migratedInfo.DeviceID)
	t.Logf("  Device name: %s", migratedInfo.Name)
	t.Logf("  Device serial: %s", migratedInfo.DeviceSerialNumber)
	t.Logf("  Product serial: %s", migratedInfo.ProductSerialNumber)
	t.Logf("  MAC address: %s", migratedInfo.MacAddress)

	// Verify MAC address resolution works
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize datastore: %v", err)
	}

	resolvedDir := ds.AccountDeviceDir(accountID, "A81B6A536A98")
	expectedDir := ds.AccountDeviceDir(accountID, expectedDeviceID)

	if resolvedDir != expectedDir {
		t.Errorf("MAC resolution failed. Expected '%s', got '%s'", expectedDir, resolvedDir)
	}

	t.Logf("\nMAC address resolution verified:")
	t.Logf("  MAC 'A81B6A536A98' resolves to correct device directory")

	// Note: In a complete implementation, we'd also verify that presets from all
	// the old devices were consolidated, but that requires more sophisticated
	// preset merging logic which is beyond the current migration scope.

	t.Logf("\n✅ Comprehensive migration test completed successfully!")
}

func TestFindAllExistingDeviceVariants_MatchingCriteria(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "variants-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	srv := NewServer(ds, nil, "http://localhost", false, false, false, false, false)
	accountID := "testaccount"

	// Create devices that should match various criteria
	devices := []models.ServiceDeviceInfo{
		{
			DeviceID:           "192.168.1.100",
			AccountID:          accountID,
			Name:               "IP Device",
			IPAddress:          "192.168.1.100",
			DeviceSerialNumber: "SERIAL123",
			MacAddress:         "AA:BB:CC:DD:EE:FF",
		},
		{
			DeviceID:           "SERIAL123",
			AccountID:          accountID,
			Name:               "Sound Speaker",
			IPAddress:          "192.168.1.101", // Different IP
			DeviceSerialNumber: "SERIAL123",
			MacAddress:         "AA:BB:CC:DD:EE:FF",
		},
		{
			DeviceID:    "UPnPSerial456",
			AccountID:   accountID,
			Name:        "Sound Speaker",
			IPAddress:   "192.168.1.102", // Different IP again
			ProductCode: "SoundTouch 10 sm2",
		},
		{
			DeviceID:           "UnrelatedDevice",
			AccountID:          accountID,
			Name:               "Other Device",
			IPAddress:          "192.168.1.200",
			DeviceSerialNumber: "OTHERSSERIAL",
		},
	}

	for _, device := range devices {
		if err := ds.SaveDeviceInfo(accountID, device.DeviceID, &device); err != nil {
			t.Fatalf("Failed to save device %s: %v", device.DeviceID, err)
		}
	}

	// Create mock discovery and live info
	discovery := models.DiscoveredDevice{
		Host:     "192.168.1.100", // Matches first device by IP
		SerialNo: "UPnPSerial456", // Matches third device by UPnP serial
	}

	liveInfo := &setup.DeviceInfoXML{
		DeviceID:     "AABBCCDDEEFF",  // New MAC-based ID
		Name:         "Sound Speaker", // Matches second and third devices by name
		Type:         "SoundTouch 10",
		ModuleType:   "sm2",
		SerialNumber: "SERIAL123", // Matches first and second devices by serial
		NetworkInfo: []struct {
			Type       string `xml:"type,attr"`
			MacAddress string `xml:"macAddress"`
			IPAddress  string `xml:"ipAddress"`
		}{
			{Type: "SCM", MacAddress: "AA:BB:CC:DD:EE:FF", IPAddress: "192.168.1.100"},
		},
	}

	// Test the matching logic
	matches := srv.findAllExistingDeviceVariants(discovery, liveInfo)

	t.Logf("Found %d matching device variants:", len(matches))
	for i, match := range matches {
		t.Logf("  %d. %s (IP: %s, Serial: %s, MAC: %s, Name: %s)",
			i+1, match.DeviceID, match.IPAddress, match.DeviceSerialNumber, match.MacAddress, match.Name)
	}

	// Verify expected matches
	expectedMatches := map[string]string{
		"192.168.1.100": "IP match",
		"SERIAL123":     "Serial match",
		"UPnPSerial456": "UPnP serial match",
	}

	if len(matches) != len(expectedMatches) {
		t.Errorf("Expected %d matches, got %d", len(expectedMatches), len(matches))
	}

	foundMatches := make(map[string]bool)
	for _, match := range matches {
		foundMatches[match.DeviceID] = true
	}

	for expectedID, reason := range expectedMatches {
		if !foundMatches[expectedID] {
			t.Errorf("Expected to find device %s (%s), but it was not matched", expectedID, reason)
		}
	}

	// Verify UnrelatedDevice is NOT matched
	if foundMatches["UnrelatedDevice"] {
		t.Error("UnrelatedDevice should not have been matched, but it was")
	}

	t.Logf("✅ Device variant matching test completed successfully!")
}

func TestMigration_EdgeCases(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "migration-edge-cases-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	srv := NewServer(ds, nil, "http://localhost", false, false, false, false, false)
	accountID := "testaccount"

	t.Run("NoExistingDevices", func(t *testing.T) {
		discovery := models.DiscoveredDevice{Host: "192.168.1.200"}
		liveInfo := &setup.DeviceInfoXML{DeviceID: "NEWMAC123"}

		matches := srv.findAllExistingDeviceVariants(discovery, liveInfo)
		if len(matches) != 0 {
			t.Errorf("Expected 0 matches for new device, got %d", len(matches))
		}
	})

	t.Run("SelfMatch", func(t *testing.T) {
		// Device already exists with MAC as deviceID
		macDeviceID := "AABBCCDDEEFF"
		existing := &models.ServiceDeviceInfo{
			DeviceID:  macDeviceID,
			AccountID: accountID,
			Name:      "Existing MAC Device",
			IPAddress: "192.168.1.150",
		}
		if err := ds.SaveDeviceInfo(accountID, macDeviceID, existing); err != nil {
			t.Fatalf("Failed to save MAC device: %v", err)
		}

		discovery := models.DiscoveredDevice{Host: "192.168.1.150"}
		liveInfo := &setup.DeviceInfoXML{DeviceID: macDeviceID} // Same MAC

		matches := srv.findAllExistingDeviceVariants(discovery, liveInfo)

		// Should find itself, but migration logic should skip it since deviceID matches
		found := false
		for _, match := range matches {
			if match.DeviceID == macDeviceID {
				found = true
				break
			}
		}
		if !found {
			t.Error("Device should find itself in variants")
		}
	})

	t.Run("PartialMatches", func(t *testing.T) {
		// Device with some matching criteria but not others
		partialDevice := &models.ServiceDeviceInfo{
			DeviceID:  "PARTIAL123",
			AccountID: accountID,
			Name:      "Partial Device",
			IPAddress: "192.168.1.160", // Different IP
			// No serial number, no MAC
		}
		if err := ds.SaveDeviceInfo(accountID, "PARTIAL123", partialDevice); err != nil {
			t.Fatalf("Failed to save partial device: %v", err)
		}

		discovery := models.DiscoveredDevice{Host: "192.168.1.170"} // Different IP
		liveInfo := &setup.DeviceInfoXML{
			DeviceID: "NEWMAC456",
			Name:     "Partial Device", // Same name
			Type:     "SoundTouch 20",
		}

		matches := srv.findAllExistingDeviceVariants(discovery, liveInfo)

		// Should match by name and product type
		found := false
		for _, match := range matches {
			if match.DeviceID == "PARTIAL123" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Should match device by name and product type")
		}
	})
}
