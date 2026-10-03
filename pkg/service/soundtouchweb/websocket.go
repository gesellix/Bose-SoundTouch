// Package soundtouchweb contains WebSocket handlers for real-time communication.
package soundtouchweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

const defaultWebSocketWriteTimeout = 2 * time.Second

// checkWebSocketOrigin is gorilla's own same-origin default (fail the
// handshake only when an Origin header is present and doesn't match the
// request host), except the comparison ignores port -- see
// sameHostIgnoringPort for why.
func checkWebSocketOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}

	u, err := url.Parse(origin)
	if err != nil {
		return false
	}

	return sameHostIgnoringPort(u.Host, r.Host)
}

// sameHostIgnoringPort reports whether a and b name the same hostname,
// ignoring any port suffix.
//
// A reverse proxy commonly forwards a portless Host header regardless of
// what public port it's listening on -- nginx's $host variable never
// includes the port, unlike $http_host -- while a browser's Origin header
// for a WebSocket handshake keeps an explicit, non-default port. Comparing
// ports as well as host would reject that handshake whenever the proxy's
// public listener uses a non-default port (e.g. :8443, a realistic shape
// for multi-service hosting behind one proxy), even though the hostname
// genuinely matches. This project's documented reverse-proxy config
// (HTTPS-SETUP.md) relies on exactly that forwarding behavior.
//
// Trade-off: ignoring port also means two unrelated services sharing one
// hostname on different ports would not be distinguished by this check
// alone. Accepted here since gorilla's own default already doesn't check
// scheme either, and the alternative is silently breaking the documented,
// encouraged reverse-proxy deployment.
func sameHostIgnoringPort(a, b string) bool {
	if h, _, err := net.SplitHostPort(a); err == nil {
		a = h
	}

	if h, _, err := net.SplitHostPort(b); err == nil {
		b = h
	}

	return strings.EqualFold(a, b)
}

type webSocketWriter interface {
	SetWriteDeadline(time.Time) error
	WriteJSON(interface{}) error
	WriteMessage(int, []byte) error
}

// webSocketWriteBatch gives every write a fresh deadline. A stalled client is
// bounded without passing an already-expired deadline to later healthy clients.
type webSocketWriteBatch struct {
	timeout time.Duration
}

func (batch webSocketWriteBatch) writeJSON(conn webSocketWriter, value interface{}) error {
	if err := conn.SetWriteDeadline(time.Now().Add(batch.timeout)); err != nil {
		return err
	}

	return conn.WriteJSON(value)
}

func (batch webSocketWriteBatch) writeMessage(conn webSocketWriter, messageType int, data []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(batch.timeout)); err != nil {
		return err
	}

	return conn.WriteMessage(messageType, data)
}

// writeTimeout returns the configured browser WebSocket write timeout, or
// defaultWebSocketWriteTimeout when unset.
func (app *WebApp) writeTimeout() time.Duration {
	if app.webSocketWriteTimeout > 0 {
		return app.webSocketWriteTimeout
	}

	return defaultWebSocketWriteTimeout
}

// errConnUnregistered is returned by withConnWrite when conn is not (or is
// no longer) present in WSClients -- e.g. it was concurrently removed.
var errConnUnregistered = errors.New("websocket connection is not registered")

// withConnWrite serializes writes to a single browser WebSocket connection.
// Gorilla permits only one concurrent writer per connection (vendored
// gorilla/websocket@v1.5.3 doc.go); this is the per-connection replacement
// for the removed global write mutex, so a stalled client's writes never
// block writes to any OTHER connection or unrelated HTTP handlers.
func (app *WebApp) withConnWrite(conn *websocket.Conn, write func(webSocketWriteBatch) error) error {
	app.WSMutex.RLock()
	mu := app.WSClients[conn]
	app.WSMutex.RUnlock()

	if mu == nil {
		return errConnUnregistered
	}

	mu.Lock()
	defer mu.Unlock()

	return write(webSocketWriteBatch{timeout: app.writeTimeout()})
}

// withDiscoveryStatusWrite keeps the authoritative discovery state ordered
// with the frame that publishes it to browser clients, across concurrent
// publications. It is deliberately independent of connection registration
// -- see discoveryPublishMu's doc comment on WebApp.
func (app *WebApp) withDiscoveryStatusWrite(
	status *webtypes.DiscoveryStatus,
	write func(webSocketWriteBatch, []*websocket.Conn) error,
) error {
	app.discoveryPublishMu.Lock()
	defer app.discoveryPublishMu.Unlock()

	app.discoveryStatus.Store(status)

	return write(webSocketWriteBatch{timeout: app.writeTimeout()}, app.globalWebSocketClients())
}

func (app *WebApp) globalWebSocketClients() []*websocket.Conn {
	app.WSMutex.RLock()
	defer app.WSMutex.RUnlock()

	clients := make([]*websocket.Conn, 0, len(app.WSClients))
	for client := range app.WSClients {
		clients = append(clients, client)
	}

	return clients
}

// removeGlobalWebSocketClient unregisters client and closes it, holding its
// own write lock across Close() so this never races an in-flight or
// about-to-start write to the same connection.
func (app *WebApp) removeGlobalWebSocketClient(client *websocket.Conn) {
	app.WSMutex.Lock()

	mu, ok := app.WSClients[client]
	if ok {
		delete(app.WSClients, client)
		app.watcherLeftLocked()
	}
	app.WSMutex.Unlock()

	if !ok {
		return
	}

	mu.Lock()
	_ = client.Close()
	mu.Unlock()
}

