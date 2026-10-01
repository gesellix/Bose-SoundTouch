package soundtouchweb

import (
	"encoding/json"
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
		app.sendError(w, err.Error(), http.StatusConflict)
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

	currentProjection, projected := projectZoneInfo(
		zoneTopology.snapshot.Zone,
		captureDeviceProjectionEntries(app.DeviceSnapshot()),
	)

	currentMember, current := findLogicalZoneMember(currentProjection, memberID)
	if !projected || !current || !sameLogicalZoneMember(member, currentMember) {
		app.sendError(w, "Zone member topology changed before volume update", http.StatusConflict)
		return
	}

	member = currentMember

	control, groupTopology, failure := app.zoneMemberVolumeControl(member)
	if failure != "" {
		app.sendError(w, failure, http.StatusConflict)
		return
	}

	result := app.applyZoneMemberVolume(control, member, groupTopology, &zoneTopology, requested)
	app.BroadcastDeviceList()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: result}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (app *WebApp) zoneMemberVolumeControl(
	member zoneMemberView,
) (*webtypes.DeviceConnection, webtypes.GroupTopology, string) {
	control, ok := app.GetDevice(member.ControlID)
	if !ok || control.Client == nil || control.DeviceInfo == nil ||
		strings.TrimSpace(control.DeviceInfo.DeviceID) != strings.TrimSpace(member.HardwareID) {
		return nil, webtypes.GroupTopology{}, "Zone member control target is unavailable"
	}

	groupTopology, confirmed := control.SnapshotGroupTopology()
	if !volumeTopologyWritable(control, groupTopology, confirmed) {
		return nil, webtypes.GroupTopology{}, "Stereo pair master is unavailable"
	}

	return control, groupTopology, ""
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
