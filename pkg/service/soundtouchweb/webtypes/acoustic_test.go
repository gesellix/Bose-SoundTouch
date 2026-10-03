package webtypes

import (
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestConfirmedBalanceTargetAcceptsEitherPairMaster(t *testing.T) {
	tests := []struct {
		name       string
		masterRole string
		reordered  bool
	}{
		{name: "left master", masterRole: "LEFT"},
		{name: "left master reordered", masterRole: "LEFT", reordered: true},
		{name: "right master", masterRole: "RIGHT"},
		{name: "right master reordered", masterRole: "RIGHT", reordered: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "master"})
			group := acousticTestPair("pair-1", "master", test.masterRole)
			if test.reordered {
				group.Roles.Roles[0], group.Roles.Roles[1] = group.Roles.Roles[1], group.Roles.Roles[0]
			}

			if !conn.ApplyGroupEvent(group, time.Time{}) {
				t.Fatal("initial pair event was not applied")
			}

			target, ok := conn.ConfirmedBalanceTarget()
			if !ok {
				t.Fatal("valid pair master was not admitted")
			}
			if target.HardwareID != "master" || target.GroupID != "pair-1" || !models.SameGroup(target.Group, group) {
				t.Fatalf("target = %+v, want master of pair-1", target)
			}
		})
	}
}

func TestConfirmedBalanceTargetRejectsNonmasterAndMalformedPairs(t *testing.T) {
	valid := acousticTestPair("pair-1", "master", "LEFT")

	t.Run("nonmaster member", func(t *testing.T) {
		conn := NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "peer"})
		conn.ApplyGroupEvent(valid, time.Time{})
		if _, ok := conn.ConfirmedBalanceTarget(); ok {
			t.Fatal("nonmaster pair member was admitted")
		}
	})

	tests := []struct {
		name  string
		group *models.Group
	}{
		{name: "ordinary group", group: &models.Group{ID: "ordinary", MasterDeviceID: "master"}},
		{name: "missing group id", group: acousticTestPair("", "master", "LEFT")},
		{name: "missing master", group: acousticTestPair("pair-1", "", "LEFT")},
		{name: "master absent", group: acousticTestPair("pair-1", "other", "LEFT")},
		{name: "duplicate device", group: &models.Group{
			ID: "pair-1", MasterDeviceID: "master",
			Roles: models.GroupRoles{Roles: []models.GroupRole{
				{DeviceID: "master", Role: "LEFT"},
				{DeviceID: "master", Role: "RIGHT"},
			}},
		}},
		{name: "duplicate role", group: &models.Group{
			ID: "pair-1", MasterDeviceID: "master",
			Roles: models.GroupRoles{Roles: []models.GroupRole{
				{DeviceID: "master", Role: "LEFT"},
				{DeviceID: "peer", Role: "LEFT"},
			}},
		}},
		{name: "unknown role", group: &models.Group{
			ID: "pair-1", MasterDeviceID: "master",
			Roles: models.GroupRoles{Roles: []models.GroupRole{
				{DeviceID: "master", Role: "LEFT"},
				{DeviceID: "peer", Role: "CENTER"},
			}},
		}},
		{name: "missing member id", group: &models.Group{
			ID: "pair-1", MasterDeviceID: "master",
			Roles: models.GroupRoles{Roles: []models.GroupRole{
				{DeviceID: "master", Role: "LEFT"},
				{Role: "RIGHT"},
			}},
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn := NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "master"})
			conn.ApplyGroupEvent(test.group, time.Time{})
			if _, ok := conn.ConfirmedBalanceTarget(); ok {
				t.Fatalf("malformed group was admitted: %+v", test.group)
			}
		})
	}
}

func TestGroupRefreshInvalidatesTargetButRetainsVerifiedBalance(t *testing.T) {
	conn, group, balance, target := confirmedAcousticConnection(t)
	baselineRevision := conn.Status().BalanceRevision

	_ = conn.BeginGroupRefresh()

	if _, ok := conn.ConfirmedBalanceTarget(); ok {
		t.Fatal("pending group refresh did not immediately disable balance writes")
	}
	if conn.BalanceTargetCurrent(target) {
		t.Fatal("target captured before group refresh remained current")
	}
	if got := conn.Status(); got.Balance != balance || got.BalanceRevision != baselineRevision {
		t.Fatalf("pending or failed refresh changed verified balance: %+v", got.Balance)
	}
	if !models.SameGroup(conn.Status().Group, group) {
		t.Fatalf("pending or failed refresh changed cached group: %+v", conn.Status().Group)
	}
}