// registerGlobalWebSocket creates conn's write lock already held, inserts it
// into WSClients, then sends its initial frames -- so a broadcast racing
// right after insertion blocks on this connection's own lock until the
// initial snapshot is complete, with no shared lock required.
func (app *WebApp) registerGlobalWebSocket(conn *websocket.Conn) error {
	connMu := &sync.Mutex{}
	connMu.Lock()
	defer connMu.Unlock()

	app.WSMutex.Lock()
	if _, known := app.WSClients[conn]; !known {
		app.watcherJoinedLocked()
	}

	app.WSClients[conn] = connMu
	app.WSMutex.Unlock()

	batch := webSocketWriteBatch{timeout: app.writeTimeout()}

	if ds, ok := app.discoveryStatus.Load().(*webtypes.DiscoveryStatus); ok {
		if err := batch.writeJSON(conn, webtypes.WebSocketMessage{
			Type: "discovery_status",
			Data: ds,
		}); err != nil {
			return err
		}
	}

	return batch.writeJSON(conn, webtypes.WebSocketMessage{
		Type: "devices",
		Data: app.deviceViewSnapshot(),
	})
}

// queueBroadcastIfChanged schedules a device-list broadcast when an
// apply-event/apply-poll call changed something dashboard-visible. Every
// such call site (including Group's own, separately-ordered path) routes
// through this so a future change to the shared policy has one place to
// change.
func (app *WebApp) queueBroadcastIfChanged(changed bool) bool {
	if changed {
		app.QueueDeviceListBroadcast()
	}

	return changed
}

// applySpeakerStatusEvent stores one speaker event for field and immediately
// publishes a fresh device projection when its dashboard-visible payload
// changed. Ordering against a concurrent poll of the same field is handled
// by ApplyFieldEvent; a different field's poll or event is never affected.
func (app *WebApp) applySpeakerStatusEvent(
	conn *webtypes.DeviceConnection,
	field webtypes.StatusField,
	mut func(*webtypes.DeviceStatus) bool,
) bool {
	changed := false

	conn.ApplyFieldEvent(field, func(status *webtypes.DeviceStatus) {
		changed = mut(status)
	})

	return app.queueBroadcastIfChanged(changed)
}

func (app *WebApp) applyNowPlayingEvent(
	conn *webtypes.DeviceConnection,
	nowPlaying *models.NowPlaying,
) bool {
	return app.applySpeakerStatusEvent(conn, webtypes.FieldNowPlaying, func(status *webtypes.DeviceStatus) bool {
		changed := !reflect.DeepEqual(status.NowPlaying, nowPlaying)
		status.NowPlaying = nowPlaying
		status.LastActivity = time.Now()

		return changed
	})
}

func (app *WebApp) applyVolumeEvent(
	conn *webtypes.DeviceConnection,
	volume *models.Volume,
) bool {
	return app.applySpeakerStatusEvent(conn, webtypes.FieldVolume, func(status *webtypes.DeviceStatus) bool {
		changed := !reflect.DeepEqual(status.Volume, volume)
		status.Volume = volume
		status.LastActivity = time.Now()

		return changed
	})
}

func (app *WebApp) applyConnectionStateEvent(
	conn *webtypes.DeviceConnection,
	connected bool,
) bool {
	return app.applySpeakerStatusEvent(conn, webtypes.FieldConnectivity, func(status *webtypes.DeviceStatus) bool {
		changed := status.IsConnected != connected
		status.IsConnected = connected
		status.LastActivity = time.Now()

		return changed
	})
}

func (app *WebApp) applyPresetEvent(
	conn *webtypes.DeviceConnection,
	presets *models.Presets,
) bool {
	return app.applySpeakerStatusEvent(conn, webtypes.FieldPresets, func(status *webtypes.DeviceStatus) bool {
		changed := !reflect.DeepEqual(status.Presets, presets)
		status.Presets = presets
		status.LastActivity = time.Now()

		return changed
	})
}

// refreshBalance reserves an ordering barrier synchronously, then starts one
// authoritative read worker. Bursts coalesce into one active and one trailing
// read; every hint still invalidates an older in-flight result.
func (app *WebApp) refreshBalance(deviceID string, conn *webtypes.DeviceConnection) {
	if conn.Client == nil {
		return
	}

	conn.InvalidateField(webtypes.FieldBalance)

	target, generation, valid := conn.BeginBalanceRead()
	if !valid {
		return
	}

	if !conn.BeginBalanceRefresh() {
		return
	}

	go app.completeBalanceRefresh(deviceID, conn, target, generation)
}

func (app *WebApp) completeBalanceRefresh(
	deviceID string,
	conn *webtypes.DeviceConnection,
	target webtypes.BalanceTarget,
	generation uint64,
) {
	valid := true
	for {
		if valid {
			balance, err := conn.Client.GetBalance()
			if err != nil {
				log.Printf("Speaker %s: balance read failed: %v", sanitizeLog(deviceID), sanitizeLog(err.Error()))
			} else if err = validBalanceReadback(balance, target.HardwareID); err != nil {
				log.Printf("Speaker %s: balance readback rejected: %v", sanitizeLog(deviceID), sanitizeLog(err.Error()))
			} else if app.applyAcousticRead(deviceID, conn, target.HardwareID, func() bool {
				_, applied := conn.ApplyBalanceRead(target, generation, balance)
				return applied
			}) {
				app.QueueDeviceListBroadcast()
			}
		}

		if !conn.EndBalanceRefresh() {
			return
		}

		target, generation, valid = conn.BeginBalanceRead()
	}
}

