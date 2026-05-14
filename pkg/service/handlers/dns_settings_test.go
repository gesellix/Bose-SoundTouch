package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestDNSSettingsValidation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dns-validation-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	r, server := setupRouter("http://localhost:8001", ds)

	// Test Case 1: Enable DNS with empty upstream (should fallback to system DNS)
	update := map[string]interface{}{
		"server_url":    "http://localhost:8001",
		"dns_enabled":   true,
		"dns_upstream":  "",
		"dns_bind_addr": ":5353",
	}

	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Failed to marshal update: %v", err)
	}
	req := httptest.NewRequest("POST", "/setup/settings", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 when enabling DNS without upstream (fallback to system), got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify DNS state in server
	if !server.dnsEnabled {
		t.Error("DNS should be enabled in server state")
	}

	// Verify it TRIED to start (either it is running, or it failed due to port conflict but state is enabled)
	if !server.dnsEnabled {
		t.Error("DNS state should be enabled")
	}

	// Test Case 2: Enable DNS with valid upstream
	// Using a random port to avoid conflicts and ensure it's fast
	updateValid := map[string]interface{}{
		"server_url":    "http://localhost:8001",
		"dns_enabled":   true,
		"dns_upstream":  "8.8.8.8",
		"dns_bind_addr": "127.0.0.1:0", // Random port
	}

	bodyValid, err := json.Marshal(updateValid)
	if err != nil {
		t.Fatalf("Failed to marshal updateValid: %v", err)
	}
	reqValid := httptest.NewRequest("POST", "/setup/settings", bytes.NewBuffer(bodyValid))
	wValid := httptest.NewRecorder()
	r.ServeHTTP(wValid, reqValid)

	if wValid.Code != http.StatusOK {
		t.Errorf("Expected status 200 when enabling DNS with valid upstream, got %d. Body: %s", wValid.Code, wValid.Body.String())
	}

	// Verify DNS state in server
	if !server.dnsEnabled {
		t.Error("DNS should be enabled in server state")
	}

	// Shutdown server to clean up
	if server.dnsDiscovery != nil {
		_ = server.dnsDiscovery.Shutdown()
	}
}
