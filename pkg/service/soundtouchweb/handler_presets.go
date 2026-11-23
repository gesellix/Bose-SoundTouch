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

const (
	presetTargetHeader = "X-AfterTouch-Preset-Target"
	presetEpochHeader  = "X-AfterTouch-Preset-Epoch"
)

type presetTarget struct {
	identity string
	epoch    int64
}

func (app *WebApp) parsePresetTarget(w http.ResponseWriter, r *http.Request, required bool) (presetTarget, bool) {
	if !required && r.Header.Values(presetTargetHeader) == nil && r.Header.Values(presetEpochHeader) == nil {
		return presetTarget{}, true
	}

	identity := strings.TrimSpace(r.Header.Get(presetTargetHeader))
	if identity == "" {
		app.sendError(w, "Preset target identity required", http.StatusBadRequest)
		return presetTarget{}, false
	}

	epoch, err := strconv.ParseInt(strings.TrimSpace(r.Header.Get(presetEpochHeader)), 10, 64)
	if err != nil || epoch <= 0 {
		app.sendError(w, "Preset target epoch must be a positive integer", http.StatusBadRequest)
		return presetTarget{}, false
	}

	return presetTarget{identity: identity, epoch: epoch}, true
}

func (app *WebApp) currentPresetTarget(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection, target presetTarget) bool {
	deviceID := chi.URLParam(r, "id")
	current, exists := app.GetDevice(deviceID)

	status := device.Status()
	if !exists || current != device || settingsTargetIdentity(device) != target.identity ||
		status == nil || status.Epoch != target.epoch {
		app.sendError(w, "Preset target changed; reopen the device", http.StatusConflict)
		return false
	}

	return true
}

func (app *WebApp) validatePresetTarget(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection, target presetTarget) bool {
	if !app.currentPresetTarget(w, r, device, target) {
		return false
	}

	if err := liveSettingsTargetError(device, target.identity); err != nil {
		app.sendError(w, "Could not verify the preset target; reopen the device", http.StatusConflict)
		return false
	}

	return app.currentPresetTarget(w, r, device, target)
}

func (app *WebApp) beginCheckedPresetWrite(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection) (func(), bool) {
	target, ok := app.parsePresetTarget(w, r, false)
	if !ok {
		return nil, false
	}

	if target.identity == "" {
		return func() {}, true
	}

	// Device removal takes this same lock, so registry ownership stays valid
	// from the final target check through the write.
	release := app.beginDeviceSettingsOperation(device)
	if !app.validatePresetTarget(w, r, device, target) || r.Context().Err() != nil {
		release()
		return nil, false
	}

	return func() {
		device.InvalidatePresetReads()
		release()
	}, true
}

// HandleDevicePresets answers a confirmation read with the physical speaker's
// whole bank and publishes it via the usual ordered status updates.
func (app *WebApp) HandleDevicePresets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")

	target, ok := app.parsePresetTarget(w, r, true)
	if !ok {
		return
	}

	device, exists := app.GetDevice(chi.URLParam(r, "id"))
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusServiceUnavailable)
		return
	}

	if !app.validatePresetTarget(w, r, device, target) {
		return
	}

	generation := device.BeginFieldPoll(webtypes.FieldPresets)

	bank, err := device.Client.GetPresets()
	if err != nil {
		app.sendError(w, "Could not read the speaker's presets", http.StatusBadGateway)
		return
	}

	if !app.validatePresetTarget(w, r, device, target) {
		return
	}

	// No registry or settings lock is held over the reads. Fence the final
	// publication against device removal and any newer preset bank.
	release := app.beginDeviceSettingsOperation(device)
	defer release()

	if r.Context().Err() != nil || !app.currentPresetTarget(w, r, device, target) {
		return
	}

	if !app.applyPresetRead(device, generation, bank) {
		app.sendError(w, "A newer preset bank arrived; read the speaker again", http.StatusConflict)
		return
	}

	status := device.Status()

	if status.Presets != bank {
		app.sendError(w, "A newer preset bank arrived; read the speaker again", http.StatusConflict)
		return
	}

	data := struct {
		Info   *models.DeviceInfo     `json:"info"`
		Status *webtypes.DeviceStatus `json:"status"`
	}{Info: device.Info(), Status: status}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: data}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
