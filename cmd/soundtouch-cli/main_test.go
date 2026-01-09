package main

import (
	"testing"
)

func TestParseHostPort(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultPort int
		wantHost    string
		wantPort    int
	}{
		{
			name:        "IPv4 with port",
			input:       "192.168.1.10:8090",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8090,
		},
		{
			name:        "IPv4 without port",
			input:       "192.168.1.10",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8080,
		},
		{
			name:        "hostname with port",
			input:       "soundtouch.local:9090",
			defaultPort: 8080,
			wantHost:    "soundtouch.local",
			wantPort:    9090,
		},
		{
			name:        "hostname without port",
			input:       "soundtouch.local",
			defaultPort: 8080,
			wantHost:    "soundtouch.local",
			wantPort:    8080,
		},
		{
			name:        "localhost with port",
			input:       "localhost:3000",
			defaultPort: 8080,
			wantHost:    "localhost",
			wantPort:    3000,
		},
		{
			name:        "IPv6 with port",
			input:       "[::1]:8090",
			defaultPort: 8080,
			wantHost:    "::1",
			wantPort:    8090,
		},
		{
			name:        "IPv6 without port",
			input:       "::1",
			defaultPort: 8080,
			wantHost:    "::1",
			wantPort:    8080,
		},
		{
			name:        "invalid port - non-numeric",
			input:       "192.168.1.10:abc",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8080,
		},
		{
			name:        "invalid port - too high",
			input:       "192.168.1.10:99999",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8080,
		},
		{
			name:        "invalid port - zero",
			input:       "192.168.1.10:0",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8080,
		},
		{
			name:        "invalid port - negative",
			input:       "192.168.1.10:-123",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8080,
		},
		{
			name:        "empty string",
			input:       "",
			defaultPort: 8080,
			wantHost:    "",
			wantPort:    8080,
		},
		{
			name:        "just colon",
			input:       ":",
			defaultPort: 8080,
			wantHost:    "",
			wantPort:    8080,
		},
		{
			name:        "multiple colons - malformed",
			input:       "192.168.1.100:8090:extra",
			defaultPort: 8080,
			wantHost:    "192.168.1.100:8090:extra",
			wantPort:    8080,
		},
		{
			name:        "standard SoundTouch default",
			input:       "192.168.1.10",
			defaultPort: 8090,
			wantHost:    "192.168.1.10",
			wantPort:    8090,
		},
		{
			name:        "valid high port",
			input:       "192.168.1.100:65535",
			defaultPort: 8080,
			wantHost:    "192.168.1.100",
			wantPort:    65535,
		},
		{
			name:        "valid low port",
			input:       "192.168.1.100:1",
			defaultPort: 8080,
			wantHost:    "192.168.1.100",
			wantPort:    1,
		},
		{
			name:        "real SoundTouch device example",
			input:       "192.168.1.10:8090",
			defaultPort: 8080,
			wantHost:    "192.168.1.10",
			wantPort:    8090,
		},
		{
			name:        "hostname only fallback",
			input:       "bose-soundtouch-20",
			defaultPort: 8090,
			wantHost:    "bose-soundtouch-20",
			wantPort:    8090,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort := parseHostPort(tt.input, tt.defaultPort)
			if gotHost != tt.wantHost {
				t.Errorf("parseHostPort() host = %v, want %v", gotHost, tt.wantHost)
			}
			if gotPort != tt.wantPort {
				t.Errorf("parseHostPort() port = %v, want %v", gotPort, tt.wantPort)
			}
		})
	}
}

func BenchmarkParseHostPort(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"with_port", "192.168.1.100:8090"},
		{"without_port", "192.168.1.100"},
		{"hostname_with_port", "soundtouch.local:8090"},
		{"ipv6_with_port", "[::1]:8090"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				parseHostPort(tc.input, 8080)
			}
		})
	}
}

// Test edge cases with real-world SoundTouch scenarios
func TestParseHostPortSoundTouchScenarios(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		defaultPort int
		description string
		wantHost    string
		wantPort    int
	}{
		{
			name:        "typical_cli_usage",
			input:       "192.168.1.10:8091",
			defaultPort: 8090,
			description: "User specifies full host:port",
			wantHost:    "192.168.1.10",
			wantPort:    8091,
		},
		{
			name:        "discovery_result_host_only",
			input:       "192.168.1.10",
			defaultPort: 8090,
			description: "Discovery returns IP, CLI uses default port",
			wantHost:    "192.168.1.10",
			wantPort:    8090,
		},
		{
			name:        "custom_port_override",
			input:       "192.168.1.100:9000",
			defaultPort: 8090,
			description: "User overrides default SoundTouch port",
			wantHost:    "192.168.1.100",
			wantPort:    9000,
		},
		{
			name:        "hostname_resolution",
			input:       "bose-kitchen.local:8090",
			defaultPort: 8090,
			description: "mDNS/Bonjour hostname with port",
			wantHost:    "bose-kitchen.local",
			wantPort:    8090,
		},
		{
			name:        "invalid_port_fallback",
			input:       "192.168.1.10:invalid",
			defaultPort: 8090,
			description: "Malformed port should fallback to default",
			wantHost:    "192.168.1.10",
			wantPort:    8090,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost, gotPort := parseHostPort(tt.input, tt.defaultPort)
			if gotHost != tt.wantHost {
				t.Errorf("parseHostPort() host = %v, want %v (scenario: %s)", gotHost, tt.wantHost, tt.description)
			}
			if gotPort != tt.wantPort {
				t.Errorf("parseHostPort() port = %v, want %v (scenario: %s)", gotPort, tt.wantPort, tt.description)
			}
		})
	}
}
