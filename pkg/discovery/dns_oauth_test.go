package discovery

import (
	"strings"
	"testing"
)

func TestDeriveOAuthHostnames(t *testing.T) {
	cases := []struct {
		name      string
		serverURL string
		want      []string
	}{
		{
			name:      "hostname with single domain part",
			serverURL: "https://aftertouch.lan:8443",
			want:      []string{"aftertouchoauth.lan"},
		},
		{
			name:      "hostname with multiple domain parts",
			serverURL: "https://aftertouch.example.local:8443",
			want:      []string{"aftertouchoauth.example.local"},
		},
		{
			name:      "HTTP scheme also works",
			serverURL: "http://aftertouch.lan:8000",
			want:      []string{"aftertouchoauth.lan"},
		},
		{
			name:      "Case is normalised to lower",
			serverURL: "https://AfterTouch.LAN:8443",
			want:      []string{"aftertouchoauth.lan"},
		},
		{
			name:      "IPv4 yields no derivation (malformed result)",
			serverURL: "https://192.168.0.30:8443",
			want:      nil,
		},
		{
			name:      "IPv6 yields no derivation",
			serverURL: "https://[fd00::1]:8443",
			want:      nil,
		},
		{
			name:      "Single-label hostname yields no derivation",
			serverURL: "https://aftertouch:8443",
			want:      nil,
		},
		{
			name:      "Empty serverURL is a no-op",
			serverURL: "",
			want:      nil,
		},
		{
			name:      "Garbage URL is a no-op",
			serverURL: ":::not a url",
			want:      nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveOAuthHostnames(tc.serverURL)
			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %v, want %v", got, tc.want)
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestShouldIntercept_DerivedHostnameFromHostnameServerURL(t *testing.T) {
	d := NewDNSDiscovery([]string{"8.8.8.8"}, "192.0.2.10", "https://aftertouch.lan:8443")

	// Bose hostnames still match by substring.
	if !d.shouldIntercept("streamingoauth.bose.com") {
		t.Errorf("expected Bose oauth host to be intercepted")
	}

	// Derived host matches exactly (case-insensitive).
	if !d.shouldIntercept("aftertouchoauth.lan") {
		t.Errorf("expected derived OAuth subdomain to be intercepted")
	}

	if !d.shouldIntercept("AFTERTOUCHOAUTH.LAN") {
		t.Errorf("expected case-insensitive match on derived OAuth subdomain")
	}

	// Unrelated hosts are not hijacked.
	if d.shouldIntercept("example.com") {
		t.Errorf("unrelated host must not be intercepted")
	}

	// The base host (without -oauth) is NOT auto-hijacked — only the
	// OAuth-derivation. Bose-substring filter and the operator's own
	// migration handle the base host.
	if d.shouldIntercept("aftertouch.lan") {
		t.Errorf("base hostname must not be auto-intercepted; only the OAuth variant is derived")
	}
}

func TestShouldIntercept_NoDerivationFromIPServerURL(t *testing.T) {
	d := NewDNSDiscovery([]string{"8.8.8.8"}, "192.168.0.30", "https://192.168.0.30:8443")

	if len(d.derivedHosts) != 0 {
		t.Errorf("expected no derived hosts for IP-based serverURL, got %v", d.derivedHosts)
	}

	if d.shouldIntercept("192oauth.168.0.30") {
		t.Errorf("malformed IP-derived OAuth name must not be intercepted (it's never a valid DNS query in the first place)")
	}
}

func TestNewDNSDiscovery_LogsDerivationOnce(t *testing.T) {
	// This is a smoke test — the constructor should not panic and should
	// store the derivation. We don't capture the log output here (the
	// dns.go init path uses package log.Printf and isn't easily diverted
	// without test infrastructure), but we do confirm the derivedHosts
	// field is populated as expected.
	d := NewDNSDiscovery(nil, "192.0.2.10", "https://aftertouch.lan")

	if len(d.derivedHosts) != 1 || !strings.Contains(d.derivedHosts[0], "oauth") {
		t.Errorf("expected derivedHosts to carry the OAuth variant, got %v", d.derivedHosts)
	}
}
