package soundtouchweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// pollCountingSpeaker answers /info and counts /now_playing requests, the
// first request of every status round, so the count is the number of rounds
// the player ran against it. Every other status endpoint answers 404, which
// the round records as a failed field and otherwise ignores.
type pollCountingSpeaker struct {
	host   string
	rounds atomic.Int64
	// gate, when set, holds each /now_playing request until it is closed.
	gate atomic.Pointer[chan struct{}]
}

func newPollCountingSpeaker(t *testing.T) *pollCountingSpeaker {
	t.Helper()

	speaker := &pollCountingSpeaker{}

	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/info":
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<info deviceID="DEVICEID01"><name>Kitchen</name><type>SoundTouch 20</type></info>`))
		case "/now_playing":
			speaker.rounds.Add(1)

			if gate := speaker.gate.Load(); gate != nil {
				select {
				case <-*gate:
				case <-r.Context().Done():
				}
			}

			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<nowPlaying deviceID="DEVICEID01" source="STANDBY"><ContentItem source="STANDBY" isPresetable="false" /></nowPlaying>`))
		default:
			http.NotFound(w, r)
		}
	}))
	server.Start()

	speaker.host = strings.TrimPrefix(server.URL, "http://")

	return speaker
}

// hold makes /now_playing block until the returned release is called.
func (s *pollCountingSpeaker) hold() (release func()) {
	gate := make(chan struct{})
	s.gate.Store(&gate)

	var once sync.Once

	return func() {
		once.Do(func() {
			s.gate.Store(nil)
			close(gate)
		})
	}
}

func newPollTestApp(t *testing.T, interval time.Duration) *WebApp {
	t.Helper()

	app := NewWebApp()
	app.statusPollInterval = interval
	cleanupRegistry(t, app)

	return app
}

// registerPollTestDevice registers speaker without the add-time status read,
// so a test counts only the rounds it causes itself.
func registerPollTestDevice(t *testing.T, app *WebApp, speaker *pollCountingSpeaker) *webtypes.DeviceConnection {
	t.Helper()

	conn := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: speaker.host, Timeout: 2 * time.Second}),
		&models.DeviceInfo{DeviceID: "DEVICEID01", Name: "Kitchen", Type: "SoundTouch 20"},
	)
	if !app.AddDevice(speaker.host, conn) {
		t.Fatalf("registering %s failed", speaker.host)
	}

	return conn
}

func waitForRounds(t *testing.T, speaker *pollCountingSpeaker, want int64) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for speaker.rounds.Load() < want {
		if time.Now().After(deadline) {
			t.Fatalf("status rounds = %d, want at least %d", speaker.rounds.Load(), want)
		}

		time.Sleep(5 * time.Millisecond)
	}
}

// assertRoundsStay fails if the speaker sees another round within d.
func assertRoundsStay(t *testing.T, speaker *pollCountingSpeaker, d time.Duration) {
	t.Helper()

	before := speaker.rounds.Load()
	time.Sleep(d)

	if after := speaker.rounds.Load(); after != before {
		t.Fatalf("status rounds went from %d to %d while nothing was watching", before, after)
	}
}

func (app *WebApp) currentPollDone() chan struct{} {
	app.watchMu.Lock()
	defer app.watchMu.Unlock()

	return app.pollDone
}

func (app *WebApp) watcherCount() int {
	app.watchMu.Lock()
	defer app.watchMu.Unlock()

	return app.watchers
}

func TestUnwatchedPlayerDoesNotPollAfterTheAddTimeRead(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, 10*time.Millisecond)

	if app.addDeviceByHost(context.Background(), speaker.host, 8090, "manual") == nil {
		t.Fatalf("registering %s failed", speaker.host)
	}

	// The one-shot read at add time still happens.
	waitForRounds(t, speaker, 1)

	// Many poll intervals pass, nobody is watching: no further rounds.
	assertRoundsStay(t, speaker, 150*time.Millisecond)
}

func TestPollStartsWithAnImmediateRefreshAndStopsWithTheLastWatcher(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	// Far longer than the test: any round seen here is the immediate one.
	app := newPollTestApp(t, time.Hour)
	registerPollTestDevice(t, app, speaker)

	first := &recordingWebSocketWriter{}
	app.registerDeviceWebSocketClient(first)

	waitForRounds(t, speaker, 1)

	// A second watcher neither starts a second poll nor refreshes again.
	second := &recordingWebSocketWriter{}
	app.registerDeviceWebSocketClient(second)

	if got := app.watcherCount(); got != 2 {
		t.Fatalf("watchers = %d, want 2", got)
	}

	done := app.currentPollDone()
	app.removeDeviceWebSocketClient(first)

	select {
	case <-done:
		t.Fatal("poll stopped while a watcher was still connected")
	case <-time.After(20 * time.Millisecond):
	}

	app.removeDeviceWebSocketClient(second)
	// Removing an unknown client must not drive the count below zero.
	app.removeDeviceWebSocketClient(second)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("poll still running after the last watcher left")
	}

	if got := app.watcherCount(); got != 0 {
		t.Fatalf("watchers = %d, want 0", got)
	}

	if got := speaker.rounds.Load(); got != 1 {
		t.Fatalf("status rounds = %d, want exactly the one immediate refresh", got)
	}
}

