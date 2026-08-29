package soundtouchweb

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gorilla/websocket"
)

func TestPublishAndConnectDeviceWebSocketLetsRemovalCancelInitialDial(t *testing.T) {
	conn := webtypes.NewDeviceConnection(nil, nil)
	wsClient := client.NewClientFromHost("192.0.2.10").NewWebSocketClient(nil)
	connectStarted := make(chan struct{})
	connectDone := make(chan struct {
		published bool
		err       error
	}, 1)

	go func() {
		published, err := publishAndConnectDeviceWebSocket(conn, wsClient, func() error {
			close(connectStarted)
			wsClient.Wait()

			return context.Canceled
		})
		connectDone <- struct {
			published bool
			err       error
		}{published: published, err: err}
	}()

	select {
	case <-connectStarted:
	case <-time.After(time.Second):
		t.Fatal("initial dial did not start")
	}

	if got := conn.CurrentWebSocket(); got != wsClient {
		t.Fatalf("CurrentWebSocket() = %p during initial dial, want %p", got, wsClient)
	}

	conn.Close()

	select {
	case result := <-connectDone:
		if !result.published || result.err == nil {
			t.Fatalf("publish/connect result = (%v, %v), want (true, error)", result.published, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("device removal did not cancel the initial dial")
	}

	if got := conn.CurrentWebSocket(); got != nil {
		t.Fatalf("CurrentWebSocket() = %p after removal, want nil", got)
	}
}

func TestUpdateDeviceStatusRefreshesGroup(t *testing.T) {
	server := newStatusTestServer(t, http.StatusOK, `<group id="pair-1">
		<name>Living Room</name>
		<masterDeviceId>master-1</masterDeviceId>
		<roles>
			<groupRole><deviceId>master-1</deviceId><role>LEFT</role></groupRole>
			<groupRole><deviceId>member-1</deviceId><role>RIGHT</role></groupRole>
		</roles>
		<status>GROUP_OK</status>
	</group>`)
	defer server.Close()

	conn := webtypes.NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{
		DeviceID: "master-1",
		Type:     "SoundTouch 10",
	})
	NewWebApp().UpdateDeviceStatus("device-1", conn)

	status := conn.Status()
	if status.Group == nil {
		t.Fatal("Group was not populated by UpdateDeviceStatus")
	}

	if status.Group.ID != "pair-1" || status.Group.MasterDeviceID != "master-1" {
		t.Errorf("Group = %+v, want refreshed stereo pair", status.Group)
	}

	if len(status.Group.Roles.Roles) != 2 {
		t.Errorf("group roles = %d, want 2", len(status.Group.Roles.Roles))
	}

	if !status.IsConnected {
		t.Error("successful status refresh should mark the device connected")
	}
	if status.Zone == nil || status.Zone.Master != "master-1" || len(status.Zone.Members) != 1 {
		t.Fatalf("Zone = %+v, want refreshed multiroom topology", status.Zone)
	}
}

func TestUpdateDeviceStatusPreservesGroupOnError(t *testing.T) {
	server := newStatusTestServer(t, http.StatusInternalServerError, "group unavailable")
	defer server.Close()

	conn := webtypes.NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{Type: "SoundTouch 10"})
	existing := &models.Group{ID: "pair-old", Name: "Existing Pair"}
	conn.SetStatus(&webtypes.DeviceStatus{Group: existing})

	NewWebApp().UpdateDeviceStatus("device-1", conn)

	status := conn.Status()
	if status.Group != existing {
		t.Errorf("Group = %+v, want previous group preserved on refresh error", status.Group)
	}

	if !status.IsConnected {
		t.Error("other successful status fetches should keep the device connected")
	}
}

func TestSpeakerConnectionEventMatchesRegisteredHardwareID(t *testing.T) {
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "AA11BB22CC33"})

	for _, eventDeviceID := range []string{"", "AA11BB22CC33", "aa11bb22cc33"} {
		if !speakerConnectionEventMatches(conn, eventDeviceID) {
			t.Fatalf("event device ID %q should match", eventDeviceID)
		}
	}
	if speakerConnectionEventMatches(conn, "DEADBEEF0000") {
		t.Fatal("mismatched speaker connection event was accepted")
	}
	if speakerConnectionEventMatches(webtypes.NewDeviceConnection(nil, nil), "AA11BB22CC33") {
		t.Fatal("identified event matched a connection without device identity")
	}
}

func TestUpdateDeviceStatusRefreshesDeviceName(t *testing.T) {
	server := newStatusTestServer(t, http.StatusOK, `<group/>`)
	defer server.Close()

	conn := webtypes.NewDeviceConnection(
		client.NewClientFromHost(server.URL),
		&models.DeviceInfo{Name: "Old Name", DeviceID: "device-1"},
	)
	NewWebApp().UpdateDeviceStatus("device-1", conn)

	if info := conn.Info(); info == nil || info.Name != "Living Room Left" {
		t.Fatalf("refreshed device info = %+v, want Living Room Left", info)
	}
}

