package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
)

func TestSpotifyBridge(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	// Mock Speaker (LISA API)
	speakerReceived := false
	speakerTS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/setMusicServiceOAuthAccount" {
			speakerReceived = true
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?><status>/setMusicServiceOAuthAccount</status>`))
		}
	}))
	defer speakerTS.Close()

	// Register the speaker in the datastore so the bridge finds it
	devInfo := &models.ServiceDeviceInfo{
		DeviceID:  "DEV123",
		AccountID: "acc123",
		Name:      "Test Speaker",
		IPAddress: strings.TrimPrefix(speakerTS.URL, "http://"),
	}
	_ = ds.SaveDeviceInfo("acc123", "DEV123", devInfo)

	// Ensure the directory structure exists for marge.AddSource
	_ = os.MkdirAll(ds.AccountDevicesDir("acc123"), 0755)
	_ = os.MkdirAll(filepath.Join(ds.AccountDevicesDir("acc123"), "DEV123"), 0755)

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
				"id":           "spotify-user",
				"display_name": "Spotify User",
				"email":        "user@example.com",
			})
		}
	}))
	defer ts.Close()

	// Initialize Spotify service
	ss := spotify.NewSpotifyService("client-id", "client-secret", "http://localhost/callback", tmpDir)
	ss.SetEndpoints(ts.URL+"/token", ts.URL)
	server.SetSpotifyService(ss)

	r := chi.NewRouter()
	r.Get("/mgmt/spotify/callback", server.HandleMgmtSpotifyCallback)

	// Trigger the callback
	req := httptest.NewRequest("GET", "/mgmt/spotify/callback?code=fake-code&account=acc123", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	// 1. Verify Marge registration
	// We need to check if the source was added to the datastore
	foundInMarge := false
	sources, err := ds.GetConfiguredSources("acc123", "DEV123")
	if err == nil {
		for _, src := range sources {
			t.Logf("  Found source: %s (User: %s)", src.SourceKey.Type, src.Username)
			if (src.Username == "spotify-user" || src.SourceKey.Account == "spotify-user") &&
				(src.SourceProviderID == "15" || src.SourceKey.Type == "SPOTIFY") {
				foundInMarge = true
				break
			}
		}
	}

	if !foundInMarge {
		// Log what we found to debug
		allDevices, _ := ds.ListAllDevices()
		t.Logf("Total devices in datastore: %d", len(allDevices))
		for _, d := range allDevices {
			t.Logf("Device: %s (Account: %s)", d.DeviceID, d.AccountID)
			s, _ := ds.GetConfiguredSources(d.AccountID, d.DeviceID)
			t.Logf("  Sources: %d", len(s))
		}
		t.Errorf("Spotify user not found in Marge configured sources")
	}

	// 2. Verify Speaker notification (LISA API)
	// Using time.Sleep for simplicity in this test
	// Wait up to 1 second
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && !speakerReceived {
		time.Sleep(50 * time.Millisecond)
	}

	if !speakerReceived {
		t.Errorf("Speaker did not receive /setMusicServiceOAuthAccount notification")
	}

	// 3. Verify Token Refresh via Surrogate
	// Now simulate the speaker asking for a fresh token using the surrogate secret it received.
	// We need to find the surrogate first.
	sources, _ = ds.GetConfiguredSources("acc123", "DEV123")
	var surrogate string
	for _, src := range sources {
		if src.SourceKey.Type == "SPOTIFY" {
			surrogate = src.Secret
			break
		}
	}

	if surrogate == "" {
		t.Fatal("Could not find surrogate token in Marge sources")
	}

	if len(surrogate) != 32 {
		t.Errorf("Expected surrogate to be 32 hex chars, got %s", surrogate)
	}

	// Request refresh
	refreshReqBody := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": surrogate,
	}
	body, err := json.Marshal(refreshReqBody)
	if err != nil {
		t.Fatalf("Failed to marshal refresh request: %v", err)
	}

	refreshReq := httptest.NewRequest("POST", "/oauth/device/DEV123/music/musicprovider/15/token/cs3", strings.NewReader(string(body)))
	refreshW := httptest.NewRecorder()

	// Need to register the route for testing
	r.Post("/oauth/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3", server.HandleBoseToken)
	r.ServeHTTP(refreshW, refreshReq)

	if refreshW.Code != http.StatusOK {
		t.Fatalf("Token refresh failed: %d: %s", refreshW.Code, refreshW.Body.String())
	}

	var refreshResp map[string]interface{}
	if err := json.Unmarshal(refreshW.Body.Bytes(), &refreshResp); err != nil {
		t.Fatalf("Failed to parse refresh response: %v", err)
	}

	if refreshResp["access_token"] != "access-123" {
		t.Errorf("Expected access_token 'access-123', got '%v'", refreshResp["access_token"])
	}
}
