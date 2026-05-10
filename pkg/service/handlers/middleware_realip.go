package handlers

import (
	"fmt"
	"net"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// defaultTrustedProxyCIDRs is the safe-by-default list applied when
// Settings.TrustedProxyCIDRs is empty. Only loopback addresses are trusted —
// i.e. a reverse proxy on the same host. Anyone deploying behind a proxy on a
// different host must override this in settings.json.
var defaultTrustedProxyCIDRs = []string{
	"127.0.0.0/8",
	"::1/128",
}

// TrustedRealIP returns a middleware that delegates to chi's RealIP — which
// rewrites r.RemoteAddr from True-Client-IP / X-Real-IP / X-Forwarded-For
// headers — but only when the immediate TCP peer is in `trustedPeers`. For
// any request whose peer is *not* trusted (i.e. anything other than the
// configured reverse proxy), the headers are ignored and r.RemoteAddr stays
// as-is.
//
// This avoids the standard X-Forwarded-* spoofing pitfall: on a flat LAN
// where a malicious speaker could send the headers itself, we won't honour
// them; behind a reverse proxy we will.
//
// Returns nil if trustedPeers is empty — caller should not Use a nil mw.
func TrustedRealIP(trustedPeers []*net.IPNet) func(http.Handler) http.Handler {
	if len(trustedPeers) == 0 {
		return nil
	}

	delegate := middleware.RealIP

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isFromTrustedPeer(r.RemoteAddr, trustedPeers) {
				delegate(next).ServeHTTP(w, r)

				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// isFromTrustedPeer reports whether remoteAddr (in the host:port shape that
// net/http populates) is contained in any of the supplied CIDR blocks.
func isFromTrustedPeer(remoteAddr string, trustedPeers []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	for _, n := range trustedPeers {
		if n.Contains(ip) {
			return true
		}
	}

	return false
}

// ParseTrustedProxyCIDRs converts string CIDRs into *net.IPNet values, falling
// back to defaultTrustedProxyCIDRs when the input is empty. An invalid CIDR
// in the input list is reported as an error and stops parsing — better to
// fail loud than silently fall back.
func ParseTrustedProxyCIDRs(cidrs []string) ([]*net.IPNet, error) {
	if len(cidrs) == 0 {
		cidrs = defaultTrustedProxyCIDRs
	}

	out := make([]*net.IPNet, 0, len(cidrs))

	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", c, err)
		}

		out = append(out, n)
	}

	return out, nil
}
