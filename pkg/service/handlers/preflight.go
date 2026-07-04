package handlers

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Probe443Result captures the outcome of probing a host on :443.
// Skipped is true when the running HTTPS listener is already on :443
// (in which case the listener itself is the proof of reachability).
// NotApplicable is true when the operator has chosen an HTTP-only
// deployment (configured serverURL is http://...) — speakers migrated
// to that URL never connect to :443, so the iptables/setcap dance
// would only matter for unmigrated speakers falling back to
// streaming.bose.com via DNS hijack. Reason carries a short
// human-readable explanation rendered in the UI.
type Probe443Result struct {
	Skipped       bool
	NotApplicable bool
	Reason        string
	Localhost     ProbeOutcome
	LAN           ProbeOutcome
	LANHost       string
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

	if scheme := schemeOf(serverURL); scheme == "http" {
		return Probe443Result{
			NotApplicable: true,
			Reason: "AfterTouch's configured serverURL is HTTP, so migrated speakers connect over plain HTTP and never use :443. " +
				"The iptables / setcap / reverse-proxy dance is only needed if you also expect unmigrated speakers to fall back to streaming.bose.com via DNS hijack.",
		}
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

// schemeOf returns the lowercased URL scheme of s, or "" if s is empty or
// unparseable. Used to decide whether the :443 reachability check is even
// applicable to the deployment.
func schemeOf(s string) string {
	if s == "" {
		return ""
	}

	u, err := url.Parse(s)
	if err != nil {
		return ""
	}

	return strings.ToLower(u.Scheme)
}

// DeriveHTTPSURL resolves the effective HTTPS URL AfterTouch advertises.
//
// The rules, in order:
//   - a non-empty override wins verbatim (set via --https-server-url /
//     HTTPS_SERVER_URL or the "advanced" field in the web UI; needed for
//     reverse-proxy setups where the public HTTPS endpoint differs).
//   - if the Target Domain is itself an https:// URL, use it as-is: the
//     operator has already named an HTTPS endpoint (host and, if given,
//     port), so we must not second-guess its port.
//   - otherwise derive from the http:// serverURL: same host, https
//     scheme, on httpsPort. This keeps the common single-host case to one
//     setting — change the Target Domain and the HTTPS URL follows.
//   - if serverURL has no usable host (e.g. not configured yet), fall
//     back to the startup default (hostname-based).
func DeriveHTTPSURL(serverURL, override, httpsPort, fallback string) string {
	if strings.TrimSpace(override) != "" {
		return override
	}

	if u, err := url.Parse(serverURL); err == nil && u.Hostname() != "" {
		// Target Domain already points at HTTPS: honour it verbatim
		// (scheme + host + whatever port the operator specified, or none).
		if strings.EqualFold(u.Scheme, "https") {
			return "https://" + u.Host
		}

		if httpsPort != "" {
			return "https://" + net.JoinHostPort(u.Hostname(), httpsPort)
		}

		return "https://" + u.Hostname()
	}

	return fallback
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
	if res.Skipped || res.NotApplicable {
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

	switch {
	case res.LAN.Reachable:
		lines = append(lines, fmt.Sprintf("  - %s:443 (LAN): reachable ✓", res.LANHost))
	case res.LANHost != "":
		lines = append(lines, fmt.Sprintf("  - %s:443 (LAN): %s", res.LANHost, res.LAN.Error))
	default:
		lines = append(lines, "  - LAN: "+res.LAN.Error)
	}

	lines = append(lines,
		"  Speakers will fail with Curl 7 / connection refused until :443 is routed to AfterTouch. Options:",
		"    1. iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port "+strconv.Itoa(httpsListenerPort),
		"    2. setcap cap_net_bind_service=+ep <binary> and pass --https-port=443",
		"    3. reverse proxy (nginx/caddy) terminating TLS on :443",
		"  Caveat: do NOT add the same REDIRECT rule on the OUTPUT chain. That would catch this host's own outbound :443 traffic (browsers, `go install`, `apt-get`) and route it to AfterTouch.",
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
