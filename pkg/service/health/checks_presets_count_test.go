package health

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newPresetsCountDS(t *testing.T, account, device string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "presets-count-test-*")
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

func writeServicePresets(t *testing.T, ds *datastore.DataStore, account, device string, count int) {
	t.Helper()

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n<presets>\n")

	for i := 1; i <= count; i++ {
		b.WriteString(`  <preset id="`)
		b.WriteString(itoa(i))
		b.WriteString(`" createdOn="2026-05-01" updatedOn="2026-05-01">
    <contentItem source="TUNEIN" type="stationurl" location="/v1/playback/station/s` + itoa(i) + `">
      <itemName>Slot ` + itoa(i) + `</itemName>
    </contentItem>
  </preset>` + "\n")
	}

	b.WriteString("</presets>\n")

	path := filepath.Join(ds.AccountDeviceDir(account, device), "Presets.xml")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatalf("write Presets.xml: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}

	neg := i < 0
	if neg {
		i = -i
	}

	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}

	if neg {
		pos--
		buf[pos] = '-'
	}

	return string(buf[pos:])
}

func stubSpeakerPresetsServer(t *testing.T, count int) string {
	t.Helper()

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<presets>` + "\n")

	for i := 1; i <= count; i++ {
		b.WriteString(`<preset id="` + itoa(i) + `"><ContentItem source="TUNEIN"/></preset>` + "\n")
	}

	b.WriteString(`</presets>`)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/presets" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(b.String()))
	}))
	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)

	return "http://" + u.Host + "/presets"
}

func TestPresetsCount_MatchingProducesNoFinding(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newPresetsCountDS(t, account, device)
	writeServicePresets(t, ds, account, device, 3)

	probeURL := stubSpeakerPresetsServer(t, 3)

	got := comparePresetsForDeviceWithURL(ds, account, device, probeURL)
	if len(got) != 0 {
		t.Errorf("expected no findings when counts match, got %+v", got)
	}
}

func TestPresetsCount_SpeakerEmptyWhileServiceHas(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newPresetsCountDS(t, account, device)
	writeServicePresets(t, ds, account, device, 3)

	probeURL := stubSpeakerPresetsServer(t, 0)

	got := comparePresetsForDeviceWithURL(ds, account, device, probeURL)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "0 preset") || !strings.Contains(got[0].Message, "3") {
		t.Errorf("expected counts in message, got %q", got[0].Message)
	}
}

func TestPresetsCount_SpeakerHasMore(t *testing.T) {
	account, device := "1000001", "DEVICEID01"

	ds := newPresetsCountDS(t, account, device)
	writeServicePresets(t, ds, account, device, 1)

	probeURL := stubSpeakerPresetsServer(t, 3)

	got := comparePresetsForDeviceWithURL(ds, account, device, probeURL)
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding, got %+v", got)
	}
}

func TestPresetsCount_UnreachableSpeaker(t *testing.T) {
	account, device := "1000001", "DEVICEID01"
	ds := newPresetsCountDS(t, account, device)
	writeServicePresets(t, ds, account, device, 2)

	got := comparePresetsForDeviceWithURL(ds, account, device, "http://127.0.0.1:1/presets")
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding for unreachable speaker, got %+v", got)
	}

	if len(got[0].ManualCommands) != 1 {
		t.Errorf("expected manual command on unreachable case, got %+v", got[0].ManualCommands)
	}
}

func TestPresetsCount_MalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("nope"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	probeURL := "http://" + u.Host + "/presets"

	account, device := "1000001", "DEVICEID01"
	ds := newPresetsCountDS(t, account, device)
	writeServicePresets(t, ds, account, device, 1)

	got := comparePresetsForDeviceWithURL(ds, account, device, probeURL)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected warning for malformed XML, got %+v", got)
	}
}
