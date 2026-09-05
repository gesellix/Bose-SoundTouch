package webtypes

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// TestSourcesSurviveASingleFailedRead: one failed /sources read is not
// evidence the inventory is wrong. Marking it stale immediately would disable
// every source button in the player on a transient hiccup.
func TestSourcesSurviveASingleFailedRead(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	inventory := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}

	conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), inventory, nil)
	conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), nil, errors.New("temporary read failure"))

	status := conn.Status()
	if status.SourcesStale {
		t.Fatalf("one failed read marked the inventory stale: %+v", status)
	}
	if status.Sources != inventory {
		t.Fatalf("failed read discarded the inventory: %+v", status)
	}
}

func TestSourcesGoStaleAfterConsecutiveFailedReads(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	inventory := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}
	conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), inventory, nil)

	for range staleSourcesFailureThreshold {
		conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), nil, errors.New("read failure"))
	}

	status := conn.Status()
	if !status.SourcesStale {
		t.Fatalf("inventory not marked stale after %d failed reads: %+v", staleSourcesFailureThreshold, status)
	}
	// Kept visible, just not actionable: the player still shows the list.
	if status.Sources != inventory {
		t.Fatalf("stale marking discarded the last known inventory: %+v", status)
	}
}

// TestSourcesSuccessClearsStaleAndResetsTheCount: a success is the strongest
// evidence available that the list can be acted on again.
func TestSourcesSuccessClearsStaleAndResetsTheCount(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	for range staleSourcesFailureThreshold {
		conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), nil, errors.New("read failure"))
	}
	if !conn.Status().SourcesStale {
		t.Fatal("precondition: inventory should be stale")
	}

	fresh := &models.Sources{SourceItem: []models.SourceItem{{Source: "BLUETOOTH"}}}
	conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), fresh, nil)

	if status := conn.Status(); status.SourcesStale || status.Sources != fresh {
		t.Fatalf("successful read did not restore actionability: %+v", status)
	}

	// The count reset too, so the next single failure must not re-mark stale.
	conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), nil, errors.New("read failure"))

	if status := conn.Status(); status.SourcesStale {
		t.Fatalf("failure count was not reset by the successful read: %+v", status)
	}
}

// TestSourcesFailureDoesNotDiscardConcurrentSuccess is the finding this
// design answers: a failed read carries no inventory to order, so it must not
// consume the field generation and throw away a slower, successful read.
func TestSourcesFailureDoesNotDiscardConcurrentSuccess(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)

	slowSuccess := conn.BeginFieldPoll(FieldSources)
	fastFailure := conn.BeginFieldPoll(FieldSources)

	conn.ApplySourcesRead(fastFailure, nil, errors.New("read failure"))

	inventory := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}
	if !conn.ApplySourcesRead(slowSuccess, inventory, nil) {
		t.Fatal("successful read was discarded by a concurrent failure")
	}

	if status := conn.Status(); status.SourcesStale || status.Sources != inventory {
		t.Fatalf("concurrent failure suppressed a successful read: %+v", status)
	}
}

// TestSourcesOlderSuccessCannotOverwriteNewer keeps the ordering that does
// still matter: two successful reads are ordered by generation.
func TestSourcesOlderSuccessCannotOverwriteNewer(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)

	older := conn.BeginFieldPoll(FieldSources)
	newer := conn.BeginFieldPoll(FieldSources)

	current := &models.Sources{SourceItem: []models.SourceItem{{Source: "BLUETOOTH"}}}
	conn.ApplySourcesRead(newer, current, nil)

	stale := &models.Sources{SourceItem: []models.SourceItem{{Source: "AUX"}}}
	if conn.ApplySourcesRead(older, stale, nil) {
		t.Fatal("older successful read was accepted after a newer one")
	}

	if status := conn.Status(); status.Sources != current {
		t.Fatalf("older read overwrote a newer inventory: %+v", status)
	}
}

func TestStaleSourcesWithoutInventoryIsExplicitInJSON(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	for range staleSourcesFailureThreshold {
		conn.ApplySourcesRead(conn.BeginFieldPoll(FieldSources), nil, errors.New("read failure"))
	}

	status := conn.Status()
	if status.Sources != nil || !status.SourcesStale {
		t.Fatalf("initial source failures not represented without inventory: %+v", status)
	}

	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal stale sources: %v", err)
	}
	if got := string(encoded); !strings.Contains(got, `"sourcesStale":true`) {
		t.Fatalf("stale state missing from JSON: %s", got)
	}
}
