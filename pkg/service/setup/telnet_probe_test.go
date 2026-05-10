package setup

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeRegistrar is a deterministic ProbeRegistrar for unit tests. It
// exposes the channel it returned from Register so the test can
// signal it manually to simulate the device's outbound landing on our
// service.
type fakeRegistrar struct {
	mu         sync.Mutex
	channels   map[string]chan struct{}
	registered []string
	forgotten  []string
}

func newFakeRegistrar() *fakeRegistrar {
	return &fakeRegistrar{channels: map[string]chan struct{}{}}
}

func (r *fakeRegistrar) Register(token string) <-chan struct{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	ch := make(chan struct{})
	r.channels[token] = ch
	r.registered = append(r.registered, token)
	return ch
}

func (r *fakeRegistrar) Forget(token string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.channels, token)
	r.forgotten = append(r.forgotten, token)
}

// fire closes the channel for the most-recently-registered token so
// the orchestrator's select wakes.
func (r *fakeRegistrar) fire() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.registered) == 0 {
		return
	}
	last := r.registered[len(r.registered)-1]
	ch, ok := r.channels[last]
	if !ok {
		return
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

// telnetProbeManager builds a Manager pre-wired for probe tests:
// fakeTelnet supplies getpdo and sys configuration responses, and
// HTTPGet is overridden so the :8090/swUpdateCheck trigger doesn't
// reach out to anything real. The httptest server simulates the
// device's swUpdateCheck so we observe the request landing.
func telnetProbeManager(ft *fakeTelnet, onTrigger func()) *Manager {
	m := &Manager{
		ServerURL: "http://example:8000",
		NewTelnet: func(string) TelnetClient { return ft },
		HTTPGet: func(url string) (*http.Response, error) {
			if onTrigger != nil {
				onTrigger()
			}
			rr := httptest.NewRecorder()
			rr.WriteHeader(200)
			return rr.Result(), nil
		},
	}
	return m
}

func TestRunTelnetRoundTripProbe_HappyPath(t *testing.T) {
	target := "http://example:8000"
	ft := &fakeTelnet{
		responses: map[string]string{
			"getpdo CurrentSystemConfiguration": `swUpdateUrl {
  text: "https://worldwide.bose.com/updates/soundtouch"
}
`,
		},
	}
	registrar := newFakeRegistrar()

	// The :8090 trigger should cause the device to fan out to the
	// probe URL. In the test we simulate by closing the channel from
	// the trigger goroutine.
	m := telnetProbeManager(ft, func() { registrar.fire() })

	// fakeTelnet returns "Command not found\n" for unmapped commands.
	// We need `sys configuration swUpdateUrl …` (any value) to look
	// like a success. Pre-populate the map with the canonical happy
	// response — the test will fill in the actual command after
	// generateProbeToken runs, but we can pattern-match instead.
	// Trick: keep the responses map empty for the set command and
	// override the fakeTelnet behaviour.
	ft.responses = map[string]string{
		"getpdo CurrentSystemConfiguration": `swUpdateUrl {
  text: "https://worldwide.bose.com/updates/soundtouch"
}
`,
	}
	// The set/restore commands aren't in the responses map; the
	// fakeTelnet defaults to "Command not found\n" which would fail
	// the run. Override by injecting an OK response for any command
	// starting with "sys configuration swUpdateUrl ".
	origSendCommand := ft.SendCommand
	_ = origSendCommand // unused — fakeTelnet uses a method, not a field.

	// Use a custom telnet client that returns OK for sys configuration.
	customTelnet := &probeFakeTelnet{
		responses: ft.responses,
	}
	m.NewTelnet = func(string) TelnetClient { return customTelnet }

	result, err := m.RunTelnetRoundTripProbe("192.0.2.1", target, registrar, 2*time.Second)
	if err != nil {
		t.Fatalf("RunTelnetRoundTripProbe: %v", err)
	}

	if !result.Reached {
		t.Errorf("Reached = false, want true")
	}

	if !result.Restored {
		t.Errorf("Restored = false, want true (restore command should have succeeded)")
	}

	if result.OriginalURL != "https://worldwide.bose.com/updates/soundtouch" {
		t.Errorf("OriginalURL = %q, want the captured value", result.OriginalURL)
	}

	if !strings.Contains(result.ProbeURL, "/probe/") {
		t.Errorf("ProbeURL = %q, want a /probe/<token> path", result.ProbeURL)
	}

	if len(registrar.forgotten) != 1 {
		t.Errorf("Forget calls = %d, want 1", len(registrar.forgotten))
	}
}

// probeFakeTelnet returns OK for any "sys configuration swUpdateUrl …"
// command and falls back to the responses map for everything else.
type probeFakeTelnet struct {
	responses map[string]string
	commands  []string
}

func (f *probeFakeTelnet) Dial() error            { return nil }
func (f *probeFakeTelnet) Close() error           { return nil }
func (f *probeFakeTelnet) Probe() (string, error) { return "", nil }
func (f *probeFakeTelnet) SendCommand(cmd string) (string, error) {
	f.commands = append(f.commands, cmd)
	if resp, ok := f.responses[cmd]; ok {
		return resp, nil
	}
	if strings.HasPrefix(cmd, "sys configuration swUpdateUrl ") {
		return "OK\n", nil
	}
	return "Command not found\n", nil
}

func TestRunTelnetRoundTripProbe_TimeoutWhenInboundNeverArrives(t *testing.T) {
	target := "http://example:8000"
	registrar := newFakeRegistrar()

	// Do NOT fire the registrar — simulate the device not making the
	// outbound (e.g. firewall, hung firmware).
	m := telnetProbeManager(nil, nil)
	m.NewTelnet = func(string) TelnetClient {
		return &probeFakeTelnet{
			responses: map[string]string{
				"getpdo CurrentSystemConfiguration": `swUpdateUrl {
  text: "https://worldwide.bose.com/updates/soundtouch"
}
`,
			},
		}
	}

	result, err := m.RunTelnetRoundTripProbe("192.0.2.1", target, registrar, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("expected nil error on timeout, got %v", err)
	}

	if result.Reached {
		t.Errorf("Reached = true, want false (no inbound was fired)")
	}

	if !result.Restored {
		t.Errorf("Restored = false, want true even on the timeout path")
	}
}

func TestRunTelnetRoundTripProbe_AbortsWhenGetpdoMissesSwUpdateURL(t *testing.T) {
	target := "http://example:8000"
	registrar := newFakeRegistrar()
	m := telnetProbeManager(nil, nil)
	m.NewTelnet = func(string) TelnetClient {
		return &probeFakeTelnet{
			responses: map[string]string{
				// No swUpdateUrl key — older firmware variant. We refuse
				// to flip anything because we wouldn't know what to
				// restore to.
				"getpdo CurrentSystemConfiguration": `margeServerUrl {
  text: "https://streaming.bose.com"
}
`,
			},
		}
	}

	_, err := m.RunTelnetRoundTripProbe("192.0.2.1", target, registrar, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when getpdo response has no swUpdateUrl, got nil")
	}

	if !strings.Contains(err.Error(), "swUpdateUrl") {
		t.Errorf("err = %v, want it to mention the missing field", err)
	}
}

func TestRunTelnetRoundTripProbe_AbortsWhenDeviceRejectsSysConfiguration(t *testing.T) {
	target := "http://example:8000"
	registrar := newFakeRegistrar()
	m := telnetProbeManager(nil, nil)
	m.NewTelnet = func(string) TelnetClient {
		return &probeFakeTelnetReject{
			responses: map[string]string{
				"getpdo CurrentSystemConfiguration": `swUpdateUrl {
  text: "https://worldwide.bose.com/updates/soundtouch"
}
`,
			},
		}
	}

	_, err := m.RunTelnetRoundTripProbe("192.0.2.1", target, registrar, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error when device rejects sys configuration, got nil")
	}

	if !strings.Contains(err.Error(), "firmware does not expose") {
		t.Errorf("err = %v, want a firmware-rejection message", err)
	}
}

type probeFakeTelnetReject struct {
	responses map[string]string
}

func (f *probeFakeTelnetReject) Dial() error            { return nil }
func (f *probeFakeTelnetReject) Close() error           { return nil }
func (f *probeFakeTelnetReject) Probe() (string, error) { return "", nil }
func (f *probeFakeTelnetReject) SendCommand(cmd string) (string, error) {
	if resp, ok := f.responses[cmd]; ok {
		return resp, nil
	}
	// Any other command, including sys configuration, is rejected.
	return "Command not found\n", nil
}

func TestRunTelnetRoundTripProbe_DialFailure(t *testing.T) {
	registrar := newFakeRegistrar()
	m := &Manager{
		ServerURL: "http://example:8000",
		NewTelnet: func(string) TelnetClient {
			return &fakeTelnet{dialErr: errors.New("connection refused")}
		},
	}

	_, err := m.RunTelnetRoundTripProbe("192.0.2.1", "http://example:8000", registrar, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected dial error, got nil")
	}

	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("err = %v, want to wrap connection refused", err)
	}
}

func TestRunTelnetRoundTripProbe_InvalidTargetURL(t *testing.T) {
	registrar := newFakeRegistrar()
	m := &Manager{
		ServerURL: "http://example:8000",
		NewTelnet: func(string) TelnetClient { return &fakeTelnet{} },
	}

	_, err := m.RunTelnetRoundTripProbe("192.0.2.1", "not-a-url", registrar, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected error on invalid target URL, got nil")
	}
}
