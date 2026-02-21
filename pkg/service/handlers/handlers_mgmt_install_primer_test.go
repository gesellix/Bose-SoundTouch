package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
)

func TestMgmtInstallSpotifyPrimer(t *testing.T) {
	// Prepare server with a mocked SSH to avoid real connections
	tmpDir := t.TempDir()
	ds := datastore.NewDataStore(tmpDir)
	_ = ds.Initialize()

	s := &Server{ds: ds}

	sm := setup.NewManager("http://localhost:8000", ds, nil)
	sm.NewSSH = func(host string) setup.SSHClient { return &mockSSH{host: host} }
	s.sm = sm

	r := chi.NewRouter()
	r.Post("/mgmt/devices/{deviceId}/spotify/install-primer", s.HandleMgmtInstallSpotifyPrimer)

	req := httptest.NewRequest(http.MethodPost, "/mgmt/devices/192.168.1.10/spotify/install-primer", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", w.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Errorf("expected ok=true, got %v", result["ok"])
	}
	if _, ok := result["output"]; !ok {
		t.Errorf("expected output field in response")
	}
}
