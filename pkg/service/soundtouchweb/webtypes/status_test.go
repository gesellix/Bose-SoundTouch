// Package webtypes tests for the atomic Status API on DeviceConnection
// (Status, SetStatus, UpdateStatus, NewDeviceConnection).
package webtypes

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestNewDeviceConnection_InitialStatus(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})

	status := conn.Status()
	if status == nil {
		t.Fatal("Status() returned nil from a NewDeviceConnection")
	}

	if status.IsConnected {
		t.Error("IsConnected should default to false")
	}
	if status.Connectivity != ConnectivityOffline {
		t.Errorf("Connectivity = %q, want %q", status.Connectivity, ConnectivityOffline)
	}

	if status.LastActivity.IsZero() {
		t.Error("LastActivity should be initialised, got zero time")
	}
}

func TestDeviceConnectionRejectsWebSocketAfterClose(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	conn.Close()

	ws := client.NewClientFromHost("192.0.2.10").NewWebSocketClient(nil)
	if conn.SetWebSocket(ws) {
		t.Fatal("SetWebSocket() accepted a transport after Close()")
	}
	if got := conn.CurrentWebSocket(); got != nil {
		t.Fatalf("CurrentWebSocket() = %p after Close(), want nil", got)
	}

	waitDone := make(chan struct{})
	go func() {
		ws.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("rejected WebSocket transport was not stopped")
	}
}

func TestDeviceConnectionWebSocketAccessConcurrentWithClose(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	ws := client.NewClientFromHost("192.0.2.10").NewWebSocketClient(nil)
	start := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for range 100 {
			conn.SetWebSocket(ws)
			_ = conn.CurrentWebSocket()
		}
	}()
	go func() {
		defer wg.Done()
		<-start
		conn.Close()
	}()

	close(start)
	wg.Wait()

	if got := conn.CurrentWebSocket(); got != nil {
		t.Fatalf("CurrentWebSocket() = %p after concurrent Close(), want nil", got)
	}
}

func TestDeviceConnectionClearWebSocketPreservesReplacement(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	soundTouchClient := client.NewClientFromHost("192.0.2.10")
	original := soundTouchClient.NewWebSocketClient(nil)
	replacement := soundTouchClient.NewWebSocketClient(nil)

	conn.SetWebSocket(original)
	conn.SetWebSocket(replacement)
	if conn.ClearWebSocket(original) {
		t.Fatal("ClearWebSocket() cleared a replacement transport")
	}
	if got := conn.CurrentWebSocket(); got != replacement {
		t.Fatalf("CurrentWebSocket() = %p, want replacement %p", got, replacement)
	}

	_ = original.Close()
	conn.Close()
}

func TestDeviceConnectionInfoReflectsUpdatedName(t *testing.T) {
	discovered := &models.DeviceInfo{Name: "Living Room", DeviceID: "DEVICE01"}
	conn := NewDeviceConnection(nil, discovered)

	conn.ApplyNameEvent("Living Room Left")
	info := conn.Info()

	if info == nil || info.Name != "Living Room Left" || info.DeviceID != "DEVICE01" {
		t.Fatalf("Info() = %+v, want updated name with original metadata", info)
	}
	if discovered.Name != "Living Room" {
		t.Fatalf("discovery snapshot was mutated: %+v", discovered)
	}
}

func TestNameEventSupersedesInFlightPoll(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "Initial"})
	generation := conn.BeginNameRefresh()

	conn.ApplyNameEvent("Event Name")
	if conn.ApplyPolledName(generation, "Stale Poll Name") {
		t.Fatal("stale name poll was accepted")
	}
	if got := conn.Info().Name; got != "Event Name" {
		t.Fatalf("Info().Name = %q, want newer event name", got)
	}
}

