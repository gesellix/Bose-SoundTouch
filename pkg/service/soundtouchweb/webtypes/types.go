// Package webtypes contains type definitions for the SoundTouch web UI.
package webtypes

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// SoundTouchClient defines the interface for SoundTouch client operations
type SoundTouchClient interface {
	Play() error
	Pause() error
	Stop() error
	NextTrack() error
	PrevTrack() error
	SetVolume(level int) error
	SetBass(level int) error
	SelectPreset(id int) error
	SelectSource(source, account string) error
	SendKey(key string) error
	GetDeviceInfo() (*models.DeviceInfo, error)
	GetNowPlaying() (*models.NowPlaying, error)
	GetVolume() (*models.Volume, error)
	GetPresets() (*models.Presets, error)
	GetSources() (*models.Sources, error)
	GetBass() (*models.Bass, error)
	NewWebSocketClient(config interface{}) *client.WebSocketClient
}

// DeviceConnection wraps a SoundTouch client with WebSocket connection.
//
// The Status field is stored behind atomic.Pointer so concurrent
// readers (HTTP handlers, WebSocket broadcasters) never observe a
// torn struct while a writer (UpdateDeviceStatus, WebSocket event
// handlers) is mid-update. Access status through Status / SetStatus
// / UpdateStatus rather than the private field; construct connections
// via NewDeviceConnection to guarantee the status is initialised.
type DeviceConnection struct {
	Client *client.Client
	// WebSocket is retained for source compatibility with callers that build
	// DeviceConnection values directly. Concurrent code must use
	// CurrentWebSocket and SetWebSocket.
	WebSocket *client.WebSocketClient
	// DeviceInfo is the immutable discovery snapshot. Use Info for player-facing
	// output so later nameUpdated events are reflected without racing readers.
	DeviceInfo *models.DeviceInfo
	LastSeen   time.Time

	deviceName atomic.Pointer[string]
	status     atomic.Pointer[DeviceStatus]

	webSocketMu          sync.RWMutex
	webSocketLoopRunning atomic.Bool

	nameMu  sync.Mutex
	nameGen uint64

	healthMu sync.Mutex

	nextPollGeneration  uint64
	lastPollGeneration  uint64
	httpReachable       bool
	consecutiveFailures int
	speakerEventGen     uint64
	pollEventGen        map[uint64]uint64

	lastTransportGeneration    uint64
	eventStreamConnected       bool
	lastDirectSuccess          time.Time
	speakerConnectionKnown     bool
	speakerConnectionConnected bool
	speakerConnectionObserved  time.Time

	// fieldGenMu guards fieldGen, the per-field generation ordering used by
	// BeginFieldPoll/CompleteFieldPoll/ApplyFieldEvent. Each StatusField gets
	// its own (issued, applied) pair so an event or a poll completion for
	// one field can never invalidate a different field's in-flight result --
	// see StatusField's doc comment.
	fieldGenMu sync.Mutex
	fieldGen   [numStatusFields]struct{ issued, applied uint64 }

	// groupMu orders polled /getGroup responses against real-time
	// groupUpdated events. groupGeneration is the highest generation
	// issued (by BeginGroupRefresh or ApplyGroupEvent);
	// groupAppliedGeneration is the highest generation whose result was
	// actually applied. Invalidation keys off completion order via
	// groupAppliedGeneration, not merely "a newer refresh has started" —
	// otherwise a later poll that starts but never applies (e.g. its own
	// GetGroup fails) would still discard an earlier poll's still-arriving
	// successful result.
	groupMu                sync.Mutex
	groupGeneration        uint64
	groupAppliedGeneration uint64

	// done is closed by Close when the device is removed from the
	// registry, signalling its background goroutines (the status poller
	// and the WebSocket reconnect loop) to exit. closeOnce keeps Close
	// idempotent.
	done      chan struct{}
	closeOnce sync.Once
}

