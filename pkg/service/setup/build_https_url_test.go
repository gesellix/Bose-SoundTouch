package setup

import (
	"testing"
)

func TestBuildServerHTTPSURL_PortResolution(t *testing.T) {
	// HTTPS_PORT must be unset for the env-var path tests to be
	// meaningful. t.Setenv("HTTPS_PORT", "") clears it for the duration
	// of each subtest.

	tests := []struct {
		name        string
		targetURL   string
		envHTTPSPort string
		want        string
	}{
		{
			name:      "https with explicit port wins over HTTPS_PORT env",
			targetURL: "https://soundtouch.fritz.box:443",
			envHTTPSPort: "8443",
			want:      "https://soundtouch.fritz.box:443/health",
		},
		{
			name:      "https without explicit port uses 443",
			targetURL: "https://soundtouch.fritz.box",
			want:      "https://soundtouch.fritz.box:443/health",
		},
		{
			name:         "http URL falls back to HTTPS_PORT env var",
			targetURL:    "http://aftertouch.local:8000",
			envHTTPSPort: "9443",
			want:         "https://aftertouch.local:9443/health",
		},
		{
			name:      "http URL with no env var defaults to 8443",
			targetURL: "http://aftertouch.local:8000",
			want:      "https://aftertouch.local:8443/health",
		},
		{
			name:      "invalid URL returns empty",
			targetURL: "::not-a-url",
			want:      "",
		},
		{
			name:      "URL with no hostname returns empty",
			targetURL: "http://",
			want:      "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.envHTTPSPort != "" {
				t.Setenv("HTTPS_PORT", tc.envHTTPSPort)
			} else {
				t.Setenv("HTTPS_PORT", "")
			}

			m := &Manager{}

			got := m.buildServerHTTPSURL(tc.targetURL)
			if got != tc.want {
				t.Errorf("buildServerHTTPSURL(%q) = %q, want %q", tc.targetURL, got, tc.want)
			}
		})
	}
}
