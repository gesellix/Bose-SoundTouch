package soundtouchweb

import (
	"bytes"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gorilla/websocket"
)

func TestSpeakerWebSocketLoggerOutageLifecycle(t *testing.T) {
	conn := webtypes.NewDeviceConnection(nil, nil)
	t.Cleanup(conn.Close)
	l := &speakerWebSocketLogger{deviceID: "speaker\r\nID", conn: conn}
	out := captureLog(t, func() {
		for range 20 {
			l.initialFailure(errors.New("dial\r\nfailed"))
		}
		// Log text alone is never evidence of recovery.
		l.Printf("Connected to %s", "speaker")
		l.initialFailure(errors.New("still unavailable"))
		if !l.transportState(true, 1) {
			t.Fatal("initial transport rejected")
		}

		var workers sync.WaitGroup
		for range 20 {
			workers.Go(func() {
				l.Printf("WebSocket read error: %v", errors.New("read failed"))
				l.Printf("Failed to send ping: %v", errors.New("ping failed"))
				l.transportState(false, 2)
			})
		}
		workers.Wait()
		if !l.transportState(true, 3) || l.transportState(false, 2) {
			t.Fatal("transport generation guard failed")
		}
		// Delayed messages from the old read/ping loops cannot start a new
		// outage after an accepted recovery.
		l.Printf("Failed to send ping: %v", errors.New("late ping failure"))
		conn.Close()
		l.transportState(false, 4)
		l.initialFailure(errors.New("closed"))
	})
	if strings.Count(out, "WebSocket unavailable") != 2 || strings.Count(out, "WebSocket recovered") != 2 {
		t.Fatalf("want two outage/recovery pairs, got:\n%s", out)
	}
	if strings.Count(out, "\n") != 4 || !strings.Contains(out, `speaker\r\nID: dial\r\nfailed`) {
		t.Fatalf("unexpected or unsanitized log lines: %q", out)
	}
}

