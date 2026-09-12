package soundtouchweb

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func projectionDevice(host, deviceID, name string, connected bool, group *models.Group) DeviceEntry {
	return projectionDeviceAt(host, host, deviceID, name, connected, group)
}

func projectionDeviceAt(controlID, address, deviceID, name string, connected bool, group *models.Group) DeviceEntry {
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
		DeviceID:  deviceID,
		Name:      name,
		Type:      "SoundTouch 10",
		IPAddress: address,
	})
	conn.SetStatus(&webtypes.DeviceStatus{IsConnected: connected, Group: group})

	return DeviceEntry{ID: controlID, Device: conn, LastSeen: conn.LastSeen}
}

func projectionDeviceWithZone(
	host, deviceID, name string,
	connected bool,
	group *models.Group,
	zone *models.ZoneInfo,
) DeviceEntry {
	entry := projectionDevice(host, deviceID, name, connected, group)
	entry.Device.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.Zone = zone
	})

	return entry
}

func testStereoGroup() *models.Group {
	return &models.Group{
		ID:             "pair-1",
		Name:           "Living Room + Living Room",
		MasterDeviceID: "left-id",
		Status:         "GROUP_OK",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: "left-id", Role: "LEFT", IPAddress: "192.0.2.10"},
			{DeviceID: "right-id", Role: "RIGHT", IPAddress: "192.0.2.11"},
		}},
	}
}

func TestProjectDeviceEntriesCollapsesStereoPairUnderMaster(t *testing.T) {
	group := testStereoGroup()
	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	})

	if len(got) != 1 {
		t.Fatalf("projected devices = %d, want one logical stereo target: %+v", len(got), got)
	}

	master, ok := got["192.0.2.10"]
	if !ok {
		t.Fatalf("master control target missing: %+v", got)
	}

	if master.StereoPair == nil {
		t.Fatal("master is missing stereo-pair metadata")
	}

	if master.StereoPair.MemberCount != 2 || master.StereoPair.AvailableMemberCount != 2 || master.StereoPair.Degraded {
		t.Errorf("unexpected pair availability: %+v", master.StereoPair)
	}

	if master.Info.Name != "Living Room" || master.StereoPair.Name != "Living Room" {
		t.Errorf("logical pair name was not projected consistently: %+v", master)
	}

	if _, ok := got["192.0.2.11"]; ok {
		t.Error("physical right member must not be a second control target")
	}
}

// TestProjectDeviceEntriesNormalizesMemberRoleAndDeviceID guards against
// emitting raw, un-normalized Role/DeviceID into the frontend-facing JSON
// while validMasterGroup/registeredMembersAgree/sameGroupClaim all
// trim/uppercase those same fields for internal comparison.
func TestProjectDeviceEntriesNormalizesMemberRoleAndDeviceID(t *testing.T) {
	group := &models.Group{
		ID:             "pair-1",
		Name:           "Living Room + Living Room",
		MasterDeviceID: "left-id",
		Status:         "GROUP_OK",
		Roles: models.GroupRoles{Roles: []models.GroupRole{
			{DeviceID: " left-id ", Role: " left ", IPAddress: "192.0.2.10"},
			{DeviceID: " right-id ", Role: " right ", IPAddress: "192.0.2.11"},
		}},
	}

	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	})

	pair := got["192.0.2.10"].StereoPair
	if pair == nil || len(pair.Members) != 2 {
		t.Fatalf("expected a projected pair with two members: %+v", got)
	}

	for _, member := range pair.Members {
		if member.DeviceID != strings.TrimSpace(member.DeviceID) {
			t.Errorf("member DeviceID = %q, want trimmed", member.DeviceID)
		}
		if member.Role != strings.ToUpper(member.Role) {
			t.Errorf("member Role = %q, want upper-cased", member.Role)
		}
	}
}

func TestProjectDeviceEntriesKeepsHostnameControlIDSeparateFromAddress(t *testing.T) {
	entry := projectionDeviceAt("kitchen.local", "192.0.2.10", "kitchen-id", "Kitchen", true, nil)
	got := projectDeviceEntries([]DeviceEntry{entry})

	view, ok := got["kitchen.local"]
	if !ok || len(got) != 1 {
		t.Fatalf("hostname-keyed projection = %+v", got)
	}
	if view.Info == nil || view.Info.IPAddress != "192.0.2.10" {
		t.Fatalf("presentation address = %+v, want canonical numeric IP", view.Info)
	}
}