func TestUpdateDeviceStatusDoesNotOverwriteNewerNameEvent(t *testing.T) {
	nameRequestStarted := make(chan struct{})
	releaseNameResponse := make(chan struct{})
	responses := map[string]string{
		"/now_playing": `<nowPlaying source="STANDBY"><playStatus>STOP_STATE</playStatus></nowPlaying>`,
		"/volume":      `<volume><targetvolume>10</targetvolume><actualvolume>10</actualvolume><muteenabled>false</muteenabled></volume>`,
		"/presets":     `<presets/>`,
		"/sources":     `<sources/>`,
		"/bass":        `<bass><targetbass>0</targetbass><actualbass>0</actualbass></bass>`,
		"/getGroup":    `<group/>`,
		"/getZone":     `<zone master="device-1"/>`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path == "/name" {
			close(nameRequestStarted)
			<-releaseNameResponse
			_, _ = w.Write([]byte(`<name>Old Poll Result</name>`))

			return
		}

		body, ok := responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected status endpoint %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)

			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	conn := webtypes.NewDeviceConnection(
		client.NewClientFromHost(server.URL),
		&models.DeviceInfo{Name: "Initial Name", DeviceID: "device-1", Type: "SoundTouch 10"},
	)
	refreshDone := make(chan struct{})
	go func() {
		NewWebApp().UpdateDeviceStatus("device-1", conn)
		close(refreshDone)
	}()

	select {
	case <-nameRequestStarted:
	case <-time.After(time.Second):
		close(releaseNameResponse)
		t.Fatal("name poll did not start")
	}

	conn.MarkEventStreamActivity(time.Now())
	conn.ApplyNameEvent("New Event Name")
	close(releaseNameResponse)

	select {
	case <-refreshDone:
	case <-time.After(time.Second):
		t.Fatal("status refresh did not finish")
	}

	if info := conn.Info(); info == nil || info.Name != "New Event Name" {
		t.Fatalf("device info after stale poll = %+v, want newer event name", info)
	}
}

func TestUpdateDeviceStatusSkipsGroupForNonStereoModel(t *testing.T) {
	var groupRequested atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/getGroup" {
			groupRequested.Store(true)
			http.Error(w, "unsupported endpoint", http.StatusInternalServerError)

			return
		}

		responses := map[string]string{
			"/now_playing": `<nowPlaying source="STANDBY"><playStatus>STOP_STATE</playStatus></nowPlaying>`,
			"/name":        `<name>Living Room Left</name>`,
			"/volume":      `<volume><targetvolume>10</targetvolume><actualvolume>10</actualvolume><muteenabled>false</muteenabled></volume>`,
			"/presets":     `<presets/>`,
			"/sources":     `<sources/>`,
			"/bass":        `<bass><targetbass>0</targetbass><actualbass>0</actualbass></bass>`,
			"/getZone":     `<zone master="device-1"/>`,
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)

			return
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	for _, model := range []string{"SoundTouch 20", "SoundTouch 30"} {
		t.Run(model, func(t *testing.T) {
			conn := webtypes.NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{Type: model})
			NewWebApp().UpdateDeviceStatus("device-1", conn)

			if !conn.Status().IsConnected {
				t.Fatal("successful ordinary status requests should mark a non-stereo model connected")
			}
		})
	}

	if groupRequested.Load() {
		t.Fatal("UpdateDeviceStatus requested /getGroup for a non-stereo model")
	}
}

// TestUpdateDeviceStatusNotConnectedWhenOnlyGroupSucceeds covers a stereo-
// capable device where every substantive status fetch fails but /getGroup
// alone succeeds (a near-guaranteed reply -- even an empty <group/> is a
// success, see Client.GetGroup's doc comment). IsConnected must not be set
// from GetGroup's success alone, or a device with genuinely stale
// NowPlaying/Volume/Presets/Sources/Bass data would be reported connected.
func TestUpdateDeviceStatusNotConnectedWhenOnlyGroupSucceeds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/getGroup" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<group/>`))

			return
		}

		http.Error(w, "device struggling", http.StatusInternalServerError)
	}))
	defer server.Close()

	conn := webtypes.NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{Type: "SoundTouch 10"})
	NewWebApp().UpdateDeviceStatus("device-1", conn)

	if conn.Status().IsConnected {
		t.Fatal("IsConnected must stay false when every substantive status fetch failed, even though GetGroup alone succeeded")
	}
}

