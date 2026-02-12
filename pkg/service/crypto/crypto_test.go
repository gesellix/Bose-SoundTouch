package crypto

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func TestCertificateManager(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "crypto-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := NewCertificateManager(filepath.Join(tempDir, "certs"))

	// Test CA generation
	if err := cm.EnsureCA(); err != nil {
		t.Fatalf("Failed to ensure CA: %v", err)
	}

	if _, err := os.Stat(cm.GetCACertPath()); os.IsNotExist(err) {
		t.Errorf("CA certificate not created")
	}
	if _, err := os.Stat(cm.GetCAKeyPath()); os.IsNotExist(err) {
		t.Errorf("CA key not created")
	}

	// Test loading CA
	caCertPEM, err := os.ReadFile(cm.GetCACertPath())
	if err != nil {
		t.Fatalf("Failed to read CA cert: %v", err)
	}
	block, _ := pem.Decode(caCertPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Errorf("Invalid CA certificate PEM")
	}
	caCert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse CA cert: %v", err)
	}
	if !caCert.IsCA {
		t.Errorf("Generated certificate is not a CA")
	}

	// Test certificate generation
	domains := []string{"streaming.bose.com", "updates.bose.com"}
	certPEM, keyPEM, err := cm.GenerateCertificate(domains)
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	if len(certPEM) == 0 || len(keyPEM) == 0 {
		t.Errorf("Generated certificate or key is empty")
	}

	// Verify generated certificate
	block, _ = pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		t.Errorf("Invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	if cert.Subject.CommonName != domains[0] {
		t.Errorf("Expected CommonName %s, got %s", domains[0], cert.Subject.CommonName)
	}

	// Check DNS names
	if len(cert.DNSNames) != len(domains) {
		t.Errorf("Expected %d DNS names, got %d", len(domains), len(cert.DNSNames))
	}

	// Verify against CA
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	opts := x509.VerifyOptions{
		DNSName: domains[0],
		Roots:   roots,
	}

	if _, err := cert.Verify(opts); err != nil {
		t.Errorf("Failed to verify certificate against CA: %v", err)
	}

	// Test GetServerTLSConfig
	tlsConfig, err := cm.GetServerTLSConfig(domains)
	if err != nil {
		t.Fatalf("Failed to get TLS config: %v", err)
	}

	if tlsConfig == nil {
		t.Fatal("TLS config is nil")
	}

	if len(tlsConfig.Certificates) == 0 {
		t.Fatal("TLS config has no certificates")
	}

	// Test certificate regeneration if domains change
	newDomains := append(domains, "mac.fritz.box")
	tlsConfig2, err := cm.GetServerTLSConfig(newDomains)
	if err != nil {
		t.Fatalf("Failed to get updated TLS config: %v", err)
	}
	if len(tlsConfig2.Certificates[0].Leaf.DNSNames) < 3 {
		// Note: tls.LoadX509KeyPair doesn't populate Leaf by default.
		// We should parse it manually or rely on the file existence/content.
		certBytes, _ := os.ReadFile(cm.GetServerCertPEMPath())
		block, _ := pem.Decode(certBytes)
		cert, _ := x509.ParseCertificate(block.Bytes)
		found := false
		for _, d := range cert.DNSNames {
			if d == "mac.fritz.box" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Regenerated certificate does not contain new domain")
		}
	}
}
