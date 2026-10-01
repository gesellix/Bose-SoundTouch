package soundtouchweb

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

type zoneReadbackExpectation struct {
	description string
	matches     func(*models.ZoneInfo) bool
}

// Reject overlapping local transactions before they can plan from stale state.
// External controllers remain independent; their changes need fresh readback.
func (app *WebApp) beginZoneMutation(w http.ResponseWriter) bool {
	if !app.zoneMutationMu.TryLock() {
		app.sendError(w, "Another zone change is in progress; refresh before retrying", http.StatusConflict)
		return false
	}

	return true
}

func (app *WebApp) revalidateZoneMutation(w http.ResponseWriter, connection *webtypes.DeviceConnection, baseline *models.ZoneInfo) bool {
	current, err := connection.Client.GetZone()
	if err != nil {
		app.sendError(w, "Cannot recheck zone before changing it: "+err.Error(), http.StatusBadGateway)
		return false
	}

	if baseline == nil || current == nil || !models.SameZone(baseline, current) {
		app.sendError(w, "Zone changed while preparing the request; refresh before retrying", http.StatusConflict)
		return false
	}

	return true
}

// A standalone speaker may become a master; a slave cannot mutate its master's
// topology through its own endpoint.
func zoneMutationMaster(zone *models.ZoneInfo, deviceID string) bool {
	deviceID = strings.TrimSpace(deviceID)
	if zone == nil || deviceID == "" {
		return false
	}

	master := strings.TrimSpace(zone.Master)

	return master == deviceID || (master == "" && len(zone.Members) == 0)
}

type pendingZoneMutationReadback struct {
	deviceID   string
	connection *webtypes.DeviceConnection
	generation uint64
	expect     zoneReadbackExpectation
	problem    string
}

type zoneMutationVerificationError struct {
	mutationErr error
	problems    []string
}

func (e *zoneMutationVerificationError) Error() string {
	if e.mutationErr != nil {
		return fmt.Sprintf(
			"zone mutation transport was uncertain (%v); updated topology could not be verified: %s",
			e.mutationErr,
			strings.Join(e.problems, "; "),
		)
	}

	return "zone change was accepted but updated topology could not be verified: " +
		strings.Join(e.problems, "; ")
}

func (app *WebApp) sendZoneMutationResponse(w http.ResponseWriter, err error, successMessage string) {
	var unverified *zoneMutationVerificationError
	if errors.As(err, &unverified) {
		app.sendError(w, err.Error(), http.StatusBadGateway)
		return
	}

	app.sendControlResponse(w, err, successMessage)
}

// prepareZoneMutationReadbacks is the pre-write ordering barrier. It reserves
// every known affected speaker and cached master before the mutation performs
// any network I/O, preventing an older poll from becoming authoritative later.
func (app *WebApp) prepareZoneMutationReadbacks(
	affectedDeviceIDs []string,
	expectations map[string]zoneReadbackExpectation,
	cachedMasterExpectation func(string) zoneReadbackExpectation,
) []pendingZoneMutationReadback {
	affected := make(map[string]struct{}, len(affectedDeviceIDs))

	candidates := make(map[string]zoneReadbackExpectation, len(expectations))
	for deviceID, expectation := range expectations {
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" {
			continue
		}

		affected[deviceID] = struct{}{}
		candidates[deviceID] = expectation
	}

	for _, deviceID := range affectedDeviceIDs {
		if deviceID = strings.TrimSpace(deviceID); deviceID != "" {
			affected[deviceID] = struct{}{}
		}
	}

	snapshot := app.DeviceSnapshot()

	connectionsByDeviceID := make(map[string][]*webtypes.DeviceConnection, len(snapshot))
	for _, entry := range snapshot {
		if entry.Device == nil || entry.Device.DeviceInfo == nil {
			continue
		}

		deviceID := strings.TrimSpace(entry.Device.DeviceInfo.DeviceID)
		if deviceID != "" {
			connectionsByDeviceID[deviceID] = append(connectionsByDeviceID[deviceID], entry.Device)
		}

		status := entry.Device.Status()
		if status == nil || status.Zone == nil {
			continue
		}

		if !zoneContainsAffectedDevice(status.Zone, affected) {
			continue
		}

		masterID := strings.TrimSpace(status.Zone.Master)
		if masterID == "" {
			continue
		}

		if _, exists := candidates[masterID]; !exists {
			candidates[masterID] = cachedMasterExpectation(masterID)
		}
	}

	deviceIDs := make([]string, 0, len(candidates))
	for deviceID := range candidates {
		deviceIDs = append(deviceIDs, deviceID)
	}

	sort.Strings(deviceIDs)

	readbacks := make([]pendingZoneMutationReadback, 0, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		readback := pendingZoneMutationReadback{
			deviceID: deviceID,
			expect:   candidates[deviceID],
		}

		connections := connectionsByDeviceID[deviceID]
		switch {
		case len(connections) == 0:
			readback.problem = "speaker is not registered"
		case len(connections) > 1:
			readback.problem = "speaker registry is ambiguous"
		case connections[0].Client == nil:
			readback.problem = "speaker client is unavailable"
		default:
			readback.connection = connections[0]
			readback.generation = readback.connection.BeginZoneEventRefresh()
		}

		readbacks = append(readbacks, readback)
	}

	return readbacks
}

