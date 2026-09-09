package setup

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeSpeaker is a minimal WebSocket endpoint that records frames sent by
// Session and responds with canned replies. Each test wires its own
// reply policy by setting reply.
type fakeSpeaker struct {
	server *httptest.Server
	mu     sync.Mutex
	frames []string
	reply  func(frame string) []string
}

func newFakeSpeaker(t *testing.T) *fakeSpeaker {
	t.Helper()

	f := &fakeSpeaker{}
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"gabbo"},
		CheckOrigin:  func(*http.Request) bool { return true },
	}

	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade: %v", err)
			return
		}

		defer func() { _ = conn.Close() }()

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}

			f.mu.Lock()
			f.frames = append(f.frames, string(data))
			policy := f.reply
			f.mu.Unlock()

			var replies []string
			if policy != nil {
				replies = policy(string(data))
			} else {
				replies = []string{ackFor(string(data))}
			}

			for _, r := range replies {
				if err := conn.WriteMessage(websocket.TextMessage, []byte(r)); err != nil {
					return
				}
			}
		}
	}))

	t.Cleanup(f.server.Close)

	return f
}

// ackFor builds a minimal echo reply that carries the same requestID as
// the incoming frame, so the Session's correlation logic accepts it.
func ackFor(frame string) string {
	id := extractAttr(frame, `requestID="`, `"`)
	return fmt.Sprintf(`<msg><header url="setup"><response requestID="%s"/></header><body><status>ok</status></body></msg>`, id)
}

func extractAttr(s, prefix, suffix string) string {
	i := strings.Index(s, prefix)
	if i < 0 {
		return ""
	}

	rest := s[i+len(prefix):]

	j := strings.Index(rest, suffix)
	if j < 0 {
		return ""
	}

	return rest[:j]
}

func (f *fakeSpeaker) recordedFrames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	out := make([]string, len(f.frames))
	copy(out, f.frames)

	return out
}

