package handlers

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
)

func TestHandleProxyRequest_RequestBodyRecording(t *testing.T) {
	t.Setenv("RECORDER_ASYNC", "false")
	tmpDir, err := os.MkdirTemp("", "proxy-request-body-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Start a backend server to receive the proxied request
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the body to ensure it's consumed
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<response>ok</response>"))
	}))
	defer backend.Close()

	ds := datastore.NewDataStore(filepath.Join(tmpDir, "test.db"))
	server := NewServer(ds, nil, "http://localhost:8000", false, false, false, false)
	server.recordEnabled = true
	server.proxyLogBody = true
	recorder := proxy.NewRecorder(tmpDir)
	server.SetRecorder(recorder)

	// Create a proxy request to the backend
	requestBody := "<request>data</request>"
	targetURL := backend.URL
	proxyPath := "/proxy/" + targetURL
	req := httptest.NewRequest("POST", proxyPath, bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/xml")

	w := httptest.NewRecorder()
	server.HandleProxyRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify that the interaction was recorded and contains the request body
	sessionID := recorder.SessionID

	// The recorder uses sanitized segments for the directory.
	// Since the target URL is http://127.0.0.1:PORT, the path is empty,
	// so it should be in the "root" directory under the category.

	// We'll search recursively to be sure
	foundBody := false
	err = filepath.Walk(filepath.Join(tmpDir, "interactions", sessionID), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".http") {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(content), requestBody) {
				foundBody = true
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("failed to walk interactions dir: %v", err)
	}

	if !foundBody {
		t.Errorf("request body %q not found in any recorded interaction file", requestBody)
		// List all files found for debugging
		_ = filepath.Walk(filepath.Join(tmpDir, "interactions", sessionID), func(path string, info os.FileInfo, err error) error {
			if !info.IsDir() {
				content, _ := os.ReadFile(path)
				t.Logf("Found file %s with content:\n%s", path, string(content))
			}
			return nil
		})
	}
}
