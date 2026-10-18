package soundtouchweb

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRegisteredSpeakerReceivesEventsBeforeAnyControl(t *testing.T) {
	// The production client uses the firmware's fixed event port. A distinct
	// loopback address keeps this mock separate from local localhost services.
	listener, err := net.Listen("tcp", "127.0.0.2:8080")
	if err != nil {
		t.Skipf("mock SoundTouch event endpoint unavailable: %v", err)
	}
	var connections atomic.Int32
	var writes atomic.Int32
	var reportedVolume atomic.Int32
	reportedVolume.Store(10)
	connected := make(chan *websocket.Conn, 2)
	closed := make(chan struct{}, 2)
	upgrader := websocket.Upgrader{Subprotocols: []string{"gabbo"}}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes.Add(1)
		}
		switch r.URL.Path {
		case "/info":
			_, _ = w.Write([]byte(`<info deviceID="EVENTDEVICE"><name>Event speaker</name><type>SoundTouch 20</type></info>`))
		case "/volume":
			value := reportedVolume.Load()
			_, _ = fmt.Fprintf(w, `<volume deviceID="EVENTDEVICE"><targetvolume>%d</targetvolume><actualvolume>%d</actualvolume><muteenabled>false</muteenabled></volume>`, value, value)
		case "/":
			ws, upgradeErr := upgrader.Upgrade(w, r, nil)
			if upgradeErr != nil {
				return
			}
			connections.Add(1)
			connected <- ws
			defer ws.Close()
			defer func() { closed <- struct{}{} }()
			for {
				if _, _, readErr := ws.ReadMessage(); readErr != nil {
					return
				}
			}
		default:
			http.NotFound(w, r)
		}
	}))
	if err := server.Listener.Close(); err != nil {
		t.Fatal(err)
	}
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	app := NewWebApp()
	cleanupRegistry(t, app)
	conn := app.addDeviceByHost(context.Background(), "127.0.0.2", 8080, "manual")
	if conn == nil {
		t.Fatal("speaker registration failed")
	}
	var ws *websocket.Conn
	select {
	case ws = <-connected:
	case <-time.After(time.Second):
		t.Fatal("registration did not start event subscription before any control or browser")
	}
	// An external player changes volume; no HTTP write or fallback poll is
	// needed to get that observation into the registered speaker's cache.
	// HTTP readback stays at 10 so it cannot satisfy the event assertion.
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`<updates deviceID="EVENTDEVICE"><volumeUpdated deviceID="EVENTDEVICE"><volume deviceID="EVENTDEVICE"><targetvolume>27</targetvolume><actualvolume>27</actualvolume><muteenabled>false</muteenabled></volume></volumeUpdated></updates>`)); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for conn.Status().Volume == nil || conn.Status().Volume.ActualVolume != 27 {
		if time.Now().After(deadline) {
			t.Fatal("speaker event was not received without a control request")
		}
		time.Sleep(time.Millisecond)
	}
	app.AddDeviceByHost("127.0.0.2", 8080, "manual")
	if connections.Load() != 1 || writes.Load() != 0 {
		t.Fatalf("connections=%d writes=%d; want one subscription and zero writes", connections.Load(), writes.Load())
	}
	if !app.RemoveDevice("127.0.0.2") {
		t.Fatal("speaker removal failed")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("removal did not close the speaker event subscription")
	}
}
