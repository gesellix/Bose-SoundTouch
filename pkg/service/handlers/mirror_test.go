package handlers

import (
	"encoding/json"
	"fmt"
	"io"
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
	server.SetMirrorSettings(true, []string{"/streaming/account/*/device/*/recent"}, nil, "local")

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

	t.Run("POST Request Body Preservation", func(t *testing.T) {
		// Set recorder to synchronous mode for testing
		os.Setenv("RECORDER_ASYNC", "false")
		defer os.Unsetenv("RECORDER_ASYNC")

		// Create a mock upstream that echoes back the request body
		postUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/scmudc/A81B6A536A98") {
				// Read the request body
				body, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}

				// Echo back the body in response for verification
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Request-Body-Length", fmt.Sprintf("%d", len(body)))
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer postUpstream.Close()

		// Setup mirroring for the POST endpoint
		server.SetMirrorSettings(true, []string{"/v1/scmudc/*"}, nil, "local")

		requestBody := `{"envelope":{"monoTime":234906,"payloadProtocolVersion":"3.1","payloadType":"scmudc","protocolVersion":"1.0","time":"2026-02-25T23:03:14.976349+00:00","uniqueId":"A81B6A536A98"},"payload":{"deviceInfo":{"boseID":"3230304","deviceID":"A81B6A536A98","deviceType":"SoundTouch 10","serialNumber":"I6332527703739342000020","softwareVersion":"27.0.6.46330.5043500 epdbuild.trunk.hepdswbld04.2022-08-04T11:20:29","systemSerialNumber":"069231P63364828AE"},"events":[{"data":{"play-state":"PAUSE_STATE"},"monoTime":234904,"time":"2026-02-25T23:03:14.973466+00:00","type":"play-state-changed"}]}}`

		path := "/v1/scmudc/A81B6A536A98"
		req, _ := http.NewRequest("POST", ts.URL+path, strings.NewReader(requestBody))
		req.Header.Set("Content-Type", "text/json; charset=utf-8")
		req.Host = strings.TrimPrefix(postUpstream.URL, "http://")

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()

		if res.StatusCode != http.StatusOK {
			t.Errorf("Expected status OK, got %v", res.Status)
		}

		// Wait briefly for the synchronous recording to complete
		time.Sleep(100 * time.Millisecond)

		// Check if the mirrored interaction was recorded with the request body
		matchesMirror, _ := filepath.Glob(filepath.Join(tempDir, "interactions", "*", "mirror", "v1", "scmudc", "*", "*-POST.http"))
		if len(matchesMirror) == 0 {
			// Try broader search pattern
			allHttpFiles, _ := filepath.Glob(filepath.Join(tempDir, "interactions", "*", "*", "*", "*", "*", "*.http"))
			t.Errorf("Expected to find mirrored POST interaction. All .http files found: %v", allHttpFiles)
		} else {
			// Read the recorded mirrored interaction
			recordedContent, err := os.ReadFile(matchesMirror[0])
			if err != nil {
				t.Fatalf("Failed to read recorded mirror interaction: %v", err)
			}

			recordedStr := string(recordedContent)

			// Check if the request body was preserved in the recording
			if !strings.Contains(recordedStr, requestBody) {
				t.Errorf("Request body not found in mirrored recording. Content: %s", recordedStr)
			}

			// Check if the Content-Type header was preserved
			if !strings.Contains(recordedStr, "Content-Type: text/json; charset=utf-8") {
				t.Errorf("Content-Type header not found in mirrored recording. Content: %s", recordedStr)
			}
		}
	})
}

// SetRecordEnabled is a helper for testing
func (s *Server) SetRecordEnabled(enabled bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recordEnabled = enabled
}
