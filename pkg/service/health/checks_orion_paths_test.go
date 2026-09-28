package health

import (
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

const (
	orionTestData       = "eyJuYW1lIjoiRG9jIn0%3D"
	orionRelLocation    = "/station?data=" + orionTestData
	orionLocOldHost     = "http://192.0.2.5:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + orionTestData
	orionLocCurrentHost = "http://192.0.2.10:8000/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + orionTestData
	orionLocBoseCloud   = "https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=" + orionTestData
	orionSpeakerIP      = "192.0.2.20"
)

func newOrionTestDS(t *testing.T, account, device string) *datastore.DataStore {
	t.Helper()

	ds := datastore.NewDataStore(t.TempDir())

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		IPAddress: orionSpeakerIP,
		Name:      "Test Speaker",
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func writePresetsXML(t *testing.T, ds *datastore.DataStore, account, device, xml string) {
	t.Helper()

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml")
	if err := os.WriteFile(path, []byte(xml), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}
}

// presetsXML renders a service-side Presets.xml with one preset per
// (source, location) pair, in slots 1, 2, ...
func presetsXML(items ...[2]string) string {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<presets>\n")

	for i, it := range items {
		loc := strings.ReplaceAll(it[1], "&", "&amp;")
		b.WriteString(`  <preset id="` + string(rune('1'+i)) + `" createdOn="2026-05-01" updatedOn="2026-05-01">` + "\n")
		b.WriteString(`    <contentItem source="` + it[0] + `" type="stationurl" location="` + loc + `" sourceAccount="">` + "\n")
		b.WriteString("      <itemName>Station " + string(rune('1'+i)) + "</itemName>\n")
		b.WriteString("      <containerArt>http://192.0.2.10/art.png</containerArt>\n")
		b.WriteString("    </contentItem>\n  </preset>\n")
	}

	b.WriteString("</presets>")

	return b.String()
}

func orionDeps(dnsEnabled bool, queriers map[string]time.Time) OrionPathsDeps {
	return OrionPathsDeps{
		ServiceURLs:   func() (string, string) { return "http://192.0.2.10:8000", "https://192.0.2.10:8443" },
		ExpectedHosts: func() []string { return []string{"192.0.2.10", "aftertouch.example"} },
		DNSEnabled:    func() bool { return dnsEnabled },
		DNSClientIPs:  func() map[string]time.Time { return queriers },
	}
}

func runOrion(t *testing.T, ds *datastore.DataStore, deps OrionPathsDeps) CheckResult {
	t.Helper()

	r := NewRegistry()
	RegisterOrionPathsCheck(r, ds, deps)

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected one check result, got %d", len(results))
	}

	return results[0]
}

// Another host is info, not a warning: it can be deliberate (discussion 487)
// and can't be told apart from a stale AfterTouch address.
func TestOrionPaths_OtherHostIsInfoWithQuickFix(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML(
		[2]string{"LOCAL_INTERNET_RADIO", orionLocOldHost},
		[2]string{"LOCAL_INTERNET_RADIO", orionRelLocation},
	))

	res := runOrion(t, ds, orionDeps(false, nil))
	if res.Severity != SeverityInfo || len(res.Findings) != 1 {
		t.Fatalf("expected one info finding, got %+v", res)
	}

	f := res.Findings[0]
	if !strings.Contains(f.Message, "1 Internet Radio preset") || !strings.Contains(f.Message, "192.0.2.5:8000") {
		t.Errorf("message should name the count and the old host, got %q", f.Message)
	}

	if len(f.QuickFixes) != 1 || f.QuickFixes[0].ID != FixIDRewriteOrionRelative {
		t.Errorf("expected the rewrite quick fix, got %+v", f.QuickFixes)
	}

	if len(f.ManualCommands) != 1 {
		t.Fatalf("expected one manual command (slot 1), got %+v", f.ManualCommands)
	}

	cmd := f.ManualCommands[0].Command
	if !strings.Contains(cmd, orionSpeakerIP+":8090/storePreset") || !strings.Contains(cmd, `location="`+orionRelLocation+`"`) ||
		!strings.Contains(cmd, `<preset id="1">`) || !strings.Contains(cmd, "<itemName>Station 1</itemName>") {
		t.Errorf("manual command should store slot 1 with the relative location, got %q", cmd)
	}
}

func TestOrionPaths_CurrentHostIsInfo(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML(
		[2]string{"LOCAL_INTERNET_RADIO", orionLocCurrentHost},
		// Same service under another of its names, on its HTTPS port.
		[2]string{"LOCAL_INTERNET_RADIO", "https://AfterTouch.example:8443/core02/svc-bmx-adapter-orion/prod/orion/station?data=x"},
	))

	res := runOrion(t, ds, orionDeps(false, nil))
	if res.Severity != SeverityInfo || len(res.Findings) != 1 {
		t.Fatalf("expected one info finding, got %+v", res)
	}

	if !strings.Contains(res.Findings[0].Message, "current address") {
		t.Errorf("message should say the presets point at the current address, got %q", res.Findings[0].Message)
	}
}

