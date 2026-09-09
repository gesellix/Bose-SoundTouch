package setup

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gorilla/websocket"
)

const (
	defaultSetupStepTimeout = 8 * time.Second
	setupHandshakeTimeout   = 10 * time.Second

	// LanguageEnglish is the sysLanguage code used by Stockholm and the
	// speaker firmware for English.
	LanguageEnglish = int(models.LanguageEnglish)

	// DefaultMargeAuthToken is the placeholder userAuthToken sent in
	// <PairDeviceWithAccount> when the caller didn't supply one. The
	// speaker accepts any non-empty value; a real Bose-issued token
	// shape (128-char base64 per docs/reference/DEVICE-PAIRING-FLOW.md
	// line 154) is not required — verified during #195 investigation
	// where the speaker happily persisted "Bearer AfterTouch" and
	// re-derived its post-pair state from the marge endpoints
	// regardless of token content.
	DefaultMargeAuthToken = "Bearer AfterTouch"

	// DefaultMargePairingEmail is the synthetic accountEmail used when
	// PairingExtras requests the extended <PairDeviceWithAccount> payload
	// but doesn't supply an email. RFC 2606 reserves ".invalid" as a TLD
	// guaranteed never to resolve, which is what we want here — the
	// speaker writes it into its persistent state but no real address
	// receives anything.
	DefaultMargePairingEmail = "local@aftertouch.invalid"
)

// MargePairingExtras carries the optional fields that the official Bose
// Android app and Zimbo88's USB-less OpenCloudTouch script include in
// their <PairDeviceWithAccount> payloads. AfterTouch historically sent
// only <accountId> + <userAuthToken>; that minimal shape is the
// suspected trigger for the post-pair AUX/preset breakage tracked in
// issues #195 and #269.
//
// Set BoseServer (and optionally UpdateServer/AccountEmail) on the
// SessionConfig to opt into the richer payload. Empty fields are
// omitted from the XML so callers can choose any subset.
//
// Reference: docs/reference/DEVICE-PAIRING-FLOW.md and
// https://github.com/scheilch/opencloudtouch/discussions/201.
type MargePairingExtras struct {
	// BoseServer is the marge server URL the speaker should use after
	// pairing. Typically equal to AfterTouch's service URL.
	BoseServer string
	// UpdateServer is the firmware-update server URL. If empty and
	// BoseServer is set, SetMargeAccount derives it as
	// BoseServer + "/updates/soundtouch".
	UpdateServer string
	// AccountEmail is the synthetic email persisted alongside the
	// account. If empty and BoseServer is set, SetMargeAccount fills
	// in DefaultMargePairingEmail.
	AccountEmail string
}

// StateMachine is the surface the InitPlan orchestrator drives. The
// concrete WebSocket-backed implementation is *Session; tests inject
// an in-memory fake via Manager.NewSession.
type StateMachine interface {
	Start(ctx context.Context) error
	IdentifyEnter(ctx context.Context, timeoutMs int) error
	SetLanguage(ctx context.Context, code int) error
	Enter(ctx context.Context) error
	IdentifyLeave(ctx context.Context) error
	SetName(ctx context.Context, name string) error
	SetMargeAccount(ctx context.Context, accountID, authToken string) error
	Leave(ctx context.Context) error
	PushCustomerSupportInfo(ctx context.Context) error
	Close() error
}

// SessionConfig configures DialSession. Zero values pick safe
// defaults; in production callers normally pass an empty struct.
type SessionConfig struct {
	// StepTimeout caps the per-message wait for an ack frame. Default 8 s.
	StepTimeout time.Duration
	// DialTimeout caps the WebSocket handshake. Default 10 s.
	DialTimeout time.Duration
	// WSScheme overrides "ws". Tests inject "ws" with httptest's host:port
	// already encoded in deviceIP and rely on the dialer to use the URL
	// as-is.
	WSScheme string
	// WSPort overrides 8080 when deviceIP does not already carry a port.
	WSPort int
	// PairingExtras opts the session into the richer
	// <PairDeviceWithAccount> payload (boseServer / updateServer /
	// accountEmail) used by the official Bose Android app. Zero value
	// retains the historical minimal payload.
	PairingExtras MargePairingExtras
}