// DeviceStatus represents the current device state
type DeviceStatus struct {
	NowPlaying             *models.NowPlaying      `json:"nowPlaying,omitempty"`
	Volume                 *models.Volume          `json:"volume,omitempty"`
	Presets                *models.Presets         `json:"presets,omitempty"`
	Sources                *models.Sources         `json:"sources,omitempty"`
	SourcesStale           bool                    `json:"sourcesStale,omitempty"`
	Bass                   *models.Bass            `json:"bass,omitempty"`
	Group                  *models.Group           `json:"group,omitempty"`
	Connectivity           Connectivity            `json:"connectivity"`
	HTTPReachable          bool                    `json:"httpReachable"`
	WebSocketConnected     bool                    `json:"webSocketConnected"`
	SpeakerConnectionState *SpeakerConnectionState `json:"speakerConnectionState,omitempty"`
	IsConnected            bool                    `json:"isConnected"`
	LastActivity           time.Time               `json:"lastActivity"`

	// Revision is a per-connection monotonic counter advanced by every
	// successful UpdateStatus. It lets the browser order a full `devices`
	// snapshot against a `status_update` delta -- without it the two frames
	// carry no sequence at all and a slow snapshot can clobber a newer delta.
	Revision uint64 `json:"revision"`
	// NowPlayingRevision is the FieldNowPlaying generation that last wrote
	// NowPlaying. Revision alone cannot answer "did now-playing actually
	// change?", because an unrelated field's merge advances it too; a source
	// selection waiting for authoritative confirmation needs exactly that
	// distinction.
	NowPlayingRevision uint64 `json:"nowPlayingRevision"`
}

// Connectivity is the player's aggregate view of HTTP and event-stream
// reachability. Speaker-reported connection state is retained as supporting
// evidence but cannot override a current direct-path success.
type Connectivity string

// Player connectivity states derived from direct and speaker-reported evidence.
const (
	ConnectivityOnline  Connectivity = "online"
	ConnectivityStale   Connectivity = "stale"
	ConnectivityOffline Connectivity = "offline"
)

const (
	offlineFailureThreshold = 2
	offlineGracePeriod      = 60 * time.Second
)

// SpeakerConnectionState is the network state reported by the speaker.
type SpeakerConnectionState struct {
	State  string `json:"state"`
	Signal string `json:"signal,omitempty"`
}

// StatusField identifies one independently-racing field of DeviceStatus for
// BeginFieldPoll/CompleteFieldPoll/ApplyFieldEvent's generation ordering.
// FieldConnectivity covers IsConnected, which is derived by a poll (from
// whether any of the other fields' fetches succeeded) but set directly by a
// real-time connectionStateUpdated event, so it races the same way the other
// fields do. Group has its own, pre-existing pair-aware ordering
// (groupMu/groupGeneration/groupAppliedGeneration below) and is not part of
// this set.
type StatusField int

// The StatusField values.
const (
	FieldNowPlaying StatusField = iota
	FieldVolume
	FieldPresets
	FieldSources
	FieldBass
	FieldConnectivity
	numStatusFields
)

// NewDeviceConnection creates a fully-initialised connection. The
// status starts with IsConnected=false and LastActivity set to now;
// real values arrive via UpdateStatus once the device responds.
func NewDeviceConnection(c *client.Client, info *models.DeviceInfo) *DeviceConnection {
	conn := &DeviceConnection{
		Client:     c,
		DeviceInfo: info,
		LastSeen:   time.Now(),
		done:       make(chan struct{}),
	}
	conn.status.Store(&DeviceStatus{
		Connectivity: ConnectivityOffline,
		IsConnected:  false,
		LastActivity: time.Now(),
	})

	if info != nil {
		conn.storeDeviceName(info.Name)
	}

	return conn
}

func (c *DeviceConnection) storeDeviceName(name string) {
	c.deviceName.Store(&name)
}

// BeginNameRefresh starts a generation for an asynchronous /name request.
func (c *DeviceConnection) BeginNameRefresh() uint64 {
	c.nameMu.Lock()
	defer c.nameMu.Unlock()

	c.nameGen++

	return c.nameGen
}

// ApplyPolledName stores a /name result unless a newer poll or event won.
func (c *DeviceConnection) ApplyPolledName(generation uint64, name string) bool {
	c.nameMu.Lock()
	defer c.nameMu.Unlock()

	if generation != c.nameGen {
		return false
	}

	c.storeDeviceName(name)

	return true
}

// ApplyNameEvent stores a nameUpdated event and invalidates in-flight polls.
func (c *DeviceConnection) ApplyNameEvent(name string) {
	c.nameMu.Lock()
	defer c.nameMu.Unlock()

	c.nameGen++
	c.storeDeviceName(name)
}

