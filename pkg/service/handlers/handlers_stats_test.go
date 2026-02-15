package handlers

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func TestStatsHandlers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "st-test-*")
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)
	s := &Server{ds: ds}

	t.Run("HandleUsageStats XML", func(t *testing.T) {
		xmlData := `
<usageStats>
    <deviceId>device123</deviceId>
    <accountId>account456</accountId>
    <timestamp>2023-10-27T10:00:00Z</timestamp>
    <eventType>PLAYBACK_START</eventType>
</usageStats>`
		req := httptest.NewRequest("POST", "/streaming/stats/usage", bytes.NewBufferString(xmlData))
		w := httptest.NewRecorder()

		s.HandleUsageStats(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}

		// Verify file creation
		files, _ := filepath.Glob(filepath.Join(tempDir, "stats", "usage", "*.json"))
		if len(files) == 0 {
			t.Error("Usage stats file was not created")
		}
	})

	t.Run("HandleErrorStats JSON", func(t *testing.T) {
		jsonData := `{"deviceId": "device123", "errorCode": "404", "errorMessage": "Not Found"}`
		req := httptest.NewRequest("POST", "/streaming/stats/error", bytes.NewBufferString(jsonData))
		w := httptest.NewRecorder()

		s.HandleErrorStats(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status OK, got %d", w.Code)
		}

		// Verify file creation
		files, _ := filepath.Glob(filepath.Join(tempDir, "stats", "error", "*.json"))
		if len(files) == 0 {
			t.Error("Error stats file was not created")
		}
	})
}
