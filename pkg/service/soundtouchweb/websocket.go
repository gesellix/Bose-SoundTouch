// Package soundtouchweb contains WebSocket handlers for real-time communication.
package soundtouchweb

import (
	"encoding/json"
	"errors"
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

func (app *WebApp) applyBassEvent(
	conn *webtypes.DeviceConnection,
	bass *models.Bass,
) bool {
	return app.applySpeakerStatusEvent(conn, webtypes.FieldBass, func(status *webtypes.DeviceStatus) bool {
		changed := !reflect.DeepEqual(status.Bass, bass)
		status.Bass = bass
		status.LastActivity = time.Now()

		return changed
	})
}

// balanceWatchSchedule is the wait before each retry after a connection comes
// up. It starts short because the common case resolves almost immediately —
// the /getGroup poll only needs a moment — and a flat interval left a visible
// gap where the pair was on screen but the slider was not. It then stretches
// out, so a speaker that genuinely has no balance stops being asked.
var balanceWatchSchedule = []time.Duration{
	2 * time.Second,
	3 * time.Second,
	5 * time.Second,
	10 * time.Second,
	15 * time.Second,
	30 * time.Second,
}

// watchBalance establishes the balance reading for a device, retrying briefly
// until the speaker answers.
//
// It reads over HTTP, deliberately. Reads do not need the WebSocket — only
// writes do — and that socket is created lazily, on the first control request
// that needs one. Hanging the read off it meant a paired speaker showed no
// slider until something was pressed: the pair was on screen, the speaker
// answered balanceAvailable=true to anyone who asked, and nobody asked.
//
// Retrying rather than reading once covers the other case: a pair that already
// existed at startup is discovered by the /getGroup poll running concurrently,
// and groupUpdated only fires when the pairing CHANGES.
//
// The loop stops as soon as a balance is known or the device goes away, so a
// speaker that genuinely has none costs a handful of cheap requests and then
// nothing. GetBalance carries its own short timeout, so a sleeping speaker
// cannot stall this either.
func (app *WebApp) watchBalance(deviceID string, conn *webtypes.DeviceConnection) {
	for attempt, wait := range balanceWatchSchedule {
		app.refreshBalance(deviceID, conn)

		if conn.Status().Balance != nil {
			if attempt > 0 {
				log.Printf("Speaker %s: balance became readable on attempt %d",
					sanitizeLog(deviceID), attempt+1)
			}

			return
		}

		select {
		case <-time.After(wait):
		case <-conn.Done():
			return
		}
	}

	// One last attempt after the final wait, so the longest interval is not
	// spent only to give up without using it.
	app.refreshBalance(deviceID, conn)
}

// refreshBalance reads the balance and stores it on the device status.
//
// The only gate is the model: balance exists on SoundTouch 10s, and asking
// anything else is pointless. Whether it currently APPLIES is the speaker's
// call, reported as balanceAvailable, and that is the single authority here —
// an unavailable answer clears the reading so the UI drops the control.
//
// It deliberately does NOT pre-check status.Group. That looks like a free
// optimisation and is actually a race: on startup this runs alongside the
// first /getGroup poll, so the group is usually still nil, and gating on it
// meant the reading was skipped exactly when it was first needed.
func (app *WebApp) refreshBalance(deviceID string, conn *webtypes.DeviceConnection) {
	if conn.Client == nil || !stereoPairCapable(conn.DeviceInfo) {
		return
	}

	// Coalesce. Dragging the slider emits a burst of balanceUpdated frames —
	// four inside one second, measured — and reading once per frame would
	// pile concurrent requests onto an endpoint that blocks on a sleeping
	// speaker. Whoever is already reading will read once more on our behalf,
	// so the final value still lands.
	if !conn.BeginBalanceRefresh() {
		return
	}

	for {
		app.readBalanceOnce(deviceID, conn)

		if !conn.EndBalanceRefresh() {
			return
		}
	}
}

// readBalanceOnce performs a single balance read and stores the result.
func (app *WebApp) readBalanceOnce(deviceID string, conn *webtypes.DeviceConnection) {
	balance, err := conn.Client.GetBalance()
	if err != nil {
		log.Printf("Speaker %s: balance read failed: %v", sanitizeLog(deviceID), sanitizeLog(err.Error()))

		return
	}

	if !balance.Available {
		// Log the whole answer, not just the fact of it: the range and target
		// reported alongside are the evidence for why it is unavailable.
		//
		// Only when the state changes. This read runs on every balanceUpdated
		// frame and every status poll, so a speaker that reports no usable
		// balance — a standalone unit, most of them — emitted one identical
		// line per poll, per speaker, and buried everything else in the log.
		if conn.MarkBalanceUnavailable() {
			log.Printf("Speaker %s: balance reported unavailable (range %d..%d, default %d, target %d, actual %d)",
				sanitizeLog(deviceID), balance.Min, balance.Max, balance.Default, balance.Target, balance.Actual)
		}

		app.clearBalance(conn)

		return
	}

	conn.MarkBalanceAvailable()

	app.applyBalanceEvent(conn, balance)
}

// clearBalance drops any stored reading, which is how the UI learns the
// control no longer applies — the pair was torn down.
func (app *WebApp) clearBalance(conn *webtypes.DeviceConnection) {
	if conn.Status().Balance == nil {
		return
	}

	app.applyBalanceEvent(conn, nil)
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

// refreshBass re-reads /bass after a payload-free bassUpdated signal.
func (app *WebApp) refreshBass(deviceID string, conn *webtypes.DeviceConnection) {
	if conn.Client == nil {
		return
	}

	bass, err := conn.Client.GetBass()
	if err != nil {
		log.Printf("Speaker %s: bass re-read failed: %v", sanitizeLog(deviceID), sanitizeLog(err.Error()))

		return
	}

	app.applyBassEvent(conn, bass)
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

// applyBalanceEvent stores a fresh balance reading on the device status.
func (app *WebApp) applyBalanceEvent(
	conn *webtypes.DeviceConnection,
	balance *models.Balance,
) {
	app.applySpeakerStatusEvent(conn, webtypes.FieldBalance, func(status *webtypes.DeviceStatus) bool {
		changed := !reflect.DeepEqual(status.Balance, balance)
		status.Balance = balance
		status.LastActivity = time.Now()

		return changed
	})
}

// registerDeviceWebSocketClient gives conn its own write-serialization lock,
// mirroring registerGlobalWebSocket's role for the browser-wide pool. Unlike
// registerGlobalWebSocket, there are no initial frames to send under it --
// callers install the lock before their first write.
func (app *WebApp) registerDeviceWebSocketClient(conn webSocketWriter) {
	app.DeviceWSMutex.Lock()
	app.DeviceWSClients[conn] = &sync.Mutex{}
	app.DeviceWSMutex.Unlock()
}

// removeDeviceWebSocketClient unregisters conn. Unlike
// removeGlobalWebSocketClient, callers close the underlying connection
// themselves (HandleDeviceWebSocket already does via its own defer), so this
// only needs to drop the registry entry.
func (app *WebApp) removeDeviceWebSocketClient(conn webSocketWriter) {
	app.DeviceWSMutex.Lock()
	delete(app.DeviceWSClients, conn)
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
			activity := time.Now()
			np := &event.NowPlaying

			// A /select returns 200 even when the source is rejected; the
			// failure shows up here as a transition to an error source. Log it
			// so it lands in a diagnostic export without needing a live trace.
			if np.Source != prevSource && isErrorSource(np.Source) {
				logNowPlayingError(deviceID, np.Source, np.SourceAccount)
			}

			// Waking up can flip whether the speaker answers for balance at
			// all, and a reading taken while it was asleep would have been
			// stored as "no balance here". Re-read on the transition out of
			// standby, once, rather than on every event.
			if np.Source != prevSource && isStandbySource(prevSource) && !isStandbySource(np.Source) {
				go app.refreshBalance(deviceID, conn)
			}

			prevSource = np.Source

			app.applyNowPlayingEvent(conn, np)
			conn.MarkEventStreamActivity(activity)
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
			go app.refreshBalance(deviceID, conn)
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

		wsClient.OnBassUpdated(func(event *models.BassUpdatedEvent) {
			activity := time.Now()

			conn.MarkEventStreamActivity(activity)

			// Every captured bassUpdated frame is empty; it is a re-read
			// signal, not a value. Taking the zero value out of one and
			// storing it is what made the Player's bass slider snap to 0 on
			// every change.
			if !event.HasPayload() {
				go app.refreshBass(deviceID, conn)

				return
			}

			app.applyBassEvent(conn, event.Bass)
		})

		wsClient.OnGroupUpdated(func(event *models.GroupUpdatedEvent) {
			app.applyGroupUpdatedEvent(conn, event)

			// Creating or tearing down a pair flips whether balance exists
			// here at all, so the reading has to follow the group.
			go app.refreshBalance(deviceID, conn)
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

func (app *WebApp) updateDeviceStatus(_ string, conn *webtypes.DeviceConnection, groupBaseline *uint64) {
	// Skip status update if client is not available (e.g., in tests)
	if conn.Client == nil {
		return
	}

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

	var groupGeneration uint64
	if stereoCapable && groupBaseline == nil {
		groupGeneration = conn.BeginGroupRefresh()
	}

	nameGeneration := conn.BeginNameRefresh()

	// Phase 1: slow network fetches. Local vars only, no shared state
	// is touched yet. Errors are recorded so the merge below can tell
	// "field N stayed unchanged" apart from "field N got refreshed".
	nowPlaying, nowPlayingErr := conn.Client.GetNowPlaying()
	name, nameErr := conn.Client.GetName()
	volume, volumeErr := conn.Client.GetVolume()
	presets, presetsErr := conn.Client.GetPresets()
	sources, sourcesErr := conn.Client.GetSources()
	bass, bassErr := conn.Client.GetBass()

	var (
		group    *models.Group
		groupErr error
	)

	if stereoCapable {
		group, groupErr = conn.Client.GetGroup()
	}

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

	if bassErr == nil {
		anyFetchSucceeded = true

		conn.CompleteFieldPoll(webtypes.FieldBass, bassGen, func(s *webtypes.DeviceStatus) {
			s.Bass = bass
			s.LastActivity = time.Now()
		})
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

	if stereoCapable && groupErr == nil {
		if groupBaseline != nil {
			conn.ApplyPolledGroupIfBaseline(*groupBaseline, group)
		} else {
			conn.ApplyPolledGroup(groupGeneration, group)
		}
	}
}

func (app *WebApp) applyGroupUpdatedEvent(
	conn *webtypes.DeviceConnection,
	event *models.GroupUpdatedEvent,
) bool {
	conn.MarkEventStreamActivity(time.Now())

	return app.queueBroadcastIfChanged(conn.ApplyGroupEvent(&event.Group, time.Now()))
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