func zoneContainsAffectedDevice(zone *models.ZoneInfo, affected map[string]struct{}) bool {
	for deviceID := range affected {
		if zone.IsInZone(deviceID) {
			return true
		}
	}

	return false
}

func (app *WebApp) runZoneMutation(
	readbacks []pendingZoneMutationReadback,
	mutate func() error,
) error {
	mutationErr := mutate()

	type readbackResult struct {
		readback pendingZoneMutationReadback
		zone     *models.ZoneInfo
		err      error
	}

	results := make(chan readbackResult, len(readbacks))
	pending := 0
	verified := 0
	problems := make([]string, 0)

	for _, readback := range readbacks {
		if readback.problem != "" {
			problems = append(problems, fmt.Sprintf("%s: %s", readback.deviceID, readback.problem))
			continue
		}

		pending++

		go func(readback pendingZoneMutationReadback) {
			zone, err := readback.connection.Client.GetZone()
			results <- readbackResult{readback: readback, zone: zone, err: err}
		}(readback)
	}

	for range pending {
		result := <-results

		readback := result.readback
		if result.err != nil {
			problems = append(problems, fmt.Sprintf("%s: /getZone failed: %v", readback.deviceID, result.err))
			continue
		}

		if result.zone == nil || readback.expect.matches == nil || !readback.expect.matches(result.zone) {
			problems = append(problems, fmt.Sprintf("%s: expected %s", readback.deviceID, readback.expect.description))
			continue
		}

		if zoneReadbackIsAuthoritative(readback.deviceID, result.zone) {
			accepted, _ := readback.connection.ApplyPolledZoneChanged(
				readback.generation,
				readback.deviceID,
				result.zone,
			)
			if !accepted {
				problems = append(problems, fmt.Sprintf("%s: readback was stale or invalid", readback.deviceID))
				continue
			}

			verified++

			app.BroadcastDeviceList()

			continue
		}

		accepted, changed := readback.connection.ApplyZoneMemberReadback(
			readback.generation,
			readback.deviceID,
			result.zone,
		)
		if !accepted {
			problems = append(problems, fmt.Sprintf("%s: member readback was stale or invalid", readback.deviceID))
			continue
		}

		verified++

		if changed {
			app.BroadcastDeviceList()
		}
	}

	if mutationErr != nil && verified == 0 && len(problems) == 0 {
		problems = append(problems, "no topology readback was available")
	}

	if len(problems) != 0 {
		sort.Strings(problems)
		return &zoneMutationVerificationError{mutationErr: mutationErr, problems: problems}
	}

	// A transport error can mean the response was lost after the speaker
	// committed the write. Matching generation-fenced readbacks are stronger
	// evidence than the missing acknowledgement, so do not replay the mutation.
	return nil
}

func zoneReadbackIsAuthoritative(deviceID string, zone *models.ZoneInfo) bool {
	masterID := strings.TrimSpace(zone.Master)
	return masterID == strings.TrimSpace(deviceID) || (masterID == "" && len(zone.Members) == 0)
}

func zoneDeviceIDs(zone *models.ZoneInfo, fallbackDeviceID string) []string {
	deviceIDs := map[string]struct{}{}

	if zone != nil {
		if masterID := strings.TrimSpace(zone.Master); masterID != "" {
			deviceIDs[masterID] = struct{}{}
		}

		for _, member := range zone.Members {
			if deviceID := strings.TrimSpace(member.DeviceID); deviceID != "" {
				deviceIDs[deviceID] = struct{}{}
			}
		}
	}

	if fallbackDeviceID = strings.TrimSpace(fallbackDeviceID); fallbackDeviceID != "" {
		deviceIDs[fallbackDeviceID] = struct{}{}
	}

	result := make([]string, 0, len(deviceIDs))
	for deviceID := range deviceIDs {
		result = append(result, deviceID)
	}

	sort.Strings(result)

	return result
}

func zoneRequestDeviceIDs(zone *models.ZoneRequest) []string {
	deviceIDs := []string{zone.Master}
	for _, member := range zone.Members {
		deviceIDs = append(deviceIDs, member.DeviceID)
	}

	return uniqueDeviceIDs(deviceIDs)
}

func uniqueDeviceIDs(deviceIDs []string) []string {
	unique := make(map[string]struct{}, len(deviceIDs))
	for _, deviceID := range deviceIDs {
		if deviceID = strings.TrimSpace(deviceID); deviceID != "" {
			unique[deviceID] = struct{}{}
		}
	}

	result := make([]string, 0, len(unique))
	for deviceID := range unique {
		result = append(result, deviceID)
	}

	sort.Strings(result)

	return result
}

