package client

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gorilla/websocket"
)

// The speaker's WebSocket is not only an event stream. It is a full
// request/response channel using the same <msg> envelope the Bose app sends,
// and some endpoints are reachable ONLY this way — /balance is the known case:
// POST /balance over HTTP hangs rather than refusing, while the app Bose ships
// on the speaker itself writes balance exclusively over the socket.
//
// Correlation rules, all confirmed against three independent captures of the
// official app and a hardware run (FW 27.0.6):
//
//   - Every response is enveloped, echoes the request's requestID, and carries
//     msgType="RESPONSE". Measured at 25-55 ms on an ST-10.
//   - The url attribute alone is NOT sufficient to identify a response; a
//     client may have several requests outstanding on the same url.
//   - Pushed frames (<updates>, errorUpdate, SoundTouchSdkInfo, …) share the
//     socket and must be routed to the event handlers, not mistaken for a reply.
//
// The read loop already runs in its own goroutine with a liveness deadline
// extended by pongs, so nothing here sets a read deadline. Using one as a
// per-request timeout would be a bug: gorilla/websocket treats any read error
// as fatal, a deadline expiry included, and returns it instantly from every
// later read — one quiet moment would poison the connection and every
// subsequent request would fail in a way indistinguishable from a silent peer.
// The timeout below therefore lives in a select, not on the socket.

const (
	// msgTypeResponse is the msgType a speaker sets on a reply.
	msgTypeResponse = "RESPONSE"

	// defaultRequestTimeout caps a single WebSocket request.
	defaultRequestTimeout = 10 * time.Second
)

// pendingRequests correlates in-flight WebSocket requests with their replies.
type pendingRequests struct {
	mu      sync.Mutex
	nextID  atomic.Int64
	waiting map[string]chan []byte
}

func newPendingRequests() *pendingRequests {
	return &pendingRequests{waiting: make(map[string]chan []byte)}
}

// register reserves a requestID and the channel its reply will arrive on.
func (p *pendingRequests) register() (string, chan []byte) {
	id := strconv.FormatInt(p.nextID.Add(1), 10)
	// Buffered so deliver never blocks on a caller that has already given up.
	ch := make(chan []byte, 1)

	p.mu.Lock()
	p.waiting[id] = ch
	p.mu.Unlock()

	return id, ch
}

func (p *pendingRequests) release(id string) {
	p.mu.Lock()
	delete(p.waiting, id)
	p.mu.Unlock()
}

// deliver hands a reply to whoever is waiting for it, reporting whether the
// frame was claimed.
func (p *pendingRequests) deliver(id string, data []byte) bool {
	p.mu.Lock()
	ch, ok := p.waiting[id]

	if ok {
		delete(p.waiting, id)
	}

	p.mu.Unlock()

	if !ok {
		return false
	}

	ch <- data

	return true
}

// failAll wakes every waiter when the transport dies, so a request in flight
// reports the disconnect instead of running out its whole timeout.
func (p *pendingRequests) failAll() {
	p.mu.Lock()
	waiting := p.waiting
	p.waiting = make(map[string]chan []byte)
	p.mu.Unlock()

	for _, ch := range waiting {
		close(ch)
	}
}

// wsEnvelope is a parsed <msg> frame.
type wsEnvelope struct {
	XMLName xml.Name `xml:"msg"`
	Header  struct {
		DeviceID string `xml:"deviceID,attr"`
		URL      string `xml:"url,attr"`
		Method   string `xml:"method,attr"`
		Request  struct {
			RequestID string `xml:"requestID,attr"`
			MsgType   string `xml:"msgType,attr"`
		} `xml:"request"`
	} `xml:"header"`
	Body struct {
		Inner  []byte         `xml:",innerxml"`
		Error  *models.Error  `xml:"error"`
		Errors []models.Error `xml:"errors>error"`
	} `xml:"body"`
}

// routeResponse claims a frame if it is a reply to an outstanding request.
// It returns true when the frame was consumed, so the caller skips event
// dispatch for it.
func (ws *WebSocketClient) routeResponse(data []byte) bool {
	// Cheap reject before parsing: pushed frames are the common case.
	root, err := models.RootElementName(data)
	if err != nil || root != "msg" {
		return false
	}

	var envelope wsEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return false
	}

	req := envelope.Header.Request
	if req.RequestID == "" || (req.MsgType != "" && req.MsgType != msgTypeResponse) {
		return false
	}

	return ws.pending.deliver(req.RequestID, data)
}

// RequestOptions tunes a single WebSocket request.
type RequestOptions struct {
	// MainNode goes into <info mainNode="…"> on the request. Some endpoints
	// require it — a balance write is "balanceSet" — and the speaker ignores
	// it otherwise.
	MainNode string
	// Timeout caps the wait for the reply. Zero uses defaultRequestTimeout.
	Timeout time.Duration
}

// SendFence runs send while the caller's target identity is still valid.
// Implementations may hold application registry and topology read locks for
// the duration of send; send returns after the frame write and before Request
// waits for the correlated reply.
type SendFence func(send func() error) error

// Request sends a <msg> envelope and returns the body of the matching
// response.
//
// route is the endpoint without a leading slash ("balance", "info"). body is
// raw XML placed inside <body>, and may be empty. The returned bytes are the
// response's inner body XML, ready to unmarshal.
//
// A device-reported error in the reply becomes a Go error naming the code and
// severity rather than being returned as a payload.
func (ws *WebSocketClient) Request(ctx context.Context, route, method, body string, opts RequestOptions) ([]byte, error) {
	return ws.requestForTarget(ctx, route, method, body, opts, "", nil)
}

