// Package handlers contains WebSocket handlers for real-time communication.
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gorilla/websocket"
)

// HandleWebSocket handles WebSocket connections for real-time updates
func (app *WebApp) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	defer func() {
		// Unregister client
		app.WSMutex.Lock()
		delete(app.WSClients, conn)
		app.WSMutex.Unlock()
		conn.Close()
	}()

	// Register client
	app.WSMutex.Lock()
	app.WSClients[conn] = true
	app.WSMutex.Unlock()

	// Send initial device list
	devices := make(map[string]interface{})
	for id, device := range app.Devices {
		devices[id] = map[string]interface{}{
			"info":     device.DeviceInfo,
			"status":   device.Status,
			"lastSeen": device.LastSeen,
		}
	}

	initialMessage := webtypes.WebSocketMessage{
		Type: "devices",
		Data: devices,
	}

	if err := conn.WriteJSON(initialMessage); err != nil {
		log.Printf("Failed to send initial data: %v", err)
		return
	}

	// Keep connection alive and send updates
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Set up ping handler to detect client disconnects
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Set initial read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Handle incoming messages in a separate goroutine
	go func() {
		defer conn.Close()

		for {
			if _, _, err := conn.NextReader(); err != nil {
				log.Printf("WebSocket read error: %v", err)
				return
			}
		}
	}()

	// Main loop for sending periodic updates
	for range ticker.C {
		// Send ping to check if client is still connected
		if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			log.Printf("Failed to send ping: %v", err)
			return
		}

		// Send periodic status updates
		for id, device := range app.Devices {
			if device.Status.IsConnected {
				statusMessage := webtypes.WebSocketMessage{
					Type:     "status_update",
					DeviceID: id,
					Data:     device.Status,
				}

				if err := conn.WriteJSON(statusMessage); err != nil {
					log.Printf("Failed to send status update: %v", err)
					return
				}
			}
		}
	}
}

// HandleAPIDiscover triggers device discovery
func (app *WebApp) HandleAPIDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		app.sendError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Discovery will be triggered by the main app
	w.Header().Set("Content-Type", "application/json")

	response := webtypes.APIResponse{
		Success: true,
		Data:    map[string]string{"message": "Discovery started"},
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

// ConnectDeviceWebSocket establishes a WebSocket connection to a device
func (app *WebApp) ConnectDeviceWebSocket(deviceID string, conn *webtypes.DeviceConnection) {
	// Skip WebSocket connection if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

	wsClient := conn.Client.NewWebSocketClient(nil)

	// Setup event handlers
	wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
		conn.Status.NowPlaying = &event.NowPlaying
		conn.Status.LastActivity = time.Now()
	})

	wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
		conn.Status.Volume = &event.Volume
		conn.Status.LastActivity = time.Now()
	})

	wsClient.OnConnectionState(func(event *models.ConnectionStateUpdatedEvent) {
		conn.Status.IsConnected = event.ConnectionState.IsConnected()
		conn.Status.LastActivity = time.Now()
	})

	wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
		conn.Status.Presets = &event.Presets
		conn.Status.LastActivity = time.Now()
	})

	// Connect WebSocket
	if err := wsClient.Connect(); err != nil {
		log.Printf("Failed to connect WebSocket for device %s: %v", deviceID, err)
		return
	}

	conn.WebSocket = wsClient
	conn.Status.IsConnected = true

	log.Printf("WebSocket connected for device %s", deviceID)

	// Wait for disconnection
	wsClient.Wait()

	conn.Status.IsConnected = false

	log.Printf("WebSocket disconnected for device %s", deviceID)
}

// UpdateDeviceStatus fetches current status from device
func (app *WebApp) UpdateDeviceStatus(_ string, conn *webtypes.DeviceConnection) {
	// Skip status update if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

	statusUpdated := false

	// Get current now playing
	if nowPlaying, err := conn.Client.GetNowPlaying(); err == nil {
		conn.Status.NowPlaying = nowPlaying
		statusUpdated = true
	}

	// Get current volume
	if volume, err := conn.Client.GetVolume(); err == nil {
		conn.Status.Volume = volume
		statusUpdated = true
	}

	// Get presets
	if presets, err := conn.Client.GetPresets(); err == nil {
		conn.Status.Presets = presets
		statusUpdated = true
	}

	// Update last activity if any status was updated
	if statusUpdated {
		conn.Status.LastActivity = time.Now()
	}
	// Get sources
	if sources, err := conn.Client.GetSources(); err == nil {
		conn.Status.Sources = sources
		statusUpdated = true
	}

	// Get bass (if available)
	if bass, err := conn.Client.GetBass(); err == nil {
		conn.Status.Bass = bass
		statusUpdated = true
	}

	// Mark as connected if we successfully got at least one status
	conn.Status.IsConnected = statusUpdated
	conn.Status.LastActivity = time.Now()
}

// HandleDeviceWebSocket handles individual device WebSocket connections for real-time device-specific updates
func (app *WebApp) HandleDeviceWebSocket(w http.ResponseWriter, r *http.Request) {
	pathParts := strings.Split(r.URL.Path, "/")
	if len(pathParts) < 4 || pathParts[1] != "api" || pathParts[2] != "device-ws" {
		http.Error(w, "Invalid path format", http.StatusBadRequest)
		return
	}

	deviceID := pathParts[3]
	if deviceID == "" {
		http.Error(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.Devices[deviceID]
	if !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Device WebSocket upgrade failed for %s: %v", deviceID, err)
		return
	}
	defer conn.Close()

	log.Printf("Device WebSocket connected for %s", deviceID)

	// Send initial device status
	initialMessage := webtypes.WebSocketMessage{
		Type:     "device_status",
		DeviceID: deviceID,
		Data: map[string]interface{}{
			"info":   device.DeviceInfo,
			"status": device.Status,
		},
	}

	if err := conn.WriteJSON(initialMessage); err != nil {
		log.Printf("Failed to send initial device status: %v", err)
		return
	}

	// Set up ping handler to detect client disconnects
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Set initial read deadline
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))

	// Handle incoming messages in a separate goroutine
	go func() {
		defer conn.Close()

		for {
			if _, _, err := conn.NextReader(); err != nil {
				log.Printf("Device WebSocket read error for %s: %v", deviceID, err)
				return
			}
		}
	}()

	// Send periodic device status updates
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Send ping to check if client is still connected
		if err := conn.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
			log.Printf("Failed to send ping to device WebSocket %s: %v", deviceID, err)
			return
		}

		// Send device status update
		statusMessage := webtypes.WebSocketMessage{
			Type:     "device_status",
			DeviceID: deviceID,
			Data: map[string]interface{}{
				"info":   device.DeviceInfo,
				"status": device.Status,
			},
		}

		if err := conn.WriteJSON(statusMessage); err != nil {
			log.Printf("Failed to send device status update for %s: %v", deviceID, err)
			return
		}

		// If device has active WebSocket connection to SoundTouch device,
		// also send any real-time updates from that connection
		if device.WebSocket != nil && device.Status.IsConnected {
			realtimeMessage := webtypes.WebSocketMessage{
				Type:     "device_realtime",
				DeviceID: deviceID,
				Data: map[string]interface{}{
					"nowPlaying": device.Status.NowPlaying,
					"volume":     device.Status.Volume,
					"timestamp":  time.Now(),
				},
			}

			if err := conn.WriteJSON(realtimeMessage); err != nil {
				log.Printf("Failed to send realtime update for %s: %v", deviceID, err)
				return
			}
		}
	}
}
