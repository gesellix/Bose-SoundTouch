package handlers

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"time"
)

// Probe443Result captures the outcome of probing a host on :443.
// Skipped is true when the running HTTPS listener is already on :443
// (in which case the listener itself is the proof of reachability).
type Probe443Result struct {
	Skipped   bool
	Localhost ProbeOutcome
	LAN       ProbeOutcome
	LANHost   string
}

// ProbeOutcome describes a single TCP-connect probe. Exactly one of
// Reachable/Error is meaningful: Reachable=true means the dial succeeded,
// otherwise Error holds the dial error string.
type ProbeOutcome struct {
	Reachable bool
	Error     string
}

// ProbeDialTimeoutStartup is the per-attempt TCP dial timeout used by the
// startup preflight, where we can afford to wait a beat for a slow LAN.
const ProbeDialTimeoutStartup = 2 * time.Second

// ProbeDialTimeoutInline is the per-attempt TCP dial timeout used by the
// settings HTTP handler, where a user is blocking on the response.
const ProbeDialTimeoutInline = 500 * time.Millisecond

// ProbeTCP attempts a TCP connection to host:port within timeout. It returns
// nil on success; an error otherwise. The connection is closed immediately —
// we only care whether *something* would answer where a speaker knocks.
func ProbeTCP(host string, port int, timeout time.Duration) error {
	addr := net.JoinHostPort(host, strconv.Itoa(port))

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}

	_ = conn.Close()

	return nil
}

// Check443Reachability probes both localhost:443 and the LAN-facing IP that
// DNS would hand out for serverURL on :443. It is intended to surface the
// most common AfterTouch misconfiguration: HTTPS listener on :8443 with no
// routing in place from :443 (speakers connect to implicit :443 and see
// Curl 7 / connection refused with nothing reaching AfterTouch).
//
// If httpsListenerPort is already 443, both probes are skipped — the running
// listener proves :443 is reachable.
//
// lanResolver is the function used to translate serverURL into a LAN IP; in
// production this is Server.resolveServerURLIP. It is injected so this can
// be tested without a full Server.
func Check443Reachability(
	httpsListenerPort int,
	serverURL string,
	lanResolver func(string) (string, error),
	timeout time.Duration,
) Probe443Result {
	if httpsListenerPort == 443 {
		return Probe443Result{Skipped: true}
	}

	res := Probe443Result{}

	if err := ProbeTCP("127.0.0.1", 443, timeout); err != nil {
		res.Localhost.Error = err.Error()
	} else {
		res.Localhost.Reachable = true
	}

	lanIP, resolveErr := lanResolver(serverURL)
	if resolveErr != nil {
		res.LAN.Error = "cannot resolve LAN target: " + resolveErr.Error()
		return res
	}

	res.LANHost = lanIP

	if err := ProbeTCP(lanIP, 443, timeout); err != nil {
		res.LAN.Error = err.Error()
	} else {
		res.LAN.Reachable = true
	}

	return res
}

// PortFromHTTPSServerURL extracts the numeric port from httpsServerURL. It
// returns 0 if the URL is empty, malformed, or has no explicit port — in
// that case the caller cannot make a determination about :443 and should
// treat the result as "unknown" rather than "definitely not 443".
func PortFromHTTPSServerURL(httpsServerURL string) int {
	if httpsServerURL == "" {
		return 0
	}

	u, err := url.Parse(httpsServerURL)
	if err != nil {
		return 0
	}

	portStr := u.Port()
	if portStr == "" {
		return 0
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}

	return port
}

// FormatPreflightGuidance returns a multi-line, human-readable warning
// summarising a failing Probe443Result, with actionable next steps. The
// returned string ends without a trailing newline so callers may use it
// with log.Print or log.Printf as they prefer.
func FormatPreflightGuidance(httpsListenerPort int, res Probe443Result) string {
	if res.Skipped {
		return ""
	}

	if res.Localhost.Reachable && res.LAN.Reachable {
		return ""
	}

	lines := []string{
		fmt.Sprintf("[WARN] HTTPS pre-flight: speakers connect to :443 but AfterTouch listens on :%d.", httpsListenerPort),
	}

	if res.Localhost.Reachable {
		lines = append(lines, "  - localhost:443: reachable ✓")
	} else {
		lines = append(lines, "  - localhost:443: "+res.Localhost.Error)
	}

	if res.LAN.Reachable {
		lines = append(lines, fmt.Sprintf("  - %s:443 (LAN): reachable ✓", res.LANHost))
	} else if res.LANHost != "" {
		lines = append(lines, fmt.Sprintf("  - %s:443 (LAN): %s", res.LANHost, res.LAN.Error))
	} else {
		lines = append(lines, "  - LAN: "+res.LAN.Error)
	}

	lines = append(lines,
		"  Speakers will fail with Curl 7 / connection refused until :443 is routed to AfterTouch. Options:",
		"    1. iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port "+strconv.Itoa(httpsListenerPort),
		"    2. setcap cap_net_bind_service=+ep <binary> and pass --https-port=443",
		"    3. reverse proxy (nginx/caddy) terminating TLS on :443",
		"  See docs/guides/HTTPS-SETUP.md for details.",
	)

	out := ""

	for i, l := range lines {
		if i > 0 {
			out += "\n"
		}

		out += l
	}

	return out
}
