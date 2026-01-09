package client

import (
	"context"
	"fmt"
	"log"
	"net/url"
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

func (d DefaultLogger) Printf(format string, v ...interface{}) {
	log.Printf("[WebSocket] "+format, v...)
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
	wsURL := url.URL{
		Scheme: "ws",
		Host:   fmt.Sprintf("%s:%d", ws.client.Host(), 8080), // SoundTouch WebSocket port is typically 8080
		Path:   "/",
	}

	ws.logger.Printf("Connecting to %s", wsURL.String())

	// Create dialer with custom buffer sizes
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   config.ReadBufferSize,
		WriteBufferSize:  config.WriteBufferSize,
	}

	// Establish connection
	conn, _, err := dialer.DialContext(ws.ctx, wsURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	ws.conn = conn
	ws.connected = true

	// Start background goroutines for connection management
	go ws.readLoop(config)
	go ws.pingLoop(config)

	ws.logger.Printf("Connected to %s", wsURL.String())
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
			ws.conn.Close()
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
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))

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
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
	// Parse the WebSocket event
	event, err := models.ParseWebSocketEvent(data)
	if err != nil {
		ws.logger.Printf("Failed to parse WebSocket message: %v", err)
		return
	}

	// Process each event type in the message
	ws.handleEvent(event)
}

// handleEvent dispatches events to appropriate handlers
func (ws *WebSocketClient) handleEvent(event *models.WebSocketEvent) {
	ws.mu.RLock()
	handlers := ws.handlers
	ws.mu.RUnlock()

	eventTypes := event.GetEventTypes()
	hasKnownEvent := false

	for _, eventType := range eventTypes {
		hasKnownEvent = true
		switch eventType {
		case models.EventTypeNowPlaying:
			if handlers.OnNowPlaying != nil && event.NowPlayingUpdated != nil {
				handlers.OnNowPlaying(event.NowPlayingUpdated)
			}

		case models.EventTypeVolumeUpdated:
			if handlers.OnVolumeUpdated != nil && event.VolumeUpdated != nil {
				handlers.OnVolumeUpdated(event.VolumeUpdated)
			}

		case models.EventTypeConnectionState:
			if handlers.OnConnectionState != nil && event.ConnectionStateUpdated != nil {
				handlers.OnConnectionState(event.ConnectionStateUpdated)
			}

		case models.EventTypePresetUpdated:
			if handlers.OnPresetUpdated != nil && event.PresetUpdated != nil {
				handlers.OnPresetUpdated(event.PresetUpdated)
			}

		case models.EventTypeZoneUpdated:
			if handlers.OnZoneUpdated != nil && event.ZoneUpdated != nil {
				handlers.OnZoneUpdated(event.ZoneUpdated)
			}

		case models.EventTypeBassUpdated:
			if handlers.OnBassUpdated != nil && event.BassUpdated != nil {
				handlers.OnBassUpdated(event.BassUpdated)
			}

		default:
			hasKnownEvent = false
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

	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteMessage(websocket.TextMessage, message)
}

// Wait blocks until the WebSocket connection is closed or context is cancelled
func (ws *WebSocketClient) Wait() {
	<-ws.ctx.Done()
}
