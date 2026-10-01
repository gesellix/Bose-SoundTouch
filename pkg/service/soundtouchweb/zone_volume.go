package soundtouchweb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

type zoneVolumeMemberResult struct {
	DeviceID  string `json:"deviceId"`
	ControlID string `json:"controlId,omitempty"`
	Name      string `json:"name,omitempty"`
	Before    *int   `json:"before,omitempty"`
	Target    *int   `json:"target,omitempty"`
	Actual    *int   `json:"actual,omitempty"`
	Error     string `json:"error,omitempty"`
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

const (
	zoneVolumeReadbackAttempts   = 3
	zoneVolumeReadbackRetryDelay = 50 * time.Millisecond
)

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

	zoneTopology, err := app.revalidateZone(masterDeviceID)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusConflict)
		return
	}

	result, readable := app.applyZoneVolume(zoneTopology.snapshot.Zone, &zoneTopology, requested)
	if !readable {
		app.sendError(w, "No zone member volume could be read", http.StatusBadGateway)
		return
	}

	result.MasterDeviceID = masterDeviceID
	logZoneVolumePartial(result)
	app.BroadcastDeviceList()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: result}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func (app *WebApp) zoneVolumeLock(masterDeviceID string) *sync.Mutex {
	value, _ := app.zoneVolumeLocks.LoadOrStore(masterDeviceID, &sync.Mutex{})

	lock, ok := value.(*sync.Mutex)
	if !ok {
		panic("zone volume lock has an invalid type")
	}

	return lock
}

func (app *WebApp) revalidateZone(masterDeviceID string) (zoneVolumeTopology, error) {
	masterControlID := app.findIPByHwID(masterDeviceID)

	master, ok := app.GetDevice(masterControlID)
	if !ok || master.Client == nil {
		return zoneVolumeTopology{}, fmt.Errorf("zone master is unavailable")
	}

	generation := master.BeginZoneRefresh()

	zone, err := master.Client.GetZone()
	if err != nil {
		return zoneVolumeTopology{}, fmt.Errorf("refresh zone master: %w", err)
	}

	changed := master.ApplyPolledZone(generation, masterDeviceID, zone)

	snapshot, current := master.SnapshotZoneTopology()
	if !current {
		return zoneVolumeTopology{}, fmt.Errorf("zone topology changed during refresh")
	}

	if zone == nil {
		return zoneVolumeTopology{}, fmt.Errorf("zone master returned no topology")
	}

	if zone.IsStandalone() {
		if changed {
			app.BroadcastDeviceList()
		}

		return zoneVolumeTopology{}, fmt.Errorf("zone has been dissolved")
	}

	if !models.SameZone(snapshot.Zone, zone) {
		return zoneVolumeTopology{}, fmt.Errorf("zone topology changed after refresh")
	}

	return zoneVolumeTopology{controlID: masterControlID, conn: master, snapshot: snapshot}, nil
}

