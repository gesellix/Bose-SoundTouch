package client

import (
	"context"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gorilla/websocket"
)

// WebSocketClient handles WebSocket connections to SoundTouch devices
type WebSocketClient struct {
	client      *Client
	conn        *websocket.Conn
	connection  *webSocketConnection
	handlers    *models.WebSocketEventHandlers
	mu          sync.RWMutex
	connectMu   sync.Mutex // serializes dial attempts without blocking shutdown
	writeMu     sync.Mutex // serializes all writes; gorilla/websocket allows one concurrent writer
	connected   bool
	reconnect   bool
	ctx         context.Context
	cancel      context.CancelFunc
	logger      Logger
	bufferSize  int
	dialContext webSocketDialContext

	transportHandler    func(connected bool, generation uint64)
	transportGeneration uint64

	// pending correlates in-flight Request calls with their replies.
	pending *pendingRequests
}

type webSocketDialContext func(context.Context, string, http.Header) (*websocket.Conn, *http.Response, error)

// webSocketConnection gives each transport generation its own lifecycle so an
// old read or ping loop cannot start using a replacement connection.
type webSocketConnection struct {
	conn   *websocket.Conn
	ctx    context.Context
	cancel context.CancelFunc
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
		pending:    newPendingRequests(),
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

// OnNameUpdated sets a handler for device name update events.
func (ws *WebSocketClient) OnNameUpdated(handler models.TypedEventHandler[*models.NameUpdatedEvent]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnNameUpdated = handler
}

// OnTransportState observes authoritative connection transitions. Generation
// numbers let consumers reject callbacks that arrive out of order.
func (ws *WebSocketClient) OnTransportState(handler func(connected bool, generation uint64)) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.transportHandler = handler
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

// OnDeviceError sets a handler for root-level <errorUpdate> frames — the
// speaker's own error reports, carrying a numeric code, a symbolic name and a
// severity. OnSpecialMessage still fires for the same frame.
func (ws *WebSocketClient) OnDeviceError(handler models.TypedEventHandler[*models.ErrorUpdate]) {
	ws.mu.Lock()
	defer ws.mu.Unlock()

	ws.handlers.OnDeviceError = handler
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
	ws.connectMu.Lock()
	defer ws.connectMu.Unlock()

	ws.mu.RLock()

	if ws.connected {
		ws.mu.RUnlock()
		return fmt.Errorf("already connected")
	}

	ctx := ws.ctx
	dialContext := ws.dialContext
	ws.mu.RUnlock()

	if err := ctx.Err(); err != nil {
		return fmt.Errorf("WebSocket client is closed: %w", err)
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

	if dialContext == nil {
		// Create dialer with custom buffer sizes and "gabbo" protocol.
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
			ReadBufferSize:   config.ReadBufferSize,
			WriteBufferSize:  config.WriteBufferSize,
			Subprotocols:     []string{"gabbo"}, // Required by SoundTouch API
		}
		dialContext = dialer.DialContext
	}

	// Dial without holding the state mutex so shutdown can cancel the context
	// immediately instead of waiting for the handshake timeout.
	conn, resp, err := dialContext(ctx, wsURL.String(), nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return fmt.Errorf("failed to connect to WebSocket: %w", err)
	}

	ws.mu.Lock()
	if err := ctx.Err(); err != nil {
		ws.mu.Unlock()

		_ = conn.Close()

		return fmt.Errorf("WebSocket client closed during connect: %w", err)
	}

	if ws.connected {
		ws.mu.Unlock()

		_ = conn.Close()

		return fmt.Errorf("already connected")
	}

	connection, transportHandler, transportGeneration := ws.activateConnectionLocked(conn)
	ws.mu.Unlock()

	notifyTransportState(transportHandler, true, transportGeneration)

	go ws.readLoop(config, connection)
	go ws.pingLoop(config, connection)

	ws.logger.Printf("Connected to %s", sanitizeLog(wsURL.String()))

	return nil
}

func (ws *WebSocketClient) activateConnectionLocked(
	conn *websocket.Conn,
) (*webSocketConnection, func(bool, uint64), uint64) {
	if ws.connection != nil {
		ws.connection.cancel()
		_ = ws.connection.conn.Close()
	}

	connectionCtx, connectionCancel := context.WithCancel(ws.ctx)
	connection := &webSocketConnection{
		conn:   conn,
		ctx:    connectionCtx,
		cancel: connectionCancel,
	}

	ws.conn = conn
	ws.connection = connection
	ws.connected = true
	ws.transportGeneration++

	// Extend the read deadline on every pong so the connection survives
	// quiet periods between speaker events. Without this, the 60-second
	// read deadline in readLoop fires reliably after one ping cycle (30 s
	// ping interval + 5 s reconnect = ~65 s disconnect loop).
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	return connection, ws.transportHandler, ws.transportGeneration
}

// Disconnect closes the WebSocket connection
func (ws *WebSocketClient) Disconnect() error {
	// Cancel first so an in-progress DialContext wakes without waiting for mu.
	ws.cancel()

	ws.mu.Lock()
	wasConnected := ws.connected

	ws.reconnect = false
	if ws.connection != nil {
		ws.connection.cancel()
		ws.connection = nil
	}

	conn := ws.conn
	ws.conn = nil
	ws.connected = false

	// Wake anything waiting on a reply: the answer is never coming, and a
	// caller should hear "connection lost" now rather than time out.
	ws.pending.failAll()

	var (
		transportHandler    func(bool, uint64)
		transportGeneration uint64
	)
	if wasConnected {
		ws.transportGeneration++
		transportHandler = ws.transportHandler
		transportGeneration = ws.transportGeneration
	}
	ws.mu.Unlock()

	if conn != nil {
		err := conn.Close()

		ws.logger.Printf("Disconnected")

		notifyTransportState(transportHandler, false, transportGeneration)

		return err
	}

	notifyTransportState(transportHandler, false, transportGeneration)

	if !wasConnected {
		return fmt.Errorf("not connected")
	}

	return nil
}

// Close permanently stops this client and is idempotent. Device registries use
// it when removal races an initial dial or an automatic reconnect.
func (ws *WebSocketClient) Close() error {
	ws.cancel()

	ws.mu.Lock()
	wasConnected := ws.connected

	ws.reconnect = false
	if ws.connection != nil {
		ws.connection.cancel()
		ws.connection = nil
	}

	conn := ws.conn
	ws.conn = nil
	ws.connected = false

	// Wake anything waiting on a reply: the answer is never coming, and a
	// caller should hear "connection lost" now rather than time out.
	ws.pending.failAll()

	var (
		transportHandler    func(bool, uint64)
		transportGeneration uint64
	)
	if wasConnected {
		ws.transportGeneration++
		transportHandler = ws.transportHandler
		transportGeneration = ws.transportGeneration
	}
	ws.mu.Unlock()

	if conn == nil {
		notifyTransportState(transportHandler, false, transportGeneration)

		return nil
	}

	err := conn.Close()

	ws.logger.Printf("Disconnected")
	notifyTransportState(transportHandler, false, transportGeneration)

	return err
}

// IsConnected returns true if the WebSocket is connected
func (ws *WebSocketClient) IsConnected() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return ws.connected
}