func TestConnectivityAggregatesHTTPAndEventStreamEvidence(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	started := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	conn.ObserveEventStream(true, started)
	status := conn.Status()
	if status.Connectivity != ConnectivityOnline || status.HTTPReachable ||
		!status.WebSocketConnected || !status.IsConnected {
		t.Fatalf("stream-only success = %+v", status)
	}

	failure := conn.BeginHTTPPoll()
	conn.CompleteHTTPPoll(failure, false, started.Add(30*time.Second), nil)
	status = conn.Status()
	if status.Connectivity != ConnectivityOnline || status.HTTPReachable ||
		!status.WebSocketConnected || !status.IsConnected {
		t.Fatalf("HTTP failure over live stream = %+v", status)
	}

	conn.ObserveEventStream(false, started.Add(30*time.Second))
	status = conn.Status()
	if status.Connectivity != ConnectivityStale || status.WebSocketConnected || !status.IsConnected {
		t.Fatalf("direct-path loss within grace = %+v", status)
	}

	secondFailure := conn.BeginHTTPPoll()
	conn.CompleteHTTPPoll(secondFailure, false, started.Add(60*time.Second), nil)
	status = conn.Status()
	if status.Connectivity != ConnectivityOffline || status.IsConnected {
		t.Fatalf("sustained direct-path loss = %+v", status)
	}

	recovery := conn.BeginHTTPPoll()
	conn.CompleteHTTPPoll(recovery, true, started.Add(61*time.Second), nil)
	status = conn.Status()
	if status.Connectivity != ConnectivityOnline || !status.HTTPReachable || !status.IsConnected {
		t.Fatalf("HTTP recovery = %+v", status)
	}
}

func TestOlderHTTPFailureCannotDemoteNewerStreamActivity(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	started := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	conn.MarkHTTPSuccess(started)

	older := conn.BeginHTTPPoll()
	conn.ObserveEventStream(true, started.Add(61*time.Second))
	if !conn.CompleteHTTPPoll(older, false, started.Add(62*time.Second), nil) {
		t.Fatal("latest HTTP-channel observation was unexpectedly rejected")
	}

	status := conn.Status()
	if status.Connectivity != ConnectivityOnline || status.HTTPReachable ||
		!status.WebSocketConnected || !status.IsConnected {
		t.Fatalf("older HTTP failure demoted newer stream success: %+v", status)
	}
}

func TestOlderHTTPPayloadCannotOverwriteNewerSpeakerEvent(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	started := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	poll := conn.BeginHTTPPoll()
	conn.ApplySpeakerEventAt(started.Add(time.Second), func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 99}
	})
	conn.CompleteHTTPPoll(poll, true, started.Add(2*time.Second), func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 42}
	})

	status := conn.Status()
	if status.Volume == nil || status.Volume.ActualVolume != 99 {
		t.Fatalf("speaker event was overwritten by older poll data: %+v", status.Volume)
	}
	if status.Connectivity != ConnectivityOnline || !status.HTTPReachable || !status.IsConnected {
		t.Fatalf("poll health was not retained: %+v", status)
	}
}

func TestOlderHTTPPollCannotOverwriteNewerSuccess(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	started := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	older := conn.BeginHTTPPoll()
	newer := conn.BeginHTTPPoll()
	if !conn.CompleteHTTPPoll(newer, true, started.Add(time.Second), nil) {
		t.Fatal("newer successful poll was unexpectedly rejected")
	}
	if conn.CompleteHTTPPoll(older, false, started.Add(2*time.Second), nil) {
		t.Fatal("older failed poll was unexpectedly accepted")
	}

	status := conn.Status()
	if status.Connectivity != ConnectivityOnline || !status.HTTPReachable || !status.IsConnected {
		t.Fatalf("older poll overwrote newer success: %+v", status)
	}
}