func TestApplyGroupUpdatedEventReplacesGroup(t *testing.T) {
	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(nil, nil)
	previousActivity := time.Unix(1, 0)
	conn.SetStatus(&webtypes.DeviceStatus{
		Group:        &models.Group{ID: "pair-old"},
		Volume:       &models.Volume{ActualVolume: 25},
		IsConnected:  true,
		LastActivity: previousActivity,
	})

	event := &models.GroupUpdatedEvent{
		Group: models.Group{ID: "pair-new", Name: "Renamed Pair"},
	}
	if !app.applyGroupUpdatedEvent(conn, event) {
		t.Fatal("new group event should publish a changed projection")
	}

	status := conn.Status()
	if status.Group != &event.Group || status.Group.ID != "pair-new" {
		t.Errorf("Group = %+v, want event group", status.Group)
	}

	if status.Volume == nil || status.Volume.ActualVolume != 25 || !status.IsConnected {
		t.Errorf("unrelated status fields were not preserved: %+v", status)
	}

	if !status.LastActivity.After(previousActivity) {
		t.Errorf("LastActivity = %s, want after %s", status.LastActivity, previousActivity)
	}

	teardown := &models.GroupUpdatedEvent{Group: models.Group{}}
	if !app.applyGroupUpdatedEvent(conn, teardown) {
		t.Fatal("teardown event should publish a changed projection")
	}

	if conn.Status().Group != nil {
		t.Errorf("teardown event did not clear the group: %+v", conn.Status().Group)
	}
}

func TestRefreshZonesAfterEventClearsCachedMasterClaim(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getZone" {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(`<zone master="master-id"/>`))
	}))
	defer server.Close()

	app := NewWebApp()
	master := webtypes.NewDeviceConnection(client.NewClientFromHost(server.URL), &models.DeviceInfo{
		DeviceID: "master-id",
		Name:     "Kitchen",
	})
	master.SetStatus(&webtypes.DeviceStatus{Zone: &models.ZoneInfo{
		Master:  "master-id",
		Members: []models.Member{{DeviceID: "member-id", IP: "192.0.2.20"}},
	}})
	app.AddDevice("192.0.2.10", master)
	member := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{
		DeviceID: "member-id",
		Name:     "Dining",
	})
	app.AddDevice("192.0.2.20", member)

	// Some firmware events omit deviceID. The callback must fall back to the
	// connection's hardware ID, not its IP-keyed registry entry.
	app.refreshZonesAfterEvent(zoneEventDeviceID("", member), "")

	if master.Status().Zone != nil {
		t.Fatalf("dissolved zone remained cached: %+v", master.Status().Zone)
	}
}

func TestReserveZoneRefreshesAfterEventFencesOlderPollBeforeIO(t *testing.T) {
	app := NewWebApp()
	master := webtypes.NewDeviceConnection(client.NewClientFromHost("http://127.0.0.1"), &models.DeviceInfo{
		DeviceID: "master-id",
		Name:     "Kitchen",
	})
	initialZone := &models.ZoneInfo{
		Master:  "master-id",
		Members: []models.Member{{DeviceID: "member-id", IP: "192.0.2.20"}},
	}
	master.SetStatus(&webtypes.DeviceStatus{Zone: initialZone})
	app.AddDevice("192.0.2.10", master)

	staleGeneration := master.BeginZoneRefresh()
	refreshes := app.reserveZoneRefreshesAfterEvent("member-id", "")
	if len(refreshes) != 1 || refreshes[0].connection != master {
		t.Fatalf("reserved refreshes = %+v, want the cached master", refreshes)
	}

	staleZone := &models.ZoneInfo{
		Master:  "master-id",
		Members: []models.Member{{DeviceID: "old-member", IP: "192.0.2.30"}},
	}
	if master.ApplyPolledZone(staleGeneration, "master-id", staleZone) {
		t.Fatal("pre-event /getZone response crossed the event reservation barrier")
	}
	if master.Status().Zone != initialZone {
		t.Fatalf("stale poll changed the cached topology: %+v", master.Status().Zone)
	}
}

func TestPeriodicPlayerMessagesPreserveStatusUpdateStream(t *testing.T) {
	app := NewWebApp()
	group := testStereoGroup()
	for _, entry := range []DeviceEntry{
		projectionDevice("192.0.2.10", "left-id", "Living Room", true, group),
		projectionDevice("192.0.2.11", "right-id", "Living Room", true, group),
		projectionDevice("192.0.2.12", "standalone-id", "Kitchen", false, nil),
	} {
		app.AddDevice(entry.ID, entry.Device)
	}

	messages := app.periodicPlayerMessages()
	if len(messages) != 3 {
		t.Fatalf("periodic messages = %d, want one devices frame and two connected status updates: %+v", len(messages), messages)
	}

	if messages[0].Type != "devices" {
		t.Fatalf("first periodic message type = %q, want devices", messages[0].Type)
	}
	devices, ok := messages[0].Data.(map[string]deviceView)
	if !ok || len(devices) != 2 || devices["192.0.2.10"].StereoPair == nil {
		t.Fatalf("periodic devices frame is not the logical projection: %#v", messages[0].Data)
	}

	statusUpdates := make(map[string]bool)
	for _, message := range messages[1:] {
		if message.Type != "status_update" {
			t.Fatalf("periodic message type = %q, want status_update", message.Type)
		}
		statusUpdates[message.DeviceID] = true
	}
	if !statusUpdates["192.0.2.10"] || !statusUpdates["192.0.2.11"] || statusUpdates["192.0.2.12"] {
		t.Fatalf("unexpected status_update device IDs: %+v", statusUpdates)
	}
}

