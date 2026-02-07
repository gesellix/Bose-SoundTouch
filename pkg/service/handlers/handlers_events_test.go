package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/go-chi/chi/v5"
)

func TestEventLog(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	s := &Server{ds: ds}

	r := chi.NewRouter()
	r.Post("/streaming/stats/usage", s.HandleUsageStats)
	r.Get("/setup/devices/{deviceId}/events", s.HandleGetDeviceEvents)

	t.Run("Record and Retrieve Events", func(t *testing.T) {
		// 1. Post a usage stat
		usageBody := `{
			"deviceId": "SPEAKER1",
			"eventType": "play-start",
			"parameters": {"source": "TUNEIN"}
		}`
		req, _ := http.NewRequest("POST", "/streaming/stats/usage", strings.NewReader(usageBody))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}

		// 2. Retrieve events
		req, _ = http.NewRequest("GET", "/setup/devices/SPEAKER1/events", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}

		var resp struct {
			Events []models.DeviceEvent `json:"events"`
		}
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("Failed to decode response: %v", err)
		}

		if len(resp.Events) != 1 {
			t.Fatalf("Expected 1 event, got %d", len(resp.Events))
		}

		if resp.Events[0].Type != "play-start" {
			t.Errorf("Expected event type 'play-start', got %q", resp.Events[0].Type)
		}
	})
}