func TestUnchangedGroupRefreshRecertifiesWithoutClearingBalance(t *testing.T) {
	conn, group, balance, _ := confirmedAcousticConnection(t)
	baselineRevision := conn.Status().BalanceRevision
	generation := conn.BeginGroupRefresh()
	reordered := acousticTestPair(group.ID, group.MasterDeviceID, "LEFT")
	reordered.Roles.Roles[0], reordered.Roles.Roles[1] = reordered.Roles.Roles[1], reordered.Roles.Roles[0]

	accepted, changed := conn.ApplyPolledGroupResult(generation, reordered)
	if !accepted || changed {
		t.Fatalf("unchanged refresh = accepted %v, changed %v; want true, false", accepted, changed)
	}
	if _, ok := conn.ConfirmedBalanceTarget(); !ok {
		t.Fatal("latest unchanged authoritative refresh did not restore pair certainty")
	}
	if got := conn.Status(); got.Balance != balance || got.BalanceRevision != baselineRevision {
		t.Fatalf("unchanged group refresh cleared verified balance: %+v", got.Balance)
	}
}

func TestOlderGroupSuccessCannotCertifyLaterPendingRefresh(t *testing.T) {
	conn, group, balance, oldTarget := confirmedAcousticConnection(t)
	older := conn.BeginGroupRefresh()
	_ = conn.BeginGroupRefresh()

	accepted, changed := conn.ApplyPolledGroupResult(older, group)
	if !accepted || changed {
		t.Fatalf("older completion = accepted %v, changed %v; want true, false", accepted, changed)
	}
	if _, ok := conn.ConfirmedBalanceTarget(); ok {
		t.Fatal("older success certified topology while a later refresh remained pending")
	}
	if conn.BalanceTargetCurrent(oldTarget) {
		t.Fatal("pre-refresh target became current again after older success")
	}
	if conn.Status().Balance != balance {
		t.Fatalf("older unchanged completion cleared verified balance: %+v", conn.Status().Balance)
	}
}

func TestLegacyBaselineGroupCompletionCannotCertifyNewerPendingRefresh(t *testing.T) {
	conn, group, balance, oldTarget := confirmedAcousticConnection(t)
	baseline := conn.GroupGeneration()
	baselineRevision := conn.Status().BalanceRevision
	_ = conn.BeginGroupRefresh()

	accepted, changed := conn.ApplyPolledGroupIfBaselineResult(baseline, group)
	if !accepted || changed {
		t.Fatalf("baseline completion = accepted %v, changed %v; want true, false", accepted, changed)
	}
	if _, ok := conn.ConfirmedBalanceTarget(); ok {
		t.Fatal("legacy baseline completion certified topology over a newer pending refresh")
	}
	if conn.BalanceTargetCurrent(oldTarget) {
		t.Fatal("pre-refresh target became current after legacy baseline completion")
	}
	if got := conn.Status(); got.Balance != balance || got.BalanceRevision != baselineRevision {
		t.Fatalf("unchanged baseline completion cleared verified balance: %+v", got.Balance)
	}
}

func TestGroupChangeAndTeardownClearVerifiedBalance(t *testing.T) {
	tests := []struct {
		name  string
		group *models.Group
	}{
		{name: "changed pair", group: acousticTestPair("pair-2", "master", "RIGHT")},
		{name: "empty group", group: &models.Group{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conn, _, _, oldTarget := confirmedAcousticConnection(t)
			baselineRevision := conn.Status().BalanceRevision

			if !conn.ApplyGroupEvent(test.group, time.Time{}) {
				t.Fatal("topology change was not reported")
			}
			if got := conn.Status().Balance; got != nil {
				t.Fatalf("Balance = %+v, want nil after topology change", got)
			}
			if got := conn.Status().BalanceRevision; got <= baselineRevision {
				t.Fatalf("BalanceRevision = %d, want newer than %d", got, baselineRevision)
			}
			if conn.BalanceTargetCurrent(oldTarget) {
				t.Fatal("target captured for old topology remained current")
			}
		})
	}
}

