package health

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
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
//   - endpoint not reachable from inside the service (no cert
//     presented) → warning, not error: the advertised HTTPS URL
//     is often intentionally unreachable from the service itself
//     (reverse proxy, Docker-published port, LAN-only hostname).
//   - HTTPS URL not configured → skip silently.
//
// caCertFn returns AfterTouch's own CA leaf certificate (nil if
// unavailable). It's called per check run; the handler-side
// implementation caches the parse via sync.Once so we don't
// re-read the PEM on every poll.
func RegisterCertChainCheck(r *Registry, httpsURLFn func() string, caCertFn func() *x509.Certificate) {
	r.Register(Check{
		ID:    CheckIDCertChain,
		Title: "HTTPS endpoint TLS configuration",
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

	// Phase 2: extract the leaf cert from Phase 1's verification
	// error. crypto/x509 attaches the offending certificate to the
	// three verification-failure error types, which lets us inspect
	// what the server actually served without ever opening a second
	// connection with InsecureSkipVerify. If the error is something
	// else (timeout, TCP reset, protocol mismatch), report it as a
	// reachability problem.
	leaf := leafFromVerifyError(err)
	if leaf == nil {
		// The dial failed before any certificate was presented
		// (connection refused, timeout, handshake reset). From
		// inside the service we can't tell "the endpoint is down"
		// apart from "the advertised HTTPS URL simply isn't
		// reachable from here" — the latter is a normal, healthy
		// setup (TLS terminated by a reverse proxy, or a
		// Docker-published port / external hostname that only
		// resolves on the LAN). Reporting a hard error there is a
		// false alarm (issue #355), so this is a warning with the
		// context to tell the two apart.
		return []Finding{{
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("Configured HTTPS endpoint %s isn't reachable from inside the service.", addr),
			Details: fmt.Sprintf("AfterTouch dialed its own configured HTTPS URL (%s) and the connection failed before any certificate was presented: %v. "+
				"This is expected — and not a problem — if TLS is terminated by a reverse proxy in front of AfterTouch, or the advertised HTTPS URL isn't reachable from inside the container (for example a Docker-published port, or a hostname that only resolves elsewhere on your network). In that case, verify the endpoint from a client instead (see below). "+
				"If AfterTouch is meant to serve HTTPS directly, check that the HTTPS listener is bound and that the URL host:port is correct.", httpsURL, err),
			ManualCommands: []ManualCommand{{
				Label:   "Verify the endpoint from a machine on your network:",
				Command: fmt.Sprintf("openssl s_client -connect %s -servername %s </dev/null", addr, host),
				Hint:    "Run from a client that reaches the advertised URL (not necessarily the service host). A successful handshake there means the endpoint is fine and this warning is expected for your setup.",
			}},
		}}
	}

	dnsNames := strings.Join(leaf.DNSNames, ", ")
	if dnsNames == "" {
		dnsNames = "(none)"
	}

	chainContext := fmt.Sprintf(
		"Leaf subject: %s. Issuer: %s. SANs: %s. Expires: %s.",
		leaf.Subject.String(), leaf.Issuer.String(), dnsNames, leaf.NotAfter.Format("2006-01-02"),
	)

	switch classifyLeaf(leaf, caCertFn) {
	case leafFromOwnCA:
		return []Finding{{
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("AfterTouch is serving its own self-signed CA chain on %s (expected).", addr),
			Details: "The service host's system trust store doesn't include AfterTouch's CA — by design. Speakers establish trust via `setup install-ca`, not via system roots. This finding is informational; nothing is wrong with the service. " +
				chainContext,
			ManualCommands: []ManualCommand{{
				Label:   "Reminder — each speaker still needs AfterTouch's CA installed once:",
				Command: "soundtouch-cli --host=<speaker-ip> setup install-ca --service-url=" + httpsURL,
				Hint:    "Verified by signature: the leaf was issued by AfterTouch's own CA. Only run install-ca for speakers that haven't been migrated yet.",
			}},
		}}

	case leafSubjectEqualsIssuer:
		return []Finding{{
			Severity: SeverityInfo,
			Message:  fmt.Sprintf("HTTPS endpoint on %s is serving a self-signed certificate.", addr),
			Details: "AfterTouch's own CA couldn't be loaded to verify the leaf's signature, so this is a heuristic match (Subject == Issuer). If this *is* AfterTouch's self-signed chain, the situation is normal and speakers trust it via `setup install-ca`. If it's some other self-signed cert (custom proxy, etc.), treat the openssl investigation command below as the primary action. " +
				chainContext,
			ManualCommands: []ManualCommand{
				{
					Label:   "If this is AfterTouch's CA, install it on each speaker:",
					Command: "soundtouch-cli --host=<speaker-ip> setup install-ca --service-url=" + httpsURL,
					Hint:    "Heuristic match — verify the served Issuer matches AfterTouch's CA before running.",
				},
				{
					Label:   "Or inspect the served chain manually:",
					Command: fmt.Sprintf("openssl s_client -connect %s -servername %s -showcerts </dev/null", addr, host),
					Hint:    "Run from the same host as the service.",
				},
			},
		}}

	default:
		return []Finding{{
			Severity: SeverityWarning,
			Message:  fmt.Sprintf("HTTPS endpoint on %s serves a chain that doesn't validate against system roots and wasn't issued by AfterTouch's CA.", addr),
			Details: fmt.Sprintf("Unexpected chain — likely a reverse proxy or ingress cert. Verification error: %v. ", err) +
				chainContext,
			ManualCommands: []ManualCommand{{
				Label:   "Inspect the chain manually:",
				Command: fmt.Sprintf("openssl s_client -connect %s -servername %s -showcerts </dev/null", addr, host),
				Hint:    "Run from the same host as the service. Shows the full chain the peer is serving.",
			}},
		}}
	}
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

// leafFromVerifyError extracts the offending certificate that
// crypto/tls attaches to a verification-failure error. Returns
// nil for non-verification errors (network unreachable, timeout,
// TLS-level handshake failure before any cert was presented).
//
// This is the substitute for an InsecureSkipVerify re-dial: the
// same leaf is reachable from the failed strict-dial error
// without ever standing up a connection that disables cert
// validation.
//
// tls.CertificateVerificationError carries
// UnverifiedCertificates across platforms (the underlying error
// is platform-specific — on darwin it comes from
// Security.framework, on linux from crypto/x509 — but the
// wrapping type is the same). The x509.* unwraps are a
// defensive fallback for the case where a caller hands us a
// verification error that bypassed the tls layer.
func leafFromVerifyError(err error) *x509.Certificate {
	var certErr *tls.CertificateVerificationError
	if errors.As(err, &certErr) && len(certErr.UnverifiedCertificates) > 0 {
		return certErr.UnverifiedCertificates[0]
	}

	var unkAuth x509.UnknownAuthorityError
	if errors.As(err, &unkAuth) {
		return unkAuth.Cert
	}

	var hostnameErr x509.HostnameError
	if errors.As(err, &hostnameErr) {
		return hostnameErr.Certificate
	}

	var invalidErr x509.CertificateInvalidError
	if errors.As(err, &invalidErr) {
		return invalidErr.Cert
	}

	return nil
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