// Info returns a read-only metadata snapshot with the latest device name.
func (c *DeviceConnection) Info() *models.DeviceInfo {
	if c.DeviceInfo == nil {
		return nil
	}

	name := c.deviceName.Load()
	if name == nil || *name == c.DeviceInfo.Name {
		return c.DeviceInfo
	}

	info := *c.DeviceInfo
	info.Name = *name

	return &info
}

// Status returns a snapshot of the current device status. The returned
// pointer is read-only from the caller's perspective and must not be
// mutated. Use UpdateStatus or SetStatus to apply changes. Never returns
// nil for connections built via NewDeviceConnection.
func (c *DeviceConnection) Status() *DeviceStatus {
	return c.status.Load()
}

// Done returns a channel that is closed when the connection is removed
// from the registry. The per-device status poller and WebSocket
// reconnect loop select on it to stop instead of running for the life
// of the process.
func (c *DeviceConnection) Done() <-chan struct{} {
	return c.done
}

// Close signals the connection's background goroutines to stop and best-
// effort disconnects the WebSocket so a blocked reconnect loop wakes
// promptly. Idempotent; safe to call on a connection that never started
// any goroutine (e.g. a test connection with a nil Client).
func (c *DeviceConnection) Close() {
	c.closeOnce.Do(func() {
		close(c.done)

		c.webSocketMu.Lock()
		ws := c.WebSocket
		c.WebSocket = nil
		c.webSocketMu.Unlock()

		if ws != nil {
			_ = ws.Close()
		}
	})
}

// CurrentWebSocket returns the current device event transport, if any.
func (c *DeviceConnection) CurrentWebSocket() *client.WebSocketClient {
	c.webSocketMu.RLock()
	defer c.webSocketMu.RUnlock()

	return c.WebSocket
}

// SetWebSocket publishes a device event transport unless the connection was
// already closed. A transport that loses that race is stopped immediately.
func (c *DeviceConnection) SetWebSocket(ws *client.WebSocketClient) bool {
	c.webSocketMu.Lock()
	select {
	case <-c.done:
		c.webSocketMu.Unlock()

		if ws != nil {
			_ = ws.Close()
		}

		return false
	default:
		c.WebSocket = ws
		c.webSocketMu.Unlock()

		return true
	}
}

// ClearWebSocket clears only the expected transport, preserving a replacement
// that may already have been published under the same device entry.
func (c *DeviceConnection) ClearWebSocket(expected *client.WebSocketClient) bool {
	c.webSocketMu.Lock()
	defer c.webSocketMu.Unlock()

	if c.WebSocket != expected {
		return false
	}

	c.WebSocket = nil

	return true
}

// TryStartWebSocketLoop claims the single event supervisor for this device.
func (c *DeviceConnection) TryStartWebSocketLoop() bool {
	return c.webSocketLoopRunning.CompareAndSwap(false, true)
}

// FinishWebSocketLoop releases event-supervisor ownership.
func (c *DeviceConnection) FinishWebSocketLoop() {
	c.webSocketLoopRunning.Store(false)
}

// SetStatus atomically replaces the entire status. Use sparingly —
// UpdateStatus is the preferred entry point because it preserves
// concurrent changes from other goroutines.
//
// Revision is derived from the currently stored status rather than trusted
// from the caller: a replacement that reset it to the caller's zero value
// would make every browser holding a higher revision reject this device's
// subsequent updates outright. Replacing the whole status also supersedes
// every field, so each StatusField generation is advanced past any poll
// still in flight.
func (c *DeviceConnection) SetStatus(s *DeviceStatus) {
	nowPlayingGeneration := c.supersedeAllFields()

	for {
		old := c.status.Load()
		next := *s
		next.Revision = old.Revision + 1
		next.NowPlayingRevision = nowPlayingGeneration

		if c.status.CompareAndSwap(old, &next) {
			return
		}
	}
}

// supersedeAllFields advances every StatusField generation past whatever is
// currently in flight and returns FieldNowPlaying's new generation.
func (c *DeviceConnection) supersedeAllFields() uint64 {
	c.fieldGenMu.Lock()
	defer c.fieldGenMu.Unlock()

	for field := range c.fieldGen {
		c.fieldGen[field].issued++
		c.fieldGen[field].applied = c.fieldGen[field].issued
	}

	return c.fieldGen[FieldNowPlaying].applied
}

