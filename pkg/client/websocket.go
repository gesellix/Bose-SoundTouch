package client

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gorilla/websocket"
)

// WebSocketClient handles WebSocket connections to SoundTouch devices
type WebSocketClient struct {
	client     *Client
	conn       *websocket.Conn
	handlers   *models.WebSocketEventHandlers
	mu         sync.RWMutex
	writeMu    sync.Mutex // serializes all writes; gorilla/websocket allows one concurrent writer
	connected  bool
	reconnect  bool
	ctx        context.Context
	cancel     context.CancelFunc
	logger     Logger
	bufferSize int
}

// Logger interface for WebSocket logging
type Logger interface {
	Printf(format string, v ...interface{})
}

// DefaultLogger uses standard log package
type DefaultLogger struct{}

// Printf implements the Logger interface by printing formatted messages with a WebSocket prefix.
func (d DefaultLogger) Printf(format string, v ...interface{}) {
	log.Printf("[WebSocket] %s", sanitizeLog(fmt.Sprintf(format, v...)))
}

// WebSocketConfig holds configuration for WebSocket client
type WebSocketConfig struct {
	// ReconnectInterval defines how long to wait between reconnection attempts
	ReconnectInterval time.Duration
	// MaxReconnectAttempts defines maximum number of reconnection attempts (0 = unlimited)
	MaxReconnectAttempts int
	// PingInterval defines how often to send ping messages to keep connection alive
	PingInterval time.Duration
	// PongTimeout defines how long to wait for pong response
	PongTimeout time.Duration
	// ReadBufferSize defines the WebSocket read buffer size
	ReadBufferSize int
	// WriteBufferSize defines the WebSocket write buffer size
	WriteBufferSize int
	// Logger for WebSocket events (nil = default logger)
	Logger Logger
}

// DefaultWebSocketConfig returns a default WebSocket configuration
func DefaultWebSocketConfig() *WebSocketConfig {
	return &WebSocketConfig{
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0, // Unlimited
		PingInterval:         30 * time.Second,
		PongTimeout:          10 * time.Second,
		ReadBufferSize:       1024,
		WriteBufferSize:      1024,
		Logger:               DefaultLogger{},
	}
}

// NewWebSocketClient creates a new WebSocket client for the given SoundTouch client
func (c *Client) NewWebSocketClient(config *WebSocketConfig) *WebSocketClient {
	if config == nil {
		config = DefaultWebSocketConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WebSocketClient{
		client:     c,
		handlers:   &models.WebSocketEventHandlers{},
		reconnect:  true,
		ctx:        ctx,
		cancel:     cancel,
		logger:     config.Logger,
		bufferSize: config.ReadBufferSize,
	}
}

// SetHandlers sets the event handlers for different WebSocket event types
func (ws *WebSocketClient) SetHandlers(handlers *models.WebSocketEventHandlers) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers = handlers
}

