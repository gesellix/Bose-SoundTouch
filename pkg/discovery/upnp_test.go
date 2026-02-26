package discovery

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestNewDiscoveryService(t *testing.T) {
	timeout := 5 * time.Second
	service := NewService(timeout)

	if service.timeout != timeout {
		t.Errorf("Expected timeout %v, got %v", timeout, service.timeout)
	}

	if service.cacheTTL != defaultCacheTTL {
		t.Errorf("Expected cacheTTL %v, got %v", defaultCacheTTL, service.cacheTTL)
	}

	if service.cache == nil {
		t.Error("Expected cache to be initialized")
	}
}

func TestNewDiscoveryServiceWithDefaultTimeout(t *testing.T) {
	service := NewService(0)

	if service.timeout != defaultTimeout {
		t.Errorf("Expected default timeout %v, got %v", defaultTimeout, service.timeout)
	}
}

func TestBuildMSearchRequest(t *testing.T) {
	t.Run("DefaultTimeout", func(t *testing.T) {
		service := NewService(5 * time.Second)
		request := service.buildMSearchRequest()

		expectedLines := []string{
			"M-SEARCH * HTTP/1.1",
			"HOST: 239.255.255.250:1900",
			"MAN: \"ssdp:discover\"",
			"ST: urn:schemas-upnp-org:device:MediaRenderer:1",
			"MX: 5",
		}

		for _, expectedLine := range expectedLines {
			if !contains(request, expectedLine) {
				t.Errorf("Expected M-SEARCH request to contain '%s'", expectedLine)
			}
		}

		if !contains(request, "\r\n\r\n") {
			t.Error("Expected M-SEARCH request to end with double CRLF")
		}
	})

	t.Run("LowTimeout", func(t *testing.T) {
		// If timeout is less than 1 second, it should still use MX: 1
		service := NewService(500 * time.Millisecond)
		request := service.buildMSearchRequest()

		if !contains(request, "MX: 1") {
			t.Errorf("Expected low timeout (500ms) to result in MX: 1, but got something else. Request:\n%s", request)
		}
	})

	t.Run("VeryHighTimeout", func(t *testing.T) {
		// MX should probably be capped at 5 for UPnP compatibility
		service := NewService(10 * time.Second)
		request := service.buildMSearchRequest()

		if !contains(request, "MX: 5") {
			t.Errorf("Expected high timeout (10s) to result in MX: 5, but got something else. Request:\n%s", request)
		}
	})
}

func TestParseLocationURL_Valid(t *testing.T) {
	service := NewService(1 * time.Second)
	location := "http://192.168.1.100:8090/device.xml"

	device, err := service.parseLocationURL(location, "")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if device.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", device.Host)
	}

	if device.Port != 8090 {
		t.Errorf("Expected port 8090, got %d", device.Port)
	}

	if device.UPnPLocation != location {
		t.Errorf("Expected UPnP location '%s', got '%s'", location, device.UPnPLocation)
	}

	if device.Name == "" {
		t.Error("Expected device name to be set")
	}

	expectedName := "SoundTouch-192.168.1.100"
	if device.Name != expectedName {
		t.Errorf("Expected name '%s', got '%s'", expectedName, device.Name)
	}
}

func TestParseLocationURL_Invalid(t *testing.T) {
	service := NewService(1 * time.Second)

	invalidURLs := []string{
		"not-a-url",
		"ftp://192.168.1.100/device.xml",
		"http://invalid-format",
		"",
	}

	for _, url := range invalidURLs {
		_, err := service.parseLocationURL(url, "")
		if err == nil {
			t.Errorf("Expected error for invalid URL '%s', got nil", url)
		}
	}
}

