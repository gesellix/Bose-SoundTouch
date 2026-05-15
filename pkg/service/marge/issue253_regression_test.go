package marge

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestIssue253_PresetsXMLEditPropagatesToMargeResponse documents the
// service-side half of issue #253:
//
//	https://github.com/gesellix/Bose-SoundTouch/issues/253
//
// The reporter edits AfterTouch's persisted Presets.xml on disk and
// expects the change to show up on the speaker's :8090/presets. That
// propagation chain has three links:
//
//  1. disk → marge: AfterTouch's marge serves the edited XML when the
//     speaker GETs /streaming/account/.../device/.../presets (or
//     /full). This is the link this test exercises.
//  2. marge → device: the speaker has to re-fetch (typically nudged by
//     a /streaming/support/power_on or by a sourcesUpdated
//     notification — not exercised here, that's a runbook concern).
//  3. device → :8090: once the device's local cache updates, its
//     /presets endpoint reflects. Out of our reach.
//
// If link (1) is broken — e.g. marge caches the rendered XML between
// requests, or ds.GetPresets returns stale data — neither (2) nor (3)
// can recover, and the reporter's symptom is inevitable. This test
// proves (1) is sound by:
//
//   - Writing testdata/issue253/presets_v1.xml directly into the
//     datastore (no SavePresets — the reporter is editing on disk).
//   - Calling PresetsToXML, asserting v1's itemName and location land
//     in the rendered response.
//   - Overwriting the file with testdata/issue253/presets_v2.xml.
//   - Calling PresetsToXML again, asserting v2's itemName and
//     location land — and v1's are gone.
//
// If link (1) ever regresses (a caching layer added without
// invalidation, a fs handle held open across edits, …), this test
// fails on the second assertion. When that happens, fix the
// invalidation rather than weakening the test.
//
// Pattern mirrors recents_sourceproviderid_regression_test.go: write
// XML directly to the datastore filesystem, exercise the marge
// function the handler uses (PresetsToXML at marge.go:370), assert on
// the rendered bytes.
func TestIssue253_PresetsXMLEditPropagatesToMargeResponse(t *testing.T) {
	v1, err := os.ReadFile(filepath.Join("testdata", "issue253", "presets_v1.xml"))
	if err != nil {
		t.Fatalf("read v1 fixture: %v", err)
	}

	v2, err := os.ReadFile(filepath.Join("testdata", "issue253", "presets_v2.xml"))
	if err != nil {
		t.Fatalf("read v2 fixture: %v", err)
	}

	// Fixture sanity — a typo in testdata would silently invalidate
	// the assertions below.
	if !strings.Contains(string(v1), "Initial Station") ||
		!strings.Contains(string(v1), "sINITIAL") {
		t.Fatalf("v1 fixture missing expected markers; got:\n%s", v1)
	}

	if !strings.Contains(string(v2), "Edited Station") ||
		!strings.Contains(string(v2), "sEDITED") {
		t.Fatalf("v2 fixture missing expected markers; got:\n%s", v2)
	}

	tempDir, err := os.MkdirTemp("", "issue253-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	const (
		account  = "issue253"
		deviceID = "DEADBEEFCAFE"
	)

	deviceDir := filepath.Join(tempDir, "accounts", account, "devices", deviceID)
	if err := os.MkdirAll(deviceDir, 0o755); err != nil {
		t.Fatalf("mkdir device dir: %v", err)
	}

	presetsPath := filepath.Join(deviceDir, "Presets.xml")

	ds := datastore.NewDataStore(tempDir)

	// First render: write v1 to disk, ask marge for the wire bytes.
	if err := os.WriteFile(presetsPath, v1, 0o644); err != nil {
		t.Fatalf("write v1: %v", err)
	}

	render1, err := PresetsToXML(ds, account, deviceID)
	if err != nil {
		t.Fatalf("PresetsToXML (v1): %v", err)
	}

	if !strings.Contains(string(render1), "Initial Station") {
		t.Errorf("v1 render missing 'Initial Station'; body:\n%s", render1)
	}

	if !strings.Contains(string(render1), "/v1/playback/station/sINITIAL") {
		t.Errorf("v1 render missing initial location; body:\n%s", render1)
	}

	// Second render after on-disk edit: must reflect v2, not v1.
	if err := os.WriteFile(presetsPath, v2, 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}

	render2, err := PresetsToXML(ds, account, deviceID)
	if err != nil {
		t.Fatalf("PresetsToXML (v2): %v", err)
	}

	if !strings.Contains(string(render2), "Edited Station") {
		t.Errorf("v2 render missing 'Edited Station' — disk edit did not propagate. Likely a caching layer added between requests; render body:\n%s", render2)
	}

	if !strings.Contains(string(render2), "/v1/playback/station/sEDITED") {
		t.Errorf("v2 render missing edited location; body:\n%s", render2)
	}

	if strings.Contains(string(render2), "Initial Station") ||
		strings.Contains(string(render2), "sINITIAL") {
		t.Errorf("v2 render still carries v1 content — propagation broken. Body:\n%s", render2)
	}
}
