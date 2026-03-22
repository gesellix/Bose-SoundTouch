package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestMirrorMiddleware_InfiniteLoop(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mirror-loop-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://localhost:8000", false, false, false)
	server.SetMirrorSettings(true, []string{"/loop"}, nil, "upstream")

	// Create a handler that would be the "next" in the chain.
	// If the loop occurs, this will be called repeatedly.
	callCount := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	middleware := server.MirrorMiddleware(handler)

	// Simulate a mirror request by adding the X-Mirror-Request header.
	// This is what performMirror adds to the proxied request.
	req := httptest.NewRequest("GET", "/loop", nil)
	req.Header.Set("X-Mirror-Request", "true")
	w := httptest.NewRecorder()

	middleware.ServeHTTP(w, req)

	// If the fix is working, the middleware should see X-Mirror-Request and
	// pass directly to the handler WITHOUT trying to mirror again.
	if callCount != 1 {
		t.Errorf("Expected 1 call to handler, got %d", callCount)
	}

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}