// OnNowPlaying sets a handler for now playing events
func (ws *WebSocketClient) OnNowPlaying(handler models.TypedEventHandler[*models.NowPlayingUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnNowPlaying = handler
}

// OnVolumeUpdated sets a handler for volume update events
func (ws *WebSocketClient) OnVolumeUpdated(handler models.TypedEventHandler[*models.VolumeUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnVolumeUpdated = handler
}

// OnConnectionState sets a handler for connection state events
func (ws *WebSocketClient) OnConnectionState(handler models.TypedEventHandler[*models.ConnectionStateUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnConnectionState = handler
}

// OnPresetUpdated sets a handler for preset update events
func (ws *WebSocketClient) OnPresetUpdated(handler models.TypedEventHandler[*models.PresetUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnPresetUpdated = handler
}

// OnZoneUpdated sets a handler for zone update events
func (ws *WebSocketClient) OnZoneUpdated(handler models.TypedEventHandler[*models.ZoneUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnZoneUpdated = handler
}

// OnGroupUpdated sets a handler for ST-10 stereo-pair update events.
// The device fans these out to both LEFT and RIGHT speakers whenever the
// pair is created, renamed, or removed, so callers will see one event per
// affected device.
func (ws *WebSocketClient) OnGroupUpdated(handler models.TypedEventHandler[*models.GroupUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnGroupUpdated = handler
}

// OnBassUpdated sets a handler for bass update events
func (ws *WebSocketClient) OnBassUpdated(handler models.TypedEventHandler[*models.BassUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnBassUpdated = handler
}

// OnUnknownEvent sets a handler for unknown events
func (ws *WebSocketClient) OnUnknownEvent(handler models.EventHandler) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnUnknownEvent = handler
}

// OnRawMessage sets a handler that fires for every incoming frame with
// the raw bytes and the result of attempting to XML-parse them. The
// typed handlers (OnNowPlaying, OnGroupUpdated, ...) still run
// afterwards on successful parses, so OnRawMessage is purely additive —
// intended for debug/observability tooling.
func (ws *WebSocketClient) OnRawMessage(handler models.RawMessageHandler) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnRawMessage = handler
}

// OnSpecialMessage sets a handler for special (non-updates) messages
func (ws *WebSocketClient) OnSpecialMessage(handler models.SpecialMessageHandler) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnSpecialMessage = handler
}

// Connect establishes a WebSocket connection to the SoundTouch device
func (ws *WebSocketClient) Connect() error {
	return ws.connectWithConfig(DefaultWebSocketConfig())
}

// ConnectWithConfig establishes a WebSocket connection with custom configuration
func (ws *WebSocketClient) ConnectWithConfig(config *WebSocketConfig) error {
	return ws.connectWithConfig(config)
}

func (ws *WebSocketClient) connectWithConfig(config *WebSocketConfig) error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if ws.connected {
		return fmt.Errorf("already connected")
	}

	// Build WebSocket URL
	// Parse the base URL to extract just the hostname
	baseURL, err := url.Parse(ws.client.BaseURL())
	if err != nil {
		return fmt.Errorf("failed to parse base URL: %w", err)
	}

	wsURL := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:8080", baseURL.Hostname()), // SoundTouch WebSocket port is typically 8080
		Path:   "/",
	}

	ws.logger.Printf("Connecting to %s", sanitizeLog(wsURL.String()))

	// Create dialer with custom buffer sizes and "gabbo" protocol
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   config.ReadBufferSize,
		WriteBufferSize:  config.WriteBufferSize,
		Subprotocols:     []string{"gabbo"}, // Required by SoundTouch API
	}

	// Establish connection
	conn, resp, err := dialer.DialContext(ws.ctx, wsURL.String(), nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	ws.conn = conn
	ws.connected = true

	// Start background goroutines for connection management
	go ws.readLoop(config)
	go ws.pingLoop(config)

	ws.logger.Printf("Connected to %s", sanitizeLog(wsURL.String()))

	return nil
}

// Disconnect closes the WebSocket connection
func (ws *WebSocketClient) Disconnect() error {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	if !ws.connected {
		return fmt.Errorf("not connected")
	}

	ws.reconnect = false
	ws.cancel() // Cancel context to stop goroutines

	if ws.conn != nil {
		err := ws.conn.Close()
		ws.conn = nil
		ws.connected = false
		ws.logger.Printf("Disconnected")

		return err
	}

	ws.connected = false

	return nil
}

// IsConnected returns true if the WebSocket is connected
func (ws *WebSocketClient) IsConnected() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return ws.connected
}

// readLoop continuously reads messages from the WebSocket connection
func (ws *WebSocketClient) readLoop(config *WebSocketConfig) {
	defer func() {
		ws.mu.Lock()

		ws.connected = false
		if ws.conn != nil {
			_ = ws.conn.Close()
			ws.conn = nil
		}

		ws.mu.Unlock()

		// Attempt reconnection if enabled
		if ws.reconnect {
			go ws.attemptReconnect(config)
		}
	}()

	for {
		select {
		case <-ws.ctx.Done():
			return
		default:
		}

		ws.mu.RLock()
		conn := ws.conn
		ws.mu.RUnlock()

		if conn == nil {
			return
		}

		// Set read deadline
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Read message
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				ws.logger.Printf("WebSocket read error: %v", err)
			}

			return
		}

		// Only process text messages
		if messageType != websocket.TextMessage {
			continue
		}

		// Parse and handle the event
		ws.handleMessage(data)
	}
}

