// Package mocks provides mock implementations for external services during testing.
package mocks

import (
	"net/http/httptest"

	"github.com/gesellix/bose-soundtouch/pkg/testutils/spotify"
)

// SpotifyMock simulates Spotify API responses for OAuth and profile interactions.
type SpotifyMock struct {
	server *httptest.Server
}

// NewSpotifyMock creates and starts a new Spotify mock server.
func NewSpotifyMock() *SpotifyMock {
	return &SpotifyMock{
		server: httptest.NewServer(spotify.NewSpotifyHandler()),
	}
}

// URL returns the base URL of the mock server.
func (m *SpotifyMock) URL() string {
	return m.server.URL
}

// TokenURL returns the OAuth token endpoint URL.
func (m *SpotifyMock) TokenURL() string {
	return m.server.URL + "/api/token"
}

// APIBase returns the base API URL.
func (m *SpotifyMock) APIBase() string {
	return m.server.URL
}

// Close stops the mock server.
func (m *SpotifyMock) Close() {
	m.server.Close()
}
