package handlers

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
)

func TestHandleNotFound_UnhandledLogging(t *testing.T) {
	// backend absorbs proxied requests so the test doesn't hit the real Bose upstream
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	backendHost := strings.TrimPrefix(backend.URL, "http://")

	captureLog := func(fn func()) string {
		var buf bytes.Buffer
		log.SetOutput(&buf)
		defer log.SetOutput(os.Stderr)
		fn()
		return buf.String()
	}

	t.Run("always logs [UNHANDLED] with method and path", func(t *testing.T) {
		ds := datastore.NewDataStore(t.TempDir())
		server := NewServer(ds, nil, "http://localhost", false, false, false)

		req := httptest.NewRequest("GET", "/some/unknown/path", nil)
		req.Host = backendHost

		logged := captureLog(func() {
			server.HandleNotFound(httptest.NewRecorder(), req)
		})

		if !strings.Contains(logged, "[UNHANDLED]") {
			t.Errorf("expected [UNHANDLED] in log, got: %s", logged)
		}
		if !strings.Contains(logged, "GET") || !strings.Contains(logged, "/some/unknown/path") {
			t.Errorf("expected method and path in log, got: %s", logged)
		}
	})

	t.Run("includes body in log when proxyLogBody is true", func(t *testing.T) {
		ds := datastore.NewDataStore(t.TempDir())
		server := NewServer(ds, nil, "http://localhost", false, true, false)

		req := httptest.NewRequest("POST", "/marge/unknown", bytes.NewBufferString("<payload/>"))
		req.Host = backendHost

		logged := captureLog(func() {
			server.HandleNotFound(httptest.NewRecorder(), req)
		})

		if !strings.Contains(logged, "<payload/>") {
			t.Errorf("expected body in log when proxyLogBody=true, got: %s", logged)
		}
	})

	t.Run("omits body from log when proxyLogBody is false", func(t *testing.T) {
		ds := datastore.NewDataStore(t.TempDir())
		server := NewServer(ds, nil, "http://localhost", false, false, false)

		req := httptest.NewRequest("POST", "/marge/unknown", bytes.NewBufferString("<secret/>"))
		req.Host = backendHost

		logged := captureLog(func() {
			server.HandleNotFound(httptest.NewRecorder(), req)
		})

		if strings.Contains(logged, "<secret/>") {
			t.Errorf("expected body omitted when proxyLogBody=false, got: %s", logged)
		}
		if !strings.Contains(logged, "[UNHANDLED]") {
			t.Errorf("expected [UNHANDLED] even without body, got: %s", logged)
		}
	})

	t.Run("body is still forwarded to proxy after being read for logging", func(t *testing.T) {
		var receivedBody string
		forwardCheck := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := io.ReadAll(r.Body)
			receivedBody = string(b)
			w.WriteHeader(http.StatusOK)
		}))
		defer forwardCheck.Close()

		ds := datastore.NewDataStore(t.TempDir())
		server := NewServer(ds, nil, "http://localhost", false, true, false)

		req := httptest.NewRequest("POST", "/marge/unknown", bytes.NewBufferString("<forwarded/>"))
		req.Host = strings.TrimPrefix(forwardCheck.URL, "http://")

		captureLog(func() {
			server.HandleNotFound(httptest.NewRecorder(), req)
		})

		if receivedBody != "<forwarded/>" {
			t.Errorf("expected body forwarded to proxy, got: %q", receivedBody)
		}
	})
}

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
	server := NewServer(ds, nil, "http://localhost", false, false, false)
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