// isStandbySource reports whether a now-playing source means the speaker is
// idle. An empty source counts: it is what we hold before the first reading.
func isStandbySource(source string) bool {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case "", "STANDBY", "INVALID_SOURCE":
		return true
	default:
		return false
	}
}

// refreshBass treats every bassUpdated frame as an invalidation hint and
// reserves its barrier before launching asynchronous I/O.
func (app *WebApp) refreshBass(deviceID string, conn *webtypes.DeviceConnection) {
	if conn.Client == nil {
		return
	}

	conn.InvalidateField(webtypes.FieldBass)

	if !conn.BeginBassRefresh() {
		return
	}

	go app.completeBassRefresh(deviceID, conn, conn.BeginFieldPoll(webtypes.FieldBass))
}

func (app *WebApp) completeBassRefresh(
	deviceID string,
	conn *webtypes.DeviceConnection,
	generation uint64,
) {
	for {
		capabilities, capabilityErr := conn.Client.GetBassCapabilities()

		bass, bassErr := conn.Client.GetBass()
		if capabilityErr == nil {
			capabilityErr = validBassCapabilityProjection(capabilities, acousticDeviceID(conn))
		}

		unavailable := capabilityErr == nil && !capabilities.BassAvailable
		if unavailable {
			bassErr = nil
		}

		if capabilityErr != nil || bassErr != nil {
			log.Printf("Speaker %s: bass re-read failed: capabilities=%v bass=%v",
				sanitizeLog(deviceID), sanitizeLog(fmt.Sprint(capabilityErr)), sanitizeLog(fmt.Sprint(bassErr)))
		} else if err := validBassReadback(bass, capabilities, acousticDeviceID(conn)); !unavailable && err != nil {
			log.Printf("Speaker %s: bass readback rejected: %v", sanitizeLog(deviceID), sanitizeLog(err.Error()))
		} else if app.applyAcousticRead(deviceID, conn, acousticDeviceID(conn), func() bool {
			return conn.CompleteFieldPoll(webtypes.FieldBass, generation, func(status *webtypes.DeviceStatus) {
				status.BassCapabilities = capabilities
				if !unavailable {
					status.Bass = bass
				}

				status.LastActivity = time.Now()
			})
		}) {
			app.QueueDeviceListBroadcast()
		}

		if !conn.EndBassRefresh() {
			return
		}

		generation = conn.BeginFieldPoll(webtypes.FieldBass)
	}
}

// refreshPresets re-reads /presets after a payload-free presetsUpdated signal.
func (app *WebApp) refreshPresets(deviceID string, conn *webtypes.DeviceConnection) {
	if conn.Client == nil {
		return
	}

	presets, err := conn.Client.GetPresets()
	if err != nil {
		log.Printf("Speaker %s: presets re-read failed: %v", sanitizeLog(deviceID), sanitizeLog(err.Error()))

		return
	}

	app.applyPresetEvent(conn, presets)
}

// registerDeviceWebSocketClient gives conn its own write-serialization lock,
// mirroring registerGlobalWebSocket's role for the browser-wide pool. Unlike
// registerGlobalWebSocket, there are no initial frames to send under it --
// callers install the lock before their first write.
func (app *WebApp) registerDeviceWebSocketClient(conn webSocketWriter) {
	app.DeviceWSMutex.Lock()
	if _, known := app.DeviceWSClients[conn]; !known {
		app.watcherJoinedLocked()
	}

	app.DeviceWSClients[conn] = &sync.Mutex{}
	app.DeviceWSMutex.Unlock()
}

// removeDeviceWebSocketClient unregisters conn. Unlike
// removeGlobalWebSocketClient, callers close the underlying connection
// themselves (HandleDeviceWebSocket already does via its own defer), so this
// only needs to drop the registry entry.
func (app *WebApp) removeDeviceWebSocketClient(conn webSocketWriter) {
	app.DeviceWSMutex.Lock()
	if _, known := app.DeviceWSClients[conn]; known {
		delete(app.DeviceWSClients, conn)
		app.watcherLeftLocked()
	}
	app.DeviceWSMutex.Unlock()
}

// withDeviceConnWrite is withConnWrite for the per-device status pool.
func (app *WebApp) withDeviceConnWrite(conn webSocketWriter, write func(webSocketWriteBatch) error) error {
	app.DeviceWSMutex.RLock()
	mu := app.DeviceWSClients[conn]
	app.DeviceWSMutex.RUnlock()

	if mu == nil {
		return errConnUnregistered
	}

	mu.Lock()
	defer mu.Unlock()

	return write(webSocketWriteBatch{timeout: app.writeTimeout()})
}

func (app *WebApp) deviceWebSocketClients() []webSocketWriter {
	app.DeviceWSMutex.RLock()
	defer app.DeviceWSMutex.RUnlock()

	clients := make([]webSocketWriter, 0, len(app.DeviceWSClients))
	for client := range app.DeviceWSClients {
		clients = append(clients, client)
	}

	return clients
}

// awaitPriorGlobalWebSocketWrites is an ordering barrier across both browser
// WebSocket connection pools (the global device-list feed and per-device
// status feeds). Once it returns, any write already in flight on any
// currently-registered connection in either pool has completed, so a caller
// that just applied a fresh projection is guaranteed a later write captures
// it rather than racing a stale one still being sent.
func (app *WebApp) awaitPriorGlobalWebSocketWrites() {
	for _, client := range app.globalWebSocketClients() {
		_ = app.withConnWrite(client, func(webSocketWriteBatch) error { return nil })
	}

	for _, client := range app.deviceWebSocketClients() {
		_ = app.withDeviceConnWrite(client, func(webSocketWriteBatch) error { return nil })
	}
}

