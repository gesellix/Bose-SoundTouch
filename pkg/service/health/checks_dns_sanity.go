package health

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/miekg/dns"
)

// CheckIDDNSSanity is the registry id of the DNS-interception
// sanity check.
const CheckIDDNSSanity = "dns_sanity"

// DNSStatusFunc reports whether the service's DNS interception
// listener is running and on which UDP bind address (host:port).
// Closure over Server.GetDNSRunning to avoid a hard dependency
// from health onto handlers.
type DNSStatusFunc func() (running bool, bindAddr string)

// ExpectedIPFunc returns the IP this service expects the
// intercepted hostnames to resolve to (i.e. its own LAN IP).
// Returns the empty string when no service URL is configured.
type ExpectedIPFunc func() string

// RegisterDNSSanityCheck registers a check that queries the
// service's own DNS server for each intercepted Bose hostname and
// verifies the answer is the configured service IP. Catches three
// classes of misconfiguration recurring in #94, #218, #269:
//
//  1. DNS server is disabled or didn't bind — speakers using it
//     as their resolver get NXDOMAIN.
//  2. DNS server is bound but answers point at the wrong IP
//     (e.g. operator changed the LAN IP without restarting).
//  3. A subset of intercepted hostnames silently fail to resolve.
func RegisterDNSSanityCheck(r *Registry, statusFn DNSStatusFunc, expectedIPFn ExpectedIPFunc) {
	r.Register(Check{
		ID:    CheckIDDNSSanity,
		Title: "DNS interception resolves Bose hostnames to this service",
		Run: func() []Finding {
			return runDNSSanityCheck(statusFn, expectedIPFn)
		},
	})
}

func runDNSSanityCheck(statusFn DNSStatusFunc, expectedIPFn ExpectedIPFunc) []Finding {
	running, bindAddr := statusFn()
	if !running {
		return []Finding{{
			Severity: SeverityInfo,
			Message:  "DNS interception is not running on this host.",
			Details:  "Speakers using this service as their DNS server would receive no answers for intercepted Bose hostnames. Enable DNS in Settings or set DNS_ENABLED=true if speakers should be redirected via DNS rather than /etc/hosts on the speaker.",
		}}
	}

	expectedIP := expectedIPFn()
	if expectedIP == "" {
		return []Finding{{
			Severity: SeverityWarning,
			Message:  "DNS server is running but no service IP could be resolved.",
			Details:  "Without a known target IP the sanity check can't validate answers. Configure SERVER_URL to a hostname that resolves to this service's LAN IP.",
		}}
	}

	// bindAddr may be "" / ":53" / "0.0.0.0:53" / "[::]:53" —
	// the DNS lib is bound to the wildcard address. Translate
	// that into something we can actually dial from inside the
	// service host. queryTarget is what we send queries to;
	// displayAddr is what we surface to the operator (the
	// originally configured value, which is what they'd see in
	// netstat).
	queryTarget := resolveDNSQueryTarget(bindAddr)

	displayAddr := bindAddr
	if displayAddr == "" {
		displayAddr = "(default)"
	}

	// Query our DNS server for the canonical intercept list and
	// classify the results.
	hostnames := append([]string(nil), discovery.InterceptedBoseHosts...)
	sort.Strings(hostnames)

	mismatches := make([]string, 0)
	unanswered := make([]string, 0)

	for _, host := range hostnames {
		ip, err := queryOwnDNS(queryTarget, host)
		if err != nil {
			unanswered = append(unanswered, host)
			continue
		}

		if ip != expectedIP {
			mismatches = append(mismatches, fmt.Sprintf("%s → %s", host, ip))
		}
	}

	var findings []Finding

	if len(unanswered) > 0 {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"%d intercepted hostname(s) didn't get an answer from the local DNS server: %s.",
				len(unanswered), strings.Join(unanswered, ", "),
			),
			Details: fmt.Sprintf("Bind: %s. Queried: %s. Expected answer: %s. If the DNS server is actually running, the queries above probably failed because the bind interface isn't reachable from inside the service container — check that DNS_BIND_ADDR is dialable from here.", displayAddr, queryTarget, expectedIP),
		})
	}

	if len(mismatches) > 0 {
		findings = append(findings, Finding{
			Severity: SeverityWarning,
			Message: fmt.Sprintf(
				"%d intercepted hostname(s) resolved to an unexpected IP (expected %s).",
				len(mismatches), expectedIP,
			),
			Details: strings.Join(mismatches, "; "),
			ManualCommands: []ManualCommand{{
				Label:   "Verify from the speaker's network:",
				Command: "nslookup " + hostnames[0] + " " + extractHost(queryTarget),
				Hint:    "Run on a host that uses this service as its DNS resolver. Replace the hostname with any other intercepted name to spot-check.",
			}},
		})
	}

	return findings
}

// resolveDNSQueryTarget translates a server-side bind address
// into a host:port we can actually dial from inside the service
// host. Empty / wildcard / port-only forms all collapse to a
// loopback target on the same port (default 53).
func resolveDNSQueryTarget(bindAddr string) string {
	if bindAddr == "" {
		return "127.0.0.1:53"
	}

	host, port, err := net.SplitHostPort(bindAddr)
	if err != nil {
		// Doesn't parse as host:port. Could be ":53" with stray
		// formatting, a bare port, or a bare host.
		switch {
		case strings.HasPrefix(bindAddr, ":"):
			return "127.0.0.1" + bindAddr
		case !strings.ContainsAny(bindAddr, ":."):
			// looks like a bare port number
			if _, atoiErr := dnsPortAtoi(bindAddr); atoiErr == nil {
				return "127.0.0.1:" + bindAddr
			}

			fallthrough
		default:
			return net.JoinHostPort(bindAddr, "53")
		}
	}

	if host == "" || host == "0.0.0.0" || host == "::" {
		return net.JoinHostPort("127.0.0.1", port)
	}

	return bindAddr
}

// dnsPortAtoi is a tiny strconv.Atoi wrapper that also rejects
// values outside 1..65535. Lets resolveDNSQueryTarget tell apart
// "bare port" from "bare hostname containing no colon".
func dnsPortAtoi(s string) (int, error) {
	v := 0

	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit in port")
		}

		v = v*10 + int(c-'0')
		if v > 65535 {
			return 0, fmt.Errorf("port out of range")
		}
	}

	if v == 0 {
		return 0, fmt.Errorf("empty or zero")
	}

	return v, nil
}

// queryOwnDNS issues an A query against the resolved query
// target (host:port). The service's DNS listener handles UDP,
// so we always dial UDP here.
func queryOwnDNS(queryTarget, hostname string) (string, error) {
	c := dns.Client{Timeout: 1 * time.Second}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	m.RecursionDesired = true

	r, _, err := c.Exchange(m, queryTarget)
	if err != nil {
		return "", err
	}

	if r.Rcode != dns.RcodeSuccess {
		return "", fmt.Errorf("rcode %d", r.Rcode)
	}

	for _, ans := range r.Answer {
		if a, ok := ans.(*dns.A); ok && a.A != nil {
			return a.A.String(), nil
		}
	}

	return "", fmt.Errorf("no A record in answer")
}

func extractHost(bindAddr string) string {
	if i := strings.LastIndex(bindAddr, ":"); i >= 0 {
		return bindAddr[:i]
	}

	return bindAddr
}
