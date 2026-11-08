package soundtouchweb

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/stereopair"
	"github.com/go-chi/chi/v5"
)

// StereoPairLifecycle is the shared mutation boundary used by the web player.
// The concrete coordinator serializes operations and verifies both speakers;
// the interface keeps HTTP tests independent from physical devices.
type StereoPairLifecycle interface {
	Inspect(memberIPAddress string) (stereopair.Result, error)
	Create(req stereopair.CreateRequest) (stereopair.Result, error)
	Rename(req stereopair.RenameRequest) (stereopair.Result, error)
	Dissolve(req stereopair.DissolveRequest) (stereopair.Result, error)
}

type stereoPairRequest struct {
	RightID string        `json:"rightId"`
	GroupID string        `json:"groupId"`
	Name    string        `json:"name"`
	Group   *models.Group `json:"group,omitempty"`
}

type stereoPairResponse struct {
	Operation            string                     `json:"operation"`
	Status               string                     `json:"status"`
	Capable              bool                       `json:"capable"`
	Paired               bool                       `json:"paired"`
	Group                *models.Group              `json:"group,omitempty"`
	Members              []stereoPairMemberResponse `json:"members,omitempty"`
	PersistenceAttempted bool                       `json:"persistenceAttempted,omitempty"`
	PersistenceComplete  bool                       `json:"persistenceComplete,omitempty"`
	PersistenceError     string                     `json:"persistenceError,omitempty"`
}

type stereoPairMemberResponse struct {
	IPAddress            string        `json:"ipAddress"`
	DeviceID             string        `json:"deviceId,omitempty"`
	Reachable            bool          `json:"reachable"`
	Verified             bool          `json:"verified"`
	Group                *models.Group `json:"group,omitempty"`
	PreflightError       string        `json:"preflightError,omitempty"`
	MutationError        string        `json:"mutationError,omitempty"`
	VerificationError    string        `json:"verificationError,omitempty"`
	CompensationError    string        `json:"compensationError,omitempty"`
	CompensationVerified bool          `json:"compensationVerified,omitempty"`
}

// HandleGetStereoPair returns a fresh speaker-backed view of one pair or
// standalone ST10. Cached player projection is deliberately not consulted.
func (app *WebApp) HandleGetStereoPair(w http.ResponseWriter, r *http.Request) {
	host, conn, ok := app.stereoPairDevice(w, r)
	if !ok {
		return
	}

	if !stereoPairCapable(conn.DeviceInfo) {
		app.writeStereoPairResult(w, conn.DeviceInfo, stereopair.Result{
			Operation: stereopair.OperationInspect,
			Status:    stereopair.StatusSucceeded,
		}, nil)

		return
	}

	result, err := app.StereoPairs.Inspect(host)
	app.writeStereoPairResult(w, conn.DeviceInfo, result, err)
}

// HandleCreateStereoPair makes {id} the LEFT/master and rightId the
// RIGHT/member. The coordinator repeats every precondition against both
// physical speakers immediately before mutation.
func (app *WebApp) HandleCreateStereoPair(w http.ResponseWriter, r *http.Request) {
	leftHost, left, ok := app.stereoPairDevice(w, r)
	if !ok {
		return
	}

	var req stereoPairRequest
	if err := decodeStereoPairRequest(r, &req); err != nil {
		app.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.RightID == "" {
		app.sendError(w, "rightId is required", http.StatusBadRequest)
		return
	}

	right, exists := app.GetDevice(req.RightID)
	if !exists || right.DeviceInfo == nil {
		app.sendError(w, "Right speaker not found", http.StatusNotFound)
		return
	}

	rightHost, err := stereoPairIPAddress(req.RightID, right)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusConflict)
		return
	}

	result, err := app.StereoPairs.Create(stereopair.CreateRequest{
		LeftIPAddress:  leftHost,
		RightIPAddress: rightHost,
		Name:           req.Name,
	})
	app.completeStereoPairMutation(w, left.DeviceInfo, result, err)
}

// HandleRenameStereoPair updates the full pair on both physical speakers and
// succeeds only after both fresh reads agree on the new name.
func (app *WebApp) HandleRenameStereoPair(w http.ResponseWriter, r *http.Request) {
	host, conn, ok := app.stereoPairDevice(w, r)
	if !ok {
		return
	}

	var req stereoPairRequest
	if err := decodeStereoPairRequest(r, &req); err != nil {
		app.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		app.sendError(w, "name is required", http.StatusBadRequest)
		return
	}

	if req.GroupID == "" {
		app.sendError(w, "groupId is required", http.StatusBadRequest)
		return
	}

	result, err := app.StereoPairs.Rename(stereopair.RenameRequest{
		MemberIPAddress: host,
		ExpectedGroupID: req.GroupID,
		Name:            req.Name,
	})
	app.completeStereoPairMutation(w, conn.DeviceInfo, result, err)
}

