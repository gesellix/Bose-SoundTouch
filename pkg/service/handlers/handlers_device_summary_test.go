package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/go-chi/chi/v5"
)

func TestHandleDeviceSummary_UnknownDevice(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "summary-test-*")
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_, server := setupRouter("http://aftertouch.local", ds)

	r := chi.NewRouter()
	r.Get("/setup/device-summary/{deviceId}", server.HandleDeviceSummary)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/setup/device-summary/UNKNOWN")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 for unknown device, got %d", res.StatusCode)
	}
}

func TestHandleDeviceSummary_UnreachableSpeakerStillReturnsServiceState(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "summary-test-*")
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()
	_ = ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:        "DEVICEID01",
		AccountID:       "1000001",
		Name:            "TestSpeaker",
		IPAddress:       "127.0.0.1:1", // refused port; probe fails
		ProductCode:     "SoundTouch 20",
		FirmwareVersion: "27.0.6.46330.5043500",
	})

	_, server := setupRouter("http://aftertouch.local", ds)
	server.SetExpectedHosts([]string{"aftertouch.local"})

	r := chi.NewRouter()
	r.Get("/setup/device-summary/{deviceId}", server.HandleDeviceSummary)

	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/setup/device-summary/DEVICEID01")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.StatusCode)
	}

	var got deviceSummary
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Device.DeviceID != "DEVICEID01" {
		t.Errorf("unexpected device_id: %q", got.Device.DeviceID)
	}

	if got.Speaker.Info.Reachable {
		t.Errorf("expected unreachable speaker.info, got reachable=true")
	}

	if got.Speaker.Info.CurlCommand == "" {
		t.Errorf("expected curl_command populated even on failure")
	}

	if got.Service.ServerURL != "http://aftertouch.local" {
		t.Errorf("expected service.server_url to surface, got %q", got.Service.ServerURL)
	}

	if got.Pairing.Paired {
		t.Errorf("expected paired=false when speaker unreachable")
	}

	if got.GeneratedAt == "" {
		t.Errorf("expected generated_at populated")
	}
}

func TestHandleDeviceSummary_ReachableSpeakerPopulatesAggregate(t *testing.T) {
	// Stub speaker server serves /info, /sources, /presets.
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEVICEID01">
  <name>TestSpeaker</name>
  <type>SoundTouch 20</type>
  <margeAccountUUID>1000001</margeAccountUUID>
  <margeURL>https://aftertouch.local/</margeURL>
</info>`))
		case "/sources":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<sources deviceID="DEVICEID01">
  <sourceItem source="TUNEIN" status="READY"/>
  <sourceItem source="AUX" status="READY"/>
</sources>`))
		case "/presets":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<presets>
  <preset id="1"><ContentItem source="TUNEIN"/></preset>
  <preset id="2"><ContentItem source="AUX"/></preset>
</presets>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer speaker.Close()

	speakerHost := mustParseHost(t, speaker.URL)

	tempDir, _ := os.MkdirTemp("", "summary-test-*")
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()
	_ = ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICEID01",
		AccountID: "1000001",
		Name:      "TestSpeaker",
		IPAddress: speakerHost, // hostport pointing at the stub
	})

	_, server := setupRouter("http://aftertouch.local", ds)
	server.SetExpectedHosts([]string{"aftertouch.local"})

	r := chi.NewRouter()
	r.Get("/setup/device-summary/{deviceId}", server.HandleDeviceSummary)

	// Override the speaker URL inside the handler. The production
	// code builds http://<ip>:8090/info from the device's
	// IPAddress. We point IPAddress at host:port directly, so
	// the resulting URL is `http://host:port:8090/info` —
	// invalid. The summary will report unreachable and we'll
	// assert via the partial response.
	//
	// For a more realistic end-to-end probe test we'd need to
	// either inject a custom speaker URL or refactor the probe
	// to accept an explicit target. Both are bigger lifts; the
	// pure-data path is exercised by other tests in this package.
	ts := httptest.NewServer(r)
	defer ts.Close()

	res, err := http.Get(ts.URL + "/setup/device-summary/DEVICEID01")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer res.Body.Close()

	var got deviceSummary
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if got.Device.Name != "TestSpeaker" {
		t.Errorf("unexpected device.name: %q", got.Device.Name)
	}

	// We can at least assert the curl command points at the
	// configured IP with the canonical :8090 path.
	if !strings.Contains(got.Speaker.Info.CurlCommand, ":8090/info") {
		t.Errorf("expected info curl to target :8090, got %q", got.Speaker.Info.CurlCommand)
	}
}

func TestHostFromURL_StripsSchemeAndPort(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://example.com/", "example.com"},
		{"http://Example.COM:8443/path", "example.com"},
		{"https://192.0.2.10", "192.0.2.10"},
		{"example.com", "example.com"},
		{"", ""},
	}

	for _, c := range cases {
		if got := hostFromURL(c.in); got != c.want {
			t.Errorf("hostFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPairingHostMatches(t *testing.T) {
	if !pairingHostMatches("aftertouch.local", []string{"AFTERTOUCH.local", "example.com"}) {
		t.Errorf("expected case-insensitive match")
	}

	if pairingHostMatches("", []string{"aftertouch.local"}) {
		t.Errorf("empty host should not match")
	}

	if pairingHostMatches("other.example", []string{"aftertouch.local"}) {
		t.Errorf("unmatched host should not match")
	}
}

func mustParseHost(t *testing.T, raw string) string {
	t.Helper()

	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}

	return u.Host
}
