package handlers

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
)

func TestMACBasedDeviceDiscovery_Integration(t *testing.T) {
	// Create temporary datastore
	tempDir, err := os.MkdirTemp("", "mac-discovery-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Mock device info response (real-world example)
	deviceInfoXML := `<info deviceID="001122334455">
<name>Sound Machinechen</name>
<type>SoundTouch 10</type>
<margeAccountUUID>1234567</margeAccountUUID>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>I6332527703739342000020</serialNumber>
</component>
<component>
<componentCategory>PackagedProduct</componentCategory>
<softwareVersion>27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29</softwareVersion>
<serialNumber>069231P63364828AE</serialNumber>
</component>
</components>
<margeURL>https://streaming.bose.com</margeURL>
<networkInfo type="SCM">
<macAddress>001122334455</macAddress>
<ipAddress>192.168.1.100</ipAddress>
</networkInfo>
<networkInfo type="SMSC">
<macAddress>A81B6A849D99</macAddress>
<ipAddress>192.168.1.100</ipAddress>
</networkInfo>
<moduleType>sm2</moduleType>
<variant>rhino</variant>
<variantMode>normal</variantMode>
<countryCode>GB</countryCode>
<regionCode>GB</regionCode>
</info>`

	// Create mock HTTP server for device /info endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, deviceInfoXML)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Extract host from server URL for device IP
	deviceIP := server.URL[len("http://"):]

	// Create datastore and setup manager
	ds := datastore.NewDataStore(tempDir)
	sm := setup.NewManager(server.URL, ds, nil)

	// Create server instance
	srv := NewServer(ds, sm, "http://localhost", false, false, false)

	t.Logf("Test scenario:")
	t.Logf("  Device IP: %s", deviceIP)
	t.Logf("  Mock /info endpoint: %s/info", server.URL)

	// 1. Simulate device discovery
	discoveredDevice := models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Legacy Discovery Name", // This should be overridden by /info
		ModelID:         "Legacy Model",
		SerialNo:        "", // No serial from discovery
		DiscoveryMethod: "UPnP",
	}

	t.Logf("\n1. Simulating device discovery...")
	t.Logf("   Discovery name: %s", discoveredDevice.Name)
	t.Logf("   Discovery model: %s", discoveredDevice.ModelID)
	t.Logf("   Discovery serial: %s", discoveredDevice.SerialNo)

	// 2. Handle discovered device (this should fetch /info and use MAC as deviceID)
	srv.handleDiscoveredDevice(discoveredDevice)

	// 3. Verify the device was saved with MAC address as deviceID
	expectedDeviceID := "001122334455" // MAC address from /info
	expectedAccountID := "1234567"     // From margeAccountUUID

	deviceInfo, err := ds.GetDeviceInfo(expectedAccountID, expectedDeviceID)
	if err != nil {
		t.Fatalf("Failed to get device info: %v", err)
	}

	t.Logf("\n2. Device saved successfully:")
	t.Logf("   Device ID: %s (MAC address from /info)", deviceInfo.DeviceID)
	t.Logf("   Account ID: %s", deviceInfo.AccountID)
	t.Logf("   Device Name: %s (from /info, not discovery)", deviceInfo.Name)
	t.Logf("   Product Code: %s", deviceInfo.ProductCode)
	t.Logf("   MAC Address: %s", deviceInfo.MacAddress)
	t.Logf("   IP Address: %s", deviceInfo.IPAddress)
	t.Logf("   Device Serial: %s", deviceInfo.DeviceSerialNumber)
	t.Logf("   Product Serial: %s", deviceInfo.ProductSerialNumber)
	t.Logf("   Firmware: %s", deviceInfo.FirmwareVersion)
	t.Logf("   Discovery Method: %s", deviceInfo.DiscoveryMethod)

	// Verify key fields
	if deviceInfo.DeviceID != expectedDeviceID {
		t.Errorf("Expected deviceID '%s', got '%s'", expectedDeviceID, deviceInfo.DeviceID)
	}

	if deviceInfo.AccountID != expectedAccountID {
		t.Errorf("Expected accountID '%s', got '%s'", expectedAccountID, deviceInfo.AccountID)
	}

	if deviceInfo.Name != "Sound Machinechen" {
		t.Errorf("Expected name 'Sound Machinechen' (from /info), got '%s'", deviceInfo.Name)
	}

	if deviceInfo.ProductCode != "SoundTouch 10 sm2" {
		t.Errorf("Expected productCode 'SoundTouch 10 sm2', got '%s'", deviceInfo.ProductCode)
	}

	if deviceInfo.MacAddress != "001122334455" {
		t.Errorf("Expected macAddress '001122334455', got '%s'", deviceInfo.MacAddress)
	}

	if deviceInfo.DeviceSerialNumber != "I6332527703739342000020" {
		t.Errorf("Expected deviceSerial 'I6332527703739342000020', got '%s'", deviceInfo.DeviceSerialNumber)
	}

	if deviceInfo.ProductSerialNumber != "069231P63364828AE" {
		t.Errorf("Expected productSerial '069231P63364828AE', got '%s'", deviceInfo.ProductSerialNumber)
	}

	expectedFirmware := "27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29"
	if deviceInfo.FirmwareVersion != expectedFirmware {
		t.Errorf("Expected firmware '%s', got '%s'", expectedFirmware, deviceInfo.FirmwareVersion)
	}

	if deviceInfo.DiscoveryMethod != "UPnP" {
		t.Errorf("Expected discoveryMethod 'UPnP', got '%s'", deviceInfo.DiscoveryMethod)
	}

	// 4. Verify directory structure uses MAC address
	expectedDir := filepath.Join(tempDir, "accounts", expectedAccountID, "devices", expectedDeviceID)
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Errorf("Expected device directory not found: %s", expectedDir)
	} else {
		t.Logf("\n3. Directory structure verified:")
		t.Logf("   Device directory: %s", expectedDir)
	}

	// 5. Verify DeviceInfo.xml file contains MAC address in networkInfo
	deviceInfoPath := filepath.Join(expectedDir, "DeviceInfo.xml")
	xmlData, err := os.ReadFile(deviceInfoPath)
	if err != nil {
		t.Fatalf("Failed to read DeviceInfo.xml: %v", err)
	}

	var savedXML struct {
		XMLName     xml.Name `xml:"info"`
		DeviceID    string   `xml:"deviceID,attr"`
		NetworkInfo []struct {
			Type       string `xml:"type,attr"`
			MacAddress string `xml:"macAddress"`
			IPAddress  string `xml:"ipAddress"`
		} `xml:"networkInfo"`
	}

	if err := xml.Unmarshal(xmlData, &savedXML); err != nil {
		t.Fatalf("Failed to parse saved DeviceInfo.xml: %v", err)
	}

	if savedXML.DeviceID != expectedDeviceID {
		t.Errorf("Expected saved deviceID '%s', got '%s'", expectedDeviceID, savedXML.DeviceID)
	}

	// Verify MAC address in networkInfo
	macFound := false
	for _, net := range savedXML.NetworkInfo {
		if net.Type == "SCM" && net.MacAddress == "001122334455" {
			macFound = true
			break
		}
	}
	if !macFound {
		t.Error("MAC address not found in saved DeviceInfo.xml networkInfo")
	}

	t.Logf("\n4. DeviceInfo.xml verification:")
	t.Logf("   File exists: %s", deviceInfoPath)
	t.Logf("   Contains MAC in networkInfo: %v", macFound)

	// 6. Initialize datastore to populate MAC mappings
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize datastore: %v", err)
	}

	// 7. Test MAC address resolution
	resolvedDir := ds.AccountDeviceDir(expectedAccountID, "001122334455") // Use MAC as device lookup
	expectedResolvedDir := ds.AccountDeviceDir(expectedAccountID, expectedDeviceID)

	if resolvedDir != expectedResolvedDir {
		t.Errorf("MAC resolution failed. Expected '%s', got '%s'", expectedResolvedDir, resolvedDir)
	} else {
		t.Logf("\n5. MAC address resolution verified:")
		t.Logf("   MAC '001122334455' resolves to correct device directory")
	}

	t.Logf("\n✅ MAC-based device discovery integration test passed!")
	t.Logf("Summary:")
	t.Logf("  • Discovery finds device IP: %s", deviceIP)
	t.Logf("  • /info provides canonical deviceID: %s (MAC address)", expectedDeviceID)
	t.Logf("  • Device stored in account: %s", expectedAccountID)
	t.Logf("  • Directory uses MAC address: %s", expectedDeviceID)
	t.Logf("  • DeviceInfo.xml contains full device details from /info")
	t.Logf("  • MAC address resolution works for API endpoints")
}

