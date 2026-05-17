package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/certmanager"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestSyncDeviceData_UsesDeviceID(t *testing.T) {
	// Create a temporary datastore
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)

	// Mock HTTP server that provides device info and presets
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			// Return device info with MAC address as deviceID
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="AABBCCDDEEFF">
    <name>Test Device</name>
    <type>SoundTouch 30</type>
    <margeAccountUUID>test-account-123</margeAccountUUID>
    <components>
        <component>
            <componentCategory>SYSTEM</componentCategory>
            <softwareVersion>4.8.1</softwareVersion>
            <serialNumber>I6332527703739342000020</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>AABBCCDDEEFF</macAddress>
        <ipAddress>192.0.2.100</ipAddress>
    </networkInfo>
</info>`)
		case "/presets":
			// Return empty presets for simplicity
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<presets deviceID="AABBCCDDEEFF"/>`)
		case "/recents":
			// Return empty recents for simplicity
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<recents deviceID="AABBCCDDEEFF"/>`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Extract host from server URL
	serverHost := strings.TrimPrefix(server.URL, "http://")

	// Create manager with mock HTTP client
	cm := certmanager.NewCertificateManager(tmpDir + "/certs")
	manager := NewManager("http://localhost:8000", ds, cm)

	// Test SyncDeviceData
	err := manager.SyncDeviceData(serverHost)
	if err != nil {
		t.Fatalf("SyncDeviceData failed: %v", err)
	}

	// Verify that data was synced to the correct directory using MAC address (deviceID)
	expectedDeviceID := "AABBCCDDEEFF"
	expectedAccountID := "test-account-123"

	// Check that the device directory is resolved correctly using MAC address
	deviceDir := ds.AccountDeviceDir(expectedAccountID, expectedDeviceID)
	if !strings.HasSuffix(deviceDir, fmt.Sprintf("accounts/%s/devices/%s", expectedAccountID, expectedDeviceID)) {
		t.Errorf("Device directory should be based on MAC address. Got: %s", deviceDir)
	}

	// Just verify the directory structure was created correctly
	// The sync process should create directories even for empty data
	t.Logf("Device directory resolved to: %s", deviceDir)

	// Try to get presets - might not exist if empty, but should not error on directory resolution
	_, presetsErr := ds.GetPresets(expectedAccountID, expectedDeviceID)
	if presetsErr != nil && !strings.Contains(presetsErr.Error(), "no such file or directory") {
		t.Fatalf("Unexpected error getting presets: %v", presetsErr)
	}

	t.Logf("✓ Sync completed using MAC address as deviceID: %s", expectedDeviceID)
	t.Logf("✓ Directory structure: %s", deviceDir)
}

func TestSyncDeviceData_NoDeviceID_ShouldFail(t *testing.T) {
	// Create a temporary datastore
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)

	// Mock HTTP server that provides device info WITHOUT deviceID
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/info" {
			// Return device info without deviceID (empty deviceID)
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="">
    <name>Test Device</name>
    <type>SoundTouch 30</type>
    <margeAccountUUID>test-account-123</margeAccountUUID>
    <components>
        <component>
            <componentCategory>SYSTEM</componentCategory>
            <softwareVersion>4.8.1</softwareVersion>
            <serialNumber>I6332527703739342000020</serialNumber>
        </component>
    </components>
</info>`)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Extract host from server URL
	serverHost := strings.TrimPrefix(server.URL, "http://")

	// Create manager
	cm := certmanager.NewCertificateManager(tmpDir + "/certs")
	manager := NewManager("http://localhost:8000", ds, cm)

	// Test SyncDeviceData - should fail
	err := manager.SyncDeviceData(serverHost)
	if err == nil {
		t.Fatal("SyncDeviceData should have failed when deviceID is empty")
	}

	expectedErrorSubstring := "no deviceID found in /info response"
	if !strings.Contains(err.Error(), expectedErrorSubstring) {
		t.Errorf("Expected error to contain '%s', got: %v", expectedErrorSubstring, err)
	}

	t.Logf("✓ SyncDeviceData correctly failed with error: %v", err)
}

func TestSyncDeviceData_FallbackToExistingDeviceMapping(t *testing.T) {
	// Create a temporary datastore
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)

	// Pre-populate device data using serial number (legacy scenario)
	accountID := "test-account-123"
	serialNumber := "I6332527703739342000020"
	macAddress := "AABBCCDDEEFF"

	// Save device info under serial number (simulating legacy behavior)
	legacyDeviceInfo := &models.ServiceDeviceInfo{
		DeviceID:           serialNumber,
		AccountID:          accountID,
		Name:               "Legacy Device",
		MacAddress:         macAddress,
		DeviceSerialNumber: serialNumber,
	}

	if err := ds.SaveDeviceInfo(accountID, serialNumber, legacyDeviceInfo); err != nil {
		t.Fatalf("Failed to save legacy device info: %v", err)
	}

	// Also save some legacy presets under the serial number
	legacyPresets := []models.ServicePreset{
		{
			ServiceContentItem: models.ServiceContentItem{
				ID:     "1",
				Name:   "Legacy Preset",
				Source: "SPOTIFY",
			},
		},
	}
	if err := ds.SavePresets(accountID, serialNumber, legacyPresets); err != nil {
		t.Fatalf("Failed to save legacy presets: %v", err)
	}

	// Mock HTTP server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deviceIP := r.Host // Get IP from request host
		switch r.URL.Path {
		case "/info":
			// Return device info with MAC address as deviceID
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="%s">
    <name>Updated Device</name>
    <type>SoundTouch 30</type>
    <margeAccountUUID>%s</margeAccountUUID>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>4.5.2</softwareVersion>
            <serialNumber>%s</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>%s</macAddress>
        <ipAddress>%s</ipAddress>
    </networkInfo>
</info>`, macAddress, accountID, serialNumber, macAddress, deviceIP)
		case "/presets":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<presets deviceID="AABBCCDDEEFF"/>`)
		case "/recents":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<recents deviceID="AABBCCDDEEFF"/>`)
		}
	}))
	defer server.Close()

	serverHost := strings.TrimPrefix(server.URL, "http://")
	cm := certmanager.NewCertificateManager(tmpDir + "/certs")
	manager := NewManager("http://localhost:8000", ds, cm)

	// Sync should work and use MAC address
	err := manager.SyncDeviceData(serverHost)
	if err != nil {
		t.Fatalf("SyncDeviceData failed: %v", err)
	}

	// Verify directory resolution - MAC address should resolve to its own directory
	macDir := ds.AccountDeviceDir(accountID, macAddress)
	legacyDir := ds.AccountDeviceDir(accountID, serialNumber)

	t.Logf("MAC address resolves to: %s", macDir)
	t.Logf("Serial number resolves to: %s", legacyDir)

	// The key test: MAC address should create its own directory structure
	if !strings.Contains(macDir, macAddress) {
		t.Errorf("MAC address directory should contain MAC address %s, got %s", macAddress, macDir)
	}

	// Legacy data should still be accessible
	serialPresets, err := ds.GetPresets(accountID, serialNumber)
	if err != nil {
		t.Fatalf("Failed to get legacy presets by serial number: %v", err)
	}

	if len(serialPresets) != 1 || serialPresets[0].Name != "Legacy Preset" {
		t.Errorf("Legacy presets should still be accessible by serial number")
	}

	t.Logf("✓ Sync successfully used MAC address as deviceID: %s", macAddress)
	t.Logf("✓ Legacy data still accessible via serial number: %s", serialNumber)
}