// The client's read error explains why the stream dropped. It is filtered as
// routine noise, so the outage line carries it instead, once: a later drop
// with no read error of its own must not inherit it.
func TestSpeakerWebSocketLoggerOutageNamesTheReadError(t *testing.T) {
	conn := webtypes.NewDeviceConnection(nil, nil)
	t.Cleanup(conn.Close)
	l := &speakerWebSocketLogger{deviceID: "speaker", conn: conn}
	out := captureLog(t, func() {
		l.transportState(true, 1)
		l.Printf("WebSocket read error: %v", errors.New("websocket: close 1011 (internal server error)\r\n"))
		l.transportState(false, 2)
		l.transportState(true, 3)
		l.transportState(false, 4)
		l.transportState(true, 5)
		// A read error whose disconnect never arrives is dropped by the next
		// accepted recovery, not reused for a later outage.
		l.Printf("WebSocket read error: %v", errors.New("stale"))
		l.transportState(true, 6)
		l.transportState(false, 7)
	})
	for _, want := range []string{
		`WebSocket unavailable for device speaker: WebSocket read error: websocket: close 1011 (internal server error)\r\n`,
		"WebSocket unavailable for device speaker: transport disconnected",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Count(out, "WebSocket unavailable") != 3 || strings.Contains(out, "stale") ||
		strings.Count(out, "transport disconnected") != 2 {
		t.Fatalf("read error was not reported exactly once:\n%s", out)
	}
}

func TestSpeakerWebSocketLoggerDevicesAreIndependent(t *testing.T) {
	a := webtypes.NewDeviceConnection(nil, nil)
	b := webtypes.NewDeviceConnection(nil, nil)
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	first := &speakerWebSocketLogger{deviceID: "first", conn: a}
	second := &speakerWebSocketLogger{deviceID: "second", conn: b}
	out := captureLog(t, func() {
		first.initialFailure(errors.New("offline"))
		second.transportState(true, 1)
		second.transportState(false, 2)
		first.initialFailure(errors.New("still offline"))
		second.transportState(true, 3)
		first.transportState(true, 1)
		first.transportState(false, 2)
		first.transportState(true, 3)
		second.conn.Close()
		second.transportState(false, 4)
	})
	for _, want := range []string{
		"WebSocket connected for device second",
		"WebSocket unavailable for device first: offline",
		"WebSocket unavailable for device second: transport disconnected",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	if strings.Count(out, "WebSocket unavailable") != 3 || strings.Count(out, "WebSocket recovered") != 3 {
		t.Fatalf("device outage state was shared: %q", out)
	}
}

func TestSpeakerWebSocketLoggerPreservesSanitizedDiagnostics(t *testing.T) {
	l := &speakerWebSocketLogger{deviceID: "device\nID"}
	out := captureLog(t, func() {
		l.Printf("Connecting to %s", "speaker")
		l.Printf("Connected to %s", "speaker")
		l.Printf("Disconnected")
		l.Printf("WebSocket read error: %v", errors.New("read failed"))
		l.Printf("Failed to send ping: %v", errors.New("ping failed"))
		l.Printf("Reconnection attempt %d", 1)
		l.Printf("Reconnection attempt %d failed: %v", 1, errors.New("dial failed"))
		l.Printf("Reconnected successfully")
		l.Printf("Max reconnection attempts reached or reconnection disabled")
		l.Printf("Failed to parse WebSocket message: %v", errors.New("malformed\r\nXML"))
		l.Printf("Unknown special message type: %s", "unknown\nroot")
		l.Printf("Raw message: %s", "raw\r\ndata")
		l.Printf("Received unhandled event types: %v", []string{"unknown"})
		l.Printf("Future diagnostic: %s", "new\nmessage")
	})
	for _, want := range []string{`malformed\r\nXML`, `unknown\nroot`, `raw\r\ndata`, "[unknown]", `new\nmessage`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing diagnostic %q in %q", want, out)
		}
	}
	if strings.Count(out, "\n") != 5 || strings.Count(out, `device\nID`) != 5 {
		t.Fatalf("diagnostics were not sanitized: %q", out)
	}
}

// Log output and snapshots are read concurrently with real client callbacks.
type speakerLogBuffer struct {
	mu sync.Mutex
	bytes.Buffer
}

func (b *speakerLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.Write(p)
}

func (b *speakerLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.Buffer.String()
}

func TestSpeakerWebSocketSupervisorQuietReconnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.3:8080")
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Fatal(err)
		}
		t.Skipf("mock firmware event port unavailable: %v", err)
	}
	var output speakerLogBuffer
	previousOutput := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previousOutput) })
	var available atomic.Bool
	var attempts atomic.Int32
	connected := make(chan *websocket.Conn, 4)
	upgrader := websocket.Upgrader{Subprotocols: []string{"gabbo"}}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		attempts.Add(1)
		if !available.Load() {
			http.Error(w, "speaker unavailable", http.StatusServiceUnavailable)
			return
		}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		connected <- ws
		for {
			if _, _, err := ws.ReadMessage(); err != nil {
				return
			}
		}
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	app := NewWebApp()
	cleanupRegistry(t, app)
	conn := webtypes.NewDeviceConnection(client.NewClientFromHost("127.0.0.3:8080"), nil)
	app.AddDevice("speaker", conn)
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		app.ConnectDeviceWebSocket("speaker", conn)
	}()
	t.Cleanup(func() {
		conn.Close()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("supervisor did not stop after removal")
		}
	})
	wait := func(condition func() bool, timeout time.Duration) {
		t.Helper()
		deadline := time.Now().Add(timeout)
		for !condition() {
			if time.Now().After(deadline) {
				t.Fatalf("condition timed out; attempts=%d logs:\n%s", attempts.Load(), output.String())
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	checkCounts := func(outages, recoveries int) {
		t.Helper()
		out := output.String()
		if strings.Count(out, "WebSocket unavailable for device speaker") != outages || strings.Count(out, "WebSocket recovered for device speaker") != recoveries {
			t.Fatalf("want %d outages, %d recoveries; got:\n%s", outages, recoveries, out)
		}
	}
	wait(func() bool { return attempts.Load() >= 3 }, 5*time.Second)
	checkCounts(1, 0)
	available.Store(true)
	wait(func() bool { return strings.Count(output.String(), "WebSocket recovered") == 1 }, 6*time.Second)
	ws := <-connected
	checkCounts(1, 1)
	wait(func() bool { return conn.Status().RecentsRevision == 1 }, time.Second)
	for _, frame := range []string{
		`<updates deviceID="DEVICE"><`,
		"unknown\r\nRAW",
		`<updates deviceID="DEVICE"><unmodeledEvent/></updates>`,
		`<errorUpdate deviceID="DEVICE"><error value="7" name="AUDIO_ERROR" severity="Error">speaker&#10;failure</error></errorUpdate>`,
	} {
		if err := ws.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
			t.Fatal(err)
		}
	}
	wait(func() bool { return strings.Contains(output.String(), `speaker\nfailure`) }, time.Second)
	for _, want := range []string{"Failed to parse WebSocket message", "Unknown special message type", `unknown\r\nRAW`, "Received unhandled event types: [unmodeledEvent]", "reported error AUDIO_ERROR"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing diagnostic %q in %q", want, output.String())
		}
	}
	available.Store(false)
	_ = ws.Close()
	wait(func() bool { return strings.Count(output.String(), "WebSocket unavailable") == 2 }, time.Second)
	// Let the production client's fixed 5s reconnect interval fail twice.
	wait(func() bool { return attempts.Load() >= 6 }, 12*time.Second)
	checkCounts(2, 1)
	available.Store(true)
	wait(func() bool { return strings.Count(output.String(), "WebSocket recovered") == 2 }, 7*time.Second)
	ws = <-connected
	wait(func() bool { return conn.Status().RecentsRevision == 2 }, time.Second)
	checkCounts(2, 2)
	available.Store(false)
	_ = ws.Close()
	wait(func() bool { return strings.Count(output.String(), "WebSocket unavailable") == 3 }, time.Second)
	if !app.RemoveDevice("speaker") {
		t.Fatal("removal failed")
	}
	<-finished
	checkCounts(3, 2)
	for _, unwanted := range []string{"[WebSocket] device speaker: Connecting", "Reconnection attempt", "Reconnected successfully", "WebSocket read error", "Failed to send ping", "Max reconnection attempts", "WebSocket connected for device speaker"} {
		if strings.Contains(output.String(), unwanted) {
			t.Errorf("routine reconnect chatter survived: %q", unwanted)
		}
	}
}

