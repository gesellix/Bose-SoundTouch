package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// The scheduled subnet scan defaults to off. An omitted field must preserve
// the stored value (issue #589's shape), an explicit value must persist and
// take effect live, and "" turns it off again.
func TestDiscoveryScanIntervalSettingRoundTrip(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	if got := server.GetDiscoveryScanInterval(); got != 0 {
		t.Fatalf("default interval %v, want 0 (off)", got)
	}

	steps := []struct {
		name    string
		body    map[string]any
		want    time.Duration
		persist string
	}{
		{"set", map[string]any{"discovery_scan_interval": "6h"}, 6 * time.Hour, "6h0m0s"},
		{"omitted preserves", map[string]any{}, 6 * time.Hour, "6h0m0s"},
		{"empty turns off", map[string]any{"discovery_scan_interval": ""}, 0, "0s"},
		{"short value is accepted", map[string]any{"discovery_scan_interval": "10m"}, 10 * time.Minute, "10m0s"},
	}

	for _, step := range steps {
		step.body["server_url"] = "http://127.0.0.1:8000"

		if status := postSettings(t, ts, step.body); status != http.StatusOK {
			t.Fatalf("%s: status %d", step.name, status)
		}

		if got := server.GetDiscoveryScanInterval(); got != step.want {
			t.Errorf("%s: live interval %v, want %v", step.name, got, step.want)
		}

		persisted, err := ds.GetSettings()
		if err != nil {
			t.Fatalf("GetSettings: %v", err)
		}

		if persisted.DiscoveryScanInterval != step.persist {
			t.Errorf("%s: persisted %q, want %q", step.name, persisted.DiscoveryScanInterval, step.persist)
		}
	}
}

func TestDiscoveryScanIntervalRejectsInvalid(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	r, server := setupRouter("http://127.0.0.1:8000", ds)
	ts := httptest.NewServer(r)

	defer ts.Close()

	for _, bad := range []string{"-1h", "soon"} {
		if status := postSettings(t, ts, map[string]any{
			"server_url": "http://127.0.0.1:8000", "discovery_scan_interval": bad,
		}); status != http.StatusBadRequest {
			t.Errorf("%q: status %d, want 400", bad, status)
		}
	}

	if got := server.GetDiscoveryScanInterval(); got != 0 {
		t.Errorf("a rejected save changed the interval to %v", got)
	}
}

func TestHandleGetSettingsIncludesDiscoveryScanInterval(t *testing.T) {
	ds := datastore.NewDataStore(t.TempDir())
	_ = ds.Initialize()

	_, server := setupRouter("http://127.0.0.1:8000", ds)
	server.SetDiscoveryScanInterval(2 * time.Hour)

	rec := httptest.NewRecorder()
	server.HandleGetSettings(rec, httptest.NewRequest(http.MethodGet, "/setup/settings", nil))

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if body["discovery_scan_interval"] != "2h0m0s" {
		t.Errorf("discovery_scan_interval = %v", body["discovery_scan_interval"])
	}
}

// A scheduled sweep scans like a click, but without a client hint, and backs
// off when another sweep holds the slot.
func TestScanDevicesScheduled(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://10.0.0.2:8000")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("198.51.100.50")}

	if !s.ScanDevicesScheduled(context.Background()) {
		t.Fatal("scheduled sweep did not run")
	}

	if f.scanCalls != 1 || f.ssdpCalls != 1 {
		t.Errorf("ssdp=%d scan=%d, want both once", f.ssdpCalls, f.scanCalls)
	}

	s.beginDiscovery()

	if s.ScanDevicesScheduled(context.Background()) || f.scanCalls != 1 {
		t.Errorf("busy slot: scan=%d, want no second scan", f.scanCalls)
	}
}
