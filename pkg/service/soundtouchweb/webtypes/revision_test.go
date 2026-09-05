package webtypes

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestNowPlayingRejectsPollOlderThanEvent is the core ordering property the
// player's source confirmation relies on: a /now_playing poll that started
// before a nowPlayingUpdated event must not overwrite the event's result when
// it finishes late.
func TestNowPlayingRejectsPollOlderThanEvent(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	pollGeneration := conn.BeginFieldPoll(FieldNowPlaying)
	eventNowPlaying := &models.NowPlaying{Track: "new event"}

	conn.ApplyFieldEvent(FieldNowPlaying, func(status *DeviceStatus) {
		status.NowPlaying = eventNowPlaying
	})

	if conn.CompleteFieldPoll(FieldNowPlaying, pollGeneration, func(status *DeviceStatus) {
		status.NowPlaying = &models.NowPlaying{Track: "old poll"}
	}) {
		t.Fatal("older NowPlaying poll was accepted after the event")
	}

	if got := conn.Status().NowPlaying; got != eventNowPlaying {
		t.Fatalf("older poll replaced NowPlaying event: %+v", got)
	}
}

func TestDeviceStatusRevisionAdvancesForEveryProjection(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	if got := conn.Status().Revision; got != 0 {
		t.Fatalf("initial revision = %d, want 0", got)
	}

	conn.UpdateStatus(func(*DeviceStatus) {})
	if got := conn.Status().Revision; got != 1 {
		t.Fatalf("revision after UpdateStatus = %d, want 1", got)
	}

	// A caller-supplied Revision is never trusted: resetting it would make a
	// browser holding a higher revision reject every later update.
	conn.SetStatus(&DeviceStatus{Revision: 99})
	if got := conn.Status().Revision; got != 2 {
		t.Fatalf("revision after SetStatus = %d, want 2", got)
	}
}

func TestUnrelatedProjectionDoesNotAdvanceNowPlayingRevision(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	conn.SetStatus(&DeviceStatus{NowPlaying: &models.NowPlaying{Source: "SPOTIFY"}})
	baseline := conn.Status()

	volumeGeneration := conn.BeginFieldPoll(FieldVolume)
	conn.CompleteFieldPoll(FieldVolume, volumeGeneration, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 35}
	})
	updated := conn.Status()

	if updated.Revision <= baseline.Revision {
		t.Fatalf("aggregate revision = %d, want newer than %d", updated.Revision, baseline.Revision)
	}
	if updated.NowPlayingRevision != baseline.NowPlayingRevision {
		t.Fatalf("now-playing revision = %d, want unchanged %d after volume update",
			updated.NowPlayingRevision, baseline.NowPlayingRevision)
	}
}

func TestNowPlayingRevisionAdvancesForPollAndEvent(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	baseline := conn.Status().NowPlayingRevision

	generation := conn.BeginFieldPoll(FieldNowPlaying)
	conn.CompleteFieldPoll(FieldNowPlaying, generation, func(status *DeviceStatus) {
		status.NowPlaying = &models.NowPlaying{Source: "AUX"}
	})

	polled := conn.Status().NowPlayingRevision
	if polled <= baseline {
		t.Fatalf("now-playing revision after poll = %d, want newer than %d", polled, baseline)
	}

	conn.ApplyFieldEvent(FieldNowPlaying, func(status *DeviceStatus) {
		status.NowPlaying = &models.NowPlaying{Source: "SPOTIFY"}
	})

	if got := conn.Status().NowPlayingRevision; got <= polled {
		t.Fatalf("now-playing revision after event = %d, want newer than %d", got, polled)
	}
}

func TestDeviceStatusRevisionIsMonotonicWithConcurrentProjections(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	const projections = 64

	var wg sync.WaitGroup
	wg.Add(projections)
	for i := range projections {
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				conn.UpdateStatus(func(*DeviceStatus) {})

				return
			}
			conn.SetStatus(&DeviceStatus{})
		}()
	}
	wg.Wait()

	if got := conn.Status().Revision; got != projections {
		t.Fatalf("final revision = %d, want %d", got, projections)
	}
}

func TestDeviceStatusJSONExposesPublicRevisionsOnly(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	conn.ApplyFieldEvent(FieldNowPlaying, func(status *DeviceStatus) {
		status.NowPlaying = &models.NowPlaying{Track: "test"}
	})

	encoded, err := json.Marshal(conn.Status())
	if err != nil {
		t.Fatalf("marshal DeviceStatus: %v", err)
	}

	jsonStatus := string(encoded)
	if !strings.Contains(jsonStatus, `"revision":1`) {
		t.Fatalf("public revision missing from JSON: %s", jsonStatus)
	}
	if !strings.Contains(jsonStatus, `"nowPlayingRevision":1`) {
		t.Fatalf("now-playing revision missing from JSON: %s", jsonStatus)
	}
	if strings.Contains(jsonStatus, "fieldGen") {
		t.Fatalf("internal field generation leaked into JSON: %s", jsonStatus)
	}
}