// BeginFieldPoll reserves a generation for an asynchronous fetch of field,
// before any network I/O starts. Pass the returned value to CompleteFieldPoll
// once the fetch completes.
func (c *DeviceConnection) BeginFieldPoll(field StatusField) uint64 {
	c.fieldGenMu.Lock()
	defer c.fieldGenMu.Unlock()

	c.fieldGen[field].issued++

	return c.fieldGen[field].issued
}

// CompleteFieldPoll applies mut only if generation is strictly newer than
// whatever last actually applied -- poll or event -- for field. A poll for
// one field losing this race never affects any other field: an unrelated
// field's event, or an unrelated field's poll completing first, cannot
// discard this field's fresh, successful data.
func (c *DeviceConnection) CompleteFieldPoll(field StatusField, generation uint64, mut func(*DeviceStatus)) bool {
	c.fieldGenMu.Lock()

	if generation <= c.fieldGen[field].applied {
		c.fieldGenMu.Unlock()

		return false
	}

	c.fieldGen[field].applied = generation
	c.fieldGenMu.Unlock()

	c.UpdateStatus(func(status *DeviceStatus) {
		mut(status)
		recordFieldRevision(status, field, generation)
	})

	return true
}

// ApplyFieldEvent always applies mut -- a real-time push event is
// authoritative evidence for field -- and invalidates any poll for field
// that began before it, without touching any other field's ordering.
func (c *DeviceConnection) ApplyFieldEvent(field StatusField, mut func(*DeviceStatus)) {
	c.fieldGenMu.Lock()
	c.fieldGen[field].issued++
	generation := c.fieldGen[field].issued
	c.fieldGen[field].applied = generation
	c.fieldGenMu.Unlock()

	c.UpdateStatus(func(status *DeviceStatus) {
		mut(status)
		recordFieldRevision(status, field, generation)
	})
}

// recordFieldRevision publishes the generation that just wrote field, for the
// fields whose ordering a client needs to observe. Only FieldNowPlaying is
// published today: a source selection confirms itself by waiting for a
// now-playing write strictly newer than the one it started from, and the
// aggregate Revision cannot express that, because any other field's merge
// advances it too.
func recordFieldRevision(status *DeviceStatus, field StatusField, generation uint64) {
	if field == FieldNowPlaying {
		status.NowPlayingRevision = generation
	}
}

// UpdateStatus atomically applies mut to a copy of the current status
// and stores the result. If another goroutine updates the status while
// mut runs, UpdateStatus retries with the newer status — so concurrent
// writers cannot silently lose each other's changes.
//
// The copy mut receives is a shallow value copy of the previous status.
// Nested pointer fields (NowPlaying, Volume, Presets, Sources, Bass, Group)
// share their backing struct with the previous version: callers MUST
// REPLACE these pointers (s.Volume = &models.Volume{...}) rather than
// mutate through them (s.Volume.ActualVolume++ would race with any
// reader still holding the previous snapshot). Production callers
// receive these values fresh from the device API, so this is the
// natural shape.
//
// Every successful store advances Revision exactly once.
func (c *DeviceConnection) UpdateStatus(mut func(*DeviceStatus)) {
	for {
		old := c.status.Load()
		next := *old
		mut(&next)
		next.Revision = old.Revision + 1

		if c.status.CompareAndSwap(old, &next) {
			return
		}
	}
}

// BeginHTTPPoll reserves an ordering generation for a status poll.
func (c *DeviceConnection) BeginHTTPPoll() uint64 {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	c.nextPollGeneration++
	if c.pollEventGen == nil {
		c.pollEventGen = make(map[uint64]uint64)
	}

	c.pollEventGen[c.nextPollGeneration] = c.speakerEventGen

	return c.nextPollGeneration
}

// ApplySpeakerEventAt applies event payload and records a live event stream.
func (c *DeviceConnection) ApplySpeakerEventAt(at time.Time, mut func(*DeviceStatus)) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	c.speakerEventGen++
	c.markEventStreamActivityLocked(at)
	c.UpdateStatus(func(status *DeviceStatus) {
		if mut != nil {
			mut(status)
		}

		c.applyConnectivityLocked(status, at)
	})
}