func TestOrionPaths_SameHostOtherPortIsAnotherAddress(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML(
		[2]string{"LOCAL_INTERNET_RADIO", "http://192.0.2.10:9000/core02/svc-bmx-adapter-orion/prod/orion/station?data=x"},
	))

	res := runOrion(t, ds, orionDeps(false, nil))
	if len(res.Findings) != 1 || !strings.Contains(res.Findings[0].Message, "not at this service's address") {
		t.Fatalf("a location on another port is not this service, want the other-host finding, got %+v", res)
	}

	// Without the scheme and the hint, "192.0.2.10:9000, not at
	// http://192.0.2.10:8000" reads like the same address.
	msg := res.Findings[0].Message
	if !strings.Contains(msg, "slot 1 points at http://192.0.2.10:9000 (this service's host, but a port it doesn't serve)") {
		t.Errorf("message should show the scheme and say only the port differs, got %q", msg)
	}
}

// SERVER_URL names only the HTTPS side, while the service also listens for
// plain HTTP on port 80. A location http://<host>/... reaches this service
// and must not be reported as another host (issue 769 hardware run).
func TestOrionPaths_HTTPListenPortCountsAsThisService(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML(
		[2]string{"LOCAL_INTERNET_RADIO", "http://aftertouch.example/core02/svc-bmx-adapter-orion/prod/orion/station?data=x"},
		[2]string{"LOCAL_INTERNET_RADIO", "https://aftertouch.example/core02/svc-bmx-adapter-orion/prod/orion/station?data=y"},
	))

	deps := orionDeps(false, nil)
	deps.ServiceURLs = func() (string, string) { return "https://aftertouch.example", "https://aftertouch.example" }
	deps.ListenPorts = func() []string { return []string{"80", "443"} }

	res := runOrion(t, ds, deps)
	if len(res.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", res)
	}

	msg := res.Findings[0].Message
	if !strings.Contains(msg, "slots 1, 2 point at this service's current address") || strings.Contains(msg, "not at this service's address") {
		t.Errorf("both locations reach this service, got %q", msg)
	}
}

func TestOrionPaths_BoseCloudDependsOnDNS(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	cases := []struct {
		name     string
		dns      bool
		queriers map[string]time.Time
		severity Severity
		contains string
	}{
		{"dns off", false, nil, SeverityWarning, "shut-down Bose cloud"},
		{"dns on, speaker seen resolving through AfterTouch", true, map[string]time.Time{orionSpeakerIP: time.Now()}, SeverityInfo, "resolves through AfterTouch"},
		{"dns on, not confirmed", true, map[string]time.Time{"192.0.2.99": time.Now()}, SeverityInfo, "not yet confirmed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := newOrionTestDS(t, account, device)
			writePresetsXML(t, ds, account, device, presetsXML([2]string{"LOCAL_INTERNET_RADIO", orionLocBoseCloud}))

			res := runOrion(t, ds, orionDeps(tc.dns, tc.queriers))
			if res.Severity != tc.severity || len(res.Findings) != 1 {
				t.Fatalf("expected one %s finding, got %+v", tc.severity, res)
			}

			if !strings.Contains(res.Findings[0].Message, tc.contains) {
				t.Errorf("message should contain %q, got %q", tc.contains, res.Findings[0].Message)
			}

			if strings.Contains(res.Findings[0].Details, "/v1/playback") {
				t.Errorf("details must not suggest the TuneIn path for a station, got %q", res.Findings[0].Details)
			}
		})
	}
}

func TestOrionPaths_IgnoresRelativeAndNonOrionPresets(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML(
		[2]string{"LOCAL_INTERNET_RADIO", orionRelLocation},
		[2]string{"TUNEIN", "/v1/playback/station/s1234"},
		[2]string{"RADIO_BROWSER", "/stations/byuuid/abc"},
		[2]string{"LOCAL_INTERNET_RADIO", "http://stream.example.org/live.mp3"},
		// Another source never counts, whatever its location looks like.
		[2]string{"TUNEIN", orionLocOldHost},
	))

	res := runOrion(t, ds, orionDeps(false, nil))
	if res.Severity != SeverityOK || len(res.Findings) != 0 {
		t.Fatalf("expected no findings, got %+v", res.Findings)
	}
}

func TestOrionPaths_NoFindingWhenNoPresetsFile(t *testing.T) {
	ds := newOrionTestDS(t, "1000001", "DEVICEID01")

	if res := runOrion(t, ds, orionDeps(false, nil)); len(res.Findings) != 0 {
		t.Errorf("expected no findings for missing Presets.xml, got %+v", res.Findings)
	}
}

// fakeOrionSpeaker serves /presets from a fixed list and records every
// /storePreset body.
type fakeOrionSpeaker struct {
	mu     sync.Mutex
	stored []models.Preset
}