// pingLoop sends periodic ping messages to keep the connection alive
func (ws *WebSocketClient) pingLoop(config *WebSocketConfig) {
	ticker := time.NewTicker(config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ws.ctx.Done():
			return
		case <-ticker.C:
			ws.mu.RLock()
			conn := ws.conn
			connected := ws.connected
			ws.mu.RUnlock()

			if !connected || conn == nil {
				return
			}

			// Set write deadline for ping
			ws.writeMu.Lock()
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			err := conn.WriteMessage(websocket.PingMessage, nil)
			ws.writeMu.Unlock()

			if err != nil {
				ws.logger.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

// attemptReconnect attempts to reconnect to the WebSocket
func (ws *WebSocketClient) attemptReconnect(config *WebSocketConfig) {
	attempt := 0
	for ws.reconnect && (config.MaxReconnectAttempts == 0 || attempt < config.MaxReconnectAttempts) {
		select {
		case <-ws.ctx.Done():
			return
		case <-time.After(config.ReconnectInterval):
		}

		attempt++
		ws.logger.Printf("Reconnection attempt %d", attempt)

		if err := ws.connectWithConfig(config); err != nil {
			ws.logger.Printf("Reconnection attempt %d failed: %v", attempt, err)
			continue
		}

		ws.logger.Printf("Reconnected successfully")

		return
	}

	ws.logger.Printf("Max reconnection attempts reached or reconnection disabled")
}

// handleMessage processes incoming WebSocket messages
func (ws *WebSocketClient) handleMessage(data []byte) {
	// Special (non-updates) messages take their own decode path and
	// surface raw payloads to the OnRawMessage hook from there, so
	// observers see exactly one notification per frame.
	if !ws.isUpdatesMessage(data) {
		ws.handleSpecialMessage(data)
		return
	}

	event, parseErr := models.ParseWebSocketEvent(data)

	ws.fireRawMessage(data, parseErr)

	if parseErr != nil {
		ws.logger.Printf("Failed to parse WebSocket message: %v", parseErr)
		return
	}

	ws.handleEvent(event)
}

// fireRawMessage invokes the OnRawMessage hook if one is registered.
// Kept separate so the read path doesn't have to repeat the locking
// dance for every frame.
func (ws *WebSocketClient) fireRawMessage(data []byte, parseErr error) {
	ws.mu.RLock()
	handler := ws.handlers.OnRawMessage
	ws.mu.RUnlock()

	if handler != nil {
		handler(data, parseErr)
	}
}

// handleSpecialMessage processes special (non-updates) WebSocket messages
func (ws *WebSocketClient) handleSpecialMessage(data []byte) {
	specialMessage, err := models.ParseSpecialMessage(data)

	ws.fireRawMessage(data, err)

	if err != nil {
		ws.logger.Printf("Unknown special message type: %s", sanitizeErr(err))
		ws.logger.Printf("Raw message: %s", sanitizeLog(string(data)))

		return
	}

	// Call handler if set
	ws.mu.RLock()
	handler := ws.handlers.OnSpecialMessage
	ws.mu.RUnlock()

	if handler != nil {
		handler(specialMessage)
	}
}

// isUpdatesMessage checks if the message contains an <updates> element
func (ws *WebSocketClient) isUpdatesMessage(data []byte) bool {
	// Simple check for <updates> element - this avoids full XML parsing
	// for messages we want to ignore like <SoundTouchSdkInfo>
	dataStr := string(data)
	return strings.Contains(dataStr, "<updates") && strings.Contains(dataStr, "deviceID=")
}

func (ws *WebSocketClient) dispatchTypedEvent(handlers *models.WebSocketEventHandlers, eventType models.WebSocketEventType, event *models.WebSocketEvent) bool {
	switch eventType {
	case models.EventTypeNowPlaying:
		if handlers.OnNowPlaying != nil && event.NowPlayingUpdated != nil {
			handlers.OnNowPlaying(event.NowPlayingUpdated)
		}

		return true

	case models.EventTypeVolumeUpdated:
		if handlers.OnVolumeUpdated != nil && event.VolumeUpdated != nil {
			handlers.OnVolumeUpdated(event.VolumeUpdated)
		}

		return true

	case models.EventTypeConnectionState:
		if handlers.OnConnectionState != nil && event.ConnectionStateUpdated != nil {
			handlers.OnConnectionState(event.ConnectionStateUpdated)
		}

		return true

	case models.EventTypePresetUpdated:
		if handlers.OnPresetUpdated != nil && event.PresetUpdated != nil {
			handlers.OnPresetUpdated(event.PresetUpdated)
		}

		return true

	default:
		return ws.dispatchTypedEventContinued(handlers, eventType, event)
	}
}

func (ws *WebSocketClient) dispatchTypedEventContinued(handlers *models.WebSocketEventHandlers, eventType models.WebSocketEventType, event *models.WebSocketEvent) bool {
	switch eventType {
	case models.EventTypeZoneUpdated:
		if handlers.OnZoneUpdated != nil && event.ZoneUpdated != nil {
			handlers.OnZoneUpdated(event.ZoneUpdated)
		}

		return true

	case models.EventTypeGroupUpdated:
		if handlers.OnGroupUpdated != nil && event.GroupUpdated != nil {
			handlers.OnGroupUpdated(event.GroupUpdated)
		}

		return true

	case models.EventTypeBassUpdated:
		if handlers.OnBassUpdated != nil && event.BassUpdated != nil {
			handlers.OnBassUpdated(event.BassUpdated)
		}

		return true

	case models.EventTypeRecentsUpdated:
		return true

	case models.EventTypeLanguageUpdated:
		return true

	default:
		return false
	}
}

// handleEvent dispatches events to appropriate handlers
func (ws *WebSocketClient) handleEvent(event *models.WebSocketEvent) {
	ws.mu.RLock()
	handlers := ws.handlers
	ws.mu.RUnlock()

	eventTypes := event.GetEventTypes()
	hasKnownEvent := false

	for _, eventType := range eventTypes {
		if ws.dispatchTypedEvent(handlers, eventType, event) {
			hasKnownEvent = true
		}
	}

	// Handle unknown events
	if !hasKnownEvent && handlers.OnUnknownEvent != nil {
		handlers.OnUnknownEvent(event)
	} else if !hasKnownEvent {
		ws.logger.Printf("Received unknown event types: %v", eventTypes)
	}
}

// SendMessage sends a message to the WebSocket (if needed for future functionality)
func (ws *WebSocketClient) SendMessage(message []byte) error {
	ws.mu.RLock()
	conn := ws.conn
	connected := ws.connected
	ws.mu.RUnlock()

	if !connected || conn == nil {
		return fmt.Errorf("not connected")
	}

	ws.writeMu.Lock()
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	err := conn.WriteMessage(websocket.TextMessage, message)
	ws.writeMu.Unlock()

	return err
}

// PairWithAccount sends a request to pair the device with a specific account
func (ws *WebSocketClient) PairWithAccount(accountID, userAuthToken string) error {
	request := models.PairDeviceWithAccount{
		AccountID:     accountID,
		UserAuthToken: userAuthToken,
	}

	data, err := xml.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal pairing request: %w", err)
	}

	ws.logger.Printf("Sending PairDeviceWithAccount for account %s", sanitizeLog(accountID))

	return ws.SendMessage(data)
}

// UnPairFromAccount sends a request to unpair the device from its account
func (ws *WebSocketClient) UnPairFromAccount() error {
	request := models.UnPairDeviceWithAccount{}

	data, err := xml.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal unpairing request: %w", err)
	}

	ws.logger.Printf("Sending UnPairDeviceWithAccount")

	return ws.SendMessage(data)
}

// Wait blocks until the WebSocket connection is closed or context is cancelled
func (ws *WebSocketClient) Wait() {
	<-ws.ctx.Done()
}