func TestSpeakerWebSocketSupervisorRemovalDuringInitialRetry(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.4:8080")
	if err != nil {
		if runtime.GOOS == "linux" {
			t.Fatal(err)
		}
		t.Skipf("mock firmware event port unavailable: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "speaker unavailable", http.StatusServiceUnavailable)
	}))
	_ = server.Listener.Close()
	server.Listener = listener
	server.Start()
	defer server.Close()
	var output speakerLogBuffer
	previousOutput := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(previousOutput)
	conn := webtypes.NewDeviceConnection(client.NewClientFromHost("127.0.0.4:8080"), nil)
	defer conn.Close()
	app := NewWebApp()
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		app.ConnectDeviceWebSocket("removed", conn)
	}()
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(output.String(), "WebSocket unavailable for device removed") {
		if time.Now().After(deadline) {
			conn.Close()
			t.Fatal("initial failure did not reach the retry wait")
		}
		time.Sleep(time.Millisecond)
	}
	conn.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		conn.Close()
		t.Fatal("removal did not cancel initial retry")
	}
	out := output.String()
	if strings.Count(out, "WebSocket unavailable") != 1 || strings.Contains(out, "WebSocket recovered") {
		t.Fatalf("removal generated an extra outage/recovery: %q", out)
	}
}
