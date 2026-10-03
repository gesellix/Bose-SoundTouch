package soundtouchweb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

const (
	acousticDeviceTargetHeader = "X-AfterTouch-Acoustic-Device"
	acousticGroupTargetHeader  = "X-AfterTouch-Acoustic-Group"
)

type acousticControlResult struct {
	Message         string  `json:"message,omitempty"`
	Requested       int     `json:"requested"`
	Target          *int    `json:"target"`
	Actual          *int    `json:"actual"`
	AtTarget        bool    `json:"atTarget"`
	Epoch           int64   `json:"epoch"`
	Revision        *uint64 `json:"revision,omitempty"`
	BassRevision    uint64  `json:"bassRevision"`
	BalanceRevision uint64  `json:"balanceRevision"`
}

type acousticControlOperation struct {
	result    acousticControlResult
	status    int
	err       string
	broadcast bool
}

func newAcousticControlOperation(requested int, message string, status *webtypes.DeviceStatus) *acousticControlOperation {
	result := acousticControlResult{Message: message, Requested: requested}
	if status != nil {
		result.Epoch = status.Epoch
		result.BassRevision = status.BassRevision
		result.BalanceRevision = status.BalanceRevision
	}

	return &acousticControlOperation{result: result, status: http.StatusOK}
}

func (operation *acousticControlOperation) fail(status int, message string) {
	operation.status = status
	operation.err = message
	operation.result.Message = ""
}

func writeAcousticControlResponse(w http.ResponseWriter, operation *acousticControlOperation) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(operation.status)

	response := webtypes.APIResponse{
		Success: operation.err == "",
		Data:    operation.result,
		Error:   operation.err,
	}
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode acoustic control response", http.StatusInternalServerError)
	}
}

func acousticDeviceID(conn *webtypes.DeviceConnection) string {
	if conn == nil || conn.DeviceInfo == nil {
		return ""
	}

	return strings.TrimSpace(conn.DeviceInfo.DeviceID)
}

func admitAcousticDevice(r *http.Request, conn *webtypes.DeviceConnection) (string, bool) {
	actual := acousticDeviceID(conn)

	requested := strings.TrimSpace(r.Header.Get(acousticDeviceTargetHeader))
	if actual == "" || (requested != "" && requested != actual) {
		return actual, false
	}

	return actual, true
}

func (app *WebApp) connectionCurrentForDevice(
	controlID string,
	conn *webtypes.DeviceConnection,
	expectedDeviceID string,
) bool {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	return app.devices[controlID] == conn && acousticDeviceID(conn) == expectedDeviceID
}

// applyAcousticRead keeps registry admission and the field merge atomic with
// respect to removal/replacement. Callers publish only after this returns.
func (app *WebApp) applyAcousticRead(controlID string, conn *webtypes.DeviceConnection, hardwareID string, merge func() bool) bool {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	if app.devices[controlID] != conn || acousticDeviceID(conn) != hardwareID {
		return false
	}

	select {
	case <-conn.Done():
		return false
	default:
	}

	return merge()
}

func validBassCapabilities(capabilities *models.BassCapabilities, expectedDeviceID string) error {
	if capabilities == nil {
		return fmt.Errorf("bass capability is unknown")
	}

	if strings.TrimSpace(capabilities.DeviceID) != expectedDeviceID {
		return fmt.Errorf("bass capability belongs to a different device")
	}

	if !capabilities.BassAvailable {
		return fmt.Errorf("bass control is unavailable")
	}

	if capabilities.BassMin > capabilities.BassMax ||
		capabilities.BassDefault < capabilities.BassMin ||
		capabilities.BassDefault > capabilities.BassMax ||
		!models.ValidateBassLevel(capabilities.BassMin) ||
		!models.ValidateBassLevel(capabilities.BassMax) ||
		!models.ValidateBassLevel(capabilities.BassDefault) {
		return fmt.Errorf("bass capability range is invalid")
	}

	return nil
}

func validBassCapabilityProjection(capabilities *models.BassCapabilities, expectedDeviceID string) error {
	if capabilities == nil || strings.TrimSpace(capabilities.DeviceID) != expectedDeviceID {
		return fmt.Errorf("bass capability belongs to a different device")
	}

	if !capabilities.BassAvailable {
		return nil
	}

	return validBassCapabilities(capabilities, expectedDeviceID)
}