// HandleWebSocket handles WebSocket connections for real-time updates
func (app *WebApp) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := app.Upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade failed: %v", err)
		return
	}

	defer func() {
		app.removeGlobalWebSocketClient(conn)
	}()

	// Register and send initial frames under the same write lock used by
	// broadcasts and periodic updates. No other goroutine can write this
	// connection before its initial snapshot is complete.
	if err := app.registerGlobalWebSocket(conn); err != nil {
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
		if err := app.withConnWrite(conn, func(batch webSocketWriteBatch) error {
			if err := batch.writeMessage(conn, websocket.PingMessage, []byte{}); err != nil {
				return err
			}

			// Capture after taking this connection's writer lock so a newer
			// broadcast to this same connection cannot be followed by a
			// periodic frame captured from older state.
			for _, message := range app.periodicPlayerMessages() {
				if err := batch.writeJSON(conn, message); err != nil {
					return err
				}
			}

			return nil
		}); err != nil {
			log.Printf("Failed to send periodic WebSocket update: %v", err)
			return
		}
	}
}

// periodicPlayerMessages refreshes the projected inventory while retaining
// the established per-device status_update stream for API clients.
func (app *WebApp) periodicPlayerMessages() []webtypes.WebSocketMessage {
	snapshot := captureDeviceProjectionEntries(app.DeviceSnapshot())
	messages := []webtypes.WebSocketMessage{{
		Type: "devices",
		Data: projectCapturedDeviceEntries(snapshot),
	}}

	for _, entry := range snapshot {
		if entry.Status == nil || !entry.Status.IsConnected {
			continue
		}

		messages = append(messages, webtypes.WebSocketMessage{
			Type:     "status_update",
			DeviceID: entry.ID,
			Data:     entry.Status,
		})
	}

	return messages
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

// ConnectDeviceWebSocket starts the single event-transport supervisor for a
// device. Initial connection failures are retried here; after the first
// success WebSocketClient owns transport reconnects and this supervisor
// observes their state until the device is removed.
//
// It deliberately takes no context: it outlives any request, and the
// connection's own lifetime (conn.Done) is the scope that matters. Work it
// starts derives its context from that rather than inheriting a caller's.
//
// It also outlives the browsers that caused it to open. Unlike the status
// poll (issue 766), it is left running when nobody watches: an established
// speaker WebSocket costs one idle connection and no requests, its events
// keep the cache correct for REST clients in between refreshes (zoneUpdated
// even triggers its own authoritative refresh), and closing it would need a
// reopen path that loses whatever the speaker reports in between.
func (app *WebApp) ConnectDeviceWebSocket(deviceID string, conn *webtypes.DeviceConnection) {
	// Skip WebSocket connection if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

	if !conn.TryStartWebSocketLoop() {
		return
	}
	defer conn.FinishWebSocketLoop()

	const (
		initialBackoff = 1 * time.Second
		maxBackoff     = 30 * time.Second
	)

	backoff := initialBackoff

	// Tracks the last now_playing source seen so a speaker stuck reporting an
	// error source is logged once per transition into it, not on every event.
	var prevSource string

	for {
		// Stop if the device was removed from the registry (conn.Close()).
		select {
		case <-conn.Done():
			return
		default:
		}

		wsClient := conn.Client.NewWebSocketClient(nil)

		// Setup event handlers. Each handler funnels its change through
		// UpdateStatus so concurrent events and the periodic poller
		// (UpdateDeviceStatus) cannot lose each other's writes.
		wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
			prevSource = app.handleNowPlayingUpdatedEvent(deviceID, conn, prevSource, event)
		})

		wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
			activity := time.Now()

			app.applyVolumeEvent(conn, &event.Volume)
			conn.MarkEventStreamActivity(activity)
		})

		wsClient.OnConnectionState(func(event *models.ConnectionStateUpdatedEvent) {
			if !speakerConnectionEventMatches(conn, event.DeviceID) {
				log.Printf("Ignoring connection state for mismatched device %s on %s",
					sanitizeLog(event.DeviceID), sanitizeLog(deviceID))

				return
			}

			app.applyConnectionStateEvent(conn, event.IsConnected())
			conn.ApplySpeakerConnectionEvent(webtypes.SpeakerConnectionState{
				State:  event.State,
				Up:     event.Up,
				Signal: event.Signal,
			}, time.Now())
		})

		// Device errors are the speaker's own diagnosis of a failure —
		// code, symbolic name, severity. Logging them costs nothing and is
		// exactly the evidence issue reports keep lacking (GH-701).
		wsClient.OnDeviceError(func(event *models.ErrorUpdate) {
			log.Printf("Speaker %s reported error %s (%s) severity=%s: %s",
				sanitizeLog(deviceID),
				sanitizeLog(event.Error.Name),
				sanitizeLog(event.Error.Value),
				sanitizeLog(event.Error.Severity),
				sanitizeLog(strings.TrimSpace(event.Error.Text)),
			)
		})

		// balanceUpdated carries no payload — it is a signal to re-read, not
		// a value.
		wsClient.OnBalanceUpdated(func(_ *models.BalanceUpdatedEvent) {
			conn.MarkEventStreamActivity(time.Now())
			app.refreshBalance(deviceID, conn)
		})

		wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
			activity := time.Now()

			conn.MarkEventStreamActivity(activity)

			// The speaker sends this element both with the full list and as a
			// bare signal. Applying the empty case as data would blank a
			// perfectly good preset list, so re-read instead.
			if !event.HasPayload() {
				go app.refreshPresets(deviceID, conn)

				return
			}

			app.applyPresetEvent(conn, event.Presets)
		})

		wsClient.OnBassUpdated(func(_ *models.BassUpdatedEvent) {
			activity := time.Now()

			conn.MarkEventStreamActivity(activity)

			app.refreshBass(deviceID, conn)
		})

		wsClient.OnGroupUpdated(func(event *models.GroupUpdatedEvent) {
			app.applyGroupUpdatedEvent(conn, event)

			// Creating or tearing down a pair flips whether balance exists
			// here at all, so the reading has to follow the group.
			app.refreshBalance(deviceID, conn)
		})

		wsClient.OnZoneUpdated(func(event *models.ZoneUpdatedEvent) {
			conn.MarkEventStreamActivity(time.Now())

			eventDeviceID := zoneEventDeviceID(event.DeviceID, conn)

			refreshes := app.reserveZoneRefreshesAfterEvent(eventDeviceID, event.Zone.Master)
			for _, refresh := range refreshes {
				refresh := refresh
				go app.completeAuthoritativeZoneRefresh(refresh)
			}
		})

		wsClient.OnNameUpdated(func(event *models.NameUpdatedEvent) {
			conn.MarkEventStreamActivity(time.Now())
			conn.ApplyNameEvent(event.Name.Value)
		})

		wsClient.OnTransportState(func(connected bool, generation uint64) {
			// ObserveEventStreamTransport already derives status.IsConnected
			// (via applyConnectivityLocked, alongside Connectivity/
			// HTTPReachable/WebSocketConnected) from this same call. Do NOT
			// also route it through applyConnectionStateEvent/
			// ApplyFieldEvent(FieldConnectivity, ...): that unconditionally
			// bumps FieldConnectivity's applied generation past whatever an
			// in-flight HTTP poll already reserved, so a transient transport
			// blip would silently discard a concurrently-completing,
			// genuinely successful poll's IsConnected=true merge.
			if !conn.ObserveEventStreamTransport(generation, connected, time.Now()) {
				return
			}

			if connected {
				if generation > 1 {
					log.Printf("WebSocket reconnected for device %s", sanitizeLog(deviceID))
					go app.UpdateDeviceStatus(deviceID, conn)
				}
			} else {
				log.Printf("WebSocket transport disconnected for device %s", sanitizeLog(deviceID))
			}
		})

		published, err := publishAndConnectDeviceWebSocket(conn, wsClient, wsClient.Connect)
		if !published {
			return
		}

		if err != nil {
			log.Printf("Failed to connect WebSocket for device %s: %v (retrying in %s)", sanitizeLog(deviceID), err, backoff)

			if sleepOrDone(conn, backoff) {
				return
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			continue
		}

		log.Printf("WebSocket connected for device %s", sanitizeLog(deviceID))

		// Fetch current state immediately: speakers do not replay events on
		// new WebSocket connections, so anything that changed while we were
		// disconnected would otherwise stay stale until the next WS event.
		go app.UpdateDeviceStatus(deviceID, conn)

		<-conn.Done()

		return
	}
}