// Session is a synchronous request/response WebSocket session driving
// the speaker's setup state machine. It is deliberately separate from
// pkg/client.WebSocketClient (which is event-oriented, auto-reconnecting,
// and stateful) — setup is a short, linear sequence and benefits from a
// purpose-built transport.
type Session struct {
	deviceID      string
	conn          *websocket.Conn
	reqID         atomic.Int64
	stepTimeout   time.Duration
	pairingExtras MargePairingExtras

	// frames carries every received frame from the reader goroutine to
	// whichever sendStep is waiting. It is closed when the read loop ends.
	frames chan []byte
	// done is closed by Close to unblock a reader parked on a send.
	done chan struct{}
	// readErr holds the error that ended the read loop, so a step waiting
	// on a dead connection reports the cause rather than "closed channel".
	readErr atomic.Pointer[error]
	// logf receives non-fatal observations (e.g. a device error pushed
	// while a step is in flight). Tests override it.
	logf func(format string, args ...any)
	// closeOnce keeps Close idempotent: closing done twice panics.
	closeOnce sync.Once
	// routeUses counts how many times each route has been sent on this
	// session, which is what makes a route ambiguous. See classifyEnvelope.
	routeUses map[string]int
}

// DialSession opens a WebSocket to the speaker at deviceIP and
// returns a session ready to drive the SETUP state machine. deviceID is
// required because every <msg> envelope embeds it in the header; obtain
// it from /info before calling.
func DialSession(deviceIP, deviceID string, cfg SessionConfig) (*Session, error) {
	if deviceID == "" {
		return nil, errors.New("DialSession: deviceID is required for message routing")
	}

	scheme := cfg.WSScheme
	if scheme == "" {
		scheme = "ws"
	}

	host := deviceIP

	if _, _, err := net.SplitHostPort(deviceIP); err != nil {
		port := cfg.WSPort
		if port == 0 {
			port = 8080
		}

		host = fmt.Sprintf("%s:%d", deviceIP, port)
	}

	wsURL := url.URL{Scheme: scheme, Host: host, Path: "/"}

	handshake := cfg.DialTimeout
	if handshake == 0 {
		handshake = setupHandshakeTimeout
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: handshake,
		Subprotocols:     []string{"gabbo"},
	}

	conn, resp, err := dialer.Dial(wsURL.String(), nil)
	if resp != nil && resp.Body != nil {
		defer func() { _ = resp.Body.Close() }()
	}

	if err != nil {
		return nil, fmt.Errorf("websocket dial %s: %w", wsURL.String(), err)
	}

	step := cfg.StepTimeout
	if step == 0 {
		step = defaultSetupStepTimeout
	}

	s := &Session{
		deviceID:      deviceID,
		conn:          conn,
		stepTimeout:   step,
		pairingExtras: cfg.PairingExtras,
		frames:        make(chan []byte),
		done:          make(chan struct{}),
		logf:          log.Printf,
		routeUses:     make(map[string]int),
	}

	go s.readLoop(conn)

	return s, nil
}

// readLoop pumps received frames to sendStep.
//
// It deliberately sets no read deadline. gorilla/websocket treats every read
// error as fatal — a SetReadDeadline expiry included — and returns it
// instantly from every later read on the same connection. Using a deadline as
// a per-step timeout therefore poisons the whole session at the first quiet
// moment, and every subsequent step fails immediately with "i/o timeout",
// which reads exactly like the device refusing. sendStep gets its timeout from
// a select instead (GH-704).
//
// conn is passed in rather than read from s: Close nils s.conn.
func (s *Session) readLoop(conn *websocket.Conn) {
	defer close(s.frames)

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			s.readErr.Store(&err)

			return
		}

		select {
		case s.frames <- data:
		case <-s.done:
			return
		}
	}
}

// Close sends a normal-closure frame and closes the underlying socket.
func (s *Session) Close() error {
	if s.conn == nil {
		return nil
	}

	s.closeOnce.Do(func() { close(s.done) })

	_ = s.conn.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
		time.Now().Add(time.Second),
	)

	err := s.conn.Close()
	s.conn = nil

	return err
}