// ApplySpeakerConnectionEvent stores speaker-reported diagnostic state. The
// event itself proves the event stream is live.
func (c *DeviceConnection) ApplySpeakerConnectionEvent(state SpeakerConnectionState, at time.Time) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	c.speakerEventGen++
	c.markEventStreamActivityLocked(at)
	c.speakerConnectionObserved = at

	switch strings.ToUpper(strings.TrimSpace(state.State)) {
	case string(models.ConnectionStateConnected):
		c.speakerConnectionKnown = true
		c.speakerConnectionConnected = true
	case string(models.ConnectionStateDisconnected):
		c.speakerConnectionKnown = true
		c.speakerConnectionConnected = false
	default:
		c.speakerConnectionKnown = false
		c.speakerConnectionConnected = false
	}

	c.UpdateStatus(func(status *DeviceStatus) {
		reported := state
		status.SpeakerConnectionState = &reported
		status.LastActivity = at
		c.applyConnectivityLocked(status, at)
	})
}

// ObserveEventStreamTransport applies an authoritative client transport
// transition. Its generation originates in WebSocketClient, so reordered
// callbacks cannot overwrite a newer transport state.
func (c *DeviceConnection) ObserveEventStreamTransport(
	generation uint64,
	connected bool,
	at time.Time,
) bool {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	if generation <= c.lastTransportGeneration {
		return false
	}

	c.lastTransportGeneration = generation

	c.eventStreamConnected = connected
	if connected {
		c.recordDirectSuccessLocked(at)
	}

	c.UpdateStatus(func(status *DeviceStatus) {
		c.applyConnectivityLocked(status, at)
	})

	return true
}

// ObserveEventStream records a transition when no transport generation is
// available, primarily for non-client callers and deterministic tests.
func (c *DeviceConnection) ObserveEventStream(connected bool, at time.Time) {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	c.eventStreamConnected = connected
	if connected {
		c.recordDirectSuccessLocked(at)
	}

	c.UpdateStatus(func(status *DeviceStatus) {
		c.applyConnectivityLocked(status, at)
	})
}

// MarkEventStreamActivity records an event handled by a field-specific path.
func (c *DeviceConnection) MarkEventStreamActivity(at time.Time) {
	c.ObserveEventStream(true, at)
}

// CompleteHTTPPoll records health and merges payload only if no newer poll or
// speaker event superseded it.
func (c *DeviceConnection) CompleteHTTPPoll(
	generation uint64,
	success bool,
	at time.Time,
	merge func(*DeviceStatus),
) bool {
	c.healthMu.Lock()
	defer c.healthMu.Unlock()

	pollEventGeneration, knownGeneration := c.pollEventGen[generation]
	delete(c.pollEventGen, generation)

	if generation <= c.lastPollGeneration {
		return false
	}

	c.lastPollGeneration = generation
	for olderGeneration := range c.pollEventGen {
		if olderGeneration < generation {
			delete(c.pollEventGen, olderGeneration)
		}
	}

	if success {
		c.httpReachable = true
		c.consecutiveFailures = 0
		c.recordDirectSuccessLocked(at)
	} else {
		c.httpReachable = false
		c.consecutiveFailures++
	}

	c.UpdateStatus(func(status *DeviceStatus) {
		if merge != nil && knownGeneration && pollEventGeneration == c.speakerEventGen {
			merge(status)
		}

		c.applyConnectivityLocked(status, at)

		if success {
			status.LastActivity = at
		}
	})

	return true
}

func (c *DeviceConnection) markEventStreamActivityLocked(at time.Time) {
	c.eventStreamConnected = true
	c.recordDirectSuccessLocked(at)
}

func (c *DeviceConnection) recordDirectSuccessLocked(at time.Time) {
	if c.lastDirectSuccess.IsZero() || at.After(c.lastDirectSuccess) {
		c.lastDirectSuccess = at
	}
}

func (c *DeviceConnection) applyConnectivityLocked(status *DeviceStatus, at time.Time) {
	connectivity := c.connectivityLocked(at)
	status.Connectivity = connectivity
	status.HTTPReachable = c.httpReachable
	status.WebSocketConnected = c.eventStreamConnected
	status.IsConnected = connectivity != ConnectivityOffline
}

func (c *DeviceConnection) connectivityLocked(at time.Time) Connectivity {
	if c.httpReachable || c.eventStreamConnected {
		return ConnectivityOnline
	}

	if c.speakerConnectionKnown && c.speakerConnectionConnected &&
		withinConnectivityGrace(at, c.speakerConnectionObserved) {
		return ConnectivityStale
	}

	if !c.lastDirectSuccess.IsZero() &&
		(c.consecutiveFailures < offlineFailureThreshold ||
			withinConnectivityGrace(at, c.lastDirectSuccess)) {
		return ConnectivityStale
	}

	return ConnectivityOffline
}

