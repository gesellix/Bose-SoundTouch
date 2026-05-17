package datastore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

func TestMacAddressCaseSensitivity(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "case-sensitivity-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	accountID := "1000001"
	serialNumber := "I6332527703739342000020"

	// Test scenarios that could occur in production
	testCases := []struct {
		name            string
		macInDeviceInfo string
		macInRequest    string
		expectedToWork  bool
		description     string
	}{
		{
			name:            "ExactMatch",
			macInDeviceInfo: "AABBCCDDEEFF",
			macInRequest:    "AABBCCDDEEFF",
			expectedToWork:  true,
			description:     "Exact case match should work",
		},
		{
			name:            "DeviceInfoUpperRequestLower",
			macInDeviceInfo: "AABBCCDDEEFF",
			macInRequest:    "aabbccddeeff",
			expectedToWork:  true,
			description:     "DeviceInfo has uppercase, request has lowercase (should work with normalization)",
		},
		{
			name:            "DeviceInfoLowerRequestUpper",
			macInDeviceInfo: "aabbccddeeff",
			macInRequest:    "AABBCCDDEEFF",
			expectedToWork:  true,
			description:     "DeviceInfo has lowercase, request has uppercase (should work with normalization)",
		},
		{
			name:            "MixedCaseInDeviceInfo",
			macInDeviceInfo: "aaBBccddEEff",
			macInRequest:    "AABBCCDDEEFF",
			expectedToWork:  true,
			description:     "Mixed case in DeviceInfo vs uppercase request (should work with normalization)",
		},
		{
			name:            "WithColonsInDeviceInfo",
			macInDeviceInfo: "AA:BB:CC:DD:EE:FF",
			macInRequest:    "AABBCCDDEEFF",
			expectedToWork:  true,
			description:     "DeviceInfo has colons, request without (should work with normalization)",
		},
		{
			name:            "WithDashesInDeviceInfo",
			macInDeviceInfo: "AA-BB-CC-DD-EE-FF",
			macInRequest:    "AABBCCDDEEFF",
			expectedToWork:  true,
			description:     "DeviceInfo has dashes, request without (should work with normalization)",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create separate directory for this test case
			testDir := filepath.Join(tmpDir, tc.name)
			deviceDir := filepath.Join(testDir, "accounts", accountID, "devices", serialNumber)
			if err := os.MkdirAll(deviceDir, 0755); err != nil {
				t.Fatalf("failed to create device dir: %v", err)
			}

			// Create DeviceInfo.xml with specific MAC format
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
        <macAddress>` + tc.macInDeviceInfo + `</macAddress>
        <ipAddress>192.0.2.100</ipAddress>
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

			// Check mapping
			ds.idMutex.RLock()
			mappedSerial, hasMappingForRequest := ds.deviceMappings[tc.macInRequest]
			mappedSerialFromDeviceInfo, hasMappingForDeviceInfo := ds.deviceMappings[tc.macInDeviceInfo]
			ds.idMutex.RUnlock()

			t.Logf("%s:", tc.description)
			t.Logf("  MAC in DeviceInfo.xml: '%s'", tc.macInDeviceInfo)
			t.Logf("  MAC in request: '%s'", tc.macInRequest)
			t.Logf("  Mapping exists for request MAC: %v", hasMappingForRequest)
			t.Logf("  Mapping exists for DeviceInfo MAC: %v", hasMappingForDeviceInfo)

			if hasMappingForRequest {
				t.Logf("  Request MAC '%s' maps to serial: '%s'", tc.macInRequest, mappedSerial)
			}
			if hasMappingForDeviceInfo {
				t.Logf("  DeviceInfo MAC '%s' maps to serial: '%s'", tc.macInDeviceInfo, mappedSerialFromDeviceInfo)
			}

			// Try GetPresets
			_, err := ds.GetPresets(accountID, tc.macInRequest)
			worked := err == nil

			if tc.expectedToWork && !worked {
				t.Errorf("Expected success but got error: %v", err)
			} else if !tc.expectedToWork && worked {
				t.Errorf("Expected failure but got success")
			} else if worked {
				t.Logf("  ✓ Successfully resolved MAC '%s'", tc.macInRequest)
			} else {
				t.Logf("  ✓ Correctly failed to resolve MAC '%s'", tc.macInRequest)
			}
		})
	}
}