// sendStep wraps body in the canonical <msg><header url="…" method="…">…
// envelope, sends it, and waits for the matching ack.
//
// Frames are classified by their ROOT ELEMENT, never by substring. That
// matters: the speaker pushes root-level <errorUpdate> frames for unrelated
// asynchronous failures (source timeouts, audio-path errors), and the previous
// "does the text contain <error" test flagged those as a rejection of the step
// in flight — aborting the whole init plan over an event that had nothing to do
// with it, because "<errorupdate" contains "<error" (GH-704).
//
// Correlation is exact for enveloped responses: a <msg> reply is this step's
// ack only if it echoes this step's requestID. Five of the nine init-plan
// steps use url="setup", so accepting a bare url match — as this used to —
// let a late response to step N satisfy step N+1.
//
// The ack payload is consumed for error detection only and never returned —
// every caller discards it.
func (s *Session) sendStep(ctx context.Context, route, method, body string) error {
	if s.conn == nil {
		return errors.New("setup session: connection closed")
	}

	id := s.reqID.Add(1)
	s.routeUses[route]++

	envelope := fmt.Sprintf(
		`<msg><header deviceID="%s" url="%s" method="%s"><request requestID="%d"/></header><body>%s</body></msg>`,
		xmlAttrEscape(s.deviceID), xmlAttrEscape(route), method, id, body,
	)

	// A context deadline, when present, wins over stepTimeout. Callers rely
	// on this: cmd_setup.go's bare-pair path deliberately grants
	// step-timeout+2s, and ExecuteInitPlan passes one context to all nine
	// steps as a whole-plan budget.
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(s.stepTimeout)
	}

	_ = s.conn.SetWriteDeadline(deadline)

	if err := s.conn.WriteMessage(websocket.TextMessage, []byte(envelope)); err != nil {
		return fmt.Errorf("send %s: %w", route, err)
	}

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	for {
		var data []byte

		select {
		case frame, open := <-s.frames:
			if !open {
				return fmt.Errorf("await ack for %s: %w", route, s.readLoopErr())
			}

			data = frame
		case <-timer.C:
			// The connection stays usable: no read deadline was ever set,
			// so nothing has been poisoned and a later step can still run.
			return fmt.Errorf("await ack for %s: %w", route, context.DeadlineExceeded)
		case <-ctx.Done():
			return fmt.Errorf("await ack for %s: %w", route, ctx.Err())
		}

		done, err := s.classifyFrame(data, route, id)
		if err != nil {
			return err
		}

		if done {
			return nil
		}
	}
}

// readLoopErr reports why the read loop ended, or a generic closure error if
// it ended without one.
func (s *Session) readLoopErr() error {
	if err := s.readErr.Load(); err != nil && *err != nil {
		return *err
	}

	return errors.New("connection closed")
}

// classifyFrame decides what a received frame means for the step identified by
// route and id. It returns (true, nil) for this step's ack, (false, nil) for a
// frame to skip, and a non-nil error when the device rejected the step.
func (s *Session) classifyFrame(data []byte, route string, id int64) (bool, error) {
	root, err := models.RootElementName(data)
	if err != nil {
		// Undecodable frames are not acks. Skipping keeps the step waiting
		// for a real answer rather than failing on a truncated push.
		s.logf("setup session: skipping undecodable frame during %s: %v", route, sanitizeLog(err.Error()))

		return false, nil
	}

	switch root {
	case "msg":
		return s.classifyEnvelope(data, route, id)

	case "status":
		// A bare <status>/route</status> reply. These carry no requestID at
		// all, so they can only be correlated by arrival order — acceptable
		// because a session runs one step at a time.
		var status struct {
			Value string `xml:",chardata"`
		}

		if err := xml.Unmarshal(data, &status); err != nil {
			// Not our ack, but not a rejection either: keep waiting for a
			// frame we can actually read.
			s.logf("setup session: skipping undecodable status frame during %s: %v", route, sanitizeLog(err.Error()))

			return false, nil
		}

		return strings.TrimSpace(status.Value) == "/"+route, nil

	case "error", "errors":
		// A rejection sent unwrapped, rather than inside an envelope.
		return false, fmt.Errorf("device rejected %s: %s", route, sanitizeLog(strings.TrimSpace(string(data))))

	case "errorUpdate":
		// An asynchronous device error, NOT a reply to this step. Report it
		// — these name the failure precisely and are worth seeing — but keep
		// waiting for the actual ack.
		s.logDeviceError(data, route)

		return false, nil

	default:
		// Pushed frames: <updates>, <SoundTouchSdkInfo>, userActivityUpdate,
		// and anything the firmware invents later.
		return false, nil
	}
}