func TestProjectDeviceEntriesUsesLatestInfoName(t *testing.T) {
	entry := projectionDevice("192.0.2.10", "kitchen-id", "Old Kitchen", true, nil)
	entry.Device.ApplyNameEvent("Kitchen")

	view := projectDeviceEntries([]DeviceEntry{entry})["192.0.2.10"]
	if view.Info == nil || view.Info.Name != "Kitchen" {
		t.Fatalf("projected info = %+v, want latest event name", view.Info)
	}
}

func TestProjectDeviceEntriesTreatsStaleStereoMemberAsAvailable(t *testing.T) {
	group := testStereoGroup()
	left := projectionDevice("192.0.2.10", "left-id", "Living Room", false, group)
	left.Device.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.Connectivity = webtypes.ConnectivityStale
	})
	right := projectionDevice("192.0.2.11", "right-id", "Living Room", true, group)
	right.Device.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.Connectivity = webtypes.ConnectivityOnline
	})

	pair := projectDeviceEntries([]DeviceEntry{left, right})["192.0.2.10"].StereoPair
	if pair == nil || pair.AvailableMemberCount != 2 || pair.Degraded {
		t.Fatalf("stale logical member was treated as offline: %+v", pair)
	}
}

func TestProjectDeviceEntriesShowsDegradedPairWhenMemberIsMissing(t *testing.T) {
	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, testStereoGroup()),
	})

	pair := got["192.0.2.10"].StereoPair
	if pair == nil {
		t.Fatal("connected master should remain a logical pair when its member is unavailable")
	}

	if pair.AvailableMemberCount != 1 || !pair.Degraded {
		t.Errorf("missing member not reflected as degraded: %+v", pair)
	}
}

func TestProjectDeviceEntriesKeepsStablePairWhenMasterIsDisconnected(t *testing.T) {
	group := testStereoGroup()
	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", false, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	})

	if len(got) != 1 {
		t.Fatalf("projected devices = %d, want a stable logical pair while its master is registered", len(got))
	}

	pair := got["192.0.2.10"].StereoPair
	if pair == nil || !pair.Degraded || pair.AvailableMemberCount != 1 {
		t.Errorf("disconnected master should produce a degraded logical pair: %+v", got)
	}
}

func TestProjectDeviceEntriesCollapsesMasterConfirmedZone(t *testing.T) {
	zone := &models.ZoneInfo{
		Master: "master-id",
		Members: []models.Member{
			{DeviceID: "master-id", IP: "192.0.2.10"},
			{DeviceID: "member-id", IP: "192.0.2.20"},
		},
	}

	got := projectDeviceEntries([]DeviceEntry{
		projectionDeviceWithZone("192.0.2.10", "master-id", "Kitchen", true, nil, zone),
		projectionDeviceWithZone("192.0.2.20", "member-id", "Dining", true, nil, nil),
		projectionDeviceWithZone("192.0.2.30", "other-id", "Bedroom", true, nil, nil),
	})

	if len(got) != 2 {
		t.Fatalf("projected devices = %d, want zone plus standalone: %+v", len(got), got)
	}

	master := got["192.0.2.10"]
	if master.Zone == nil {
		t.Fatalf("logical zone master missing: %+v", got)
	}
	if master.Zone.MasterDeviceID != "master-id" || master.Zone.MasterControlID != "192.0.2.10" ||
		master.Zone.MemberCount != 2 || master.Zone.PhysicalMemberCount != 2 ||
		master.Zone.AvailableMemberCount != 2 || master.Zone.Degraded {
		t.Fatalf("unexpected zone projection: %+v", master.Zone)
	}
	if member := master.Zone.Members[1]; member.ControlID != "192.0.2.20" ||
		member.HardwareID != "member-id" || member.Name != "Dining" ||
		member.Type != "SoundTouch 10" || member.IP != "192.0.2.20" ||
		!member.Available || member.Connectivity != "online" || len(member.PhysicalMembers) != 1 {
		t.Fatalf("unexpected logical zone member: %+v", member)
	}
	if _, exists := got["192.0.2.20"]; exists {
		t.Fatal("zone member remained a separate control target")
	}
}

