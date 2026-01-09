package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.DiscoveryTimeout != 5*time.Second {
		t.Errorf("Expected discovery timeout 5s, got %v", config.DiscoveryTimeout)
	}

	if !config.UPnPEnabled {
		t.Error("Expected UPnP to be enabled by default")
	}

	if !config.MDNSEnabled {
		t.Error("Expected mDNS to be enabled by default")
	}

	if config.HTTPTimeout != 10*time.Second {
		t.Errorf("Expected HTTP timeout 10s, got %v", config.HTTPTimeout)
	}

	if config.UserAgent != "Bose-SoundTouch-Go-Client/1.0" {
		t.Errorf("Expected default user agent, got %s", config.UserAgent)
	}

	if !config.CacheEnabled {
		t.Error("Expected cache to be enabled by default")
	}

	if config.CacheTTL != 30*time.Second {
		t.Errorf("Expected cache TTL 30s, got %v", config.CacheTTL)
	}

	if len(config.PreferredDevices) != 0 {
		t.Errorf("Expected no preferred devices by default, got %d", len(config.PreferredDevices))
	}
}

func TestLoadFromEnv_NoEnvVars(t *testing.T) {
	// Clear relevant environment variables
	clearTestEnvVars()

	config, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Should have default values
	if config.DiscoveryTimeout != 5*time.Second {
		t.Errorf("Expected default discovery timeout, got %v", config.DiscoveryTimeout)
	}

	if !config.UPnPEnabled {
		t.Error("Expected UPnP enabled by default")
	}
}

func TestLoadFromEnv_WithEnvVars(t *testing.T) {
	clearTestEnvVars()

	// Set test environment variables
	os.Setenv("DISCOVERY_TIMEOUT", "15s")
	os.Setenv("UPNP_ENABLED", "false")
	os.Setenv("MDNS_ENABLED", "false")
	os.Setenv("HTTP_TIMEOUT", "20s")
	os.Setenv("USER_AGENT", "Test-Client/1.0")
	os.Setenv("CACHE_ENABLED", "false")
	os.Setenv("CACHE_TTL", "60s")

	defer clearTestEnvVars()

	config, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if config.DiscoveryTimeout != 15*time.Second {
		t.Errorf("Expected discovery timeout 15s, got %v", config.DiscoveryTimeout)
	}

	if config.UPnPEnabled {
		t.Error("Expected UPnP to be disabled")
	}

	if config.MDNSEnabled {
		t.Error("Expected mDNS to be disabled")
	}

	if config.HTTPTimeout != 20*time.Second {
		t.Errorf("Expected HTTP timeout 20s, got %v", config.HTTPTimeout)
	}

	if config.UserAgent != "Test-Client/1.0" {
		t.Errorf("Expected custom user agent, got %s", config.UserAgent)
	}

	if config.CacheEnabled {
		t.Error("Expected cache to be disabled")
	}

	if config.CacheTTL != 60*time.Second {
		t.Errorf("Expected cache TTL 60s, got %v", config.CacheTTL)
	}
}

func TestParseDeviceString_HostOnly(t *testing.T) {
	device, err := parseDeviceString("192.168.1.100")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if device.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", device.Host)
	}

	if device.Port != 8090 {
		t.Errorf("Expected default port 8090, got %d", device.Port)
	}

	if device.Name != "SoundTouch-192.168.1.100" {
		t.Errorf("Expected default name 'SoundTouch-192.168.1.100', got '%s'", device.Name)
	}
}

func TestParseDeviceString_HostPort(t *testing.T) {
	device, err := parseDeviceString("192.168.1.100:8091")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if device.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", device.Host)
	}

	if device.Port != 8091 {
		t.Errorf("Expected port 8091, got %d", device.Port)
	}

	if device.Name != "SoundTouch-192.168.1.100" {
		t.Errorf("Expected default name 'SoundTouch-192.168.1.100', got '%s'", device.Name)
	}
}

