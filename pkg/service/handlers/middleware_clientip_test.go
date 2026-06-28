package handlers

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5/middleware"
)

// captureClientIP is a tiny handler that records the resolved client IP from
// the request context (as set by clientIPMiddleware).
func captureClientIP(got *string) http.Handler {
	return http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		*got = middleware.GetClientIP(r.Context())
	})
}

func TestClientIPMiddleware(t *testing.T) {
	defaultCIDRs := []string{"127.0.0.0/8", "::1/128"}

	defaultPeers, err := ParseTrustedProxyCIDRs(defaultCIDRs)
	if err != nil {
		t.Fatalf("ParseTrustedProxyCIDRs: %v", err)
	}

	cases := []struct {
		name                  string
		trustForwardedHeaders bool
		trustedPeers          []*net.IPNet
		trustedCIDRStrings    []string
		remoteAddr            string
		xForwardedFor         string
		wantClientIP          string
	}{
		{
			name:                  "trust disabled: peer recorded, XFF ignored",
			trustForwardedHeaders: false,
			remoteAddr:            "1.2.3.4:5555",
			xForwardedFor:         "9.9.9.9",
			wantClientIP:          "1.2.3.4",
		},
		{
			name:                  "trust enabled, trusted loopback peer, XFF honoured",
			trustForwardedHeaders: true,
			trustedPeers:          defaultPeers,
			trustedCIDRStrings:    defaultCIDRs,
			remoteAddr:            "127.0.0.1:5555",
			xForwardedFor:         "9.9.9.9",
			wantClientIP:          "9.9.9.9",
		},
		{
			name:                  "trust enabled, untrusted peer, XFF ignored (peer gate)",
			trustForwardedHeaders: true,
			trustedPeers:          defaultPeers,
			trustedCIDRStrings:    defaultCIDRs,
			remoteAddr:            "8.8.8.8:5555",
			xForwardedFor:         "9.9.9.9",
			wantClientIP:          "8.8.8.8",
		},
		{
			name:                  "trust enabled, trusted peer, XFF chain rightmost-untrusted wins",
			trustForwardedHeaders: true,
			trustedPeers:          defaultPeers,
			trustedCIDRStrings:    defaultCIDRs,
			remoteAddr:            "127.0.0.1:5555",
			xForwardedFor:         "9.9.9.9, 127.0.0.2",
			wantClientIP:          "9.9.9.9",
		},
		{
			name:                  "trust enabled, trusted peer, garbage XFF falls back to peer",
			trustForwardedHeaders: true,
			trustedPeers:          defaultPeers,
			trustedCIDRStrings:    defaultCIDRs,
			remoteAddr:            "127.0.0.1:5555",
			xForwardedFor:         "not-an-ip",
			wantClientIP:          "127.0.0.1",
		},
		{
			name:                  "trust enabled, trusted IPv6 loopback peer, XFF honoured",
			trustForwardedHeaders: true,
			trustedPeers:          defaultPeers,
			trustedCIDRStrings:    defaultCIDRs,
			remoteAddr:            "[::1]:5555",
			xForwardedFor:         "9.9.9.9",
			wantClientIP:          "9.9.9.9",
		},
		{
			name:                  "trust enabled, trusted peer, no XFF header: peer recorded",
			trustForwardedHeaders: true,
			trustedPeers:          defaultPeers,
			trustedCIDRStrings:    defaultCIDRs,
			remoteAddr:            "127.0.0.1:5555",
			xForwardedFor:         "",
			wantClientIP:          "127.0.0.1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got string

			mw := clientIPMiddleware(tc.trustForwardedHeaders, tc.trustedPeers, tc.trustedCIDRStrings)
			h := mw(captureClientIP(&got))

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr

			if tc.xForwardedFor != "" {
				req.Header.Set("X-Forwarded-For", tc.xForwardedFor)
			}

			h.ServeHTTP(httptest.NewRecorder(), req)

			if got != tc.wantClientIP {
				t.Errorf("GetClientIP = %q, want %q", got, tc.wantClientIP)
			}
		})
	}
}

func TestClientIPMiddleware_AlwaysPopulated(t *testing.T) {
	// Regardless of trust settings, GetClientIP must always return a non-empty
	// string after clientIPMiddleware has run (the socket peer is the fallback).
	mw := clientIPMiddleware(false, nil, nil)

	var got string

	h := mw(captureClientIP(&got))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == "" {
		t.Error("GetClientIP must not be empty after clientIPMiddleware runs")
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