func TestProjectDeviceEntriesFoldsStereoBeforeZone(t *testing.T) {
	group := testStereoGroup()
	group.Name = "Living Room"
	zone := &models.ZoneInfo{
		Master: "master-id",
		Members: []models.Member{
			{DeviceID: "master-id", IP: "192.0.2.5"},
			{DeviceID: "left-id", IP: "192.0.2.10"},
		},
	}

	got := projectDeviceEntries([]DeviceEntry{
		projectionDeviceWithZone("192.0.2.5", "master-id", "Kitchen", true, nil, zone),
		projectionDeviceWithZone("192.0.2.10", "left-id", "Living Room Left", true, group, nil),
		projectionDeviceWithZone("192.0.2.11", "right-id", "Living Room Right", false, group, nil),
	})

	view := got["192.0.2.5"].Zone
	if len(got) != 1 || view == nil {
		t.Fatalf("zone with stereo member was not collapsed: %+v", got)
	}
	if view.MemberCount != 2 || view.PhysicalMemberCount != 3 || !view.Degraded {
		t.Fatalf("logical/physical counts or degradation are wrong: %+v", view)
	}
	pair := view.Members[1]
	if pair.Kind != "stereoPair" || pair.ControlID != "192.0.2.10" ||
		pair.HardwareID != "left-id" || pair.StereoPair == nil ||
		len(pair.DeviceIDs) != 2 || len(pair.PhysicalMembers) != 2 {
		t.Fatalf("stereo zone member was not nested: %+v", pair)
	}
	if pair.PhysicalMembers[0].Role != "LEFT" || pair.PhysicalMembers[1].Role != "RIGHT" ||
		pair.PhysicalMembers[1].Available || pair.PhysicalMembers[1].Connectivity != "offline" {
		t.Fatalf("physical stereo status was not preserved: %+v", pair.PhysicalMembers)
	}
}

func TestProjectDeviceEntriesFailsOpenWithoutMasterZoneClaim(t *testing.T) {
	zone := &models.ZoneInfo{
		Master:  "master-id",
		Members: []models.Member{{DeviceID: "member-id", IP: "192.0.2.20"}},
	}

	got := projectDeviceEntries([]DeviceEntry{
		projectionDeviceWithZone("192.0.2.10", "master-id", "Kitchen", true, nil, nil),
		projectionDeviceWithZone("192.0.2.20", "member-id", "Dining", true, nil, zone),
	})

	if len(got) != 2 || got["192.0.2.10"].Zone != nil || got["192.0.2.20"].Zone != nil {
		t.Fatalf("member-only zone claim hid physical cards: %+v", got)
	}
}

func TestProjectDeviceEntriesFailsOpenForConflictingZoneClaims(t *testing.T) {
	zoneA := &models.ZoneInfo{
		Master:  "a-id",
		Members: []models.Member{{DeviceID: "b-id", IP: "192.0.2.20"}},
	}
	zoneB := &models.ZoneInfo{
		Master:  "b-id",
		Members: []models.Member{{DeviceID: "c-id", IP: "192.0.2.30"}},
	}

	got := projectDeviceEntries([]DeviceEntry{
		projectionDeviceWithZone("192.0.2.10", "a-id", "A", true, nil, zoneA),
		projectionDeviceWithZone("192.0.2.20", "b-id", "B", true, nil, zoneB),
		projectionDeviceWithZone("192.0.2.30", "c-id", "C", true, nil, nil),
	})

	if len(got) != 3 {
		t.Fatalf("conflicting zones hid a physical card: %+v", got)
	}
	for id, view := range got {
		if view.Zone != nil {
			t.Fatalf("conflicting zone was projected for %s: %+v", id, view.Zone)
		}
	}
}

func TestProjectDeviceEntriesFailsOpenForUnknownPhysicalMemberConflict(t *testing.T) {
	zoneA := &models.ZoneInfo{
		Master:  "a-id",
		Members: []models.Member{{DeviceID: "unknown-id", IP: "192.0.2.99"}},
	}
	zoneB := &models.ZoneInfo{
		Master:  "b-id",
		Members: []models.Member{{DeviceID: "unknown-id", IP: "192.0.2.99"}},
	}

	got := projectDeviceEntries([]DeviceEntry{
		projectionDeviceWithZone("192.0.2.10", "a-id", "A", true, nil, zoneA),
		projectionDeviceWithZone("192.0.2.20", "b-id", "B", true, nil, zoneB),
	})

	if len(got) != 2 {
		t.Fatalf("zones sharing an unknown physical member hid a master card: %+v", got)
	}
	for id, view := range got {
		if view.Zone != nil {
			t.Fatalf("conflicting zone was projected for %s: %+v", id, view.Zone)
		}
	}
}