func TestParseDeviceString_NameHostPort(t *testing.T) {
	device, err := parseDeviceString("Living Room@192.168.1.100:8090")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if device.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", device.Host)
	}

	if device.Port != 8090 {
		t.Errorf("Expected port 8090, got %d", device.Port)
	}

	if device.Name != "Living Room" {
		t.Errorf("Expected name 'Living Room', got '%s'", device.Name)
	}
}

func TestParseDeviceString_NameHost(t *testing.T) {
	device, err := parseDeviceString("Kitchen Speaker@192.168.1.101")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if device.Host != "192.168.1.101" {
		t.Errorf("Expected host '192.168.1.101', got '%s'", device.Host)
	}

	if device.Port != 8090 {
		t.Errorf("Expected default port 8090, got %d", device.Port)
	}

	if device.Name != "Kitchen Speaker" {
		t.Errorf("Expected name 'Kitchen Speaker', got '%s'", device.Name)
	}
}

func TestParseDeviceString_InvalidPort(t *testing.T) {
	_, err := parseDeviceString("192.168.1.100:invalid")
	if err == nil {
		t.Error("Expected error for invalid port, got nil")
	}

	_, err = parseDeviceString("192.168.1.100:0")
	if err == nil {
		t.Error("Expected error for port 0, got nil")
	}

	_, err = parseDeviceString("192.168.1.100:70000")
	if err == nil {
		t.Error("Expected error for port > 65535, got nil")
	}
}

func TestParseDeviceString_EmptyHost(t *testing.T) {
	_, err := parseDeviceString("")
	if err == nil {
		t.Error("Expected error for empty string, got nil")
	}

	_, err = parseDeviceString("@")
	if err == nil {
		t.Error("Expected error for empty host with @, got nil")
	}
}

func TestParsePreferredDevices_SingleDevice(t *testing.T) {
	clearTestEnvVars()
	os.Setenv("PREFERRED_DEVICES", "192.168.1.100")
	defer clearTestEnvVars()

	devices, err := parsePreferredDevices()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(devices) != 1 {
		t.Errorf("Expected 1 device, got %d", len(devices))
	}

	if devices[0].Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", devices[0].Host)
	}
}

func TestParsePreferredDevices_MultipleDevices(t *testing.T) {
	clearTestEnvVars()
	os.Setenv("PREFERRED_DEVICES", "Living Room@192.168.1.100:8090;Kitchen@192.168.1.101;192.168.1.102:8091")
	defer clearTestEnvVars()

	devices, err := parsePreferredDevices()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(devices) != 3 {
		t.Errorf("Expected 3 devices, got %d", len(devices))
	}

	// Check first device
	if devices[0].Name != "Living Room" {
		t.Errorf("Expected first device name 'Living Room', got '%s'", devices[0].Name)
	}
	if devices[0].Host != "192.168.1.100" {
		t.Errorf("Expected first device host '192.168.1.100', got '%s'", devices[0].Host)
	}
	if devices[0].Port != 8090 {
		t.Errorf("Expected first device port 8090, got %d", devices[0].Port)
	}

	// Check second device
	if devices[1].Name != "Kitchen" {
		t.Errorf("Expected second device name 'Kitchen', got '%s'", devices[1].Name)
	}
	if devices[1].Host != "192.168.1.101" {
		t.Errorf("Expected second device host '192.168.1.101', got '%s'", devices[1].Host)
	}
	if devices[1].Port != 8090 {
		t.Errorf("Expected second device port 8090, got %d", devices[1].Port)
	}

	// Check third device
	if devices[2].Name != "SoundTouch-192.168.1.102" {
		t.Errorf("Expected third device default name, got '%s'", devices[2].Name)
	}
	if devices[2].Host != "192.168.1.102" {
		t.Errorf("Expected third device host '192.168.1.102', got '%s'", devices[2].Host)
	}
	if devices[2].Port != 8091 {
		t.Errorf("Expected third device port 8091, got %d", devices[2].Port)
	}
}

