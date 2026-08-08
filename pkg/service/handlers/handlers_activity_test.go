package handlers

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// TestIsAnnouncementDismissed_EmptyByDefault verifies a freshly-constructed
// server (no prior activity log) reports nothing as dismissed.
func TestIsAnnouncementDismissed_EmptyByDefault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dismissal-empty-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	if server.IsAnnouncementDismissed("admin-gate-notice") {
		t.Error("Expected no announcement to be dismissed on a fresh install")
	}
}

// TestRecordDismissal_UpdatesCacheAndPersists is a regression test for the
// #419 design's performance requirement: after RecordDismissal, the
// in-memory cache must reflect it immediately (no disk re-read needed), and
// it must also be durably persisted via the activity log.
func TestRecordDismissal_UpdatesCacheAndPersists(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dismissal-record-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	if err := server.RecordDismissal("admin-gate-notice"); err != nil {
		t.Fatalf("RecordDismissal failed: %v", err)
	}

	if !server.IsAnnouncementDismissed("admin-gate-notice") {
		t.Error("Expected admin-gate-notice to be dismissed after RecordDismissal")
	}

	records, err := ds.GetActivityRecords(activityKindNotificationDismissed)
	if err != nil {
		t.Fatalf("GetActivityRecords failed: %v", err)
	}
	if len(records) != 1 || records[0].ID != "admin-gate-notice" {
		t.Errorf("Expected exactly 1 persisted dismissal record, got: %+v", records)
	}
}

// TestLoadDismissedAnnouncements_ReadsPriorHistoryAtStartup verifies a
// restarted server picks up dismissals recorded in a previous run — the
// startup scan, not just the live write-through path.
func TestLoadDismissedAnnouncements_ReadsPriorHistoryAtStartup(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dismissal-startup-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	// Simulate a dismissal recorded in a prior run, before this process's
	// Server ever existed.
	if err := ds.RecordActivity(activityKindNotificationDismissed, "admin-gate-notice", nil); err != nil {
		t.Fatalf("Seeding activity record failed: %v", err)
	}

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	if !server.IsAnnouncementDismissed("admin-gate-notice") {
		t.Error("Expected startup scan to pick up a dismissal recorded in a prior run")
	}
	if server.IsAnnouncementDismissed("some-other-notice") {
		t.Error("Expected an unrelated id to not be reported as dismissed")
	}
}

// TestRecordDismissal_SameIDTwiceAppendsBothKeepsCacheSane verifies dismissing
// the same announcement twice (e.g. re-shown, dismissed again) appends two
// log entries but the in-memory cache still reports it dismissed exactly
// once (a boolean check, not a count).
func TestRecordDismissal_SameIDTwiceAppendsBothKeepsCacheSane(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dismissal-recur-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	ds := datastore.NewDataStore(tempDir)
	_ = ds.Initialize()

	server := NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)

	if err := server.RecordDismissal("admin-gate-notice"); err != nil {
		t.Fatalf("First RecordDismissal failed: %v", err)
	}
	if err := server.RecordDismissal("admin-gate-notice"); err != nil {
		t.Fatalf("Second RecordDismissal failed: %v", err)
	}

	records, err := ds.GetActivityRecords(activityKindNotificationDismissed)
	if err != nil {
		t.Fatalf("GetActivityRecords failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("Expected 2 append-only log entries for a recurring dismissal, got %d", len(records))
	}
	if !server.IsAnnouncementDismissed("admin-gate-notice") {
		t.Error("Expected admin-gate-notice to still be reported dismissed")
	}
}
