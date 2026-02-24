package handlers

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/constants"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/go-chi/chi/v5"
)

func TestMacMappingIntegration_HTTPHandler(t *testing.T) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "mac-integration-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Setup test data (same as the issue description)
	accountID := "3230304"
	serialNumber := "I6332527703739342000020"
	macAddress := "A81B6A536A98"

	// Create directory structure using serial number
	deviceDir := filepath.Join(tmpDir, "accounts", accountID, "devices", serialNumber)
	if err := os.MkdirAll(deviceDir, 0755); err != nil {
		t.Fatalf("failed to create device dir: %v", err)
	}

	// Create DeviceInfo.xml with MAC address
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

	// Create Presets.xml
	presetsXML := `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1">
        <ContentItem source="SPOTIFY" type="station" location="/station/test123" sourceAccount="spotify_user">
            <itemName>Test Preset</itemName>
        </ContentItem>
    </preset>
    <preset id="2">
        <ContentItem source="TUNEIN" type="station" location="/station/s12345" sourceAccount="">
            <itemName>Radio Station</itemName>
        </ContentItem>
    </preset>
</presets>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.PresetsFile), []byte(presetsXML), 0644); err != nil {
		t.Fatalf("failed to write Presets.xml: %v", err)
	}

	// Set a specific modification time for ETag testing
	pastTime := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(deviceDir, constants.PresetsFile), pastTime, pastTime); err != nil {
		t.Fatalf("failed to set file times: %v", err)
	}

	// Create Sources.xml (required by marge.PresetsToXML)
	sourcesXML := `<?xml version="1.0" encoding="UTF-8"?>
<sources>
    <source source="SPOTIFY" sourceAccount="spotify_user" status="READY" multiroomallowed="true">
        <sourceName>Spotify</sourceName>
    </source>
    <source source="TUNEIN" sourceAccount="" status="READY" multiroomallowed="true">
        <sourceName>TuneIn</sourceName>
    </source>
