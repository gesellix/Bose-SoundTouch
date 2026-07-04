package handlers

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestProbeTCP_OpenPortSucceeds(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	if err := ProbeTCP("127.0.0.1", port, 500*time.Millisecond); err != nil {
		t.Errorf("expected probe of open port to succeed, got: %v", err)
	}
}

func TestProbeTCP_ClosedPortFails(t *testing.T) {
	// Bind, capture port, close — leaves the port verifiably unbound.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start listener: %v", err)
	}

	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	if err := ProbeTCP("127.0.0.1", port, 500*time.Millisecond); err == nil {
		t.Errorf("expected probe of closed port to fail, got nil")
	}
}

func TestCheck443Reachability_SkipsWhenListenerOn443(t *testing.T) {
	res := Check443Reachability(443, "http://example.test:8000", func(string) (string, error) {
		t.Errorf("resolver should not be called when listener is on :443")
		return "", nil
	}, 100*time.Millisecond)

	if !res.Skipped {
		t.Errorf("expected Skipped=true when httpsListenerPort=443, got %+v", res)
	}
}

func TestCheck443Reachability_ReportsResolverError(t *testing.T) {
	// Use an HTTPS server URL so the NotApplicable short-circuit doesn't fire.
	res := Check443Reachability(8443, "https://broken", func(string) (string, error) {
		return "", errResolve("no DNS")
	}, 100*time.Millisecond)

	if res.Skipped {
		t.Errorf("expected Skipped=false, got true")
	}

	if res.LAN.Reachable {
		t.Errorf("expected LAN.Reachable=false, got true")
	}

	if !strings.Contains(res.LAN.Error, "cannot resolve LAN target") {
		t.Errorf("expected LAN.Error to wrap resolver failure, got %q", res.LAN.Error)
	}
}

func TestCheck443Reachability_NotApplicableWhenServerURLIsHTTP(t *testing.T) {
	res := Check443Reachability(8443, "http://aftertouch.local:8000", func(string) (string, error) {
		t.Errorf("resolver should not be called when serverURL scheme is HTTP")
		return "", nil
	}, 100*time.Millisecond)

	if !res.NotApplicable {
		t.Errorf("expected NotApplicable=true when serverURL is HTTP, got %+v", res)
	}

	if res.Reason == "" {
		t.Errorf("expected NotApplicable verdict to carry a human-readable Reason, got empty")
	}

	if res.Skipped {
		t.Errorf("Skipped should only be set when the listener is already on :443; got Skipped=true for HTTP serverURL")
	}
}

func TestCheck443Reachability_NotApplicableTakesPrecedenceOverProbe(t *testing.T) {
	// Even if the listener isn't on :443 and probes would fail, an HTTP
	// serverURL should short-circuit to NotApplicable.
	res := Check443Reachability(8443, "http://1.2.3.4:8000", func(string) (string, error) {
		return "1.2.3.4", nil
	}, 100*time.Millisecond)

	if !res.NotApplicable {
		t.Errorf("expected NotApplicable=true, got %+v", res)
	}

	if res.LAN.Error != "" || res.Localhost.Error != "" {
		t.Errorf("expected no probe errors when NotApplicable short-circuits, got %+v", res)
	}
}

func TestCheck443Reachability_HTTPSServerURLStillProbes(t *testing.T) {
	resolverCalled := false
	res := Check443Reachability(8443, "https://aftertouch.local:8443", func(string) (string, error) {
		resolverCalled = true
		return "127.0.0.1", nil
	}, 100*time.Millisecond)

	if !resolverCalled {
		t.Errorf("resolver should be called for HTTPS serverURL")
	}

	if res.NotApplicable {
		t.Errorf("HTTPS serverURL should not produce NotApplicable, got %+v", res)
	}
}

