package setup

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestSyncDeviceData_DestructiveSyncRequiresConfirmation is a regression
// test for #614's 2026-08-23 finding: SyncDeviceData used to overwrite the
// datastore unconditionally with whatever the speaker's live :8090 API
// returned, even if that snapshot had fewer presets than what was already
// stored — e.g. because the speaker's own local cache was stale or
// incomplete at that exact moment. This is a real, confirmed mechanism for
// "Sync wipes my presets" reports.
//
// A device already has 3 stored presets. The mock speaker's live /presets
// only reports 1. The first (unconfirmed) sync must NOT write anything and
// must report the shrink; a confirmed retry must apply it.
func TestSyncDeviceData_DestructiveSyncRequiresConfirmation(t *testing.T) {
	const (
		accountID = "1234567"
		deviceID  = "AABBCCDDEEFF"
	)

	mockDevice := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="%s">
    <name>Test Device</name>
    <type>SoundTouch 20</type>
    <margeAccountUUID>%s</margeAccountUUID>
</info>`, deviceID, accountID)
		case "/presets":
			// Only one preset survived on the speaker's own live cache —
			// the datastore already has three (seeded below).
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?>
<presets>
    <preset id="1">
        <ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="/custom/v1/playback/station1" isPresetable="true">
            <itemName>Station 1</itemName>
        </ContentItem>
    </preset>
</presets>`)
		case "/recents":
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0" encoding="UTF-8"?><recents></recents>`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockDevice.Close()

	tempDir, err := os.MkdirTemp("", "sync-destructive-guard-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	seeded := []models.ServicePreset{
		{ID: "1", ButtonNumber: "1", ServiceContentItem: models.ServiceContentItem{Name: "Station 1"}},
		{ID: "2", ButtonNumber: "2", ServiceContentItem: models.ServiceContentItem{Name: "Station 2"}},
		{ID: "3", ButtonNumber: "3", ServiceContentItem: models.ServiceContentItem{Name: "Station 3"}},
	}
	if err := ds.SavePresets(accountID, deviceID, seeded); err != nil {
		t.Fatalf("seed SavePresets: %v", err)
	}

	m := NewManager("http://localhost:8000", ds, nil)
	deviceIP := mockDevice.Listener.Addr().String()

	// Unconfirmed: must refuse to write and report the shrink.
	result, err := m.SyncDeviceData(deviceIP, false)
	if err != nil {
		t.Fatalf("SyncDeviceData(confirmed=false): %v", err)
	}

	if result.Applied {
		t.Fatal("expected unconfirmed destructive sync to NOT apply")
	}

	if !result.Destructive {
		t.Fatal("expected result.Destructive=true for a 3->1 preset shrink")
	}

	var presetDiff *SyncResourceDiff
	for i := range result.Diffs {
		if result.Diffs[i].Resource == "presets" {
			presetDiff = &result.Diffs[i]
		}
	}

	if presetDiff == nil {
		t.Fatal("expected a presets diff in the result")
	}

	if presetDiff.CurrentCount != 3 || presetDiff.IncomingCount != 1 {
		t.Errorf("expected presets diff 3->1, got %d->%d", presetDiff.CurrentCount, presetDiff.IncomingCount)
	}

	if len(presetDiff.Removed) != 2 {
		t.Errorf("expected 2 removed preset names (slots 2 and 3), got %v", presetDiff.Removed)
	}

	presetsAfterRefusal, err := ds.GetPresets(accountID, deviceID)
	if err != nil {
		t.Fatalf("GetPresets after refused sync: %v", err)
	}

	if len(presetsAfterRefusal) != 3 {
		t.Fatalf("expected the original 3 presets to survive an unconfirmed destructive sync, got %d", len(presetsAfterRefusal))
	}

	// Confirmed: must re-check fresh state and apply.
	result, err = m.SyncDeviceData(deviceIP, true)
	if err != nil {
		t.Fatalf("SyncDeviceData(confirmed=true): %v", err)
	}

	if !result.Applied {
		t.Fatal("expected confirmed destructive sync to apply")
	}

	presetsAfterConfirm, err := ds.GetPresets(accountID, deviceID)
	if err != nil {
		t.Fatalf("GetPresets after confirmed sync: %v", err)
	}

	if len(presetsAfterConfirm) != 1 {
		t.Fatalf("expected confirmed sync to shrink to 1 preset, got %d", len(presetsAfterConfirm))
	}
}