// requestForTarget is Request with an optional physical-device check and a
// fence around the single frame write. The device identity is resolved before
// queueing for the socket writer, while the transport itself is selected only
// after the queue wait and inside sendFence.
func (ws *WebSocketClient) requestForTarget(
	ctx context.Context,
	route, method, body string,
	opts RequestOptions,
	expectedDeviceID string,
	sendFence SendFence,
) ([]byte, error) {
	var admittedTransport *webSocketConnection

	if sendFence != nil {
		ws.mu.RLock()
		admittedTransport = ws.connection
		ws.mu.RUnlock()
	}

	deviceID, err := ws.deviceIDForEnvelope()
	if err != nil {
		return nil, err
	}

	expectedDeviceID = strings.TrimSpace(expectedDeviceID)
	if expectedDeviceID != "" && strings.TrimSpace(deviceID) != expectedDeviceID {
		return nil, fmt.Errorf(
			"websocket request: resolved device ID %q does not match target %q",
			deviceID,
			expectedDeviceID,
		)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultRequestTimeout
	}

	id, replies := ws.pending.register()
	defer ws.pending.release(id)

	info := `<info type="new"/>`
	if opts.MainNode != "" {
		info = fmt.Sprintf(`<info mainNode="%s" type="new"/>`, xmlAttrEscape(opts.MainNode))
	}

	envelope := fmt.Sprintf(
		`<msg><header deviceID="%s" url="%s" method="%s"><request requestID="%s">%s</request></header><body>%s</body></msg>`,
		xmlAttrEscape(deviceID), xmlAttrEscape(route), xmlAttrEscape(method), id, info, body,
	)

	if err := ws.sendRequestMessage(ctx, []byte(envelope), sendFence, admittedTransport); err != nil {
		return nil, fmt.Errorf("send %s: %w", route, err)
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case data, open := <-replies:
		if !open {
			return nil, fmt.Errorf("await response for %s: connection lost", route)
		}

		return responseBody(route, data)
	case <-timer.C:
		return nil, fmt.Errorf("await response for %s: %w", route, context.DeadlineExceeded)
	case <-ctx.Done():
		return nil, fmt.Errorf("await response for %s: %w", route, ctx.Err())
	}
}

// sendRequestMessage writes one request after waiting for all earlier socket
// writers. A guarded write acquires writeMu before entering the application
// fence, then selects and holds the current transport through the frame write.
func (ws *WebSocketClient) sendRequestMessage(
	ctx context.Context, message []byte, sendFence SendFence, admittedTransport *webSocketConnection,
) error {
	if sendFence == nil {
		return ws.SendMessage(message)
	}

	ws.writeMu.Lock()
	defer ws.writeMu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}

	called := false
	write := func() error {
		if called {
			return fmt.Errorf("send fence invoked the frame write more than once")
		}

		called = true

		if err := ctx.Err(); err != nil {
			return err
		}

		ws.mu.RLock()
		defer ws.mu.RUnlock()

		if err := ctx.Err(); err != nil {
			return err
		}

		connection := ws.connection
		if !ws.connected || connection == nil || connection != admittedTransport || connection.ctx.Err() != nil {
			return fmt.Errorf("not connected")
		}

		deadline := time.Now().Add(10 * time.Second)
		if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
			deadline = contextDeadline
		}

		if err := connection.conn.SetWriteDeadline(deadline); err != nil {
			return err
		}

		return connection.conn.WriteMessage(websocket.TextMessage, message)
	}

	if err := sendFence(write); err != nil {
		return err
	}

	if !called {
		return fmt.Errorf("send fence rejected the frame write")
	}

	return nil
}

// responseBody extracts the body of a reply, turning a device error into a Go
// error.
func responseBody(route string, data []byte) ([]byte, error) {
	var envelope wsEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("parse response for %s: %w", route, err)
	}

	if devErr := firstDeviceError(envelope.Body.Error, envelope.Body.Errors); devErr != nil {
		return nil, fmt.Errorf("device rejected %s: %s", route, describeDeviceError(devErr))
	}

	return envelope.Body.Inner, nil
}

func firstDeviceError(single *models.Error, wrapped []models.Error) *models.Error {
	if single != nil {
		return single
	}

	if len(wrapped) > 0 {
		return &wrapped[0]
	}

	return nil
}

// describeDeviceError renders a device error for an operator: code, symbolic
// name, severity and detail, rather than a raw XML dump.
func describeDeviceError(devErr *models.Error) string {
	parts := make([]string, 0, 4)

	if devErr.Name != "" {
		parts = append(parts, devErr.Name)
	}

	if devErr.Value != "" {
		parts = append(parts, "code "+devErr.Value)
	}

	if devErr.Severity != "" {
		parts = append(parts, "severity "+devErr.Severity)
	}

	if text := strings.TrimSpace(devErr.Text); text != "" {
		parts = append(parts, text)
	}

	if len(parts) == 0 {
		return "unspecified error"
	}

	return sanitizeLog(strings.Join(parts, ", "))
}

// deviceIDForEnvelope returns the deviceID every envelope header must carry.
func (ws *WebSocketClient) deviceIDForEnvelope() (string, error) {
	if ws.client == nil {
		return "", fmt.Errorf("websocket request: no client to resolve the device ID from")
	}

	info, err := ws.client.GetDeviceInfo()
	if err != nil {
		return "", fmt.Errorf("websocket request: resolve device ID: %w", err)
	}

	if info.DeviceID == "" {
		return "", fmt.Errorf("websocket request: device reported an empty device ID")
	}

	return info.DeviceID, nil
}

// xmlAttrEscape escapes the characters that would break an XML attribute.
// Envelopes are built by concatenation because the body fragments are already
// valid XML — running them through encoding/xml would re-escape nested tags.
func xmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")

	return s
}
