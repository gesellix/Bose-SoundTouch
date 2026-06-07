package soundtouchweb

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// NewServiceHTTPClient builds an *http.Client that trusts the AfterTouch
// service's CA certificate (PEM at caPath) in addition to the system trust
// store. soundtouch-player uses it for the only server-side call it makes to the
// service (the TTS proxy in handlers_tts.go): the service serves a self-signed
// certificate signed by its own "AfterTouch Local Root CA", which isn't in any
// system trust store, so http.DefaultClient would reject it with
// "x509: certificate signed by unknown authority".
//
// The CA is appended to a copy of the system pool (not a fresh empty one) so a
// deployment whose service URL happens to use a publicly trusted certificate
// keeps working.
func NewServiceHTTPClient(caPath string) (*http.Client, error) {
	pem, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA: %w", err)
	}

	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}

	if !pool.AppendCertsFromPEM(pem) {
		return nil, fmt.Errorf("no valid certificate found in %s", caPath)
	}

	return &http.Client{
		// TTS round-trips through Google Cloud synthesis and speaker playback,
		// so allow more than the bare connect time.
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    pool,
				MinVersion: tls.VersionTLS12,
			},
		},
	}, nil
}