// classifyEnvelope handles a <msg> frame: this step's ack, another step's
// late response, or a rejection.
func (s *Session) classifyEnvelope(data []byte, route string, id int64) (bool, error) {
	var envelope struct {
		Header struct {
			URL string `xml:"url,attr"`
			// Real hardware answers with <request requestID="…"
			// msgType="RESPONSE">; the shape below also picks up a
			// <response requestID="…"/> child. Matching the attribute
			// wherever it appears avoids pinning either spelling.
			Request  *envelopeRequest `xml:"request"`
			Response *envelopeRequest `xml:"response"`
		} `xml:"header"`
		Body struct {
			Error  *models.Error  `xml:"error"`
			Errors []models.Error `xml:"errors>error"`
			Status string         `xml:"status"`
		} `xml:"body"`
	}

	if err := xml.Unmarshal(data, &envelope); err != nil {
		s.logf("setup session: skipping undecodable envelope during %s: %v", route, sanitizeLog(err.Error()))

		return false, nil
	}

	request := envelope.Header.Request
	if request == nil || request.RequestID == "" {
		request = envelope.Header.Response
	}

	requestID := ""
	if request != nil {
		requestID = request.RequestID
	}

	// A response that names a different requestID belongs to another step —
	// including its errors. Attributing those to the step in flight would
	// reintroduce, on the error path, exactly the cross-step misattribution
	// this function exists to prevent.
	if requestID != "" && requestID != strconv.FormatInt(id, 10) {
		return false, nil
	}

	// Rejections arrive either as a bare <error> or wrapped in <errors>
	// (models.ErrorsResponse's shape, which some firmware versions use).
	if devErr := firstError(&envelope.Body.Error, envelope.Body.Errors); devErr != nil {
		return false, fmt.Errorf("device rejected %s: %s", route, describeError(devErr))
	}

	if requestID != "" {
		// An empty msgType is accepted because not every reply shape carries
		// one; anything else that is not a RESPONSE is not an answer to us.
		return request.MsgType == "" || request.MsgType == msgTypeResponse, nil
	}

	// No requestID to correlate on. Fall back to the route, but only while
	// this session has used it once: five of the nine init-plan steps share
	// url="setup", and accepting a route match there is what let a late
	// response to step N satisfy step N+1 (GH-704). For a route used once,
	// a match is unambiguous and the fallback costs nothing.
	if s.routeUses[route] > 1 {
		return false, nil
	}

	return envelope.Header.URL == route ||
		strings.TrimSpace(envelope.Body.Status) == "/"+route, nil
}

// firstError picks the first device error out of either body shape.
func firstError(single **models.Error, wrapped []models.Error) *models.Error {
	if *single != nil {
		return *single
	}

	if len(wrapped) > 0 {
		return &wrapped[0]
	}

	return nil
}

// msgTypeResponse is the msgType attribute a speaker sets on a reply.
const msgTypeResponse = "RESPONSE"

// describeError renders a device error for an operator: code, symbolic name,
// severity and detail, rather than a raw XML dump.
func describeError(devErr *models.Error) string {
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

	// Speaker-supplied text reaches logs through this error; strip newlines
	// so a device cannot forge log lines (go/log-injection).
	return sanitizeLog(strings.Join(parts, ", "))
}

// envelopeRequest models the requestID-bearing child of a <msg> header,
// whichever tag name the firmware or a test fake uses for it.
type envelopeRequest struct {
	RequestID string `xml:"requestID,attr"`
	MsgType   string `xml:"msgType,attr"`
}

// logDeviceError surfaces an asynchronous <errorUpdate> pushed while a step is
// in flight, so it is not silently dropped.
func (s *Session) logDeviceError(data []byte, route string) {
	var update models.ErrorUpdate
	if err := xml.Unmarshal(data, &update); err != nil {
		s.logf("setup session: device error during %s (unparsed): %s", route, sanitizeLog(strings.TrimSpace(string(data))))

		return
	}

	s.logf("setup session: device error during %s: %s (%s) severity=%s: %s",
		route,
		sanitizeLog(update.Error.Name),
		sanitizeLog(update.Error.Value),
		sanitizeLog(update.Error.Severity),
		sanitizeLog(strings.TrimSpace(update.Error.Text)),
	)
}

// Start sends SETUP_START.
func (s *Session) Start(ctx context.Context) error {
	return s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_START"/>`)
}

// IdentifyEnter sends SETUP_IDENTIFY_DEVICE_ENTER. timeoutMs defaults to
// the value observed in captures (300 000 ms).
func (s *Session) IdentifyEnter(ctx context.Context, timeoutMs int) error {
	if timeoutMs <= 0 {
		timeoutMs = 300000
	}

	body := fmt.Sprintf(`<setupState state="SETUP_IDENTIFY_DEVICE_ENTER" timeout="%d"/>`, timeoutMs)

	return s.sendStep(ctx, "setup", "POST", body)
}

