package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMirrorMiddleware_PreferredSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mirror-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	// 1. Setup local handler
	r := http.NewServeMux()
	r.HandleFunc("/test/local", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Source", "local")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("local response"))
	})

	// 2. Setup "upstream" mock server
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Source", "upstream")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("upstream response"))
	}))
	defer upstreamServer.Close()

	// 3. Setup our server with MirrorMiddleware
	server := NewServer(ds, nil, "http://localhost:8000", false, false, false, false, false)
	server.SetMirrorSettings(true, []string{"/test/local"}, "local")

	// We need to trick performMirror to use our mock upstream.
	// performMirror uses r.Host.
	upstreamURL := upstreamServer.URL
	upstreamHost := strings.TrimPrefix(upstreamURL, "http://")

	middleware := server.MirrorMiddleware(r)

	t.Run("PreferredLocal", func(t *testing.T) {
		server.SetMirrorSettings(true, []string{"/test/local"}, "local")

		req := httptest.NewRequest("GET", "/test/local", nil)
		req.Host = upstreamHost // So performMirror targets the mock upstream
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
		if w.Header().Get("X-Source") != "local" {
			t.Errorf("Expected X-Source: local, got %s", w.Header().Get("X-Source"))
		}
		if w.Body.String() != "local response" {
			t.Errorf("Expected 'local response', got '%s'", w.Body.String())
		}
	})

	t.Run("PreferredUpstream", func(t *testing.T) {
		server.SetMirrorSettings(true, []string{"/test/local"}, "upstream")

		req := httptest.NewRequest("GET", "/test/local", nil)
		req.Host = upstreamHost
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", w.Code)
		}
		if w.Header().Get("X-Source") != "upstream" {
			t.Errorf("Expected X-Source: upstream, got %s", w.Header().Get("X-Source"))
		}
		if w.Body.String() != "upstream response" {
			t.Errorf("Expected 'upstream response', got '%s'", w.Body.String())
		}
	})

	t.Run("FallbackToLocal", func(t *testing.T) {
		server.SetMirrorSettings(true, []string{"/test/local"}, "upstream")

		// Use a non-existent host for mirror to trigger failure
		req := httptest.NewRequest("GET", "/test/local", nil)
		req.Host = "nonexistent.invalid"
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		// Should fallback to local
		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200 (fallback), got %d", w.Code)
		}
		if w.Header().Get("X-Source") != "local" {
			t.Errorf("Expected X-Source: local (fallback), got %s", w.Header().Get("X-Source"))
		}
	})
}

func TestSettingsAPI_PreferredSource(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "settings-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://localhost:8000", false, false, false, false, false)

	// Test GET initial
	req := httptest.NewRequest("GET", "/setup/settings", nil)
	w := httptest.NewRecorder()
	server.HandleGetSettings(w, req)

	var settings map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &settings)
	if settings["preferred_source"] != "" && settings["preferred_source"] != "local" {
		t.Errorf("Initial preferred_source unexpected: %v", settings["preferred_source"])
	}

	// Test UPDATE
	update := map[string]interface{}{
		"preferred_source": "upstream",
	}
	body, err := json.Marshal(update)
	if err != nil {
		t.Fatalf("Failed to marshal update: %v", err)
	}
	req = httptest.NewRequest("POST", "/setup/settings", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	server.HandleUpdateSettings(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("POST /setup/settings failed: %d", w.Code)
	}

	if server.preferredSource != "upstream" {
		t.Errorf("Server preferredSource did not update: %s", server.preferredSource)
	}

	// Verify persistence
	persisted, _ := ds.GetSettings()
	if persisted.PreferredSource != "upstream" {
		t.Errorf("Datastore did not persist PreferredSource: %s", persisted.PreferredSource)
	}
}