func (app *WebApp) handleNowPlayingUpdatedEvent(
	deviceID string,
	conn *webtypes.DeviceConnection,
	previousSource string,
	event *models.NowPlayingUpdatedEvent,
) string {
	activity := time.Now()
	nowPlaying := &event.NowPlaying

	// A /select returns 200 even when the source is rejected; the failure
	// shows up here as a transition to an error source. Log it so it lands in
	// a diagnostic export without needing a live trace.
	if nowPlaying.Source != previousSource && isErrorSource(nowPlaying.Source) {
		logNowPlayingError(deviceID, nowPlaying.Source, nowPlaying.SourceAccount)
	}

	// Waking up can flip whether the speaker answers for balance at all, and a
	// reading taken while it was asleep would have been stored as "no balance
	// here". Re-read on the transition out of standby, once, rather than on
	// every event.
	if nowPlaying.Source != previousSource &&
		isStandbySource(previousSource) &&
		!isStandbySource(nowPlaying.Source) {
		app.refreshBalance(deviceID, conn)
	}

	app.applyNowPlayingEvent(conn, nowPlaying)
	conn.MarkEventStreamActivity(activity)

	return nowPlaying.Source
}

func publishAndConnectDeviceWebSocket(
	conn *webtypes.DeviceConnection,
	wsClient *client.WebSocketClient,
	connect func() error,
) (bool, error) {
	if !conn.SetWebSocket(wsClient) {
		return false, nil
	}

	if err := connect(); err != nil {
		conn.ClearWebSocket(wsClient)
		_ = wsClient.Close()

		return true, err
	}

	return true, nil
}

func speakerConnectionEventMatches(conn *webtypes.DeviceConnection, eventDeviceID string) bool {
	eventDeviceID = strings.TrimSpace(eventDeviceID)
	if eventDeviceID == "" {
		return true
	}

	info := conn.Info()
	if info == nil || strings.TrimSpace(info.DeviceID) == "" {
		return false
	}

	return strings.EqualFold(eventDeviceID, strings.TrimSpace(info.DeviceID))
}

// sleepOrDone waits for d to elapse or for the connection to be closed,
// whichever comes first. It returns true if the connection was closed
// (the caller should stop), false if the timer fired normally.
func sleepOrDone(conn *webtypes.DeviceConnection, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-timer.C:
		return false
	case <-conn.Done():
		return true
	}
}