func expectZoneMaster(masterID string, requiredDeviceIDs, excludedDeviceIDs []string) zoneReadbackExpectation {
	required := append([]string(nil), requiredDeviceIDs...)
	excluded := append([]string(nil), excludedDeviceIDs...)

	return zoneReadbackExpectation{
		description: "the confirmed master topology",
		matches: func(zone *models.ZoneInfo) bool {
			if zone == nil || strings.TrimSpace(zone.Master) != strings.TrimSpace(masterID) {
				return false
			}

			for _, deviceID := range required {
				if !zone.IsInZone(deviceID) {
					return false
				}
			}

			for _, deviceID := range excluded {
				if zone.IsInZone(deviceID) {
					return false
				}
			}

			return true
		},
	}
}

func expectZoneMember(masterID, deviceID string) zoneReadbackExpectation {
	return zoneReadbackExpectation{
		description: fmt.Sprintf("membership in zone %s", masterID),
		matches: func(zone *models.ZoneInfo) bool {
			return zone != nil && strings.TrimSpace(zone.Master) == strings.TrimSpace(masterID) &&
				zone.IsInZone(deviceID)
		},
	}
}

func expectStandalone(deviceID string) zoneReadbackExpectation {
	return zoneReadbackExpectation{
		description: "standalone topology",
		matches: func(zone *models.ZoneInfo) bool {
			if zone == nil {
				return false
			}

			for _, currentDeviceID := range zoneDeviceIDs(zone, "") {
				if currentDeviceID != deviceID {
					return false
				}
			}

			return true
		},
	}
}

func zoneReadbackDescribesDevice(deviceID string, zone *models.ZoneInfo) bool {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || zone == nil {
		return false
	}

	masterID := strings.TrimSpace(zone.Master)
	if masterID == "" {
		return len(zone.Members) == 0
	}

	return masterID == deviceID || zone.IsMember(deviceID)
}

func expectDevicesAbsent(queriedDeviceID string, deviceIDs []string) zoneReadbackExpectation {
	excluded := append([]string(nil), deviceIDs...)

	return zoneReadbackExpectation{
		description: "affected devices to be absent from the cached zone",
		matches: func(zone *models.ZoneInfo) bool {
			if !zoneReadbackDescribesDevice(queriedDeviceID, zone) {
				return false
			}

			for _, deviceID := range excluded {
				if zone.IsInZone(deviceID) {
					return false
				}
			}

			return true
		},
	}
}

func zoneAddMutationPlan(
	zone *models.ZoneRequest,
	addedDeviceID string,
) ([]string, map[string]zoneReadbackExpectation, func(string) zoneReadbackExpectation) {
	affected := zoneRequestDeviceIDs(zone)
	expectations := make(map[string]zoneReadbackExpectation, len(affected))

	requiredMembers := make([]string, 0, len(affected)-1)
	for _, deviceID := range affected {
		if deviceID != zone.Master {
			requiredMembers = append(requiredMembers, deviceID)
		}
	}

	expectations[zone.Master] = expectZoneMaster(zone.Master, requiredMembers, nil)
	for _, deviceID := range requiredMembers {
		expectations[deviceID] = expectZoneMember(zone.Master, deviceID)
	}

	return affected, expectations, func(candidateDeviceID string) zoneReadbackExpectation {
		return expectDevicesAbsent(candidateDeviceID, []string{addedDeviceID})
	}
}

func zoneRemoveMutationPlan(
	zone *models.ZoneInfo,
	masterID string,
	removedDeviceID string,
) ([]string, map[string]zoneReadbackExpectation, func(string) zoneReadbackExpectation) {
	affected := zoneDeviceIDs(zone, masterID)
	affected = append(affected, removedDeviceID)
	affected = uniqueDeviceIDs(affected)

	remainingMembers := make([]string, 0, len(affected))
	for _, deviceID := range affected {
		if deviceID != masterID && deviceID != removedDeviceID {
			remainingMembers = append(remainingMembers, deviceID)
		}
	}

	expectations := make(map[string]zoneReadbackExpectation, len(affected))
	if len(remainingMembers) == 0 {
		expectations[masterID] = expectStandalone(masterID)
	} else {
		expectations[masterID] = expectZoneMaster(masterID, remainingMembers, []string{removedDeviceID})
		for _, deviceID := range remainingMembers {
			expectations[deviceID] = expectZoneMember(masterID, deviceID)
		}
	}

	expectations[removedDeviceID] = expectStandalone(removedDeviceID)

	return affected, expectations, func(candidateDeviceID string) zoneReadbackExpectation {
		return expectDevicesAbsent(candidateDeviceID, []string{removedDeviceID})
	}
}

func zoneDissolveMutationPlan(
	zone *models.ZoneInfo,
	masterID string,
) ([]string, map[string]zoneReadbackExpectation, func(string) zoneReadbackExpectation) {
	affected := zoneDeviceIDs(zone, masterID)

	expectations := make(map[string]zoneReadbackExpectation, len(affected))
	for _, deviceID := range affected {
		expectations[deviceID] = expectStandalone(deviceID)
	}

	return affected, expectations, func(candidateDeviceID string) zoneReadbackExpectation {
		return expectDevicesAbsent(candidateDeviceID, affected)
	}
}
