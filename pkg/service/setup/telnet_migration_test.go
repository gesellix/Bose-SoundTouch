package setup

import (
	"errors"
	"strings"
	"testing"
)

// fakeTelnet is a deterministic TelnetClient for unit tests. The responses
// map keys on the exact command string; the value is what SendCommand
// returns. Commands not in the map return "Command not found\n".
type fakeTelnet struct {
	dialErr   error
	banner    string
	responses map[string]string
	// fail returns this error from SendCommand for the named command.
	fail map[string]error
	// commands records every command actually sent, in order, so tests can
	// assert on sequencing.
	commands []string
}

func (f *fakeTelnet) Dial() error            { return f.dialErr }
func (f *fakeTelnet) Probe() (string, error) { return f.banner, nil }
func (f *fakeTelnet) Close() error           { return nil }

func (f *fakeTelnet) SendCommand(cmd string) (string, error) {
	f.commands = append(f.commands, cmd)

	if err, ok := f.fail[cmd]; ok {
		return "", err
	}

	if resp, ok := f.responses[cmd]; ok {
		return resp, nil
	}

	return "Command not found\n", nil
}

func newFakeTelnetManager(f *fakeTelnet) *Manager {
	m := &Manager{
		ServerURL: "http://example:8000",
		NewTelnet: func(host string) TelnetClient { return f },
	}

	return m
}

func flatGetpdoResponse(urls telnetURLs) string {
	return "margeServerUrl=" + urls.Marge + "\n" +
		"statsServerUrl=" + urls.Stats + "\n" +
		"swUpdateUrl=" + urls.SwUpdate + "\n" +
		"bmxRegistryUrl=" + urls.BmxRegistry + "\n"
}

func protobufGetpdoResponse(urls telnetURLs) string {
	return `margeServerUrl {
  text: "` + urls.Marge + `"
}
statsServerUrl {
  text: "` + urls.Stats + `"
}
swUpdateUrl {
  text: "` + urls.SwUpdate + `"
}
bmxRegistryUrl {
  text: "` + urls.BmxRegistry + `"
}
->OK
`
}

func telnetResponses(urls telnetURLs, verify string) map[string]string {
	return map[string]string{
		"sys configuration bmxRegistryUrl " + urls.BmxRegistry:       "OK\n",
		"sys configuration statsServerUrl " + urls.Stats:             "OK\n",
		"sys configuration margeServerUrl " + urls.Marge:             "OK\n",
		"sys configuration swUpdateUrl " + urls.SwUpdate:             "OK\n",
		"envswitch boseurls set " + urls.Marge + " " + urls.SwUpdate: "Setting Bose Server URLs to " + urls.Marge + " and " + urls.SwUpdate + " ->\n",
		"getpdo CurrentSystemConfiguration":                          verify,
	}
}

func happyResponses(targetURL string) map[string]string {
	urls := defaultTelnetURLs(targetURL)

	return telnetResponses(urls, flatGetpdoResponse(urls))
}

func TestMigrateViaTelnet_HappyPath(t *testing.T) {
	target := "http://example:8000"
	f := &fakeTelnet{
		banner:    "BoseShell\n-> ",
		responses: happyResponses(target),
	}
	m := newFakeTelnetManager(f)

	logs, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs(target))
	if err != nil {
		t.Fatalf("migrateViaTelnet: %v", err)
	}

	wantOrder := []string{
		"sys configuration bmxRegistryUrl " + target + "/bmx/registry/v1/services",
		"sys configuration statsServerUrl " + target,
		"sys configuration margeServerUrl " + target,
		"sys configuration swUpdateUrl " + target + "/updates/soundtouch",
		"envswitch boseurls set " + target + " " + target + "/updates/soundtouch",
		"getpdo CurrentSystemConfiguration",
	}

	if len(f.commands) != len(wantOrder) {
		t.Fatalf("sent %d commands, want %d:\n%v", len(f.commands), len(wantOrder), f.commands)
	}

	for i, want := range wantOrder {
		if f.commands[i] != want {
			t.Errorf("command[%d] = %q, want %q", i, f.commands[i], want)
		}
	}

	if !strings.Contains(logs, "accepted") {
		t.Errorf("logs missing success marker:\n%s", logs)
	}

	if !strings.Contains(logs, "BoseShell") {
		t.Errorf("logs missing banner echo:\n%s", logs)
	}
}