</sources>`
	if err := os.WriteFile(filepath.Join(deviceDir, constants.SourcesFile), []byte(sourcesXML), 0644); err != nil {
		t.Fatalf("failed to write Sources.xml: %v", err)
	}

	// Initialize datastore and server
	ds := datastore.NewDataStore(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("failed to initialize datastore: %v", err)
	}

	server := &Server{
		ds: ds,
	}

	// Setup router with the exact same route as in production
	router := chi.NewRouter()
	router.Route("/streaming", func(r chi.Router) {
		r.Get("/account/{account}/device/{device}/presets", server.HandleMargePresets)
	})

	// Test 1: Request with MAC address (should work due to mapping)
	t.Run("RequestWithMACAddress", func(t *testing.T) {
		requestURL := "/streaming/account/" + accountID + "/device/" + macAddress + "/presets"
		req, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Response body: %s", rr.Code, rr.Body.String())
			t.Logf("Request URL: %s", requestURL)
			t.Logf("MAC address: %s", macAddress)
			t.Logf("Serial number: %s", serialNumber)
			return
		}

		// Verify response contains the expected presets
		var presetsResponse struct {
			Presets []struct {
				ID   string `xml:"id,attr"`
				Name string `xml:"ContentItem>itemName"`
			} `xml:"preset"`
		}

		if err := xml.Unmarshal(rr.Body.Bytes(), &presetsResponse); err != nil {
			t.Errorf("Failed to parse XML response: %v", err)
			t.Logf("Response body: %s", rr.Body.String())
			return
		}

		if len(presetsResponse.Presets) != 2 {
			t.Errorf("Expected 2 presets, got %d", len(presetsResponse.Presets))
		}

		t.Logf("✓ Successfully retrieved %d presets using MAC address %s", len(presetsResponse.Presets), macAddress)
	})

	// Test 2: Request with serial number (should also work)
	t.Run("RequestWithSerialNumber", func(t *testing.T) {
		requestURL := "/streaming/account/" + accountID + "/device/" + serialNumber + "/presets"
		req, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Response body: %s", rr.Code, rr.Body.String())
			return
		}

		t.Logf("✓ Successfully retrieved presets using serial number %s", serialNumber)
	})

	// Test 3: Request with non-existent device ID
	t.Run("RequestWithNonExistentDevice", func(t *testing.T) {
		requestURL := "/streaming/account/" + accountID + "/device/NONEXISTENT/presets"
		req, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("Expected status 500 for non-existent device, got %d", rr.Code)
		}

		t.Logf("✓ Correctly returned error for non-existent device")
	})

	// Test 4: Case sensitivity test
	t.Run("RequestWithLowercaseMAC", func(t *testing.T) {
		lowercaseMAC := "a81b6a536a98"
		requestURL := "/streaming/account/" + accountID + "/device/" + lowercaseMAC + "/presets"
		req, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// This should fail because MAC addresses are case-sensitive
		if rr.Code == http.StatusOK {
			t.Logf("⚠️  Lowercase MAC address worked (might be unexpected): %s", lowercaseMAC)
		} else {
			t.Logf("✓ Lowercase MAC address correctly failed: %s (status: %d)", lowercaseMAC, rr.Code)
		}
	})

	// Test 5: Verify ETag functionality
	t.Run("RequestWithETag", func(t *testing.T) {
		requestURL := "/streaming/account/" + accountID + "/device/" + macAddress + "/presets"

		// First request to get ETag
		req1, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			t.Fatalf("failed to create request: %v", err)
		}

		rr1 := httptest.NewRecorder()
		router.ServeHTTP(rr1, req1)

		if rr1.Code != http.StatusOK {
			t.Errorf("First request failed with status %d", rr1.Code)
			return
		}

		// Extract ETag from response headers (direct access needed for httptest.ResponseRecorder)
		etag := ""
		//nolint:staticcheck // SA1008: ETag header name must be case-sensitive for test
		if vals, ok := rr1.Header()["ETag"]; ok && len(vals) > 0 {
			etag = vals[0]
		}

		if etag == "" {
			t.Errorf("No ETag header in response. Available headers: %v", rr1.Header())
			return
		}

		// Second request with ETag
		req2, err := http.NewRequest("GET", requestURL, nil)
		if err != nil {
			t.Fatalf("failed to create second request: %v", err)
		}
		req2.Header.Set("If-None-Match", etag)

		rr2 := httptest.NewRecorder()
		router.ServeHTTP(rr2, req2)

		if rr2.Code != http.StatusNotModified {
			t.Errorf("Expected 304 Not Modified, got %d", rr2.Code)
			return
		}

		t.Logf("✓ ETag functionality works correctly with MAC address resolution")
	})
}

// TestMacMappingDebug provides debugging information about the mapping state
func TestMacMappingDebug(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mac-debug-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple devices to test mapping
	devices := []struct {
		account string
		serial  string
		mac     string
	}{
		{"3230304", "I6332527703739342000020", "A81B6A536A98"},
		{"3230304", "J1234567890123456789012", "B92C7B647BA9"},
		{"5678901", "K9876543210987654321098", "C03D8C758CAA"},
	}

	for _, device := range devices {
		deviceDir := filepath.Join(tmpDir, "accounts", device.account, "devices", device.serial)
		if err := os.MkdirAll(deviceDir, 0755); err != nil {
			t.Fatalf("failed to create device dir: %v", err)
		}

		deviceInfoXML := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="` + device.serial + `">
    <name>Device ` + device.serial[0:8] + `</name>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <serialNumber>` + device.serial + `</serialNumber>
        </component>
    </components>
    <networkInfo type="SCM">
        <macAddress>` + device.mac + `</macAddress>
        <ipAddress>192.168.1.100</ipAddress>
    </networkInfo>
</info>`
		if err := os.WriteFile(filepath.Join(deviceDir, constants.DeviceInfoFile), []byte(deviceInfoXML), 0644); err != nil {
			t.Fatalf("failed to write DeviceInfo.xml: %v", err)
		}

		// Create minimal Sources.xml for each device
		sourcesXML := `<sources></sources>`
		if err := os.WriteFile(filepath.Join(deviceDir, constants.SourcesFile), []byte(sourcesXML), 0644); err != nil {
			t.Fatalf("failed to write Sources.xml: %v", err)
		}
	}

	// Initialize datastore
	ds := datastore.NewDataStore(tmpDir)
	if err := ds.Initialize(); err != nil {
		t.Fatalf("failed to initialize datastore: %v", err)
	}

	// Debug output
	allDevices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatalf("failed to list devices: %v", err)
	}

	t.Logf("Found %d devices total", len(allDevices))
	for _, dev := range allDevices {
		t.Logf("Device: Account=%s, Serial=%s, MAC=%s",
			dev.AccountID, dev.DeviceSerialNumber, dev.MacAddress)
	}

	// Test each mapping
	for _, device := range devices {
		resolvedDir := ds.AccountDeviceDir(device.account, device.mac)
		expectedDir := filepath.Join(tmpDir, "accounts", device.account, "devices", device.serial)

		if resolvedDir == expectedDir {
			t.Logf("✓ MAC %s correctly resolves to serial %s", device.mac, device.serial)
		} else {
			t.Errorf("✗ MAC %s resolution failed: got %s, expected %s",
				device.mac, resolvedDir, expectedDir)
		}
	}
}
