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

func newSpeakerInfoTestDatastore(t *testing.T, account, device, ipAddress string) *datastore.DataStore {
	t.Helper()

	tempDir, err := os.MkdirTemp("", "speaker-info-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}

	t.Cleanup(func() { os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)

	if err := ds.SaveDeviceInfo(account, device, &models.ServiceDeviceInfo{
		DeviceID:  device,
		AccountID: account,
		Name:      "TestSpeaker",
		IPAddress: ipAddress,
	}); err != nil {
		t.Fatalf("SaveDeviceInfo: %v", err)
	}

	return ds
}

// stubSpeakerServer returns an httptest server that mimics
// :8090/info, plus the host:port the speaker is "reachable" at
// (for plugging into the device record).
func stubSpeakerServer(t *testing.T, body string) (*httptest.Server, string) {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))

	t.Cleanup(srv.Close)

	u, _ := url.Parse(srv.URL)

	return srv, u.Host
}

func TestSpeakerInfoReachable_FlagsUnreachable(t *testing.T) {
	// Point the device at a refused port; probe will fail.
	ds := newSpeakerInfoTestDatastore(t, "1000001", "DEVICEID01", "127.0.0.1:1")

	r := NewRegistry()
	RegisterSpeakerInfoReachable(r, ds)

	results := r.RunAll()
	if len(results) != 1 || results[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning result, got %+v", results)
	}

	finding := results[0].Findings[0]
	if !strings.Contains(finding.Message, "not reachable") {
		t.Errorf("unexpected message: %q", finding.Message)
	}

	if len(finding.ManualCommands) != 1 {
		t.Fatalf("expected one ManualCommand, got %d", len(finding.ManualCommands))
	}

	if !strings.Contains(finding.ManualCommands[0].Command, "127.0.0.1:1") {
		t.Errorf("manual command should target the speaker URL, got %q", finding.ManualCommands[0].Command)
	}
}

func TestSpeakerInfoReachable_FlagsEmptyMargeAccountUUID(t *testing.T) {
	// /info responds 200 with margeAccountUUID empty.
	body := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEVICEID01">
  <name>TestSpeaker</name>
  <type>SoundTouch 20</type>
  <margeAccountUUID></margeAccountUUID>
  <margeURL>http://example/</margeURL>
</info>`

	_, hostport := stubSpeakerServer(t, body)
	ds := newSpeakerInfoTestDatastore(t, "1000001", "DEVICEID01", hostport)

	// Because the stub serves on a random port and the production
	// check unconditionally appends ":8090", we override the IP
	// to include the actual port. ListAllDevices returns whatever
	// we wrote.
	r := NewRegistry()
	r.Register(Check{
		ID:    CheckIDSpeakerInfoReachable,
		Title: "Speakers respond on :8090/info",
		Run: func() []Finding {
			return probeAndAssessSpeaker(ds, "1000001", "DEVICEID01", hostport+"_skip_port_append")
		},
	})

	// Probe directly with the actual hostport to bypass the
	// ":8090" formatting in production code (since httptest
	// servers can't bind to 8090 in tests).
	got := probeAndAssessSpeakerWithURL(nil, "1000001", "DEVICEID01", hostport, "http://"+hostport+"/info")

	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "margeAccountUUID") {
		t.Errorf("unexpected message: %q", got[0].Message)
	}

	_ = ds
}

func TestSpeakerInfoReachable_NoFindingsWhenHealthy(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEVICEID01">
  <name>TestSpeaker</name>
  <margeAccountUUID>1000001</margeAccountUUID>
  <margeURL>http://example/</margeURL>
</info>`

	_, hostport := stubSpeakerServer(t, body)

	got := probeAndAssessSpeakerWithURL(nil, "1000001", "DEVICEID01", hostport, "http://"+hostport+"/info")
	if len(got) != 0 {
		t.Errorf("expected no findings, got %+v", got)
	}
}

func TestSpeakerInfoReachable_FlagsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	got := probeAndAssessSpeakerWithURL(nil, "1000001", "DEVICEID01", u.Host, "http://"+u.Host+"/info")

	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning for HTTP 500, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "HTTP 500") {
		t.Errorf("expected HTTP 500 in message, got %q", got[0].Message)
	}
}

func TestSpeakerInfoReachable_FlagsMalformedXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not xml at all"))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	got := probeAndAssessSpeakerWithURL(nil, "1000001", "DEVICEID01", u.Host, "http://"+u.Host+"/info")

	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning for malformed XML, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "not valid XML") {
		t.Errorf("unexpected message: %q", got[0].Message)
	}
}
