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
// SetupSession and responds with canned replies. Each test wires its own
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
// the incoming frame, so the SetupSession's correlation logic accepts it.
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

// dialFakeSession opens a SetupSession against the fake speaker. We turn
// the httptest server URL inside-out (http → ws, keep host:port) so the
// dialer reaches our handler.
func dialFakeSession(t *testing.T, f *fakeSpeaker, deviceID string) *SetupSession {
	t.Helper()

	u, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	s, err := DialSetupSession(u.Host, deviceID, SetupSessionConfig{
		StepTimeout: 2 * time.Second,
		DialTimeout: 2 * time.Second,
		WSScheme:    "ws",
	})
	if err != nil {
		t.Fatalf("DialSetupSession: %v", err)
	}

	t.Cleanup(func() { _ = s.Close() })

	return s
}

func TestSetupSession_SendsCanonicalEnvelopes(t *testing.T) {
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
	mustContain(t, frames[6], `url="setMargeAccount"`, `<accountId>1234567</accountId>`, `<userAuthToken>Bearer aftertouch</userAuthToken>`)
	mustContain(t, frames[7], `<setupState state="SETUP_LEAVE"/>`)
	mustContain(t, frames[8], `url="pushCustomerSupportInfoToMarge"`, `method="GET"`)
}

func TestSetupSession_RequestIDsAreUniquePerStep(t *testing.T) {
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

func TestSetupSession_IgnoresUpdatesFramesBeforeAck(t *testing.T) {
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

func TestSetupSession_SurfacesDeviceErrors(t *testing.T) {
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
}

func TestSetupSession_RejectsEmptyDeviceID(t *testing.T) {
	_, err := DialSetupSession("127.0.0.1:8080", "", SetupSessionConfig{})
	if err == nil {
		t.Fatal("expected error for empty deviceID")
	}
}

func TestSetupSession_RejectsEmptyAccountID(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")

	err := s.SetMargeAccount(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty accountID")
	}
}

func TestSetupSession_EmptyNameIsNoOp(t *testing.T) {
	f := newFakeSpeaker(t)
	s := dialFakeSession(t, f, "X")

	if err := s.SetName(context.Background(), ""); err != nil {
		t.Fatalf("SetName(\"\") should be no-op, got %v", err)
	}

	if len(f.recordedFrames()) != 0 {
		t.Errorf("expected no frames for empty name, got %v", f.recordedFrames())
	}
}

func TestSetupSession_XMLAttributeEscape(t *testing.T) {
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

func mustContain(t *testing.T, s string, needles ...string) {
	t.Helper()

	for _, n := range needles {
		if !strings.Contains(s, n) {
			t.Errorf("frame missing %q in: %s", n, s)
		}
	}
}
