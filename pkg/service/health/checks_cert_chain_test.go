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
	got := runCertChainCheck("")
	if len(got) != 0 {
		t.Errorf("expected no findings for empty URL, got %+v", got)
	}
}

func TestCertChain_UnparseableURLWarns(t *testing.T) {
	got := runCertChainCheck("://nope")
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning, got %+v", got)
	}
}

func TestCertChain_UnreachableEndpoint(t *testing.T) {
	// 127.0.0.1:1 refuses; using https:// to force TLS path.
	got := runCertChainCheck("https://127.0.0.1:1/")
	if len(got) != 1 || got[0].Severity != SeverityError {
		t.Fatalf("expected one error for unreachable endpoint, got %+v", got)
	}
}

func TestCertChain_SelfSignedFlagsAndSuggestsInstallCA(t *testing.T) {
	srv := newSelfSignedTLSServer(t)
	defer srv.Close()

	// httptest's TLS URL uses 127.0.0.1; the test cert below uses
	// "127.0.0.1" as SAN, so SNI matches but the chain is
	// self-signed and won't validate against system roots.
	got := runCertChainCheck(srv.URL)
	if len(got) != 1 || got[0].Severity != SeverityWarning {
		t.Fatalf("expected one warning for self-signed cert, got %+v", got)
	}

	if !strings.Contains(got[0].Details, "Issuer") {
		t.Errorf("expected issuer detail, got %q", got[0].Details)
	}

	if len(got[0].ManualCommands) == 0 {
		t.Fatalf("expected at least one manual command, got none")
	}

	cmd := got[0].ManualCommands[0].Command
	if !strings.Contains(cmd, "install-ca") {
		t.Errorf("expected install-ca suggestion for self-signed cert, got %q", cmd)
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

	// Use ecdsa via x509 helpers — but keep it simple with a tiny
	// RSA key from the test. Actually use crypto/rand + ed25519
	// would be cleaner; for parity with stdlib examples, use the
	// built-in helper path.
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