// SetLanguage POSTs a validated sysLanguage code.
func (s *Session) SetLanguage(ctx context.Context, code int) error {
	if err := models.LanguageCode(code).Validate(); err != nil {
		return fmt.Errorf("SetLanguage: %w", err)
	}

	body := fmt.Sprintf(`<sysLanguage>%d</sysLanguage>`, code)

	return s.sendStep(ctx, "language", "POST", body)
}

// Enter sends SETUP_ENTER.
func (s *Session) Enter(ctx context.Context) error {
	return s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_ENTER"/>`)
}

// IdentifyLeave sends SETUP_IDENTIFY_DEVICE_LEAVE.
func (s *Session) IdentifyLeave(ctx context.Context) error {
	return s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_IDENTIFY_DEVICE_LEAVE"/>`)
}

// SetName POSTs a device-name change. An empty name is a no-op.
func (s *Session) SetName(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}

	body := fmt.Sprintf(`<name>%s</name>`, xmlBodyEscape(name))

	return s.sendStep(ctx, "name", "POST", body)
}

// SetMargeAccount sends the canonical PairDeviceWithAccount envelope.
// authToken defaults to DefaultMargeAuthToken when empty.
//
// If SessionConfig.PairingExtras.BoseServer is set, the payload is
// extended with <boseServer>, <updateServer>, and <accountEmail>
// matching the official Bose app's shape (and Zimbo88's OpenCloudTouch
// USB-less script). UpdateServer and AccountEmail derive from
// BoseServer when not explicitly set.
func (s *Session) SetMargeAccount(ctx context.Context, accountID, authToken string) error {
	if accountID == "" {
		return errors.New("SetMargeAccount: accountID is required")
	}

	if authToken == "" {
		authToken = DefaultMargeAuthToken
	}

	return s.sendStep(ctx, "setMargeAccount", "POST", buildPairDeviceWithAccountXML(accountID, authToken, s.pairingExtras))
}

// buildPairDeviceWithAccountXML serializes the <PairDeviceWithAccount>
// body. Extracted so tests can pin the exact shape without driving a
// full WebSocket session.
func buildPairDeviceWithAccountXML(accountID, authToken string, extras MargePairingExtras) string {
	var b strings.Builder
	b.WriteString(`<PairDeviceWithAccount>`)
	b.WriteString(`<accountId>` + xmlBodyEscape(accountID) + `</accountId>`)
	b.WriteString(`<userAuthToken>` + xmlBodyEscape(authToken) + `</userAuthToken>`)

	if extras.BoseServer != "" {
		b.WriteString(`<boseServer>` + xmlBodyEscape(extras.BoseServer) + `</boseServer>`)

		updateServer := extras.UpdateServer
		if updateServer == "" {
			updateServer = strings.TrimRight(extras.BoseServer, "/") + "/updates/soundtouch"
		}

		b.WriteString(`<updateServer>` + xmlBodyEscape(updateServer) + `</updateServer>`)

		email := extras.AccountEmail
		if email == "" {
			email = DefaultMargePairingEmail
		}

		b.WriteString(`<accountEmail>` + xmlBodyEscape(email) + `</accountEmail>`)
	}

	b.WriteString(`</PairDeviceWithAccount>`)

	return b.String()
}

// Leave sends SETUP_LEAVE.
func (s *Session) Leave(ctx context.Context) error {
	return s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_LEAVE"/>`)
}

// PushCustomerSupportInfo triggers the post-setup telemetry sync. Harmless
// on our local service.
func (s *Session) PushCustomerSupportInfo(ctx context.Context) error {
	return s.sendStep(ctx, "pushCustomerSupportInfoToMarge", "GET", "")
}

// xmlAttrEscape escapes the small set of characters that would break an
// XML attribute context. We build envelopes by concatenation because the
// body fragments are already valid XML — running them through encoding/xml
// would re-escape nested tags.
func xmlAttrEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "<", "&lt;")

	return s
}

// xmlBodyEscape escapes text-node content using the encoding/xml helper.
func xmlBodyEscape(s string) string {
	var b strings.Builder

	_ = xml.EscapeText(&b, []byte(s))

	return b.String()
}