func (app *WebApp) applyZoneVolume(
	zone *models.ZoneInfo,
	zoneTopology *zoneVolumeTopology,
	requested int,
) (zoneVolumeResult, bool) {
	projected, ok := projectZoneInfo(zone, captureDeviceProjectionEntries(app.DeviceSnapshot()))
	if !ok {
		return zoneVolumeResult{Requested: requested}, false
	}

	members := projected.Members
	result := zoneVolumeResult{
		Requested: requested,
		Members:   make([]zoneVolumeMemberResult, len(members)),
	}
	targets := make([]*zoneVolumeTarget, len(members))

	var readWG sync.WaitGroup
	readWG.Add(len(members))

	for index := range members {
		go func() {
			defer readWG.Done()

			projectedMember := &members[index]
			member := zoneVolumeResultForMember(projectedMember)
			member.Before = nil

			conn, ok := app.GetDevice(projectedMember.ControlID)
			if !ok || conn.Client == nil || conn.DeviceInfo == nil ||
				strings.TrimSpace(conn.DeviceInfo.DeviceID) != strings.TrimSpace(projectedMember.HardwareID) {
				member.Error = "speaker unavailable"
				result.Members[index] = member

				return
			}

			groupTopology, confirmed := conn.SnapshotGroupTopology()
			if !volumeTopologyWritable(conn, groupTopology, confirmed) {
				member.Error = "stereo pair master unavailable"
				result.Members[index] = member

				return
			}

			volumeGeneration := conn.BeginVolumeRefresh()

			volume, err := conn.Client.GetVolume()
			if err != nil || volume == nil {
				if err != nil {
					member.Error = fmt.Sprintf("read volume: %v", err)
				} else {
					member.Error = "read volume: empty response"
				}

				result.Members[index] = member

				return
			}

			if !app.applyCurrentVolumeReadback(
				projectedMember.ControlID,
				conn,
				groupTopology,
				zoneTopology,
				volumeGeneration,
				volume,
			) {
				member.Error = "speaker topology or volume state changed while reading volume"
				result.Members[index] = member

				return
			}

			before := volume.ActualVolume
			member.Before = intPointer(before)
			result.Members[index] = member

			targets[index] = &zoneVolumeTarget{
				index:         index,
				controlID:     projectedMember.ControlID,
				conn:          conn,
				groupTopology: groupTopology,
				before:        before,
			}
		}()
	}

	readWG.Wait()

	baseline, targetCount := zoneVolumeBaseline(targets)

	if targetCount == 0 {
		return result, false
	}

	result.Baseline = baseline
	result.Delta = requested - result.Baseline

	partial := targetCount != len(members)

	for _, target := range targets {
		if target == nil {
			continue
		}

		member := &result.Members[target.index]
		level := models.ClampVolumeLevel(target.before + result.Delta)
		member.Target = intPointer(level)

		atTarget, _ := app.applyVolumeTarget(
			member,
			target.controlID,
			target.conn,
			target.groupTopology,
			zoneTopology,
			level,
		)
		if !atTarget {
			partial = true
		}
	}

	result.Partial = partial

	return result, true
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
	result := zoneVolumeMemberResult{
		DeviceID:  member.HardwareID,
		ControlID: member.ControlID,
		Name:      member.Name,
		Before:    member.ActualVolume,
	}
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

	masterDeviceID := strings.TrimSpace(topology.Group.MasterDeviceID)

	return masterDeviceID != "" && masterDeviceID == strings.TrimSpace(conn.DeviceInfo.DeviceID)
}

func volumeTopologyWritable(
	conn *webtypes.DeviceConnection,
	topology webtypes.GroupTopology,
	confirmed bool,
) bool {
	if conn == nil || conn.DeviceInfo == nil {
		return false
	}

	if stereoPairCapable(conn.DeviceInfo) && !confirmed {
		return false
	}

	return authoritativeVolumeTopology(conn, topology)
}

func (app *WebApp) applyVolumeTarget(
	member *zoneVolumeMemberResult,
	controlID string,
	conn *webtypes.DeviceConnection,
	groupTopology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
	level int,
) (bool, bool) {
	atTarget := false
	confirmed := false

	conn.WithVolumeOperation(func() {
		var (
			writeErr         error
			volumeGeneration uint64
		)

		if !app.withCurrentVolumeWrite(controlID, conn, groupTopology, zoneTopology, func() {
			volumeGeneration = conn.BeginVolumeRefresh()
			writeErr = conn.Client.SetVolume(level)
		}) {
			appendZoneVolumeError(member, "speaker topology changed before volume update")
			return
		}

		var (
			volume  *models.Volume
			readErr error
		)

		for attempt := 0; attempt < zoneVolumeReadbackAttempts; attempt++ {
			if attempt > 0 {
				app.waitForVolumeReadbackRetry()

				if !app.volumeTargetCurrent(controlID, conn, groupTopology, zoneTopology) {
					confirmed = false
					break
				}

				volumeGeneration = conn.BeginVolumeRefresh()
			}

			volume, readErr = conn.Client.GetVolume()
			if readErr != nil || volume == nil {
				break
			}

			confirmed = app.applyCurrentVolumeReadback(
				controlID,
				conn,
				groupTopology,
				zoneTopology,
				volumeGeneration,
				volume,
			)
			if confirmed && volume.TargetVolume == level && volume.ActualVolume == level {
				atTarget = true
				break
			}

			if !app.volumeTargetCurrent(controlID, conn, groupTopology, zoneTopology) {
				break
			}
		}

		if readErr != nil || volume == nil {
			if writeErr != nil {
				appendZoneVolumeError(member, fmt.Sprintf("set volume: %v", writeErr))
			}

			if readErr != nil {
				appendZoneVolumeError(member, fmt.Sprintf("readback volume: %v", readErr))
			} else {
				appendZoneVolumeError(member, "readback volume: empty response")
			}

			return
		}

		if !confirmed {
			appendZoneVolumeError(member, "speaker topology or volume state changed during readback")
			return
		}

		member.Actual = intPointer(volume.ActualVolume)

		if atTarget {
			return
		}

		if writeErr != nil {
			appendZoneVolumeError(member, fmt.Sprintf("set volume: %v", writeErr))
		}

		appendZoneVolumeError(member, fmt.Sprintf(
			"readback target %d actual %d does not both match requested %d",
			volume.TargetVolume,
			volume.ActualVolume,
			level,
		))
	})

	return atTarget, confirmed
}

