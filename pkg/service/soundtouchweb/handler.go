// Package soundtouchweb contains HTTP handlers for the SoundTouch web UI.
package soundtouchweb

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	bmxpkg "github.com/gesellix/bose-soundtouch/pkg/service/bmx"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/service/stations"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// WebApp holds the application state and dependencies.
//
// The device registry (devices map + devicesMu) is encapsulated:
// callers go through GetDevice / DeviceSnapshot / AddDevice /
// TouchDevice / DeviceCount instead of touching the map directly.
// This prevents the concurrent-map-read/write panic that would
// otherwise be reachable any time an HTTP handler runs while
// discovery or the /api/discover endpoint is registering devices.
type WebApp struct {
	devicesMu sync.RWMutex
	devices   map[string]*webtypes.DeviceConnection

	Upgrader  websocket.Upgrader
	WSClients map[*websocket.Conn]bool
	WSMutex   sync.RWMutex

	Version    string
	Commit     string
	Date       string
	RepoURL    string
	ServiceURL string

	// InternalServiceURL is the base URL the player uses for its own
	// server-side calls back to the AfterTouch service (currently the TTS
	// proxy at /api/setup/tts/speak). The embedded build sets it to the
	// service's loopback HTTP listener so those self-calls never depend on TLS
	// or the service's self-signed CA. Standalone soundtouch-player leaves it
	// empty and falls back to ServiceURL.
	InternalServiceURL string

	// ServiceClient is used for server-side calls to the AfterTouch service
	// (currently the TTS proxy). When nil, serviceHTTPClient falls back to
	// http.DefaultClient. Set it via NewServiceHTTPClient to trust the
	// service's self-signed CA.
	ServiceClient *http.Client

	// ExtraDeviceHosts, when set, returns additional device host IPs to
	// register alongside mDNS/UPnP discovery. The embedded build in
	// soundtouch-service points it at the service datastore's known devices so
	// the UI shows manually-added speakers even when network discovery is
	// disabled. Standalone soundtouch-player leaves it nil.
	ExtraDeviceHosts func() []string

	// TriggerDiscovery, when set, runs an external discovery sweep instead of
	// this app's own mDNS/UPnP. The embedded build wires it to the host
	// service's discovery (the single source of truth, which updates the shared
	// datastore); DiscoverDevices then re-syncs from ExtraDeviceHosts. Standalone
	// soundtouch-player leaves it nil and runs its own sweep.
	TriggerDiscovery func(ctx context.Context)

	// RemoveDeviceHook, when set, removes a device from the backing store by
	// its device ID (MAC). The embedded build wires it to the service's
	// datastore removal so a removal from the player UI also clears the
	// persisted device; standalone soundtouch-player leaves it nil (no store, so
	// removal only prunes the in-memory registry).
	RemoveDeviceHook func(deviceID string) error

	discoveryStatus atomic.Value // stores *webtypes.DiscoveryStatus
}

// serviceHTTPClient returns the client used for outbound calls to the
// AfterTouch service, falling back to http.DefaultClient when no CA-trusting
// client was configured.
func (app *WebApp) serviceHTTPClient() *http.Client {
	if app.ServiceClient != nil {
		return app.ServiceClient
	}

	return http.DefaultClient
}

// proxyServiceURL returns the base URL for the player's own server-side calls
// back to the AfterTouch service (the TTS proxy). It prefers the loopback
// InternalServiceURL (plain HTTP, no CA needed) and falls back to the public
// ServiceURL for the standalone build where no internal URL is set.
func (app *WebApp) proxyServiceURL() string {
	if app.InternalServiceURL != "" {
		return app.InternalServiceURL
	}

	return app.ServiceURL
}

// DeviceEntry pairs a device id with its connection. Used by
// DeviceSnapshot so callers can iterate without holding the lock.
type DeviceEntry struct {
	ID     string
	Device *webtypes.DeviceConnection
}

