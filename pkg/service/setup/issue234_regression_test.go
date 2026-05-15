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

// TestIssue234_FactoryResetSpeakerSyncsReducedSources captures the
// device-side state reported in
//
//	https://github.com/gesellix/Bose-SoundTouch/issues/234
//
// After a factory reset the SoundTouch's `/sources` only lists the
// always-on local sources (AUX, BLUETOOTH, AIRPLAY, NOTIFICATION,
// QPLAY) plus a placeholder SPOTIFY entry for the Spotify Connect
// fallback. TUNEIN, LOCAL_INTERNET_RADIO, DEEZER, and any
// post-pairing Spotify accounts are absent. The reporter's
// workaround is a POST to `:8090/notification` with a
// `<sourcesUpdated/>` payload — that nudges the device to re-render
// its source list. Separately, `/info` reports an empty
// `<margeAccountUUID/>` because `Marge.xml` is missing in the
// persistence partition.
//
// What this test locks in (current behaviour):
//
//   - GetLiveDeviceInfo against a factory-reset speaker correctly
//     reports an empty MargeAccountUUID, so downstream code that
//     keys on "is the device paired?" (e.g. setup.go:632 sets
//     IsPaired from AccountID) gets the right answer.
//   - syncSources persists exactly the reduced list verbatim — AUX
//     and BLUETOOTH survive as `<sourceKey type="…">` entries, but
//     TUNEIN / LOCAL_INTERNET_RADIO are NOT in the persisted
//     Sources.xml.
//
// What this test would catch if it flipped:
//
//   - If AfterTouch grows auto-recovery (POST sourcesUpdated on the
//     speaker's behalf during sync, or marge-side source
//     replenishment from the catalog), the "TUNEIN absent" assertion
//     below would start failing — at which point flip it to assert
//     TUNEIN *is* present, and adjust the comment to reflect the new
//     contract.
//
// Pattern mirrors pkg/service/setup/issue218_regression_test.go.
func TestIssue234_FactoryResetSpeakerSyncsReducedSources(t *testing.T) {
	infoXML, err := os.ReadFile(filepath.Join("testdata", "issue234", "info.xml"))
	if err != nil {
		t.Fatalf("read issue234 info fixture: %v", err)
	}

	sourcesXML, err := os.ReadFile(filepath.Join("testdata", "issue234", "sources.xml"))
	if err != nil {
		t.Fatalf("read issue234 sources fixture: %v", err)
	}

	// Sanity-check the fixtures before relying on the round-trip:
	// a typo in testdata would silently invalidate the assertions.
	if !strings.Contains(string(infoXML), "<margeAccountUUID></margeAccountUUID>") {
		t.Fatalf("issue234 info fixture must carry an empty <margeAccountUUID> to model a factory-reset device; got:\n%s", infoXML)
	}

	if strings.Contains(string(sourcesXML), `source="TUNEIN"`) ||
		strings.Contains(string(sourcesXML), `source="LOCAL_INTERNET_RADIO"`) {
		t.Fatalf("issue234 sources fixture must NOT contain TUNEIN or LOCAL_INTERNET_RADIO — they're the symptom we're modelling; got:\n%s", sourcesXML)
	}

	s, err := fakespeaker.Start(fakespeaker.Config{
		FixtureOverrides: map[string][]byte{
			"/info":    infoXML,
			"/sources": sourcesXML,
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

	tempDir, err := os.MkdirTemp("", "issue234-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	ds := datastore.NewDataStore(tempDir)
	m := NewManager("http://localhost:8080", ds, nil)

	deviceIP := s.HTTPAddr() // "127.0.0.1:<random>" — host:port form routes via the bare-URL branch in syncSources

	// 1. Factory-reset detection: /info reports no margeAccountUUID,
	// so downstream code can refuse to claim "paired" status.
	info, err := m.GetLiveDeviceInfo(deviceIP)
	if err != nil {
		t.Fatalf("GetLiveDeviceInfo: %v", err)
	}

	if info.MargeAccountUUID != "" {
		t.Errorf("MargeAccountUUID = %q, want empty (factory-reset speaker has no account yet)", info.MargeAccountUUID)
	}

	if info.DeviceID != "DEADBEEFCAFE" {
		t.Errorf("DeviceID = %q, want %q", info.DeviceID, "DEADBEEFCAFE")
	}

	// 2. End-to-end sync. Driving SyncDeviceData rather than
	// syncSources directly exercises the wiring between
	// syncSources and notifySpeakerSourcesUpdated — the source
	// list still lands on disk (assertions below) AND the
	// sourcesUpdated notification fires against the device.
	// SyncDeviceData derives accountID/deviceID from /info; with
	// an empty margeAccountUUID the account falls through to
	// "default".
	if err := m.SyncDeviceData(deviceIP); err != nil {
		t.Fatalf("SyncDeviceData: %v", err)
	}

	const (
		accountID = "default"
		deviceID  = "DEADBEEFCAFE"
	)

	sourcesPath := filepath.Join(tempDir, "accounts", accountID, "devices", deviceID, "Sources.xml")

	persisted, err := os.ReadFile(sourcesPath)
	if err != nil {
		t.Fatalf("read persisted sources at %s: %v", sourcesPath, err)
	}

	content := string(persisted)

	// Survivors: the local-only sources reported by the factory-reset
	// device should land in the persisted file.
	for _, sourceKey := range []string{
		`<sourceKey type="AUX"`,
		`<sourceKey type="BLUETOOTH"`,
		`<sourceKey type="AIRPLAY"`,
	} {
		if !strings.Contains(content, sourceKey) {
			t.Errorf("persisted Sources.xml missing %s; body:\n%s", sourceKey, content)
		}
	}

	// Casualties: TUNEIN / LOCAL_INTERNET_RADIO are the symptom of
	// #234 — they should remain absent on the persisted side
	// because fakespeaker is stateless (the next /sources read
	// returns the same reduced fixture even after the
	// notification). On a real speaker the device would react to
	// the notification, re-expose the missing sources, and the
	// next Data Sync would persist them — that second-sync step
	// is the runbook user-facing flow, not something we model
	// here.
	for _, missingKey := range []string{
		`<sourceKey type="TUNEIN"`,
		`<sourceKey type="LOCAL_INTERNET_RADIO"`,
	} {
		if strings.Contains(content, missingKey) {
			t.Errorf("persisted Sources.xml unexpectedly contains %s — fixture changed?;\nbody:\n%s",
				missingKey, content)
		}
	}

	// 3. The sourcesUpdated notification must have fired against
	// the device with the right deviceID and shape, regardless of
	// what the fakespeaker decided to do with it.
	notifs := s.Notifications()
	if len(notifs) != 1 {
		t.Fatalf("Notifications() returned %d entries, want exactly 1 sourcesUpdated POST", len(notifs))
	}

	// The exact serialization (self-closing vs long-form
	// <sourcesUpdated></sourcesUpdated>) is up to encoding/xml and
	// not protocol-meaningful — assert on the load-bearing pieces
	// instead of the byte-identical body.
	body := string(notifs[0].Body)
	if !strings.Contains(body, `deviceID="DEADBEEFCAFE"`) {
		t.Errorf("notification body missing deviceID; got: %q", body)
	}

	if !strings.Contains(body, "sourcesUpdated") {
		t.Errorf("notification body missing sourcesUpdated; got: %q", body)
	}

	if !strings.Contains(notifs[0].ContentType, "xml") {
		t.Errorf("notification Content-Type = %q, want something xml-shaped", notifs[0].ContentType)
	}
}
