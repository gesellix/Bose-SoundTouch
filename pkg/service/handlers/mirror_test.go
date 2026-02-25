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

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
)

func TestMirroring(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-mirror-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)

	// Create a mock Bose Upstream
	boseUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Only handle requests to the actual path
		if strings.HasSuffix(r.URL.Path, "/recent") {
			w.Header().Set("Content-Type", "application/vnd.bose.streaming-v1.2+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<bose-response/>"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer boseUpstream.Close()

	// Setup local server
	r, server := setupRouter("http://localhost:8001", ds)

	// Setup recorder
	recorder := proxy.NewRecorder(tempDir)
	server.SetRecorder(recorder)
	server.SetRecordEnabled(true)
	server.SetMirrorSettings(true, []string{"/streaming/account/*/device/*/recent"})

	ts := httptest.NewServer(r)
	defer ts.Close()

	account := "123"
	deviceID := "DEV1"

	// Ensure the datastore has the necessary directories for the local handler
	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	_ = os.MkdirAll(deviceDir, 0755)
	_ = os.WriteFile(filepath.Join(deviceDir, "Recents.xml"), []byte("<recents/>"), 0644)
	_ = os.WriteFile(filepath.Join(deviceDir, "Sources.xml"), []byte("<sources/>"), 0644)

	t.Run("Mirrored Endpoint", func(t *testing.T) {
		path := "/streaming/account/" + account + "/device/" + deviceID + "/recent"
		req, _ := http.NewRequest("GET", ts.URL+path, nil)
		// We set the host to our mock upstream so performMirror finds it
		req.Host = strings.TrimPrefix(boseUpstream.URL, "http://")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		// Wait a bit for the async mirror to complete and be recorded
		time.Sleep(500 * time.Millisecond)

		// Check if the interaction was recorded twice
		// Category: self
		matchesSelf, _ := filepath.Glob(filepath.Join(tempDir, "interactions", "*", "self", "*", "*"))
		if len(matchesSelf) == 0 {
			// List directory for debugging
			files, _ := filepath.Glob(filepath.Join(tempDir, "interactions", "*", "*", "*", "*"))
			t.Errorf("Expected to find local interaction in logs (category: self). Found: %v", files)
		}

		// Category: mirror
		matchesMirror, _ := filepath.Glob(filepath.Join(tempDir, "interactions", "*", "mirror", "*", "*"))
		if len(matchesMirror) == 0 {
			// List directory for debugging
			files, _ := filepath.Glob(filepath.Join(tempDir, "interactions", "*", "*", "*", "*"))
			t.Errorf("Expected to find mirrored interaction in logs (category: mirror). Found: %v", files)
		}
	})

	t.Run("Parity Mismatch Header Capture", func(t *testing.T) {
		// The previous test already triggered a mismatch because the bodies and content-types differ
		// local: <recents/> (from file), content-type: text/xml (default)
		// upstream: <bose-response/>, content-type: application/vnd.bose.streaming-v1.2+xml

		matchesMismatch, _ := filepath.Glob(filepath.Join(tempDir, "parity_mismatches", "*.json"))
		if len(matchesMismatch) == 0 {
			t.Fatal("Expected to find parity mismatch JSON file")
		}

		data, err := os.ReadFile(matchesMismatch[0])
		if err != nil {
			t.Fatalf("Failed to read mismatch file: %v", err)
		}

		var record struct {
			Local struct {
				Headers http.Header `json:"headers"`
			} `json:"local"`
			Upstream struct {
				Headers http.Header `json:"headers"`
			} `json:"upstream"`
		}

		if err := json.Unmarshal(data, &record); err != nil {
			t.Fatalf("Failed to unmarshal mismatch record: %v", err)
		}

		if len(record.Local.Headers) == 0 {
			t.Error("Expected local headers in parity mismatch, got none")
		}
		if len(record.Upstream.Headers) == 0 {
			t.Error("Expected upstream headers in parity mismatch, got none")
		}

		// Check specifically for Content-Type
		if ct := record.Local.Headers.Get("Content-Type"); ct == "" {
			t.Error("Expected Content-Type in local headers")
		}
		if ct := record.Upstream.Headers.Get("Content-Type"); ct != "application/vnd.bose.streaming-v1.2+xml" {
			t.Errorf("Expected Upstream Content-Type application/vnd.bose.streaming-v1.2+xml, got %s", ct)
		}
	})
}

// SetRecordEnabled is a helper for testing
func (s *Server) SetRecordEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordEnabled = enabled
}