// TestProductionScenarioSimulation simulates the exact issue described
func TestProductionScenarioSimulation(t *testing.T) {
	// This test specifically simulates the production scenario where:
	// Request: GET /streaming/account/1000001/device/AABBCCDDEEFF/presets
	// File exists at: /var/lib/soundtouch-service/accounts/1000001/devices/I6332527703739342000020/Presets.xml

	tmpDir, err := os.MkdirTemp("", "production-scenario")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	accountID := "1000001"
	serialNumber := "I6332527703739342000020"
	requestMAC := "AABBCCDDEEFF"

	// Test different MAC address formats that could be in DeviceInfo.xml
	possibleMACFormats := []string{
		"AABBCCDDEEFF",      // Exact match
		"aabbccddeeff",      // All lowercase
		"AAbbccddEEff",      // Mixed case
		"AA:BB:CC:DD:EE:FF", // With colons
		"AA-BB-CC-DD-EE-FF", // With dashes
		"aa:bb:cc:dd:ee:ff", // Lowercase with colons
		"aa-bb-cc-dd-ee-ff", // Lowercase with dashes
	}

	t.Logf("Production scenario simulation:")
	t.Logf("Request URL: GET /streaming/account/%s/device/%s/presets", accountID, requestMAC)
	t.Logf("Expected file location: accounts/%s/devices/%s/Presets.xml", accountID, serialNumber)
	t.Logf("")

	for i, macFormat := range possibleMACFormats {
		t.Run(fmt.Sprintf("MACFormat_%d", i), func(t *testing.T) {
			// Create fresh directory for this test
			testDir := filepath.Join(tmpDir, fmt.Sprintf("test_%d", i))
			deviceDir := filepath.Join(testDir, "accounts", accountID, "devices", serialNumber)
			if err := os.MkdirAll(deviceDir, 0755); err != nil {
				t.Fatalf("failed to create device dir: %v", err)
			}

			// Create DeviceInfo.xml with this MAC format
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
        <macAddress>` + macFormat + `</macAddress>
        <ipAddress>192.0.2.100</ipAddress>
    </networkInfo>
</info>`
			if err := os.WriteFile(filepath.Join(deviceDir, constants.DeviceInfoFile), []byte(deviceInfoXML), 0644); err != nil {
				t.Fatalf("failed to write DeviceInfo.xml: %v", err)
			}

			// Create the target file that should be found
			presetsXML := `<presets><preset id="1">test</preset></presets>`
			if err := os.WriteFile(filepath.Join(deviceDir, constants.PresetsFile), []byte(presetsXML), 0644); err != nil {
				t.Fatalf("failed to write Presets.xml: %v", err)
			}

			// Initialize datastore
			ds := NewDataStore(testDir)
			if err := ds.Initialize(); err != nil {
				t.Fatalf("failed to initialize datastore: %v", err)
			}

			// Try to access using the request MAC
			presets, err := ds.GetPresets(accountID, requestMAC)

			if err == nil {
				t.Logf("✓ SUCCESS: MAC format '%s' in DeviceInfo allows request with '%s' to work (%d presets found)",
					macFormat, requestMAC, len(presets))
			} else {
				t.Logf("✗ FAILED: MAC format '%s' in DeviceInfo does not allow request with '%s' (error: %v)",
					macFormat, requestMAC, err)
			}

			// Check what actually got mapped
			ds.idMutex.RLock()
			for mac, serial := range ds.deviceMappings {
				t.Logf("  Mapping: '%s' -> '%s'", mac, serial)
			}
			ds.idMutex.RUnlock()
		})
	}
}

// TestNormalizationSuggestion tests if we should implement MAC address normalization
func TestNormalizationSuggestion(t *testing.T) {
	// This test demonstrates how MAC address normalization could solve the issue

	normalizeMAC := func(mac string) string {
		// Remove common separators and convert to uppercase
		mac = strings.ReplaceAll(mac, ":", "")
		mac = strings.ReplaceAll(mac, "-", "")
		mac = strings.ToUpper(mac)
		return mac
	}

	testCases := []struct {
		original   string
		normalized string
	}{
		{"AABBCCDDEEFF", "AABBCCDDEEFF"},
		{"aabbccddeeff", "AABBCCDDEEFF"},
		{"AA:BB:CC:DD:EE:FF", "AABBCCDDEEFF"},
		{"aa:bb:cc:dd:ee:ff", "AABBCCDDEEFF"},
		{"AA-BB-CC-DD-EE-FF", "AABBCCDDEEFF"},
		{"aa-bb-cc-dd-ee-ff", "AABBCCDDEEFF"},
		{"aaBBccddEEff", "AABBCCDDEEFF"},
	}

	t.Log("MAC Address Normalization Test:")
	t.Log("This shows how normalization could solve case/format sensitivity issues")
	t.Log("")

	allNormalizedSame := true
	expectedNormalized := "AABBCCDDEEFF"

	for _, tc := range testCases {
		normalized := normalizeMAC(tc.original)
		matches := normalized == expectedNormalized

		if !matches {
			allNormalizedSame = false
		}

		t.Logf("'%s' -> '%s' (matches expected: %v)", tc.original, normalized, matches)
	}

	if allNormalizedSame {
		t.Log("")
		t.Log("✓ All MAC address formats normalize to the same value")
		t.Log("✓ Implementing normalization would solve case/format sensitivity issues")
	} else {
		t.Error("✗ Normalization failed to produce consistent results")
	}
}