func TestParseResponse_ValidMediaRenderer(t *testing.T) {
	xmlPayload := `<?xml version="1.0" encoding="utf-8"?>
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <friendlyName>Test Device</friendlyName>
        <modelName>SoundTouch 10</modelName>
        <serialNumber>AABBCCDDEEFF</serialNumber>
    </device>
</root>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		fmt.Fprint(w, xmlPayload)
	}))
	defer server.Close()

	service := NewService(1 * time.Second)
	service.httpClient = server.Client()

	validResponse := fmt.Sprintf(`HTTP/1.1 200 OK
Cache-Control: max-age=1800
Location: %s
ST: urn:schemas-upnp-org:device:MediaRenderer:1
USN: uuid:12345678-1234-5678-9012-123456789012::urn:schemas-upnp-org:device:MediaRenderer:1

`, server.URL)

	device, err := service.parseResponse(validResponse)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if device == nil {
		t.Fatal("Expected device, got nil")
	}

	if device.Name != "Test Device" {
		t.Errorf("Expected name 'Test Device', got '%s'", device.Name)
	}
}

func TestParseResponse_NotMediaRenderer(t *testing.T) {
	service := NewService(1 * time.Second)

	nonMediaRendererResponse := `HTTP/1.1 200 OK
Cache-Control: max-age=1800
Date: Mon, 22 Jun 1998 09:55:21 GMT
EXT:
Location: http://192.168.1.100:8090/device.xml
Server: Linux/3.14.0 UPnP/1.0 SomeDevice/1.0
ST: urn:schemas-upnp-org:device:SomeOtherDevice:1
USN: uuid:12345678-1234-5678-9012-123456789012::urn:schemas-upnp-org:device:SomeOtherDevice:1

`

	device, err := service.parseResponse(nonMediaRendererResponse)
	if err == nil {
		t.Error("Expected error for non-MediaRenderer device, got nil")
	}

	if device != nil {
		t.Error("Expected nil device for non-MediaRenderer, got device")
	}

	expectedError := "not a MediaRenderer device"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestParseResponse_InvalidHTTP(t *testing.T) {
	service := NewService(1 * time.Second)

	invalidResponses := []string{
		"not http response",
		"HTTP/1.1 404 Not Found\r\n\r\n",
		"",
		"HTTP/1.1 200 OK\r\nLocation: invalid-location\r\nST: urn:schemas-upnp-org:device:MediaRenderer:1\r\n\r\n",
	}

	for _, response := range invalidResponses {
		device, err := service.parseResponse(response)
		if err == nil && response != "" {
			t.Errorf("Expected error for invalid response, got nil for: %s", response)
		}

		if device != nil {
			t.Errorf("Expected nil device for invalid response, got device for: %s", response)
		}
	}
}

func TestParseResponse_NoLocation(t *testing.T) {
	service := NewService(1 * time.Second)

	responseWithoutLocation := `HTTP/1.1 200 OK
Cache-Control: max-age=1800
Date: Mon, 22 Jun 1998 09:55:21 GMT
ST: urn:schemas-upnp-org:device:MediaRenderer:1

`

	device, err := service.parseResponse(responseWithoutLocation)
	if err == nil {
		t.Error("Expected error for response without location, got nil")
	}

	if device != nil {
		t.Error("Expected nil device for response without location")
	}

	expectedError := "no location header found"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestCacheOperations(t *testing.T) {
	service := NewService(1 * time.Second)

	// Test empty cache
	devices := service.GetCachedDevices()
	if len(devices) != 0 {
		t.Errorf("Expected empty cache, got %d devices", len(devices))
	}

	// Add devices to cache
	testDevices := []*models.DiscoveredDevice{
		{
			Host:     "192.168.1.100",
			Port:     8090,
			Name:     "Device 1",
			LastSeen: time.Now(),
		},
		{
			Host:     "192.168.1.101",
			Port:     8090,
			Name:     "Device 2",
			LastSeen: time.Now(),
		},
	}

	service.updateCache(testDevices)

	// Test cached devices retrieval
	cachedDevices := service.GetCachedDevices()
	if len(cachedDevices) != 2 {
		t.Errorf("Expected 2 cached devices, got %d", len(cachedDevices))
	}

	// Test cache clear
	service.ClearCache()

	devices = service.GetCachedDevices()
	if len(devices) != 0 {
		t.Errorf("Expected empty cache after clear, got %d devices", len(devices))
	}
}

func TestCacheExpiration(t *testing.T) {
	service := NewService(5 * time.Second)
	service.cacheTTL = 100 * time.Millisecond // Short TTL for testing

	// Add device with old timestamp
	expiredDevice := &models.DiscoveredDevice{
		Host:     "192.168.1.100",
		Port:     8090,
		Name:     "Expired Device",
		LastSeen: time.Now().Add(-200 * time.Millisecond), // Expired
	}

	// Add device with recent timestamp
	freshDevice := &models.DiscoveredDevice{
		Host:     "192.168.1.101",
		Port:     8090,
		Name:     "Fresh Device",
		LastSeen: time.Now(), // Fresh
	}

	service.updateCache([]*models.DiscoveredDevice{expiredDevice, freshDevice})

	// Wait a bit to ensure expiration
	time.Sleep(50 * time.Millisecond)

	// Test that expired device is filtered out
	devices := service.GetCachedDevices()
	if len(devices) != 1 {
		t.Errorf("Expected 1 non-expired device, got %d", len(devices))
	}

	if len(devices) > 0 && devices[0].Host != "192.168.1.101" {
		t.Errorf("Expected fresh device, got %s", devices[0].Host)
	}
}

func TestDiscoverDevices_UseCache(t *testing.T) {
	service := NewService(5 * time.Second)

	// Add fresh device to cache
	freshDevice := &models.DiscoveredDevice{
		Host:     "192.168.1.100",
		Port:     8090,
		Name:     "Cached Device",
		LastSeen: time.Now(),
	}

	service.updateCache([]*models.DiscoveredDevice{freshDevice})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Should return cached device without network discovery
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 cached device, got %d", len(devices))
	}

	if len(devices) > 0 && devices[0].Host != "192.168.1.100" {
		t.Errorf("Expected cached device host, got %s", devices[0].Host)
	}
}

func TestDiscoverDevice_FromCache(t *testing.T) {
	service := NewService(5 * time.Second)

	// Add device to cache
	device := &models.DiscoveredDevice{
		Host:     "192.168.1.100",
		Port:     8090,
		Name:     "Test Device",
		LastSeen: time.Now(),
	}

	service.updateCache([]*models.DiscoveredDevice{device})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Discover specific device from cache
	foundDevice, err := service.DiscoverDevice(ctx, "192.168.1.100")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if foundDevice.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", foundDevice.Host)
	}

	if foundDevice.Name != "Test Device" {
		t.Errorf("Expected name 'Test Device', got '%s'", foundDevice.Name)
	}
}

func TestDiscoverDevice_NotFound(t *testing.T) {
	service := NewService(100 * time.Millisecond) // Short timeout

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Try to discover non-existent device
	_, err := service.DiscoverDevice(ctx, "192.168.1.999")
	if err == nil {
		t.Error("Expected error for non-existent device, got nil")
	}

	expectedError := "device with host 192.168.1.999 not found"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestNewDiscoveryServiceWithConfig(t *testing.T) {
	cfg := &config.Config{
		DiscoveryTimeout: 15 * time.Second,
		CacheTTL:         60 * time.Second,
		UPnPEnabled:      false,
		PreferredDevices: []config.DeviceConfig{
			{Name: "Test Device", Host: "192.168.1.100", Port: 8090},
		},
	}

	service := NewServiceWithConfig(cfg)

	if service.timeout != 15*time.Second {
		t.Errorf("Expected timeout 15s, got %v", service.timeout)
	}

	if service.cacheTTL != 60*time.Second {
		t.Errorf("Expected cache TTL 60s, got %v", service.cacheTTL)
	}

	if service.config.UPnPEnabled {
		t.Error("Expected UPnP to be disabled")
	}
}

func TestGetConfiguredDevices(t *testing.T) {
	cfg := &config.Config{
		PreferredDevices: []config.DeviceConfig{
			{Name: "Living Room", Host: "192.168.1.100", Port: 8090},
			{Name: "Kitchen", Host: "192.168.1.101", Port: 8091},
		},
	}

	service := NewServiceWithConfig(cfg)
	devices := service.getConfiguredDevices()

	if len(devices) != 2 {
		t.Errorf("Expected 2 configured devices, got %d", len(devices))
	}

	if devices[0].Name != "Living Room" {
		t.Errorf("Expected first device name 'Living Room', got '%s'", devices[0].Name)
	}

	if devices[0].Host != "192.168.1.100" {
		t.Errorf("Expected first device host '192.168.1.100', got '%s'", devices[0].Host)
	}

	if devices[1].Port != 8091 {
		t.Errorf("Expected second device port 8091, got %d", devices[1].Port)
	}
}

func TestMergeDevices(t *testing.T) {
	service := NewService(5 * time.Second)

	existing := []*models.DiscoveredDevice{
		{Host: "192.168.1.100", Name: "Device 1", Port: 8090},
		{Host: "192.168.1.101", Name: "Device 2", Port: 8090},
	}

	newDevices := []*models.DiscoveredDevice{
		{Host: "192.168.1.101", Name: "Duplicate", Port: 8090}, // Duplicate
		{Host: "192.168.1.102", Name: "Device 3", Port: 8090},  // New
	}

	merged := service.mergeDevices(existing, newDevices)

	if len(merged) != 3 {
		t.Errorf("Expected 3 merged devices, got %d", len(merged))
	}

	// Check that duplicates are avoided
	hosts := make(map[string]int)
	for _, device := range merged {
		hosts[device.Host]++
	}

	for host, count := range hosts {
		if count > 1 {
			t.Errorf("Host %s appears %d times (should be unique)", host, count)
		}
	}

	// Check that all unique hosts are present
	expectedHosts := []string{"192.168.1.100", "192.168.1.101", "192.168.1.102"}
	for _, expectedHost := range expectedHosts {
		if _, exists := hosts[expectedHost]; !exists {
			t.Errorf("Expected host %s not found in merged devices", expectedHost)
		}
	}
}

func TestDiscoverDevices_ConfiguredOnly(t *testing.T) {
	cfg := &config.Config{
		UPnPEnabled: false, // Disable UPnP
		PreferredDevices: []config.DeviceConfig{
			{Name: "Test Device", Host: "192.168.1.100", Port: 8090},
		},
	}

	service := NewServiceWithConfig(cfg)
	ctx := context.Background()

	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 configured device, got %d", len(devices))
	}

	if devices[0].Name != "Test Device" {
		t.Errorf("Expected device name 'Test Device', got '%s'", devices[0].Name)
	}

	if devices[0].Host != "192.168.1.100" {
		t.Errorf("Expected device host '192.168.1.100', got '%s'", devices[0].Host)
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