func validBassReadback(
	bass *models.Bass,
	capabilities *models.BassCapabilities,
	expectedDeviceID string,
) error {
	if bass == nil || strings.TrimSpace(bass.DeviceID) != expectedDeviceID {
		return fmt.Errorf("bass readback belongs to a different device")
	}

	if err := validBassCapabilities(capabilities, expectedDeviceID); err != nil {
		return err
	}

	if !models.ValidateBassLevel(bass.TargetBass) || !models.ValidateBassLevel(bass.ActualBass) ||
		!capabilities.ValidateLevel(bass.TargetBass) || !capabilities.ValidateLevel(bass.ActualBass) {
		return fmt.Errorf("bass readback is outside the advertised capability range")
	}

	return nil
}

func (app *WebApp) handleBassControl(
	w http.ResponseWriter,
	r *http.Request,
	device *webtypes.DeviceConnection,
) {
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required for bass control", http.StatusMethodNotAllowed)
		return
	}

	var request webtypes.BassRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		app.sendError(w, "Invalid bass data", http.StatusBadRequest)
		return
	}

	if !models.ValidateBassLevel(request.Level) {
		app.sendError(w, "Invalid bass level (must be between -9 and 9)", http.StatusBadRequest)
		return
	}

	expectedDeviceID, admitted := admitAcousticDevice(r, device)

	operation := newAcousticControlOperation(request.Level, fmt.Sprintf("Bass set to %d", request.Level), device.Status())
	if !admitted {
		operation.fail(http.StatusConflict, "Bass target device changed")
		writeAcousticControlResponse(w, operation)

		return
	}

	release := app.beginDeviceSettingsOperation(device)
	defer release()

	device.WithBassOperation(func() {
		if err := r.Context().Err(); err != nil {
			operation.fail(http.StatusRequestTimeout, err.Error())
			return
		}

		app.runBassControl(chi.URLParam(r, "id"), device, expectedDeviceID, request.Level, operation)
	})

	if operation.broadcast {
		app.BroadcastDeviceList()
	}

	writeAcousticControlResponse(w, operation)
}

func (app *WebApp) runBassControl(
	controlID string,
	conn *webtypes.DeviceConnection,
	expectedDeviceID string,
	requested int,
	operation *acousticControlOperation,
) {
	if conn.Client == nil || !app.connectionCurrentForDevice(controlID, conn, expectedDeviceID) {
		operation.fail(http.StatusConflict, "Bass target device changed")
		return
	}

	capabilities, admitted := app.admitBassCapabilities(controlID, conn, expectedDeviceID, operation)
	if !admitted {
		return
	}

	if err := validBassCapabilities(capabilities, expectedDeviceID); err != nil {
		operation.fail(http.StatusConflict, err.Error())
		return
	}

	if !models.ValidateBassLevel(requested) || !capabilities.ValidateLevel(requested) {
		operation.fail(http.StatusBadRequest, fmt.Sprintf(
			"Invalid bass level (must be between %d and %d)", capabilities.BassMin, capabilities.BassMax,
		))

		return
	}

	current, writeErr := app.writeBassForTarget(controlID, conn, expectedDeviceID, requested, capabilities)
	if !current {
		operation.fail(http.StatusConflict, "Device registration or bass capabilities changed")
		return
	}

	generation, current := conn.ReserveBassRead(capabilities)
	if !current {
		app.refreshBass(controlID, conn)
		operation.fail(http.StatusConflict, "Bass capabilities changed during the write; outcome is unverified")

		return
	}

	bass, readErr := conn.Client.GetBass()
	if readErr != nil || validBassReadback(bass, capabilities, expectedDeviceID) != nil {
		message := "Bass readback is unverified"
		if readErr != nil {
			message = fmt.Sprintf("Bass readback is unverified: %v", readErr)
		}

		if writeErr != nil {
			message = fmt.Sprintf("Bass write and readback are unverified: write: %v; read: %v", writeErr, readErr)
		}

		operation.fail(http.StatusBadGateway, message)

		return
	}

	if !app.connectionCurrentForDevice(controlID, conn, expectedDeviceID) {
		operation.fail(http.StatusConflict, "Device registration changed during bass update")
		return
	}

	applied := app.applyAcousticRead(controlID, conn, expectedDeviceID, func() bool {
		return conn.CompleteBassRead(generation, capabilities, bass)
	})
	if !applied {
		operation.fail(http.StatusConflict, "A newer bass readback superseded this update")
		return
	}

	status := conn.Status()
	operation.broadcast = true
	operation.result.Target = intPointer(bass.TargetBass)
	operation.result.Actual = intPointer(bass.ActualBass)
	operation.result.AtTarget = bass.TargetBass == requested && bass.ActualBass == requested
	operation.result.Revision = uint64Pointer(generation)
	operation.result.BassRevision = generation

	operation.result.BalanceRevision = status.BalanceRevision
	if !operation.result.AtTarget {
		operation.fail(http.StatusConflict, fmt.Sprintf(
			"Bass readback did not reach requested level %d (target %d, actual %d)",
			requested, bass.TargetBass, bass.ActualBass,
		))
	}
}

