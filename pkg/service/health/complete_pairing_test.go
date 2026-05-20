package health

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestEmptyMargeAccountFinding_CarriesCompletePairingQuickFix verifies
// the finding for the GH-329 case (speaker /info has empty
// margeAccountUUID) carries the Complete-pairing QuickFix plus the
// CLI ManualCommand fallback. The executor itself lives in
// handlers/server.go (where setup.Manager is available); this test
// just pins the finding shape.
func TestEmptyMargeAccountFinding_CarriesCompletePairingQuickFix(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEVICEID01">
    <name>SoundTouch 20</name>
    <margeAccountUUID></margeAccountUUID>
    <margeURL>http://aftertouch.local:8000</margeURL>
</info>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)

	tempDir, _ := os.MkdirTemp("", "complete-pairing-empty-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	got := probeAndAssessSpeakerWithURL(ds, "default", "DEVICEID01", u.Host, "http://"+u.Host+"/info")
	if len(got) != 1 {
		t.Fatalf("expected one finding for empty margeAccountUUID, got %d: %+v", len(got), got)
	}

	if got[0].Severity != SeverityWarning {
		t.Errorf("expected SeverityWarning, got %v", got[0].Severity)
	}

	if len(got[0].QuickFixes) != 1 || got[0].QuickFixes[0].ID != FixIDCompleteSpeakerPairing {
		t.Errorf("expected QuickFix with ID=%s; got %+v", FixIDCompleteSpeakerPairing, got[0].QuickFixes)
	}

	if !strings.Contains(got[0].QuickFixes[0].Confirm, "fresh 7-digit account") {
		t.Errorf("expected Confirm to mention fresh-account fallback when no on-disk account exists; got %q", got[0].QuickFixes[0].Confirm)
	}

	if len(got[0].ManualCommands) != 1 || !strings.Contains(got[0].ManualCommands[0].Command, "setup pair") {
		t.Errorf("expected ManualCommand with CLI setup pair invocation; got %+v", got[0].ManualCommands)
	}

	if !strings.Contains(got[0].ManualCommands[0].Command, "--mode=bare") {
		t.Errorf("expected ManualCommand to use --mode=bare path; got %q", got[0].ManualCommands[0].Command)
	}
}

// TestEmptyMargeAccountFinding_SuggestsExistingAccountWhenOnDisk pins
// the on-disk-account suggestion: when an account directory matching
// the 7-digit shape already contains this device (typical state when
// the speaker was partially paired earlier but forgot its binding),
// the QuickFix target carries that account and the Confirm copy
// names it explicitly.
func TestEmptyMargeAccountFinding_SuggestsExistingAccountWhenOnDisk(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="DEVICEID01">
    <margeAccountUUID></margeAccountUUID>
    <margeURL>http://aftertouch.local:8000</margeURL>
</info>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)

	tempDir, _ := os.MkdirTemp("", "complete-pairing-suggest-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	if err := os.MkdirAll(filepath.Join(tempDir, "accounts", "1234567", "devices", "DEVICEID01"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	got := probeAndAssessSpeakerWithURL(ds, "default", "DEVICEID01", u.Host, "http://"+u.Host+"/info")
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %d: %+v", len(got), got)
	}

	if got[0].Target.Account != "1234567" {
		t.Errorf("expected Target.Account to be the on-disk suggestion 1234567, got %q", got[0].Target.Account)
	}

	if !strings.Contains(got[0].QuickFixes[0].Confirm, "1234567") {
		t.Errorf("expected Confirm to name the on-disk account 1234567, got %q", got[0].QuickFixes[0].Confirm)
	}

	if !strings.Contains(got[0].ManualCommands[0].Command, "--account=1234567") {
		t.Errorf("expected ManualCommand to pre-fill the on-disk account, got %q", got[0].ManualCommands[0].Command)
	}
}

// TestIsSevenDigitAccountID pins the local validator we use in
// suggestAccountForPairing to avoid importing setup.
func TestIsSevenDigitAccountID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1234567", true},
		{"0000001", true},
		{"9999999", true},
		{"123456", false},   // 6 digits
		{"12345678", false}, // 8 digits
		{"default", false},
		{"123456a", false},
		{"", false},
	}

	for _, tc := range cases {
		if got := isSevenDigitAccountID(tc.in); got != tc.want {
			t.Errorf("isSevenDigitAccountID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
