package health

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// CheckIDOAuthTargetReachable is the registry id of the OAuth-target
// configuration check. It fires when AfterTouch's configured serverURL
// is an IP literal AND the built-in DNS hijack is running — the
// combination that breaks Spotify / Amazon Music OAuth because the
// speaker firmware constructs `<first-label>oauth.<rest>` from the
// streaming hostname, producing a malformed name (e.g. `192oauth.168.0.30`)
// when the first label is the numeric part of an IP.
//
// See docs/concepts/amazon-music-oauth.md for the underlying mechanism
// and pkg/discovery/dns.go DeriveOAuthHostnames for the auto-derivation
// that makes the hostname case work without operator intervention.
const CheckIDOAuthTargetReachable = "oauth_target_reachable"

// RegisterOAuthTargetReachableCheck registers the OAuth-target check.
// getServerURL returns the operator's currently-configured streaming
// URL (typically Server.GetSettings's first return value);
// getDNSRunning reports whether AfterTouch's DNS hijack server is
// actually serving.
//
// The check is intentionally narrow: it doesn't probe the OAuth flow
// end-to-end. It surfaces the one misconfiguration the speaker firmware
// cannot recover from — IP-based serverURL — so operators see the
// problem before they wire up Spotify / Amazon Music and wonder why
// the speaker's OAuth callback never reaches them.
func RegisterOAuthTargetReachableCheck(r *Registry, getServerURL func() string, getDNSRunning func() (bool, string)) {
	r.Register(Check{
		ID:    CheckIDOAuthTargetReachable,
		Title: "OAuth subdomain is resolvable from the configured serverURL",
		Run: func() []Finding {
			return runOAuthTargetReachableCheck(getServerURL(), getDNSRunning)
		},
	})
}

func runOAuthTargetReachableCheck(serverURL string, getDNSRunning func() (bool, string)) []Finding {
	if strings.TrimSpace(serverURL) == "" {
		return nil
	}

	u, err := url.Parse(serverURL)
	if err != nil {
		return nil
	}

	host := u.Hostname()
	if host == "" {
		return nil
	}

	// IP-based serverURL is the only case the speaker can't recover from.
	// Hostname-based serverURLs are auto-handled by the DNS interceptor
	// (see pkg/discovery/dns.go DeriveOAuthHostnames).
	if net.ParseIP(host) == nil {
		return nil
	}

	dnsRunning := false
	if getDNSRunning != nil {
		dnsRunning, _ = getDNSRunning()
	}

	return []Finding{{
		Severity: SeverityWarning,
		Message: fmt.Sprintf(
			"Configured serverURL %q uses an IP literal. Spotify and Amazon Music OAuth won't work — the speaker firmware constructs the OAuth host by appending \"oauth\" to the first label of the streaming hostname, which for an IP yields a malformed name no DNS resolver can answer (e.g. %s).",
			serverURL, exampleMalformedOAuthHost(host),
		),
		Details: oauthTargetDetails(dnsRunning),
		ManualCommands: []ManualCommand{
			{
				Label:   "Switch the service URL to a real LAN hostname (restart required):",
				Command: "soundtouch-service --server-url=https://aftertouch.lan:8443 …",
				Hint:    "Replace `aftertouch.lan` with whatever LAN-resolvable name you prefer; ensure DNS resolves it to this host's IP.",
			},
			{
				Label:   "Or set via the web UI:",
				Command: "Settings tab → Target Domain → enter the hostname-based URL → Save → restart the service.",
			},
		},
	}}
}

// exampleMalformedOAuthHost returns what the speaker firmware would
// construct given the configured IP. Used in the warning message to
// make the failure mode concrete for the operator.
func exampleMalformedOAuthHost(ipHost string) string {
	idx := strings.IndexByte(ipHost, '.')
	if idx <= 0 {
		return ipHost + "oauth"
	}

	return ipHost[:idx] + "oauth" + ipHost[idx:]
}

func oauthTargetDetails(dnsRunning bool) string {
	base := "After switching to a hostname-based serverURL and restarting, AfterTouch's DNS server auto-derives the `<host>oauth.<rest>` alias and hijacks it to its own IP — no manual DNS-alias work needed."
	if !dnsRunning {
		base += " (The DNS hijack server isn't currently running on this host. Enable it via Settings → DNS Discovery, or set up the alias on an external LAN DNS / each speaker's /etc/hosts. See docs/concepts/amazon-music-oauth.md.)"
	}

	return base
}