func TestMigrateViaTelnet_AcceptsPromptedOKAfterEnvswitchConfirmation(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	responses := happyResponses(target)
	responses["envswitch boseurls set "+urls.Marge+" "+urls.SwUpdate] =
		"Setting Bose Server URLs to " + urls.Marge + " and " + urls.SwUpdate + "\n->OK\n->"

	f := &fakeTelnet{responses: responses}
	m := newFakeTelnetManager(f)

	if _, err := m.migrateViaTelnet("192.0.2.1", urls); err != nil {
		t.Fatalf("migrateViaTelnet: %v", err)
	}
}

func TestValidateTelnetMutationResponse_PersistenceShapes(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	command := "envswitch boseurls set " + urls.Marge + " " + urls.SwUpdate
	confirmation := "Setting Bose Server URLs to " + urls.Marge + " and " + urls.SwUpdate

	tests := []struct {
		name     string
		response string
		accept   bool
	}{
		{name: "legacy prompt suffix", response: confirmation + " ->\n", accept: true},
		{name: "prompted ok", response: confirmation + "\n->OK\n->", accept: true},
		{name: "bare confirmation", response: confirmation + "\n"},
		{name: "bare ok", response: "OK\n"},
		{name: "reversed order", response: "OK\n" + confirmation + "\n"},
		{name: "duplicate ok", response: confirmation + "\n->OK\n->OK\n->"},
		{name: "legacy plus ok", response: confirmation + " ->\n->OK\n->"},
		{name: "unexpected line", response: confirmation + "\nwarning: not persisted\n->OK\n->"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTelnetMutationResponse(command, tt.response, true, urls)
			if tt.accept && err != nil {
				t.Fatalf("response rejected: %v", err)
			}

			if !tt.accept && err == nil {
				t.Fatal("response accepted")
			}
		})
	}
}

func TestMigrateViaTelnet_RejectsUnsafeURLsBeforeCreatingClient(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{name: "line break", value: "http://example:8000/marge\r\nsys reboot"},
		{name: "space", value: "http://example:8000/not safe"},
		{name: "shell metacharacter", value: "http://example:8000/;sys-reboot"},
		{name: "unsupported scheme", value: "ftp://example:8000/marge"},
		{name: "missing host", value: "http:/marge"},
		{name: "userinfo", value: "http://user:secret@example:8000/marge"},
		{name: "query", value: "http://example:8000/marge?mode=test"},
		{name: "fragment", value: "http://example:8000/marge#section"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			urls := defaultTelnetURLs("http://example:8000")
			urls.Marge = tt.value
			clientCreated := false
			m := &Manager{NewTelnet: func(string) TelnetClient {
				clientCreated = true
				return &fakeTelnet{}
			}}

			_, err := m.migrateViaTelnet("192.0.2.1", urls)
			if !errors.Is(err, ErrInvalidTelnetURL) {
				t.Fatalf("error = %v, want ErrInvalidTelnetURL", err)
			}

			if clientCreated {
				t.Fatal("telnet client was created for rejected URL input")
			}
		})
	}
}

