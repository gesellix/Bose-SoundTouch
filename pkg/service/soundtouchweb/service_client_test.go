package soundtouchweb

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeTestCA generates a throwaway self-signed CA and returns its PEM path.
func writeTestCA(t *testing.T) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	path := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	return path
}

func TestNewServiceHTTPClientValidCA(t *testing.T) {
	client, err := NewServiceHTTPClient(writeTestCA(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", client.Transport)
	}

	if tr.TLSClientConfig == nil || tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("expected a non-nil RootCAs pool")
	}

	if client.Timeout == 0 {
		t.Fatal("expected a non-zero timeout")
	}
}

func TestNewServiceHTTPClientMissingFile(t *testing.T) {
	if _, err := NewServiceHTTPClient(filepath.Join(t.TempDir(), "absent.crt")); err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestNewServiceHTTPClientNoCertInFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.crt")
	if err := os.WriteFile(path, []byte("not a pem certificate"), 0o600); err != nil {
		t.Fatalf("write junk: %v", err)
	}

	if _, err := NewServiceHTTPClient(path); err == nil {
		t.Fatal("expected an error for a file with no certificate")
	}
}

func TestHostOnly(t *testing.T) {
	cases := map[string]string{
		"http://192.168.178.35:8090": "192.168.178.35",
		"https://soundtouch.local":   "soundtouch.local",
		"192.168.178.35:8090":        "192.168.178.35",
		"192.168.178.35":             "192.168.178.35",
		"":                           "",
	}

	for in, want := range cases {
		if got := hostOnly(in); got != want {
			t.Errorf("hostOnly(%q) = %q, want %q", in, got, want)
		}
	}
}
