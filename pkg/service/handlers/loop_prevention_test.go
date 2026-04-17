package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestHandleBoseProxy_LoopPrevention(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "proxy-loop-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := datastore.NewDataStore(filepath.Join(tmpDir, "test.db"))
	server := NewServer(ds, nil, "http://localhost", false, false, false)

	t.Run("first hop should be allowed", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/unknown-endpoint", nil)
		req.Host = "localhost"
		w := httptest.NewRecorder()
		server.HandleBoseProxy(w, req)
		if w.Code == http.StatusNotFound {
			t.Errorf("Expected first hop to be allowed (even if it fails later), but got 404")
		}
	})

	t.Run("second hop should be blocked", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/unknown-endpoint", nil)
		req.Host = "localhost"
		req.Header.Set("X-Bose-Proxy-Hop", "1")
		w := httptest.NewRecorder()
		server.HandleBoseProxy(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected second hop to be blocked with 404, but got %d", w.Code)
		}
	})

	t.Run("HandleProxyRequest loop detection", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/proxy/http://example.com", nil)
		req.Header.Set("X-Bose-Proxy-Hop", "1")
		w := httptest.NewRecorder()
		server.HandleProxyRequest(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected HandleProxyRequest loop to be blocked with 404, but got %d", w.Code)
		}
	})
}