func TestMigrateViaTelnet_AcceptsHTTPAndHTTPSServiceURLs(t *testing.T) {
	for _, target := range []string{
		"http://unifi:8001",
		"HTTPS://example:8443/aftertouch",
		"https://[2001:db8::1]:8443/aftertouch",
	} {
		t.Run(target, func(t *testing.T) {
			urls := defaultTelnetURLs(target)
			f := &fakeTelnet{responses: telnetResponses(urls, flatGetpdoResponse(urls))}
			m := newFakeTelnetManager(f)

			if _, err := m.migrateViaTelnet("192.0.2.1", urls); err != nil {
				t.Fatalf("migrateViaTelnet: %v", err)
			}
		})
	}
}

func TestMigrateViaTelnet_DialFailureReturnsError(t *testing.T) {
	f := &fakeTelnet{dialErr: errors.New("connection refused")}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs("http://example:8000"))
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}

	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("err = %v, want to wrap connection refused", err)
	}

	if len(f.commands) != 0 {
		t.Errorf("expected no commands sent on dial failure, got %v", f.commands)
	}
}

func TestMigrateViaTelnet_CommandNotFoundAborts(t *testing.T) {
	target := "http://example:8000"
	resp := happyResponses(target)
	// The ST20-Portable case: `envswitch` is not implemented.
	delete(resp, "envswitch boseurls set "+target+" "+target+"/updates/soundtouch")

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs(target))
	if err == nil {
		t.Fatal("expected error when envswitch is rejected, got nil")
	}

	if !strings.Contains(err.Error(), "envswitch") {
		t.Errorf("err = %v, want to mention the rejected command", err)
	}

	assertNoTelnetWritesAfterRejection(t, f.commands, "envswitch")
}

func TestMigrateViaTelnet_GenericRuntimeRejectionReportsPartialState(t *testing.T) {
	target := "http://example:8000"
	resp := happyResponses(target)
	resp["sys configuration statsServerUrl "+target] = "NOT OK\n"

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs(target))
	if err == nil {
		t.Fatal("expected generic rejection error, got nil")
	}

	if !strings.Contains(err.Error(), "1 runtime URL write(s) were confirmed") {
		t.Errorf("err = %v, want partial-runtime classification", err)
	}

	for _, command := range f.commands {
		if strings.Contains(command, "margeServerUrl") {
			t.Errorf("migration continued after rejected runtime write: %v", f.commands)
		}
	}
}

func TestMigrateViaTelnet_EnvswitchRejectionReportsUncertainPersistence(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	resp := happyResponses(target)
	resp["envswitch boseurls set "+urls.Marge+" "+urls.SwUpdate] =
		"Not setting Bose Server URLs to " + urls.Marge + " and " + urls.SwUpdate + " ->\n"

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", urls)
	if err == nil {
		t.Fatal("expected envswitch rejection error, got nil")
	}

	if !strings.Contains(err.Error(), "persistence outcome is uncertain") {
		t.Errorf("err = %v, want uncertain-persistence classification", err)
	}

	assertNoTelnetWritesAfterRejection(t, f.commands, "envswitch")
}

// assertNoTelnetWritesAfterRejection checks the property that matters once a
// command is rejected: no further command may CHANGE the device. The
// read-only getpdo read-back is expected, since the failure report says what
// the device actually holds rather than telling the user to go and find out.
func assertNoTelnetWritesAfterRejection(t *testing.T, commands []string, rejected string) {
	t.Helper()

	seenRejected := false
	for _, command := range commands {
		if strings.Contains(command, rejected) {
			seenRejected = true

			continue
		}

		if !seenRejected {
			continue
		}

		if strings.HasPrefix(command, "sys configuration") || strings.HasPrefix(command, "envswitch") {
			t.Errorf("a write was sent after the rejected command: %v", commands)
		}
	}
}

func TestMigrateViaTelnet_VerifyMismatchFails(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	resp := happyResponses(target)
	// The old substring check passed this response because the target still
	// appeared in it, despite one field having the wrong exact value.
	urls.Stats = target + "/wrong"
	resp["getpdo CurrentSystemConfiguration"] = flatGetpdoResponse(urls)

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs(target))
	if err == nil {
		t.Fatal("expected verification mismatch error, got nil")
	}

	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("err = %v, want to mention verification failure", err)
	}

	if !strings.Contains(err.Error(), "persistence may already have changed") {
		t.Errorf("err = %v, want post-envswitch uncertainty", err)
	}
}