// UpdateDeviceStatus fetches current status from the device.
//
// Network calls run outside any atomic merge so slow I/O never blocks a
// concurrent CAS retry. Each field (NowPlaying/Name/Volume/Presets/Sources/
// Bass, plus derived connectivity) is merged and ordered independently via
// its own StatusField generation (BeginFieldPoll/CompleteFieldPoll/
// ApplyFieldEvent in webtypes) -- a real-time push event, or a concurrent
// poll, for one field can supersede only that field. A slow-but-successful
// fetch for one field is never discarded merely because a DIFFERENT field's
// event or poll completion happened to land first. Independently, the same
// round also feeds BeginHTTPPoll/CompleteHTTPPoll, which derives HTTP
// reachability and the Online/Stale/Offline connectivity classification --
// CompleteHTTPPoll is called with a nil merge func here since field merging
// is already handled per-field above; it only records health/connectivity.
func (app *WebApp) UpdateDeviceStatus(deviceID string, conn *webtypes.DeviceConnection) {
	app.updateDeviceStatus(deviceID, conn, nil)
}

// refreshDeviceStatusAfterStereoPairMutation behaves like UpdateDeviceStatus,
// but only applies its /getGroup read if the device's applied group
// generation is still exactly groupBaseline -- i.e. nothing (no push event,
// no other poll) has changed group state since the caller captured that
// baseline immediately after applying its own lifecycle projection. This
// stops a slow, now-stale follow-up read from clobbering a fresher result
// that already landed while it was in flight.
func (app *WebApp) refreshDeviceStatusAfterStereoPairMutation(deviceID string, conn *webtypes.DeviceConnection, groupBaseline uint64) {
	app.updateDeviceStatus(deviceID, conn, &groupBaseline)
}

func (app *WebApp) updateDeviceStatus(deviceID string, conn *webtypes.DeviceConnection, groupBaseline *uint64) {
	// Skip status update if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

	defer func() { conn.MarkStatusRefreshed(time.Now()) }()

	nowPlayingGen := conn.BeginFieldPoll(webtypes.FieldNowPlaying)
	volumeGen := conn.BeginFieldPoll(webtypes.FieldVolume)
	presetsGen := conn.BeginFieldPoll(webtypes.FieldPresets)
	sourcesGen := conn.BeginFieldPoll(webtypes.FieldSources)
	bassGen := conn.BeginFieldPoll(webtypes.FieldBass)
	connectivityGen := conn.BeginFieldPoll(webtypes.FieldConnectivity)
	pollGeneration := conn.BeginHTTPPoll()

	// /getGroup must be gated to ST10 models -- see Client.GetGroup's doc
	// comment (verified against real hardware: a ST20 never replies at all,
	// hanging until the client's timeout instead of returning quickly).
	stereoCapable := stereoPairCapable(conn.DeviceInfo)

	groupGeneration, groupReserved := reserveAcousticGroupPoll(conn, stereoCapable, groupBaseline)

	nameGeneration := conn.BeginNameRefresh()

	// Phase 1: slow network fetches. Local vars only, no shared state
	// is touched yet. Errors are recorded so the merge below can tell
	// "field N stayed unchanged" apart from "field N got refreshed".
	nowPlaying, nowPlayingErr := conn.Client.GetNowPlaying()
	name, nameErr := conn.Client.GetName()
	volume, volumeErr := conn.Client.GetVolume()
	presets, presetsErr := conn.Client.GetPresets()
	sources, sourcesErr := conn.Client.GetSources()
	bassCapabilities, bassCapabilitiesErr := conn.Client.GetBassCapabilities()
	bass, bassErr := conn.Client.GetBass()
	zoneGeneration := conn.BeginZoneRefresh()
	zone, zoneErr := conn.Client.GetZone()

	var (
		group    *models.Group
		groupErr error
	)

	if stereoCapable && groupReserved {
		group, groupErr = conn.Client.GetGroup()
	}

	bassCapabilitiesErr, bassErr = validatePolledBass(conn, bassCapabilities, bass, bassCapabilitiesErr, bassErr)

	logStatusFailures(deviceID, conn, []statusRequestResult{
		{"now_playing", nowPlayingErr},
		{"name", nameErr},
		{"volume", volumeErr},
		{"presets", presetsErr},
		{"sources", sourcesErr},
		{"bassCapabilities", bassCapabilitiesErr},
		{"bass", bassErr},
		{"getZone", zoneErr},
		{"getGroup", groupErr},
	})

	// Phase 2: fast, independently-ordered merges. Each field applies only
	// if this round's fetch succeeded AND no newer poll or push event has
	// already applied for that specific field.
	anyFetchSucceeded := false

	if nowPlayingErr == nil {
		anyFetchSucceeded = true

		conn.CompleteFieldPoll(webtypes.FieldNowPlaying, nowPlayingGen, func(s *webtypes.DeviceStatus) {
			s.NowPlaying = nowPlaying
			s.LastActivity = time.Now()
		})
	}

	if volumeErr == nil {
		anyFetchSucceeded = true

		conn.CompleteFieldPoll(webtypes.FieldVolume, volumeGen, func(s *webtypes.DeviceStatus) {
			s.Volume = volume
			s.LastActivity = time.Now()
		})
	}

	if presetsErr == nil {
		anyFetchSucceeded = true

		conn.CompleteFieldPoll(webtypes.FieldPresets, presetsGen, func(s *webtypes.DeviceStatus) {
			s.Presets = presets
			s.LastActivity = time.Now()
		})
	}

	if sourcesErr == nil {
		anyFetchSucceeded = true
	}

	// Unlike the other fields, a FAILED /sources read is recorded too: the
	// inventory drives which source buttons the player offers, and acting on
	// a list the speaker no longer confirms is worse than offering none. See
	// ApplySourcesRead for why a failure is counted rather than fenced.
	conn.ApplySourcesRead(sourcesGen, sources, sourcesErr)

	if app.applyPolledBass(deviceID, conn, bassGen, bassCapabilities, bass, bassCapabilitiesErr, bassErr) {
		anyFetchSucceeded = true
	}

	if nameErr == nil {
		anyFetchSucceeded = true

		conn.ApplyPolledName(nameGeneration, name.Value)
	}

	// Mark as connected if we successfully got at least one status from
	// this round. Mirrors prior behaviour: deliberately does NOT fold
	// groupErr in here. GetGroup is gated to stereo-capable models and
	// trivially succeeds even when a device is otherwise struggling (an
	// empty <group/> is a near-guaranteed reply), so counting it would let
	// a device report connected while every substantive status fetch above
	// actually failed this round.
	conn.CompleteFieldPoll(webtypes.FieldConnectivity, connectivityGen, func(s *webtypes.DeviceStatus) {
		s.IsConnected = anyFetchSucceeded
		s.LastActivity = time.Now()
	})

	// Independently of the per-field merges above, also record this round
	// against the health/connectivity generation. merge is nil: field data
	// was already merged field-by-field above, so this call only derives
	// HTTPReachable/WebSocketConnected/Connectivity (Online/Stale/Offline)
	// from anyFetchSucceeded -- it must never re-apply payload fields, or a
	// concurrent speaker event landing between BeginHTTPPoll and here could
	// cause this call to silently discard part of the merge above.
	conn.CompleteHTTPPoll(pollGeneration, anyFetchSucceeded, time.Now(), nil)

	if stereoCapable && groupReserved && groupErr == nil {
		var accepted, changed bool
		if groupBaseline != nil {
			accepted, changed = conn.ApplyPolledGroupForBaseline(*groupBaseline, groupGeneration, group)
		} else {
			accepted, changed = conn.ApplyPolledGroupResult(groupGeneration, group)
		}

		if changed {
			app.QueueDeviceListBroadcast()
		}

		if accepted {
			app.refreshBalance(deviceID, conn)
		}
	}

	if zoneErr == nil && conn.DeviceInfo != nil &&
		conn.ApplyPolledZone(zoneGeneration, conn.DeviceInfo.DeviceID, zone) {
		app.BroadcastDeviceList()
	}
}

