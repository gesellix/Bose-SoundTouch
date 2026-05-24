// Package soundtouchweb contains WebSocket handlers for real-time communication.
package soundtouchweb

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
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

	// Send current discovery status
	if ds, ok := app.discoveryStatus.Load().(*webtypes.DiscoveryStatus); ok {
		if err := conn.WriteJSON(webtypes.WebSocketMessage{
			Type: "discovery_status",
			Data: ds,
		}); err != nil {
			log.Printf("Failed to send initial discovery status: %v", err)
			return
		}
	}

	// Send initial device list
	snapshot := app.DeviceSnapshot()
	devices := make(map[string]interface{}, len(snapshot))

	for _, entry := range snapshot {
		devices[entry.ID] = map[string]interface{}{
			"info":     entry.Device.DeviceInfo,
			"status":   entry.Device.Status(),
			"lastSeen": entry.Device.LastSeen,
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
		for _, entry := range app.DeviceSnapshot() {
			status := entry.Device.Status()
			if status.IsConnected {
				statusMessage := webtypes.WebSocketMessage{
					Type:     "status_update",
					DeviceID: entry.ID,
					Data:     status,
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
// and keeps it alive: on disconnect or connect failure, it reconnects
// with exponential backoff (1 s → 30 s cap, reset after each successful
// connect). The goroutine runs for the lifetime of the device entry,
// so status flows from the speaker keep streaming through transient
// network blips, speaker reboots, and idle timeouts.
//
// conn.WebSocket is only updated on a successful (re)connect, never
// cleared, so the duplicate-spawn guards at the callsites (which check
// `if device.WebSocket == nil`) stay correct — once this goroutine is
// running for a device, no second one is needed.
func (app *WebApp) ConnectDeviceWebSocket(deviceID string, conn *webtypes.DeviceConnection) {
	// Skip WebSocket connection if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)

	backoff := initialBackoff

	for {
		wsClient := conn.Client.NewWebSocketClient(nil)

		// Setup event handlers. Each handler funnels its change through
		// UpdateStatus so concurrent events and the periodic poller
		// (UpdateDeviceStatus) cannot lose each other's writes.
		wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
			conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
				s.NowPlaying = &event.NowPlaying
				s.LastActivity = time.Now()
			})
		})

		wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
			conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
				s.Volume = &event.Volume
				s.LastActivity = time.Now()
			})
		})

		wsClient.OnConnectionState(func(event *models.ConnectionStateUpdatedEvent) {
			conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
				s.IsConnected = event.ConnectionState.IsConnected()
				s.LastActivity = time.Now()
			})
		})

		wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
			conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
				s.Presets = &event.Presets
				s.LastActivity = time.Now()
			})
		})

		if err := wsClient.Connect(); err != nil {
			log.Printf("Failed to connect WebSocket for device %s: %v (retrying in %s)", sanitizeLog(deviceID), err, backoff)
			time.Sleep(backoff)

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			continue
		}

		conn.WebSocket = wsClient

		conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
			s.IsConnected = true
		})

		log.Printf("WebSocket connected for device %s", sanitizeLog(deviceID))

		// Reset backoff after a successful connect so the next failure
		// starts at the lowest cadence again.
		backoff = initialBackoff

		// Block until the device-side WebSocket disconnects.
		wsClient.Wait()

		conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
			s.IsConnected = false
		})

		log.Printf("WebSocket disconnected for device %s — reconnecting in %s", sanitizeLog(deviceID), backoff)
		time.Sleep(backoff)

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// UpdateDeviceStatus fetches current status from the device.
//
// Network calls run outside the atomic merge so the CAS loop in
// UpdateStatus stays fast and doesn't retry slow IO. WebSocket event
// handlers running concurrently are not lost: their UpdateStatus
// runs against whichever snapshot they observe, and the merge below
// sees their changes when it CAS-loops onto the latest status.
func (app *WebApp) UpdateDeviceStatus(_ string, conn *webtypes.DeviceConnection) {
	// Skip status update if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

	// Phase 1: slow network fetches. Local vars only, no shared state
	// is touched yet. Errors are recorded so the merge below can tell
	// "field N stayed unchanged" apart from "field N got refreshed".
	nowPlaying, nowPlayingErr := conn.Client.GetNowPlaying()
	volume, volumeErr := conn.Client.GetVolume()
	presets, presetsErr := conn.Client.GetPresets()
	sources, sourcesErr := conn.Client.GetSources()
	bass, bassErr := conn.Client.GetBass()

	// Phase 2: fast merge. Only fields we successfully fetched
	// overwrite; everything else keeps the value other goroutines may
	// have just written.
	conn.UpdateStatus(func(s *webtypes.DeviceStatus) {
		statusUpdated := false

		if nowPlayingErr == nil {
			s.NowPlaying = nowPlaying
			statusUpdated = true
		}

		if volumeErr == nil {
			s.Volume = volume
			statusUpdated = true
		}

		if presetsErr == nil {
			s.Presets = presets
			statusUpdated = true
		}

		if sourcesErr == nil {
			s.Sources = sources
			statusUpdated = true
		}

		if bassErr == nil {
			s.Bass = bass
			statusUpdated = true
		}

		// Mark as connected if we successfully got at least one
		// status from this round. Mirrors prior behaviour.
		s.IsConnected = statusUpdated
		s.LastActivity = time.Now()
	})
}

// HandleDeviceWebSocket handles individual device WebSocket connections for real-time device-specific updates
func (app *WebApp) HandleDeviceWebSocket(w http.ResponseWriter, r *http.Request) {
	deviceID := chi.URLParam(r, "id")
	if deviceID == "" {
		http.Error(w, "Device ID required", http.StatusBadRequest)
		return
	}

	device, exists := app.GetDevice(deviceID)
	if !exists {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Device WebSocket upgrade failed for %s: %v", sanitizeLog(deviceID), err)
		return
	}
	defer conn.Close()

	log.Printf("Device WebSocket connected for %s", sanitizeLog(deviceID))

	// Send initial device status
	initialMessage := webtypes.WebSocketMessage{
		Type:     "device_status",
		DeviceID: deviceID,
		Data: map[string]interface{}{
			"info":   device.DeviceInfo,
			"status": device.Status(),
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
				log.Printf("Device WebSocket read error for %s: %v", sanitizeLog(deviceID), err)
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
			log.Printf("Failed to send ping to device WebSocket %s: %v", sanitizeLog(deviceID), err)
			return
		}

		// Send device status update
		status := device.Status()
		statusMessage := webtypes.WebSocketMessage{
			Type:     "device_status",
			DeviceID: deviceID,
			Data: map[string]interface{}{
				"info":   device.DeviceInfo,
				"status": status,
			},
		}

		if err := conn.WriteJSON(statusMessage); err != nil {
			log.Printf("Failed to send device status update for %s: %v", sanitizeLog(deviceID), err)
			return
		}

		// If device has active WebSocket connection to SoundTouch device,
		// also send any real-time updates from that connection
		if device.WebSocket != nil && status.IsConnected {
			realtimeMessage := webtypes.WebSocketMessage{
				Type:     "device_realtime",
				DeviceID: deviceID,
				Data: map[string]interface{}{
					"nowPlaying": status.NowPlaying,
					"volume":     status.Volume,
					"timestamp":  time.Now(),
				},
			}

			if err := conn.WriteJSON(realtimeMessage); err != nil {
				log.Printf("Failed to send realtime update for %s: %v", sanitizeLog(deviceID), err)
				return
			}
		}
	}
}