func TestMigrateViaTelnet_VerifyProtobufFormatSucceeds(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	f := &fakeTelnet{responses: telnetResponses(urls, protobufGetpdoResponse(urls))}
	m := newFakeTelnetManager(f)

	if _, err := m.migrateViaTelnet("192.0.2.1", urls); err != nil {
		t.Fatalf("migrateViaTelnet: %v", err)
	}
}

func TestMigrateViaTelnet_VerifyMissingFieldFails(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	verify := "margeServerUrl=" + urls.Marge + "\n" +
		"statsServerUrl=" + urls.Stats + "\n" +
		"swUpdateUrl=" + urls.SwUpdate + "\n"
	f := &fakeTelnet{responses: telnetResponses(urls, verify)}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", urls)
	if err == nil {
		t.Fatal("expected verification error for missing bmxRegistryUrl, got nil")
	}

	if !strings.Contains(err.Error(), "missing bmxRegistryUrl") {
		t.Errorf("err = %v, want missing field name", err)
	}
}

func TestMigrateViaTelnet_TransportErrorAborts(t *testing.T) {
	target := "http://example:8000"
	f := &fakeTelnet{
		responses: happyResponses(target),
		fail: map[string]error{
			"sys configuration margeServerUrl " + target: errors.New("write: broken pipe"),
		},
	}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs(target))
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}

	if !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("err = %v, want to wrap broken pipe", err)
	}
}

func TestMigrateViaTelnet_VerificationTransportFailureReportsUncertainPersistence(t *testing.T) {
	target := "http://example:8000"
	f := &fakeTelnet{
		responses: happyResponses(target),
		fail: map[string]error{
			"getpdo CurrentSystemConfiguration": errors.New("read: connection reset"),
		},
	}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs(target))
	if err == nil {
		t.Fatal("expected verification transport error, got nil")
	}

	if !strings.Contains(err.Error(), "persistence may already have changed") {
		t.Errorf("err = %v, want post-envswitch uncertainty", err)
	}
}

func TestMigrateViaTelnet_MissingNewTelnetIsClearError(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"} // NewTelnet deliberately nil

	_, err := m.migrateViaTelnet("192.0.2.1", defaultTelnetURLs("http://example:8000"))
	if err == nil {
		t.Fatal("expected error when NewTelnet is nil")
	}

	if !strings.Contains(err.Error(), "NewTelnet") {
		t.Errorf("err = %v, want a configuration error mentioning NewTelnet", err)
	}
}

