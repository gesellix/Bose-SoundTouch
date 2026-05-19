package health

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCertChain_EmptyURLSkips(t *testing.T) {
	got := runCertChainCheck("", nil)
	if len(got) != 0 {
		t.Errorf("expected no findings for empty URL, got %+v", got)
	}
}

func TestCertChain_UnparseableURLWarns(t *testing.T) {
	got := runCertChainCheck("://nope", nil)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}
}

func TestCertChain_UnreachableEndpoint(t *testing.T) {
	// 127.0.0.1:1 refuses; using https:// to force TLS path.
	got := runCertChainCheck("https://127.0.0.1:1/", nil)
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("expected one error for unreachable endpoint, got %+v", got)
	}
}

func TestCertChain_SelfSigned_SubjectEqualsIssuerFallback(t *testing.T) {
	srv := newSelfSignedTLSServer(t)
	defer srv.Close()

	// No CA provided → fallback to Subject==Issuer heuristic.
	// This is informational, not a warning — a self-signed
	// AfterTouch chain is the expected default deployment shape.
	got := runCertChainCheck(srv.URL, nil)
	if len(got) != 1 || got[0].Severity != SeverityInfo {
		t.Fatalf("expected one info finding for self-signed cert, got %+v", got)
	}

	if !strings.Contains(got[0].Details, "Issuer") {
		t.Errorf("expected issuer detail, got %q", got[0].Details)
	}

	if !strings.Contains(got[0].Details, "heuristic") {
		t.Errorf("expected heuristic disclosure in details, got %q", got[0].Details)
	}

	if len(got[0].ManualCommands) < 2 {
		t.Fatalf("expected at least install-ca + openssl commands, got %+v", got[0].ManualCommands)
	}

	var sawInstallCA, sawOpenssl bool
	for _, c := range got[0].ManualCommands {
		if strings.Contains(c.Command, "install-ca") {
			sawInstallCA = true
		}
		if strings.Contains(c.Command, "openssl s_client") {
			sawOpenssl = true
		}
	}

	if !sawInstallCA {
		t.Errorf("expected install-ca suggestion among manual commands")
	}
	if !sawOpenssl {
		t.Errorf("expected openssl investigation command among manual commands")
	}
}

func TestCertChain_LeafSignedByOwnCA_IsInformationalNotAWarning(t *testing.T) {
	// AfterTouch's own CA is the *default* deployment shape.
	// Calling that a warning would mislead non-technical users.
	// The check should report INFO, explain the situation, and
	// remind to install-ca on speakers — but not present the
	// service host's lack of trust as a defect.
	caTLS, ca := generateInternalCA(t)
	leafTLS := generateLeafSignedBy(t, ca, caTLS.PrivateKey)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{leafTLS}}
	srv.StartTLS()
	defer srv.Close()

	got := runCertChainCheck(srv.URL, func() *x509.Certificate { return ca })
	if len(got) != 1 {
		t.Fatalf("expected one finding, got %+v", got)
	}

	if got[0].Severity != SeverityInfo {
		t.Errorf("expected SeverityInfo for AfterTouch's own CA chain, got %q", got[0].Severity)
	}

	if !strings.Contains(got[0].Message, "expected") {
		t.Errorf("expected message to call this state 'expected', got %q", got[0].Message)
	}

	if !strings.Contains(got[0].Details, "by design") {
		t.Errorf("expected details to explain it's by design, got %q", got[0].Details)
	}

	if len(got[0].ManualCommands) == 0 {
		t.Fatalf("expected install-ca reminder among manual commands")
	}

	cmd := got[0].ManualCommands[0]
	if !strings.Contains(cmd.Command, "install-ca") {
		t.Errorf("expected install-ca reminder, got %q", cmd.Command)
	}

	if !strings.Contains(cmd.Hint, "Verified by signature") {
		t.Errorf("expected signature-verified hint, got %q", cmd.Hint)
	}
}

func TestCertChain_ForeignChain_SuggestsOpenSSL(t *testing.T) {
	// Build an "external" CA + leaf, then provide a *different*
	// CA via caCertFn. Signature check fails → classifier returns
	// leafForeign → openssl suggestion (since Subject==Issuer
	// would also fail for a properly chained leaf).
	externalCATLS, externalCA := generateInternalCA(t)
	leafTLS := generateLeafSignedBy(t, externalCA, externalCATLS.PrivateKey)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{leafTLS}}
	srv.StartTLS()
	defer srv.Close()

	// Different CA — pretend it's "our" AfterTouch CA.
	_, ourCA := generateInternalCA(t)

	got := runCertChainCheck(srv.URL, func() *x509.Certificate { return ourCA })
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}

	cmd := got[0].ManualCommands[0]
	if !strings.Contains(cmd.Command, "openssl s_client") {
		t.Errorf("expected openssl suggestion for foreign chain, got %q", cmd.Command)
	}
}

func TestSplitHTTPSHostPort(t *testing.T) {
	cases := []struct {
		in, host, port string
	}{
		{"https://example.com/", "example.com", "443"},
		{"https://example.com:8443/", "example.com", "8443"},
		{"https://192.0.2.10/", "192.0.2.10", "443"},
		{"https://", "", ""},
		{"://broken", "", ""},
	}

	for _, c := range cases {
		h, p := splitHTTPSHostPort(c.in)
		if h != c.host || p != c.port {
			t.Errorf("splitHTTPSHostPort(%q) = (%q, %q), want (%q, %q)", c.in, h, p, c.host, c.port)
		}
	}
}

// newSelfSignedTLSServer returns an httptest.Server whose TLS
// config uses a self-signed cert we generate inline. httptest's
// default TLS server uses a built-in cert, but verifying its
// Subject == Issuer property without inspecting innards is
// fiddly; making our own keeps the assertion deterministic.
func newSelfSignedTLSServer(t *testing.T) *httptest.Server {
	t.Helper()

	cert := generateSelfSignedCert(t)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()

	return srv
}

func generateSelfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "aftertouch-test"},
		Issuer:       pkix.Name{CommonName: "aftertouch-test"}, // self-signed
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"127.0.0.1"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	return tls.Certificate{
		Certificate: [][]byte{derBytes},
		PrivateKey:  key,
	}
}

// generateInternalCA returns a self-signed CA suitable for
// signing leaves. The returned tls.Certificate carries the CA
// key (needed to sign leaves below); the *x509.Certificate is
// the parsed CA leaf.
func generateInternalCA(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2026),
		Subject:               pkix.Name{CommonName: "AfterTouch Test CA", Organization: []string{"AfterTouch Test"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	template.Issuer = template.Subject

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create CA cert: %v", err)
	}

	caParsed, err := x509.ParseCertificate(derBytes)
	if err != nil {
		t.Fatalf("parse CA cert: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{derBytes}, PrivateKey: key}, caParsed
}

// generateLeafSignedBy issues a TLS leaf cert (CN=leaf) signed
// by ca/caKey, with SAN 127.0.0.1 so httptest's loopback SNI
// matches. Subject != Issuer by construction — the case that
// caught my old heuristic.
func generateLeafSignedBy(t *testing.T, ca *x509.Certificate, caKey any) tls.Certificate {
	t.Helper()

	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("leaf rsa key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(42),
		Subject:      pkix.Name{CommonName: "soundtouch", Organization: []string{"AfterTouch"}},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"127.0.0.1"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, template, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf cert: %v", err)
	}

	return tls.Certificate{Certificate: [][]byte{derBytes}, PrivateKey: leafKey}
}