// statusRequestResult is the outcome of one request of a status round.
type statusRequestResult struct {
	request string
	err     error
}

// logStatusFailures logs the requests of a status round that failed, but
// only when the set of failing requests changes: once when a speaker stops
// answering (or one endpoint starts failing), once when it recovers. A
// speaker that stays offline would otherwise add a line to the log on every
// round.
func logStatusFailures(deviceID string, conn *webtypes.DeviceConnection, results []statusRequestResult) {
	var (
		failed   []string
		firstErr error
	)

	for _, result := range results {
		if result.err == nil {
			continue
		}

		failed = append(failed, result.request)
		if firstErr == nil {
			firstErr = result.err
		}
	}

	failures := strings.Join(failed, ", ")
	if !conn.ObserveStatusFailures(failures) {
		return
	}

	if failures == "" {
		log.Printf("Status requests to %s succeed again", sanitizeLog(deviceID))

		return
	}

	log.Printf("Status requests to %s failed: %s (first error: %s); not logged again until this changes",
		sanitizeLog(deviceID), failures, sanitizeLog(firstErr.Error()))
}

func (app *WebApp) applyGroupUpdatedEvent(
	conn *webtypes.DeviceConnection,
	event *models.GroupUpdatedEvent,
) bool {
	conn.MarkEventStreamActivity(time.Now())

	return app.queueBroadcastIfChanged(conn.ApplyGroupEvent(&event.Group, time.Now()))
}

type pendingZoneRefresh struct {
	masterDeviceID string
	connection     *webtypes.DeviceConnection
	generation     uint64
}

func zoneEventDeviceID(eventDeviceID string, conn *webtypes.DeviceConnection) string {
	if eventDeviceID = strings.TrimSpace(eventDeviceID); eventDeviceID != "" {
		return eventDeviceID
	}

	if conn == nil || conn.DeviceInfo == nil {
		return ""
	}

	return strings.TrimSpace(conn.DeviceInfo.DeviceID)
}

// reserveZoneRefreshesAfterEvent is the synchronous ordering barrier for a
// zoneUpdated event. Every affected connection receives a new generation
// before any /getZone request starts, so an older in-flight poll cannot restore
// stale topology while the event refresh is pending or fails.
func (app *WebApp) reserveZoneRefreshesAfterEvent(
	eventDeviceID string,
	eventMasterID string,
) []pendingZoneRefresh {
	candidates := map[string]struct{}{}
	if eventDeviceID = strings.TrimSpace(eventDeviceID); eventDeviceID != "" {
		candidates[eventDeviceID] = struct{}{}
	}

	if eventMasterID = strings.TrimSpace(eventMasterID); eventMasterID != "" {
		candidates[eventMasterID] = struct{}{}
	}

	snapshot := app.DeviceSnapshot()

	connectionsByDeviceID := make(map[string][]*webtypes.DeviceConnection, len(snapshot))
	for _, entry := range snapshot {
		if entry.Device == nil || entry.Device.DeviceInfo == nil {
			continue
		}

		deviceID := strings.TrimSpace(entry.Device.DeviceInfo.DeviceID)
		if deviceID != "" {
			connectionsByDeviceID[deviceID] = append(connectionsByDeviceID[deviceID], entry.Device)
		}

		status := entry.Device.Status()
		if eventDeviceID == "" || status == nil || status.Zone == nil ||
			!status.Zone.IsInZone(eventDeviceID) {
			continue
		}

		if masterID := strings.TrimSpace(status.Zone.Master); masterID != "" {
			candidates[masterID] = struct{}{}
		}
	}

	refreshes := make([]pendingZoneRefresh, 0, len(candidates))

	seenConnections := make(map[*webtypes.DeviceConnection]struct{}, len(candidates))
	for masterID := range candidates {
		connections := connectionsByDeviceID[masterID]
		if len(connections) != 1 || connections[0].Client == nil {
			continue
		}

		connection := connections[0]
		if _, duplicate := seenConnections[connection]; duplicate {
			continue
		}

		seenConnections[connection] = struct{}{}

		refreshes = append(refreshes, pendingZoneRefresh{
			masterDeviceID: masterID,
			connection:     connection,
			generation:     connection.BeginZoneEventRefresh(),
		})
	}

	return refreshes
}