// TestTelnetRevertNotOfferedOnUnmigratedSpeakers: offering the revert here
// would rewrite a pristine speaker's genuine factory URLs and commit them
// through envswitch. Telnet migration takes no backup, so the record of what
// those URLs were is then gone.
func TestTelnetRevertNotOfferedOnUnmigratedSpeakers(t *testing.T) {
	getpdo := func(marge, stats, swUpdate, bmx string) string {
		return "margeServerUrl {\n  text: \"" + marge + "\"\n}\n" +
			"statsServerUrl {\n  text: \"" + stats + "\"\n}\n" +
			"swUpdateUrl {\n  text: \"" + swUpdate + "\"\n}\n" +
			"bmxRegistryUrl {\n  text: \"" + bmx + "\"\n}\n->OK\n"
	}

	for _, test := range []struct {
		name     string
		response string
		want     bool
	}{
		{
			// Observed on real hardware, and what canonicalBoseTelnetURLs holds.
			name: "canonical original variant",
			response: getpdo("https://streaming.bose.com", "https://events.api.bosecm.com",
				"https://worldwide.bose.com/updates/soundtouch",
				"https://content.api.bose.io/bmx/registry/v1/services"),
		},
		{
			// The variant pkg/service/testing/fakespeaker models. Comparing
			// against the canonical set alone offered a revert here.
			name: "older original variant",
			response: getpdo("https://streaming.bose.com", "https://stats.bose.com",
				"https://worldwide.bose.com/updates/soundtouch",
				"https://bmxservice.bose.com/bmx/registry/v1/services"),
		},
		{
			name: "original with firmware-normalised casing and trailing slash",
			response: getpdo("https://STREAMING.BOSE.COM/", "https://events.api.bosecm.com",
				"https://worldwide.bose.com/updates/soundtouch",
				"https://content.api.bose.io/bmx/registry/v1/services"),
		},
		{
			name: "migrated to AfterTouch",
			response: getpdo("http://aftertouch.example:8000", "http://aftertouch.example:8000",
				"http://aftertouch.example:8000/updates/soundtouch",
				"http://aftertouch.example:8000/bmx/registry/v1/services"),
			want: true,
		},
		{
			// A single changed field is still a changed device.
			name: "partially migrated",
			response: getpdo("http://aftertouch.example:8000", "https://events.api.bosecm.com",
				"https://worldwide.bose.com/updates/soundtouch",
				"https://content.api.bose.io/bmx/registry/v1/services"),
			want: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := telnetRevertAvailable(test.response); got != test.want {
				t.Errorf("telnetRevertAvailable() = %v, want %v", got, test.want)
			}
		})
	}
}

// TestMigrateViaTelnet_FailureReportsWhatTheDeviceHolds: the failure advice is
// "read back and reconcile all four URL fields", which the service can do
// itself. An unrecognised reply is not proof the write failed, so the readback
// is better evidence than the reply shape.
func TestMigrateViaTelnet_FailureReportsWhatTheDeviceHolds(t *testing.T) {
	target := "http://example:8000"
	urls := defaultTelnetURLs(target)
	resp := happyResponses(target)
	resp["envswitch boseurls set "+urls.Marge+" "+urls.SwUpdate] = "something unfamiliar\n"

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	logs, err := m.migrateViaTelnet("192.0.2.1", urls)
	if err == nil {
		t.Fatal("expected an error for an unrecognised envswitch response")
	}

	if !strings.Contains(err.Error(), "the device currently reports") {
		t.Errorf("err = %v, want it to carry the live URL state", err)
	}
	if !strings.Contains(err.Error(), "margeServerUrl="+target) {
		t.Errorf("err = %v, want the actual margeServerUrl value", err)
	}
	if !strings.Contains(logs, "read-back after the failure") {
		t.Errorf("logs did not record the read-back:\n%s", logs)
	}
}

// TestResyncBoseURLsAfterXML_DistinguishesRejectedURL: applyURLOverrides lets
// the XML path accept URLs the telnet validator refuses, so the XML write
// succeeds while the re-sync is skipped. Reporting that as "could not re-sync
// over telnet" points the user at the wrong thing.
func TestResyncBoseURLsAfterXML_DistinguishesRejectedURL(t *testing.T) {
	m := newFakeTelnetManager(&fakeTelnet{responses: map[string]string{}})

	rejected := m.resyncBoseURLsAfterXML("192.0.2.1", telnetURLs{
		Marge:       "http://example:8000?probe=1",
		Stats:       "http://example:8000",
		SwUpdate:    "http://example:8000/updates/soundtouch",
		BmxRegistry: "http://example:8000/bmx/registry/v1/services",
	})

	if !strings.Contains(rejected, "a URL was rejected") {
		t.Errorf("note = %q, want it to name the rejected URL as the cause", rejected)
	}
	if strings.Contains(rejected, "could not re-sync boseurls over telnet") {
		t.Errorf("note = %q, want it not to blame telnet availability", rejected)
	}
}