// dialFakeSession opens a Session against the fake speaker. We turn
// the httptest server URL inside-out (http → ws, keep host:port) so the
// dialer reaches our handler.
func dialFakeSession(t *testing.T, f *fakeSpeaker, deviceID string) *Session {
	t.Helper()

	u, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	s, err := DialSession(u.Host, deviceID, SessionConfig{
		StepTimeout: 2 * time.Second,
		DialTimeout: 2 * time.Second,
		WSScheme:    "ws",
	})
	if err != nil {
		t.Fatalf("DialSession: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func TestSession_SendsCanonicalEnvelopes(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "AABBCCDDEEFF")

	ctx := context.Background()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := s.IdentifyEnter(ctx, 300000); err != nil {
		t.Fatalf("IdentifyEnter: %v", err)
	}

	if err := s.SetLanguage(ctx, 2); err != nil {
		t.Fatalf("SetLanguage: %v", err)
	}

	if err := s.Enter(ctx); err != nil {
		t.Fatalf("Enter: %v", err)
	}

	if err := s.IdentifyLeave(ctx); err != nil {
		t.Fatalf("IdentifyLeave: %v", err)
	}

	if err := s.SetName(ctx, "Living Room"); err != nil {
		t.Fatalf("SetName: %v", err)
	}

	if err := s.SetMargeAccount(ctx, "1234567", ""); err != nil {
		t.Fatalf("SetMargeAccount: %v", err)
	}

	if err := s.Leave(ctx); err != nil {
		t.Fatalf("Leave: %v", err)
	}

	if err := s.PushCustomerSupportInfo(ctx); err != nil {
		t.Fatalf("PushCustomerSupportInfo: %v", err)
	}

	frames := f.recordedFrames()
	if len(frames) != 9 {
		t.Fatalf("got %d frames, want 9: %v", len(frames), frames)
	}

	mustContain(t, frames[0], `deviceID="AABBCCDDEEFF"`, `url="setup"`, `method="POST"`, `<setupState state="SETUP_START"/>`)
	mustContain(t, frames[1], `url="setup"`, `<setupState state="SETUP_IDENTIFY_DEVICE_ENTER" timeout="300000"/>`)
	mustContain(t, frames[2], `url="language"`, `<sysLanguage>2</sysLanguage>`)
	mustContain(t, frames[3], `<setupState state="SETUP_ENTER"/>`)
	mustContain(t, frames[4], `<setupState state="SETUP_IDENTIFY_DEVICE_LEAVE"/>`)
	mustContain(t, frames[5], `url="name"`, `<name>Living Room</name>`)
	mustContain(t, frames[6], `url="setMargeAccount"`, `<accountId>1234567</accountId>`, `<userAuthToken>`+DefaultMargeAuthToken+`</userAuthToken>`)
	mustContain(t, frames[7], `<setupState state="SETUP_LEAVE"/>`)
	mustContain(t, frames[8], `url="pushCustomerSupportInfoToMarge"`, `method="GET"`)
}

func TestSession_RequestIDsAreUniquePerStep(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")
	ctx := context.Background()

	if err := s.Start(ctx); err != nil {
		t.Fatal(err)
	}

	if err := s.Enter(ctx); err != nil {
		t.Fatal(err)
	}

	frames := f.recordedFrames()
	id1 := extractAttr(frames[0], `requestID="`, `"`)
	id2 := extractAttr(frames[1], `requestID="`, `"`)

	if id1 == "" || id2 == "" {
		t.Fatalf("missing requestIDs: %q %q", id1, id2)
	}

	if id1 == id2 {
		t.Errorf("requestIDs must be unique per step, got %s twice", id1)
	}
}

func TestSession_SetLanguageRejectsUnknownCodeBeforeWrite(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")

	err := s.SetLanguage(context.Background(), 14)
	if err == nil {
		t.Fatal("expected unknown language code to be rejected")
	}

	if frames := f.recordedFrames(); len(frames) != 0 {
		t.Fatalf("invalid language wrote %d frames: %v", len(frames), frames)
	}
}

func TestSession_IgnoresUpdatesFramesBeforeAck(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(frame string) []string {
		id := extractAttr(frame, `requestID="`, `"`)
		// Push a sourcesUpdated frame first; the session must ignore it
		// and keep reading until the actual ack arrives.
		return []string{
			`<updates deviceID="X"><sourcesUpdated/></updates>`,
			`<SoundTouchSdkInfo build="x"/>`,
			fmt.Sprintf(`<msg><header url="setup"><response requestID="%s"/></header><body><status>/setup</status></body></msg>`, id),
		}
	}

	s := dialFakeSession(t, f, "X")
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start should succeed despite pushed update frames, got %v", err)
	}
}

func TestSession_SurfacesDeviceErrors(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(frame string) []string {
		return []string{
			`<msg><header url="setMargeAccount"><response/></header><body><error value="1003" name="ACCOUNT_REJECTED">no</error></body></msg>`,
		}
	}

	s := dialFakeSession(t, f, "X")

	err := s.SetMargeAccount(context.Background(), "1234567", "")
	if err == nil {
		t.Fatal("expected error from <error/> body")
	}

	if !strings.Contains(err.Error(), "device rejected setMargeAccount") {
		t.Errorf("err = %v, want to mention device rejection", err)
	}

	// The message names the failure rather than dumping raw XML.
	for _, want := range []string{"ACCOUNT_REJECTED", "code 1003"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
}

// TestSession_IgnoresNonResponseEnvelope pins the msgType guard: a <msg>
// frame that echoes our requestID but is not marked as a response is not an
// ack.
func TestSession_IgnoresNonResponseEnvelope(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(frame string) []string {
		id := extractAttr(frame, `requestID="`, `"`)

		return []string{
			fmt.Sprintf(`<msg><header deviceID="X" url="setup" method="POST">`+
				`<request requestID="%s" msgType="REQUEST"/></header><body/></msg>`, id),
		}
	}

	s := dialFakeSession(t, f, "X")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := s.Start(ctx); err == nil {
		t.Fatal("a non-RESPONSE envelope must not be accepted as an ack")
	}
}

func TestSession_RejectsEmptyDeviceID(t *testing.T) {
	_, err := DialSession("127.0.0.1:8080", "", SessionConfig{})
	if err == nil {
		t.Fatal("expected error for empty deviceID")
	}
}

func TestSession_RejectsEmptyAccountID(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")

	err := s.SetMargeAccount(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty accountID")
	}
}

func TestSession_EmptyNameIsNoOp(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")

	if err := s.SetName(context.Background(), ""); err != nil {
		t.Fatalf("SetName(\"\") should be no-op, got %v", err)
	}

	if len(f.recordedFrames()) != 0 {
		t.Errorf("expected no frames for empty name, got %v", f.recordedFrames())
	}
}

func TestSession_XMLAttributeEscape(t *testing.T) {
	// Device names with special characters must not break the envelope.
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, `quoted"<id>`)

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	frames := f.recordedFrames()
	if len(frames) != 1 {
		t.Fatalf("want 1 frame, got %d", len(frames))
	}

	mustContain(t, frames[0], `deviceID="quoted&quot;&lt;id>"`)
}

// TestBuildPairDeviceWithAccountXML pins both the minimal-payload
// shape (historical AfterTouch behaviour) and the extended-payload
// shape introduced for #195/#269 investigation. The extended path
// mirrors what the official Bose app and Zimbo88's OpenCloudTouch
// USB-less script send (see docs/reference/DEVICE-PAIRING-FLOW.md
// and https://github.com/scheilch/opencloudtouch/discussions/201).
func TestBuildPairDeviceWithAccountXML(t *testing.T) {
	t.Run("minimal payload — no extras", func(t *testing.T) {
		got := buildPairDeviceWithAccountXML("1234567", "Bearer tok", MargePairingExtras{})

		want := `<PairDeviceWithAccount>` +
			`<accountId>1234567</accountId>` +
			`<userAuthToken>Bearer tok</userAuthToken>` +
			`</PairDeviceWithAccount>`
		if got != want {
			t.Errorf("\n got: %s\nwant: %s", got, want)
		}
	})

	t.Run("extended payload — BoseServer triggers derived defaults", func(t *testing.T) {
		got := buildPairDeviceWithAccountXML(
			"1234567", "Bearer tok",
			MargePairingExtras{BoseServer: "https://soundtouch.local"},
		)

		mustContain(t, got,
			`<boseServer>https://soundtouch.local</boseServer>`,
			`<updateServer>https://soundtouch.local/updates/soundtouch</updateServer>`,
			`<accountEmail>`+DefaultMargePairingEmail+`</accountEmail>`,
		)
	})

	t.Run("extended payload — explicit UpdateServer + AccountEmail honoured", func(t *testing.T) {
		got := buildPairDeviceWithAccountXML(
			"1234567", "Bearer tok",
			MargePairingExtras{
				BoseServer:   "https://example.test",
				UpdateServer: "https://updates.example.test/firmware",
				AccountEmail: "user@example.test",
			},
		)

		mustContain(t, got,
			`<boseServer>https://example.test</boseServer>`,
			`<updateServer>https://updates.example.test/firmware</updateServer>`,
			`<accountEmail>user@example.test</accountEmail>`,
		)
	})

	t.Run("extended payload — BoseServer trailing slash trimmed when deriving UpdateServer", func(t *testing.T) {
		got := buildPairDeviceWithAccountXML(
			"1234567", "Bearer tok",
			MargePairingExtras{BoseServer: "https://soundtouch.local/"},
		)

		// Derived path uses TrimRight on BoseServer so we don't get
		// "soundtouch.local//updates/soundtouch".
		mustContain(t, got,
			`<updateServer>https://soundtouch.local/updates/soundtouch</updateServer>`,
		)
	})
}

func mustContain(t *testing.T, s string, needles ...string) {
	t.Helper()

	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Errorf("frame missing %q in: %s", n, s)
		}
	}
}

