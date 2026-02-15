// Package certmanager provides tools for managing Root CAs and generating SSL certificates.
package certmanager

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// CertificateManager handles CA and certificate generation.
type CertificateManager struct {
	CertsDir string
}

// NewCertificateManager creates a new CertificateManager.
func NewCertificateManager(certsDir string) *CertificateManager {
	return &CertificateManager{CertsDir: certsDir}
}

// GetCACertPath returns the path to the CA certificate.
func (cm *CertificateManager) GetCACertPath() string {
	return filepath.Join(cm.CertsDir, "ca.crt")
}

// GetCAKeyPath returns the path to the CA private key.
func (cm *CertificateManager) GetCAKeyPath() string {
	return filepath.Join(cm.CertsDir, "ca.key")
}

// EnsureCA ensures that a CA certificate and key exist.
func (cm *CertificateManager) EnsureCA() error {
	certPath := cm.GetCACertPath()
	keyPath := cm.GetCAKeyPath()

	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			return nil
		}
	}

	return cm.GenerateCA()
}

// GetServerCertPEMPath returns the path to the server certificate PEM.
func (cm *CertificateManager) GetServerCertPEMPath() string {
	return filepath.Join(cm.CertsDir, "server.crt")
}

// GetServerKeyPEMPath returns the path to the server private key PEM.
func (cm *CertificateManager) GetServerKeyPEMPath() string {
	return filepath.Join(cm.CertsDir, "server.key")
}

// GetServerTLSConfig returns a TLS config with the server certificate.
// If the certificate doesn't exist, it generates one for the given domains.
func (cm *CertificateManager) GetServerTLSConfig(domains []string) (*tls.Config, error) {
	certPath := cm.GetServerCertPEMPath()
	keyPath := cm.GetServerKeyPEMPath()

	generate := false
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		generate = true
	} else {
		// Check if the current certificate covers all requested domains
		certBytes, err := os.ReadFile(certPath)
		if err == nil {
			block, _ := pem.Decode(certBytes)
			if block != nil {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					domainMap := make(map[string]bool)
					for _, d := range cert.DNSNames {
						domainMap[d] = true
					}

					for _, d := range domains {
						if !domainMap[d] {
							generate = true
							break
						}
					}
				} else {
					generate = true
				}
			} else {
				generate = true
			}
		} else {
			generate = true
		}
	}

	if generate {
		certPEM, keyPEM, err := cm.GenerateCertificate(domains)
		if err != nil {
			return nil, err
		}

		if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
			return nil, err
		}

		if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
			return nil, err
		}
	}

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_RSA_WITH_AES_256_GCM_SHA384,
		},
	}, nil
}

// GenerateCA generates a new CA certificate and key.
func (cm *CertificateManager) GenerateCA() error {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(10 * 365 * 24 * time.Hour) // 10 years

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"AfterTouch"},
			CommonName:   "AfterTouch Local Root CA",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return err
	}

	certPath := cm.GetCACertPath()
	if mkdirErr := os.MkdirAll(cm.CertsDir, 0755); mkdirErr != nil {
		return mkdirErr
	}

	certOut, err := os.Create(certPath)
	if err != nil {
		return err
	}

	if encodeErr := pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes}); encodeErr != nil {
		return encodeErr
	}

	certOut.Close()

	keyOut, err := os.OpenFile(cm.GetCAKeyPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	if err := pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}); err != nil {
		return err
	}

	keyOut.Close()

	return nil
}

// GenerateCertificate generates a certificate for the given domains signed by the CA.
func (cm *CertificateManager) GenerateCertificate(domains []string) ([]byte, []byte, error) {
	if err := cm.EnsureCA(); err != nil {
		return nil, nil, err
	}

	caCertPEM, err := os.ReadFile(cm.GetCACertPath())
	if err != nil {
		return nil, nil, err
	}

	caKeyPEM, err := os.ReadFile(cm.GetCAKeyPath())
	if err != nil {
		return nil, nil, err
	}

	caBlock, _ := pem.Decode(caCertPEM)

	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	keyBlock, _ := pem.Decode(caKeyPEM)

	caKey, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(365 * 24 * time.Hour) // 1 year

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"AfterTouch"},
			CommonName:   domains[0],
		},
		NotBefore:   notBefore,
		NotAfter:    notAfter,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    domains,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &priv.PublicKey, caKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return certPEM, keyPEM, nil
}
