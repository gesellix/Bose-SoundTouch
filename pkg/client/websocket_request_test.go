package client

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeSpeakerWS is an httptest server that answers WebSocket <msg> requests
// the way a real speaker does — enveloped, echoing the requestID, marked
// msgType="RESPONSE" — and also serves the /info the envelope builder needs to
// learn the device ID.
type fakeSpeakerWS struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []string

	// reply builds the frames sent in response to one request. Returning nil
	// means "stay silent", which is how timeouts are exercised.
	reply func(route, requestID, frame string) []string
}

const fakeSpeakerDeviceID = "DEVICEID01"

func newFakeSpeakerWS(t *testing.T) *fakeSpeakerWS {
	t.Helper()

	f := &fakeSpeakerWS{}
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"gabbo"},
		CheckOrigin:  func(*http.Request) bool { return true },
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<info deviceID="%s"><name>Fake</name><type>SoundTouch 10</type></info>`, fakeSpeakerDeviceID)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}

		defer func() { _ = conn.Close() }()

		for {
			_, data, readErr := conn.ReadMessage()
			if readErr != nil {
				return
			}

			frame := string(data)

			f.mu.Lock()
			f.requests = append(f.requests, frame)
			policy := f.reply
			f.mu.Unlock()

			route := extractAttrValue(frame, `url="`)
			id := extractAttrValue(frame, `requestID="`)

			var replies []string
			if policy != nil {
				replies = policy(route, id, frame)
			} else {
				replies = []string{okResponse(route, id, "")}
			}

			for _, reply := range replies {
				if writeErr := conn.WriteMessage(websocket.TextMessage, []byte(reply)); writeErr != nil {
					return
				}
			}
		}
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)

	return f
}

// okResponse builds a well-formed reply carrying body.
func okResponse(route, requestID, body string) string {
	return fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8" ?>`+
			`<msg><header deviceID="%s" url="%s" method="GET">`+
			`<request requestID="%s" msgType="RESPONSE"><info type="new" /></request>`+
			`</header><body>%s</body></msg>`,
		fakeSpeakerDeviceID, route, requestID, body)
}

func extractAttrValue(s, prefix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}

	rest := s[i+len(prefix):]

	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}

	return rest[:j]
}

func (f *fakeSpeakerWS) recordedRequests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, len(f.requests))
	copy(out, f.requests)

	return out
}

// connect wires a WebSocketClient to the fake through the injectable dialer,
// so the client's hardcoded :8080 does not have to be honoured.
func (f *fakeSpeakerWS) connect(t *testing.T) *WebSocketClient {
	t.Helper()

	host, port := splitHostPort(t, f.server.URL)
	soundTouchClient := NewClient(&Config{Host: host, Port: port, Timeout: 5 * time.Second})
	wsClient := soundTouchClient.NewWebSocketClient(&WebSocketConfig{Logger: &mockLogger{}})

	wsURL := "ws" + strings.TrimPrefix(f.server.URL, "http") + "/ws"
	wsClient.dialContext = func(ctx context.Context, _ string, header http.Header) (*websocket.Conn, *http.Response, error) {
		dialer := websocket.Dialer{Subprotocols: []string{"gabbo"}}
		return dialer.DialContext(ctx, wsURL, header)
	}

	if err := wsClient.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	t.Cleanup(func() { _ = wsClient.Disconnect() })

	return wsClient
}

// splitHostPort turns an httptest URL into the host/port pair Config wants.
func splitHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse %q: %v", rawURL, err)
	}

	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		t.Fatalf("port of %q: %v", rawURL, err)
	}

	return parsed.Hostname(), port
}

func TestWebSocketRequestCorrelatesByRequestID(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{
			// A pushed event and an unrelated response arrive first; neither
			// may be mistaken for our reply.
			`<updates deviceID="DEVICEID01"><volumeUpdated><volume><actualvolume>3</actualvolume></volume></volumeUpdated></updates>`,
			okResponse(route, "9999", `<wrong/>`),
			okResponse(route, id, `<right/>`),
		}
	}

	wsClient := f.connect(t)

	body, err := wsClient.Request(context.Background(), "info", "GET", "", RequestOptions{})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if !strings.Contains(string(body), "<right/>") {
		t.Errorf("body = %q, want the reply matching our requestID", body)
	}
}