func TestInvalidateFieldRejectsOldReadWithoutAffectingOtherField(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	oldBass := &models.Bass{DeviceID: "speaker", TargetBass: 1, ActualBass: 1}
	oldBalance := &models.Balance{DeviceID: "speaker", Available: true, Target: 0, Actual: 0}
	conn.ApplyFieldEvent(FieldBass, func(status *DeviceStatus) { status.Bass = oldBass })
	conn.ApplyFieldEvent(FieldBalance, func(status *DeviceStatus) { status.Balance = oldBalance })
	baselineBassRevision := conn.Status().BassRevision

	bassRead := conn.BeginFieldPoll(FieldBass)
	balanceRead := conn.BeginFieldPoll(FieldBalance)
	barrier := conn.InvalidateField(FieldBass)
	if barrier <= bassRead {
		t.Fatalf("invalidation generation = %d, want newer than read %d", barrier, bassRead)
	}

	newBalance := &models.Balance{DeviceID: "speaker", Available: true, Target: 2, Actual: 2}
	if !conn.CompleteFieldPoll(FieldBalance, balanceRead, func(status *DeviceStatus) {
		status.Balance = newBalance
	}) {
		t.Fatal("bass invalidation rejected independent balance read")
	}
	if conn.CompleteFieldPoll(FieldBass, bassRead, func(status *DeviceStatus) {
		status.Bass = &models.Bass{DeviceID: "speaker", TargetBass: 9, ActualBass: 9}
	}) {
		t.Fatal("read reserved before invalidation barrier was accepted")
	}

	got := conn.Status()
	if got.Bass != oldBass || got.BassRevision != baselineBassRevision {
		t.Fatalf("invalidation changed last verified bass: %+v", got.Bass)
	}
	if got.Balance != newBalance || got.BalanceRevision != balanceRead {
		t.Fatalf("independent balance read was not merged: %+v", got.Balance)
	}
}

func TestAcceptedFieldMergeIsAtomicWithNewerResult(t *testing.T) {
	conn := NewDeviceConnection(nil, nil)
	older := conn.BeginFieldPoll(FieldBass)
	newer := conn.BeginFieldPoll(FieldBass)
	olderEntered := make(chan struct{})
	releaseOlder := make(chan struct{})
	olderResult := make(chan bool, 1)

	go func() {
		olderResult <- conn.CompleteFieldPoll(FieldBass, older, func(status *DeviceStatus) {
			close(olderEntered)
			<-releaseOlder
			status.Bass = &models.Bass{DeviceID: "speaker", TargetBass: 1, ActualBass: 1}
		})
	}()
	<-olderEntered

	if conn.fieldGenMu.TryLock() {
		conn.fieldGenMu.Unlock()
		close(releaseOlder)
		t.Fatal("field generation lock was released during an accepted status merge")
	}

	newerStarted := make(chan struct{})
	newerResult := make(chan bool, 1)
	go func() {
		close(newerStarted)
		newerResult <- conn.CompleteFieldPoll(FieldBass, newer, func(status *DeviceStatus) {
			status.Bass = &models.Bass{DeviceID: "speaker", TargetBass: 2, ActualBass: 2}
		})
	}()
	<-newerStarted
	close(releaseOlder)

	if !<-olderResult {
		t.Fatal("older result was unexpectedly rejected before any field result had applied")
	}
	if !<-newerResult {
		t.Fatal("newer result was not accepted after the paused merge completed")
	}
	if got := conn.Status(); got.Bass == nil || got.Bass.ActualBass != 2 || got.BassRevision != newer {
		t.Fatalf("final Bass = %+v at revision %d, want newer result at %d", got.Bass, got.BassRevision, newer)
	}
}

func confirmedAcousticConnection(t *testing.T) (*DeviceConnection, *models.Group, *models.Balance, BalanceTarget) {
	t.Helper()

	conn := NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "master"})
	group := acousticTestPair("pair-1", "master", "LEFT")
	if !conn.ApplyGroupEvent(group, time.Time{}) {
		t.Fatal("initial pair event was not applied")
	}

	balance := &models.Balance{
		DeviceID:  "master",
		Available: true,
		Min:       -7,
		Max:       7,
		Target:    1,
		Actual:    1,
	}
	conn.ApplyFieldEvent(FieldBalance, func(status *DeviceStatus) {
		status.Balance = balance
	})

	target, ok := conn.ConfirmedBalanceTarget()
	if !ok {
		t.Fatal("test pair master was not admitted")
	}

	return conn, group, balance, target
}

func acousticTestPair(id, masterID, masterRole string) *models.Group {
	peerRole := "RIGHT"
	if masterRole == "RIGHT" {
		peerRole = "LEFT"
	}

	return &models.Group{
		ID:             id,
		MasterDeviceID: masterID,
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "master", Role: masterRole},
			{DeviceID: "peer", Role: peerRole},
		}},
	}
}