func (f *fakeOrionSpeaker) server(t *testing.T, presetsBody string) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/presets":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, presetsBody)
		case "/storePreset":
			var p models.Preset
			if err := xml.NewDecoder(r.Body).Decode(&p); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			f.mu.Lock()
			f.stored = append(f.stored, p)
			f.mu.Unlock()

			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return srv
}

const orionSpeakerPresetsXML = `<?xml version="1.0" encoding="UTF-8" ?>
<presets>
  <preset id="1" createdOn="1" updatedOn="1"><ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="` + orionLocOldHost + `" sourceAccount="" isPresetable="true"><itemName>Doc Radio</itemName><containerArt>http://192.0.2.10/doc.png</containerArt></ContentItem></preset>
  <preset id="2" createdOn="1" updatedOn="1"><ContentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s1234" sourceAccount="" isPresetable="true"><itemName>TuneIn</itemName></ContentItem></preset>
  <preset id="3" createdOn="1" updatedOn="1"><ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="/station?data=other" sourceAccount="" isPresetable="true"><itemName>Already relative</itemName></ContentItem></preset>
  <preset id="4" createdOn="1" updatedOn="1"><ContentItem source="LOCAL_INTERNET_RADIO" type="stationurl" location="http://stream.example.org/live.mp3" sourceAccount="" isPresetable="true"><itemName>Raw stream</itemName></ContentItem></preset>
  <preset id="5" createdOn="1" updatedOn="1"><ContentItem source="TUNEIN" type="stationurl" location="` + orionLocOldHost + `" sourceAccount="" isPresetable="true"><itemName>Not Orion source</itemName></ContentItem></preset>
</presets>`

func TestOrionPaths_QuickFixRewritesOnlyOrionSlotsOnTheSpeaker(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML(
		[2]string{"LOCAL_INTERNET_RADIO", orionLocOldHost},
		[2]string{"TUNEIN", "/v1/playback/station/s1234"},
	))

	speaker := &fakeOrionSpeaker{}
	srv := speaker.server(t, orionSpeakerPresetsXML)

	deps := orionDeps(false, nil)
	deps.NewSpeakerClient = func(ip string) *client.Client {
		if ip != orionSpeakerIP {
			t.Errorf("fix contacted %q, want the device's IP %q", ip, orionSpeakerIP)
		}

		return client.NewClient(&client.Config{Host: srv.URL, Timeout: 2 * time.Second})
	}

	r := NewRegistry()
	RegisterOrionPathsCheck(r, ds, deps)

	msg, _, err := r.RunFix(CheckIDOrionPaths, FixIDRewriteOrionRelative, Target{Account: account, Device: device})
	if err != nil {
		t.Fatalf("RunFix: %v", err)
	}

	if !strings.Contains(msg, "slot(s) 1 ") {
		t.Errorf("message should name slot 1, got %q", msg)
	}

	if len(speaker.stored) != 1 {
		t.Fatalf("expected exactly one /storePreset (slot 1), got %d: %+v", len(speaker.stored), speaker.stored)
	}

	got := speaker.stored[0]
	if got.ID != 1 || got.ContentItem == nil {
		t.Fatalf("stored preset = %+v", got)
	}

	ci := got.ContentItem
	if ci.Location != orionRelLocation || ci.Source != "LOCAL_INTERNET_RADIO" || ci.Type != "stationurl" ||
		ci.ItemName != "Doc Radio" || ci.ContainerArt != "http://192.0.2.10/doc.png" || !ci.IsPresetable {
		t.Errorf("stored content item = %+v, want the same item with the relative location", ci)
	}

	presets, err := ds.GetPresets(account, device)
	if err != nil {
		t.Fatalf("GetPresets: %v", err)
	}

	if presets[0].Location != orionRelLocation {
		t.Errorf("datastore slot 1 = %q, want the relative location", presets[0].Location)
	}

	if presets[1].Location != "/v1/playback/station/s1234" {
		t.Errorf("datastore slot 2 changed to %q", presets[1].Location)
	}

	if res := r.RunAll(); res[0].Severity != SeverityOK {
		t.Errorf("after the fix the check should pass, got %+v", res[0].Findings)
	}
}

func TestOrionPaths_QuickFixChangesNothingWhenSpeakerUnreachable(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newOrionTestDS(t, account, device)
	writePresetsXML(t, ds, account, device, presetsXML([2]string{"LOCAL_INTERNET_RADIO", orionLocOldHost}))

	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // nothing listens there any more

	deps := orionDeps(false, nil)
	deps.NewSpeakerClient = func(string) *client.Client {
		return client.NewClient(&client.Config{Host: srv.URL, Timeout: time.Second})
	}

	r := NewRegistry()
	RegisterOrionPathsCheck(r, ds, deps)

	_, _, err := r.RunFix(CheckIDOrionPaths, FixIDRewriteOrionRelative, Target{Account: account, Device: device})
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("expected an unreachable-speaker error, got %v", err)
	}

	presets, _ := ds.GetPresets(account, device)
	if presets[0].Location != orionLocOldHost {
		t.Errorf("datastore changed without a speaker write: %q", presets[0].Location)
	}
}
