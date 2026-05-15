package setup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/testing/fakespeaker"
)

// TestIssue218_LocalInternetRadioPresetSurvivesSync drives a syncPresets
// against a fakespeaker that emits the exact LOCAL_INTERNET_RADIO preset
// XML pasted by the reporter in issue #218:
//
//	https://github.com/gesellix/Bose-SoundTouch/issues/218
//
// The preset's contentItem location points at
// `https://content.api.bose.io/core02/svc-bmx-adapter-orion/prod/orion/station?data=...`
// — a Bose cloud URL that broke when the cloud shut down. After
// migration AfterTouch must keep that URL reachable (via the DNS
// interception hook + serving the /core02/svc-bmx-adapter-orion path
// itself); the first step is verifying the URL is preserved verbatim
// through the device → datastore sync round-trip rather than getting
// rewritten or dropped.
//
// This test locks in the "location preserved verbatim" contract. When
// AfterTouch starts rewriting the URL to its own base (the eventual
// fix for #218 — see also issue #195's AUX divergence and #234's
// factory-reset preset revert which overlap with the same DNS/HTTPS
// interception story), this test will need to flip its assertion
// accordingly. The fixture stays — the assertion records the
// decision.
//
// Pattern reference: pkg/service/marge/recents_sourceproviderid_regression_test.go
// is the existing "regression test = locked-in behaviour" exemplar in
// this codebase; this is the first one to drive the fake speaker via
// fakespeaker.Config.FixtureOverrides rather than an inline
// httptest.NewServer.
func TestIssue218_LocalInternetRadioPresetSurvivesSync(t *testing.T) {
	presetsXML, err := os.ReadFile(filepath.Join("testdata", "issue218", "presets.xml"))
	if err != nil {
		t.Fatalf("read issue218 presets fixture: %v", err)
	}

	const boseCloudURL = "https://content.api.bose.io/core02/svc-bmx-adapter-orion/"

	// Sanity-check the fixture itself before trusting any assertion
	// downstream — a typo in the testdata would silently turn the
	// regression test into a no-op.
	if !strings.Contains(string(presetsXML), boseCloudURL) {
		t.Fatalf("fixture missing expected Bose cloud URL prefix %q; got:\n%s", boseCloudURL, presetsXML)
	}

	s, err := fakespeaker.Start(fakespeaker.Config{
		FixtureOverrides: map[string][]byte{
			"/presets": presetsXML,
		},
	})
	if err != nil {
		t.Fatalf("start fakespeaker: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		_ = s.Stop(ctx)
	})

	tempDir, err := os.MkdirTemp("", "issue218-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	m := NewManager("http://localhost:8080", ds, nil)

	deviceIP := s.HTTPAddr() // e.g. "127.0.0.1:54321" — syncPresets routes via host:port form

	const accountID = "issue218"

	const deviceID = "DEADBEEFCAFE"

	m.syncPresets(deviceIP, accountID, deviceID)

	persistedPath := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Presets.xml")

	persisted, err := os.ReadFile(persistedPath)
	if err != nil {
		t.Fatalf("read persisted presets at %s: %v", persistedPath, err)
	}

	// The locked-in contract: the cloud URL survives the round-trip.
	// When AfterTouch starts rewriting it (the actual fix for #218),
	// flip this assertion to assert the rewritten URL.
	if !strings.Contains(string(persisted), boseCloudURL) {
		t.Errorf("persisted Presets.xml dropped the Bose cloud URL.\nfixture URL prefix:\n  %s\npersisted body:\n%s",
			boseCloudURL, persisted)
	}

	// Round-trip should preserve preset id and source as well — basic
	// shape checks borrowed from sync_regression_test.go.
	if !strings.Contains(string(persisted), `id="1"`) {
		t.Errorf("persisted Presets.xml missing id=\"1\"; body:\n%s", persisted)
	}

	if !strings.Contains(string(persisted), `source="LOCAL_INTERNET_RADIO"`) {
		t.Errorf("persisted Presets.xml missing source=\"LOCAL_INTERNET_RADIO\"; body:\n%s", persisted)
	}
}
