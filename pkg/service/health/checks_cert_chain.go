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
//   - chain doesn't validate but the served leaf was issued by
//     our own AfterTouch CA → warning with an `install-ca`
//     suggestion (definitive: we checked the signature against
//     our CA, not a Subject==Issuer heuristic).
//   - chain doesn't validate and the served leaf was issued by
//     something else → warning with an `openssl s_client`
//     investigation prompt (foreign chain / reverse proxy /
//     ingress cert).
//   - HTTPS URL not configured → skip silently.
//
// caCertFn returns AfterTouch's own CA leaf certificate (nil if
// unavailable). It's called per check run; the handler-side
// implementation caches the parse via sync.Once so we don't
// re-read the PEM on every poll.
func RegisterCertChainCheck(r *Registry, httpsURLFn func() string, caCertFn func() *x509.Certificate) {
	r.Register(Check{
		ID:    CheckIDCertChain,
		Title: "HTTPS endpoint certificate validates",
		Run: func() []Finding {
			return runCertChainCheck(httpsURLFn(), caCertFn)
		},
	})
}

func runCertChainCheck(httpsURL string, caCertFn func() *x509.Certificate) []Finding {
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

	classification := classifyLeaf(leaf, caCertFn)
	switch classification {
	case leafFromOwnCA:
		hints = append(hints, ManualCommand{
			Label:   "Install AfterTouch's CA on each speaker:",
			Command: "soundtouch-cli --host=<speaker-ip> setup install-ca --service-url=" + httpsURL,
			Hint:    "The served leaf was issued by AfterTouch's own CA (verified by signature). Requires SSH on the speaker. After install, re-run this check.",
		})
	case leafSubjectEqualsIssuer:
		hints = append(hints, ManualCommand{
			Label:   "If this is a self-signed cert from AfterTouch, install its CA on each speaker:",
			Command: "soundtouch-cli --host=<speaker-ip> setup install-ca --service-url=" + httpsURL,
			Hint:    "Heuristic match (Subject == Issuer) — AfterTouch's own CA wasn't loadable, so this is a best guess. If wrong, treat the chain as foreign.",
		})
	default:
		hints = append(hints, ManualCommand{
			Label:   "Investigate the chain manually:",
			Command: fmt.Sprintf("openssl s_client -connect %s -servername %s -showcerts </dev/null", addr, host),
			Hint:    "Run from the same host as the service. Shows the full chain the peer is serving — likely a reverse proxy or ingress cert.",
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

// leafClassification labels how the leaf relates to AfterTouch's
// own CA. Drives the install-ca-vs-openssl suggestion branch.
type leafClassification int

const (
	leafForeign             leafClassification = iota // chain we don't recognise
	leafFromOwnCA                                     // signature verified by AfterTouch's CA
	leafSubjectEqualsIssuer                           // fallback heuristic when CA isn't loadable
)

// classifyLeaf returns leafFromOwnCA when caCertFn returns a CA
// cert that signed `leaf` (verified by CheckSignatureFrom). When
// the CA cert isn't available, falls back to the
// Subject==Issuer heuristic. Anything else is leafForeign.
func classifyLeaf(leaf *x509.Certificate, caCertFn func() *x509.Certificate) leafClassification {
	if leaf == nil {
		return leafForeign
	}

	if caCertFn != nil {
		if ca := caCertFn(); ca != nil {
			if err := leaf.CheckSignatureFrom(ca); err == nil {
				return leafFromOwnCA
			}
		}
	}

	if leaf.Subject.String() == leaf.Issuer.String() {
		return leafSubjectEqualsIssuer
	}

	return leafForeign
}
