package webtypes

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestSourceCacheStatusAtTTLBoundary(t *testing.T) {
	readAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	sources := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}
	status := &DeviceStatus{
		Sources:       sources,
		SourcesReadAt: readAt,
	}

	fresh := sourceCacheStatusAt(status, readAt.Add(sourceCacheTTL-time.Nanosecond), sourceCacheTTL)
	if fresh.SourcesStale {
		t.Fatal("source cache became stale before its TTL elapsed")
	}

	stale := sourceCacheStatusAt(status, readAt.Add(sourceCacheTTL), sourceCacheTTL)
	if !stale.SourcesStale {
		t.Fatal("source cache was not stale at its TTL boundary")
	}
	if stale.Sources != sources {
		t.Fatal("stale projection did not retain the last successful source list")
	}

	refreshed := *stale
	refreshed.SourcesReadAt = readAt.Add(sourceCacheTTL + time.Second)
	refreshed.SourcesStale = false

	got := sourceCacheStatusAt(&refreshed, refreshed.SourcesReadAt, sourceCacheTTL)
	if got.SourcesStale {
		t.Fatal("successful source readback did not clear staleness immediately")
	}
}

func TestSourceCacheWithoutSuccessfulReadIsNotStale(t *testing.T) {
	status := &DeviceStatus{Sources: &models.Sources{}}

	got := sourceCacheStatusAt(status, time.Now().Add(time.Hour), time.Nanosecond)
	if got.SourcesStale {
		t.Fatal("source cache without a recorded successful read was marked stale")
	}
}

func TestSourceCacheFailureWithoutInventoryIsExplicit(t *testing.T) {
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

// TestSourceCacheFailureFencesOlderOverlappingSuccess: a /sources read that
// FAILED must not be undone by an older, still-in-flight read that happens to
// succeed after it. Both go through CompleteFieldPoll(FieldSources, ...), so
// the failure's newer generation wins.
func TestSourceCacheFailureFencesOlderOverlappingSuccess(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	readAt := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	retained := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}

	firstGeneration := conn.BeginFieldPoll(FieldSources)
	secondGeneration := conn.BeginFieldPoll(FieldSources)

	conn.CompleteFieldPoll(FieldSources, secondGeneration, func(status *DeviceStatus) {
		status.Sources = retained
		status.SourcesReadAt = readAt
		status.SourcesStale = true
	})

	older := &models.Sources{SourceItem: []models.SourceItem{{Source: "PRODUCT"}}}
	if conn.CompleteFieldPoll(FieldSources, firstGeneration, func(status *DeviceStatus) {
		status.Sources = older
		status.SourcesStale = false
	}) {
		t.Fatal("older success was accepted after newer failure")
	}

	if status := conn.Status(); !status.SourcesStale || status.Sources != retained {
		t.Fatalf("older success cleared newer failure: %+v", status)
	}

	newer := &models.Sources{SourceItem: []models.SourceItem{{Source: "BLUETOOTH"}}}
	newerRead := time.Now()
	conn.CompleteFieldPoll(FieldSources, conn.BeginFieldPoll(FieldSources), func(status *DeviceStatus) {
		status.Sources = newer
		status.SourcesReadAt = newerRead
		status.SourcesStale = false
	})

	if status := conn.Status(); status.SourcesStale || status.Sources != newer {
		t.Fatalf("newer source success did not restore actionability: %+v", status)
	}
}
