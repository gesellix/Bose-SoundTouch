package handlers

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"net/netip"

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

// clientIPMiddleware resolves the client IP into the request context (read via
// middleware.GetClientIP). The socket peer is always recorded. When
// trustForwardedHeaders is set AND the immediate TCP peer is one of trustedPeers,
// the X-Forwarded-For chain is consulted (chi walks it right-to-left, skipping
// trustedCIDRStrings, taking the first untrusted entry). On a flat LAN a
// non-trusted peer's XFF is ignored, so a malicious speaker can't spoof its IP.
func clientIPMiddleware(trustForwardedHeaders bool, trustedPeers []*net.IPNet, trustedCIDRStrings []string) func(http.Handler) http.Handler {
	base := middleware.ClientIPFromRemoteAddr

	if !trustForwardedHeaders || len(trustedPeers) == 0 {
		return func(next http.Handler) http.Handler { return base(next) }
	}

	xff := middleware.ClientIPFromXFF(trustedCIDRStrings...)

	return func(next http.Handler) http.Handler {
		trusted := base(xff(next)) // peer set first, XFF overrides when found
		untrusted := base(next)    // peer only

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isFromTrustedPeer(r.RemoteAddr, trustedPeers) {
				trusted.ServeHTTP(w, r)

				return
			}

			untrusted.ServeHTTP(w, r)
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

// validateCIDRStringsForXFF checks that every CIDR string can be parsed by
// netip.ParsePrefix, which is what middleware.ClientIPFromXFF uses internally
// (it calls netip.MustParsePrefix and panics on failure). Returns an error
// listing the first bad entry, so the caller can fall back to peer-only mode
// rather than panicking at startup.
func validateCIDRStringsForXFF(cidrs []string) error {
	for _, c := range cidrs {
		if _, err := netip.ParsePrefix(c); err != nil {
			return fmt.Errorf("CIDR %q is not valid for ClientIPFromXFF: %w", c, err)
		}
	}

	return nil
}

// buildClientIPMiddleware is the server-level helper that reads settings and
// returns a ready-to-use chi middleware. It is split out of ClientIPMiddleware
// so tests can drive the logic without a full Server.
func buildClientIPMiddleware(trustForwardedHeaders bool, cidrStrings []string) func(http.Handler) http.Handler {
	if !trustForwardedHeaders {
		return clientIPMiddleware(false, nil, nil)
	}

	if len(cidrStrings) == 0 {
		cidrStrings = defaultTrustedProxyCIDRs
	}

	// Validate for netip.MustParsePrefix (panic guard).
	if err := validateCIDRStringsForXFF(cidrStrings); err != nil {
		log.Printf("[ClientIP] invalid trusted_proxy_cidrs: %v — falling back to peer-only", err)
		return clientIPMiddleware(false, nil, nil)
	}

	// Parse for the peer gate (net.IPNet).
	cidrs, err := ParseTrustedProxyCIDRs(cidrStrings)
	if err != nil {
		log.Printf("[ClientIP] invalid trusted_proxy_cidrs: %v — falling back to peer-only", err)
		return clientIPMiddleware(false, nil, nil)
	}

	return clientIPMiddleware(true, cidrs, cidrStrings)
}
