package certmanager

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
	cm.CommonName = "test.local"

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

	if cert.Subject.CommonName != cm.CommonName {
		t.Errorf("Expected CommonName %s, got %s", cm.CommonName, cert.Subject.CommonName)
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
	newDomains := append(domains, "foo.local")
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
			if d == "foo.local" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Regenerated certificate does not contain new domain")
		}
	}
}

func TestCertificateManagerIPAddress(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "crypto-ip-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	cm := NewCertificateManager(filepath.Join(tempDir, "certs"))

	// Test certificate generation with an IP address
	domains := []string{"192.0.2.100", "localhost"}
	certPEM, _, err := cm.GenerateCertificate(domains)
	if err != nil {
		t.Fatalf("Failed to generate certificate: %v", err)
	}

	// Verify generated certificate
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Check IP addresses
	foundIP := false
	for _, ip := range cert.IPAddresses {
		if ip.String() == "192.0.2.100" {
			foundIP = true
			break
		}
	}

	if !foundIP {
		t.Errorf("Expected IP address 192.0.2.100 in IPAddresses, but it was not found")
	}

	// Check if it was mistakenly added to DNSNames
	for _, dns := range cert.DNSNames {
		if dns == "192.0.2.100" {
			t.Errorf("IP address 192.0.2.100 should NOT be in DNSNames")
		}
	}
}