func TestOlderTransportCallbackCannotOverwriteNewerGeneration(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	started := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	if !conn.ObserveEventStreamTransport(3, true, started) {
		t.Fatal("newer connected transport generation was unexpectedly rejected")
	}
	conn.ApplySpeakerEventAt(started.Add(time.Second), func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 42}
	})
	if conn.ObserveEventStreamTransport(2, false, started.Add(2*time.Second)) {
		t.Fatal("older disconnected transport callback was unexpectedly accepted")
	}

	status := conn.Status()
	if status.Connectivity != ConnectivityOnline || !status.WebSocketConnected || !status.IsConnected {
		t.Fatalf("older transport callback demoted newer success: %+v", status)
	}
	if status.Volume == nil || status.Volume.ActualVolume != 42 {
		t.Fatalf("speaker event payload was lost: %+v", status.Volume)
	}
}

func TestInitialHTTPFailureStaysOfflineWithoutPriorSuccess(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	started := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)

	first := conn.BeginHTTPPoll()
	conn.CompleteHTTPPoll(first, false, started, nil)
	if status := conn.Status(); status.Connectivity != ConnectivityOffline || status.IsConnected {
		t.Fatalf("first initial failure = %+v", status)
	}

	second := conn.BeginHTTPPoll()
	conn.CompleteHTTPPoll(second, false, started.Add(time.Second), nil)
	if status := conn.Status(); status.Connectivity != ConnectivityOffline || status.IsConnected {
		t.Fatalf("second initial failure = %+v", status)
	}
}

func TestWebSocketLoopHasSingleOwner(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})

	if !conn.TryStartWebSocketLoop() {
		t.Fatal("first supervisor did not acquire ownership")
	}
	if conn.TryStartWebSocketLoop() {
		t.Fatal("second supervisor acquired duplicate ownership")
	}

	conn.FinishWebSocketLoop()
	if !conn.TryStartWebSocketLoop() {
		t.Fatal("supervisor ownership was not released")
	}
}

func TestSetStatus_ReplacesEntireStatus(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	conn.SetStatus(&DeviceStatus{
		Volume:      &models.Volume{ActualVolume: 42},
		IsConnected: true,
	})

	got := conn.Status()
	if got.Volume == nil || got.Volume.ActualVolume != 42 {
		t.Errorf("Volume not stored: got %+v", got.Volume)
	}

	// Setting a sparser status should wipe previously-set fields.
	conn.SetStatus(&DeviceStatus{IsConnected: false})

	got = conn.Status()
	if got.Volume != nil {
		t.Error("SetStatus did not wipe previously-set Volume")
	}

	if got.IsConnected {
		t.Error("SetStatus did not wipe IsConnected")
	}
}

func TestUpdateStatus_AppliesMutator(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})

	conn.UpdateStatus(func(s *DeviceStatus) {
		s.IsConnected = true
		s.Volume = &models.Volume{ActualVolume: 30}
	})

	got := conn.Status()
	if !got.IsConnected {
		t.Error("UpdateStatus did not set IsConnected")
	}

	if got.Volume == nil || got.Volume.ActualVolume != 30 {
		t.Errorf("UpdateStatus did not set Volume: %+v", got.Volume)
	}
}

func TestUpdateStatus_PreservesUnchangedFields(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	conn.SetStatus(&DeviceStatus{
		Volume:      &models.Volume{ActualVolume: 10},
		Bass:        &models.Bass{ActualBass: 3},
		Group:       &models.Group{ID: "pair-1", Name: "Living Room"},
		Zone:        &models.ZoneInfo{Master: "zone-master", Members: []models.Member{{DeviceID: "zone-member"}}},
		IsConnected: true,
	})

	// Only touch Volume; Bass and IsConnected must survive.
	conn.UpdateStatus(func(s *DeviceStatus) {
		s.Volume = &models.Volume{ActualVolume: 99}
	})

	got := conn.Status()
	if got.Volume.ActualVolume != 99 {
		t.Errorf("Volume = %d, want 99", got.Volume.ActualVolume)
	}

	if got.Bass == nil || got.Bass.ActualBass != 3 {
		t.Errorf("Bass not preserved: %+v", got.Bass)
	}

	if got.Group == nil || got.Group.ID != "pair-1" {
		t.Errorf("Group not preserved: %+v", got.Group)
	}

	if got.Zone == nil || got.Zone.Master != "zone-master" {
		t.Errorf("Zone not preserved: %+v", got.Zone)
	}

	if !got.IsConnected {
		t.Error("IsConnected not preserved")
	}
}