// TestSession_IgnoresAsyncErrorUpdateFrames is the regression for GH-704 (1).
//
// The speaker pushes device errors as a root-level <errorUpdate> frame, which
// has nothing to do with the step in flight. The old classifier tested for the
// substring "<error" after skipping <updates> and <SoundTouchSdkInfo>, and
// "<errorupdate" contains "<error" — so an unrelated source timeout aborted
// the whole init plan with "device rejected setup".
func TestSession_IgnoresAsyncErrorUpdateFrames(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(frame string) []string {
		id := extractAttr(frame, `requestID="`, `"`)

		return []string{
			`<errorUpdate deviceID="DEVICEID01">` +
				`<error value="1654" name="STORED_MUSIC_AP_TIMEOUT" severity="Unrecoverable">APServer: Timeout</error>` +
				`</errorUpdate>`,
			ackWithID(id),
		}
	}

	s := dialFakeSession(t, f, "DEVICEID01")

	var logged []string

	s.logf = func(format string, args ...any) { logged = append(logged, fmt.Sprintf(format, args...)) }

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start must ignore an unrelated errorUpdate, got %v", err)
	}

	// The frame is skipped, not discarded: it names the failure precisely and
	// is worth seeing in the log.
	if !strings.Contains(strings.Join(logged, "\n"), "STORED_MUSIC_AP_TIMEOUT") {
		t.Errorf("device error was not logged; got %v", logged)
	}
}

