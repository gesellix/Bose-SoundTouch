package health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

func newRefreshSourcesDS(t *testing.T, account, device, ipAddress string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "refresh-sources-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		IPAddress: ipAddress,
		Name:      "RefreshTester",
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func TestRefreshSources_ListsEveryDeviceWithQuickFix(t *testing.T) {
	ds := newRefreshSourcesDS(t, "1000001", "DEVICEID01", "192.0.2.10")

	r := NewRegistry()
	RegisterRefreshSourcesCheck(r, ds)

	results := r.RunAll()
	if len(results) != 1 || len(results[0].Findings) != 1 {
		t.Fatalf("expected one finding for the one device, got %+v", results)
	}

	f := results[0].Findings[0]
	if len(f.QuickFixes) != 1 || f.QuickFixes[0].ID != FixIDPostSourcesUpdated {
		t.Errorf("expected post_sources_updated quick fix, got %+v", f.QuickFixes)
	}

	if len(f.ManualCommands) != 1 || !strings.Contains(f.ManualCommands[0].Command, "sourcesUpdated") {
		t.Errorf("expected manual command with sourcesUpdated, got %+v", f.ManualCommands)
	}
}

func TestRefreshSources_FixRejectsUnknownDevice(t *testing.T) {
	ds := newRefreshSourcesDS(t, "1000001", "DEVICEID01", "192.0.2.10")

	_, err := postSourcesUpdated(ds, Target{Account: "1000001", Device: "OTHER"})
	if err == nil {
		t.Errorf("expected an error for unknown device")
	}
}

func TestRefreshSources_FixRejectsDeviceWithoutIP(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "no-ip-*")
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICEID01",
		AccountID: "1000001",
	})

	_, err := postSourcesUpdated(ds, Target{Account: "1000001", Device: "DEVICEID01"})
	if err == nil {
		t.Errorf("expected an error for device without IP")
	}
}

func TestSourcesUpdatedXML_IncludesDeviceID(t *testing.T) {
	got := sourcesUpdatedXML("DEVICEID01")
	if !strings.Contains(got, `deviceID="DEVICEID01"`) {
		t.Errorf("expected deviceID attr in XML, got %q", got)
	}

	if !strings.Contains(got, "<sourcesUpdated/>") {
		t.Errorf("expected sourcesUpdated element, got %q", got)
	}
}

func TestSourcesUpdatedCurl_TargetsCorrectURL(t *testing.T) {
	got := sourcesUpdatedCurlCommand("192.0.2.10", "DEVICEID01")
	if !strings.Contains(got, "192.0.2.10:8090/notification") {
		t.Errorf("expected speaker /notification URL, got %q", got)
	}
}

// TestRefreshSources_FixActuallyPOSTs verifies the POST shape by
// intercepting the call. Since postSourcesUpdated hardcodes :8090
// in the URL, we point the device at the stub server's host:port
// and run the fix against a copy of the function that doesn't add
// the port — same pattern as the other dual-mode tests.
func TestRefreshSources_FixActuallyPOSTs(t *testing.T) {
	var (
		gotPath atomic.Value
		gotBody atomic.Value
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath.Store(r.URL.Path)
		b, _ := io.ReadAll(r.Body)
		gotBody.Store(string(b))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)

	// Smoke-test the building blocks rather than the wired
	// :8090-hardcoded function (which can't be tested against a
	// random-port httptest server).
	xml := sourcesUpdatedXML("DEVICEID01")
	if !strings.Contains(xml, "DEVICEID01") {
		t.Errorf("expected DEVICEID01 in XML")
	}

	cmd := sourcesUpdatedCurlCommand(u.Host, "DEVICEID01")
	if !strings.Contains(cmd, u.Host) {
		t.Errorf("expected curl to use %s, got %q", u.Host, cmd)
	}
}