func TestDeviceStatusGroupJSON(t *testing.T) {
	status := DeviceStatus{
		Group: &models.Group{
			ID:             "pair-1",
			Name:           "Living Room",
			MasterDeviceID: "master-1",
		},
	}

	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal DeviceStatus: %v", err)
	}

	var decoded struct {
		Group *models.Group `json:"group"`
	}

	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("Unmarshal DeviceStatus: %v", err)
	}

	if decoded.Group == nil || decoded.Group.ID != "pair-1" || decoded.Group.MasterDeviceID != "master-1" {
		t.Fatalf("group did not round-trip in status JSON: %+v", decoded.Group)
	}

	emptyPayload, err := json.Marshal(DeviceStatus{})
	if err != nil {
		t.Fatalf("Marshal empty DeviceStatus: %v", err)
	}

	var emptyDecoded map[string]json.RawMessage
	if err := json.Unmarshal(emptyPayload, &emptyDecoded); err != nil {
		t.Fatalf("Unmarshal empty DeviceStatus: %v", err)
	}

	if _, ok := emptyDecoded["group"]; ok {
		t.Errorf("nil group should be omitted, JSON = %s", emptyPayload)
	}
}

func TestGroupEventSupersedesInFlightPoll(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	generation := conn.BeginGroupRefresh()

	eventGroup := &models.Group{ID: "new-pair", MasterDeviceID: "master"}
	if !conn.ApplyGroupEvent(eventGroup, time.Now()) {
		t.Fatal("new group event should change group state")
	}

	if conn.ApplyPolledGroup(generation, &models.Group{ID: "stale-pair"}) {
		t.Fatal("stale poll must not replace a newer group event")
	}

	if got := conn.Status().Group; got == nil || got.ID != "new-pair" {
		t.Fatalf("Group = %+v, want newer event state", got)
	}
}

// TestPolledGroupAppliesAfterNewerRefreshStartFailsToApply guards against
// invalidating by start order: a second poll starting (and never applying,
// e.g. its own GetGroup failed) must not discard an earlier poll's
// still-arriving successful result.
func TestPolledGroupAppliesAfterNewerRefreshStartFailsToApply(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	genA := conn.BeginGroupRefresh()
	_ = conn.BeginGroupRefresh() // a second poll starts but never calls ApplyPolledGroup

	if !conn.ApplyPolledGroup(genA, &models.Group{ID: "pair-1", MasterDeviceID: "master"}) {
		t.Fatal("an older poll's successful result must still apply when nothing newer ever actually applied")
	}

	if got := conn.Status().Group; got == nil || got.ID != "pair-1" {
		t.Fatalf("Group = %+v, want pair-1 applied", got)
	}
}

// TestOlderPolledGroupRejectedAfterNewerPollApplies guards the original
// protection this mechanism exists for: a genuinely newer successful poll
// must not be clobbered by an older one arriving late.
func TestOlderPolledGroupRejectedAfterNewerPollApplies(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	genA := conn.BeginGroupRefresh()
	genB := conn.BeginGroupRefresh()

	if !conn.ApplyPolledGroup(genB, &models.Group{ID: "pair-new", MasterDeviceID: "master"}) {
		t.Fatal("newer poll result should apply")
	}

	if conn.ApplyPolledGroup(genA, &models.Group{ID: "pair-stale", MasterDeviceID: "master"}) {
		t.Fatal("an older poll's result arriving after a newer one already applied must be rejected")
	}

	if got := conn.Status().Group; got == nil || got.ID != "pair-new" {
		t.Fatalf("Group = %+v, want pair-new preserved", got)
	}
}