// TestSession_RejectsStaleAckForSameRoute is the regression for GH-704 (2).
//
// Five of the nine init-plan steps use url="setup". Ack matching used to be
// `requestID || <status>/route || url="route"`, so a late response to step N
// satisfied the url needle while step N+1 was waiting and was accepted as
// N+1's ack. Correlation is now exact for enveloped replies.
func TestSession_RejectsStaleAckForSameRoute(t *testing.T) {
	f := newFakeSpeaker(t)

	var stepCount int

	f.reply = func(frame string) []string {
		stepCount++
		if stepCount == 1 {
			return []string{ackWithID(extractAttr(frame, `requestID="`, `"`))}
		}

		// Step 2 gets NO genuine ack — only a late reply carrying step 1's
		// requestID and one carrying an unrelated requestID. Both are
		// url="setup" frames, so the old url-needle match accepted the
		// first of them and the step wrongly succeeded.
		return []string{
			ackWithID("1"),
			`<msg><header deviceID="X" url="setup" method="POST">` +
				`<request requestID="99" msgType="RESPONSE"/></header><body/></msg>`,
		}
	}

	s := dialFakeSession(t, f, "X")

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	err := s.Enter(ctx)
	if err == nil {
		t.Fatal("Enter accepted another step's response as its ack")
	}

	if !strings.Contains(err.Error(), "await ack for setup") {
		t.Errorf("err = %v, want a timeout awaiting the real ack", err)
	}
}

// TestSession_AcceptsBareStatusAck pins that a bare <status>/route</status>
// reply — the shape documented for pushCustomerSupportInfoToMarge and
// selectLastWiFiSource — is still accepted. These frames carry no requestID,
// so they can only be correlated by arrival order.
func TestSession_AcceptsBareStatusAck(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(string) []string {
		return []string{
			`<status>/someOtherRoute</status>`,
			`<?xml version="1.0" encoding="UTF-8" ?><status>/pushCustomerSupportInfoToMarge</status>`,
		}
	}

	s := dialFakeSession(t, f, "X")

	if err := s.PushCustomerSupportInfo(context.Background()); err != nil {
		t.Fatalf("PushCustomerSupportInfo: %v", err)
	}
}

// TestSession_SurvivesStepTimeout is the regression for GH-704 (3).
//
// sendStep used to set a per-step read deadline on a long-lived connection.
// gorilla/websocket treats any read error as fatal — a deadline expiry
// included — and returns it instantly from every later read, so one timed-out
// step made every subsequent step fail immediately with "i/o timeout", which
// reads exactly like the device refusing.
func TestSession_SurvivesStepTimeout(t *testing.T) {
	f := newFakeSpeaker(t)

	var stepCount int

	f.reply = func(frame string) []string {
		stepCount++
		if stepCount == 1 {
			return nil // silence: this step must time out
		}

		return []string{ackWithID(extractAttr(frame, `requestID="`, `"`))}
	}

	s := dialFakeSession(t, f, "X")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := s.Start(ctx); err == nil {
		t.Fatal("expected the first step to time out")
	}

	// The connection must still be usable.
	if err := s.Enter(context.Background()); err != nil {
		t.Fatalf("step after a timeout must still work, got %v", err)
	}
}

// TestSession_ReportsClosedConnection pins that a step waiting on a dead
// connection reports the read error rather than hanging or reporting a
// closed channel.
func TestSession_ReportsClosedConnection(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(string) []string { return nil }

	s := dialFakeSession(t, f, "X")

	// Drop the server side out from under the session.
	f.server.CloseClientConnections()

	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected an error once the connection is gone")
	}

	if !strings.Contains(err.Error(), "await ack for setup") {
		t.Errorf("err = %v, want it to name the step being awaited", err)
	}
}

// ackWithID builds an ack in the shape real hardware uses: the requestID
// echoed on <request> together with msgType="RESPONSE".
func ackWithID(id string) string {
	return fmt.Sprintf(
		`<msg><header deviceID="X" url="setup" method="POST">`+
			`<request requestID="%s" msgType="RESPONSE"><info type="new"/></request>`+
			`</header><body><status>ok</status></body></msg>`, id)
}