// TestWebSocketRequestIgnoresNonResponseEnvelope pins the msgType guard: a
// frame echoing our requestID but not marked as a response is not an answer.
func TestWebSocketRequestIgnoresNonResponseEnvelope(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{fmt.Sprintf(
			`<msg><header deviceID="%s" url="%s" method="GET">`+
				`<request requestID="%s" msgType="REQUEST"/></header><body/></msg>`,
			fakeSpeakerDeviceID, route, id)}
	}

	wsClient := f.connect(t)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if _, err := wsClient.Request(ctx, "info", "GET", "", RequestOptions{Timeout: 300 * time.Millisecond}); err == nil {
		t.Fatal("a non-RESPONSE envelope must not satisfy a request")
	}
}

func TestWebSocketRequestSurfacesDeviceError(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(route, id, _ string) []string {
		return []string{okResponse(route, id,
			`<error value="1005" name="UNKNOWN_SOURCE_ERROR" severity="Unrecoverable">nope</error>`)}
	}

	wsClient := f.connect(t)

	_, err := wsClient.Request(context.Background(), "select", "POST", "", RequestOptions{})
	if err == nil {
		t.Fatal("expected the device error to become a Go error")
	}

	for _, want := range []string{"device rejected select", "UNKNOWN_SOURCE_ERROR", "code 1005"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
}

// TestWebSocketRequestTimeoutLeavesConnectionUsable is the guard against the
// read-deadline trap: a timed-out request must not poison the socket, because
// gorilla/websocket makes any read error fatal for the connection.
func TestWebSocketRequestTimeoutLeavesConnectionUsable(t *testing.T) {
	f := newFakeSpeakerWS(t)

	var calls int

	f.reply = func(route, id, _ string) []string {
		calls++
		if calls == 1 {
			return nil // silence: this one must time out
		}

		return []string{okResponse(route, id, `<recovered/>`)}
	}

	wsClient := f.connect(t)

	if _, err := wsClient.Request(context.Background(), "info", "GET", "", RequestOptions{Timeout: 200 * time.Millisecond}); err == nil {
		t.Fatal("expected the first request to time out")
	}

	body, err := wsClient.Request(context.Background(), "info", "GET", "", RequestOptions{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("a request after a timeout must still work, got %v", err)
	}

	if !strings.Contains(string(body), "<recovered/>") {
		t.Errorf("body = %q, want the second reply", body)
	}
}

// TestWebSocketRequestSendsCanonicalEnvelope pins the wire shape, including
// the mainNode a balance write needs.
func TestWebSocketRequestSendsCanonicalEnvelope(t *testing.T) {
	f := newFakeSpeakerWS(t)
	wsClient := f.connect(t)

	_, err := wsClient.Request(context.Background(), "balance", "POST",
		`<balance><targetBalance>-3</targetBalance></balance>`,
		RequestOptions{MainNode: "balanceSet"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	requests := f.recordedRequests()
	if len(requests) != 1 {
		t.Fatalf("want 1 request, got %d: %v", len(requests), requests)
	}

	for _, want := range []string{
		`deviceID="DEVICEID01"`,
		`url="balance"`,
		`method="POST"`,
		`<info mainNode="balanceSet" type="new"/>`,
		`<balance><targetBalance>-3</targetBalance></balance>`,
	} {
		if !strings.Contains(requests[0], want) {
			t.Errorf("request missing %q in: %s", want, requests[0])
		}
	}
}

// TestWebSocketRequestFailsFastOnDisconnect pins that a waiter is woken when
// the transport dies, rather than sitting out its whole timeout.
func TestWebSocketRequestFailsFastOnDisconnect(t *testing.T) {
	f := newFakeSpeakerWS(t)
	f.reply = func(string, string, string) []string { return nil }

	wsClient := f.connect(t)

	go func() {
		time.Sleep(100 * time.Millisecond)

		_ = wsClient.Disconnect()
	}()

	start := time.Now()

	_, err := wsClient.Request(context.Background(), "info", "GET", "", RequestOptions{Timeout: 30 * time.Second})
	if err == nil {
		t.Fatal("expected an error once the connection went away")
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("waited %s; the request should fail as soon as the transport drops", elapsed)
	}
}