func TestEmptyGroupClearsCurrentClaim(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	conn.SetStatus(&DeviceStatus{Group: &models.Group{ID: "pair-1"}})

	if !conn.ApplyGroupEvent(&models.Group{}, time.Now()) {
		t.Fatal("empty teardown event should change group state")
	}

	if got := conn.Status().Group; got != nil {
		t.Fatalf("Group = %+v, want nil after teardown", got)
	}
}

// TestApplyGroupEventIgnoresRoleOrder guards replaceGroup's change-detection
// against a spurious "changed" report when the same pair's roles simply
// arrive in a different order -- a polled /getGroup response and a pushed
// groupUpdated event both populate Roles.Roles straight from XML unmarshal
// in wire order, so nothing guarantees they list LEFT/RIGHT the same way
// every time for the identical pair.
func TestApplyGroupEventIgnoresRoleOrder(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})

	leftFirst := &models.Group{
		ID:             "pair-1",
		MasterDeviceID: "master",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "master", Role: "LEFT"},
			{DeviceID: "member", Role: "RIGHT"},
		}},
	}
	conn.SetStatus(&DeviceStatus{Group: leftFirst})

	rightFirst := &models.Group{
		ID:             "pair-1",
		MasterDeviceID: "master",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "member", Role: "RIGHT"},
			{DeviceID: "master", Role: "LEFT"},
		}},
	}

	if conn.ApplyGroupEvent(rightFirst, time.Now()) {
		t.Fatal("reordered roles for the same pair must not report a change")
	}
}

func TestFieldPollCannotOverwriteNewerFieldEvent(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	poll := conn.BeginFieldPoll(FieldVolume)

	conn.ApplyFieldEvent(FieldVolume, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 99}
	})

	if conn.CompleteFieldPoll(FieldVolume, poll, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 42}
	}) {
		t.Fatal("poll that began before a same-field event was applied")
	}

	if got := conn.Status().Volume; got == nil || got.ActualVolume != 99 {
		t.Fatalf("field event was overwritten by older poll data: %+v", got)
	}
}

func TestNewerFieldPollSupersedesOlderFieldPoll(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	older := conn.BeginFieldPoll(FieldVolume)
	newer := conn.BeginFieldPoll(FieldVolume)

	if !conn.CompleteFieldPoll(FieldVolume, newer, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 30}
	}) {
		t.Fatal("newer poll was not applied")
	}

	if conn.CompleteFieldPoll(FieldVolume, older, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 10}
	}) {
		t.Fatal("older poll completed after a newer same-field poll was applied")
	}

	if got := conn.Status().Volume; got == nil || got.ActualVolume != 30 {
		t.Fatalf("newer poll state was overwritten: %+v", got)
	}
}

func TestDuplicateFieldEventStillInvalidatesOlderSameFieldPoll(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	conn.SetStatus(&DeviceStatus{Volume: &models.Volume{ActualVolume: 25}})
	poll := conn.BeginFieldPoll(FieldVolume)

	conn.ApplyFieldEvent(FieldVolume, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 25}
		status.LastActivity = time.Now()
	})

	if conn.CompleteFieldPoll(FieldVolume, poll, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 10}
	}) {
		t.Fatal("poll that preceded duplicate same-field event evidence was applied")
	}
}

// TestUnrelatedFieldEventDoesNotInvalidateInFlightPoll guards the actual
// production bug this per-field design replaces a connection-wide gate to
// fix: a push event for one field must never discard a DIFFERENT field's
// still-in-flight, ultimately-successful poll. Sources in particular has no
// push event at all (see StatusField's doc comment) and would go stale
// indefinitely under ordinary event traffic if this regressed.
func TestUnrelatedFieldEventDoesNotInvalidateInFlightPoll(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	sourcesPoll := conn.BeginFieldPoll(FieldSources)

	// An unrelated field's event fires while the Sources poll is still in
	// flight.
	conn.ApplyFieldEvent(FieldVolume, func(status *DeviceStatus) {
		status.Volume = &models.Volume{ActualVolume: 50}
	})

	if !conn.CompleteFieldPoll(FieldSources, sourcesPoll, func(status *DeviceStatus) {
		status.Sources = &models.Sources{}
	}) {
		t.Fatal("an unrelated field's event must not invalidate this field's in-flight poll")
	}

	if got := conn.Status().Sources; got == nil {
		t.Fatal("Sources poll result was discarded by an unrelated field's event")
	}

	if got := conn.Status().Volume; got == nil || got.ActualVolume != 50 {
		t.Fatalf("unrelated field event itself was lost: %+v", got)
	}
}

