package health

import (
	"fmt"
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

	// Query our DNS server for the canonical intercept list and
	// classify the results.
	hostnames := append([]string(nil), discovery.InterceptedBoseHosts...)
	sort.Strings(hostnames)

	mismatches := make([]string, 0)
	unanswered := make([]string, 0)

	for _, host := range hostnames {
		ip, err := queryOwnDNS(bindAddr, host)
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
			Details: fmt.Sprintf("Queried %s. Expected: %s. Check the dns server logs in the Logs tab for shouldIntercept misses.", bindAddr, expectedIP),
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
				Command: "nslookup " + hostnames[0] + " " + extractHost(bindAddr),
				Hint:    "Run on a host that uses this service as its DNS resolver. Replace the hostname with any other intercepted name to spot-check.",
			}},
		})
	}

	return findings
}

// queryOwnDNS issues an A query against bindAddr (host:port). The
// service's DNS listener is UDP-only at the listener layer, so we
// always dial UDP here.
func queryOwnDNS(bindAddr, hostname string) (string, error) {
	c := dns.Client{Timeout: 1 * time.Second}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(hostname), dns.TypeA)
	m.RecursionDesired = true

	// Loopback substitution: a bind of 0.0.0.0:53 means "all
	// interfaces" — we can't dial that, so resolve to 127.0.0.1
	// instead.
	addr := bindAddr
	if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	} else if strings.HasPrefix(addr, "[::]:") {
		addr = "[::1]:" + strings.TrimPrefix(addr, "[::]:")
	}

	r, _, err := c.Exchange(m, addr)
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