func TestPollTicksWhileWatchedAndNotAfterwards(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, 10*time.Millisecond)
	registerPollTestDevice(t, app, speaker)

	watcher := &recordingWebSocketWriter{}
	app.registerDeviceWebSocketClient(watcher)

	waitForRounds(t, speaker, 3)

	done := app.currentPollDone()
	app.removeDeviceWebSocketClient(watcher)
	<-done

	// A round may have been in flight when the poll stopped; let it land.
	time.Sleep(20 * time.Millisecond)
	assertRoundsStay(t, speaker, 100*time.Millisecond)
}

func TestImmediateRefreshSkipsAFreshCache(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, time.Hour)
	conn := registerPollTestDevice(t, app, speaker)

	// A page closed and reopened within the TTL does not cost a new round.
	conn.MarkStatusRefreshed(time.Now())

	watcher := &recordingWebSocketWriter{}
	app.registerDeviceWebSocketClient(watcher)
	t.Cleanup(func() { app.removeDeviceWebSocketClient(watcher) })

	assertRoundsStay(t, speaker, 100*time.Millisecond)
}

func TestBrowserWebSocketCountsAsWatcher(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, time.Hour)
	registerPollTestDevice(t, app, speaker)

	server := httptest.NewTestServer(t, http.HandlerFunc(app.HandleWebSocket))
	server.Start()

	browser, response, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if response != nil {
		_ = response.Body.Close()
	}

	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	waitForRounds(t, speaker, 1)

	done := app.currentPollDone()
	if done == nil {
		t.Fatal("no status poll running while a browser is connected")
	}

	_ = browser.Close()

	// The server notices the close on its next read or write; the periodic
	// frame goes out every 5 s, so allow for that.
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("poll still running after the browser disconnected")
	}
}

func TestDevicesEndpointRefreshesAStaleCacheOnceForABurst(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, time.Hour)
	conn := registerPollTestDevice(t, app, speaker)

	// The device must not look offline, or the request would not wait.
	conn.MarkHTTPSuccess(time.Now())
	conn.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.Connectivity = webtypes.ConnectivityOnline
	})

	release := speaker.hold()
	defer release()

	const burst = 8

	var wg sync.WaitGroup

	for range burst {
		wg.Add(1)

		go func() {
			defer wg.Done()

			recorder := httptest.NewRecorder()
			app.HandleAPIDevices(recorder, httptest.NewRequest(http.MethodGet, "/api/control/devices", nil))

			if recorder.Code != http.StatusOK {
				t.Errorf("status = %d", recorder.Code)
			}
		}()
	}

	waitForRounds(t, speaker, 1)
	// Give the rest of the burst time to pile up behind the held round.
	time.Sleep(50 * time.Millisecond)
	release()
	wg.Wait()

	if got := speaker.rounds.Load(); got != 1 {
		t.Fatalf("status rounds = %d for a burst of %d requests, want 1", got, burst)
	}

	// Within the TTL the cache answers on its own.
	recorder := httptest.NewRecorder()
	app.HandleAPIDevices(recorder, httptest.NewRequest(http.MethodGet, "/api/control/devices", nil))

	if got := speaker.rounds.Load(); got != 1 {
		t.Fatalf("status rounds = %d after a request within the TTL, want 1", got)
	}

	if refreshed := conn.StatusRefreshedAt(); refreshed.IsZero() {
		t.Fatal("round did not record its end")
	}
}

func TestDevicesEndpointRefreshesAgainOnceTheTTLHasPassed(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, time.Hour)
	app.statusCacheTTL = 20 * time.Millisecond
	conn := registerPollTestDevice(t, app, speaker)
	conn.UpdateStatus(func(status *webtypes.DeviceStatus) {
		status.Connectivity = webtypes.ConnectivityOnline
	})

	get := func() {
		recorder := httptest.NewRecorder()
		app.HandleAPIDevices(recorder, httptest.NewRequest(http.MethodGet, "/api/control/devices", nil))
	}

	get()
	waitForRounds(t, speaker, 1)

	time.Sleep(40 * time.Millisecond)
	get()
	waitForRounds(t, speaker, 2)
}

func TestRemovingAWatchedDeviceStopsItsPolling(t *testing.T) {
	speaker := newPollCountingSpeaker(t)
	app := newPollTestApp(t, 10*time.Millisecond)
	conn := registerPollTestDevice(t, app, speaker)

	watcher := &recordingWebSocketWriter{}
	app.registerDeviceWebSocketClient(watcher)
	t.Cleanup(func() { app.removeDeviceWebSocketClient(watcher) })

	waitForRounds(t, speaker, 2)

	if !app.RemoveDevice(speaker.host) {
		t.Fatal("device was not registered")
	}

	select {
	case <-conn.Done():
	default:
		t.Fatal("RemoveDevice did not close the connection")
	}

	// A round may have been in flight at removal; let it land.
	time.Sleep(30 * time.Millisecond)
	assertRoundsStay(t, speaker, 100*time.Millisecond)

	// A shared refresh for the removed connection is refused outright.
	if done := app.refreshDeviceStatusShared(speaker.host, conn, 0); done != nil {
		t.Fatal("refresh started for a removed device")
	}
}

func TestWatcherCountIsRaceFree(t *testing.T) {
	app := newPollTestApp(t, time.Hour)

	var wg sync.WaitGroup

	for range 32 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for range 20 {
				writer := &recordingWebSocketWriter{}
				app.registerDeviceWebSocketClient(writer)
				app.removeDeviceWebSocketClient(writer)
			}
		}()
	}

	wg.Wait()

	if got := app.watcherCount(); got != 0 {
		t.Fatalf("watchers = %d after every client left, want 0", got)
	}

	if app.currentPollDone() != nil {
		t.Fatal("a status poll is still registered after every client left")
	}
}
