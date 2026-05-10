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

func happyResponses(targetURL string) map[string]string {
	return map[string]string{
		"sys configuration bmxRegistryUrl " + targetURL + "/bmx/registry/v1/services":   "OK\n",
		"sys configuration statsServerUrl " + targetURL:                                 "OK\n",
		"sys configuration margeServerUrl " + targetURL:                                 "OK\n",
		"sys configuration swUpdateUrl " + targetURL + "/updates/soundtouch":            "OK\n",
		"envswitch boseurls set " + targetURL + " " + targetURL + "/updates/soundtouch": "OK\n",
		"getpdo CurrentSystemConfiguration":                                             "margeServerUrl=" + targetURL + "\nbmxRegistryUrl=" + targetURL + "/bmx/registry/v1/services\n",
	}
}

func TestMigrateViaTelnet_HappyPath(t *testing.T) {
	target := "http://example:8000"
	f := &fakeTelnet{
		banner:    "BoseShell\n-> ",
		responses: happyResponses(target),
	}
	m := newFakeTelnetManager(f)

	logs, err := m.migrateViaTelnet("192.0.2.1", target)
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

	if !strings.Contains(logs, "succeeded") {
		t.Errorf("logs missing success marker:\n%s", logs)
	}

	if !strings.Contains(logs, "BoseShell") {
		t.Errorf("logs missing banner echo:\n%s", logs)
	}
}

func TestMigrateViaTelnet_DialFailureReturnsError(t *testing.T) {
	f := &fakeTelnet{dialErr: errors.New("connection refused")}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", "http://example:8000")
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

	_, err := m.migrateViaTelnet("192.0.2.1", target)
	if err == nil {
		t.Fatal("expected error when envswitch is rejected, got nil")
	}

	if !strings.Contains(err.Error(), "envswitch") {
		t.Errorf("err = %v, want to mention the rejected command", err)
	}

	// The verification command must NOT have been sent — the run aborts on
	// the first rejection.
	for _, c := range f.commands {
		if c == "getpdo CurrentSystemConfiguration" {
			t.Errorf("verification was sent after a rejected command: %v", f.commands)
		}
	}
}

func TestMigrateViaTelnet_VerifyMismatchFails(t *testing.T) {
	target := "http://example:8000"
	resp := happyResponses(target)
	// Device echoes the OLD URLs (envswitch/sys configuration silently dropped).
	resp["getpdo CurrentSystemConfiguration"] = "margeServerUrl=https://streaming.bose.com\n"

	f := &fakeTelnet{responses: resp}
	m := newFakeTelnetManager(f)

	_, err := m.migrateViaTelnet("192.0.2.1", target)
	if err == nil {
		t.Fatal("expected verification mismatch error, got nil")
	}

	if !strings.Contains(err.Error(), "verification failed") {
		t.Errorf("err = %v, want to mention verification failure", err)
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

	_, err := m.migrateViaTelnet("192.0.2.1", target)
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}

	if !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("err = %v, want to wrap broken pipe", err)
	}
}

func TestMigrateViaTelnet_MissingNewTelnetIsClearError(t *testing.T) {
	m := &Manager{ServerURL: "http://example:8000"} // NewTelnet deliberately nil

	_, err := m.migrateViaTelnet("192.0.2.1", "http://example:8000")
	if err == nil {
		t.Fatal("expected error when NewTelnet is nil")
	}

	if !strings.Contains(err.Error(), "NewTelnet") {
		t.Errorf("err = %v, want a configuration error mentioning NewTelnet", err)
	}
}
