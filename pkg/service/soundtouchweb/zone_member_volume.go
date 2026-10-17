package soundtouchweb

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

type zoneMemberVolumeResult struct {
	Requested int                      `json:"requested"`
	ControlID string                   `json:"controlId"`
	Partial   bool                     `json:"partial"`
	Members   []zoneVolumeMemberResult `json:"members"`
}

// HandleZoneMemberVolume sets one current logical zone member through its
// authoritative standalone or confirmed stereo-master control target.
func (app *WebApp) HandleZoneMemberVolume(w http.ResponseWriter, r *http.Request) {
	zoneMasterID := chi.URLParam(r, "id")
	memberID := chi.URLParam(r, "memberId")

	requested, err := strconv.Atoi(chi.URLParam(r, "volume"))
	if err != nil || !models.ValidateVolumeLevel(requested) {
		app.sendError(w, "Invalid volume level (0-100)", http.StatusBadRequest)
		return
	}

	initialMaster, ok := app.deviceViewSnapshot()[zoneMasterID]
	if !ok {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if initialMaster.Zone == nil {
		app.sendError(w, "Device is not a logical zone master", http.StatusConflict)
		return
	}

	initialMember, ok := findLogicalZoneMember(initialMaster.Zone, memberID)
	if !ok {
		app.sendError(w, "Device is not a current logical zone member", http.StatusConflict)
		return
	}

	masterDeviceID := strings.TrimSpace(initialMaster.Zone.MasterDeviceID)
	lock := app.zoneVolumeLock(masterDeviceID)
	lock.Lock()
	defer lock.Unlock()

	zoneTopology, err := app.revalidateZone(masterDeviceID)
	if err != nil {
		app.sendError(w, err.Error(), revalidateZoneStatus(err))
		return
	}

	projection, projected := projectZoneInfo(
		zoneTopology.snapshot.Zone,
		captureDeviceProjectionEntries(app.DeviceSnapshot()),
	)
	if !projected || projection.MasterControlID != zoneMasterID {
		app.sendError(w, "Zone topology changed during refresh", http.StatusConflict)
		return
	}

	member, ok := findLogicalZoneMember(projection, memberID)
	if !ok || !sameLogicalZoneMember(initialMember, member) {
		app.sendError(w, "Zone member topology changed during refresh", http.StatusConflict)
		return
	}

	control, groupTopology, failure := app.zoneMemberVolumeControl(member)
	if failure != nil {
		result := zoneMemberVolumeResult{
			Requested: requested, ControlID: member.ControlID, Partial: true,
			Members: []zoneVolumeMemberResult{zoneVolumeResultForMember(&member)},
		}
		result.Members[0].Error = failure.Error()
		result.Members[0].failure = failure
		app.sendVolumeResults(w, result, result.Members)

		return
	}

	result := app.applyZoneMemberVolume(control, member, groupTopology, &zoneTopology, requested)
	app.BroadcastDeviceList()

	app.sendVolumeResults(w, result, result.Members)
}

func (app *WebApp) zoneMemberVolumeControl(
	member zoneMemberView,
) (*webtypes.DeviceConnection, webtypes.GroupTopology, error) {
	control, ok := app.GetDevice(member.ControlID)
	if !ok || control.Client == nil || control.DeviceInfo == nil ||
		strings.TrimSpace(control.DeviceInfo.DeviceID) != strings.TrimSpace(member.HardwareID) {
		return nil, webtypes.GroupTopology{}, errors.New("zone member control target is unavailable")
	}

	groupTopology, confirmed := control.SnapshotGroupTopology()
	if !logicalVolumeTopologyMatches(member, groupTopology.Group) {
		return nil, webtypes.GroupTopology{}, errVolumeTopologyConflict
	}

	if !volumeTopologyWritable(control, groupTopology, confirmed) {
		return nil, webtypes.GroupTopology{}, errors.New("stereo pair master is unavailable")
	}

	return control, groupTopology, nil
}

func logicalVolumeTopologyMatches(member zoneMemberView, group *models.Group) bool {
	if member.Kind != "stereoPair" {
		return group == nil
	}

	pair := member.StereoPair
	if pair == nil || group == nil || pair.ID != group.ID ||
		strings.TrimSpace(pair.MasterDeviceID) != strings.TrimSpace(group.MasterDeviceID) ||
		len(pair.Members) != len(group.Roles.Roles) {
		return false
	}

	for _, physical := range pair.Members {
		matched := false

		for _, role := range group.Roles.Roles {
			if strings.TrimSpace(physical.DeviceID) == strings.TrimSpace(role.DeviceID) &&
				strings.EqualFold(strings.TrimSpace(physical.Role), strings.TrimSpace(role.Role)) {
				matched = true
				break
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

func findLogicalZoneMember(zone *zoneView, memberID string) (zoneMemberView, bool) {
	if zone == nil {
		return zoneMemberView{}, false
	}

	memberID = strings.TrimSpace(memberID)

	for index := range zone.Members {
		member := &zone.Members[index]
		if member.ControlID == memberID || member.HardwareID == memberID {
			return *member, true
		}
	}

	return zoneMemberView{}, false
}

func sameLogicalZoneMember(left, right zoneMemberView) bool {
	if left.Kind != right.Kind || left.ControlID != right.ControlID ||
		strings.TrimSpace(left.HardwareID) != strings.TrimSpace(right.HardwareID) ||
		len(left.DeviceIDs) != len(right.DeviceIDs) {
		return false
	}

	for index := range left.DeviceIDs {
		if strings.TrimSpace(left.DeviceIDs[index]) != strings.TrimSpace(right.DeviceIDs[index]) {
			return false
		}
	}

	return true
}

func (app *WebApp) applyZoneMemberVolume(
	control *webtypes.DeviceConnection,
	member zoneMemberView,
	groupTopology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
	requested int,
) zoneMemberVolumeResult {
	result := zoneMemberVolumeResult{
		Requested: requested,
		ControlID: member.ControlID,
		Members:   []zoneVolumeMemberResult{zoneVolumeResultForMember(&member)},
	}
	result.Members[0].Target = intPointer(requested)
	atTarget, _ := app.applyVolumeTarget(
		&result.Members[0],
		member.ControlID,
		control,
		groupTopology,
		zoneTopology,
		requested,
	)
	result.Partial = !atTarget

	return result
}
