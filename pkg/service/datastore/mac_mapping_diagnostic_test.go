package datastore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

func TestMacMappingDiagnostic(t *testing.T) {
	// Test the exact scenario described in the issue
	tmpDir, err := os.MkdirTemp("", "mac-mapping-diagnostic")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	accountID := "3230304"
	serialNumber := "I6332527703739342000020"
	macAddress := "A81B6A536A98"

	// Create the directory structure as it exists in production
	deviceDir := filepath.Join(tmpDir, "accounts", accountID, "devices", serialNumber)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("failed to create device dir: %v", err)
	}

	// Create DeviceInfo.xml with the MAC address
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="` + serialNumber + `">
    <name>SoundTouch Device</name>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>4.8.1</softwareVersion>
            <serialNumber>` + serialNumber + `</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>` + macAddress + `</macAddress>
        <ipAddress>192.168.1.100</ipAddress>
    </networkInfo>
</info>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.DeviceInfoFile), []byte(deviceInfoXML), 0644); err != nil {
		t.Fatalf("failed to write DeviceInfo.xml: %v", err)
	}

	// Create Presets.xml to simulate the file that should be found
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1">
        <ContentItem source="SPOTIFY" type="station" location="/station/abc123" sourceAccount="spotify_user">
            <itemName>My Preset</itemName>
        </ContentItem>
    </preset>
</presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.PresetsFile), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("failed to write Presets.xml: %v", err)
	}

	// Initialize the datastore
	ds := NewDataStore(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("failed to initialize datastore: %v", err)
	}

	// Test 1: Check if the mapping was populated
	t.Run("CheckMappingPopulation", func(t *testing.T) {
		ds.idMutex.RLock()
		serial, ok := ds.macToSerial[macAddress]
		ds.idMutex.RUnlock()

		if !ok {
			t.Errorf("MAC address %s not found in mapping", macAddress)
		} else if serial != serialNumber {
			t.Errorf("MAC address %s mapped to %s, expected %s", macAddress, serial, serialNumber)
		} else {
			t.Logf("✓ MAC address %s correctly mapped to %s", macAddress, serial)
		}
	})

	// Test 2: Check AccountDeviceDir resolution
	t.Run("CheckAccountDeviceDir", func(t *testing.T) {
		// Test with MAC address (should resolve to serial)
		resolvedDir := ds.AccountDeviceDir(accountID, macAddress)
		expectedDir := filepath.Join(tmpDir, "accounts", accountID, "devices", serialNumber)

		if resolvedDir != expectedDir {
			t.Errorf("AccountDeviceDir with MAC %s resolved to %s, expected %s", macAddress, resolvedDir, expectedDir)
		} else {
			t.Logf("✓ AccountDeviceDir correctly resolved MAC %s to path %s", macAddress, resolvedDir)
		}

		// Test with serial number (should work as-is)
		resolvedDirSerial := ds.AccountDeviceDir(accountID, serialNumber)
		if resolvedDirSerial != expectedDir {
			t.Errorf("AccountDeviceDir with serial %s resolved to %s, expected %s", serialNumber, resolvedDirSerial, expectedDir)
		} else {
			t.Logf("✓ AccountDeviceDir works correctly with serial number %s", serialNumber)
		}
	})

	// Test 3: Check GetPresets functionality with MAC address
	t.Run("CheckGetPresetsWithMAC", func(t *testing.T) {
		presets, err := ds.GetPresets(accountID, macAddress)
		if err != nil {
			t.Errorf("GetPresets failed with MAC address %s: %v", macAddress, err)
		} else if len(presets) == 0 {
			t.Error("GetPresets returned no presets")
		} else {
			t.Logf("✓ GetPresets successfully returned %d presets using MAC address %s", len(presets), macAddress)
		}
	})

	// Test 4: Check GetPresets functionality with serial number
	t.Run("CheckGetPresetsWithSerial", func(t *testing.T) {
		presets, err := ds.GetPresets(accountID, serialNumber)
		if err != nil {
			t.Errorf("GetPresets failed with serial number %s: %v", serialNumber, err)
		} else if len(presets) == 0 {
			t.Error("GetPresets returned no presets")
		} else {
			t.Logf("✓ GetPresets successfully returned %d presets using serial number %s", len(presets), serialNumber)
		}
	})

	// Test 5: Check case sensitivity
	t.Run("CheckCaseSensitivity", func(t *testing.T) {
		lowercaseMAC := "a81b6a536a98"
		uppercaseMAC := "A81B6A536A98"

		ds.idMutex.RLock()
		_, lowercaseOk := ds.macToSerial[lowercaseMAC]
		_, uppercaseOk := ds.macToSerial[uppercaseMAC]
		ds.idMutex.RUnlock()

		t.Logf("Lowercase MAC '%s' in mapping: %v", lowercaseMAC, lowercaseOk)
		t.Logf("Uppercase MAC '%s' in mapping: %v", uppercaseMAC, uppercaseOk)

		// Test GetPresets with different cases
		_, errLower := ds.GetPresets(accountID, lowercaseMAC)
		_, errUpper := ds.GetPresets(accountID, uppercaseMAC)

		t.Logf("GetPresets with lowercase MAC error: %v", errLower)
		t.Logf("GetPresets with uppercase MAC error: %v", errUpper)
	})

	// Test 6: Dump all mappings for debugging
	t.Run("DumpMappings", func(t *testing.T) {
		ds.idMutex.RLock()
		defer ds.idMutex.RUnlock()

		t.Logf("Total mappings found: %d", len(ds.macToSerial))
		for mac, serial := range ds.macToSerial {
			t.Logf("  MAC '%s' -> Serial '%s'", mac, serial)
		}
	})

	// Test 7: Check actual file paths
	t.Run("CheckFilePaths", func(t *testing.T) {
		// Path that should work (with serial number)
		correctPath := filepath.Join(tmpDir, "accounts", accountID, "devices", serialNumber, constants.PresetsFile)
		if _, err := os.Stat(correctPath); err != nil {
			t.Errorf("File not found at correct path %s: %v", correctPath, err)
		} else {
			t.Logf("✓ File found at correct path: %s", correctPath)
		}

		// Path that would be wrong (with MAC address)
		wrongPath := filepath.Join(tmpDir, "accounts", accountID, "devices", macAddress, constants.PresetsFile)
		if _, err := os.Stat(wrongPath); err == nil {
			t.Logf("⚠️  File also found at MAC path (unexpected): %s", wrongPath)
		} else {
			t.Logf("✓ File correctly not found at MAC path: %s", wrongPath)
		}
	})
}

