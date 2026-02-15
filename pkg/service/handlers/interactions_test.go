package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/proxy"
	"github.com/go-chi/chi/v5"
)

func TestInteractionHandlers(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "interaction-handlers-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := datastore.NewDataStore(filepath.Join(tmpDir, "test.db"))
	server := &Server{ds: ds}

	t.Run("HandleGetInteractionStats_NoRecorder", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup/interaction-stats", nil)
		w := httptest.NewRecorder()
		server.HandleGetInteractionStats(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status 503, got %d", w.Code)
		}
	})

	recorder := proxy.NewRecorder(tmpDir)
	server.SetRecorder(recorder)

	// Create a dummy interaction file
	sessionID := recorder.SessionID
	relPath := filepath.Join(sessionID, "self", "test", "0001-12-00-00.000-GET.http")
	fullPath := filepath.Join(tmpDir, "interactions", relPath)
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	os.WriteFile(fullPath, []byte("### GET /test\n\n> {% \n    // Response: 200 OK\n%}\n"), 0644)

	r := chi.NewRouter()
	r.Route("/setup", func(r chi.Router) {
		r.Get("/interaction-stats", server.HandleGetInteractionStats)
		r.Get("/interactions", server.HandleListInteractions)
		r.Get("/interaction-content", server.HandleGetInteractionContent)
	})

	t.Run("HandleGetInteractionStats", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup/interaction-stats", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var stats proxy.InteractionStats
		if err := json.NewDecoder(w.Body).Decode(&stats); err != nil {
			t.Fatalf("Failed to decode stats: %v", err)
		}

		if stats.TotalRequests != 1 {
			t.Errorf("Expected 1 total request, got %d", stats.TotalRequests)
		}
	})

	t.Run("HandleListInteractions", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup/interactions?category=self", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var interactions []proxy.Interaction
		if err := json.NewDecoder(w.Body).Decode(&interactions); err != nil {
			t.Fatalf("Failed to decode interactions: %v", err)
		}

		if len(interactions) != 1 {
			t.Errorf("Expected 1 interaction, got %d", len(interactions))
		}
	})

	t.Run("HandleGetInteractionContent", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup/interaction-content?file="+relPath, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		if !strings.Contains(w.Body.String(), "### GET /test") {
			t.Errorf("Unexpected content: %s", w.Body.String())
		}
	})

	t.Run("HandleGetInteractionContent_MissingFile", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/setup/interaction-content", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", w.Code)
		}
	})
}

func TestRecordMiddleware(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "record-middleware-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ds := datastore.NewDataStore(filepath.Join(tmpDir, "test.db"))
	server := &Server{
		ds:            ds,
		recordEnabled: true,
	}
	recorder := proxy.NewRecorder(tmpDir)
	server.SetRecorder(recorder)

	r := chi.NewRouter()
	r.Use(server.RecordMiddleware)
	r.Get("/test-middleware", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "Value")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("created"))

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	})

	req := httptest.NewRequest("GET", "/test-middleware", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	t.Run("HandleRecordMiddleware_Disabled", func(t *testing.T) {
		server.recordEnabled = false
		req := httptest.NewRequest("GET", "/test-middleware", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d", w.Code)
		}
	})
}
