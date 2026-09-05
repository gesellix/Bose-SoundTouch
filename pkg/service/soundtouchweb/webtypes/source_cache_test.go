package webtypes

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestSourcesFailureFencesOlderOverlappingSuccess is the ordering property
// that makes the stale marker trustworthy: a /sources read that FAILED must
// not be undone by an older, still-in-flight read that happens to succeed
// after it. Both go through CompleteFieldPoll(FieldSources, ...), so the
// failure's newer generation wins.
func TestSourcesFailureFencesOlderOverlappingSuccess(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	retained := &models.Sources{SourceItem: []models.SourceItem{{Source: "PRODUCT"}}}

	firstGeneration := conn.BeginFieldPoll(FieldSources)
	secondGeneration := conn.BeginFieldPoll(FieldSources)

	conn.CompleteFieldPoll(FieldSources, secondGeneration, func(status *DeviceStatus) {
		status.Sources = retained
		status.SourcesStale = true
	})

	older := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}
	if conn.CompleteFieldPoll(FieldSources, firstGeneration, func(status *DeviceStatus) {
		status.Sources = older
		status.SourcesStale = false
	}) {
		t.Fatal("older successful source poll was accepted after the newer failure")
	}

	status := conn.Status()
	if status.Sources != retained || !status.SourcesStale {
		t.Fatalf("older success cleared the newer source failure: %+v", status)
	}
}

// TestSourcesStaleSurvivesAnUnrelatedFieldMerge guards the reason staleness is
// stored rather than derived per read: an unrelated field's merge copies the
// status, and must carry the marker along.
func TestSourcesStaleSurvivesAnUnrelatedFieldMerge(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	conn.CompleteFieldPoll(FieldSources, conn.BeginFieldPoll(FieldSources), func(status *DeviceStatus) {
		status.SourcesStale = true
	})

	conn.CompleteFieldPoll(FieldVolume, conn.BeginFieldPoll(FieldVolume), func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 35}
	})

	if !conn.Status().SourcesStale {
		t.Fatal("an unrelated field merge cleared the source stale marker")
	}
}

// TestSourcesFailureWithoutInventoryIsExplicit covers the first-poll case: the
// player has no inventory at all AND cannot trust one, and the browser has to
// see that in the JSON.
func TestSourcesFailureWithoutInventoryIsExplicit(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	conn.CompleteFieldPoll(FieldSources, conn.BeginFieldPoll(FieldSources), func(status *DeviceStatus) {
		status.SourcesStale = true
	})

	status := conn.Status()
	if status.Sources != nil || !status.SourcesStale {
		t.Fatalf("initial source failure was not represented without inventory: %+v", status)
	}

	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal initial source failure: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, `"sourcesStale":true`) {
		t.Fatalf("initial source failure omitted stale state: %s", got)
	}
}

// TestSourcesSuccessClearsStale is the recovery half: once a read succeeds the
// inventory is actionable again, with no TTL to wait out.
func TestSourcesSuccessClearsStale(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	conn.CompleteFieldPoll(FieldSources, conn.BeginFieldPoll(FieldSources), func(status *DeviceStatus) {
		status.SourcesStale = true
	})

	fresh := &models.Sources{SourceItem: []models.SourceItem{{Source: "BLUETOOTH"}}}
	conn.CompleteFieldPoll(FieldSources, conn.BeginFieldPoll(FieldSources), func(status *DeviceStatus) {
		status.Sources = fresh
		status.SourcesStale = false
	})

	status := conn.Status()
	if status.SourcesStale || status.Sources != fresh {
		t.Fatalf("successful source readback did not restore actionability: %+v", status)
	}

	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal refreshed sources: %v", err)
	}
	if got := string(encoded); strings.Contains(got, "sourcesStale") {
		t.Fatalf("cleared stale marker should be omitted from JSON: %s", got)
	}
}