func (app *WebApp) waitForVolumeReadbackRetry() {
	if app.volumeReadbackRetryWait != nil {
		app.volumeReadbackRetryWait(zoneVolumeReadbackRetryDelay)
		return
	}

	time.Sleep(zoneVolumeReadbackRetryDelay)
}

func (app *WebApp) volumeTargetCurrent(
	controlID string,
	conn *webtypes.DeviceConnection,
	topology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
) bool {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	return app.volumeTargetCurrentLocked(controlID, conn, topology, zoneTopology)
}

// volumeTargetCurrentLocked requires devicesMu to be held for reading or
// writing. Keeping that lock through the immediately following mutation makes
// the accepted write or cache merge precede any later registry removal.
func (app *WebApp) volumeTargetCurrentLocked(
	controlID string,
	conn *webtypes.DeviceConnection,
	topology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
) bool {
	return app.devices[controlID] == conn && conn.GroupTopologyCurrent(topology) &&
		authoritativeVolumeTopology(conn, topology) && app.zoneTopologyCurrentLocked(zoneTopology)
}

func (app *WebApp) zoneTopologyCurrentLocked(topology *zoneVolumeTopology) bool {
	return topology == nil ||
		(app.devices[topology.controlID] == topology.conn && topology.conn.ZoneTopologyCurrent(topology.snapshot))
}

// withCurrentVolumeWrite holds the registry read lock across one accepted
// device write. operation must not call a registry method: Go's RWMutex does
// not permit recursive read locking when a writer is pending.
func (app *WebApp) withCurrentVolumeWrite(
	controlID string,
	conn *webtypes.DeviceConnection,
	topology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
	operation func(),
) bool {
	current := false

	withZoneVolumeFence(zoneTopology, func() {
		conn.WithGroupWriteFence(func() {
			app.devicesMu.RLock()
			defer app.devicesMu.RUnlock()

			current = app.volumeTargetCurrentLocked(controlID, conn, topology, zoneTopology)
			if current {
				operation()
			}
		})
	})

	return current
}

// applyCurrentVolumeReadback fences only the local cache merge. The preceding
// network read and any retry wait run without the registry lock.
func (app *WebApp) applyCurrentVolumeReadback(
	controlID string,
	conn *webtypes.DeviceConnection,
	topology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
	volumeGeneration uint64,
	volume *models.Volume,
) bool {
	return app.withCurrentVolumeReadbackMerge(
		controlID,
		conn,
		topology,
		zoneTopology,
		func() bool {
			return conn.ApplyPolledVolume(volumeGeneration, volume)
		},
	)
}

// withCurrentVolumeReadbackMerge keeps the accepted identity check and its
// local cache merge in one registry read-lock lifetime. merge must not call a
// registry method for the same recursive-RLock reason as the write operation.
func (app *WebApp) withCurrentVolumeReadbackMerge(
	controlID string,
	conn *webtypes.DeviceConnection,
	topology webtypes.GroupTopology,
	zoneTopology *zoneVolumeTopology,
	merge func() bool,
) bool {
	applied := false

	withZoneVolumeFence(zoneTopology, func() {
		conn.WithGroupWriteFence(func() {
			app.devicesMu.RLock()
			defer app.devicesMu.RUnlock()

			if app.volumeTargetCurrentLocked(controlID, conn, topology, zoneTopology) {
				applied = merge()
			}
		})
	})

	return applied
}

func withZoneVolumeFence(topology *zoneVolumeTopology, operation func()) {
	if topology == nil {
		operation()
		return
	}

	topology.conn.WithZoneWriteFence(operation)
}

func intPointer(value int) *int {
	return &value
}

func appendZoneVolumeError(member *zoneVolumeMemberResult, message string) {
	if member.Error != "" {
		member.Error += "; "
	}

	member.Error += message
}
