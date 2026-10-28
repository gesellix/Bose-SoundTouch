package soundtouchweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

type volumeResultStatus string

const (
	volumeConfirmed  volumeResultStatus = "confirmed"
	volumeUnverified volumeResultStatus = "unverified"
	volumeFailed     volumeResultStatus = "failed"
)

var errVolumeTopologyConflict = errors.New("speaker topology changed before volume update")

const (
	zoneVolumeReadbackAttempts   = 3
	zoneVolumeReadbackRetryDelay = 50 * time.Millisecond
)

// errZoneMasterUnreachable marks a zone check that could not ask the master,
// as opposed to one whose answer showed a changed topology. The first is an
// upstream failure (502), the second a conflict (409).
var errZoneMasterUnreachable = errors.New("zone master is unavailable")

// revalidateZoneStatus maps a revalidateZone error to its HTTP status.
func revalidateZoneStatus(err error) int {
	if errors.Is(err, errZoneMasterUnreachable) {
		return http.StatusBadGateway
	}

	return http.StatusConflict
}

type zoneVolumeMemberResult struct {
	DeviceID  string             `json:"deviceId"`
	ControlID string             `json:"controlId,omitempty"`
	Name      string             `json:"name,omitempty"`
	Before    *int               `json:"before,omitempty"`
	Target    *int               `json:"target,omitempty"`
	Actual    *int               `json:"actual,omitempty"`
	Error     string             `json:"error,omitempty"`
	Status    volumeResultStatus `json:"status"`
	failure   error
}

type zoneVolumeResult struct {
	MasterDeviceID string                   `json:"masterDeviceId"`
	Requested      int                      `json:"requested"`
	Baseline       int                      `json:"baseline"`
	Delta          int                      `json:"delta"`
	Partial        bool                     `json:"partial"`
	Members        []zoneVolumeMemberResult `json:"members"`
}

type zoneVolumeTarget struct {
	index         int
	controlID     string
	conn          *webtypes.DeviceConnection
	groupTopology webtypes.GroupTopology
	before        int
}

type zoneVolumeTopology struct {
	controlID string
	conn      *webtypes.DeviceConnection
	snapshot  webtypes.ZoneTopology
}