// HandleDissolveStereoPair removes the pair from both preflighted members and
// reports a degraded outcome rather than hiding a partial teardown.
func (app *WebApp) HandleDissolveStereoPair(w http.ResponseWriter, r *http.Request) {
	host, conn, ok := app.stereoPairDevice(w, r)
	if !ok {
		return
	}

	var req stereoPairRequest
	if err := decodeStereoPairRequest(r, &req); err != nil {
		app.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.GroupID == "" {
		app.sendError(w, "groupId is required", http.StatusBadRequest)
		return
	}

	if err := app.validateStereoPairRecoverySnapshot(req.Group); err != nil {
		app.sendError(w, err.Error(), http.StatusConflict)
		return
	}

	result, err := app.StereoPairs.Dissolve(stereopair.DissolveRequest{
		MemberIPAddress: host,
		ExpectedGroupID: req.GroupID,
		ExpectedGroup:   req.Group,
	})
	app.completeStereoPairMutation(w, conn.DeviceInfo, result, err)
}

func (app *WebApp) validateStereoPairRecoverySnapshot(group *models.Group) error {
	if group == nil {
		return nil
	}

	for i := range group.Roles.Roles {
		role := &group.Roles.Roles[i]
		if net.ParseIP(role.IPAddress) == nil {
			return errors.New("recovery snapshot has an invalid member IP address")
		}

		conn, found := app.deviceByStereoPairIPAddress(role.IPAddress)
		if !found || conn == nil || conn.DeviceInfo == nil || conn.DeviceInfo.DeviceID != role.DeviceID {
			return errors.New("recovery snapshot does not match the registered speakers")
		}
	}

	return nil
}

func (app *WebApp) stereoPairDevice(w http.ResponseWriter, r *http.Request) (string, *webtypes.DeviceConnection, bool) {
	deviceID := strings.TrimSpace(chi.URLParam(r, "id"))
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return "", nil, false
	}

	if app.StereoPairs == nil {
		app.sendError(w, "Stereo-pair lifecycle is unavailable", http.StatusServiceUnavailable)
		return "", nil, false
	}

	conn, ok := app.GetDevice(deviceID)
	if !ok || conn.DeviceInfo == nil {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return "", nil, false
	}

	host, err := stereoPairIPAddress(deviceID, conn)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusConflict)
		return "", nil, false
	}

	return host, conn, true
}

func stereoPairIPAddress(deviceID string, conn *webtypes.DeviceConnection) (string, error) {
	if conn != nil && conn.DeviceInfo != nil {
		if ip := strings.TrimSpace(conn.DeviceInfo.IPAddress); net.ParseIP(ip) != nil {
			return ip, nil
		}
	}

	if ip := strings.TrimSpace(deviceID); net.ParseIP(ip) != nil {
		return ip, nil
	}

	return "", errors.New("device has no valid IP address for stereo pairing")
}

func decodeStereoPairRequest(r *http.Request, req *stereoPairRequest) error {
	if err := json.NewDecoder(r.Body).Decode(req); err != nil {
		return errors.New("invalid request body")
	}

	req.RightID = strings.TrimSpace(req.RightID)
	req.GroupID = strings.TrimSpace(req.GroupID)

	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) > 64 {
		return errors.New("name must not exceed 64 characters")
	}

	return nil
}

func (app *WebApp) completeStereoPairMutation(
	w http.ResponseWriter,
	info *models.DeviceInfo,
	result stereopair.Result,
	operationErr error,
) {
	app.applyStereoPairProjection(result)
	baselines := app.stereoPairGroupBaselines(result)
	app.awaitPriorGlobalWebSocketWrites()
	app.writeStereoPairResult(w, info, result, operationErr)
	app.refreshStereoPairMembersAsync(result, baselines)
}

// stereoPairGroupBaselines captures each member's just-applied group
// generation immediately after applyStereoPairProjection, so the async
// follow-up refresh can tell "nothing changed since our projection landed"
// apart from "a fresher event or poll already superseded it."
func (app *WebApp) stereoPairGroupBaselines(result stereopair.Result) map[string]uint64 {
	baselines := make(map[string]uint64, len(result.Members))

	for i := range result.Members {
		if conn, ok := app.deviceByStereoPairIPAddress(result.Members[i].IPAddress); ok && conn != nil {
			baselines[result.Members[i].IPAddress] = conn.GroupGeneration()
		}
	}

	return baselines
}