// TestDeviceWebSocketUpdateCapturesStatusAfterLifecycleOrderingBarrier proves
// writeDeviceWebSocketUpdate captures status only after taking this specific
// connection's own write lock (registerDeviceWebSocketClient), not before --
// the same "capture after taking the lock" guarantee awaitPriorGlobalWebSocketWrites
// relies on for the ordering barrier in completeStereoPairMutation. Holding
// the lock from before the update goroutine starts, across the mutation,
// makes this deterministic via happens-before rather than timing: the update
// can only proceed once we release the lock, by which point ApplyGroupEvent
// has already run in this (the only other) goroutine.
func TestDeviceWebSocketUpdateCapturesStatusAfterLifecycleOrderingBarrier(t *testing.T) {
	app := NewWebApp()
	device := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "left-id"})
	device.SetStatus(&webtypes.DeviceStatus{Group: &models.Group{ID: "pair-old"}})

	recorder := &recordingWebSocketWriter{}
	app.registerDeviceWebSocketClient(recorder)
	defer app.removeDeviceWebSocketClient(recorder)

	app.DeviceWSMutex.RLock()
	connMu := app.DeviceWSClients[recorder]
	app.DeviceWSMutex.RUnlock()

	connMu.Lock()

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- app.writeDeviceWebSocketUpdate(recorder, "left-id", device)
	}()

	device.ApplyGroupEvent(&models.Group{ID: "pair-new"}, time.Now())
	connMu.Unlock()

	select {
	case err := <-updateDone:
		if err != nil {
			t.Fatalf("writeDeviceWebSocketUpdate: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("device WebSocket update remained blocked")
	}

	message, ok := recorder.firstMessage()
	if !ok || message.Type != "device_status" {
		t.Fatalf("first device message = %#v", message)
	}
	data, ok := message.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("device message data = %#v", message.Data)
	}
	status, ok := data["status"].(*webtypes.DeviceStatus)
	if !ok || status.Group == nil || status.Group.ID != "pair-new" {
		t.Fatalf("device frame captured stale group: %#v", data["status"])
	}
}

func newStatusTestServer(t *testing.T, groupStatus int, groupBody string) *httptest.Server {
	t.Helper()

	responses := map[string]string{
		"/now_playing": `<nowPlaying source="STANDBY"><playStatus>STOP_STATE</playStatus></nowPlaying>`,
		"/name":        `<name>Living Room Left</name>`,
		"/volume":      `<volume><targetvolume>10</targetvolume><actualvolume>10</actualvolume><muteenabled>false</muteenabled></volume>`,
		"/presets":     `<presets/>`,
		"/sources":     `<sources/>`,
		"/bass":        `<bass><targetbass>0</targetbass><actualbass>0</actualbass></bass>`,
		"/getZone":     `<zone master="master-1"><member ipaddress="192.0.2.20">member-2</member></zone>`,
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method for %s = %s, want GET", r.URL.Path, r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)

			return
		}

		w.Header().Set("Content-Type", "application/xml")

		if r.URL.Path == "/getGroup" {
			w.WriteHeader(groupStatus)
			_, _ = w.Write([]byte(groupBody))

			return
		}

		body, ok := responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected status endpoint %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)

			return
		}

		_, _ = w.Write([]byte(body))
	}))
}

type recordingWebSocketWriter struct {
	mu        sync.Mutex
	deadlines []time.Time
	messages  []interface{}
}

func (writer *recordingWebSocketWriter) SetWriteDeadline(deadline time.Time) error {
	writer.mu.Lock()
	writer.deadlines = append(writer.deadlines, deadline)
	writer.mu.Unlock()

	return nil
}

func (writer *recordingWebSocketWriter) WriteJSON(value interface{}) error {
	writer.mu.Lock()
	writer.messages = append(writer.messages, value)
	writer.mu.Unlock()

	return nil
}

func (*recordingWebSocketWriter) WriteMessage(int, []byte) error { return nil }

func (writer *recordingWebSocketWriter) lastDeadline() (time.Time, bool) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.deadlines) == 0 {
		return time.Time{}, false
	}

	return writer.deadlines[len(writer.deadlines)-1], true
}

func (writer *recordingWebSocketWriter) messageSnapshot() []interface{} {
	writer.mu.Lock()
	defer writer.mu.Unlock()

	return append([]interface{}(nil), writer.messages...)
}