func TestPortFromHTTPSServerURL(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"https://example.test:8443", 8443},
		{"https://example.test:443", 443},
		{"https://example.test", 0},
		{":::not a url", 0},
	}

	for _, tc := range cases {
		got := PortFromHTTPSServerURL(tc.in)
		if got != tc.want {
			t.Errorf("PortFromHTTPSServerURL(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestDeriveHTTPSURL(t *testing.T) {
	const fallback = "https://myhost:8443"

	cases := []struct {
		name                           string
		serverURL, override, httpsPort string
		want                           string
	}{
		{"override wins verbatim", "http://192.0.2.10:8000", "https://proxy.example:443", "8443", "https://proxy.example:443"},
		{"override wins over https serverURL", "https://192.0.2.10:9000", "https://proxy.example", "8443", "https://proxy.example"},
		{"http derives to https on https port", "http://192.0.2.10:8000", "", "8443", "https://192.0.2.10:8443"},
		{"http host without port still derives on https port", "http://192.0.2.10", "", "8443", "https://192.0.2.10:8443"},
		// The Target Domain is already HTTPS: honour it verbatim, do not
		// re-impose the configured https port (issue #355 follow-up).
		{"https serverURL with port kept as-is", "https://192.0.2.10:8443", "", "8443", "https://192.0.2.10:8443"},
		{"https serverURL with custom port kept as-is", "https://192.0.2.10:9000", "", "8443", "https://192.0.2.10:9000"},
		{"https serverURL without port kept as-is", "https://192.0.2.10", "", "8443", "https://192.0.2.10"},
		{"empty serverURL falls back", "", "", "8443", fallback},
		{"unparseable serverURL falls back", "://nope", "", "8443", fallback},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveHTTPSURL(tc.serverURL, tc.override, tc.httpsPort, fallback)
			if got != tc.want {
				t.Errorf("DeriveHTTPSURL(%q, %q, %q) = %q, want %q", tc.serverURL, tc.override, tc.httpsPort, got, tc.want)
			}
		})
	}
}

func TestFormatPreflightGuidance_SkippedAndAllOK(t *testing.T) {
	if FormatPreflightGuidance(443, Probe443Result{Skipped: true}) != "" {
		t.Errorf("expected empty guidance when skipped")
	}

	bothOK := Probe443Result{
		Localhost: ProbeOutcome{Reachable: true},
		LAN:       ProbeOutcome{Reachable: true},
		LANHost:   "10.0.0.1",
	}
	if FormatPreflightGuidance(8443, bothOK) != "" {
		t.Errorf("expected empty guidance when both probes succeed")
	}

	if FormatPreflightGuidance(8443, Probe443Result{NotApplicable: true, Reason: "HTTP only"}) != "" {
		t.Errorf("expected empty guidance when NotApplicable (UI renders the reason separately)")
	}
}

func TestFormatPreflightGuidance_BothFailMentionsRedirectPort(t *testing.T) {
	res := Probe443Result{
		Localhost: ProbeOutcome{Error: "connection refused"},
		LAN:       ProbeOutcome{Error: "connection refused"},
		LANHost:   "192.0.2.151",
	}

	out := FormatPreflightGuidance(8443, res)
	if !strings.Contains(out, "--to-port 8443") {
		t.Errorf("guidance must reference configured listener port for iptables, got: %s", out)
	}

	if !strings.Contains(out, "192.0.2.151:443") {
		t.Errorf("guidance must mention probed LAN host, got: %s", out)
	}

	if !strings.Contains(out, "[WARN]") {
		t.Errorf("guidance must be marked as a warning, got: %s", out)
	}
}

func TestFormatPreflightGuidance_IncludesOutputChainCaveat(t *testing.T) {
	res := Probe443Result{
		Localhost: ProbeOutcome{Error: "connection refused"},
		LAN:       ProbeOutcome{Error: "connection refused"},
		LANHost:   "192.0.2.151",
	}

	out := FormatPreflightGuidance(8443, res)
	if !strings.Contains(out, "OUTPUT") {
		t.Errorf("guidance must warn about the iptables OUTPUT chain side-effect, got: %s", out)
	}
}

type errResolve string

func (e errResolve) Error() string { return string(e) }

func TestCheck443Reachability_LANProbeMatchesListenerOutcome(t *testing.T) {
	// Spin up a listener on a random port and use that port via resolver
	// trickery: we point the LAN host at 127.0.0.1 and rely on the fact that
	// nothing answers on :443 in test environments. The point of this test
	// is to lock in the result-shape: when localhost:443 is closed (the
	// default in CI), the function still returns a well-formed result and
	// reports the resolved LAN host. Uses HTTPS so the NotApplicable
	// short-circuit doesn't fire.
	res := Check443Reachability(8443, "https://1.2.3.4:8443", func(string) (string, error) {
		return "1.2.3.4", nil
	}, 200*time.Millisecond)

	if res.Skipped {
		t.Fatalf("expected Skipped=false, got true")
	}

	if res.LANHost != "1.2.3.4" {
		t.Errorf("expected LANHost=1.2.3.4, got %q", res.LANHost)
	}

	// In any sane CI environment nothing is listening on :443, so both
	// probes should report errors. We don't assert the exact error string
	// (varies by OS) but we do assert it's populated.
	if res.LAN.Reachable {
		t.Errorf("did not expect LAN:443 to be reachable in test env")
	}

	if res.LAN.Error == "" {
		t.Errorf("expected LAN.Error to be populated when unreachable")
	}
}
