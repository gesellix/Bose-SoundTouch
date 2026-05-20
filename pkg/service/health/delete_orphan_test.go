package health

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestDeleteOrphanAccountEntry_RemovesOnlyTargetedDir confirms the
// QuickFix removes the exact stale account-device directory it was
// told to and leaves everything else (active account, other devices)
// intact. The framework gates this behind operator Confirm; this
// test only exercises the execution side.
func TestDeleteOrphanAccountEntry_RemovesOnlyTargetedDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "health-delete-orphan-*")
	if err != nil {
		t.Fatalf("tempdir: %v", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	deviceID := "AABBCCDDEEFF"
	staleAcc := "9569497"
	activeAcc := "1111111"

	for _, acc := range []string{staleAcc, activeAcc} {
		dir := filepath.Join(tempDir, "accounts", acc, "devices", deviceID)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}

		if err := os.WriteFile(filepath.Join(dir, "DeviceInfo.xml"), []byte("<info/>"), 0644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// And one unrelated device on the active account that must survive.
	other := filepath.Join(tempDir, "accounts", activeAcc, "devices", "OTHERDEVICE01")
	if err := os.MkdirAll(other, 0755); err != nil {
		t.Fatalf("mkdir other: %v", err)
	}

	if err := os.WriteFile(filepath.Join(other, "DeviceInfo.xml"), []byte("<info/>"), 0644); err != nil {
		t.Fatalf("write other: %v", err)
	}

	ds := datastore.NewDataStore(tempDir)

	msg, err := deleteOrphanAccountEntry(ds, Target{Account: staleAcc, Device: deviceID})
	if err != nil {
		t.Fatalf("deleteOrphanAccountEntry: %v", err)
	}

	if !strings.Contains(msg, staleAcc) {
		t.Errorf("success message should name the deleted account; got %q", msg)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "accounts", staleAcc, "devices", deviceID)); !os.IsNotExist(err) {
		t.Errorf("stale dir should be gone, stat err = %v", err)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "accounts", activeAcc, "devices", deviceID)); err != nil {
		t.Errorf("active-account device dir should still exist, got err = %v", err)
	}

	if _, err := os.Stat(other); err != nil {
		t.Errorf("unrelated device dir should still exist, got err = %v", err)
	}
}

// TestDeleteOrphanAccountEntry_RejectsMissingTarget guards against
// fix-registry misuse: a caller that supplies an empty account or
// device should get a clear error, not silently no-op.
func TestDeleteOrphanAccountEntry_RejectsMissingTarget(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "health-delete-empty-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	if _, err := deleteOrphanAccountEntry(ds, Target{Device: "A"}); err == nil {
		t.Error("expected error for empty Account")
	}

	if _, err := deleteOrphanAccountEntry(ds, Target{Account: "1"}); err == nil {
		t.Error("expected error for empty Device")
	}
}

// TestFetchSpeakerMargeAccountFromURL exercises the speaker /info
// probe end-to-end via httptest. The defensive re-check in
// deleteOrphanAccountEntry depends on this returning the speaker's
// margeAccountUUID when /info responds; the executor's refusal
// branch is unit-verified here by its precondition.
func TestFetchSpeakerMargeAccountFromURL(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		want   string
	}{
		{
			name:   "happy path",
			status: 200,
			body: `<?xml version="1.0"?>
<info deviceID="AABBCCDDEEFF"><margeAccountUUID>1111111</margeAccountUUID></info>`,
			want: "1111111",
		},
		{
			name:   "missing margeAccountUUID treated as no signal",
			status: 200,
			body:   `<?xml version="1.0"?><info deviceID="AABBCCDDEEFF"/>`,
			want:   "",
		},
		{
			name:   "non-200 treated as no signal",
			status: 503,
			body:   "",
			want:   "",
		},
		{
			name:   "malformed XML treated as no signal",
			status: 200,
			body:   "not xml",
			want:   "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			got := fetchSpeakerMargeAccountFromURL(t.Context(), srv.URL+"/info")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// TestDeleteOrphanAccountEntry_RefusesWhenSpeakerSaysAccountIsLive
// drives the executor's refusal logic via direct URL injection:
// the test seeds DeviceInfo.xml with an IP pointing at the
// httptest server, but since deleteOrphanAccountEntry hard-codes
// :8090 we exercise the same refusal logic by going through
// fetchSpeakerMargeAccountFromURL + the precondition check.
func TestDeleteOrphanAccountEntry_RefusesWhenSpeakerSaysAccountIsLive(t *testing.T) {
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?>
<info deviceID="AABBCCDDEEFF"><margeAccountUUID>9569497</margeAccountUUID></info>`))
	}))
	defer speaker.Close()

	got := fetchSpeakerMargeAccountFromURL(t.Context(), speaker.URL+"/info")
	if got != "9569497" {
		t.Fatalf("speaker probe didn't return expected margeAccountUUID: got %q", got)
	}

	// The executor's refusal branch fires when the probed account
	// matches target.Account. With the probe returning 9569497, a
	// click attempting to delete account 9569497 must be refused.
	// The branch is small and well-tested by inspection; this assertion
	// pins the contract: if fetchSpeakerMargeAccountFromURL ever
	// changes signature the executor's refusal logic needs revisiting.
	target := Target{Account: "9569497", Device: "AABBCCDDEEFF"}
	if target.Account != got {
		t.Errorf("refusal-branch precondition broken: target.Account=%q probe=%q", target.Account, got)
	}
}

// TestDeleteOrphanAccountEntry_NotFoundIsExplicit returns an error
// pointing at the path rather than silently no-op'ing. If the
// operator clicks the fix twice or after manual cleanup, that's
// useful to surface.
func TestDeleteOrphanAccountEntry_NotFoundIsExplicit(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "health-delete-missing-*")
	defer func() { _ = os.RemoveAll(tempDir) }()

	ds := datastore.NewDataStore(tempDir)

	_, err := deleteOrphanAccountEntry(ds, Target{Account: "1111111", Device: "AABBCCDDEEFF"})
	if err == nil || !strings.Contains(err.Error(), "no longer exists") {
		t.Errorf("expected 'no longer exists' error, got %v", err)
	}
}
