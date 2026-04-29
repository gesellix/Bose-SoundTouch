package mocks

import (
	"net/http/httptest"

	"github.com/gesellix/bose-soundtouch/pkg/testutils/amazon"
)

// AmazonMock simulates Amazon LWA API responses for OAuth and profile interactions.
type AmazonMock struct {
	server *httptest.Server
}

// NewAmazonMock creates and starts a new Amazon LWA mock server.
func NewAmazonMock() *AmazonMock {
	return &AmazonMock{
		server: httptest.NewServer(amazon.NewAmazonHandler()),
	}
}

// URL returns the base URL of the mock server.
func (m *AmazonMock) URL() string {
	return m.server.URL
}

// TokenURL returns the LWA token endpoint URL.
func (m *AmazonMock) TokenURL() string {
	return m.server.URL + "/auth/o2/token"
}

// ProfileURL returns the LWA user profile endpoint URL.
func (m *AmazonMock) ProfileURL() string {
	return m.server.URL + "/user/profile"
}

// Close stops the mock server.
func (m *AmazonMock) Close() {
	m.server.Close()
}