// TestSession_SurfacesWrappedDeviceErrors covers the <errors> wrapper shape
// that some firmware versions use (models.ErrorsResponse). Matching only a
// direct <error> child of <body> would let the rejection through as an
// uncorrelated frame, turning a clear refusal into a step timeout.
func TestSession_SurfacesWrappedDeviceErrors(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(frame string) []string {
		id := extractAttr(frame, `requestID="`, `"`)

		return []string{fmt.Sprintf(
			`<msg><header deviceID="X" url="setMargeAccount" method="POST">`+
				`<request requestID="%s" msgType="RESPONSE"/></header>`+
				`<body><errors deviceID="X">`+
				`<error value="1003" name="ACCOUNT_REJECTED" severity="Unrecoverable">no</error>`+
				`</errors></body></msg>`, id)}
	}

	s := dialFakeSession(t, f, "X")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := s.SetMargeAccount(ctx, "1234567", "")
	if err == nil {
		t.Fatal("expected an error from an <errors>-wrapped rejection")
	}

	for _, want := range []string{"device rejected setMargeAccount", "ACCOUNT_REJECTED", "code 1003"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %v, want it to contain %q", err, want)
		}
	}
}

// TestSession_IgnoresOtherStepsErrors pins that a late error response to an
// earlier step does not abort the step in flight. Without the requestID gate
// this would be the same cross-step misattribution as
// TestSession_RejectsStaleAckForSameRoute, just on the error path.
func TestSession_IgnoresOtherStepsErrors(t *testing.T) {
	f := newFakeSpeaker(t)

	var stepCount int

	f.reply = func(frame string) []string {
		stepCount++

		id := extractAttr(frame, `requestID="`, `"`)
		if stepCount == 1 {
			return []string{ackWithID(id)}
		}

		return []string{
			// A rejection carrying step 1's requestID, then our real ack.
			`<msg><header deviceID="X" url="setup" method="POST">` +
				`<request requestID="1" msgType="RESPONSE"/></header>` +
				`<body><error value="1003" name="ACCOUNT_REJECTED">no</error></body></msg>`,
			ackWithID(id),
		}
	}

	s := dialFakeSession(t, f, "X")
	ctx := context.Background()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := s.Enter(ctx); err != nil {
		t.Fatalf("Enter must ignore another step's error response, got %v", err)
	}
}

// TestSession_AcceptsRouteMatchForUniqueRoute pins the fallback for a reply
// that carries no requestID at all. It is allowed only while a route has been
// used once: unique routes (language, name, setMargeAccount, telemetry) cannot
// cross-talk, whereas url="setup" is shared by five steps and must not match
// on the route alone.
func TestSession_AcceptsRouteMatchForUniqueRoute(t *testing.T) {
	f := newFakeSpeaker(t)
	f.reply = func(string) []string {
		return []string{
			`<msg><header deviceID="X" url="name" method="POST"/>` +
				`<body><status>/name</status></body></msg>`,
		}
	}

	s := dialFakeSession(t, f, "X")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := s.SetName(ctx, "Kitchen"); err != nil {
		t.Fatalf("SetName: %v", err)
	}
}

// TestSession_RouteMatchIsRefusedForRepeatedRoute is the other half: once a
// route has been used twice, an uncorrelated reply naming it is not enough.
func TestSession_RouteMatchIsRefusedForRepeatedRoute(t *testing.T) {
	f := newFakeSpeaker(t)

	var stepCount int

	f.reply = func(frame string) []string {
		stepCount++
		if stepCount == 1 {
			return []string{ackWithID(extractAttr(frame, `requestID="`, `"`))}
		}

		return []string{
			`<msg><header deviceID="X" url="setup" method="POST"/>` +
				`<body><status>/setup</status></body></msg>`,
		}
	}

	s := dialFakeSession(t, f, "X")

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if err := s.Enter(ctx); err == nil {
		t.Fatal("an uncorrelated url=\"setup\" reply must not ack the second setup step")
	}
}

// TestSession_CloseIsIdempotent guards the reader-goroutine teardown: closing
// the done channel twice would panic.
func TestSession_CloseIsIdempotent(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")

	if err := s.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
