package handlers

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrustedRealIP(t *testing.T) {
	cidrs, err := ParseTrustedProxyCIDRs([]string{"127.0.0.0/8", "::1/128"})
	if err != nil {
		t.Fatalf("ParseTrustedProxyCIDRs: %v", err)
	}

	mw := TrustedRealIP(cidrs)
	if mw == nil {
		t.Fatal("TrustedRealIP returned nil for non-empty trustedPeers")
	}

	cases := []struct {
		name           string
		remoteAddr     string
		xRealIP        string
		xForwardedFor  string
		wantRemoteAddr string
	}{
		{
			name:           "trusted peer with X-Real-IP is honoured",
			remoteAddr:     "127.0.0.1:54321",
			xRealIP:        "192.0.2.10",
			wantRemoteAddr: "192.0.2.10",
		},
		{
			name:           "trusted peer with X-Forwarded-For is honoured",
			remoteAddr:     "127.0.0.1:54321",
			xForwardedFor:  "192.0.2.20, 10.0.0.1",
			wantRemoteAddr: "192.0.2.20",
		},
		{
			name:           "trusted peer with no headers leaves RemoteAddr alone",
			remoteAddr:     "127.0.0.1:54321",
			wantRemoteAddr: "127.0.0.1:54321",
		},
		{
			name:           "untrusted peer's X-Real-IP is ignored",
			remoteAddr:     "192.0.2.99:54321",
			xRealIP:        "1.2.3.4",
			wantRemoteAddr: "192.0.2.99:54321",
		},
		{
			name:           "untrusted peer's X-Forwarded-For is ignored",
			remoteAddr:     "192.0.2.99:54321",
			xForwardedFor:  "1.2.3.4",
			wantRemoteAddr: "192.0.2.99:54321",
		},
		{
			name:           "trusted peer with garbage X-Real-IP leaves RemoteAddr alone",
			remoteAddr:     "127.0.0.1:54321",
			xRealIP:        "not-an-ip",
			wantRemoteAddr: "127.0.0.1:54321",
		},
		{
			name:           "trusted IPv6 loopback peer is honoured",
			remoteAddr:     "[::1]:54321",
			xRealIP:        "fe80::1",
			wantRemoteAddr: "fe80::1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string

			h := mw(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
				got = r.RemoteAddr
			}))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr

			if tc.xRealIP != "" {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}

			if tc.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tc.xForwardedFor)
			}

			h.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.wantRemoteAddr {
				t.Errorf("RemoteAddr = %q, want %q", got, tc.wantRemoteAddr)
			}
		})
	}
}

func TestTrustedRealIP_NilForEmptyPeers(t *testing.T) {
	if mw := TrustedRealIP(nil); mw != nil {
		t.Error("TrustedRealIP(nil) returned non-nil; expected nil so caller can skip Use()")
	}

	if mw := TrustedRealIP([]*net.IPNet{}); mw != nil {
		t.Error("TrustedRealIP([]) returned non-nil; expected nil so caller can skip Use()")
	}
}

func TestParseTrustedProxyCIDRs(t *testing.T) {
	t.Run("empty input yields loopback default", func(t *testing.T) {
		got, err := ParseTrustedProxyCIDRs(nil)
		if err != nil {
			t.Fatalf("ParseTrustedProxyCIDRs: %v", err)
		}

		if len(got) != 2 {
			t.Fatalf("default CIDR count = %d, want 2 (127/8 + ::1/128)", len(got))
		}

		// Should contain 127.0.0.1 and ::1.
		if !isFromTrustedPeer("127.0.0.1:1", got) {
			t.Error("default CIDRs should include 127.0.0.1")
		}

		if !isFromTrustedPeer("[::1]:1", got) {
			t.Error("default CIDRs should include ::1")
		}
	})

	t.Run("custom CIDRs override defaults", func(t *testing.T) {
		got, err := ParseTrustedProxyCIDRs([]string{"10.0.0.0/8"})
		if err != nil {
			t.Fatalf("ParseTrustedProxyCIDRs: %v", err)
		}

		if len(got) != 1 {
			t.Errorf("custom CIDR count = %d, want 1", len(got))
		}

		if !isFromTrustedPeer("10.1.2.3:1", got) {
			t.Error("10.1.2.3 should be in 10.0.0.0/8")
		}

		if isFromTrustedPeer("127.0.0.1:1", got) {
			t.Error("127.0.0.1 should NOT match when default is overridden")
		}
	})

	t.Run("invalid CIDR returns error", func(t *testing.T) {
		_, err := ParseTrustedProxyCIDRs([]string{"not-a-cidr"})
		if err == nil {
			t.Fatal("expected error on invalid CIDR")
		}
	})
}
