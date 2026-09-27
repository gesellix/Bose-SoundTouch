package handlers

import "testing"

// TestResolvConfHasNameserver covers the pure parsing logic behind
// readSpeakerDNSResolution's usesAfterTouch signal. The SSH round-trip
// itself isn't unit-tested here (no real speaker to dial), matching the
// existing precedent for readSpeakerBmxRegistryURL in this package.
func TestResolvConfHasNameserver(t *testing.T) {
	tests := []struct {
		name       string
		resolvConf string
		ip         string
		want       bool
	}{
		{
			name:       "matching nameserver first line",
			resolvConf: "nameserver 192.0.2.1\nnameserver 198.51.100.1\n",
			ip:         "192.0.2.1",
			want:       true,
		},
		{
			name:       "matching nameserver not first line",
			resolvConf: "nameserver 198.51.100.1\nnameserver 192.0.2.1\n",
			ip:         "192.0.2.1",
			want:       true,
		},
		{
			name:       "only ISP nameservers, no AfterTouch entry",
			resolvConf: "nameserver 198.51.100.1\nnameserver 198.51.100.2\n",
			ip:         "192.0.2.1",
			want:       false,
		},
		{
			name:       "empty resolv.conf",
			resolvConf: "",
			ip:         "192.0.2.1",
			want:       false,
		},
		{
			name:       "trailing whitespace / CRLF tolerated",
			resolvConf: "nameserver 192.0.2.1\r\n",
			ip:         "192.0.2.1",
			want:       true,
		},
		{
			name:       "substring IP does not falsely match",
			resolvConf: "nameserver 192.0.2.100\n",
			ip:         "192.0.2.1",
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolvConfHasNameserver(tt.resolvConf, tt.ip); got != tt.want {
				t.Errorf("resolvConfHasNameserver(%q, %q) = %v, want %v", tt.resolvConf, tt.ip, got, tt.want)
			}
		})
	}
}
