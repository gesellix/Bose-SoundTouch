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

	return &Session{
		deviceID:      deviceID,
		conn:          conn,
		stepTimeout:   step,
		pairingExtras: cfg.PairingExtras,
	}, nil
}

// Close sends a normal-closure frame and closes the underlying socket.
func (s *Session) Close() error {
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
// Pushed <updates> and <SoundTouchSdkInfo> frames are ignored. The ack
// payload is consumed for error detection (<error …/>) only and never
// returned — every caller discards it.
func (s *Session) sendStep(ctx context.Context, route, method, body string) error {
	if s.conn == nil {
		return errors.New("setup session: connection closed")
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
		return fmt.Errorf("send %s: %w", route, err)
	}

	idNeedle := fmt.Sprintf(`requestID="%d"`, id)
	statusNeedle := fmt.Sprintf(`<status>/%s</status>`, route)
	urlNeedle := fmt.Sprintf(`url="%s"`, route)

	for {
		_ = s.conn.SetReadDeadline(deadline)

		_, data, err := s.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("await ack for %s: %w", route, err)
		}

		text := string(data)

		// Pushed event frames during setup (sourcesUpdated etc.) and the
		// SDK banner are not acks.
		if strings.Contains(text, "<updates ") || strings.Contains(text, "<SoundTouchSdkInfo") {
			continue
		}

		// Device-side errors surface as <error …/> in the body.
		if strings.Contains(strings.ToLower(text), "<error") {
			return fmt.Errorf("device rejected %s: %s", route, strings.TrimSpace(text))
		}

		if strings.Contains(text, idNeedle) || strings.Contains(text, statusNeedle) || strings.Contains(text, urlNeedle) {
			return nil
		}
	}
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

// SetLanguage POSTs sysLanguage. Code 2 = English.
func (s *Session) SetLanguage(ctx context.Context, code int) error {
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
