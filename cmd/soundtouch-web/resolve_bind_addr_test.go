package main

import (
	"net"
	"strings"
	"testing"
)

func TestResolveBindAddr_PassThrough(t *testing.T) {
	// Inputs that don't match any local interface name must be returned
	// unchanged: empty string, hostnames, IPv4/IPv6 literals, and bogus
	// strings the user might have typed.
	tests := []string{
		"",
		"localhost",
		"127.0.0.1",
		"192.168.1.5",
		"::1",
		"definitely-not-an-iface-xyz",
	}

	for _, input := range tests {
		t.Run(quoted(input), func(t *testing.T) {
			got, err := resolveBindAddr(input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != input {
				t.Errorf("got %q, want %q (input should pass through unchanged)", got, input)
			}
		})
	}
}

func TestResolveBindAddr_LoopbackInterface(t *testing.T) {
	loopback, expected, ok := findLoopbackWithSingleIPv4(t)
	if !ok {
		t.Skipf("no loopback interface with exactly one IPv4 address found")
	}

	got, err := resolveBindAddr(loopback)
	if err != nil {
		t.Fatalf("unexpected error resolving %q: %v", loopback, err)
	}

	if got != expected {
		t.Errorf("got %q, want %q for loopback interface %q", got, expected, loopback)
	}
}

// findLoopbackWithSingleIPv4 returns the name of a loopback interface and the
// single IPv4 address attached to it. If the host has multiple loopback
// interfaces or the loopback has zero or several IPv4 addresses, it returns
// ok=false so the caller can skip the test rather than fail on an environment
// quirk.
func findLoopbackWithSingleIPv4(t *testing.T) (name, addr string, ok bool) {
	t.Helper()

	ifaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces: %v", err)
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback == 0 {
			continue
		}

		addrs, addrErr := iface.Addrs()
		if addrErr != nil {
			continue
		}

		var ipv4s []string

		for _, a := range addrs {
			if ipnet, isIPNet := a.(*net.IPNet); isIPNet {
				if v4 := ipnet.IP.To4(); v4 != nil {
					ipv4s = append(ipv4s, v4.String())
				}
			}
		}

		if len(ipv4s) == 1 {
			return iface.Name, ipv4s[0], true
		}
	}

	return "", "", false
}

func TestDefaultDiscoveryInterface(t *testing.T) {
	tests := []struct {
		name         string
		rawInterface string
		rawBind      string
		resolvedBind string
		want         string
	}{
		{
			name:         "explicit interface wins over bind-derived default",
			rawInterface: "eth1",
			rawBind:      "eth0",
			resolvedBind: "192.168.1.5",
			want:         "eth1",
		},
		{
			name:         "derive from --bind when --bind was an interface name",
			rawInterface: "",
			rawBind:      "eth0",
			resolvedBind: "192.168.1.5",
			want:         "eth0",
		},
		{
			name:         "no derivation when --bind was an IP literal",
			rawInterface: "",
			rawBind:      "192.168.1.5",
			resolvedBind: "192.168.1.5",
			want:         "",
		},
		{
			name:         "no derivation when --bind was a hostname (pass-through)",
			rawInterface: "",
			rawBind:      "localhost",
			resolvedBind: "localhost",
			want:         "",
		},
		{
			name:         "both empty stays empty (auto-pick)",
			rawInterface: "",
			rawBind:      "",
			resolvedBind: "",
			want:         "",
		},
		{
			name:         "explicit interface alone, --bind empty",
			rawInterface: "eth1",
			rawBind:      "",
			resolvedBind: "",
			want:         "eth1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := defaultDiscoveryInterface(tc.rawInterface, tc.rawBind, tc.resolvedBind)
			if got != tc.want {
				t.Errorf("got %q, want %q (rawInterface=%q rawBind=%q resolvedBind=%q)",
					got, tc.want, tc.rawInterface, tc.rawBind, tc.resolvedBind)
			}
		})
	}
}

func quoted(s string) string {
	if s == "" {
		return "(empty)"
	}

	return strings.ReplaceAll(s, "/", "_")
}