func (writer *recordingWebSocketWriter) firstMessage() (webtypes.WebSocketMessage, bool) {
	writer.mu.Lock()
	defer writer.mu.Unlock()
	if len(writer.messages) == 0 {
		return webtypes.WebSocketMessage{}, false
	}

	message, ok := writer.messages[0].(webtypes.WebSocketMessage)

	return message, ok
}

type deadlineBlockingWebSocketWriter struct {
	mu       sync.Mutex
	deadline time.Time
	started  chan struct{}
	once     sync.Once
}

func (writer *deadlineBlockingWebSocketWriter) SetWriteDeadline(deadline time.Time) error {
	writer.mu.Lock()
	writer.deadline = deadline
	writer.mu.Unlock()

	return nil
}

func (writer *deadlineBlockingWebSocketWriter) WriteJSON(interface{}) error {
	writer.once.Do(func() { close(writer.started) })
	writer.mu.Lock()
	deadline := writer.deadline
	writer.mu.Unlock()

	if delay := time.Until(deadline); delay > 0 {
		time.Sleep(delay)
	}

	return errors.New("write deadline exceeded")
}

func (writer *deadlineBlockingWebSocketWriter) WriteMessage(int, []byte) error {
	return writer.WriteJSON(nil)
}

// dialTestWebSocket opens a real client/server WebSocket pair against app's
// Upgrader. remote is the server-side *websocket.Conn (the one production
// code operates on -- e.g. as a WSClients key); client is the dialer side,
// for tests that need to read the frames production code writes to remote.
// It does not register remote into app.WSClients; callers do that
// themselves so each test controls its own per-connection lock.
func dialTestWebSocket(t *testing.T, app *WebApp) (remote *websocket.Conn, client *websocket.Conn, cleanup func()) {
	t.Helper()

	server, serverConnection, release := newTestWebSocketServer(t, app)

	client, response, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(server.URL, "http"), nil,
	)
	if response != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		release()
		server.Close()
		t.Fatalf("dial test WebSocket: %v", err)
	}

	remote = <-serverConnection

	return remote, client, func() {
		release()
		_ = client.Close()
		server.Close()
	}
}

// newTestWebSocketServer starts an httptest.Server that upgrades every
// request through app.Upgrader and holds each connection open until
// release is called. Shared by dialTestWebSocket (which always expects a
// successful handshake) and tests that also need to exercise an *expected*
// rejection (e.g. origin-policy tests), which is why the handshake failure
// path here stays silent rather than failing the test.
func newTestWebSocketServer(t *testing.T, app *WebApp) (
	server *httptest.Server,
	serverConnection <-chan *websocket.Conn,
	release func(),
) {
	t.Helper()

	// Buffered generously: some callers (e.g. table-driven origin-policy
	// tests) dial the same server multiple times without draining
	// serverConnection, and an unread successful upgrade must not block
	// its own handler goroutine on this channel send.
	conns := make(chan *websocket.Conn, 8)
	releaseServer := make(chan struct{})
	var releaseOnce sync.Once

	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := app.Upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns <- conn
		<-releaseServer
		_ = conn.Close()
	}))

	return server, conns, func() { releaseOnce.Do(func() { close(releaseServer) }) }
}

func TestConnWriteSerializesWritesToSameConnection(t *testing.T) {
	app := NewWebApp()
	remote, _, cleanup := dialTestWebSocket(t, app)
	defer cleanup()

	connMu := &sync.Mutex{}
	app.WSMutex.Lock()
	app.WSClients[remote] = connMu
	app.WSMutex.Unlock()

	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	go func() {
		_ = app.withConnWrite(remote, func(webSocketWriteBatch) error {
			close(entered)
			<-release

			return nil
		})
		close(done)
	}()

	<-entered
	if connMu.TryLock() {
		connMu.Unlock()
		t.Fatal("connection writer lock was not held across the write seam")
	}

	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("connection writer lock was not released")
	}
}

// TestConnWriteDoesNotBlockUnrelatedConnection is the money test for the
// removed global write mutex: a write to one connection must never block a
// write to a different connection.
func TestConnWriteDoesNotBlockUnrelatedConnection(t *testing.T) {
	app := NewWebApp()
	remoteA, _, cleanupA := dialTestWebSocket(t, app)
	defer cleanupA()
	remoteB, _, cleanupB := dialTestWebSocket(t, app)
	defer cleanupB()

	app.WSMutex.Lock()
	app.WSClients[remoteA] = &sync.Mutex{}
	app.WSClients[remoteB] = &sync.Mutex{}
	app.WSMutex.Unlock()

	blockedA := make(chan struct{})
	releaseA := make(chan struct{})

	go func() {
		_ = app.withConnWrite(remoteA, func(webSocketWriteBatch) error {
			close(blockedA)
			<-releaseA

			return nil
		})
	}()
	<-blockedA

	done := make(chan error, 1)
	go func() {
		done <- app.withConnWrite(remoteB, func(webSocketWriteBatch) error {
			return nil
		})
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write to unrelated connection failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("write to an unrelated connection blocked on a stalled connection's lock")
	}

	close(releaseA)
}