// HandleZoneVolume moves every current logical member by one shared delta.
func (app *WebApp) HandleZoneVolume(w http.ResponseWriter, r *http.Request) {
	controlID := chi.URLParam(r, "id")

	requested, err := strconv.Atoi(chi.URLParam(r, "volume"))
	if err != nil || !models.ValidateVolumeLevel(requested) {
		app.sendError(w, "Invalid volume level (0-100)", http.StatusBadRequest)
		return
	}

	view, ok := app.deviceViewSnapshot()[controlID]
	if !ok {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if view.Zone == nil {
		app.sendError(w, "Device is not a logical zone master", http.StatusConflict)
		return
	}

	masterDeviceID := strings.TrimSpace(view.Zone.MasterDeviceID)
	lock := app.zoneVolumeLock(masterDeviceID)
	lock.Lock()
	defer lock.Unlock()

	topology, err := app.revalidateZone(masterDeviceID)
	if err != nil {
		app.sendError(w, err.Error(), revalidateZoneStatus(err))
		return
	}

	if topology.controlID != controlID {
		app.sendError(w, "Zone master identity changed", http.StatusConflict)
		return
	}

	result, _ := app.applyZoneVolume(topology.snapshot.Zone, &topology, requested)
	result.MasterDeviceID = masterDeviceID
	logZoneVolumePartial(result)
	app.BroadcastDeviceList()
	app.sendVolumeResults(w, result, result.Members)
}

func (app *WebApp) zoneVolumeLock(masterDeviceID string) *sync.Mutex {
	value, _ := app.zoneVolumeLocks.LoadOrStore(masterDeviceID, &sync.Mutex{})

	lock, ok := value.(*sync.Mutex)
	if !ok {
		panic("zone volume lock has an invalid type")
	}

	return lock
}

func (app *WebApp) sendVolumeResults(w http.ResponseWriter, data interface{}, members []zoneVolumeMemberResult) {
	status, success := volumeResponseStatus(members)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := webtypes.APIResponse{Success: success, Data: data}
	if !success {
		response.Error = "No member volume update succeeded"
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func volumeResponseStatus(members []zoneVolumeMemberResult) (int, bool) {
	conflict := false

	for _, member := range members {
		if member.Status == volumeConfirmed || member.Status == volumeUnverified {
			return http.StatusOK, true
		}

		conflict = conflict || errors.Is(member.failure, errVolumeTopologyConflict)
	}

	if conflict {
		return http.StatusConflict, false
	}

	return http.StatusBadGateway, false
}

func (app *WebApp) revalidateZone(masterDeviceID string) (zoneVolumeTopology, error) {
	controlID := app.findIPByHwID(masterDeviceID)

	master, ok := app.GetDevice(controlID)
	if !ok || master.Client == nil {
		return zoneVolumeTopology{}, errZoneMasterUnreachable
	}

	generation := master.BeginZoneRefresh()

	zone, err := master.Client.GetZone()
	if err != nil {
		return zoneVolumeTopology{}, fmt.Errorf("%w: refresh zone master: %w", errZoneMasterUnreachable, err)
	}

	changed := master.ApplyPolledZone(generation, masterDeviceID, zone)

	snapshot, current := master.SnapshotZoneTopology()
	if !current || zone == nil || !models.SameZone(snapshot.Zone, zone) {
		return zoneVolumeTopology{}, fmt.Errorf("zone topology changed during refresh")
	}

	if zone.IsStandalone() {
		if changed {
			app.BroadcastDeviceList()
		}

		return zoneVolumeTopology{}, fmt.Errorf("zone has been dissolved")
	}

	return zoneVolumeTopology{controlID: controlID, conn: master, snapshot: snapshot}, nil
}

func (app *WebApp) applyZoneVolume(zone *models.ZoneInfo, topology *zoneVolumeTopology, requested int) (zoneVolumeResult, bool) {
	projected, ok := projectZoneInfo(zone, captureDeviceProjectionEntries(app.DeviceSnapshot()))
	if !ok {
		return zoneVolumeResult{Requested: requested, Partial: true}, false
	}

	result := zoneVolumeResult{Requested: requested, Members: make([]zoneVolumeMemberResult, len(projected.Members))}
	targets := make([]*zoneVolumeTarget, len(projected.Members))
	// Capture every reachable member from fresh parallel reads before computing
	// the shared delta. Cached projections can lag the preceding volume move.
	var reads sync.WaitGroup
	for index := range projected.Members {
		reads.Add(1)
		go func() {
			defer reads.Done()

			member := &projected.Members[index]
			result.Members[index] = zoneVolumeResultForMember(member)
			result.Members[index].Before = nil

			conn, group, failure := app.zoneMemberVolumeControl(*member)
			if failure != nil {
				result.Members[index].Error = failure.Error()
				result.Members[index].failure = failure
				result.Members[index].Before = nil

				return
			}

			volume, err := conn.Client.GetVolume()
			if err != nil || volume == nil || !models.ValidateVolumeLevel(volume.ActualVolume) {
				result.Members[index].Error = "member volume read unavailable"
				return
			}

			if !app.volumeTargetCurrent(member.ControlID, conn, group, topology) {
				result.Members[index].failure = errVolumeTopologyConflict
				result.Members[index].Error = "speaker topology changed while reading volume"

				return
			}

			result.Members[index].Before = intPointer(volume.ActualVolume)
			targets[index] = &zoneVolumeTarget{index: index, controlID: member.ControlID, conn: conn, groupTopology: group, before: volume.ActualVolume}
		}()
	}

	reads.Wait()

	result.Baseline, _ = zoneVolumeBaseline(targets)
	result.Delta = requested - result.Baseline

	var writes sync.WaitGroup

	for _, target := range targets {
		if target == nil {
			continue
		}

		writes.Add(1)
		go func() {
			defer writes.Done()

			member := &result.Members[target.index]
			level := models.ClampVolumeLevel(target.before + result.Delta)
			member.Target = intPointer(level)
			app.applyVolumeTarget(member, target.controlID, target.conn, target.groupTopology, topology, level)
		}()
	}

	writes.Wait()

	for _, member := range result.Members {
		result.Partial = result.Partial || member.Status != volumeConfirmed
	}

	_, success := volumeResponseStatus(result.Members)

	return result, success
}

func zoneVolumeBaseline(targets []*zoneVolumeTarget) (baseline, count int) {
	for _, target := range targets {
		if target == nil {
			continue
		}

		count++

		if target.before > baseline {
			baseline = target.before
		}
	}

	return baseline, count
}

func zoneVolumeResultForMember(member *zoneMemberView) zoneVolumeMemberResult {
	result := zoneVolumeMemberResult{DeviceID: member.HardwareID, ControlID: member.ControlID, Name: member.Name, Before: member.ActualVolume, Status: volumeFailed}
	if result.DeviceID == "" && len(member.DeviceIDs) != 0 {
		result.DeviceID = member.DeviceIDs[0]
	}

	return result
}

func authoritativeVolumeTopology(conn *webtypes.DeviceConnection, topology webtypes.GroupTopology) bool {
	if conn == nil || conn.DeviceInfo == nil {
		return false
	}

	if topology.Group == nil {
		return true
	}

	master := strings.TrimSpace(topology.Group.MasterDeviceID)

	return master != "" && master == strings.TrimSpace(conn.DeviceInfo.DeviceID)
}

func volumeTopologyWritable(conn *webtypes.DeviceConnection, topology webtypes.GroupTopology, confirmed bool) bool {
	return conn != nil && conn.DeviceInfo != nil &&
		(!stereoPairCapable(conn.DeviceInfo) || confirmed) && authoritativeVolumeTopology(conn, topology)
}

func uncertainVolumeWrite(err error) bool {
	var (
		apiError     *models.APIError
		deviceErrors *models.ErrorsResponse
	)
	if errors.As(err, &apiError) || errors.As(err, &deviceErrors) {
		return false
	}
	// A connection that was never established cannot have applied anything.
	var opError *net.OpError
	if errors.As(err, &opError) && opError.Op == "dial" {
		return false
	}
	// The shared client does not type every HTTP or transport error. Without
	// an explicit firmware rejection, a write may already have taken effect.
	return true
}

func (app *WebApp) applyVolumeTarget(member *zoneVolumeMemberResult, controlID string, conn *webtypes.DeviceConnection, group webtypes.GroupTopology, zone *zoneVolumeTopology, level int) (bool, bool) {
	member.Status = volumeFailed
	atTarget, merged := false, false

	conn.WithVolumeOperation(func() {
		if !app.volumeTargetCurrent(controlID, conn, group, zone) {
			member.failure = errVolumeTopologyConflict
			member.Error = member.failure.Error()

			return
		}
		// Admission and cache guards cover local state only. The independent
		// volume operation mutex may span I/O without blocking device events.
		writeErr := conn.Client.SetVolume(level)
		if writeErr != nil && !uncertainVolumeWrite(writeErr) {
			member.failure = writeErr
			member.Error = fmt.Sprintf("set volume: %v", writeErr)

			return
		}

		member.Status = volumeUnverified

		for attempt := range zoneVolumeReadbackAttempts {
			if attempt > 0 {
				app.waitForVolumeReadbackRetry()

				if !app.volumeTargetCurrent(controlID, conn, group, zone) {
					merged = false
					member.Actual = nil
					member.Error = "Volume update unverified: newer topology or volume state"

					return
				}
			}
			// Each retry gets its own reservation so an event that invalidates
			// one read does not invalidate all subsequent convergence reads.
			generation := conn.BeginVolumeRefresh()

			volume, readErr := conn.Client.GetVolume()
			if readErr != nil || volume == nil ||
				!models.ValidateVolumeLevel(volume.TargetVolume) || !models.ValidateVolumeLevel(volume.ActualVolume) {
				merged = false
				member.Actual = nil
				member.Error = "Volume update unverified: readback unavailable"

				return
			}

			merged = app.applyCurrentVolumeReadback(controlID, conn, group, zone, generation, volume)

			member.Actual = nil
			if merged {
				member.Actual = intPointer(volume.ActualVolume)
				if volume.TargetVolume == level && volume.ActualVolume == level {
					member.Status = volumeConfirmed
					member.Error = ""
					atTarget = true

					return
				}

				member.Error = fmt.Sprintf("Volume update unverified: target %d actual %d does not both match requested %d", volume.TargetVolume, volume.ActualVolume, level)
			} else {
				member.Error = "Volume update unverified: newer topology or volume state"
			}

			if !app.volumeTargetCurrent(controlID, conn, group, zone) {
				return
			}
		}
	})

	return atTarget, merged
}

func (app *WebApp) waitForVolumeReadbackRetry() {
	if app.volumeReadbackRetryWait != nil {
		app.volumeReadbackRetryWait(zoneVolumeReadbackRetryDelay)
		return
	}

	time.Sleep(zoneVolumeReadbackRetryDelay)
}

func (app *WebApp) volumeTargetCurrent(controlID string, conn *webtypes.DeviceConnection, group webtypes.GroupTopology, zone *zoneVolumeTopology) bool {
	current := false

	app.withCurrentVolumeReadbackMerge(controlID, conn, group, zone, func() bool {
		current = true
		return true
	})

	return current
}

func (app *WebApp) applyCurrentVolumeReadback(controlID string, conn *webtypes.DeviceConnection, group webtypes.GroupTopology, zone *zoneVolumeTopology, generation uint64, volume *models.Volume) bool {
	return app.withCurrentVolumeReadbackMerge(controlID, conn, group, zone, func() bool {
		return conn.ApplyPolledVolume(generation, volume)
	})
}

// These guards protect only the local identity check and cache merge.
func (app *WebApp) withCurrentVolumeReadbackMerge(controlID string, conn *webtypes.DeviceConnection, group webtypes.GroupTopology, zone *zoneVolumeTopology, merge func() bool) bool {
	applied := false

	operation := func() {
		conn.WithGroupWriteFence(func() {
			app.devicesMu.RLock()
			defer app.devicesMu.RUnlock()

			if app.devices[controlID] != conn || !conn.GroupTopologyCurrent(group) ||
				!authoritativeVolumeTopology(conn, group) {
				return
			}

			if zone != nil && (app.devices[zone.controlID] != zone.conn || !zone.conn.ZoneTopologyCurrent(zone.snapshot)) {
				return
			}

			applied = merge()
		})
	}
	if zone == nil {
		operation()
	} else {
		zone.conn.WithZoneWriteFence(operation)
	}

	return applied
}

func intPointer(value int) *int {
	return &value
}
