// Package handlers contains HTTP handlers for the SoundTouch web UI.
package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gorilla/websocket"
)

// WebApp holds the application state and dependencies
type WebApp struct {
	Devices   map[string]*webtypes.DeviceConnection
	Upgrader  websocket.Upgrader
	WSClients map[*websocket.Conn]bool
	WSMutex   sync.RWMutex
}

// NewWebApp creates a new WebApp instance for SPA mode
func NewWebApp() *WebApp {
	return &WebApp{
		Devices:   make(map[string]*webtypes.DeviceConnection),
		WSClients: make(map[*websocket.Conn]bool),
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(_ *http.Request) bool { return true },
		},
	}
}

// HandleAPIDevices returns all devices as JSON
func (app *WebApp) HandleAPIDevices(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Return all devices as JSON
	devices := make(map[string]interface{})
	for id, device := range app.Devices {
		devices[id] = map[string]interface{}{
			"info":     device.DeviceInfo,
			"status":   device.Status,
			"lastSeen": device.LastSeen,
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
	deviceID := strings.TrimPrefix(r.URL.Path, "/api/device/")
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.Devices[deviceID]
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
			"status": device.Status,
		},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// HandleAPIControl handles device control commands
func (app *WebApp) HandleAPIControl(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/control/")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		app.sendError(w, "Invalid control path", http.StatusBadRequest)
		return
	}

	deviceID := parts[0]
	action := parts[1]

	// Check for empty device ID
	if deviceID == "" {
		app.sendError(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.Devices[deviceID]
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

	if device.Client == nil {
		app.sendError(w, "Device client not available", http.StatusInternalServerError)
		return
	}

	err := device.Client.SelectSource(sourceParam, "")
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
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 || pathParts[1] != "api" || pathParts[2] != "device-key" {
		app.sendError(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	deviceID := pathParts[3]
	key := pathParts[4]

	device, exists := app.Devices[deviceID]
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
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 5 || pathParts[1] != "api" || pathParts[2] != "device-volume" {
		app.sendError(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	deviceID := pathParts[3]

	volumeLevel, err := strconv.Atoi(pathParts[4])
	if err != nil || volumeLevel < 0 || volumeLevel > 100 {
		app.sendError(w, "Invalid volume level (0-100)", http.StatusBadRequest)
		return
	}

	device, exists := app.Devices[deviceID]
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
	if r.Method != http.MethodPost {
		app.sendError(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 || pathParts[1] != "api" || pathParts[2] != "device-power" {
		app.sendError(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	deviceID := pathParts[3]

	device, exists := app.Devices[deviceID]
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
	if r.Method != http.MethodGet {
		app.sendError(w, "GET required", http.StatusMethodNotAllowed)
		return
	}

	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 || pathParts[1] != "api" || pathParts[2] != "device-power-status" {
		app.sendError(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	deviceID := pathParts[3]

	device, exists := app.Devices[deviceID]
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

	devices := make(map[string]interface{})
	for id, device := range app.Devices {
		devices[id] = map[string]interface{}{
			"info":     device.DeviceInfo,
			"status":   device.Status,
			"lastSeen": device.LastSeen,
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
	app.WSMutex.RLock()
	defer app.WSMutex.RUnlock()

	message := webtypes.WebSocketMessage{
		Type: "discovery_status",
		Data: map[string]interface{}{
			"status":      status,
			"deviceCount": deviceCount,
		},
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
