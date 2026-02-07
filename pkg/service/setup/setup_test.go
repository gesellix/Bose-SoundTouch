package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetLiveDeviceInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			t.Errorf("Expected to request /info, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="08DF1F0BA325">
    <name>Test Speaker</name>
    <type>SoundTouch 20</type>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>19.0.5</softwareVersion>
            <serialNumber>08DF1F0BA325</serialNumber>
        </component>
    </components>
</info>`)
	}))
	defer server.Close()

	// Extract IP and port from the test server URL
	// The test server URL is like http://127.0.0.1:54321
	host := server.Listener.Addr().String()

	manager := NewManager("http://localhost:8000", nil)
	info, err := manager.GetLiveDeviceInfo(host)

	if err != nil {
		t.Fatalf("Failed to get live device info: %v", err)
	}

	if info.Name != "Test Speaker" {
		t.Errorf("Expected Name 'Test Speaker', got '%s'", info.Name)
	}
	if info.SoftwareVer != "19.0.5" {
		t.Errorf("Expected SoftwareVer '19.0.5', got '%s'", info.SoftwareVer)
	}
	if info.SerialNumber != "08DF1F0BA325" {
		t.Errorf("Expected SerialNumber '08DF1F0BA325', got '%s'", info.SerialNumber)
	}
}

func TestGetMigrationSummary_SSHFailure(t *testing.T) {
	// Use an IP that is unlikely to have an SSH server running or reachable
	// or use a local port that is closed.
	// We'll use a local port that we know is closed.
	manager := NewManager("http://localhost:8000", nil)
	summary, err := manager.GetMigrationSummary("127.0.0.1", "", "", nil)

	// Currently it might return an error OR it might return a summary with SSHSuccess: false
	// but the issue description says the user is told connection SUCCEEDED.

	if err == nil {
		if summary.SSHSuccess {
			t.Errorf("Expected SSHSuccess to be false for closed port, got true")
		}
		if summary.CurrentConfig == "" {
			t.Errorf("Expected CurrentConfig to contain error message, got empty string")
		}
	} else {
		t.Errorf("Expected no error from GetMigrationSummary, got %v", err)
	}
}

func TestGetMigrationSummary_WithProxyOptions(t *testing.T) {
	// Setup a mock server for live info
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, `<info deviceID="123"><name>Test</name></info>`)
	}))
	defer server.Close()

	host := server.Listener.Addr().String()
	manager := NewManager("http://soundcork:8000", nil)

	// Since we can't easily mock SSH here without a full SSH server,
	// we are testing the logic that depends on ParsedCurrentConfig being nil or not.
	// However, GetMigrationSummary tries to connect via SSH.
	// If SSH fails, ParsedCurrentConfig will be nil.

	options := map[string]string{
		"marge":     "original",
		"stats":     "soundcork",
		"sw_update": "original",
		"bmx":       "soundcork",
	}

	summary, err := manager.GetMigrationSummary(host, "http://target:8000", "http://proxy:8000", options)
	if err != nil {
		t.Fatalf("GetMigrationSummary failed: %v", err)
	}

	// When SSH fails (which it will here), PlannedConfig should be the default one for target:8000
	if !contains(summary.PlannedConfig, "http://target:8000/marge") {
		t.Errorf("Expected default marge URL when SSH fails, got: %s", summary.PlannedConfig)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