// refreshZonesAfterEvent is the synchronous form used by focused tests and
// callers that already run outside an event callback. Production callbacks
// reserve synchronously, then complete each network request asynchronously.
func (app *WebApp) refreshZonesAfterEvent(eventDeviceID, eventMasterID string) {
	for _, refresh := range app.reserveZoneRefreshesAfterEvent(eventDeviceID, eventMasterID) {
		app.completeAuthoritativeZoneRefresh(refresh)
	}
}

func (app *WebApp) completeAuthoritativeZoneRefresh(refresh pendingZoneRefresh) {
	zone, err := refresh.connection.Client.GetZone()
	if err != nil {
		log.Printf("Failed to refresh zone master %s: %v", sanitizeLog(refresh.masterDeviceID), err)
		return
	}

	if refresh.connection.ApplyPolledZone(refresh.generation, refresh.masterDeviceID, zone) {
		app.BroadcastDeviceList()
	}
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

	app.registerDeviceWebSocketClient(conn)
	defer app.removeDeviceWebSocketClient(conn)

	log.Printf("Device WebSocket connected for %s", sanitizeLog(deviceID))

	// Capture and send under the same ordering seam used by lifecycle responses.
	if err := app.withDeviceConnWrite(conn, func(batch webSocketWriteBatch) error {
		return batch.writeJSON(conn, webtypes.WebSocketMessage{
			Type:     "device_status",
			DeviceID: deviceID,
			Data: map[string]interface{}{
				"info":   device.Info(),
				"status": device.Status(),
			},
		})
	}); err != nil {
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
		if err := app.writeDeviceWebSocketUpdate(conn, deviceID, device); err != nil {
			log.Printf("Failed to send device WebSocket update for %s: %v", sanitizeLog(deviceID), err)
			return
		}
	}
}

func (app *WebApp) writeDeviceWebSocketUpdate(
	conn webSocketWriter,
	deviceID string,
	device *webtypes.DeviceConnection,
) error {
	return app.withDeviceConnWrite(conn, func(batch webSocketWriteBatch) error {
		// Capture after taking the lifecycle ordering lock. A status frame
		// captured before a pair mutation therefore cannot follow its response.
		status := device.Status()

		if err := batch.writeMessage(conn, websocket.PingMessage, []byte{}); err != nil {
			return err
		}

		if err := batch.writeJSON(conn, webtypes.WebSocketMessage{
			Type:     "device_status",
			DeviceID: deviceID,
			Data: map[string]interface{}{
				"info":   device.Info(),
				"status": status,
			},
		}); err != nil {
			return err
		}

		if device.CurrentWebSocket() == nil || !status.IsConnected {
			return nil
		}

		return batch.writeJSON(conn, webtypes.WebSocketMessage{
			Type:     "device_realtime",
			DeviceID: deviceID,
			Data: map[string]interface{}{
				"nowPlaying": status.NowPlaying,
				"volume":     status.Volume,
				"timestamp":  time.Now(),
			},
		})
	})
}

func reserveAcousticGroupPoll(conn *webtypes.DeviceConnection, stereoCapable bool, baseline *uint64) (uint64, bool) {
	if !stereoCapable {
		return 0, false
	}

	if baseline != nil {
		return conn.BeginGroupRefreshIfBaseline(*baseline)
	}

	return conn.BeginGroupRefresh(), true
}

func validatePolledBass(conn *webtypes.DeviceConnection, bassCapabilities *models.BassCapabilities, bass *models.Bass, bassCapabilitiesErr, bassErr error) (error, error) {
	if bassCapabilitiesErr == nil {
		bassCapabilitiesErr = validBassCapabilityProjection(bassCapabilities, acousticDeviceID(conn))
	}

	if bassErr == nil {
		if bassCapabilitiesErr != nil {
			bassErr = bassCapabilitiesErr
		} else {
			bassErr = validBassReadback(bass, bassCapabilities, acousticDeviceID(conn))
		}
	}

	return bassCapabilitiesErr, bassErr
}

func (app *WebApp) applyPolledBass(deviceID string, conn *webtypes.DeviceConnection, bassGen uint64, bassCapabilities *models.BassCapabilities, bass *models.Bass, bassCapabilitiesErr, bassErr error) bool {
	if bassCapabilitiesErr == nil || bassErr == nil {
		app.applyAcousticRead(deviceID, conn, acousticDeviceID(conn), func() bool {
			return conn.CompleteFieldPoll(webtypes.FieldBass, bassGen, func(s *webtypes.DeviceStatus) {
				if bassCapabilitiesErr == nil {
					s.BassCapabilities = bassCapabilities
				}

				if bassErr == nil {
					s.Bass = bass
				}

				s.LastActivity = time.Now()
			})
		})

		return true
	}

	return false
}
