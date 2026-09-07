package main

import "testing"

// TestBrowsableURL covers the startup log line: a wildcard listen address is
// what net.Listen wants but is not something a terminal can turn into a
// clickable link, so it has to be rewritten before it is logged.
func TestBrowsableURL(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"wildcard", ":8080", "http://localhost:8080"},
		{"wildcard IPv4", "0.0.0.0:8080", "http://localhost:8080"},
		{"wildcard IPv6", "[::]:8080", "http://localhost:8080"},
		{"explicit host", "192.0.2.5:8080", "http://192.0.2.5:8080"},
		{"loopback", "127.0.0.1:8000", "http://127.0.0.1:8000"},
		{"hostname", "speaker.local:8080", "http://speaker.local:8080"},
		{"IPv6 literal stays bracketed", "[2001:db8::1]:8080", "http://[2001:db8::1]:8080"},
		{"not host:port is left alone", "nonsense", "http://nonsense"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := browsableURL(tc.addr); got != tc.want {
				t.Errorf("browsableURL(%q) = %q, want %q", tc.addr, got, tc.want)
			}
		})
	}
}
