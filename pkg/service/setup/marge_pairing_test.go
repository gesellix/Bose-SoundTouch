package setup

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

// fakeDevice spins up an httptest.Server that pretends to be the SoundTouch
// device's :8090 HTTP API. It records POSTs to /setMargeAccount so tests
// can assert on the body.
type fakeDevice struct {
	srv                 *httptest.Server
	addr                string // "host:port" usable as deviceIP
	supportsSetMarge    bool
	postStatus          int // status code returned for POST /setMargeAccount
	postDelay           time.Duration
	gotPostBody         string
	margeAccountUUID    string // served by /info; empty means "unpaired"
	configurationStatus string // served by /soundTouchConfigurationStatus; empty = route not served (404)

	// nowPlayingSources is the sequence of source attributes served by
	// /now_playing, one per request, repeating the last entry once exhausted.
	// A speaker that is not stuck reports something like "STANDBY"; one still
	// onboarding reports "SETUP".
	nowPlayingSources []string
}

func newFakeDevice(t *testing.T) *fakeDevice {
	t.Helper()

	d := &fakeDevice{
		supportsSetMarge:  true,
		postStatus:        http.StatusOK,
		nowPlayingSources: []string{"STANDBY"},
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/supportedURLs", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		if d.supportsSetMarge {
			_, _ = w.Write([]byte(`<supportedURLs><URL location="/setMargeAccount"/><URL location="/info"/></supportedURLs>`))
			return
		}

		_, _ = w.Write([]byte(`<supportedURLs><URL location="/info"/></supportedURLs>`))
	})

	mux.HandleFunc("/setMargeAccount", func(w http.ResponseWriter, r *http.Request) {
		if d.postDelay > 0 {
			time.Sleep(d.postDelay)
		}

		body, _ := io.ReadAll(r.Body)
		d.gotPostBody = string(body)

		w.WriteHeader(d.postStatus)
	})

	mux.HandleFunc("/info", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<info deviceID="AABBCCDDEE0A"><margeAccountUUID>%s</margeAccountUUID></info>`, d.margeAccountUUID)
	})

	mux.HandleFunc("/now_playing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		source := "STANDBY"
		if len(d.nowPlayingSources) > 0 {
			source = d.nowPlayingSources[0]
			if len(d.nowPlayingSources) > 1 {
				d.nowPlayingSources = d.nowPlayingSources[1:]
			}
		}

		fmt.Fprintf(w, `<nowPlaying deviceID="AABBCCDDEE0A" source="%s"><ContentItem source="%s"/></nowPlaying>`, source, source)
	})

	mux.HandleFunc("/soundTouchConfigurationStatus", func(w http.ResponseWriter, _ *http.Request) {
		if d.configurationStatus == "" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprintf(w, `<SoundTouchConfigurationStatus status="%s" />`, d.configurationStatus)
	})

	d.srv = httptest.NewServer(mux)

	u := d.srv.URL[len("http://"):]

	host, port, err := net.SplitHostPort(u)
	if err != nil {
		t.Fatalf("split httptest URL: %v", err)
	}

	d.addr = host + ":" + port

	t.Cleanup(d.srv.Close)

	return d
}

func TestPairAccount_HappyPathHTTP(t *testing.T) {
	d := newFakeDevice(t)

	m := &Manager{}

	res, _, err := m.PairAccount(t.Context(), d.addr, "1234567", nil)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if res.Method != "http" {
		t.Errorf("Method = %q, want http", res.Method)
	}

	if !res.SetMargeAccountSupported {
		t.Error("SetMargeAccountSupported should be true")
	}

	if !res.HTTPAttempted {
		t.Error("HTTPAttempted should be true")
	}

	if res.TelnetAttempted {
		t.Error("TelnetAttempted should be false on the happy HTTP path")
	}

	if !strings.Contains(d.gotPostBody, "<accountId>1234567</accountId>") {
		t.Errorf("device received %q, want <accountId>1234567</accountId>", d.gotPostBody)
	}
}

func TestPairAccount_FallsBackWhenSetMargeAccountMissing(t *testing.T) {
	d := newFakeDevice(t)
	d.supportsSetMarge = false

	f := &fakeTelnet{
		responses: map[string]string{"envswitch accountid set 1234567": "OK\n"},
	}

	m := &Manager{}

	res, _, err := m.PairAccount(t.Context(), d.addr, "1234567", f)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if res.Method != "telnet" {
		t.Errorf("Method = %q, want telnet", res.Method)
	}

	if res.SetMargeAccountSupported {
		t.Error("SetMargeAccountSupported should be false")
	}

	if res.HTTPAttempted {
		t.Error("HTTPAttempted should be false when supportedURLs reports the endpoint missing")
	}

	if !res.TelnetAttempted {
		t.Error("TelnetAttempted should be true")
	}

	if len(f.commands) != 1 || f.commands[0] != "envswitch accountid set 1234567" {
		t.Errorf("telnet commands = %v, want one envswitch accountid", f.commands)
	}
}

func TestPairAccount_FallsBackWhenHTTPReturnsServerError(t *testing.T) {
	d := newFakeDevice(t)
	d.postStatus = http.StatusBadGateway

	f := &fakeTelnet{
		responses: map[string]string{"envswitch accountid set 7654321": "OK\n"},
	}

	m := &Manager{}

	res, _, err := m.PairAccount(t.Context(), d.addr, "7654321", f)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if res.Method != "telnet" {
		t.Errorf("Method = %q, want telnet", res.Method)
	}

	if res.HTTPError == "" {
		t.Error("HTTPError should be populated when POST returned 502")
	}

	if !res.TelnetAttempted {
		t.Error("TelnetAttempted should be true after HTTP failure")
	}
}

func TestPairAccount_HTTPSuccessSkipsTelnet(t *testing.T) {
	d := newFakeDevice(t)

	f := &fakeTelnet{}

	m := &Manager{}

	res, _, err := m.PairAccount(t.Context(), d.addr, "1234567", f)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if res.Method != "http" {
		t.Errorf("Method = %q, want http", res.Method)
	}

	if len(f.commands) != 0 {
		t.Errorf("telnet should not have been used; commands = %v", f.commands)
	}
}

func TestPairAccount_NoTelnetAndHTTPMissingReturnsClearError(t *testing.T) {
	d := newFakeDevice(t)
	d.supportsSetMarge = false

	m := &Manager{}

	_, _, err := m.PairAccount(t.Context(), d.addr, "1234567", nil)
	if err == nil {
		t.Fatal("expected error when both paths are unavailable")
	}

	if !strings.Contains(err.Error(), "no telnet client") {
		t.Errorf("err = %v, want to mention missing telnet client", err)
	}
}

func TestPairAccount_TelnetCommandNotFoundReportsBothPaths(t *testing.T) {
	d := newFakeDevice(t)
	d.supportsSetMarge = false

	f := &fakeTelnet{
		responses: map[string]string{"envswitch accountid set 1234567": "Command not found\n"},
	}

	m := &Manager{}

	_, _, err := m.PairAccount(t.Context(), d.addr, "1234567", f)
	if err == nil {
		t.Fatal("expected error when telnet rejects the fallback")
	}

	if !strings.Contains(err.Error(), "envswitch") {
		t.Errorf("err = %v, want to mention envswitch", err)
	}
}

func TestPairAccount_RejectsInvalidAccountID(t *testing.T) {
	m := &Manager{}

	for _, badID := range []string{"", "12345", "12345678", "abcdefg", "12345 6"} {
		_, _, err := m.PairAccount(t.Context(), "127.0.0.1:9999", badID, nil)
		if err == nil {
			t.Errorf("PairAccount accepted invalid ID %q", badID)
		}
	}
}

func TestPairAccount_TelnetTransportErrorReturned(t *testing.T) {
	d := newFakeDevice(t)
	d.supportsSetMarge = false

	f := &fakeTelnet{
		fail: map[string]error{"envswitch accountid set 1234567": errors.New("connection reset")},
	}

	m := &Manager{}

	_, _, err := m.PairAccount(t.Context(), d.addr, "1234567", f)
	if err == nil {
		t.Fatal("expected telnet transport error to be surfaced")
	}

	if !strings.Contains(err.Error(), "connection reset") {
		t.Errorf("err = %v, want to wrap connection reset", err)
	}
}

func TestEnsureMargeAccountPaired_AlreadyPairedSkipsPairing(t *testing.T) {
	d := newFakeDevice(t)
	d.margeAccountUUID = "1234567"

	f := &fakeTelnet{}

	m := NewManager("", nil, nil)

	accountID, alreadyPaired, _, err := m.EnsureMargeAccountPaired(t.Context(), d.addr, "", f)
	if err != nil {
		t.Fatalf("EnsureMargeAccountPaired: %v", err)
	}

	if !alreadyPaired {
		t.Error("alreadyPaired should be true")
	}

	if accountID != "1234567" {
		t.Errorf("accountID = %q, want the existing margeAccountUUID", accountID)
	}

	if len(f.commands) != 0 || d.gotPostBody != "" {
		t.Error("pairing should not have been attempted for an already-paired device")
	}
}

func TestEnsureMargeAccountPaired_UnpairedGeneratesAndPairs(t *testing.T) {
	d := newFakeDevice(t)
	d.margeAccountUUID = ""

	m := NewManager("", nil, nil)

	accountID, alreadyPaired, _, err := m.EnsureMargeAccountPaired(t.Context(), d.addr, "", nil)
	if err != nil {
		t.Fatalf("EnsureMargeAccountPaired: %v", err)
	}

	if alreadyPaired {
		t.Error("alreadyPaired should be false for an unpaired device")
	}

	if !datastore.IsSafeIdentifier(accountID) {
		t.Errorf("accountID %q is not a valid generated ID", accountID)
	}

	if !strings.Contains(d.gotPostBody, "<accountId>"+accountID+"</accountId>") {
		t.Errorf("device received %q, want it to be paired with the generated %q", d.gotPostBody, accountID)
	}
}

func TestEnsureMargeAccountPaired_UnpairedUsesWantAccountID(t *testing.T) {
	d := newFakeDevice(t)
	d.margeAccountUUID = ""

	m := NewManager("", nil, nil)

	accountID, alreadyPaired, _, err := m.EnsureMargeAccountPaired(t.Context(), d.addr, "7654321", nil)
	if err != nil {
		t.Fatalf("EnsureMargeAccountPaired: %v", err)
	}

	if alreadyPaired {
		t.Error("alreadyPaired should be false for an unpaired device")
	}

	if accountID != "7654321" {
		t.Errorf("accountID = %q, want the requested 7654321", accountID)
	}

	if !strings.Contains(d.gotPostBody, "<accountId>7654321</accountId>") {
		t.Errorf("device received %q, want the requested account id", d.gotPostBody)
	}
}

func TestEnsureMargeAccountPaired_RejectsInvalidWantAccountID(t *testing.T) {
	d := newFakeDevice(t)
	d.margeAccountUUID = ""

	m := NewManager("", nil, nil)

	_, _, _, err := m.EnsureMargeAccountPaired(t.Context(), d.addr, "not/valid", nil)
	if err == nil {
		t.Fatal("expected an error for an invalid --account value")
	}
}

func TestEnsureMargeAccountPaired_PropagatesPairingFailure(t *testing.T) {
	d := newFakeDevice(t)
	d.margeAccountUUID = ""
	d.supportsSetMarge = false

	m := NewManager("", nil, nil)

	_, _, _, err := m.EnsureMargeAccountPaired(t.Context(), d.addr, "1234567", nil)
	if err == nil {
		t.Fatal("expected an error when HTTP pairing is unsupported and no telnet client is given")
	}
}

func TestReadConfigurationStatus_ReturnsRawStatus(t *testing.T) {
	d := newFakeDevice(t)
	d.configurationStatus = ConfigurationStatusConfigured

	m := &Manager{}

	status, err := m.ReadConfigurationStatus(d.addr)
	if err != nil {
		t.Fatalf("ReadConfigurationStatus: %v", err)
	}

	if status != ConfigurationStatusConfigured {
		t.Errorf("status = %q, want %q", status, ConfigurationStatusConfigured)
	}
}

func TestReadConfigurationStatus_ErrorsWhenRouteUnsupported(t *testing.T) {
	d := newFakeDevice(t)
	d.configurationStatus = ""

	m := &Manager{}

	if _, err := m.ReadConfigurationStatus(d.addr); err == nil {
		t.Fatal("expected an error when the route is unsupported (404)")
	}
}

func TestPreflightInitPlan_NotConfiguredNeedsRepair(t *testing.T) {
	d := newFakeDevice(t)
	d.configurationStatus = ConfigurationStatusNotConfigured

	m := &Manager{}

	needed, status, err := m.PreflightInitPlan(d.addr)
	if err != nil {
		t.Fatalf("PreflightInitPlan: %v", err)
	}

	if !needed {
		t.Error("needed should be true for SOUNDTOUCH_NOT_CONFIGURED")
	}

	if status != ConfigurationStatusNotConfigured {
		t.Errorf("status = %q, want %q", status, ConfigurationStatusNotConfigured)
	}
}

func TestPreflightInitPlan_AlreadyConfiguredIsNoOp(t *testing.T) {
	d := newFakeDevice(t)
	d.configurationStatus = ConfigurationStatusConfigured

	m := &Manager{}

	needed, status, err := m.PreflightInitPlan(d.addr)
	if err != nil {
		t.Fatalf("PreflightInitPlan: %v", err)
	}

	if needed {
		t.Error("needed should be false for SOUNDTOUCH_CONFIGURED")
	}

	if status != ConfigurationStatusConfigured {
		t.Errorf("status = %q, want %q", status, ConfigurationStatusConfigured)
	}
}

func TestPreflightInitPlan_UnsupportedSetMargeAccountFailsClosed(t *testing.T) {
	d := newFakeDevice(t)
	d.supportsSetMarge = false
	d.configurationStatus = ConfigurationStatusNotConfigured

	m := &Manager{}

	needed, _, err := m.PreflightInitPlan(d.addr)
	if err == nil {
		t.Fatal("expected an error when /setMargeAccount is not listed in /supportedURLs")
	}

	if needed {
		t.Error("needed should be false when preflight fails")
	}
}

func TestPreflightInitPlan_UnrecognisedStatusFailsClosed(t *testing.T) {
	d := newFakeDevice(t)
	d.configurationStatus = "SOMETHING_UNEXPECTED"

	m := &Manager{}

	needed, status, err := m.PreflightInitPlan(d.addr)
	if err == nil {
		t.Fatal("expected an error for an unrecognised status value")
	}

	if needed {
		t.Error("needed should be false when the status is unrecognised")
	}

	if status != "SOMETHING_UNEXPECTED" {
		t.Errorf("status = %q, want the raw unrecognised value returned alongside the error", status)
	}
}

// Account-ID format validation is now solely datastore.IsSafeIdentifier's
// responsibility (see datastore.TestIsSafeIdentifier); setup no longer has
// its own account-ID validator to test.

func TestGenerateAccountID_AvoidsCollisions(t *testing.T) {
	id, err := GenerateAccountID(nil)
	if err != nil {
		t.Fatalf("GenerateAccountID(nil): %v", err)
	}

	if !datastore.IsSafeIdentifier(id) {
		t.Errorf("generated ID %q is not valid", id)
	}

	// Block out a fairly small space and check we still get a fresh ID.
	known := []string{"1000000", "1000001", "1000002"}

	for i := 0; i < 5; i++ {
		got, err := GenerateAccountID(known)
		if err != nil {
			t.Fatalf("GenerateAccountID: %v", err)
		}

		for _, k := range known {
			if got == k {
				t.Errorf("generated %q collides with known list %v", got, known)
			}
		}
	}
}

// withFastSettle shrinks the post-escalation settle so the stuck-path tests
// do not sit through the production 2 s.
func withFastSettle(t *testing.T) {
	t.Helper()

	prev := wsSettleDelay
	wsSettleDelay = time.Millisecond

	t.Cleanup(func() { wsSettleDelay = prev })
}

// TestPairAccount_NoEscalationWhenSpeakerLeftSetup is the ordinary case: the
// account was written and the speaker is playing (or idle), so nothing extra
// should happen. In particular no WebSocket session is opened against a
// perfectly healthy speaker.
func TestPairAccount_NoEscalationWhenSpeakerLeftSetup(t *testing.T) {
	d := newFakeDevice(t)
	d.nowPlayingSources = []string{"STANDBY"}

	var dialled bool

	m := &Manager{NewSession: func(_, _ string, _ time.Duration) (StateMachine, error) {
		dialled = true
		return &fakeSession{errors: map[string]error{}}, nil
	}}

	res, logs, err := m.PairAccount(t.Context(), d.addr, "1234567", nil)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if dialled {
		t.Error("opened a setup WebSocket against a speaker that was not stuck")
	}

	if res.StuckInSetup || res.WSAttempted {
		t.Errorf("unexpected escalation: %+v", res)
	}

	if res.Method != "http" {
		t.Errorf("Method = %q, want http", res.Method)
	}

	if !strings.Contains(logs, "out of SETUP") {
		t.Errorf("logs should record the check, got: %s", logs)
	}
}

// TestPairAccount_EscalatesWhenStuckInSetup is the #646 shape: the HTTP call
// succeeds and the account ID lands, but the firmware stays in onboarding. A
// plain fallback chain would never reach the WebSocket path here, because
// nothing failed.
func TestPairAccount_EscalatesWhenStuckInSetup(t *testing.T) {
	withFastSettle(t)

	d := newFakeDevice(t)
	// Stuck on the post-pair check, recovered on the re-read afterwards.
	d.nowPlayingSources = []string{"SETUP", "STANDBY"}

	session := &fakeSession{errors: map[string]error{}}
	m := &Manager{NewSession: func(_, _ string, _ time.Duration) (StateMachine, error) {
		return session, nil
	}}

	res, logs, err := m.PairAccount(t.Context(), d.addr, "1234567", nil)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if !res.StuckInSetup {
		t.Error("StuckInSetup should be set when the speaker still reports SETUP")
	}

	if !res.WSAttempted {
		t.Error("WSAttempted should be set")
	}

	if !res.LeftSetup {
		t.Error("LeftSetup should be set once the speaker reports a different source")
	}

	if res.Method != "ws" {
		t.Errorf("Method = %q, want ws (the WebSocket call is what completed the pairing)", res.Method)
	}

	// The escalation sends the minimal payload: no boseServer extras, since
	// this is an escalation of the same request, not a migration.
	want := "SetMargeAccount(1234567,)"
	if len(session.calls) != 1 || session.calls[0] != want {
		t.Errorf("session calls = %v, want exactly [%s]", session.calls, want)
	}

	if !session.closed {
		t.Error("session should be closed")
	}

	if !strings.Contains(logs, "escalating") {
		t.Errorf("logs should explain the escalation, got: %s", logs)
	}
}

// TestPairAccount_EscalationFailureDoesNotFailPairing covers a speaker that is
// stuck and also refuses the WebSocket. The account ID was still written, so
// reporting the whole call as failed would be wrong and would push the UI into
// an error state over a best-effort extra step.
func TestPairAccount_EscalationFailureDoesNotFailPairing(t *testing.T) {
	d := newFakeDevice(t)
	d.nowPlayingSources = []string{"SETUP"}

	m := &Manager{NewSession: func(_, _ string, _ time.Duration) (StateMachine, error) {
		return nil, errors.New("connection refused")
	}}

	res, _, err := m.PairAccount(t.Context(), d.addr, "1234567", nil)
	if err != nil {
		t.Fatalf("PairAccount should still succeed, got: %v", err)
	}

	if !res.StuckInSetup || !res.WSAttempted {
		t.Errorf("escalation should be recorded as attempted: %+v", res)
	}

	if res.WSError == "" {
		t.Error("WSError should explain why the escalation failed")
	}

	if res.LeftSetup {
		t.Error("LeftSetup must stay false when the escalation never ran")
	}

	if res.Method != "http" {
		t.Errorf("Method = %q, want http (the HTTP call is what wrote the account)", res.Method)
	}
}

// TestPairAccount_UnreadableNowPlayingSkipsEscalation covers firmware that
// does not answer /now_playing. Not being able to check is not evidence of
// being stuck, so nothing should escalate on the strength of it.
func TestPairAccount_UnreadableNowPlayingSkipsEscalation(t *testing.T) {
	d := newFakeDevice(t)
	d.nowPlayingSources = nil

	var dialled bool

	m := &Manager{
		HTTPGet: func(url string) (*http.Response, error) {
			if strings.HasSuffix(url, "/now_playing") {
				return nil, errors.New("connection reset")
			}

			return http.Get(url) //nolint:gosec,noctx // test-local httptest URL
		},
		NewSession: func(_, _ string, _ time.Duration) (StateMachine, error) {
			dialled = true
			return &fakeSession{errors: map[string]error{}}, nil
		},
	}

	res, logs, err := m.PairAccount(t.Context(), d.addr, "1234567", nil)
	if err != nil {
		t.Fatalf("PairAccount: %v", err)
	}

	if dialled || res.StuckInSetup || res.WSAttempted {
		t.Errorf("must not escalate when the SETUP state is unknown: %+v", res)
	}

	if !strings.Contains(logs, "SETUP check failed") {
		t.Errorf("logs should note the failed check, got: %s", logs)
	}
}