func (app *WebApp) handleBalanceControl(
	w http.ResponseWriter,
	r *http.Request,
	device *webtypes.DeviceConnection,
) {
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required for balance control", http.StatusMethodNotAllowed)
		return
	}

	var request webtypes.BalanceRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		app.sendError(w, "Invalid balance data", http.StatusBadRequest)
		return
	}

	expectedDeviceID, admitted := admitAcousticDevice(r, device)
	target, targetOK := device.ConfirmedBalanceTarget()
	requestedGroupID := strings.TrimSpace(r.Header.Get(acousticGroupTargetHeader))
	wsClient := device.CurrentWebSocket()

	operation := newAcousticControlOperation(request.Level, fmt.Sprintf("Balance set to %d", request.Level), device.Status())
	if !admitted || !targetOK || target.HardwareID != expectedDeviceID ||
		(requestedGroupID != "" && requestedGroupID != target.GroupID) {
		operation.fail(http.StatusConflict, "Device is not the master of a confirmed stereo pair")
		writeAcousticControlResponse(w, operation)

		return
	}

	if wsClient == nil || !wsClient.IsConnected() {
		operation.fail(http.StatusServiceUnavailable, "Balance needs a live WebSocket to the speaker, and none is connected")
		writeAcousticControlResponse(w, operation)

		return
	}

	release := app.beginDeviceSettingsOperation(device)
	defer release()

	requestContext := r.Context()

	device.WithBalanceOperation(func() {
		app.runBalanceControl(
			requestContext, chi.URLParam(r, "id"), device, wsClient, target, request.Level, operation,
		)
	})

	if operation.broadcast {
		app.BroadcastDeviceList()
	}

	writeAcousticControlResponse(w, operation)
}

func (app *WebApp) runBalanceControl(
	requestContext context.Context,
	controlID string,
	conn *webtypes.DeviceConnection,
	wsClient *client.WebSocketClient,
	target webtypes.BalanceTarget,
	requested int,
	operation *acousticControlOperation,
) {
	if conn.Client == nil || !app.connectionCurrentForDevice(controlID, conn, target.HardwareID) ||
		!conn.BalanceTargetCurrent(target) {
		operation.fail(http.StatusConflict, "Stereo pair topology changed")
		return
	}

	current := target.Balance
	if err := validBalanceReadback(current, target.HardwareID); err != nil {
		readTarget, generation, ok := conn.BeginBalanceRead()
		if !ok || readTarget.HardwareID != target.HardwareID || readTarget.GroupID != target.GroupID {
			operation.fail(http.StatusConflict, "Stereo balance capability is unknown or unavailable")
			return
		}

		var readErr error

		current, readErr = wsClient.GetBalance(requestContext)
		if readErr != nil || validBalanceReadback(current, target.HardwareID) != nil {
			operation.fail(http.StatusBadGateway, fmt.Sprintf("Stereo balance readback is unverified: %v", readErr))
			return
		}

		if !app.applyAcousticRead(controlID, conn, target.HardwareID, func() bool {
			_, applied := conn.ApplyBalanceRead(readTarget, generation, current)
			return applied
		}) {
			operation.fail(http.StatusConflict, "Stereo pair topology changed")
			return
		}

		target.Balance = current
	}

	if err := current.Validate(requested); err != nil {
		operation.fail(http.StatusBadRequest, err.Error())
		return
	}

	var generation uint64

	ctx, cancel := context.WithTimeout(requestContext, balanceControlTimeout)
	defer cancel()

	sendFence := func(send func() error) error {
		app.devicesMu.RLock()
		defer app.devicesMu.RUnlock()

		if app.devices[controlID] != conn || acousticDeviceID(conn) != target.HardwareID {
			return fmt.Errorf("device registration changed")
		}

		return conn.WithCurrentWebSocket(wsClient, func() error {
			return conn.WithConfirmedBalanceTarget(target, func() error {
				conn.InvalidateField(webtypes.FieldBalance)
				generation = conn.BeginFieldPoll(webtypes.FieldBalance)

				return send()
			})
		})
	}

	readback, writeErr := wsClient.SetBalanceWithBoundsForTarget(
		ctx, requested, current, target.HardwareID, sendFence,
	)

	readback, revision, applied := app.confirmBalanceReadback(ctx, controlID, conn, wsClient, target, generation, readback, writeErr)
	if !applied {
		operation.fail(http.StatusConflict, fmt.Sprintf("Stereo balance write is unverified or superseded: %v", writeErr))
		return
	}

	status := conn.Status()
	operation.broadcast = true
	operation.result.Target = intPointer(readback.Target)
	operation.result.Actual = intPointer(readback.Actual)
	operation.result.AtTarget = readback.Target == requested && readback.Actual == requested
	operation.result.Revision = uint64Pointer(revision)
	operation.result.BassRevision = status.BassRevision

	operation.result.BalanceRevision = revision
	if !operation.result.AtTarget {
		operation.fail(http.StatusConflict, fmt.Sprintf(
			"Stereo balance readback did not reach requested level %d (target %d, actual %d)",
			requested, readback.Target, readback.Actual,
		))
	}
}

