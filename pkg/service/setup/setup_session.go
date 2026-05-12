package setup

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultSetupStepTimeout = 8 * time.Second
	setupHandshakeTimeout   = 10 * time.Second

	// LanguageEnglish is the sysLanguage code for English. ‹2› is the
	// value the official Bose app sends during English-locale setup.
	LanguageEnglish = 2
)

// SetupStateMachine is the surface the InitPlan orchestrator drives. The
// concrete WebSocket-backed implementation is *SetupSession; tests inject
// an in-memory fake via Manager.NewSetupSession.
type SetupStateMachine interface {
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

// SetupSessionConfig configures DialSetupSession. Zero values pick safe
// defaults; in production callers normally pass an empty struct.
type SetupSessionConfig struct {
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
}

// SetupSession is a synchronous request/response WebSocket session driving
// the speaker's setup state machine. It is deliberately separate from
// pkg/client.WebSocketClient (which is event-oriented, auto-reconnecting,
// and stateful) — setup is a short, linear sequence and benefits from a
// purpose-built transport.
type SetupSession struct {
	deviceID    string
	conn        *websocket.Conn
	reqID       atomic.Int64
	stepTimeout time.Duration
}

// DialSetupSession opens a WebSocket to the speaker at deviceIP and
// returns a session ready to drive the SETUP state machine. deviceID is
// required because every <msg> envelope embeds it in the header; obtain
// it from /info before calling.
func DialSetupSession(deviceIP, deviceID string, cfg SetupSessionConfig) (*SetupSession, error) {
	if deviceID == "" {
		return nil, errors.New("DialSetupSession: deviceID is required for message routing")
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

	return &SetupSession{deviceID: deviceID, conn: conn, stepTimeout: step}, nil
}

// Close sends a normal-closure frame and closes the underlying socket.
func (s *SetupSession) Close() error {
	if s.conn == nil {
		return nil
	}

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
// envelope, sends it, and drains incoming frames until one references the
// same requestID, status path, or url attribute — that frame is the ack.
// Pushed <updates> and <SoundTouchSdkInfo> frames are ignored.
func (s *SetupSession) sendStep(ctx context.Context, route, method, body string) (string, error) {
	if s.conn == nil {
		return "", errors.New("setup session: connection closed")
	}

	id := s.reqID.Add(1)

	envelope := fmt.Sprintf(
		`<msg><header deviceID="%s" url="%s" method="%s"><request requestID="%d"/></header><body>%s</body></msg>`,
		xmlAttrEscape(s.deviceID), xmlAttrEscape(route), method, id, body,
	)

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(s.stepTimeout)
	}

	_ = s.conn.SetWriteDeadline(deadline)

	if err := s.conn.WriteMessage(websocket.TextMessage, []byte(envelope)); err != nil {
		return "", fmt.Errorf("send %s: %w", route, err)
	}

	idNeedle := fmt.Sprintf(`requestID="%d"`, id)
	statusNeedle := fmt.Sprintf(`<status>/%s</status>`, route)
	urlNeedle := fmt.Sprintf(`url="%s"`, route)

	for {
		_ = s.conn.SetReadDeadline(deadline)

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return "", fmt.Errorf("await ack for %s: %w", route, err)
		}

		text := string(data)

		// Pushed event frames during setup (sourcesUpdated etc.) and the
		// SDK banner are not acks.
		if strings.Contains(text, "<updates ") || strings.Contains(text, "<SoundTouchSdkInfo") {
			continue
		}

		// Device-side errors surface as <error …/> in the body.
		if strings.Contains(strings.ToLower(text), "<error") {
			return text, fmt.Errorf("device rejected %s: %s", route, strings.TrimSpace(text))
		}

		if strings.Contains(text, idNeedle) || strings.Contains(text, statusNeedle) || strings.Contains(text, urlNeedle) {
			return text, nil
		}
	}
}

// Start sends SETUP_START.
func (s *SetupSession) Start(ctx context.Context) error {
	_, err := s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_START"/>`)
	return err
}

// IdentifyEnter sends SETUP_IDENTIFY_DEVICE_ENTER. timeoutMs defaults to
// the value observed in captures (300 000 ms).
func (s *SetupSession) IdentifyEnter(ctx context.Context, timeoutMs int) error {
	if timeoutMs <= 0 {
		timeoutMs = 300000
	}

	body := fmt.Sprintf(`<setupState state="SETUP_IDENTIFY_DEVICE_ENTER" timeout="%d"/>`, timeoutMs)
	_, err := s.sendStep(ctx, "setup", "POST", body)

	return err
}

// SetLanguage POSTs sysLanguage. Code 2 = English.
func (s *SetupSession) SetLanguage(ctx context.Context, code int) error {
	body := fmt.Sprintf(`<sysLanguage>%d</sysLanguage>`, code)
	_, err := s.sendStep(ctx, "language", "POST", body)

	return err
}

// Enter sends SETUP_ENTER.
func (s *SetupSession) Enter(ctx context.Context) error {
	_, err := s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_ENTER"/>`)
	return err
}

// IdentifyLeave sends SETUP_IDENTIFY_DEVICE_LEAVE.
func (s *SetupSession) IdentifyLeave(ctx context.Context) error {
	_, err := s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_IDENTIFY_DEVICE_LEAVE"/>`)
	return err
}

// SetName POSTs a device-name change. An empty name is a no-op.
func (s *SetupSession) SetName(ctx context.Context, name string) error {
	if name == "" {
		return nil
	}

	body := fmt.Sprintf(`<name>%s</name>`, xmlBodyEscape(name))
	_, err := s.sendStep(ctx, "name", "POST", body)

	return err
}

// SetMargeAccount sends the canonical PairDeviceWithAccount envelope.
// authToken defaults to "Bearer aftertouch" when empty — our local
// service does not validate it, but a non-empty value matches the
// official app's shape.
func (s *SetupSession) SetMargeAccount(ctx context.Context, accountID, authToken string) error {
	if accountID == "" {
		return errors.New("SetMargeAccount: accountID is required")
	}

	if authToken == "" {
		authToken = "Bearer aftertouch"
	}

	body := fmt.Sprintf(
		`<PairDeviceWithAccount><accountId>%s</accountId><userAuthToken>%s</userAuthToken></PairDeviceWithAccount>`,
		xmlBodyEscape(accountID), xmlBodyEscape(authToken),
	)
	_, err := s.sendStep(ctx, "setMargeAccount", "POST", body)

	return err
}

// Leave sends SETUP_LEAVE.
func (s *SetupSession) Leave(ctx context.Context) error {
	_, err := s.sendStep(ctx, "setup", "POST", `<setupState state="SETUP_LEAVE"/>`)
	return err
}

// PushCustomerSupportInfo triggers the post-setup telemetry sync. Harmless
// on our local service.
func (s *SetupSession) PushCustomerSupportInfo(ctx context.Context) error {
	_, err := s.sendStep(ctx, "pushCustomerSupportInfoToMarge", "GET", "")
	return err
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