// NewWebApp creates a new WebApp instance for SPA mode
func NewWebApp() *WebApp {
	return &WebApp{
		devices:   make(map[string]*webtypes.DeviceConnection),
		WSClients: make(map[*websocket.Conn]bool),
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

// GetDevice returns the device for id and whether it exists.
func (app *WebApp) GetDevice(id string) (*webtypes.DeviceConnection, bool) {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	device, ok := app.devices[id]

	return device, ok
}

// DeviceSnapshot returns a list of (id, *DeviceConnection) pairs taken
// under a single read lock. Callers can iterate the result without
// holding any registry lock. Devices added or removed after the call
// are not reflected. A pointer captured here stays valid even if the
// device is later removed (RemoveDevice only detaches it from the map
// and stops its goroutines), so iterating a stale snapshot is safe.
func (app *WebApp) DeviceSnapshot() []DeviceEntry {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	out := make([]DeviceEntry, 0, len(app.devices))
	for id, device := range app.devices {
		out = append(out, DeviceEntry{ID: id, Device: device})
	}

	return out
}

// DeviceCount returns the number of registered devices at call time.
func (app *WebApp) DeviceCount() int {
	app.devicesMu.RLock()
	defer app.devicesMu.RUnlock()

	return len(app.devices)
}

// AddDevice atomically registers conn under id when id is not already
// known. If id existed, its LastSeen is bumped and AddDevice returns
// false (the caller should discard conn). Returns true if conn was
// inserted.
func (app *WebApp) AddDevice(id string, conn *webtypes.DeviceConnection) bool {
	app.devicesMu.Lock()
	defer app.devicesMu.Unlock()

	if existing, ok := app.devices[id]; ok {
		existing.LastSeen = time.Now()
		return false
	}

	app.devices[id] = conn

	return true
}

// TouchDevice bumps LastSeen for id if it exists; returns true if
// found. Use this as a fast-path check before doing the network work
// needed to construct a new DeviceConnection.
func (app *WebApp) TouchDevice(id string) bool {
	app.devicesMu.Lock()
	defer app.devicesMu.Unlock()

	existing, ok := app.devices[id]
	if !ok {
		return false
	}

	existing.LastSeen = time.Now()

	return true
}

// RemoveDevice removes the device registered under id and stops its
// background goroutines (status poller + WebSocket reconnect loop) via
// conn.Close. Returns true if id was present. Close runs outside the
// registry lock because it performs network I/O (WebSocket disconnect).
func (app *WebApp) RemoveDevice(id string) bool {
	app.devicesMu.Lock()

	conn, ok := app.devices[id]
	if ok {
		delete(app.devices, id)
	}

	app.devicesMu.Unlock()

	if ok {
		conn.Close()
	}

	return ok
}

// HandleAPIDevices returns all devices as JSON
func (app *WebApp) HandleAPIDevices(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Return all devices as JSON
	snapshot := app.DeviceSnapshot()
	devices := make(map[string]interface{}, len(snapshot))

	for _, entry := range snapshot {
		devices[entry.ID] = map[string]interface{}{
			"info":     entry.Device.DeviceInfo,
			"status":   entry.Device.Status(),
			"lastSeen": entry.Device.LastSeen,
		}
	}

	response := webtypes.APIResponse{
		Success: true,
		Data:    devices,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleAPIDevice returns a specific device as JSON
func (app *WebApp) HandleAPIDevice(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	// Update device status to get fresh power state
	app.UpdateDeviceStatus(deviceID, device)

	// Connect WebSocket for real-time updates if not already connected
	if device.WebSocket == nil {
		go app.ConnectDeviceWebSocket(deviceID, device)
	}

	w.Header().Set("Content-Type", "application/json")

	response := webtypes.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"info":   device.DeviceInfo,
			"status": device.Status(),
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleDeleteDevice removes a device from the registry and, in the
// embedded build, from the service datastore. The registry is keyed by
// host/IP (the {id} URL param); the datastore is keyed by device ID
// (MAC), so we resolve one to the other via the connection's DeviceInfo
// before cascading. A device still live on the network is re-discovered
// on the next sweep — removal is "remove now", not a permanent ban.
func (app *WebApp) HandleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	host := chi.URLParam(r, "id")
	if host == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	conn, exists := app.GetDevice(host)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	// Cascade to the backing store (embedded build only). Standalone
	// soundtouch-player has no datastore and leaves the hook nil, so removal
	// only prunes the in-memory registry below.
	if app.RemoveDeviceHook != nil {
		deviceID := ""
		if conn.DeviceInfo != nil {
			deviceID = conn.DeviceInfo.DeviceID
		}

		if err := app.RemoveDeviceHook(deviceID); err != nil {
			log.Printf("Failed to remove device %s from store: %v", sanitizeLog(host), err)
			app.sendError(w, "Failed to remove device from store", http.StatusBadGateway)

			return
		}
	}

	app.RemoveDevice(host)
	app.BroadcastDeviceList()

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleAPIControl handles device control commands
func (app *WebApp) HandleAPIControl(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	action := chi.URLParam(r, "action")

	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	// Connect WebSocket for real-time updates if not already connected
	if device.WebSocket == nil {
		go app.ConnectDeviceWebSocket(deviceID, device)
	}

	w.Header().Set("Content-Type", "application/json")

	app.handleControlAction(w, r, action, device)
}

// handleControlAction processes different control actions
func (app *WebApp) handleControlAction(w http.ResponseWriter, r *http.Request, action string, device *webtypes.DeviceConnection) {
	switch action {
	case "play":
		if device.Client == nil {
			app.sendError(w, "Device client not available", http.StatusInternalServerError)
			return
		}

		err := device.Client.Play()
		app.sendControlResponse(w, err, "Started playback")
	case "pause":
		if device.Client == nil {
			app.sendError(w, "Device client not available", http.StatusInternalServerError)
			return
		}

		err := device.Client.Pause()
		app.sendControlResponse(w, err, "Paused playback")
	case "stop":
		if device.Client == nil {
			app.sendError(w, "Device client not available", http.StatusInternalServerError)
			return
		}

		err := device.Client.Stop()
		app.sendControlResponse(w, err, "Stopped playback")
	case "next":
		if device.Client == nil {
			app.sendError(w, "Device client not available", http.StatusInternalServerError)
			return
		}

		err := device.Client.NextTrack()
		app.sendControlResponse(w, err, "Next track")
	case "previous":
		if device.Client == nil {
			app.sendError(w, "Device client not available", http.StatusInternalServerError)
			return
		}

		err := device.Client.PrevTrack()
		app.sendControlResponse(w, err, "Previous track")
	case "volume":
		app.handleVolumeControl(w, r, device)
	case "mute":
		if device.Client == nil {
			app.sendError(w, "Device client not available", http.StatusInternalServerError)
			return
		}

		err := device.Client.SendKey(models.KeyMute)
		app.sendControlResponse(w, err, "Toggled mute")
	case "preset":
		app.handlePresetControl(w, r, device)
	case "storepreset":
		app.handleStorePreset(w, r, device)
	case "bass":
		app.handleBassControl(w, r, device)
	case "source":
		app.handleSourceControl(w, r, device)
	default:
		app.sendError(w, "Unknown action", http.StatusBadRequest)
	}
}

// handleVolumeControl processes volume control requests
func (app *WebApp) handleVolumeControl(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection) {
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required for volume control", http.StatusMethodNotAllowed)
		return
	}

	var volumeReq webtypes.VolumeRequest
	if err := json.NewDecoder(r.Body).Decode(&volumeReq); err != nil {
		app.sendError(w, "Invalid volume data", http.StatusBadRequest)
		return
	}

	if volumeReq.Level < 0 || volumeReq.Level > 100 {
		app.sendError(w, "Volume must be between 0 and 100", http.StatusBadRequest)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	err := device.Client.SetVolume(volumeReq.Level)
	app.sendControlResponse(w, err, fmt.Sprintf("Volume set to %d", volumeReq.Level))
}

// handlePresetControl processes preset control requests
func (app *WebApp) handlePresetControl(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection) {
	presetParam := r.URL.Query().Get("id")
	if presetParam == "" {
		app.sendError(w, "Preset ID required", http.StatusBadRequest)
		return
	}

	presetID, err := strconv.Atoi(presetParam)
	if err != nil {
		app.sendError(w, "Invalid preset ID", http.StatusBadRequest)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	err = device.Client.SelectPreset(presetID)
	app.sendControlResponse(w, err, fmt.Sprintf("Selected preset %d", presetID))
}

// handleStorePreset stores the currently playing content as a numbered preset.
func (app *WebApp) handleStorePreset(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection) {
	presetParam := r.URL.Query().Get("id")
	if presetParam == "" {
		app.sendError(w, "Preset ID required", http.StatusBadRequest)
		return
	}

	presetID, err := strconv.Atoi(presetParam)
	if err != nil {
		app.sendError(w, "Invalid preset ID", http.StatusBadRequest)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	err = device.Client.StoreCurrentAsPreset(presetID)
	app.sendControlResponse(w, err, fmt.Sprintf("Stored current as preset %d", presetID))
}

// handleBassControl processes bass control requests
func (app *WebApp) handleBassControl(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection) {
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required for bass control", http.StatusMethodNotAllowed)
		return
	}

	var bassReq webtypes.BassRequest
	if err := json.NewDecoder(r.Body).Decode(&bassReq); err != nil {
		app.sendError(w, "Invalid bass data", http.StatusBadRequest)
		return
	}

	if bassReq.Level < -9 || bassReq.Level > 9 {
		app.sendError(w, "Bass must be between -9 and 9", http.StatusBadRequest)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	err := device.Client.SetBass(bassReq.Level)
	app.sendControlResponse(w, err, fmt.Sprintf("Bass set to %d", bassReq.Level))
}

// handleSourceControl processes source control requests
func (app *WebApp) handleSourceControl(w http.ResponseWriter, r *http.Request, device *webtypes.DeviceConnection) {
	sourceParam := r.URL.Query().Get("name")
	if sourceParam == "" {
		app.sendError(w, "Source name required", http.StatusBadRequest)
		return
	}

	// Forward the optional account parameter as sourceAccount. Devices with
	// multiple jacks that share source="AUX" (e.g. ST-5 CD/Aux inputs)
	// disambiguate them via distinct sourceAccount values (AUX, AUX1, …).
	accountParam := r.URL.Query().Get("account")

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	logPlaybackRequest("source-select", chi.URLParam(r, "id"), sourceParam, accountParam, "", "")

	err := device.Client.SelectSource(sourceParam, accountParam)
	app.sendControlResponse(w, err, fmt.Sprintf("Selected source %s", sourceParam))
}

// sendControlResponse sends a control command response
func (app *WebApp) sendControlResponse(w http.ResponseWriter, err error, successMessage string) {
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := webtypes.APIResponse{
		Success: true,
		Data:    map[string]string{"message": successMessage},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// sendError sends an error response
func (app *WebApp) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := webtypes.APIResponse{
		Success: false,
		Error:   message,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode error response", http.StatusInternalServerError)
	}
}

// HandleDeviceKey handles sending key commands to devices
func (app *WebApp) HandleDeviceKey(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	key := chi.URLParam(r, "key")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	// Connect WebSocket for real-time updates if not already connected
	if device.WebSocket == nil {
		go app.ConnectDeviceWebSocket(deviceID, device)
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err := device.Client.SendKey(key)
	app.sendControlResponse(w, err, fmt.Sprintf("Sent key command: %s", key))
}

// HandleDirectVolumeControl handles direct volume setting via URL parameter
func (app *WebApp) HandleDirectVolumeControl(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	volumeLevel, err := strconv.Atoi(chi.URLParam(r, "volume"))
	if err != nil || volumeLevel < 0 || volumeLevel > 100 {
		app.sendError(w, "Invalid volume level (0-100)", http.StatusBadRequest)
		return
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	// Connect WebSocket for real-time updates if not already connected
	if device.WebSocket == nil {
		go app.ConnectDeviceWebSocket(deviceID, device)
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	err = device.Client.SetVolume(volumeLevel)
	app.sendControlResponse(w, err, fmt.Sprintf("Volume set to %d", volumeLevel))
}

// HandleDevicePower handles power toggle commands for devices
func (app *WebApp) HandleDevicePower(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	// Connect WebSocket for real-time updates if not already connected
	if device.WebSocket == nil {
		go app.ConnectDeviceWebSocket(deviceID, device)
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Send POWER key command to toggle device power
	err := device.Client.SendKey("POWER")
	app.sendControlResponse(w, err, "Power toggle command sent")
}

// HandleDevicePowerStatus handles lightweight power status check
func (app *WebApp) HandleDevicePowerStatus(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Quick power status check by getting now playing
	nowPlaying, err := device.Client.GetNowPlaying()
	if err != nil {
		app.sendControlResponse(w, err, "Failed to get power status")
		return
	}

	isPoweredOn := nowPlaying != nil && nowPlaying.Source != "STANDBY"

	response := webtypes.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"deviceId":    deviceID,
			"isPoweredOn": isPoweredOn,
			"source":      nowPlaying.Source,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// BroadcastDeviceList sends updated device list to all connected WebSocket clients
func (app *WebApp) BroadcastDeviceList() {
	app.WSMutex.RLock()
	defer app.WSMutex.RUnlock()

	snapshot := app.DeviceSnapshot()
	devices := make(map[string]interface{}, len(snapshot))

	for _, entry := range snapshot {
		devices[entry.ID] = map[string]interface{}{
			"info":     entry.Device.DeviceInfo,
			"status":   entry.Device.Status(),
			"lastSeen": entry.Device.LastSeen,
		}
	}

	message := webtypes.WebSocketMessage{
		Type: "devices",
		Data: devices,
	}

	// Send to all connected clients
	var failedClients []*websocket.Conn

	for client := range app.WSClients {
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Failed to send device update to WebSocket client: %v", err)
			// Mark for removal to avoid modifying map during iteration
			failedClients = append(failedClients, client)
		}
	}

	// Remove failed clients
	for _, client := range failedClients {
		delete(app.WSClients, client)
		client.Close()
	}
}

// BroadcastDiscoveryStatus sends discovery progress updates to all connected WebSocket clients
func (app *WebApp) BroadcastDiscoveryStatus(status string, deviceCount int) {
	discoveryStatus := &webtypes.DiscoveryStatus{
		Status:      status,
		DeviceCount: deviceCount,
	}

	switch status {
	case "starting":
		discoveryStatus.IsDiscovering = true
	case "completed", "failed":
		discoveryStatus.IsDiscovering = false
	}

	app.discoveryStatus.Store(discoveryStatus)

	app.WSMutex.RLock()
	defer app.WSMutex.RUnlock()

	message := webtypes.WebSocketMessage{
		Type: "discovery_status",
		Data: discoveryStatus,
	}

	// Send to all connected clients
	var failedClients []*websocket.Conn

	for client := range app.WSClients {
		if err := client.WriteJSON(message); err != nil {
			log.Printf("Failed to send discovery status to WebSocket client: %v", err)
			// Mark for removal to avoid modifying map during iteration
			failedClients = append(failedClients, client)
		}
	}

	// Remove failed clients
	for _, client := range failedClients {
		delete(app.WSClients, client)
		client.Close()
	}
}

// HandleTuneInSearch handles TuneIn search requests, proxying directly to the bmx package.
func (app *WebApp) HandleTuneInSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		app.sendError(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	resp, err := stations.Search(stations.ProviderTuneIn, query)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: resp}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleTuneInSearchNext returns the next page of TuneIn search results using an opaque cursor.
func (app *WebApp) HandleTuneInSearchNext(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("cursor")
	if cursor == "" {
		app.sendError(w, "cursor parameter required", http.StatusBadRequest)
		return
	}

	resp, err := stations.SearchNext(stations.ProviderTuneIn, cursor)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: resp}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleTuneInNavigate handles TuneIn browse/navigate requests, proxying directly to the bmx package.
// Supported path suffixes (relative to /api/tunein/navigate):
//   - (empty)                             → top-level browse
//   - /{encodedURI}                       → browse the given TuneIn URI
//   - /sub/{n}/{encodedURI}               → single subsection
//   - /profiles/{type}/{id}/{encodedURI}  → artist/program profile
func (app *WebApp) HandleTuneInNavigate(w http.ResponseWriter, r *http.Request) {
	wildcard := chi.URLParam(r, "*")

	resp, err := stations.Navigate(stations.ProviderTuneIn, wildcard)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: resp}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// findIPByHwID returns the registry key (IP) for the device whose
// hardware ID matches hwID. Used by zone handlers to bridge between
// the speaker's hwID-keyed zone protocol and our IP-keyed registry.
// Returns "" when no match is found.
func (app *WebApp) findIPByHwID(hwID string) string {
	for _, entry := range app.DeviceSnapshot() {
		if entry.Device.DeviceInfo != nil && entry.Device.DeviceInfo.DeviceID == hwID {
			return entry.ID
		}
	}

	return ""
}

// HandleGetZone returns zone info for a device, enriched with member
// names and role flags (isMaster / isSlave / isStandalone) computed
// from the perspective of the queried device.
func (app *WebApp) HandleGetZone(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	zone, err := device.Client.GetZone()
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	currentHwID := ""
	if device.DeviceInfo != nil {
		currentHwID = device.DeviceInfo.DeviceID
	}

	masterIP := app.findIPByHwID(zone.Master)

	masterName := ""
	if conn, ok := app.GetDevice(masterIP); ok && conn.DeviceInfo != nil {
		masterName = conn.DeviceInfo.Name
	}

	type memberInfo struct {
		IP   string `json:"ip"`
		HwID string `json:"hwId"`
		Name string `json:"name"`
	}

	members := make([]memberInfo, 0, len(zone.Members))

	for _, m := range zone.Members {
		name := ""
		if conn, ok := app.GetDevice(m.IP); ok && conn.DeviceInfo != nil {
			name = conn.DeviceInfo.Name
		}

		members = append(members, memberInfo{IP: m.IP, HwID: m.DeviceID, Name: name})
	}

	isMaster := zone.Master == currentHwID && !zone.IsStandalone()
	isSlave := false

	for _, m := range zone.Members {
		if m.DeviceID == currentHwID {
			isSlave = true
			break
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data: map[string]interface{}{
			"masterIp":     masterIP,
			"masterHwId":   zone.Master,
			"masterName":   masterName,
			"members":      members,
			"isMaster":     isMaster,
			"isSlave":      isSlave,
			"isStandalone": !isMaster && !isSlave,
		},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleZoneAdd adds a slave device to the zone where {id} is or
// becomes the master.
func (app *WebApp) HandleZoneAdd(w http.ResponseWriter, r *http.Request) {
	masterIP := chi.URLParam(r, "id")
	slaveIP := chi.URLParam(r, "slaveId")

	masterConn, ok := app.GetDevice(masterIP)
	if !ok {
		app.sendError(w, "Master device not found", http.StatusNotFound)
		return
	}

	slaveConn, ok := app.GetDevice(slaveIP)
	if !ok {
		app.sendError(w, "Slave device not found", http.StatusNotFound)
		return
	}

	if masterConn.Client == nil || masterConn.DeviceInfo == nil || slaveConn.DeviceInfo == nil {
		app.sendError(w, "Device not ready", http.StatusInternalServerError)
		return
	}

	masterHwID := masterConn.DeviceInfo.DeviceID
	slaveHwID := slaveConn.DeviceInfo.DeviceID

	zone, err := masterConn.Client.GetZone()
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var zoneReq *models.ZoneRequest
	if zone.IsStandalone() {
		zoneReq = models.NewZoneRequest(masterHwID)
	} else {
		zoneReq = zone.ToZoneRequest()
	}

	zoneReq.AddMember(slaveHwID, slaveIP)

	w.Header().Set("Content-Type", "application/json")
	app.sendControlResponse(w, masterConn.Client.SetZone(zoneReq), "Device added to zone")
}

// HandleZoneRemove removes a slave from the zone.
func (app *WebApp) HandleZoneRemove(w http.ResponseWriter, r *http.Request) {
	masterIP := chi.URLParam(r, "id")
	slaveIP := chi.URLParam(r, "slaveId")

	masterConn, ok := app.GetDevice(masterIP)
	if !ok {
		app.sendError(w, "Master device not found", http.StatusNotFound)
		return
	}

	slaveConn, ok := app.GetDevice(slaveIP)
	if !ok {
		app.sendError(w, "Slave device not found", http.StatusNotFound)
		return
	}

	if masterConn.Client == nil || masterConn.DeviceInfo == nil || slaveConn.DeviceInfo == nil {
		app.sendError(w, "Device not ready", http.StatusInternalServerError)
		return
	}

	masterHwID := masterConn.DeviceInfo.DeviceID
	slaveHwID := slaveConn.DeviceInfo.DeviceID

	// Remove a single member with the dedicated /removeZoneSlave endpoint.
	// Rebuilding the zone via /setZone with the remaining members does not
	// reliably drop a member when the zone has more than one: the speaker only
	// goes standalone when the resulting member set is empty, so removing one of
	// several members appeared to do nothing (#511). /removeZoneSlave targets the
	// specific member.
	w.Header().Set("Content-Type", "application/json")
	app.sendControlResponse(w, masterConn.Client.RemoveZoneSlave(masterHwID, slaveHwID, slaveIP), "Device removed from zone")
}

// HandleZoneDissolve dissolves the zone, making all devices standalone.
func (app *WebApp) HandleZoneDissolve(w http.ResponseWriter, r *http.Request) {
	masterIP := chi.URLParam(r, "id")

	masterConn, ok := app.GetDevice(masterIP)
	if !ok {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if masterConn.Client == nil || masterConn.DeviceInfo == nil {
		app.sendError(w, "Device not ready", http.StatusInternalServerError)
		return
	}

	zoneReq := models.NewZoneRequest(masterConn.DeviceInfo.DeviceID)

	w.Header().Set("Content-Type", "application/json")
	app.sendControlResponse(w, masterConn.Client.SetZone(zoneReq), "Zone dissolved")
}

// HandleZoneLeave removes the calling device from its zone (slave
// perspective). The slave is identified by {id}; the master is
// located by walking the registry for the hwID the slave's zone
// names as Master, then SetZone is issued against that master.
func (app *WebApp) HandleZoneLeave(w http.ResponseWriter, r *http.Request) {
	slaveIP := chi.URLParam(r, "id")

	slaveConn, ok := app.GetDevice(slaveIP)
	if !ok {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if slaveConn.Client == nil || slaveConn.DeviceInfo == nil {
		app.sendError(w, "Device not ready", http.StatusInternalServerError)
		return
	}

	zone, err := slaveConn.Client.GetZone()
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	masterIP := app.findIPByHwID(zone.Master)
	if masterIP == "" {
		app.sendError(w, "Zone master not found in device list", http.StatusNotFound)
		return
	}

	masterConn, ok := app.GetDevice(masterIP)
	if !ok || masterConn.Client == nil {
		app.sendError(w, "Master device not available", http.StatusInternalServerError)
		return
	}

	// Drop this slave with the dedicated /removeZoneSlave endpoint sent to the
	// master. Rebuilding the zone via /setZone with the remaining members does
	// not drop a member from a multi-member zone (the master only goes standalone
	// when the resulting set is empty), so leaving a 3+ device zone did nothing
	// (#511). zone.Master is the master's hwID.
	w.Header().Set("Content-Type", "application/json")
	app.sendControlResponse(w,
		masterConn.Client.RemoveZoneSlave(zone.Master, slaveConn.DeviceInfo.DeviceID, slaveIP),
		"Left zone")
}

// HandleDeviceRecents returns recently played items for a device.
func (app *WebApp) HandleDeviceRecents(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	recents, err := device.Client.GetRecents()
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: recents}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// storedMusicTypeForReplay derives a STORED_MUSIC ContentItem type from the
// speaker-native location, which ends with the item kind (e.g. "1$4$2 TRACK"
// or a container's "… DIR"). Recents don't store the type, and the speaker
// rejects an empty-type STORED_MUSIC select with INVALID_SOURCE. Falls back to
// "track" when the location has no kind suffix.
func storedMusicTypeForReplay(location string) string {
	if fields := strings.Fields(location); len(fields) >= 2 {
		return strings.ToLower(fields[len(fields)-1])
	}

	return "track"
}

// HandleDevicePlay plays an arbitrary content item on a device. Generic
// counterpart to HandlePlayTuneIn — used by the Recents panel to replay
// items the speaker reports under /recents, regardless of their source.
func (app *WebApp) HandleDevicePlay(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		Source        string `json:"source"`
		Type          string `json:"type"`
		Location      string `json:"location"`
		SourceAccount string `json:"sourceAccount"`
		ItemName      string `json:"itemName"`
		ContainerArt  string `json:"containerArt"`
		IsPresetable  bool   `json:"isPresetable"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Location == "" {
		app.sendError(w, "location is required", http.StatusBadRequest)
		return
	}

	// Recents don't carry a contentItemType for STORED_MUSIC, and the speaker
	// rejects an empty-type STORED_MUSIC select with INVALID_SOURCE. The
	// speaker-native location ends with the item kind (e.g. "1$4$2 TRACK"), so
	// derive the type from it when the caller didn't supply one.
	ciType := req.Type
	if ciType == "" && req.Source == "STORED_MUSIC" {
		ciType = storedMusicTypeForReplay(req.Location)
	}

	contentItem := &models.ContentItem{
		Source:       req.Source,
		Type:         ciType,
		Location:     req.Location,
		ItemName:     req.ItemName,
		ContainerArt: req.ContainerArt,
		IsPresetable: req.IsPresetable,
	}
	// Only pass SourceAccount when it's a real credential, not the placeholder
	// value that speakers echo back (source name == source account, e.g. "TUNEIN").
	if req.SourceAccount != "" && req.SourceAccount != req.Source {
		contentItem.SourceAccount = req.SourceAccount
	}

	logPlaybackRequest("device-play", deviceID, contentItem.Source, contentItem.SourceAccount, contentItem.Location, contentItem.ItemName)

	if err := device.Client.SelectContentItem(contentItem); err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Playing " + req.ItemName},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandlePlayURL plays a custom stream URL on a device. When ServiceURL is
// configured the stream is wrapped in the Orion location format so the
// speaker's BMX module receives JSON instead of raw audio bytes. This also
// ensures that the ★ preset save flow stores a working location.
func (app *WebApp) HandlePlayURL(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	var req struct {
		URL        string `json:"url"`
		Name       string `json:"name"`
		ImageURL   string `json:"imageUrl"`
		ServiceURL string `json:"serviceUrl"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		app.sendError(w, "url is required", http.StatusBadRequest)
		return
	}

	// Server-side --service-url wins; fall back to client-supplied value.
	serviceURL := app.ServiceURL
	if serviceURL == "" {
		serviceURL = strings.TrimRight(req.ServiceURL, "/")
	}

	if serviceURL == "" {
		app.sendError(w,
			"AfterTouch service URL is required for LOCAL_INTERNET_RADIO playback. "+
				"Start soundtouch-player with --service-url <https://your-aftertouch-host> or enter it in the Play URL settings.",
			http.StatusBadRequest)

		return
	}

	location := bmxpkg.BuildOrionLocation(serviceURL, req.Name, req.ImageURL, req.URL)

	contentItem := &models.ContentItem{
		Source:       "LOCAL_INTERNET_RADIO",
		Type:         "stationurl",
		Location:     location,
		ItemName:     req.Name,
		IsPresetable: true,
	}

	logPlaybackRequest("play-url", deviceID, contentItem.Source, contentItem.SourceAccount, contentItem.Location, contentItem.ItemName)

	if err := device.Client.SelectContentItem(contentItem); err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Playing " + req.Name},
	}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleAPIVersion returns the current version of the application.
func (app *WebApp) HandleAPIVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	versionInfo := map[string]string{
		"version":     app.Version,
		"commit":      app.Commit,
		"date":        app.Date,
		"repo_url":    app.RepoURL,
		"release_url": app.RepoURL + "/releases/tag/" + app.Version,
		"commit_url":  app.RepoURL + "/commit/" + app.Commit,
		"service_url": app.ServiceURL,
	}
	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: versionInfo}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleHealth returns a minimal liveness response.
func (app *WebApp) HandleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok", "version": app.Version}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleRadioBrowserSearch handles RadioBrowser search requests.
func (app *WebApp) HandleRadioBrowserSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		app.sendError(w, "query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	resp, err := stations.Search(stations.ProviderRadioBrowser, query)
	if err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: resp}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandlePlayRadioBrowser plays a RadioBrowser station on a specific device.
func (app *WebApp) HandlePlayRadioBrowser(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, "Device not found", http.StatusNotFound)
		return
	}

	var req struct {
		Location string `json:"location"`
		Name     string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	ci := stations.ResolveContentItem(stations.PlayItem{
		Provider: stations.ProviderRadioBrowser,
		Location: req.Location,
		Name:     req.Name,
	})

	logPlaybackRequest("radiobrowser", deviceID, ci.Source, ci.SourceAccount, ci.Location, ci.ItemName)

	if err := device.Client.SelectContentItem(ci); err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if err := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]string{"message": "Playing " + req.Name}}); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandlePlayTuneIn plays a TuneIn content item on a specific device via POST /select.
func (app *WebApp) HandlePlayTuneIn(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		app.sendError(w, fmt.Sprintf("Device '%s' not found", deviceID), http.StatusNotFound)
		return
	}

	var req struct {
		Location     string `json:"location"`
		Name         string `json:"name"`
		Type         string `json:"type"`
		ContainerArt string `json:"containerArt"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		app.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Location == "" {
		app.sendError(w, "location is required", http.StatusBadRequest)
		return
	}

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	ci := stations.ResolveContentItem(stations.PlayItem{
		Provider:     stations.ProviderTuneIn,
		Location:     req.Location,
		Name:         req.Name,
		Type:         req.Type,
		ContainerArt: req.ContainerArt,
	})

	logPlaybackRequest("tunein", deviceID, ci.Source, ci.SourceAccount, ci.Location, ci.ItemName)

	if err := device.Client.SelectContentItem(ci); err != nil {
		app.sendError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if encErr := json.NewEncoder(w).Encode(webtypes.APIResponse{Success: true, Data: map[string]string{"message": "Playing " + req.Name}}); encErr != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}