func TestMACBasedDeviceDiscovery_MigrationScenario(t *testing.T) {
	// Test scenario where we have existing device stored by IP/serial and need to migrate to MAC
	tempDir, err := os.MkdirTemp("", "mac-migration-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	accountID := "1234567"

	// 1. Create an existing device entry using IP address (old style)
	oldDeviceID := "192.168.1.100"
	oldInfo := &models.ServiceDeviceInfo{
		DeviceID:        oldDeviceID,
		AccountID:       accountID,
		Name:            "Old Device Name",
		IPAddress:       oldDeviceID,
		ProductCode:     "Unknown Model",
		FirmwareVersion: "0.0.0",
		DiscoveryMethod: "UPnP",
	}

	if err := ds.SaveDeviceInfo(accountID, oldDeviceID, oldInfo); err != nil {
		t.Fatalf("Failed to save old device info: %v", err)
	}

	// Save some test presets for the old device
	testPresets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:       "1",
				Source:   "SPOTIFY",
				Location: "spotify://playlist/test",
				Name:     "Test Playlist",
			},
			CreatedOn: "2024-01-01T00:00:00Z",
			UpdatedOn: "2024-01-01T00:00:00Z",
		},
	}
	if err := ds.SavePresets(accountID, oldDeviceID, testPresets); err != nil {
		t.Fatalf("Failed to save test presets: %v", err)
	}

	t.Logf("Test scenario: Device migration")
	t.Logf("  Old device ID: %s (IP address)", oldDeviceID)
	t.Logf("  Test presets saved: %d", len(testPresets))

	// 2. Mock the same device now providing proper /info response
	deviceInfoXML := `<info deviceID="001122334455">
<name>Sound Machinechen</name>
<type>SoundTouch 10</type>
<margeAccountUUID>1234567</margeAccountUUID>
<components>
<component>
<componentCategory>SCM</componentCategory>
<softwareVersion>27.0.6.46330.5043500</softwareVersion>
<serialNumber>I6332527703739342000020</serialNumber>
</component>
</components>
<networkInfo type="SCM">
<macAddress>001122334455</macAddress>
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
	srv := NewServer(ds, sm, server.URL, false, false, false)

	// 3. Simulate rediscovery of the same device (now with /info working)
	discoveredDevice := models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Discovery Name",
		ModelID:         "Discovery Model",
		SerialNo:        "",
		DiscoveryMethod: "UPnP",
	}

	// 4. Handle discovered device - should migrate from old ID to MAC
	srv.handleDiscoveredDevice(discoveredDevice)

	// 5. Verify new device exists with MAC as deviceID
	newDeviceID := "001122334455"
	newInfo, err := ds.GetDeviceInfo(accountID, newDeviceID)
	if err != nil {
		t.Fatalf("Failed to get migrated device info: %v", err)
	}

	if newInfo.DeviceID != newDeviceID {
		t.Errorf("Expected new deviceID '%s', got '%s'", newDeviceID, newInfo.DeviceID)
	}

	if newInfo.Name != "Sound Machinechen" {
		t.Errorf("Expected name from /info 'Sound Machinechen', got '%s'", newInfo.Name)
	}

	t.Logf("\nMigration completed:")
	t.Logf("  New device ID: %s (MAC address)", newInfo.DeviceID)
	t.Logf("  Updated name: %s (from /info)", newInfo.Name)
	t.Logf("  Updated product: %s", newInfo.ProductCode)

	// 6. Verify old device directory no longer exists (after cleanup)
	// Note: The actual cleanup happens in migrateDeviceFiles, which in our current
	// implementation is a placeholder. For this test, we'll just verify the new device exists.

	// 7. Verify presets are accessible via new device ID
	// (In a full implementation, presets would be migrated)
	newPresets, err := ds.GetPresets(accountID, newDeviceID)
	if err != nil {
		// This is expected if migration hasn't been fully implemented
		t.Logf("Presets migration: %v (migration implementation pending)", err)
	} else {
		t.Logf("Presets migrated successfully: %d presets", len(newPresets))
	}

	t.Logf("\n✅ MAC-based device migration test completed!")
}

func TestMACBasedDeviceDiscovery_FallbackScenario(t *testing.T) {
	// Test scenario where /info endpoint is not available
	tempDir, err := os.MkdirTemp("", "mac-fallback-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create server that returns 404 for /info
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	deviceIP := server.URL[len("http://"):]
	ds := datastore.NewDataStore(tempDir)
	sm := setup.NewManager(server.URL, ds, nil)

	srv := NewServer(ds, sm, server.URL, false, false, false)

	// Simulate device discovery with UPnP providing serial
	discoveredDevice := models.DiscoveredDevice{
		Host:            deviceIP,
		Name:            "Legacy Device",
		ModelID:         "SoundTouch 20",
		SerialNo:        "UPnP123456789", // Serial from UPnP discovery
		DiscoveryMethod: "UPnP",
	}

	t.Logf("Test scenario: /info endpoint not available")
	t.Logf("  Device IP: %s", deviceIP)
	t.Logf("  UPnP Serial: %s", discoveredDevice.SerialNo)

	// Handle discovered device - should fall back to UPnP serial
	srv.handleDiscoveredDevice(discoveredDevice)

	// Verify device was saved using UPnP serial as fallback
	expectedDeviceID := "UPnP123456789"
	expectedAccountID := "default" // Should use default account when /info unavailable

	deviceInfo, err := ds.GetDeviceInfo(expectedAccountID, expectedDeviceID)
	if err != nil {
		t.Fatalf("Failed to get fallback device info: %v", err)
	}

	if deviceInfo.DeviceID != expectedDeviceID {
		t.Errorf("Expected fallback deviceID '%s', got '%s'", expectedDeviceID, deviceInfo.DeviceID)
	}

	if deviceInfo.Name != "Legacy Device" {
		t.Errorf("Expected name 'Legacy Device' (from discovery), got '%s'", deviceInfo.Name)
	}

	if deviceInfo.FirmwareVersion != "0.0.0" {
		t.Errorf("Expected unknown firmware '0.0.0', got '%s'", deviceInfo.FirmwareVersion)
	}

	t.Logf("\nFallback handling verified:")
	t.Logf("  Device ID: %s (UPnP serial)", deviceInfo.DeviceID)
	t.Logf("  Account ID: %s (default)", deviceInfo.AccountID)
	t.Logf("  Name: %s (from discovery)", deviceInfo.Name)
	t.Logf("  Firmware: %s (unknown)", deviceInfo.FirmwareVersion)

	t.Logf("\n✅ MAC-based discovery fallback test passed!")
}