func withinConnectivityGrace(at, success time.Time) bool {
	if success.IsZero() {
		return false
	}

	if at.Before(success) {
		return true
	}

	return at.Sub(success) < offlineGracePeriod
}

// MarkHTTPSuccess records a successful out-of-band request such as /info.
func (c *DeviceConnection) MarkHTTPSuccess(at time.Time) {
	generation := c.BeginHTTPPoll()
	c.CompleteHTTPPoll(generation, true, at, nil)
}

// BeginGroupRefresh starts a new generation for an asynchronous /getGroup
// request. Only the latest started request may later update Group.
func (c *DeviceConnection) BeginGroupRefresh() uint64 {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	c.groupGeneration++

	return c.groupGeneration
}

// ApplyPolledGroup stores a /getGroup result only if no strictly newer
// result (poll or event) has already applied. Empty groups clear the
// current claim.
func (c *DeviceConnection) ApplyPolledGroup(generation uint64, group *models.Group) bool {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	if generation <= c.groupAppliedGeneration {
		return false
	}

	c.groupAppliedGeneration = generation

	return c.replaceGroup(normalizeGroup(group), time.Time{})
}

// GroupGeneration reports the generation of the most recently applied group
// result (poll or event). Callers use this to snapshot a baseline before
// starting an async /getGroup read they only want to apply if nothing else
// has changed group state in the meantime.
func (c *DeviceConnection) GroupGeneration() uint64 {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	return c.groupAppliedGeneration
}

// ApplyPolledGroupIfBaseline stores a /getGroup result only if the applied
// group generation is still exactly baseline, i.e. no poll or event has
// applied since the caller captured that baseline via GroupGeneration. Unlike
// ApplyPolledGroup, a caller here never minted its own generation up front,
// so it cannot rely on "strictly newer" to detect a stale read -- an
// unconditionally-incrementing generation would always look newer than a
// baseline captured earlier, even when the read itself raced a fresher
// event or poll to completion first.
func (c *DeviceConnection) ApplyPolledGroupIfBaseline(baseline uint64, group *models.Group) bool {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	if c.groupAppliedGeneration != baseline {
		return false
	}

	c.groupGeneration++
	c.groupAppliedGeneration = c.groupGeneration

	return c.replaceGroup(normalizeGroup(group), time.Time{})
}

// ApplyGroupEvent stores the newest groupUpdated event and invalidates all
// in-flight /getGroup requests, including ones that have not started yet.
// Empty teardown events clear the current claim.
func (c *DeviceConnection) ApplyGroupEvent(group *models.Group, activity time.Time) bool {
	c.groupMu.Lock()
	defer c.groupMu.Unlock()

	c.groupGeneration++
	c.groupAppliedGeneration = c.groupGeneration

	return c.replaceGroup(normalizeGroup(group), activity)
}

func (c *DeviceConnection) replaceGroup(group *models.Group, activity time.Time) bool {
	changed := !models.SameGroup(c.Status().Group, group)
	c.UpdateStatus(func(s *DeviceStatus) {
		s.Group = group
		if !activity.IsZero() {
			s.LastActivity = activity
		}
	})

	return changed
}

func normalizeGroup(group *models.Group) *models.Group {
	if group == nil || group.IsEmpty() {
		return nil
	}

	return group
}

// APIResponse is a standard JSON response wrapper
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// VolumeRequest represents a volume control request
type VolumeRequest struct {
	Level int `json:"level"`
}

// BassRequest represents a bass control request
type BassRequest struct {
	Level int `json:"level"`
}

// SourceRequest represents an exact source selection request.
type SourceRequest struct {
	Source  string `json:"source"`
	Account string `json:"account"`
}

// WebSocketMessage represents messages sent over WebSocket
type WebSocketMessage struct {
	Type     string      `json:"type"`
	DeviceID string      `json:"deviceId,omitempty"`
	Data     interface{} `json:"data,omitempty"`
}

// DiscoveryStatus represents the status of device discovery
type DiscoveryStatus struct {
	IsDiscovering bool   `json:"isDiscovering"`
	Status        string `json:"status,omitempty"`
	DeviceCount   int    `json:"deviceCount,omitempty"`
}
