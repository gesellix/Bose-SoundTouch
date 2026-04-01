package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
)

func TestSpotifyAdditionFlow(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	// Mock Spotify response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token":  "access-123",
				"refresh_token": "refresh-123",
				"expires_in":    3600,
			})
		case "/me":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id":           "user123",
				"display_name": "Test User",
				"email":        "user@example.com",
			})
		}
	}))
	defer ts.Close()

	// Initialize Spotify service with mock URLs
	ss := spotify.NewSpotifyService("client-id", "client-secret", "http://localhost/callback", tmpDir)
	ss.SetEndpoints(ts.URL+"/token", ts.URL)

	server.SetSpotifyService(ss)

	r := chi.NewRouter()
	r.Post("/oauth/account/{account}/music/musicprovider/{sourceID}/token/cs", server.HandleBoseAccountToken)
	r.Post("/streaming/account/{account}/source", server.HandleMargeAddSource)
	r.Get("/streaming/account/{account}/full", server.HandleMargeAccountFull)
	r.Post("/streaming/account/{account}/device/{device}", server.HandleMargeAddDevice)

	// Pre-step: Add a device to the account so sources can be linked to it
	t.Run("Add Device", func(t *testing.T) {
		deviceXML := `<device deviceid="DEV123"><name>Speaker</name><macaddress>00:11:22:33:44:55</macaddress></device>`
		req := httptest.NewRequest("POST", "/streaming/account/123/device/DEV123", strings.NewReader(deviceXML))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK && w.Code != http.StatusCreated {
			t.Fatalf("Expected 200/201, got %d: %s", w.Code, w.Body.String())
		}

		// Verify ListAllDevices sees it
		devs, err := ds.ListAllDevices()
		if err != nil {
			t.Fatalf("ListAllDevices failed: %v", err)
		}
		found := false
		for _, d := range devs {
			if d.DeviceID == "DEV123" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("ListAllDevices did not find DEV123. Found: %+v", devs)
		}
	})

	// 1. Step: OAuth Exchange
	t.Run("OAuth Exchange (Step 1)", func(t *testing.T) {
		// Since I can't easily point the service to the mock server without modifying service.go,
		// I will just test that the handler correctly parses the body and calls the service.
		// If I can't mock the service, I'll mock the service's behavior by pre-loading an account if needed,
		// or just check that the handler reaches the service call.

		// For this test, let's just assume the service call would fail but the handler logic is correct.
		// Or better, let's pre-populate the accounts.json so HandleBoseSpotifyToken can return something.

		spotifyDir := filepath.Join(tmpDir, "spotify")
		_ = os.MkdirAll(spotifyDir, 0755)
		_ = os.WriteFile(filepath.Join(spotifyDir, "accounts.json"), []byte("{}"), 0644)

		body := `{"grant_type": "authorization_code", "code": "fake-code", "redirect_uri": "http://localhost"}`
		req := httptest.NewRequest("POST", "/oauth/account/123/music/musicprovider/15/token/cs", strings.NewReader(body))
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d: %s", w.Code, w.Body.String())
		}
	})

	// 2. Step: Marge Add Source
	t.Run("Marge Add Source (Step 2)", func(t *testing.T) {
		sourceXML := `<?xml version="1.0" encoding="UTF-8"?>
<source>
    <username>user123</username>
    <sourceproviderid>15</sourceproviderid>
    <credential type="token_version_3">access-123</credential>
    <sourcename>My Spotify</sourcename>
</source>`
		req := httptest.NewRequest("POST", "/streaming/account/123/source", strings.NewReader(sourceXML))
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		if !strings.Contains(w.Body.String(), "<sourceID>SRC_") {
			t.Errorf("Response missing sourceID: %s", w.Body.String())
		}
	})

	// 3. Step: Verify in Account Full
	t.Run("Verify in Account Full (Step 3)", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/streaming/account/123/full", nil)
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected 200 OK, got %d", w.Code)
		}

		body := w.Body.String()
		// Debug: log the body to see what's in there
		// t.Logf("Full response body: %s", body)

		if !strings.Contains(body, "user123") {
			t.Errorf("Full response missing 'user123': %s", body)
		}
		if !strings.Contains(body, "access-123") {
			t.Errorf("Full response missing 'access-123': %s", body)
		}
	})
}
