package health

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

// CheckIDCertChain is the registry id of the cert-chain probe.
const CheckIDCertChain = "service_cert_chain"

// RegisterCertChainCheck registers a check that dials the
// configured HTTPS endpoint and reports whether its certificate
// chain validates against the system trust store. Three outcomes:
//
//   - validates against system roots → no finding (the
//     speaker's firmware ships with the major roots, so a public-
//     CA chain such as Let's Encrypt is usable directly).
//   - chain doesn't validate → warning, with a note that
//     `install-ca` is the fix when AfterTouch is using its own
//     self-signed CA, or "review the proxy / ingress cert" when
//     the chain looks foreign.
//   - HTTPS URL not configured → skip silently.
//
// httpsURLFn is a closure so config changes are picked up at run
// time (today only at restart, but cheap to keep flexible).
func RegisterCertChainCheck(r *Registry, httpsURLFn func() string) {
	r.Register(Check{
		ID:    CheckIDCertChain,
		Title: "HTTPS endpoint certificate validates",
		Run: func() []Finding {
			return runCertChainCheck(httpsURLFn())
		},
	})
}

func runCertChainCheck(httpsURL string) []Finding {
	if strings.TrimSpace(httpsURL) == "" {
		return nil
	}

	host, port := splitHTTPSHostPort(httpsURL)
	if host == "" {
		return []Finding{{
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("Configured HTTPS URL %q is not parseable.", httpsURL),
		}}
	}

	addr := net.JoinHostPort(host, port)

	dialer := &net.Dialer{Timeout: 2 * time.Second}

	// Phase 1: try with the system trust store. ServerName is set
	// from the URL so the verifier checks SAN coverage too.
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	})
	if err == nil {
		_ = conn.Close()
		return nil // validates against system roots
	}

	// Phase 2: re-dial with InsecureSkipVerify so we can read the
	// chain and report what was actually served.
	insecureConn, insecureErr := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS12,
	})
	if insecureErr != nil {
		return []Finding{{
			Severity: SeverityError,
			Message:  fmt.Sprintf("Could not connect to %s: %v", addr, insecureErr),
			Details:  "AfterTouch's HTTPS endpoint isn't reachable from inside the service. Check that the listener is bound and the URL host:port resolves correctly.",
		}}
	}
	defer func() { _ = insecureConn.Close() }()

	peers := insecureConn.ConnectionState().PeerCertificates
	if len(peers) == 0 {
		return []Finding{{
			Severity: SeverityWarning,
			Message:  "HTTPS endpoint connected but presented no certificates.",
		}}
	}

	leaf := peers[0]

	subject := leaf.Subject.String()
	issuer := leaf.Issuer.String()
	notAfter := leaf.NotAfter.Format("2006-01-02")

	dnsNames := strings.Join(leaf.DNSNames, ", ")
	if dnsNames == "" {
		dnsNames = "(none)"
	}

	details := fmt.Sprintf(
		"Verification error: %v. Leaf subject: %s. Issuer: %s. SANs: %s. Expires: %s.",
		err, subject, issuer, dnsNames, notAfter,
	)

	var hints []ManualCommand

	if leafLooksSelfSigned(leaf) {
		hints = append(hints, ManualCommand{
			Label:   "If this is AfterTouch's built-in CA, install it on each speaker:",
			Command: "soundtouch-cli --host=<speaker-ip> setup install-ca --service-url=" + httpsURL,
			Hint:    "Requires SSH on the speaker. After install, re-run this check.",
		})
	} else {
		hints = append(hints, ManualCommand{
			Label:   "Investigate the chain manually:",
			Command: fmt.Sprintf("openssl s_client -connect %s -servername %s -showcerts </dev/null", addr, host),
			Hint:    "Run from the same host as the service. Shows the full chain the peer is serving.",
		})
	}

	return []Finding{{
		Severity:       SeverityWarning,
		Message:        fmt.Sprintf("HTTPS certificate at %s does not validate against system roots.", addr),
		Details:        details,
		ManualCommands: hints,
	}}
}

func splitHTTPSHostPort(raw string) (string, string) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" {
		return "", ""
	}

	host := u.Hostname()

	port := u.Port()
	if port == "" {
		port = "443"
	}

	return host, port
}

// leafLooksSelfSigned reports whether the leaf certificate's
// Subject and Issuer match — a strong hint that we're looking at
// AfterTouch's own self-signed CA-issued cert rather than a public
// CA chain. This is intentionally a heuristic, not a guarantee.
func leafLooksSelfSigned(leaf *x509.Certificate) bool {
	if leaf == nil {
		return false
	}

	return leaf.Subject.String() == leaf.Issuer.String()
}