func TestParsePreferredDevices_EmptyString(t *testing.T) {
	clearTestEnvVars()
	os.Setenv("PREFERRED_DEVICES", "")
	defer clearTestEnvVars()

	devices, err := parsePreferredDevices()
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if len(devices) != 0 {
		t.Errorf("Expected 0 devices for empty string, got %d", len(devices))
	}
}

func TestParsePreferredDevices_InvalidDevice(t *testing.T) {
	clearTestEnvVars()
	os.Setenv("PREFERRED_DEVICES", "192.168.1.100:invalid")
	defer clearTestEnvVars()

	_, err := parsePreferredDevices()
	if err == nil {
		t.Error("Expected error for invalid device configuration, got nil")
	}
}

func TestGetPreferredDevicesAsDiscovered(t *testing.T) {
	config := &Config{
		PreferredDevices: []DeviceConfig{
			{Name: "Living Room", Host: "192.168.1.100", Port: 8090},
			{Name: "Kitchen", Host: "192.168.1.101", Port: 8091},
		},
	}

	devices := config.GetPreferredDevicesAsDiscovered()

	if len(devices) != 2 {
		t.Errorf("Expected 2 discovered devices, got %d", len(devices))
	}

	// Check first device
	if devices[0].Name != "Living Room" {
		t.Errorf("Expected name 'Living Room', got '%s'", devices[0].Name)
	}
	if devices[0].Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", devices[0].Host)
	}
	if devices[0].Port != 8090 {
		t.Errorf("Expected port 8090, got %d", devices[0].Port)
	}
	expectedLocation := "http://192.168.1.100:8090/info"
	if devices[0].Location != expectedLocation {
		t.Errorf("Expected location '%s', got '%s'", expectedLocation, devices[0].Location)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	config := &Config{
		DiscoveryTimeout: 5 * time.Second,
		HTTPTimeout:      10 * time.Second,
		CacheTTL:         30 * time.Second,
		PreferredDevices: []DeviceConfig{
			{Name: "Test", Host: "192.168.1.100", Port: 8090},
		},
	}

	err := config.Validate()
	if err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

func TestValidate_InvalidTimeouts(t *testing.T) {
	config := &Config{
		DiscoveryTimeout: 0,
		HTTPTimeout:      10 * time.Second,
		CacheTTL:         30 * time.Second,
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected error for zero discovery timeout, got nil")
	}

	config.DiscoveryTimeout = 5 * time.Second
	config.HTTPTimeout = 0

	err = config.Validate()
	if err == nil {
		t.Error("Expected error for zero HTTP timeout, got nil")
	}

	config.HTTPTimeout = 10 * time.Second
	config.CacheTTL = 0

	err = config.Validate()
	if err == nil {
		t.Error("Expected error for zero cache TTL, got nil")
	}
}

func TestValidate_InvalidDevices(t *testing.T) {
	config := &Config{
		DiscoveryTimeout: 5 * time.Second,
		HTTPTimeout:      10 * time.Second,
		CacheTTL:         30 * time.Second,
		PreferredDevices: []DeviceConfig{
			{Name: "Test", Host: "", Port: 8090}, // Empty host
		},
	}

	err := config.Validate()
	if err == nil {
		t.Error("Expected error for empty host, got nil")
	}

	config.PreferredDevices[0].Host = "192.168.1.100"
	config.PreferredDevices[0].Port = 0 // Invalid port

	err = config.Validate()
	if err == nil {
		t.Error("Expected error for invalid port, got nil")
	}

	config.PreferredDevices[0].Port = 70000 // Port too high

	err = config.Validate()
	if err == nil {
		t.Error("Expected error for port > 65535, got nil")
	}
}

// Helper function to clear test environment variables
func clearTestEnvVars() {
	envVars := []string{
		"DISCOVERY_TIMEOUT",
		"UPNP_ENABLED",
		"MDNS_ENABLED",
		"HTTP_TIMEOUT",
		"USER_AGENT",
		"CACHE_ENABLED",
		"CACHE_TTL",
		"PREFERRED_DEVICES",
	}

	for _, env := range envVars {
		os.Unsetenv(env)
	}
}