// TestTransportStateObservationDoesNotFenceFieldConnectivityPoll guards
// against routing ObserveEventStreamTransport through
// ApplyFieldEvent(FieldConnectivity, ...): that would unconditionally bump
// FieldConnectivity's applied generation past whatever an in-flight HTTP
// poll already reserved, so a transient WebSocket transport blip could
// silently discard a concurrently-completing, genuinely successful poll's
// IsConnected merge.
func TestTransportStateObservationDoesNotFenceFieldConnectivityPoll(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	connectivityPoll := conn.BeginFieldPoll(FieldConnectivity)

	if !conn.ObserveEventStreamTransport(1, false, time.Now()) {
		t.Fatal("first transport observation should be accepted")
	}

	if !conn.CompleteFieldPoll(FieldConnectivity, connectivityPoll, func(status *DeviceStatus) {
		status.IsConnected = true
	}) {
		t.Fatal("a transport-state observation must not invalidate an in-flight FieldConnectivity poll")
	}

	if !conn.Status().IsConnected {
		t.Fatal("FieldConnectivity poll result was discarded by an unrelated transport-state observation")
	}
}

func TestZoneCacheRequiresAuthoritativeMaster(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "master"})
	zone := &models.ZoneInfo{
		Master: "MASTER",
		Members: []models.Member{
			{DeviceID: "MASTER", IP: "192.0.2.10"},
			{DeviceID: "MEMBER", IP: "192.0.2.20"},
		},
	}

	refresh := conn.BeginZoneRefresh()
	if !conn.ApplyPolledZone(refresh, "MASTER", zone) {
		t.Fatal("master-confirmed zone was not stored")
	}

	memberRefresh := conn.BeginZoneRefresh()
	if conn.ApplyPolledZone(memberRefresh, "MEMBER", zone) {
		t.Fatal("member response was accepted as authoritative")
	}
	if conn.Status().Zone == nil || conn.Status().Zone.Master != "MASTER" {
		t.Fatalf("member response cleared cached topology: %+v", conn.Status().Zone)
	}
}

func TestZoneCacheRejectsStaleRefreshAndClearsOnMasterStandalone(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "master"})
	zone := &models.ZoneInfo{
		Master: "MASTER",
		Members: []models.Member{
			{DeviceID: "MASTER", IP: "192.0.2.10"},
			{DeviceID: "MEMBER", IP: "192.0.2.20"},
		},
	}

	initial := conn.BeginZoneRefresh()
	if !conn.ApplyPolledZone(initial, "MASTER", zone) {
		t.Fatal("initial zone was not stored")
	}

	stale := conn.BeginZoneRefresh()
	standalone := conn.BeginZoneRefresh()
	if !conn.ApplyPolledZone(standalone, "MASTER", &models.ZoneInfo{
		Master:  "MASTER",
		Members: []models.Member{{DeviceID: "MASTER", IP: "192.0.2.10"}},
	}) {
		t.Fatal("master-confirmed standalone response did not clear the zone")
	}
	if conn.Status().Zone != nil {
		t.Fatalf("standalone topology remained cached: %+v", conn.Status().Zone)
	}
	if conn.ApplyPolledZone(stale, "MASTER", zone) {
		t.Fatal("older response was accepted after a newer result applied")
	}
	if conn.Status().Zone != nil {
		t.Fatal("stale response restored cleared topology")
	}
}

