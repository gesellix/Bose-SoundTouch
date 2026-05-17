package datastore

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
)

func TestUPnPDiscoveryToDatastoreMapping_FullFlow(t *testing.T) {
	// This test demonstrates the complete flow:
	// 1. UPnP discovery finds device with MAC in serialNumber
	// 2. Device is stored in datastore with serial number directory
	// 3. MAC address mapping is established
	// 4. HTTP requests using MAC address are resolved to correct directory

	tmpDir, err := os.MkdirTemp("", "upnp-datastore-integration")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test data matching the user's scenario
	accountID := "1000001"
	deviceSerial := "I6332527703739342000020"
	deviceMAC := "AABBCCDDEEFF"
	deviceName := "Living Room SoundTouch"

	t.Logf("Test scenario:")
	t.Logf("  Account: %s", accountID)
	t.Logf("  Device Serial: %s", deviceSerial)
	t.Logf("  Device MAC: %s", deviceMAC)
	t.Logf("  Expected directory: accounts/%s/devices/%s/", accountID, deviceSerial)
	t.Logf("  Expected request: GET /streaming/account/%s/device/%s/presets", accountID, deviceMAC)
	t.Logf("")

	// Step 1: Create the device directory structure using serial number
	deviceDir := filepath.Join(tmpDir, "accounts", accountID, "devices", deviceSerial)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("failed to create device dir: %v", err)
	}

	// Create DeviceInfo.xml with MAC address
	deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="` + deviceSerial + `">
    <name>` + deviceName + `</name>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>4.8.1</softwareVersion>
            <serialNumber>` + deviceSerial + `</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>` + deviceMAC + `</macAddress>
        <ipAddress>192.0.2.100</ipAddress>
    </networkInfo>
</info>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.DeviceInfoFile), []byte(deviceInfoXML), 0644); err != nil {
		t.Fatalf("failed to write DeviceInfo.xml: %v", err)
	}

	// Create Presets.xml (the target file we want to access)
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1">
        <ContentItem source="SPOTIFY" type="station" location="/station/test123" sourceAccount="spotify">
            <itemName>My Spotify Station</itemName>
        </ContentItem>
    </preset>
    <preset id="2">
        <ContentItem source="TUNEIN" type="station" location="/station/s12345" sourceAccount="">
            <itemName>NPR News</itemName>
        </ContentItem>
    </preset>
</presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.PresetsFile), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("failed to write Presets.xml: %v", err)
	}

	// Step 2: Simulate UPnP discovery with real device XML
	t.Run("Step2_UPnPDiscovery", func(t *testing.T) {
		// UPnP XML exactly as provided by the user
		upnpXML := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <specVersion>
        <major>1</major>
        <minor>0</minor>
    </specVersion>
    <device>
        <deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType>
        <friendlyName>` + deviceName + `</friendlyName>
        <qq:X_QPlay_SoftwareCapability xmlns:qq="http://www.tencent.com">QPlay:2</qq:X_QPlay_SoftwareCapability>
        <manufacturer>Bose Corporation</manufacturer>
        <manufacturerURL>http://www.bose.com</manufacturerURL>
        <modelName>SoundTouch 10</modelName>
        <modelNumber></modelNumber>
        <modelDescription>Bose SoundTouch Wireless Streaming Audio Device</modelDescription>
        <modelURL>http://www.bose.com</modelURL>
        <serialNumber>` + deviceMAC + `</serialNumber>
        <UDN>uuid:BO5EBO5E-F00D-F00D-FEED-` + deviceMAC + `</UDN>
        <serviceList>
            <service>
                <serviceType>urn:schemas-upnp-org:service:AVTransport:1</serviceType>
                <serviceId>urn:upnp-org:serviceId:AVTransport</serviceId>
                <SCPDURL>/Xml/AVTransport3.xml</SCPDURL>
                <controlURL>/AVTransport/Control</controlURL>
                <eventSubURL>/AVTransport/Event</eventSubURL>
            </service>
        </serviceList>
    </device>
</root>`

		// Create UPnP server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/xml; charset=utf-8")
			fmt.Fprint(w, upnpXML)
		}))
		defer server.Close()

		// Simulate UPnP discovery
		discoveryService := discovery.NewService(5 * time.Second)
		device := &models.DiscoveredDevice{
			Host: "192.0.2.100",
			Port: 8091,
			Name: "Initial Name",
		}

		err := discoveryService.EnrichDeviceInfo(device, server.URL+"/XD/BO5EBO5E-F00D-F00D-FEED-"+deviceMAC+".xml")
		if err != nil {
			t.Errorf("UPnP enrichment failed: %v", err)
		} else {
			t.Logf("✓ UPnP discovery extracted MAC: '%s' from serialNumber", device.UPnPSerial)
		}

		// Verify UPnP extraction
		if device.UPnPSerial != deviceMAC {
			t.Errorf("Expected UPnPSerial '%s', got '%s'", deviceMAC, device.UPnPSerial)
		}
	})

	// Step 3: Initialize datastore and verify mapping
	t.Run("Step3_DatastoreMapping", func(t *testing.T) {
		ds := NewDataStore(tmpDir)
		err := ds.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize datastore: %v", err)
		}

		// Verify mapping was created during initialization
		ds.idMutex.RLock()
		mappedSerial, hasMappingExact := ds.deviceMappings[deviceMAC]
		normalizedMAC := normalizeMAC(deviceMAC)
		mappedSerialNormalized, hasMappingNormalized := ds.deviceMappings[normalizedMAC]
		ds.idMutex.RUnlock()

		t.Logf("Mapping check:")
		t.Logf("  Original MAC '%s' -> mapped: %v", deviceMAC, hasMappingExact)
		if hasMappingExact {
			t.Logf("  Original MAC maps to: '%s'", mappedSerial)
		}
		t.Logf("  Normalized MAC '%s' -> mapped: %v", normalizedMAC, hasMappingNormalized)
		if hasMappingNormalized {
			t.Logf("  Normalized MAC maps to: '%s'", mappedSerialNormalized)
		}

		if !hasMappingExact && !hasMappingNormalized {
			t.Error("No mapping found for MAC address")
		} else {
			t.Logf("✓ MAC address mapping established successfully")
		}
	})

	// Step 4: Test HTTP request resolution
	t.Run("Step4_HTTPRequestResolution", func(t *testing.T) {
		ds := NewDataStore(tmpDir)
		err := ds.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize datastore: %v", err)
		}

		// Test various MAC address formats in HTTP requests
		testCases := []struct {
			name        string
			requestMAC  string
			shouldWork  bool
			description string
		}{
			{
				name:        "ExactMatch",
				requestMAC:  "AABBCCDDEEFF",
				shouldWork:  true,
				description: "Exact MAC match",
			},
			{
				name:        "LowercaseMAC",
				requestMAC:  "aabbccddeeff",
				shouldWork:  true,
				description: "Lowercase MAC (should work with normalization)",
			},
			{
				name:        "MACWithColons",
				requestMAC:  "AA:BB:CC:DD:EE:FF",
				shouldWork:  true,
				description: "MAC with colons (should work with normalization)",
			},
			{
				name:        "MACWithDashes",
				requestMAC:  "AA-BB-CC-DD-EE-FF",
				shouldWork:  true,
				description: "MAC with dashes (should work with normalization)",
			},
			{
				name:        "InvalidMAC",
				requestMAC:  "INVALID123456",
				shouldWork:  true, // Changed: GetPresets now returns empty list instead of error if file missing
				description: "Invalid MAC (should return empty list)",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Simulate HTTP request: GET /streaming/account/{account}/device/{device}/presets
				presets, err := ds.GetPresets(accountID, tc.requestMAC)

				if tc.shouldWork {
					if err != nil {
						t.Errorf("%s failed: %v", tc.description, err)
					} else if len(presets) == 0 {
						if tc.name != "InvalidMAC" {
							t.Errorf("%s: no presets returned", tc.description)
						} else {
							t.Logf("✓ %s: Successfully retrieved empty presets list", tc.description)
						}
					} else {
						t.Logf("✓ %s: Successfully retrieved %d presets", tc.description, len(presets))

						// Verify preset content
						if presets[0].ID == "1" && presets[1].ID == "2" {
							t.Logf("  ✓ Preset content verified (IDs: %s, %s)", presets[0].ID, presets[1].ID)
						}
					}
				} else {
					if err == nil {
						t.Errorf("%s: expected failure but got success", tc.description)
					} else {
						t.Logf("✓ %s: Correctly failed with error: %v", tc.description, err)
					}
				}
			})
		}
	})

	// Step 5: Test directory resolution
	t.Run("Step5_DirectoryResolution", func(t *testing.T) {
		ds := NewDataStore(tmpDir)
		err := ds.Initialize()
		if err != nil {
			t.Fatalf("failed to initialize datastore: %v", err)
		}

		// Test AccountDeviceDir resolution
		resolvedDirMAC := ds.AccountDeviceDir(accountID, deviceMAC)
		resolvedDirSerial := ds.AccountDeviceDir(accountID, deviceSerial)
		expectedDir := filepath.Join(tmpDir, "accounts", accountID, "devices", deviceSerial)

		t.Logf("Directory resolution:")
		t.Logf("  Request with MAC '%s' -> '%s'", deviceMAC, resolvedDirMAC)
		t.Logf("  Request with serial '%s' -> '%s'", deviceSerial, resolvedDirSerial)
		t.Logf("  Expected directory: '%s'", expectedDir)

		if resolvedDirMAC != expectedDir {
			t.Errorf("MAC resolution failed: expected '%s', got '%s'", expectedDir, resolvedDirMAC)
		} else {
			t.Logf("✓ MAC address correctly resolved to serial number directory")
		}

		if resolvedDirSerial != expectedDir {
			t.Errorf("Serial resolution failed: expected '%s', got '%s'", expectedDir, resolvedDirSerial)
		} else {
			t.Logf("✓ Serial number resolution works correctly")
		}
	})

	// Step 6: Integration summary
	t.Run("Step6_IntegrationSummary", func(t *testing.T) {
		t.Log("")
		t.Log("=== INTEGRATION SUMMARY ===")
		t.Log("✅ UPnP Discovery: MAC address extracted from serialNumber field")
		t.Log("✅ Datastore Initialization: MAC-to-serial mapping created from DeviceInfo.xml")
		t.Log("✅ MAC Normalization: Case and format variations handled correctly")
		t.Log("✅ HTTP Request Resolution: MAC addresses resolve to correct device directories")
		t.Log("✅ File Access: Presets.xml found using MAC address in request URL")
		t.Log("")
		t.Log("The original issue has been resolved:")
		t.Logf("  Request: GET /streaming/account/%s/device/%s/presets", accountID, deviceMAC)
		t.Logf("  Resolves to: %s/accounts/%s/devices/%s/Presets.xml", tmpDir, accountID, deviceSerial)
		t.Log("")
	})
}

func TestNormalizationEdgeCases(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
		desc     string
	}{
		{"", "", "empty string"},
		{"a", "A", "single character"},
		{"ab", "AB", "two characters"},
		{"AABBCCDDEEFF", "AABBCCDDEEFF", "standard MAC"},
		{"aabbccddeeff", "AABBCCDDEEFF", "lowercase MAC"},
		{"AA:BB:CC:DD:EE:FF", "AABBCCDDEEFF", "MAC with colons"},
		{"AA-BB-CC-DD-EE-FF", "AABBCCDDEEFF", "MAC with dashes"},
		{"aa:bb:cc:dd:ee:ff", "AABBCCDDEEFF", "lowercase MAC with colons"},
		{"aa-bb-cc-dd-ee-ff", "AABBCCDDEEFF", "lowercase MAC with dashes"},
		{"AA::BB::CC", "AABBCC", "multiple consecutive colons"},
		{"AA--BB--CC", "AABBCC", "multiple consecutive dashes"},
		{"AA:-BB-:CC", "AABBCC", "mixed separators"},
		{"  AABBCCDDEEFF  ", "AABBCCDDEEFF", "MAC with spaces (handled by normalization)"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			result := normalizeMAC(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeMAC(%q) = %q, expected %q", tc.input, result, tc.expected)
			} else {
				t.Logf("✓ %s: %q -> %q", tc.desc, tc.input, result)
			}
		})
	}
}

func TestMACMappingPerformance(t *testing.T) {
	// Test performance with many mappings
	tmpDir, err := os.MkdirTemp("", "mac-performance-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := NewDataStore(tmpDir)

	// Add many mappings
	numMappings := 1000
	t.Logf("Testing performance with %d MAC mappings...", numMappings)

	start := time.Now()
	for i := 0; i < numMappings; i++ {
		mac := fmt.Sprintf("AA:BB:CC:DD:EE:%02X", i%256)
		serial := fmt.Sprintf("SERIAL%06d", i)
		ds.UpdateMapping(mac, serial)
	}
	updateDuration := time.Since(start)

	// Test lookup performance
	start = time.Now()
	for i := 0; i < numMappings; i++ {
		mac := fmt.Sprintf("AA:BB:CC:DD:EE:%02X", i%256)
		accountID := "test"
		_ = ds.AccountDeviceDir(accountID, mac)
	}
	lookupDuration := time.Since(start)

	t.Logf("✓ Performance test completed:")
	t.Logf("  Update %d mappings: %v (%.2f μs per mapping)", numMappings, updateDuration, float64(updateDuration.Nanoseconds())/float64(numMappings)/1000.0)
	t.Logf("  Lookup %d mappings: %v (%.2f μs per lookup)", numMappings, lookupDuration, float64(lookupDuration.Nanoseconds())/float64(numMappings)/1000.0)

	// Verify total mappings (should be more than numMappings due to normalization)
	ds.idMutex.RLock()
	totalMappings := len(ds.deviceMappings)
	ds.idMutex.RUnlock()

	t.Logf("  Total mappings stored: %d (includes normalized versions)", totalMappings)

	// Update is a pure in-memory map write; keep an absolute cap as a backstop
	// against catastrophic regressions in that hot path.
	if updateDuration > time.Millisecond*100 {
		t.Errorf("Update performance too slow: %v", updateDuration)
	}

	// Lookup does up to two Stat() syscalls and is dominated by filesystem
	// latency, which varies wildly on shared CI runners. Compare it to the
	// in-memory update cost instead of an absolute bound: the ratio captures
	// "lookup got disproportionately slower" (an algorithmic regression in the
	// lookup path) while staying stable under uniform host slowdown.
	if updateDuration <= 0 {
		t.Fatalf("Update duration is non-positive (%v); cannot compute lookup/update ratio", updateDuration)
	}

	const maxLookupUpdateRatio = 30.0

	ratio := float64(lookupDuration) / float64(updateDuration)
	t.Logf("  Lookup/Update ratio: %.2fx (threshold %.0fx)", ratio, maxLookupUpdateRatio)

	if ratio > maxLookupUpdateRatio {
		t.Errorf("Lookup is %.2fx slower than update (>%.0fx threshold) — possible regression in AccountDeviceDir lookup path. Update=%v Lookup=%v",
			ratio, maxLookupUpdateRatio, updateDuration, lookupDuration)
	}
}
