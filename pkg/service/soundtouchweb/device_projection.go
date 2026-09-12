package soundtouchweb

import (
	"net"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// deviceView is the player-facing representation of one control target.
// A stereo pair is projected as one target keyed by its master speaker's host;
// the underlying registry continues to track both physical speakers.
type deviceView struct {
	Info       *models.DeviceInfo     `json:"info"`
	Status     *webtypes.DeviceStatus `json:"status"`
	LastSeen   time.Time              `json:"lastSeen"`
	StereoPair *stereoPairView        `json:"stereoPair,omitempty"`
	Zone       *zoneView              `json:"zone,omitempty"`
}

// deviceProjectionEntry captures one immutable status pointer per physical
// device. Projection must not re-read live connection state midway through
// building a response, otherwise group membership and the emitted status can
// describe different moments.
type deviceProjectionEntry struct {
	ID       string
	Info     *models.DeviceInfo
	Status   *webtypes.DeviceStatus
	LastSeen time.Time
}

// stereoPairView describes the physical members represented by a logical
// player target. Controls are always sent to MasterDeviceID via the map key.
type stereoPairView struct {
	ID                   string                 `json:"id"`
	Name                 string                 `json:"name,omitempty"`
	MasterDeviceID       string                 `json:"masterDeviceId"`
	Status               string                 `json:"status,omitempty"`
	MemberCount          int                    `json:"memberCount"`
	AvailableMemberCount int                    `json:"availableMemberCount"`
	Degraded             bool                   `json:"degraded"`
	Members              []stereoPairMemberView `json:"members"`
}

// stereoPairMemberView is the player-facing role and availability of one
// physical speaker in a stereo pair.
type stereoPairMemberView struct {
	DeviceID  string `json:"deviceId"`
	Role      string `json:"role"`
	IPAddress string `json:"ipAddress,omitempty"`
	Name      string `json:"name,omitempty"`
	Available bool   `json:"available"`
}

// zoneView describes one master-authoritative multiroom zone after physical
// stereo members have been folded into logical player targets.
type zoneView struct {
	MasterDeviceID       string           `json:"masterDeviceId"`
	MasterControlID      string           `json:"masterControlId"`
	MemberCount          int              `json:"memberCount"`
	PhysicalMemberCount  int              `json:"physicalMemberCount"`
	AvailableMemberCount int              `json:"availableMemberCount"`
	Degraded             bool             `json:"degraded"`
	Members              []zoneMemberView `json:"members"`
}

type zoneMemberView struct {
	Kind            string                   `json:"kind"`
	ControlID       string                   `json:"controlId,omitempty"`
	IP              string                   `json:"ip,omitempty"`
	HardwareID      string                   `json:"hwId,omitempty"`
	Name            string                   `json:"name,omitempty"`
	Model           string                   `json:"model"`
	Type            string                   `json:"type"`
	DeviceIDs       []string                 `json:"deviceIds"`
	Available       bool                     `json:"available"`
	Connectivity    string                   `json:"connectivity"`
	PhysicalMembers []zonePhysicalMemberView `json:"physicalMembers"`
	StereoPair      *stereoPairView          `json:"stereoPair,omitempty"`
}

type zonePhysicalMemberView struct {
	DeviceID     string `json:"deviceId"`
	Role         string `json:"role,omitempty"`
	IP           string `json:"ip"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Available    bool   `json:"available"`
	Connectivity string `json:"connectivity"`
}

// stereoPairCapable reports whether info's model supports stereo pairing
// (ST-10 only). This must stay a model-name check rather than a runtime
// capability probe: verified against real hardware, a SoundTouch 20 lists
// /getGroup (and /addGroup, /removeGroup, /updateGroup) in its own
// /supportedURLs response even though the device doesn't actually reply to
// /getGroup -- see Client.GetGroup's doc comment. The device's supportedURLs
// listing reflects firmware-level route registration, not per-model feature
// support, so it cannot be used to detect stereo-pair capability either.
func stereoPairCapable(info *models.DeviceInfo) bool {
	if info == nil {
		return false
	}

	typeName := strings.ToLower(strings.TrimSpace(info.Type))

	return typeName == "st10" || typeName == "soundtouch 10"
}

// deviceViewSnapshot projects the physical registry into logical control
// targets for the HTTP API and the global player WebSocket.
func (app *WebApp) deviceViewSnapshot() map[string]deviceView {
	return projectDeviceEntries(app.DeviceSnapshot())
}

// deviceViewForID projects the registry into a single logical control
// target and reports whether id is currently visible in the player-facing
// inventory. A hidden stereo-pair member is not visible under its own id --
// only its pair's master key exposes it, via StereoPair.Members.
func (app *WebApp) deviceViewForID(id string) (deviceView, bool) {
	view, ok := app.deviceViewSnapshot()[id]

	return view, ok
}

func projectDeviceEntries(snapshot []DeviceEntry) map[string]deviceView {
	return projectCapturedDeviceEntries(captureDeviceProjectionEntries(snapshot))
}

func captureDeviceProjectionEntries(snapshot []DeviceEntry) []deviceProjectionEntry {
	captured := make([]deviceProjectionEntry, 0, len(snapshot))
	for _, entry := range snapshot {
		if entry.Device == nil {
			continue
		}

		captured = append(captured, deviceProjectionEntry{
			ID:       entry.ID,
			Info:     entry.Device.Info(),
			Status:   entry.Device.Status(),
			LastSeen: entry.LastSeen,
		})
	}

	return captured
}

func projectCapturedDeviceEntries(snapshot []deviceProjectionEntry) map[string]deviceView {
	devices, physicalToLogical, byDeviceID := projectLogicalDeviceEntries(snapshot)

	return projectZoneViews(snapshot, devices, physicalToLogical, byDeviceID)
}

// projectLogicalDeviceEntries folds physical stereo members into their shared
// control target. Zone projection and the zone-detail endpoint both build on
// this representation so names and member status cannot diverge.
func projectLogicalDeviceEntries(snapshot []deviceProjectionEntry) (
	map[string]deviceView,
	map[string]string,
	map[string][]deviceProjectionEntry,
) {
	byDeviceID := make(map[string][]deviceProjectionEntry, len(snapshot))
	for _, entry := range snapshot {
		if entry.Info == nil {
			continue
		}

		deviceID := strings.TrimSpace(entry.Info.DeviceID)
		if deviceID != "" {
			byDeviceID[deviceID] = append(byDeviceID[deviceID], entry)
		}
	}

	masters := make(map[string]*stereoPairView)
	hidden := make(map[string]bool)

	physicalToLogical := make(map[string]string, len(byDeviceID))
	for deviceID, entries := range byDeviceID {
		if len(entries) == 1 {
			physicalToLogical[deviceID] = entries[0].ID
		}
	}

	for _, entry := range snapshot {
		if entry.Info == nil {
			continue
		}

		if entry.Status == nil || !validMasterGroup(entry.Info.DeviceID, entry.Status.Group) {
			continue
		}

		master, unique := uniqueDeviceEntry(byDeviceID, entry.Status.Group.MasterDeviceID)
		if !unique || master.ID != entry.ID || !registeredMembersAgree(entry.Status.Group, byDeviceID) {
			continue
		}

		pair := newStereoPairView(entry.Status.Group, byDeviceID)
		masters[entry.ID] = pair

		for _, role := range entry.Status.Group.Roles.Roles {
			physicalToLogical[strings.TrimSpace(role.DeviceID)] = entry.ID

			member, ok := uniqueDeviceEntry(byDeviceID, role.DeviceID)
			if ok && member.ID != entry.ID {
				hidden[member.ID] = true
			}
		}
	}

	devices := make(map[string]deviceView, len(snapshot))
	for _, entry := range snapshot {
		if hidden[entry.ID] {
			continue
		}

		pair := masters[entry.ID]
		devices[entry.ID] = deviceView{
			Info:       projectedDeviceInfo(entry.ID, entry.Info, pair),
			Status:     entry.Status,
			LastSeen:   entry.LastSeen,
			StereoPair: pair,
		}
	}

	return devices, physicalToLogical, byDeviceID
}

func projectZoneInfo(zone *models.ZoneInfo, snapshot []deviceProjectionEntry) (*zoneView, bool) {
	devices, physicalToLogical, byDeviceID := projectLogicalDeviceEntries(snapshot)
	candidate, ok := newZoneProjectionCandidate(zone, devices, physicalToLogical, byDeviceID)

	return candidate.view, ok
}

type zoneProjectionCandidate struct {
	masterControlID string
	view            *zoneView
	logicalMembers  []string
	physicalMembers []string
}

func projectZoneViews(
	snapshot []deviceProjectionEntry,
	devices map[string]deviceView,
	physicalToLogical map[string]string,
	byDeviceID map[string][]deviceProjectionEntry,
) map[string]deviceView {
	candidates := make([]zoneProjectionCandidate, 0)
	logicalClaims := make(map[string]int)
	physicalClaims := make(map[string]int)

	for _, entry := range snapshot {
		if entry.Info == nil || entry.Status == nil ||
			!validMasterZone(entry.Info.DeviceID, entry.Status.Zone) {
			continue
		}

		candidate, ok := newZoneProjectionCandidate(entry.Status.Zone, devices, physicalToLogical, byDeviceID)
		if !ok {
			continue
		}

		candidates = append(candidates, candidate)
		for _, logicalID := range candidate.logicalMembers {
			logicalClaims[logicalID]++
		}

		for _, deviceID := range candidate.physicalMembers {
			physicalClaims[deviceID]++
		}
	}

	for _, candidate := range candidates {
		conflict := false

		for _, logicalID := range candidate.logicalMembers {
			if logicalClaims[logicalID] != 1 {
				conflict = true
				break
			}
		}

		if !conflict {
			for _, deviceID := range candidate.physicalMembers {
				if physicalClaims[deviceID] != 1 {
					conflict = true
					break
				}
			}
		}

		if conflict {
			continue
		}

		master := devices[candidate.masterControlID]
		master.Zone = candidate.view
		devices[candidate.masterControlID] = master

		for _, logicalID := range candidate.logicalMembers {
			if logicalID != candidate.masterControlID {
				delete(devices, logicalID)
			}
		}
	}

	return devices
}

func validMasterZone(deviceID string, zone *models.ZoneInfo) bool {
	return zone != nil && zoneHasMultipleDevices(zone) &&
		strings.TrimSpace(zone.Master) != "" &&
		strings.TrimSpace(zone.Master) == strings.TrimSpace(deviceID)
}

func zoneHasMultipleDevices(zone *models.ZoneInfo) bool {
	if zone == nil {
		return false
	}

	master := strings.TrimSpace(zone.Master)
	for _, member := range zone.Members {
		memberID := strings.TrimSpace(member.DeviceID)
		if memberID != "" && memberID != master {
			return true
		}
	}

	return false
}

func newZoneProjectionCandidate(
	zone *models.ZoneInfo,
	devices map[string]deviceView,
	physicalToLogical map[string]string,
	byDeviceID map[string][]deviceProjectionEntry,
) (zoneProjectionCandidate, bool) {
	if !zoneHasMultipleDevices(zone) {
		return zoneProjectionCandidate{}, false
	}

	masterDeviceID := strings.TrimSpace(zone.Master)

	masterControlID := physicalToLogical[masterDeviceID]
	if masterControlID == "" {
		return zoneProjectionCandidate{}, false
	}

	if _, ok := devices[masterControlID]; !ok {
		return zoneProjectionCandidate{}, false
	}

	zoneMemberIPs := make(map[string]string, len(zone.Members))
	for _, zoneMember := range zone.Members {
		deviceID := strings.TrimSpace(zoneMember.DeviceID)
		if deviceID != "" {
			zoneMemberIPs[deviceID] = strings.TrimSpace(zoneMember.IP)
		}
	}

	members := make([]zoneMemberView, 0, zone.GetTotalDeviceCount())
	memberByLogicalID := make(map[string]bool)
	logicalMembers := make([]string, 0, zone.GetTotalDeviceCount())
	physicalMembers := make([]string, 0, zone.GetTotalDeviceCount())
	seenPhysical := make(map[string]bool)
	availableCount := 0
	physicalMemberCount := 0
	degraded := false

	for _, rawDeviceID := range zone.GetAllDeviceIDs() {
		deviceID := strings.TrimSpace(rawDeviceID)
		if deviceID == "" || seenPhysical[deviceID] {
			continue
		}

		seenPhysical[deviceID] = true
		physicalMembers = append(physicalMembers, deviceID)

		logicalID := physicalToLogical[deviceID]
		if logicalID != "" && memberByLogicalID[logicalID] {
			continue
		}

		controlID := logicalID
		if controlID == "" {
			controlID = zoneMemberIPs[deviceID]
		}

		member, memberDegraded := newZoneMember(
			deviceID,
			logicalID,
			controlID,
			devices,
			zoneMemberIPs,
			byDeviceID,
		)

		degraded = degraded || memberDegraded || !member.Available
		if member.Available {
			availableCount++
		}

		members = append(members, member)
		physicalMemberCount += len(member.PhysicalMembers)

		if logicalID != "" {
			memberByLogicalID[logicalID] = true
			logicalMembers = append(logicalMembers, logicalID)
		}
	}

	if len(members) < 2 {
		return zoneProjectionCandidate{}, false
	}

	return zoneProjectionCandidate{
		masterControlID: masterControlID,
		logicalMembers:  logicalMembers,
		physicalMembers: physicalMembers,
		view: &zoneView{
			MasterDeviceID:       masterDeviceID,
			MasterControlID:      masterControlID,
			MemberCount:          len(members),
			PhysicalMemberCount:  physicalMemberCount,
			AvailableMemberCount: availableCount,
			Degraded:             degraded,
			Members:              members,
		},
	}, true
}

func newZoneMember(
	deviceID string,
	logicalID string,
	controlID string,
	devices map[string]deviceView,
	zoneMemberIPs map[string]string,
	byDeviceID map[string][]deviceProjectionEntry,
) (zoneMemberView, bool) {
	member := zoneMemberView{
		Kind:         "speaker",
		ControlID:    controlID,
		IP:           controlID,
		HardwareID:   deviceID,
		DeviceIDs:    []string{deviceID},
		Connectivity: "offline",
		PhysicalMembers: []zonePhysicalMemberView{{
			DeviceID:     deviceID,
			IP:           zoneMemberIPs[deviceID],
			Connectivity: "offline",
		}},
	}

	view, ok := devices[logicalID]
	if !ok {
		return member, true
	}

	if view.Info != nil {
		member.Name = view.Info.Name
		member.Model = view.Info.Type
		member.Type = view.Info.Type
	}

	member.Connectivity = string(projectedConnectivity(view.Status))
	member.Available = member.Connectivity == "online"

	if view.StereoPair != nil {
		member.Kind = "stereoPair"
		member.HardwareID = view.StereoPair.MasterDeviceID
		member.StereoPair = view.StereoPair

		member.DeviceIDs = member.DeviceIDs[:0]
		for _, pairMember := range view.StereoPair.Members {
			member.DeviceIDs = append(member.DeviceIDs, pairMember.DeviceID)
		}

		member.PhysicalMembers = physicalZoneMembers(view.StereoPair, byDeviceID)
	} else {
		member.PhysicalMembers = []zonePhysicalMemberView{
			newZonePhysicalMember(deviceID, "", zoneMemberIPs[deviceID], byDeviceID),
		}
	}

	return member, view.StereoPair != nil && view.StereoPair.Degraded
}

func physicalZoneMembers(
	pair *stereoPairView,
	byDeviceID map[string][]deviceProjectionEntry,
) []zonePhysicalMemberView {
	members := make([]zonePhysicalMemberView, 0, len(pair.Members))
	for _, pairMember := range pair.Members {
		members = append(members, newZonePhysicalMember(
			pairMember.DeviceID,
			pairMember.Role,
			pairMember.IPAddress,
			byDeviceID,
		))
	}

	return members
}

func newZonePhysicalMember(
	deviceID string,
	role string,
	fallbackIP string,
	byDeviceID map[string][]deviceProjectionEntry,
) zonePhysicalMemberView {
	member := zonePhysicalMemberView{
		DeviceID:     deviceID,
		Role:         role,
		IP:           fallbackIP,
		Connectivity: "offline",
	}

	entry, ok := uniqueDeviceEntry(byDeviceID, deviceID)
	if !ok {
		return member
	}

	member.IP = entry.ID
	if entry.Info != nil {
		member.Name = entry.Info.Name
		member.Type = entry.Info.Type
	}

	member.Connectivity = string(projectedConnectivity(entry.Status))
	member.Available = member.Connectivity == "online"

	return member
}

func validMasterGroup(deviceID string, group *models.Group) bool {
	if group == nil || group.IsEmpty() || strings.TrimSpace(group.ID) == "" ||
		strings.TrimSpace(group.MasterDeviceID) == "" || len(group.Roles.Roles) != 2 ||
		strings.TrimSpace(deviceID) != strings.TrimSpace(group.MasterDeviceID) {
		return false
	}

	seenDevices := make(map[string]bool, len(group.Roles.Roles))
	seenRoles := make(map[string]bool, len(group.Roles.Roles))
	masterPresent := false

	for _, role := range group.Roles.Roles {
		memberID := strings.TrimSpace(role.DeviceID)

		memberRole := strings.ToUpper(strings.TrimSpace(role.Role))
		if memberID == "" || seenDevices[memberID] || (memberRole != "LEFT" && memberRole != "RIGHT") || seenRoles[memberRole] {
			return false
		}

		seenDevices[memberID] = true
		seenRoles[memberRole] = true
		masterPresent = masterPresent || memberID == strings.TrimSpace(group.MasterDeviceID)
	}

	return masterPresent && seenRoles["LEFT"] && seenRoles["RIGHT"]
}

func uniqueDeviceEntry(byDeviceID map[string][]deviceProjectionEntry, deviceID string) (deviceProjectionEntry, bool) {
	entries := byDeviceID[strings.TrimSpace(deviceID)]
	if len(entries) != 1 {
		return deviceProjectionEntry{}, false
	}

	return entries[0], true
}

func registeredMembersAgree(group *models.Group, byDeviceID map[string][]deviceProjectionEntry) bool {
	for _, role := range group.Roles.Roles {
		entries := byDeviceID[strings.TrimSpace(role.DeviceID)]
		if len(entries) > 1 {
			return false
		}

		if len(entries) == 0 {
			continue
		}

		if entries[0].Status == nil || !models.SameGroup(group, entries[0].Status.Group) {
			return false
		}
	}

	return true
}

func newStereoPairView(group *models.Group, byDeviceID map[string][]deviceProjectionEntry) *stereoPairView {
	members := make([]stereoPairMemberView, 0, len(group.Roles.Roles))
	available := 0

	for _, role := range group.Roles.Roles {
		member := stereoPairMemberView{
			DeviceID:  strings.TrimSpace(role.DeviceID),
			Role:      strings.ToUpper(strings.TrimSpace(role.Role)),
			IPAddress: role.IPAddress,
		}

		if entry, ok := uniqueDeviceEntry(byDeviceID, role.DeviceID); ok {
			if entry.Info != nil {
				member.Name = entry.Info.Name
				member.IPAddress = projectedIPAddress(entry.ID, entry.Info, member.IPAddress)
			}

			member.Available = projectedConnectivity(entry.Status) != webtypes.ConnectivityOffline
			if member.Available {
				available++
			}
		}

		members = append(members, member)
	}

	return &stereoPairView{
		ID:                   group.ID,
		Name:                 logicalPairName(group.Name, members),
		MasterDeviceID:       group.MasterDeviceID,
		Status:               group.Status,
		MemberCount:          len(members),
		AvailableMemberCount: available,
		Degraded:             available != len(members) || (group.Status != "" && group.Status != "GROUP_OK"),
		Members:              members,
	}
}

func projectedDeviceInfo(controlID string, info *models.DeviceInfo, pair *stereoPairView) *models.DeviceInfo {
	if info == nil {
		return nil
	}

	address := projectedIPAddress(controlID, info, "")

	name := info.Name
	if pair != nil && pair.Name != "" {
		name = pair.Name
	}

	if address == info.IPAddress && name == info.Name {
		return info
	}

	projected := *info
	projected.IPAddress = address
	projected.Name = name

	return &projected
}

func projectedIPAddress(controlID string, info *models.DeviceInfo, fallback string) string {
	candidates := []string{controlID, fallback}
	if info != nil {
		candidates = append([]string{info.IPAddress}, candidates...)
	}

	for _, candidate := range candidates {
		if ip := net.ParseIP(strings.TrimSpace(candidate)); ip != nil {
			return ip.String()
		}
	}

	return ""
}

func projectedConnectivity(status *webtypes.DeviceStatus) webtypes.Connectivity {
	if status == nil {
		return webtypes.ConnectivityOffline
	}

	if status.Connectivity != "" {
		return status.Connectivity
	}

	if status.IsConnected {
		return webtypes.ConnectivityOnline
	}

	return webtypes.ConnectivityOffline
}

func logicalPairName(groupName string, members []stereoPairMemberView) string {
	commonName := ""

	for _, member := range members {
		name := strings.TrimSpace(member.Name)
		if name == "" {
			return groupName
		}

		if commonName == "" {
			commonName = name
			continue
		}

		if !strings.EqualFold(commonName, name) {
			return groupName
		}
	}

	if commonName != "" {
		return commonName
	}

	return groupName
}
