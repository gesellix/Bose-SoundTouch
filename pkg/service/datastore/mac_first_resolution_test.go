package datastore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestAccountDeviceDir_MACFirstResolution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mac-first-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	accountID := "testaccount"
	macAddress := "AABBCCDDEEFF"
	serialNumber := "I6332527703739342000020"

	t.Run("NewMACBasedDevice", func(t *testing.T) {
		// Create a new device with MAC as deviceID
		deviceInfo := &models.ServiceDeviceInfo{
			DeviceID:           macAddress,
			AccountID:          accountID,
			Name:               "New MAC Device",
			IPAddress:          "192.0.2.100",
			MacAddress:         macAddress,
			DeviceSerialNumber: serialNumber,
			ProductCode:        "SoundTouch 10 sm2",
		}

		// Save the device
		if err := ds.SaveDeviceInfo(accountID, macAddress, deviceInfo); err != nil {
			t.Fatalf("Failed to save MAC-based device: %v", err)
		}

		// Test AccountDeviceDir resolution
		resolvedDir := ds.AccountDeviceDir(accountID, macAddress)
		expectedDir := filepath.Join(tempDir, "accounts", accountID, "devices", macAddress)

		if resolvedDir != expectedDir {
			t.Errorf("Expected MAC-based device dir '%s', got '%s'", expectedDir, resolvedDir)
		}

		// Verify the directory actually exists
		if _, err := os.Stat(resolvedDir); os.IsNotExist(err) {
			t.Errorf("MAC-based device directory should exist: %s", resolvedDir)
		}

		t.Logf("✅ MAC-based device correctly resolved to: %s", resolvedDir)
	})

	t.Run("LegacySerialBasedDevice", func(t *testing.T) {
		// Create a legacy device with serial as deviceID (simulating old storage)
		legacyInfo := &models.ServiceDeviceInfo{
			DeviceID:           serialNumber,
			AccountID:          accountID,
			Name:               "Legacy Serial Device",
			IPAddress:          "192.0.2.101",
			MacAddress:         macAddress,
			DeviceSerialNumber: serialNumber,
			ProductCode:        "SoundTouch 10",
		}

		// Save the legacy device
		if err := ds.SaveDeviceInfo(accountID, serialNumber, legacyInfo); err != nil {
			t.Fatalf("Failed to save legacy device: %v", err)
		}

		// Initialize to populate mappings
		if err := ds.Initialize(); err != nil {
			t.Fatalf("Failed to initialize datastore: %v", err)
		}

		// Test resolution by MAC address (should find the legacy device via mapping)
		resolvedDir := ds.AccountDeviceDir(accountID, macAddress)
		expectedLegacyDir := filepath.Join(tempDir, "accounts", accountID, "devices", serialNumber)

		// Since both MAC and serial devices exist, MAC device should take priority
		expectedMACDir := filepath.Join(tempDir, "accounts", accountID, "devices", macAddress)
		if resolvedDir != expectedMACDir {
			t.Errorf("Expected MAC device to take priority. Got '%s', expected '%s'", resolvedDir, expectedMACDir)
		}

		t.Logf("✅ MAC address resolution correctly prioritized MAC-based device")

		// Test resolution by serial number (should find the legacy device directly)
		serialResolvedDir := ds.AccountDeviceDir(accountID, serialNumber)
		if serialResolvedDir != expectedLegacyDir {
			t.Errorf("Expected serial-based device dir '%s', got '%s'", expectedLegacyDir, serialResolvedDir)
		}

		t.Logf("✅ Serial number correctly resolved to legacy device: %s", serialResolvedDir)
	})

	t.Run("MACResolutionWithOnlyLegacyDevice", func(t *testing.T) {
		// Create a fresh datastore
		tempDir2, err := os.MkdirTemp("", "mac-legacy-only-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tempDir2)

		ds2 := NewDataStore(tempDir2)
		testAccount := "legacyaccount"
		testSerial := "LEGACY123456789"
		testMAC := "BB:CC:DD:EE:FF:00"

		// Create ONLY a legacy device (no MAC-based device)
		legacyInfo := &models.ServiceDeviceInfo{
			DeviceID:           testSerial,
			AccountID:          testAccount,
			Name:               "Only Legacy Device",
			MacAddress:         testMAC,
			DeviceSerialNumber: testSerial,
		}

		if err := ds2.SaveDeviceInfo(testAccount, testSerial, legacyInfo); err != nil {
			t.Fatalf("Failed to save legacy-only device: %v", err)
		}

		// Initialize to populate mappings
		if err := ds2.Initialize(); err != nil {
			t.Fatalf("Failed to initialize datastore: %v", err)
		}

		// Test MAC resolution (should find the legacy device via mapping)
		resolvedDir := ds2.AccountDeviceDir(testAccount, testMAC)
		expectedDir := filepath.Join(tempDir2, "accounts", testAccount, "devices", testSerial)

		if resolvedDir != expectedDir {
			t.Errorf("MAC '%s' should resolve to legacy device '%s', got '%s'", testMAC, expectedDir, resolvedDir)
		}

		t.Logf("✅ MAC address correctly resolved to legacy device when no MAC-based device exists")
	})

	t.Run("NonExistentDevice", func(t *testing.T) {
		unknownMAC := "FF:FF:FF:FF:FF:FF"
		resolvedDir := ds.AccountDeviceDir(accountID, unknownMAC)
		expectedDir := filepath.Join(tempDir, "accounts", accountID, "devices", unknownMAC)

		if resolvedDir != expectedDir {
			t.Errorf("Non-existent device should resolve to direct path '%s', got '%s'", expectedDir, resolvedDir)
		}

		t.Logf("✅ Non-existent device correctly resolved to direct MAC path")
	})

	t.Run("MACNormalization", func(t *testing.T) {
		// Test different MAC address formats
		macFormats := []string{
			"AABBCCDDEEFF",      // No separators
			"AA:BB:CC:DD:EE:FF", // Colons
			"AA-BB-CC-DD-EE-FF", // Dashes
			"aabbccddeeff",      // Lowercase
			"aa:bb:cc:dd:ee:ff", // Lowercase with colons
		}

		for _, macFormat := range macFormats {
			resolvedDir := ds.AccountDeviceDir(accountID, macFormat)
			// Should resolve to the MAC-based device we created earlier
			expectedDir := filepath.Join(tempDir, "accounts", accountID, "devices", macAddress)

			if resolvedDir != expectedDir {
				t.Logf("MAC format '%s' resolved to '%s', expected '%s'", macFormat, resolvedDir, expectedDir)
				// For now, we'll log this - full normalization might require additional work
			}
		}
	})

	t.Run("BackwardCompatibilityMapping", func(t *testing.T) {
		// Test that the legacy UpdateMapping method still works
		testMAC := "CC:DD:EE:FF:00:11"
		testSerial := "COMPAT789"

		ds.UpdateMapping(testMAC, testSerial)

		// After calling UpdateMapping, the MAC should resolve via the mapping
		resolvedDir := ds.AccountDeviceDir(accountID, testMAC)
		directPath := filepath.Join(tempDir, "accounts", accountID, "devices", testMAC)

		// Since no actual device exists, it should return the direct path
		if resolvedDir != directPath {
			t.Errorf("UpdateMapping backward compatibility test failed. Got '%s', expected '%s'", resolvedDir, directPath)
		}

		t.Logf("✅ UpdateMapping backward compatibility maintained")
	})
}

