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
)

// fakeDevice spins up an httptest.Server that pretends to be the SoundTouch
// device's :8090 HTTP API. It records POSTs to /setMargeAccount so tests
// can assert on the body.
type fakeDevice struct {
	srv              *httptest.Server
	addr             string // "host:port" usable as deviceIP
	supportsSetMarge bool
	postStatus       int // status code returned for POST /setMargeAccount
	postDelay        time.Duration
	gotPostBody      string
	margeAccountUUID string // served by /info; empty means "unpaired"
}

func newFakeDevice(t *testing.T) *fakeDevice {
	t.Helper()

	d := &fakeDevice{
		supportsSetMarge: true,
		postStatus:       http.StatusOK,
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

	res, _, err := m.PairAccount(d.addr, "1234567", nil)
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

	res, _, err := m.PairAccount(d.addr, "1234567", f)
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

	res, _, err := m.PairAccount(d.addr, "7654321", f)
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

	res, _, err := m.PairAccount(d.addr, "1234567", f)
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

	_, _, err := m.PairAccount(d.addr, "1234567", nil)
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

	_, _, err := m.PairAccount(d.addr, "1234567", f)
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
		_, _, err := m.PairAccount("127.0.0.1:9999", badID, nil)
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

	_, _, err := m.PairAccount(d.addr, "1234567", f)
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

	accountID, alreadyPaired, _, err := m.EnsureMargeAccountPaired(d.addr, "", f)
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

	accountID, alreadyPaired, _, err := m.EnsureMargeAccountPaired(d.addr, "", nil)
	if err != nil {
		t.Fatalf("EnsureMargeAccountPaired: %v", err)
	}

	if alreadyPaired {
		t.Error("alreadyPaired should be false for an unpaired device")
	}

	if !IsValidAccountID(accountID) {
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

	accountID, alreadyPaired, _, err := m.EnsureMargeAccountPaired(d.addr, "7654321", nil)
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

	_, _, _, err := m.EnsureMargeAccountPaired(d.addr, "not-7-digits", nil)
	if err == nil {
		t.Fatal("expected an error for an invalid --account value")
	}
}

func TestEnsureMargeAccountPaired_PropagatesPairingFailure(t *testing.T) {
	d := newFakeDevice(t)
	d.margeAccountUUID = ""
	d.supportsSetMarge = false

	m := NewManager("", nil, nil)

	_, _, _, err := m.EnsureMargeAccountPaired(d.addr, "1234567", nil)
	if err == nil {
		t.Fatal("expected an error when HTTP pairing is unsupported and no telnet client is given")
	}
}

func TestIsValidAccountID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"1234567", true},
		{"0000000", true},
		{"9999999", true},
		{"", false},
		{"123456", false},
		{"12345678", false},
		{"123456a", false},
		{"-123456", false},
		{" 123456", false},
	}

	for _, tc := range cases {
		if got := IsValidAccountID(tc.in); got != tc.want {
			t.Errorf("IsValidAccountID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestGenerateAccountID_AvoidsCollisions(t *testing.T) {
	id, err := GenerateAccountID(nil)
	if err != nil {
		t.Fatalf("GenerateAccountID(nil): %v", err)
	}

	if !IsValidAccountID(id) {
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
