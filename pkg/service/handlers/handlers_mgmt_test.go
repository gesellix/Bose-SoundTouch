package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/spotify"
	"github.com/go-chi/chi/v5"
)

func TestHandleMgmtSpotifyInit(t *testing.T) {
	s := &Server{}
	// No spotify service configured
	req := httptest.NewRequest("POST", "/mgmt/spotify/init", nil)
	w := httptest.NewRecorder()
	s.HandleMgmtSpotifyInit(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	// With spotify service
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	w = httptest.NewRecorder()
	s.HandleMgmtSpotifyInit(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(resp["redirectUrl"], "client_id=cid") {
		t.Errorf("expected redirectUrl to contain client_id=cid, got %s", resp["redirectUrl"])
	}
}

func TestHandleMgmtSpotifyAccounts(t *testing.T) {
	s := &Server{}
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	req := httptest.NewRequest("GET", "/mgmt/spotify/accounts", nil)
	w := httptest.NewRecorder()
	s.HandleMgmtSpotifyAccounts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string][]spotify.Account
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if len(resp["accounts"]) != 0 {
		t.Errorf("expected 0 accounts, got %d", len(resp["accounts"]))
	}
}

func TestHandleMgmtListSpeakers(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	_, s := setupRouter("http://localhost:8000", ds)

	req := httptest.NewRequest("GET", "/mgmt/accounts/default/speakers", nil)
	w := httptest.NewRecorder()
	s.HandleMgmtListSpeakers(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if _, ok := resp["speakers"]; !ok {
		t.Error("expected 'speakers' in response")
	}
}

func TestHandleMgmtSpotifyCallback(t *testing.T) {
	s := &Server{}
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	// Mock Spotify token and profile endpoints
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "at",
			"refresh_token": "rt",
			"expires_in":    3600,
		})
	}))
	defer tokenServer.Close()

	profileServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           "user123",
			"display_name": "Test User",
		})
	}))
	defer profileServer.Close()

	// Use internal members to override URLs (available because we are in the same package)
	// Actually we need to reach through s.spotifyService which is private.
	// But s.spotifyService is *spotify.Service, which we have a handle to (svc).
	// We can't access private fields of spotify.Service from handlers package.
	// Wait, I can't override tokenURL from here if it's unexported in spotify package.
	// Let's check service.go again. Yes, tokenURL and apiBase are unexported.

	// Since I can't easily mock the external Spotify API here without exported fields,
	// I will test the error paths.

	t.Run("Missing code", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mgmt/spotify/callback", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyCallback(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "Missing authorization code") {
			t.Errorf("expected missing code error message, got %s", w.Body.String())
		}
	})

	t.Run("Spotify error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/mgmt/spotify/callback?error=access_denied", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyCallback(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "access_denied") {
			t.Errorf("expected access_denied error message, got %s", w.Body.String())
		}
	})
}

func TestHandleMgmtSpotifyConfirm(t *testing.T) {
	s := &Server{}
	svc := spotify.NewSpotifyService("cid", "secret", "http://localhost/cb", t.TempDir())
	s.SetSpotifyService(svc)

	t.Run("Missing code", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/mgmt/spotify/confirm", nil)
		w := httptest.NewRecorder()
		s.HandleMgmtSpotifyConfirm(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", w.Code)
		}
	})
}

func TestHandleMgmtDeviceEvents(t *testing.T) {
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	_, s := setupRouter("http://localhost:8000", ds)

	r := chi.NewRouter()
	r.Get("/mgmt/devices/{deviceId}/events", s.HandleMgmtDeviceEvents)

	req := httptest.NewRequest("GET", "/mgmt/devices/device123/events", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if _, ok := resp["events"]; !ok {
		t.Error("expected 'events' in response")
	}
}

func TestBasicAuthMgmt(t *testing.T) {
	s := &Server{}
	s.SetMgmtConfig("admin", "secret123")

	handler := s.BasicAuthMgmt()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	t.Run("Valid credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("admin", "secret123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
		}
		if rr.Body.String() != "OK" {
			t.Errorf("expected body 'OK', got %q", rr.Body.String())
		}
	})

	t.Run("Wrong username", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("wrong", "secret123")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
		if rr.Header().Get("WWW-Authenticate") == "" {
			t.Error("expected WWW-Authenticate header to be set")
		}
	})

	t.Run("Wrong password", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("admin", "wrongpass")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("Missing auth header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})

	t.Run("Empty credentials", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/mgmt/test", nil)
		req.SetBasicAuth("", "")
		rr := httptest.NewRecorder()

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("expected status %d, got %d", http.StatusUnauthorized, rr.Code)
		}
	})
}