// TestBroadcastDeviceListSkipsStalledClientWithoutBlockingHealthyClient
// encodes the HandleDeleteDevice failure scenario from the code review:
// a stalled/backgrounded client must not delay a healthy client's OWN
// periodic update (which runs on a separate goroutine, contending only on
// that client's own connection lock) merely because BroadcastDeviceList --
// possibly called synchronously from an HTTP handler -- is concurrently
// stuck writing to the stalled client.
func TestBroadcastDeviceListSkipsStalledClientWithoutBlockingHealthyClient(t *testing.T) {
	app := NewWebApp()
	remoteA, _, cleanupA := dialTestWebSocket(t, app)
	defer cleanupA()
	remoteB, _, cleanupB := dialTestWebSocket(t, app)
	defer cleanupB()

	app.WSMutex.Lock()
	app.WSClients[remoteA] = &sync.Mutex{}
	app.WSClients[remoteB] = &sync.Mutex{}
	app.WSMutex.Unlock()

	// Simulate client A's own in-flight write (e.g. its periodic ticker
	// mid-write) by holding its connection lock directly.
	app.WSMutex.RLock()
	connMuA := app.WSClients[remoteA]
	app.WSMutex.RUnlock()
	connMuA.Lock()

	broadcastDone := make(chan struct{})
	go func() {
		app.BroadcastDeviceList()
		close(broadcastDone)
	}()

	// Client B's own periodic update path (a separate goroutine, exactly as
	// HandleWebSocket's ticker loop does) must succeed immediately, whether
	// or not BroadcastDeviceList's internal loop currently happens to be
	// stuck waiting on A.
	bDone := make(chan error, 1)
	go func() {
		bDone <- app.withConnWrite(remoteB, func(webSocketWriteBatch) error { return nil })
	}()

	select {
	case err := <-bDone:
		if err != nil {
			t.Fatalf("healthy client's own update failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("healthy client's own update was blocked by an unrelated stalled client")
	}

	connMuA.Unlock()
	select {
	case <-broadcastDone:
	case <-time.After(time.Second):
		t.Fatal("BroadcastDeviceList did not complete after the stalled client's lock was released")
	}
}

func TestDiscoveryStatusStateAndFramesShareWriteOrder(t *testing.T) {
	app := NewWebApp()
	writer := &recordingWebSocketWriter{}
	starting := &webtypes.DiscoveryStatus{Status: "starting", IsDiscovering: true}
	completed := &webtypes.DiscoveryStatus{Status: "completed", DeviceCount: 3}
	firstEntered := make(chan struct{})
	releaseFirst := make(chan struct{})
	firstDone := make(chan error, 1)

	go func() {
		firstDone <- app.withDiscoveryStatusWrite(starting, func(
			batch webSocketWriteBatch,
			_ []*websocket.Conn,
		) error {
			close(firstEntered)
			<-releaseFirst

			return batch.writeJSON(writer, webtypes.WebSocketMessage{
				Type: "discovery_status",
				Data: starting,
			})
		})
	}()

	<-firstEntered
	secondStarted := make(chan struct{})
	secondDone := make(chan error, 1)
	go func() {
		close(secondStarted)
		secondDone <- app.withDiscoveryStatusWrite(completed, func(
			batch webSocketWriteBatch,
			_ []*websocket.Conn,
		) error {
			return batch.writeJSON(writer, webtypes.WebSocketMessage{
				Type: "discovery_status",
				Data: completed,
			})
		})
	}()
	<-secondStarted

	if stored := app.discoveryStatus.Load(); stored != starting {
		t.Fatalf("discovery state changed ahead of its serialized frame: %#v", stored)
	}
	select {
	case err := <-secondDone:
		t.Fatalf("second discovery publication bypassed the writer lock: %v", err)
	default:
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("publish starting discovery status: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("publish completed discovery status: %v", err)
	}

	if stored := app.discoveryStatus.Load(); stored != completed {
		t.Fatalf("final discovery state = %#v, want completed", stored)
	}
	messages := writer.messageSnapshot()
	if len(messages) != 2 {
		t.Fatalf("discovery frames = %d, want 2", len(messages))
	}
	last, ok := messages[1].(webtypes.WebSocketMessage)
	if !ok || last.Data != completed {
		t.Fatalf("last discovery frame = %#v, want completed", messages[1])
	}
}

// TestRegistrationDuringDiscoveryPublicationIsNotBlockedAndSeesLatestState
// replaces a prior version of this test that asserted registration BLOCKS
// until an in-flight, unrelated discovery publication finishes. That
// coupling was removed deliberately (see discoveryPublishMu's doc comment):
// registration no longer shares any lock with publication. This proves the
// weaker, now-true property instead: registration is never blocked by an
// in-flight publication, yet still never observes a state older than what
// was stored before that publication began -- because
// withDiscoveryStatusWrite always calls discoveryStatus.Store() before
// taking its client snapshot, so by the time the publication's write
// callback is reached (clientsSelected below), the new status is already
// the one any concurrent registration will read.
func TestRegistrationDuringDiscoveryPublicationIsNotBlockedAndSeesLatestState(t *testing.T) {
	app := NewWebApp()
	oldStatus := &webtypes.DiscoveryStatus{Status: "starting", IsDiscovering: true}
	newStatus := &webtypes.DiscoveryStatus{Status: "completed", DeviceCount: 3}
	app.discoveryStatus.Store(oldStatus)

	remote, client, cleanup := dialTestWebSocket(t, app)
	defer cleanup()
	t.Cleanup(func() { app.removeGlobalWebSocketClient(remote) })

	clientsSelected := make(chan struct{})
	releasePublication := make(chan struct{})
	published := make(chan error, 1)
	go func() {
		published <- app.withDiscoveryStatusWrite(newStatus, func(
			_ webSocketWriteBatch,
			_ []*websocket.Conn,
		) error {
			close(clientsSelected)
			<-releasePublication

			return nil
		})
	}()

	select {
	case <-clientsSelected:
	case <-time.After(time.Second):
		t.Fatal("discovery publication did not reach its write callback")
	}

	registered := make(chan error, 1)
	go func() {
		registered <- app.registerGlobalWebSocket(remote)
	}()

	// Registration must complete WITHOUT waiting for the in-flight,
	// unrelated publication to release releasePublication.
	select {
	case err := <-registered:
		if err != nil {
			t.Fatalf("register global WebSocket: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("registration blocked on an in-flight, unrelated discovery publication")
	}

	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set test WebSocket deadline: %v", err)
	}
	var discoveryMessage webtypes.WebSocketMessage
	if err := client.ReadJSON(&discoveryMessage); err != nil {
		t.Fatalf("read initial discovery frame: %v", err)
	}
	if discoveryMessage.Type != "discovery_status" {
		t.Fatalf("initial frame type = %q, want discovery_status", discoveryMessage.Type)
	}
	data, ok := discoveryMessage.Data.(map[string]interface{})
	if !ok || data["status"] != "completed" || data["deviceCount"] != float64(3) {
		t.Fatalf("initial discovery frame = %#v, want new completed state (already stored before this publication reached its write callback)", discoveryMessage.Data)
	}

	close(releasePublication)
	if err := <-published; err != nil {
		t.Fatalf("publish discovery status: %v", err)
	}
}

func TestWebSocketWriteBatchRefreshesDeadlineForHealthyWriter(t *testing.T) {
	timeout := 30 * time.Millisecond
	batch := webSocketWriteBatch{timeout: timeout}
	blocked := &deadlineBlockingWebSocketWriter{started: make(chan struct{})}

	if err := batch.writeJSON(blocked, struct{}{}); err == nil {
		t.Fatal("blocked writer unexpectedly succeeded")
	}

	healthy := &recordingWebSocketWriter{}
	started := time.Now()
	if err := batch.writeJSON(healthy, webtypes.WebSocketMessage{Type: "devices"}); err != nil {
		t.Fatalf("healthy writer inherited the failed client's deadline: %v", err)
	}

	deadline, ok := healthy.lastDeadline()
	if !ok || deadline.Before(started.Add(timeout/2)) {
		t.Fatalf("healthy writer deadline = %v, want a fresh deadline after %v", deadline, started)
	}
}

func TestSpeakerEventHelpersPublishOnlyChangedPayloads(t *testing.T) {
	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})

	nowPlaying := &models.NowPlaying{Source: "LOCAL_INTERNET_RADIO", Track: "Test station"}
	if !app.applyNowPlayingEvent(conn, nowPlaying) {
		t.Fatal("new now-playing payload was not reported as changed")
	}
	if app.applyNowPlayingEvent(conn, nowPlaying) {
		t.Fatal("duplicate now-playing payload was reported as changed")
	}

	volume := &models.Volume{ActualVolume: 25, TargetVolume: 25}
	if !app.applyVolumeEvent(conn, volume) {
		t.Fatal("new volume payload was not reported as changed")
	}
	if app.applyVolumeEvent(conn, volume) {
		t.Fatal("duplicate volume payload was reported as changed")
	}

	if !app.applyConnectionStateEvent(conn, true) {
		t.Fatal("new connection state was not reported as changed")
	}
	if app.applyConnectionStateEvent(conn, true) {
		t.Fatal("duplicate connection state was reported as changed")
	}

	presets := &models.Presets{Preset: []models.Preset{{ID: 1}}}
	if !app.applyPresetEvent(conn, presets) {
		t.Fatal("new presets payload was not reported as changed")
	}
	if app.applyPresetEvent(conn, presets) {
		t.Fatal("duplicate presets payload was reported as changed")
	}

	bass := &models.Bass{ActualBass: -2}
	if !app.applyBassEvent(conn, bass) {
		t.Fatal("new bass payload was not reported as changed")
	}
	if app.applyBassEvent(conn, bass) {
		t.Fatal("duplicate bass payload was reported as changed")
	}
}

func TestSpeakerEventPublishesImmediateDeviceProjection(t *testing.T) {
	app := NewWebApp()
	device := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{Name: "test"})
	device.SetStatus(&webtypes.DeviceStatus{
		Volume:      &models.Volume{ActualVolume: 10, TargetVolume: 10},
		IsConnected: true,
	})
	app.AddDevice("speaker", device)

	remote, client, cleanup := dialTestWebSocket(t, app)
	defer cleanup()
	t.Cleanup(func() { app.removeGlobalWebSocketClient(remote) })

	if err := app.registerGlobalWebSocket(remote); err != nil {
		t.Fatalf("register browser WebSocket: %v", err)
	}
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set initial read deadline: %v", err)
	}
	var initial webtypes.WebSocketMessage
	if err := client.ReadJSON(&initial); err != nil {
		t.Fatalf("read initial device projection: %v", err)
	}

	// Simulate the browser fan-out being busy by holding this connection's
	// own write lock directly (the per-connection replacement for the
	// removed global webSocketWriteMu -- see withConnWrite).
	app.WSMutex.RLock()
	connMu := app.WSClients[remote]
	app.WSMutex.RUnlock()
	connMu.Lock()
	writerLocked := true
	defer func() {
		if writerLocked {
			connMu.Unlock()
		}
	}()

	applied := make(chan bool, 1)
	go func() {
		applied <- app.applyVolumeEvent(device, &models.Volume{ActualVolume: 25, TargetVolume: 25})
	}()

	select {
	case changed := <-applied:
		if !changed {
			t.Fatal("volume event was not applied")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("speaker event blocked on browser WebSocket I/O")
	}

	connMu.Unlock()
	writerLocked = false

	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set event read deadline: %v", err)
	}
	var update webtypes.WebSocketMessage
	if err := client.ReadJSON(&update); err != nil {
		t.Fatalf("read immediate device projection: %v", err)
	}
	if update.Type != "devices" {
		t.Fatalf("event frame type = %q, want devices", update.Type)
	}

	devices, ok := update.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event projection = %#v, want device map", update.Data)
	}
	speaker, ok := devices["speaker"].(map[string]interface{})
	if !ok {
		t.Fatalf("speaker projection = %#v, want object", devices["speaker"])
	}
	status, ok := speaker["status"].(map[string]interface{})
	if !ok {
		t.Fatalf("speaker status = %#v, want object", speaker["status"])
	}
	volume, ok := status["volume"].(map[string]interface{})
	if !ok || volume["ActualVolume"] != float64(25) {
		t.Fatalf("projected volume = %#v, want 25", status["volume"])
	}
}