// TestMacMappingWithDifferentFormats tests various MAC address formats
func TestMacMappingWithDifferentFormats(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mac-format-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		name         string
		macInXML     string
		macInRequest string
		shouldWork   bool
	}{
		{"ExactMatch", "A81B6A536A98", "A81B6A536A98", true},
		{"LowerCase", "A81B6A536A98", "a81b6a536a98", true},       // Should work with normalization
		{"UpperCase", "a81b6a536a98", "A81B6A536A98", true},       // Should work with normalization
		{"WithColons", "A8:1B:6A:53:6A:98", "A81B6A536A98", true}, // Should work with normalization
		{"WithDashes", "A8-1B-6A-53-6A-98", "A81B6A536A98", true}, // Should work with normalization
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create separate directory for each test case
			testDir := filepath.Join(tmpDir, tc.name)
			accountID := "12345"
			serialNumber := "TEST123456789"

			deviceDir := filepath.Join(testDir, "accounts", accountID, "devices", serialNumber)
			if err := os.MkdirAll(deviceDir, 0755); err != nil {
				t.Fatalf("failed to create device dir: %v", err)
			}

			// Create DeviceInfo.xml with the specific MAC format
			deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="` + serialNumber + `">
    <name>Test Device</name>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>4.8.1</softwareVersion>
            <serialNumber>` + serialNumber + `</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>` + tc.macInXML + `</macAddress>
        <ipAddress>192.168.1.100</ipAddress>
    </networkInfo>
</info>`
			if err := os.WriteFile(filepath.Join(deviceDir, constants.DeviceInfoFile), []byte(deviceInfoXML), 0644); err != nil {
				t.Fatalf("failed to write DeviceInfo.xml: %v", err)
			}

			// Create Presets.xml
			presetsXML := `<presets><preset id="1">test</preset></presets>`
			if err := os.WriteFile(filepath.Join(deviceDir, constants.PresetsFile), []byte(presetsXML), 0644); err != nil {
				t.Fatalf("failed to write Presets.xml: %v", err)
			}

			// Initialize datastore
			ds := NewDataStore(testDir)
			if err := ds.Initialize(); err != nil {
				t.Fatalf("failed to initialize datastore: %v", err)
			}

			// Try to get presets using the request MAC format
			_, err := ds.GetPresets(accountID, tc.macInRequest)

			if tc.shouldWork && err != nil {
				t.Errorf("Expected success but got error: %v", err)
			} else if !tc.shouldWork && err == nil {
				t.Errorf("Expected failure but got success")
			} else if tc.shouldWork {
				t.Logf("✓ Successfully resolved MAC '%s' to serial '%s' (normalization worked)", tc.macInRequest, serialNumber)
			} else {
				t.Logf("✓ Correctly failed to resolve MAC '%s' (XML had '%s')", tc.macInRequest, tc.macInXML)
			}
		})
	}
}