func (ws *WebSocketClient) shouldReconnect() bool {
	ws.mu.RLock()
	defer ws.mu.RUnlock()

	return ws.reconnect
}

func notifyTransportState(handler func(bool, uint64), connected bool, generation uint64) {
	if handler != nil {
		handler(connected, generation)
	}
}

// readLoop continuously reads messages from the WebSocket connection
func (ws *WebSocketClient) readLoop(config *WebSocketConfig, connection *webSocketConnection) {
	defer func() {
		ws.mu.Lock()
		if ws.connection != connection {
			ws.mu.Unlock()
			connection.cancel()
			_ = connection.conn.Close()

			return
		}

		connection.cancel()
		ws.connection = nil
		ws.conn = nil
		ws.connected = false
		ws.pending.failAll()
		ws.transportGeneration++
		transportHandler := ws.transportHandler
		transportGeneration := ws.transportGeneration
		reconnect := ws.reconnect
		ws.mu.Unlock()

		_ = connection.conn.Close()

		notifyTransportState(transportHandler, false, transportGeneration)

		// Attempt reconnection if enabled
		if reconnect {
			go ws.attemptReconnect(config)
		}
	}()

	for {
		select {
		case <-connection.ctx.Done():
			return
		default:
		}

		// Set read deadline
		_ = connection.conn.SetReadDeadline(time.Now().Add(60 * time.Second))

		// Read message
		messageType, data, err := connection.conn.ReadMessage()
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
func (ws *WebSocketClient) pingLoop(config *WebSocketConfig, connection *webSocketConnection) {
	ticker := time.NewTicker(config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-connection.ctx.Done():
			return
		case <-ticker.C:
			active, err := ws.writePing(connection)
			if !active {
				return
			}

			if err != nil {
				ws.logger.Printf("Failed to send ping: %v", err)
				return
			}
		}
	}
}

func (ws *WebSocketClient) writePing(connection *webSocketConnection) (bool, error) {
	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()

	ws.mu.RLock()
	defer ws.mu.RUnlock()

	if connection.ctx.Err() != nil || ws.connection != connection || !ws.connected {
		return false, nil
	}

	_ = connection.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))

	return true, connection.conn.WriteMessage(websocket.PingMessage, nil)
}

// attemptReconnect attempts to reconnect to the WebSocket
func (ws *WebSocketClient) attemptReconnect(config *WebSocketConfig) {
	attempt := 0
	for ws.shouldReconnect() && (config.MaxReconnectAttempts == 0 || attempt < config.MaxReconnectAttempts) {
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
	// A reply to an outstanding Request is consumed here and never reaches
	// the event handlers: it is an answer to us, not a device notification.
	if ws.routeResponse(data) {
		ws.fireRawMessage(data, nil)
		return
	}

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

	// Call handlers if set. A device error goes to the dedicated typed
	// handler first, then to the catch-all, so a caller may register
	// either or both.
	ws.mu.RLock()
	handler := ws.handlers.OnSpecialMessage
	deviceErrHandler := ws.handlers.OnDeviceError
	ws.mu.RUnlock()

	if deviceErrHandler != nil {
		if errorUpdate := specialMessage.GetErrorUpdate(); errorUpdate != nil {
			deviceErrHandler(errorUpdate)
		}
	}

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
	case models.EventTypeBalanceUpdated:
		if handlers.OnBalanceUpdated != nil && event.BalanceUpdated != nil {
			handlers.OnBalanceUpdated(event.BalanceUpdated)
		}

		return true

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

	case models.EventTypeNameUpdated:
		if handlers.OnNameUpdated != nil && event.NameUpdated != nil {
			handlers.OnNameUpdated(event.NameUpdated)
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
		// Log the actual unmodeled element names (e.g. nowSelectionUpdated)
		// rather than an empty list; skip frames that carry no child events.
		if names := event.UnknownEventNames(); len(names) > 0 {
			sanitizedNames := make([]string, 0, len(names))
			for _, name := range names {
				sanitizedNames = append(sanitizedNames, sanitizeLog(name))
			}

			ws.logger.Printf("Received unhandled event types: %v", sanitizedNames)
		}
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