func TestQueuedDeviceBroadcastCoalescesBurst(t *testing.T) {
	app := NewWebApp()

	// A registered connection whose write lock is held externally gives
	// BroadcastDeviceList something to block on -- the per-connection
	// replacement for holding the removed global webSocketWriteMu.
	remote, _, cleanup := dialTestWebSocket(t, app)
	defer cleanup()
	t.Cleanup(func() { app.removeGlobalWebSocketClient(remote) })
	if err := app.registerGlobalWebSocket(remote); err != nil {
		t.Fatalf("register browser WebSocket: %v", err)
	}

	app.WSMutex.RLock()
	connMu := app.WSClients[remote]
	app.WSMutex.RUnlock()
	connMu.Lock()
	writerLocked := true
	defer func() {
		if writerLocked {
			connMu.Unlock()
		}
	}()

	app.QueueDeviceListBroadcast()
	deadline := time.Now().Add(time.Second)
	for {
		app.deviceBroadcastMu.Lock()
		running := app.deviceBroadcastRunning
		pending := app.deviceBroadcastPending
		app.deviceBroadcastMu.Unlock()
		if running && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("broadcast worker did not begin its blocked write")
		}
		time.Sleep(time.Millisecond)
	}

	for range 100 {
		app.QueueDeviceListBroadcast()
	}

	app.deviceBroadcastMu.Lock()
	if !app.deviceBroadcastPending {
		app.deviceBroadcastMu.Unlock()
		t.Fatal("burst did not retain one coalesced follow-up")
	}
	app.deviceBroadcastMu.Unlock()

	connMu.Unlock()
	writerLocked = false

	deadline = time.Now().Add(time.Second)
	for {
		app.deviceBroadcastMu.Lock()
		running := app.deviceBroadcastRunning
		pending := app.deviceBroadcastPending
		app.deviceBroadcastMu.Unlock()
		if !running && !pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("coalesced worker did not drain: running=%v pending=%v", running, pending)
		}
		time.Sleep(time.Millisecond)
	}
}