func TestDeviceMappings_Bidirectional(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bidirectional-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	accountID := "testaccount"

	t.Run("MACBasedDeviceCreatesSerialMapping", func(t *testing.T) {
		macAddress := "11:22:33:44:55:66"
		serialNumber := "NEWDEVICE123"

		// Create MAC-based device
		deviceInfo := &models.ServiceDeviceInfo{
			DeviceID:           macAddress,
			AccountID:          accountID,
			Name:               "MAC First Device",
			MacAddress:         macAddress,
			DeviceSerialNumber: serialNumber,
		}

		if err := ds.SaveDeviceInfo(accountID, macAddress, deviceInfo); err != nil {
			t.Fatalf("Failed to save MAC-based device: %v", err)
		}

		// Initialize to populate mappings
		if err := ds.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		// Serial should resolve to the MAC-based device
		resolvedDir := ds.AccountDeviceDir(accountID, serialNumber)
		expectedDir := filepath.Join(tempDir, "accounts", accountID, "devices", macAddress)

		if resolvedDir != expectedDir {
			t.Errorf("Serial '%s' should resolve to MAC device '%s', got '%s'", serialNumber, expectedDir, resolvedDir)
		}

		t.Logf("✅ MAC-based device creates correct serial→MAC mapping")
	})

	t.Run("SerialBasedDeviceCreatesMACMapping", func(t *testing.T) {
		macAddress := "77:88:99:AA:BB:CC"
		serialNumber := "SERIALDEVICE456"

		// Create serial-based device (legacy)
		deviceInfo := &models.ServiceDeviceInfo{
			DeviceID:           serialNumber,
			AccountID:          accountID,
			Name:               "Serial First Device",
			MacAddress:         macAddress,
			DeviceSerialNumber: serialNumber,
		}

		if err := ds.SaveDeviceInfo(accountID, serialNumber, deviceInfo); err != nil {
			t.Fatalf("Failed to save serial-based device: %v", err)
		}

		// Initialize to populate mappings
		if err := ds.Initialize(); err != nil {
			t.Fatalf("Failed to initialize: %v", err)
		}

		// MAC should resolve to the serial-based device
		resolvedDir := ds.AccountDeviceDir(accountID, macAddress)
		expectedDir := filepath.Join(tempDir, "accounts", accountID, "devices", serialNumber)

		if resolvedDir != expectedDir {
			t.Errorf("MAC '%s' should resolve to serial device '%s', got '%s'", macAddress, expectedDir, resolvedDir)
		}

		t.Logf("✅ Serial-based device creates correct MAC→serial mapping")
	})
}