func TestZoneEventBarrierSurvivesLaterRoutineRefreshStart(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "master"})
	conn.SetStatus(&DeviceStatus{Zone: &models.ZoneInfo{
		Master:  "MASTER",
		Members: []models.Member{{DeviceID: "MEMBER", IP: "192.0.2.20"}},
	}})

	eventRefresh := conn.BeginZoneEventRefresh()
	_ = conn.BeginZoneRefresh() // Simulates a later routine poll that then fails.

	if !conn.ApplyPolledZone(eventRefresh, "MASTER", &models.ZoneInfo{
		Master:  "MASTER",
		Members: []models.Member{{DeviceID: "MASTER", IP: "192.0.2.10"}},
	}) {
		t.Fatal("later routine start invalidated the successful event refresh")
	}
	if conn.Status().Zone != nil {
		t.Fatalf("event-confirmed standalone topology remained cached: %+v", conn.Status().Zone)
	}
}

func TestStatusSnapshotIsolation(t *testing.T) {
	// A snapshot returned by Status() must NOT change when a later
	// UpdateStatus replaces a pointer field. This proves the atomic
	// store gives readers a stable view (so long as the writer
	// follows the docstring contract of replacing nested pointers).
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	conn.SetStatus(&DeviceStatus{Volume: &models.Volume{ActualVolume: 1}})

	first := conn.Status()

	conn.UpdateStatus(func(s *DeviceStatus) {
		s.Volume = &models.Volume{ActualVolume: 2}
	})

	if first.Volume.ActualVolume != 1 {
		t.Errorf("Snapshot mutated after later UpdateStatus: got %d, want 1",
			first.Volume.ActualVolume)
	}

	if conn.Status().Volume.ActualVolume != 2 {
		t.Errorf("Current status not updated: got %d, want 2",
			conn.Status().Volume.ActualVolume)
	}
}

// TestStatusConcurrent runs many UpdateStatus writers alongside many
// Status() readers. Before atomic.Pointer[DeviceStatus] this pattern
// would be flagged by the race detector (writers mutate
// conn.Status.X while readers copy conn.Status). With the atomic
// pointer it must run clean under `go test -race`.
func TestStatusConcurrent(t *testing.T) {
	conn := NewDeviceConnection(nil, &models.DeviceInfo{Name: "concurrent"})

	const writers = 16

	const readersPerKind = 16

	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(writers + 2*readersPerKind)

	// Writers: each goroutine replaces NowPlaying with a fresh struct
	// carrying its worker id. Replacement (not in-place mutation)
	// is what the UpdateStatus contract requires for nested
	// pointers.
	for w := 0; w < writers; w++ {
		go func(worker int) {
			defer wg.Done()

			for i := 0; i < opsPerGoroutine; i++ {
				conn.UpdateStatus(func(s *DeviceStatus) {
					s.NowPlaying = &models.NowPlaying{
						Track: fmt.Sprintf("w%d-%d", worker, i),
					}
					s.IsConnected = true
				})
			}
		}(w)
	}

	// Readers via Status() — full snapshot.
	for r := 0; r < readersPerKind; r++ {
		go func() {
			defer wg.Done()

			for i := 0; i < opsPerGoroutine; i++ {
				_ = conn.Status()
			}
		}()
	}

	// Readers that deref a single field. Tests the common
	// "device.Status().IsConnected" pattern.
	for r := 0; r < readersPerKind; r++ {
		go func() {
			defer wg.Done()

			for i := 0; i < opsPerGoroutine; i++ {
				_ = conn.Status().IsConnected
			}
		}()
	}

	wg.Wait()

	// After all writers finish, IsConnected should be true (every
	// writer sets it). The exact NowPlaying value is whichever
	// writer landed last, but it must be a valid non-nil pointer.
	final := conn.Status()
	if !final.IsConnected {
		t.Error("IsConnected should be true after writers ran")
	}

	if final.NowPlaying == nil {
		t.Error("NowPlaying should be non-nil after writers ran")
	}
}