func TestProjectDeviceEntriesLeavesMemberPhysicalWhenMasterIsAbsent(t *testing.T) {
	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, testStereoGroup()),
	})

	if len(got) != 1 || got["192.0.2.11"].StereoPair != nil {
		t.Fatalf("member without a registered master must remain a physical target: %+v", got)
	}
}

func TestProjectDeviceEntriesRequiresMasterReportedGroup(t *testing.T) {
	group := testStereoGroup()
	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, nil),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	})

	if len(got) != 2 {
		t.Fatalf("slave-only group data must not collapse the registry: %+v", got)
	}
}

func TestProjectDeviceEntriesRejectsMalformedGroup(t *testing.T) {
	group := testStereoGroup()
	group.Roles.Roles[1].DeviceID = group.Roles.Roles[0].DeviceID

	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	})

	if len(got) != 2 {
		t.Fatalf("malformed pair must not hide a physical device: %+v", got)
	}
}

func TestProjectDeviceEntriesRejectsConflictingMemberClaim(t *testing.T) {
	masterGroup := testStereoGroup()
	memberGroup := testStereoGroup()
	memberGroup.ID = "different-pair"

	got := projectDeviceEntries([]DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, masterGroup),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, memberGroup),
	})

	if len(got) != 2 {
		t.Fatalf("conflicting pair claims must fail open: %+v", got)
	}
}

func TestProjectCapturedDeviceEntriesUsesOneCoherentStatusPerDevice(t *testing.T) {
	group := testStereoGroup()
	entries := []DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	}
	captured := captureDeviceProjectionEntries(entries)

	entries[0].Device.ApplyGroupEvent(&models.Group{}, time.Now())
	entries[1].Device.ApplyGroupEvent(&models.Group{}, time.Now())

	got := projectCapturedDeviceEntries(captured)
	master := got["192.0.2.10"]
	if master.StereoPair == nil || master.Status == nil || master.Status.Group == nil || master.Status.Group.ID != "pair-1" {
		t.Fatalf("captured projection mixed newer connection state into its response: %+v", got)
	}

	if fresh := projectDeviceEntries(entries); len(fresh) != 2 {
		t.Fatalf("fresh projection did not observe the cleared group: %+v", fresh)
	}
}

func TestDeviceViewSnapshotConcurrentTouchUsesCapturedLastSeen(t *testing.T) {
	app := NewWebApp()
	conn := newRegistryDevice("Living Room")
	if !app.AddDevice("192.0.2.10", conn) {
		t.Fatal("AddDevice returned false on first insert")
	}

	stale := app.DeviceSnapshot()
	if len(stale) != 1 {
		t.Fatalf("DeviceSnapshot len = %d, want 1", len(stale))
	}

	if !app.TouchDevice("192.0.2.10") {
		t.Fatal("TouchDevice returned false for registered device")
	}
	if got := projectDeviceEntries(stale)["192.0.2.10"].LastSeen; got != stale[0].LastSeen {
		t.Fatalf("projection LastSeen = %s, want captured value %s", got, stale[0].LastSeen)
	}

	const iterations = 1000
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			app.TouchDevice("192.0.2.10")
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			_ = app.deviceViewSnapshot()
		}
	}()

	close(start)
	wg.Wait()
}

func TestHandleAPIDevicesUsesLogicalStereoProjection(t *testing.T) {
	app := NewWebApp()
	group := testStereoGroup()
	for _, entry := range []DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
	} {
		app.AddDevice(entry.ID, entry.Device)
	}

	response := httptest.NewRecorder()
	app.HandleAPIDevices(response, httptest.NewRequest("GET", "/api/control/devices", nil))

	var payload struct {
		Success bool                  `json:"success"`
		Data    map[string]deviceView `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode devices response: %v", err)
	}

	if response.Code != http.StatusOK || !payload.Success || len(payload.Data) != 1 {
		t.Fatalf("unexpected devices response: status=%d payload=%+v", response.Code, payload)
	}

	if pair := payload.Data["192.0.2.10"].StereoPair; pair == nil || pair.ID != "pair-1" || pair.MemberCount != 2 {
		t.Fatalf("logical stereo metadata missing from devices API: %+v", payload.Data)
	}
}