// applyStereoPairProjection publishes the coordinator's final fresh group
// reads locally before any follow-up poll starts. ApplyGroupEvent invalidates
// older in-flight /getGroup generations, so stale status cannot replace this
// newer lifecycle observation.
func (app *WebApp) applyStereoPairProjection(result stereopair.Result) {
	activity := time.Now()
	changed := false

	for i := range result.Members {
		member := &result.Members[i]
		if member.Group == nil {
			continue
		}

		if conn, ok := app.deviceByStereoPairIPAddress(member.IPAddress); ok && conn != nil {
			if conn.ApplyGroupEvent(member.Group, activity) {
				changed = true
			}
		}
	}

	if changed {
		app.QueueDeviceListBroadcast()
	}
}

func (app *WebApp) refreshStereoPairMembersAsync(result stereopair.Result, baselines map[string]uint64) {
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("Stereo-pair follow-up refresh failed: %v", recovered)
			}
		}()

		app.refreshStereoPairMembers(result, baselines)
	}()
}

func (app *WebApp) refreshStereoPairMembers(result stereopair.Result, baselines map[string]uint64) {
	for i := range result.Members {
		ipAddress := result.Members[i].IPAddress

		if conn, ok := app.deviceByStereoPairIPAddress(ipAddress); ok && conn != nil {
			app.refreshDeviceStatusAfterStereoPairMutation(ipAddress, conn, baselines[ipAddress])
		}
	}

	app.BroadcastDeviceList()
}

func (app *WebApp) deviceByStereoPairIPAddress(ipAddress string) (*webtypes.DeviceConnection, bool) {
	if conn, ok := app.GetDevice(ipAddress); ok {
		return conn, true
	}

	for _, entry := range app.DeviceSnapshot() {
		if entry.Device != nil && entry.Device.DeviceInfo != nil && entry.Device.DeviceInfo.IPAddress == ipAddress {
			return entry.Device, true
		}
	}

	return nil, false
}

func (app *WebApp) writeStereoPairResult(w http.ResponseWriter, info *models.DeviceInfo, result stereopair.Result, operationErr error) {
	data := stereoPairResponse{
		Operation:            string(result.Operation),
		Status:               string(result.Status),
		Capable:              stereoPairCapable(info),
		Paired:               result.Group != nil && !result.Group.IsEmpty(),
		Group:                result.Group,
		Members:              make([]stereoPairMemberResponse, 0, len(result.Members)),
		PersistenceAttempted: result.PersistenceAttempted,
		PersistenceComplete:  result.PersistenceComplete,
		PersistenceError:     errorString(result.PersistenceError),
	}
	for i := range result.Members {
		member := &result.Members[i]
		data.Members = append(data.Members, stereoPairMemberResponse{
			IPAddress:            member.IPAddress,
			DeviceID:             member.DeviceID,
			Reachable:            member.Reachable,
			Verified:             member.Verified,
			Group:                member.Group,
			PreflightError:       errorString(member.PreflightError),
			MutationError:        errorString(member.MutationError),
			VerificationError:    errorString(member.VerificationError),
			CompensationError:    errorString(member.CompensationError),
			CompensationVerified: member.CompensationVerified,
		})
	}

	status := http.StatusOK

	response := webtypes.APIResponse{Success: operationErr == nil, Data: data}
	if operationErr != nil {
		status = stereoPairHTTPStatus(result)
		response.Error = operationErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode stereo-pair response: %v", err)
	}
}

func stereoPairHTTPStatus(result stereopair.Result) int {
	if resultHasStereoPairError(result, stereopair.ErrUnavailable) {
		return http.StatusBadGateway
	}

	if resultHasStereoPairError(result, stereopair.ErrInvalidRequest) {
		return http.StatusBadRequest
	}

	if resultHasStereoPairError(result, stereopair.ErrConflict) {
		return http.StatusConflict
	}

	if result.Status == stereopair.StatusDegraded {
		// Every speaker answered and the read itself worked. success:false
		// and the error text still report the degraded pair; a browser would
		// otherwise log the successful read as a failed request.
		if result.Operation == stereopair.OperationInspect {
			return http.StatusOK
		}

		return http.StatusBadGateway
	}

	return http.StatusConflict
}

func resultHasStereoPairError(result stereopair.Result, target error) bool {
	if errors.Is(result.PersistenceError, target) {
		return true
	}

	for i := range result.Members {
		member := &result.Members[i]
		for _, candidate := range []error{
			member.PreflightError,
			member.MutationError,
			member.VerificationError,
			member.CompensationError,
		} {
			if errors.Is(candidate, target) {
				return true
			}
		}
	}

	return false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