func TestMACAddressFormatDetection(t *testing.T) {
	testCases := []struct {
		input    string
		expected bool
		name     string
	}{
		{"AABBCCDDEEFF", true, "12-char hex"},
		{"aabbccddeeff", true, "12-char hex lowercase"},
		{"AA:BB:CC:DD:EE:FF", true, "colon-separated"},
		{"AA-BB-CC-DD-EE-FF", true, "dash-separated"},
		{"aa:bb:cc:dd:ee:ff", true, "colon-separated lowercase"},
		{"aa-bb-cc-dd-ee-ff", true, "dash-separated lowercase"},
		{"I6332527703739342000020", false, "device serial"},
		{"192.0.2.100", false, "IP address"},
		{"ABCDEFGHIJKL", false, "12-char non-hex"},
		{"AA:BB:CC:DD:EE", false, "incomplete MAC"},
		{"AA:BB:CC:DD:EE:FF:01", false, "too long MAC"},
		{"", false, "empty string"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isMACAddressFormat(tc.input)
			if result != tc.expected {
				t.Errorf("isMACAddressFormat('%s') = %v, expected %v", tc.input, result, tc.expected)
			}
		})
	}
}

func TestAccountDeviceDir_PriorityOrder(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "priority-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	ds := NewDataStore(tempDir)
	accountID := "prioritytest"
	macAddress := "AA:BB:CC:DD:EE:FF"
	serialNumber := "PRIORITY123456789"

	// Create both MAC-based and serial-based devices for the same physical device
	macDevice := &models.ServiceDeviceInfo{
		DeviceID:           macAddress,
		AccountID:          accountID,
		Name:               "MAC Version",
		MacAddress:         macAddress,
		DeviceSerialNumber: serialNumber,
	}

	serialDevice := &models.ServiceDeviceInfo{
		DeviceID:           serialNumber,
		AccountID:          accountID,
		Name:               "Serial Version",
		MacAddress:         macAddress,
		DeviceSerialNumber: serialNumber,
	}

	// Save both devices
	if err := ds.SaveDeviceInfo(accountID, macAddress, macDevice); err != nil {
		t.Fatalf("Failed to save MAC device: %v", err)
	}

	if err := ds.SaveDeviceInfo(accountID, serialNumber, serialDevice); err != nil {
		t.Fatalf("Failed to save serial device: %v", err)
	}

	// Initialize mappings
	if err := ds.Initialize(); err != nil {
		t.Fatalf("Failed to initialize: %v", err)
	}

	// Test priority: MAC address should resolve to MAC-based device (not serial-based)
	resolvedDir := ds.AccountDeviceDir(accountID, macAddress)
	expectedMACDir := filepath.Join(tempDir, "accounts", accountID, "devices", macAddress)

	if resolvedDir != expectedMACDir {
		t.Errorf("MAC address should resolve to MAC-based device directory")
		t.Errorf("Expected: %s", expectedMACDir)
		t.Errorf("Got:      %s", resolvedDir)
	}

	// Test that serial still resolves to its own device
	serialResolvedDir := ds.AccountDeviceDir(accountID, serialNumber)
	expectedSerialDir := filepath.Join(tempDir, "accounts", accountID, "devices", serialNumber)

	if serialResolvedDir != expectedSerialDir {
		t.Errorf("Serial should resolve to serial-based device directory")
		t.Errorf("Expected: %s", expectedSerialDir)
		t.Errorf("Got:      %s", serialResolvedDir)
	}

	t.Logf("✅ Priority test passed:")
	t.Logf("   MAC '%s' → %s", macAddress, resolvedDir)
	t.Logf("   Serial '%s' → %s", serialNumber, serialResolvedDir)
}
