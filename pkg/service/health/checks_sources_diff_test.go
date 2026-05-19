package health

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newSourcesDiffDS(t *testing.T, account, device string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "sources-diff-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func setServiceSources(t *testing.T, ds *datastore.DataStore, account, device string, types ...string) {
	t.Helper()

	sources := make([]models.ConfiguredSource, 0, len(types))
	for i, ty := range types {
		var src models.ConfiguredSource
		src.ID = "1000" + string(rune('0'+i))
		src.Type = "Audio"
		src.SourceKey.Type = ty
		sources = append(sources, src)
	}

	if err := ds.SaveConfiguredSources(account, device, sources); err != nil {
		t.Fatalf("SaveConfiguredSources: %v", err)
	}
}

func stubSpeakerSourcesServer(t *testing.T, sources ...string) string {
	t.Helper()

	var body strings.Builder
	body.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	body.WriteString(`<sources deviceID="DEVICEID01">` + "\n")
	for _, s := range sources {
		body.WriteString(`  <sourceItem source="` + s + `" status="READY"/>` + "\n")
	}
	body.WriteString(`</sources>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sources" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body.String()))
	}))
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)

	return "http://" + u.Host + "/sources"
}

func TestSourcesDiff_FlagsMissingOnSpeaker(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newSourcesDiffDS(t, account, device)
	setServiceSources(t, ds, account, device, "TUNEIN", "RADIO_BROWSER", "AUX")

	speakerURL := stubSpeakerSourcesServer(t, "AUX") // missing TUNEIN, RADIO_BROWSER

	got := diffSourcesForDeviceWithURL(ds, account, device, "192.0.2.10", speakerURL)
	if len(got) == 0 {
		t.Fatalf("expected at least one finding")
	}

	var foundMissing bool
	for _, f := range got {
		if strings.Contains(f.Message, "missing") && f.Severity == SeverityWarning {
			foundMissing = true
			if !strings.Contains(f.Message, "TUNEIN") || !strings.Contains(f.Message, "RADIO_BROWSER") {
				t.Errorf("expected TUNEIN + RADIO_BROWSER in message, got %q", f.Message)
			}

			if len(f.ManualCommands) != 1 || !strings.Contains(f.ManualCommands[0].Command, "/notification") {
				t.Errorf("expected notify command, got %+v", f.ManualCommands)
			}

			if !strings.Contains(f.ManualCommands[0].Command, "192.0.2.10") {
				t.Errorf("notify command should target the device IP, got %q", f.ManualCommands[0].Command)
			}
		}
	}

	if !foundMissing {
		t.Errorf("expected a 'missing on speaker' warning, got %+v", got)
	}
}

func TestSourcesDiff_FlagsMissingOnService(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newSourcesDiffDS(t, account, device)
	setServiceSources(t, ds, account, device, "AUX")

	speakerURL := stubSpeakerSourcesServer(t, "AUX", "BLUETOOTH")

	got := diffSourcesForDeviceWithURL(ds, account, device, "192.0.2.10", speakerURL)

	var foundExtra bool
	for _, f := range got {
		if strings.Contains(f.Message, "doesn't know about") && f.Severity == SeverityInfo {
			foundExtra = true
			if !strings.Contains(f.Message, "BLUETOOTH") {
				t.Errorf("expected BLUETOOTH in message, got %q", f.Message)
			}
		}
	}

	if !foundExtra {
		t.Errorf("expected an info finding for sources missing on service, got %+v", got)
	}
}

func TestSourcesDiff_NoFindingsWhenMatched(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newSourcesDiffDS(t, account, device)
	setServiceSources(t, ds, account, device, "TUNEIN", "AUX")

	speakerURL := stubSpeakerSourcesServer(t, "TUNEIN", "AUX")

	got := diffSourcesForDeviceWithURL(ds, account, device, "192.0.2.10", speakerURL)
	if len(got) != 0 {
		t.Errorf("expected no findings, got %+v", got)
	}
}

func TestSourcesDiff_UnreachableSpeakerEmitsManualCommand(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newSourcesDiffDS(t, account, device)
	setServiceSources(t, ds, account, device, "TUNEIN")

	// Refused port; probe fails.
	got := diffSourcesForDeviceWithURL(ds, account, device, "127.0.0.1", "http://127.0.0.1:1/sources")
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding for unreachable speaker, got %+v", got)
	}

	if len(got[0].ManualCommands) != 1 {
		t.Errorf("expected a manual command, got %+v", got[0].ManualCommands)
	}
}

func TestSourcesDiff_MalformedXMLWarns(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not xml"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	probeURL := "http://" + u.Host + "/sources"

	account, device := "1000001", "DEVICEID01"

	ds := newSourcesDiffDS(t, account, device)
	setServiceSources(t, ds, account, device, "TUNEIN")

	got := diffSourcesForDeviceWithURL(ds, account, device, "192.0.2.10", probeURL)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning for malformed XML, got %+v", got)
	}
}

func TestSourcesDiff_EmptyServiceSetDoesNotDoubleWarn(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newSourcesDiffDS(t, account, device)
	// Don't write any Sources.xml — sources_xml_present already flags this.

	speakerURL := stubSpeakerSourcesServer(t, "AUX", "TUNEIN")

	got := diffSourcesForDeviceWithURL(ds, account, device, "192.0.2.10", speakerURL)

	for _, f := range got {
		if f.Severity == SeverityWarning && strings.Contains(f.Message, "missing") {
			t.Errorf("should not emit a 'missing on speaker' warning when service set is empty, got %+v", f)
		}
	}
}