func validBalanceReadback(balance *models.Balance, expectedDeviceID string) error {
	if balance == nil || strings.TrimSpace(balance.DeviceID) != expectedDeviceID {
		return fmt.Errorf("balance readback belongs to a different device")
	}

	if !balance.Available || balance.Min > balance.Max ||
		balance.Default < balance.Min || balance.Default > balance.Max ||
		balance.Target < balance.Min || balance.Target > balance.Max ||
		balance.Actual < balance.Min || balance.Actual > balance.Max {
		return fmt.Errorf("balance capability or readback is invalid")
	}

	return nil
}

func intPointer(value int) *int { return &value }

func uint64Pointer(value uint64) *uint64 { return &value }

// writeBassForTarget holds registry identity through physical verification and
// the one HTTP mutation. Removal cannot replace the target during the write.
func (app *WebApp) writeBassForTarget(controlID string, conn *webtypes.DeviceConnection, hardwareID string, requested int, capabilities *models.BassCapabilities) (bool, error) {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	if app.devices[controlID] != conn || acousticDeviceID(conn) != hardwareID {
		return false, nil
	}

	select {
	case <-conn.Done():
		return false, nil
	default:
	}

	info, err := conn.Client.GetDeviceInfo()
	if err != nil || info == nil || strings.TrimSpace(info.DeviceID) != hardwareID {
		return false, err
	}

	if !conn.ReserveBassWrite(capabilities) {
		return false, nil
	}

	return true, conn.Client.SetBass(requested)
}

func (app *WebApp) confirmBalanceReadback(ctx context.Context, controlID string, conn *webtypes.DeviceConnection, wsClient *client.WebSocketClient, target webtypes.BalanceTarget, generation uint64, readback *models.Balance, writeErr error) (*models.Balance, uint64, bool) {
	var revision uint64

	apply := func() bool {
		if validBalanceReadback(readback, target.HardwareID) != nil {
			return false
		}

		return app.applyAcousticRead(controlID, conn, target.HardwareID, func() bool {
			var accepted bool

			revision, accepted = conn.ApplyBalanceRead(target, generation, readback)

			return accepted
		})
	}

	applied := writeErr == nil && apply()
	if !applied && conn.CurrentWebSocket() == wsClient {
		var currentTarget bool

		generation, currentTarget = conn.ReserveBalanceField(target)
		if currentTarget {
			readback, _ = wsClient.GetBalance(ctx)
			applied = apply()
		}
	}

	return readback, revision, applied
}

func (app *WebApp) admitBassCapabilities(controlID string, conn *webtypes.DeviceConnection, expectedDeviceID string, operation *acousticControlOperation) (*models.BassCapabilities, bool) {
	capabilities := conn.Status().BassCapabilities
	if capabilities == nil {
		generation := conn.BeginFieldPoll(webtypes.FieldBass)

		var err error

		capabilities, err = conn.Client.GetBassCapabilities()
		if err != nil {
			operation.fail(http.StatusBadGateway, fmt.Sprintf("Bass capability is unverified: %v", err))
			return nil, false
		}

		if validBassCapabilities(capabilities, expectedDeviceID) == nil {
			accepted := app.applyAcousticRead(controlID, conn, expectedDeviceID, func() bool {
				return conn.CompleteFieldPoll(webtypes.FieldBass, generation, func(status *webtypes.DeviceStatus) {
					status.BassCapabilities = capabilities
				})
			})
			if !accepted {
				operation.fail(http.StatusConflict, "Bass capabilities changed during admission")
				return nil, false
			}

			operation.broadcast = true
		}
	}

	return capabilities, true
}
