package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestHandleBMXRegistry_DNSDependent(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bmx-registry-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	localURL := "https://127.0.0.1"
	server := NewServer(ds, nil, localURL, false, false, false)

	t.Run("DNSEnabled_UsesBoseURL", func(t *testing.T) {
		server.SetDNSSettings(true, "8.8.8.8", ":5353")

		req := httptest.NewRequest("GET", "/bmx/v1/services", nil)
		w := httptest.NewRecorder()

		server.HandleBMXRegistry(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		services := resp["bmx_services"].([]interface{})
		foundTuneIn := false
		for _, s := range services {
			service := s.(map[string]interface{})
			if service["id"].(map[string]interface{})["name"] == "TUNEIN" {
				foundTuneIn = true
				baseURL := service["baseUrl"].(string)
				if baseURL != "https://content.api.bose.io/bmx/tunein" {
					t.Errorf("Expected baseUrl https://content.api.bose.io/bmx/tunein, got %s", baseURL)
				}

				// Check assets (MEDIA_SERVER) - should still be local
				assets := service["assets"].(map[string]interface{})
				icons := assets["icons"].(map[string]interface{})
				for k, v := range icons {
					iconURL := v.(string)
					if strings.HasPrefix(iconURL, "{") {
						t.Errorf("Icon %s still has placeholder: %s", k, iconURL)
					}
					if !strings.HasPrefix(iconURL, localURL+"/media/bmx-icons/tunein") {
						t.Errorf("Icon %s should point to local media server bmx-icons subdirectory, got %s", k, iconURL)
					}
				}
			}
		}
		if !foundTuneIn {
			t.Error("TuneIn service not found in registry")
		}
	})

	t.Run("DNSDisabled_UsesLocalURL", func(t *testing.T) {
		server.SetDNSSettings(false, "", "")

		req := httptest.NewRequest("GET", "/bmx/v1/services", nil)
		w := httptest.NewRecorder()

		server.HandleBMXRegistry(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		services := resp["bmx_services"].([]interface{})
		foundTuneIn := false
		for _, s := range services {
			service := s.(map[string]interface{})
			if service["id"].(map[string]interface{})["name"] == "TUNEIN" {
				foundTuneIn = true
				baseURL := service["baseUrl"].(string)
				if baseURL != localURL+"/bmx/tunein" {
					t.Errorf("Expected baseUrl %s/bmx/tunein, got %s", localURL, baseURL)
				}
			}
		}
		if !foundTuneIn {
			t.Error("TuneIn service not found in registry")
		}
	})
}

func TestNormalizeServerURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"http://host:8000", "http://host:8000"},
		{"http://host:8000/", "http://host:8000"},
		{"http://host:8000///", "http://host:8000"},
		{"  http://host:8000/  ", "http://host:8000"},
		{"https://127.0.0.1", "https://127.0.0.1"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeServerURL(c.in); got != c.want {
			t.Errorf("NormalizeServerURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestHandleBMXRegistry_TrailingSlashServerURL is a regression test for the
// trailing-slash double-slash bug: a server_url configured with a trailing
// slash must not produce a "//bmx/..."
// base URL. The speaker concatenates "/v1/playback/station/{id}" onto the base,
// so a doubled slash yields a "//bmx/tunein/..." request the router 404s and
// TuneIn playback fails with INVALID_SOURCE.
func TestHandleBMXRegistry_TrailingSlashServerURL(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "bmx-registry-slash-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	// Operator typed a trailing slash (the reported trailing-slash case).
	server := NewServer(ds, nil, "https://127.0.0.1/", false, false, false)
	server.SetDNSSettings(false, "", "")

	req := httptest.NewRequest("GET", "/bmx/v1/services", nil)
	w := httptest.NewRecorder()
	server.HandleBMXRegistry(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if strings.Contains(body, "//bmx") || strings.Contains(body, "//media") {
		t.Errorf("registry response contains a doubled slash from the trailing-slash server_url:\n%s", body)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	for _, s := range resp["bmx_services"].([]interface{}) {
		service := s.(map[string]interface{})
		if service["id"].(map[string]interface{})["name"] == "TUNEIN" {
			if baseURL := service["baseUrl"].(string); baseURL != "https://127.0.0.1/bmx/tunein" {
				t.Errorf("Expected baseUrl https://127.0.0.1/bmx/tunein, got %s", baseURL)
			}
			return
		}
	}
	t.Error("TuneIn service not found in registry")
}
