package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
)

func TestHandleBoseSpotifyToken_LocalResponse(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	// Wait, I can't easily inject an account into spotify.Service from here because fields are private.
	// Let's check if there's any other way.
	// I could mock the spotify.Service if it was an interface, but it's a struct.

	// ss.load() expects accounts in tmpDir/spotify/accounts.json
	spotifyDir := filepath.Join(tmpDir, "spotify")
	_ = os.MkdirAll(spotifyDir, 0755)

	account := map[string]interface{}{
		"user1": map[string]interface{}{
			"user_id":       "user1",
			"display_name":  "Test User",
			"access_token":  "valid-token",
			"refresh_token": "refresh-token",
			"expires_at":    time.Now().Add(1 * time.Hour).Unix(),
		},
	}
	data, err := json.Marshal(account)
	if err != nil {
		t.Fatalf("Failed to marshal account: %v", err)
	}
	_ = os.WriteFile(filepath.Join(spotifyDir, "accounts.json"), data, 0644)

	// Initialize ss so it loads the data
	ss := spotify.NewSpotifyService("client-id", "client-secret", "http://localhost/callback", tmpDir)

	server.SetSpotifyService(ss)

	// chi.URLParam works when using chi router
	r := chi.NewRouter()
	r.Post("/oauth/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3", server.HandleBoseToken)

	req := httptest.NewRequest("POST", "/oauth/device/DEVICE123/music/musicprovider/15/token/cs3", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if w.Header().Get("X-Proxy-Origin") != "self" {
		t.Errorf("Expected X-Proxy-Origin: self, got %s", w.Header().Get("X-Proxy-Origin"))
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if resp["access_token"] != "valid-token" {
		t.Errorf("Expected access_token 'valid-token', got %v", resp["access_token"])
	}
}

func TestHandleBoseSpotifyToken_FallbackToProxy(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	// Mirroring must be enabled for HandleBoseProxy to work (based on previous changes)
	// Actually I reverted that, so it should work regardless of MirrorEnabled now.
	server.SetMirrorSettings(true, nil, nil, "")

	// chi.URLParam works when using chi router
	r := chi.NewRouter()
	r.Post("/oauth/device/{deviceID}/music/musicprovider/{sourceID}/token/cs3", server.HandleBoseToken)

	// Since there's no Spotify service, it should fall back to HandleBoseProxy.
	// HandleBoseProxy will try to contact streaming.bose.com.
	// We can check if it returns a 502 or 404 (since we are not actually proxying to real Bose).

	req := httptest.NewRequest("POST", "/oauth/device/DEVICE123/music/musicprovider/15/token/cs3", nil)
	req.Host = "localhost" // use localhost to avoid real network call
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	// If it fell back to proxy, it should NOT have X-Proxy-Origin: self
	if w.Header().Get("X-Proxy-Origin") == "self" {
		t.Errorf("Expected fallback to proxy, but got X-Proxy-Origin: self")
	}

	// HandleBoseProxy sets X-Proxy-Origin: upstream
	if w.Header().Get("X-Proxy-Origin") != "upstream" {
		// It might fail before setting the header if the target host is invalid,
		// but our HandleBoseProxy sets it in ModifyResponse.
		// If it fails to connect, it might return 502 without the header.
		if w.Code != http.StatusBadGateway && w.Code != http.StatusNotFound {
			t.Errorf("Expected fallback to proxy (upstream), got status %d and origin %s", w.Code, w.Header().Get("X-Proxy-Origin"))
		}
	}
}

func TestHandleBoseLegacyToken(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	r := chi.NewRouter()
	r.Post("/oauth/device/{deviceID}/music/musicprovider/{sourceID}/token", server.HandleBoseLegacyToken)

	// Since we are not configuring Spotify, it should fall back to proxy
	req := httptest.NewRequest("POST", "/oauth/device/DEVICE123/music/musicprovider/15/token", nil)
	req.Host = "localhost"
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Header().Get("X-Proxy-Origin") == "self" {
		t.Errorf("Expected fallback to proxy")
	}
}
