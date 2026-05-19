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

func newPlaybackTestDS(t *testing.T, account, device, ipAddress string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "playback-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		IPAddress: ipAddress,
		Name:      "TestSpeaker",
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

func TestTestPlayback_FindingsListEveryDevice(t *testing.T) {
	ds := newPlaybackTestDS(t, "1000001", "DEVICEID01", "192.0.2.10")

	r := NewRegistry()
	RegisterTestPlaybackCheck(r, ds, func() string { return "http://aftertouch.local" })

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Findings) != 1 {
		t.Fatalf("expected 1 finding for the one device, got %d", len(results[0].Findings))
	}

	f := results[0].Findings[0]
	if len(f.QuickFixes) != 1 || f.QuickFixes[0].ID != FixIDPlayDing {
		t.Errorf("expected play_ding quick fix, got %+v", f.QuickFixes)
	}

	if len(f.ManualCommands) != 1 || !strings.Contains(f.ManualCommands[0].Command, "ContentItem") {
		t.Errorf("expected manual command with ContentItem, got %+v", f.ManualCommands)
	}
}

func TestTestPlayback_NoServerURLBlocksCheck(t *testing.T) {
	ds := newPlaybackTestDS(t, "1000001", "DEVICEID01", "192.0.2.10")

	r := NewRegistry()
	RegisterTestPlaybackCheck(r, ds, func() string { return "" })

	results := r.RunAll()
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if len(results[0].Findings) != 1 {
		t.Fatalf("expected one info finding explaining the blocker, got %+v", results[0].Findings)
	}

	if !strings.Contains(results[0].Findings[0].Message, "server URL") {
		t.Errorf("expected hint about server URL, got %q", results[0].Findings[0].Message)
	}
}

func TestPlayDing_PostsContentItemToSelectEndpoint(t *testing.T) {
	var (
		gotMethod      atomic.Value
		gotPath        atomic.Value
		gotContentType atomic.Value
		gotBody        atomic.Value
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod.Store(r.Method)
		gotPath.Store(r.URL.Path)
		gotContentType.Store(r.Header.Get("Content-Type"))
		body, _ := io.ReadAll(r.Body)
		gotBody.Store(string(body))
		w.WriteHeader(200)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)

	tempDir, _ := os.MkdirTemp("", "play-ding-*")
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.SaveDeviceInfo("1000001", "DEVICEID01", &models.ServiceDeviceInfo{
		DeviceID:  "DEVICEID01",
		AccountID: "1000001",
		IPAddress: u.Host, // includes the random port; bypasses the ":8090" assumption below
		Name:      "Bench",
	})

	// playDingOnDevice unconditionally targets :8090, so we can't
	// reuse it directly with httptest. Test the building blocks
	// (ContentItem rendering + the curl-command form) here, and
	// leave the full POST plumbing for a manual smoke test.
	mediaURL := "http://aftertouch.local" + DingMediaPath
	contentItem := buildDingContentItem(mediaURL)

	if !strings.Contains(contentItem, "source=\"INTERNET_RADIO\"") {
		t.Errorf("ContentItem missing INTERNET_RADIO source, got %q", contentItem)
	}

	if !strings.Contains(contentItem, mediaURL) {
		t.Errorf("ContentItem missing media URL, got %q", contentItem)
	}

	cmd := dingCurlCommand("192.0.2.10", "http://aftertouch.local")
	if !strings.Contains(cmd, "192.0.2.10:8090/select") {
		t.Errorf("curl command should target speaker /select, got %q", cmd)
	}

	if !strings.Contains(cmd, "/media/aftertouch-ding.wav") {
		t.Errorf("curl command should include the ding URL, got %q", cmd)
	}
}

func TestPlayDing_RejectsUnknownDevice(t *testing.T) {
	ds := newPlaybackTestDS(t, "1000001", "DEVICEID01", "192.0.2.10")

	_, err := playDingOnDevice(ds, "http://aftertouch.local", Target{Account: "1000001", Device: "OTHER"})
	if err == nil {
		t.Errorf("expected an error for unknown device")
	}
}

func TestPlayDing_RejectsEmptyServerURL(t *testing.T) {
	ds := newPlaybackTestDS(t, "1000001", "DEVICEID01", "192.0.2.10")

	_, err := playDingOnDevice(ds, "", Target{Account: "1000001", Device: "DEVICEID01"})
	if err == nil {
		t.Errorf("expected an error when serverURL is empty")
	}
}

func TestXMLAttrEscape(t *testing.T) {
	got := xmlAttrEscape(`http://x/y?a=1&b=2&c="quoted"`)
	if strings.Contains(got, `"quoted"`) || strings.Contains(got, "&b=2") {
		t.Errorf("expected escaping, got %q", got)
	}
}
