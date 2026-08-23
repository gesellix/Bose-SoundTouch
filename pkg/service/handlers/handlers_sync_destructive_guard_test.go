package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
	"github.com/go-chi/chi/v5"
)

// TestHandleInitialSync_DestructiveSyncReturns409ThenAppliesWhenConfirmed is
// an HTTP-level regression test for #614's Sync-button data-loss bug (see
// setup.TestSyncDeviceData_DestructiveSyncRequiresConfirmation for the
// lower-level coverage of the same fix): a device already has more presets
// stored than the mock speaker's live /presets now reports. The first,
// unconfirmed sync request must come back 409 with the diff and must not
// write anything; a retry with ?confirmed=true must apply it.
func TestHandleInitialSync_DestructiveSyncReturns409ThenAppliesWhenConfirmed(t *testing.T) {
	const (
		accountID = "1234567"
		deviceID  = "AABBCCDDEEFF"
	)

	// A real local server, not a black-hole IP: notifySpeakerSourcesUpdated
	// (part of the confirmed-apply path) uses its own HTTP client rather
	// than the injectable sm.HTTPGet, so it needs somewhere real to fail
	// fast against (404) instead of timing out.
	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><info deviceID="%s"><name>Test Device</name><type>SoundTouch 20</type><margeAccountUUID>%s</margeAccountUUID></info>`, deviceID, accountID)
		case "/presets":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><presets><preset id="1"><ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="/x" isPresetable="true"><itemName>Station 1</itemName></ContentItem></preset></presets>`)
		case "/recents":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><recents></recents>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	deviceIP := mockDevice.Listener.Addr().String()

	tempDir, err := os.MkdirTemp("", "handlers-sync-destructive-guard-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	seeded := []models.ServicePreset{
		{ID: "1", ButtonNumber: "1", ServiceContentItem: models.ServiceContentItem{Name: "Station 1"}},
		{ID: "2", ButtonNumber: "2", ServiceContentItem: models.ServiceContentItem{Name: "Station 2"}},
	}
	if err := ds.SavePresets(accountID, deviceID, seeded); err != nil {
		t.Fatalf("seed SavePresets: %v", err)
	}

	if err := ds.SaveDeviceInfo(accountID, deviceID, &models.ServiceDeviceInfo{
		DeviceID:  deviceID,
		AccountID: accountID,
		IPAddress: deviceIP,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	sm := setup.NewManager("http://localhost:8000", ds, nil)

	server := NewServer(ds, sm, "http://localhost:8000", false, false, false)

	r := chi.NewRouter()
	r.Post("/api/setup/sync/{deviceId}", server.HandleInitialSync)

	ts := httptest.NewServer(r)
	defer ts.Close()

	// First, unconfirmed request: must be refused with 409.
	resp, err := http.Post(ts.URL+"/api/setup/sync/"+deviceID, "application/json", nil)
	if err != nil {
		t.Fatalf("POST sync (unconfirmed): %v", err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 409 for a destructive unconfirmed sync, got %d: %s", resp.StatusCode, body)
	}

	var result setup.SyncResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode 409 body: %v", err)
	}

	if result.Applied {
		t.Fatal("expected Applied=false in the 409 response")
	}

	if !result.Destructive {
		t.Fatal("expected Destructive=true in the 409 response")
	}

	presetsAfterRefusal, err := ds.GetPresets(accountID, deviceID)
	if err != nil {
		t.Fatalf("GetPresets after refused sync: %v", err)
	}

	if len(presetsAfterRefusal) != 2 {
		t.Fatalf("expected the original 2 presets to survive the refused sync, got %d", len(presetsAfterRefusal))
	}

	// Retry, confirmed: must apply.
	resp2, err := http.Post(ts.URL+"/api/setup/sync/"+deviceID+"?confirmed=true", "application/json", nil)
	if err != nil {
		t.Fatalf("POST sync (confirmed): %v", err)
	}

	defer func() { _ = resp2.Body.Close() }()

	if resp2.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp2.Body)
		t.Fatalf("expected 200 for a confirmed sync, got %d: %s", resp2.StatusCode, body)
	}

	var confirmedResult setup.SyncResult
	if err := json.NewDecoder(resp2.Body).Decode(&confirmedResult); err != nil {
		t.Fatalf("decode 200 body: %v", err)
	}

	if !confirmedResult.Applied {
		t.Fatal("expected Applied=true after confirming")
	}

	presetsAfterConfirm, err := ds.GetPresets(accountID, deviceID)
	if err != nil {
		t.Fatalf("GetPresets after confirmed sync: %v", err)
	}

	if len(presetsAfterConfirm) != 1 {
		t.Fatalf("expected confirmed sync to shrink to 1 preset, got %d", len(presetsAfterConfirm))
	}
}
