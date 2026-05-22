package health

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func stubInfoServer(t *testing.T, margeURL string) string {
	t.Helper()

	body := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEVICEID01">
  <name>TestSpeaker</name>
  <margeAccountUUID>1000001</margeAccountUUID>
  <margeURL>` + margeURL + `</margeURL>
</info>`

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

	return "http://" + u.Host + "/info"
}

func TestMargeURL_NoFindingsWhenMatched(t *testing.T) {
	probeURL := stubInfoServer(t, "https://aftertouch.local/")
	expected := normaliseHosts([]string{"aftertouch.local", "192.0.2.10"})

	got := assessMargeURLForDeviceWithURL("1000001", "DEVICEID01", probeURL, expected)
	if len(got) != 0 {
		t.Errorf("expected no findings when host matches, got %+v", got)
	}
}

func TestMargeURL_FlagsMismatch(t *testing.T) {
	probeURL := stubInfoServer(t, "https://other-host.example/")
	expected := normaliseHosts([]string{"aftertouch.local"})

	got := assessMargeURLForDeviceWithURL("1000001", "DEVICEID01", probeURL, expected)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}

	if !strings.Contains(got[0].Message, "other-host.example") {
		t.Errorf("expected mismatched host in message, got %q", got[0].Message)
	}

	if len(got[0].ManualCommands) != 1 {
		t.Fatalf("expected a manual command, got %+v", got[0].ManualCommands)
	}

	cmd := got[0].ManualCommands[0].Command
	if !strings.Contains(cmd, "tls-extra-host=other-host.example") {
		t.Errorf("expected --tls-extra-host suggestion, got %q", cmd)
	}

	if len(got[0].QuickFixes) != 1 || got[0].QuickFixes[0].ID != FixIDAddMargeHostToTLS {
		t.Fatalf("expected QuickFix with ID=%s, got %+v", FixIDAddMargeHostToTLS, got[0].QuickFixes)
	}

	if !strings.Contains(got[0].QuickFixes[0].Label, "other-host.example") {
		t.Errorf("expected QuickFix label to name the missing host, got %q", got[0].QuickFixes[0].Label)
	}

	if got[0].QuickFixes[0].Confirm == "" {
		t.Errorf("expected QuickFix to carry a Confirm message (operator needs to know a restart is required)")
	}
}

func TestMargeURL_SkipsWhenMargeURLEmpty(t *testing.T) {
	probeURL := stubInfoServer(t, "")
	expected := normaliseHosts([]string{"aftertouch.local"})

	got := assessMargeURLForDeviceWithURL("1000001", "DEVICEID01", probeURL, expected)
	if len(got) != 0 {
		t.Errorf("expected no findings for empty margeURL, got %+v", got)
	}
}

func TestMargeURL_SkipsWhenSpeakerUnreachable(t *testing.T) {
	expected := normaliseHosts([]string{"aftertouch.local"})

	// speaker_info_reachable already covers the unreachable case,
	// so this check should stay silent.
	got := assessMargeURLForDeviceWithURL("1000001", "DEVICEID01", "http://127.0.0.1:1/info", expected)
	if len(got) != 0 {
		t.Errorf("expected no findings when speaker unreachable, got %+v", got)
	}
}

func TestHostFromURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://example.com/", "example.com"},
		{"https://Example.COM:8443/", "example.com"},
		{"http://192.0.2.10/", "192.0.2.10"},
		{"example.com", "example.com"},
		{"example.com:8443", "example.com"},
		{"", ""},
		{"://broken", ""},
	}

	for _, c := range cases {
		if got := hostFromURL(c.in); got != c.want {
			t.Errorf("hostFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormaliseHosts_DedupsAndLowercases(t *testing.T) {
	out := normaliseHosts([]string{"AFTERTOUCH.local", "aftertouch.local", "https://example.com/", "  ", ""})
	if !out["aftertouch.local"] {
		t.Errorf("expected aftertouch.local")
	}

	if !out["example.com"] {
		t.Errorf("expected example.com")
	}

	if len(out) != 2 {
		t.Errorf("expected 2 unique hosts, got %d", len(out))
	}
}
