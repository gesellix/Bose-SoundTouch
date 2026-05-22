// Package discovery provides device discovery functionality for Bose SoundTouch devices using mDNS and UPnP protocols.
package discovery

import (
	"strings"
	"time"
)

const (
	// SSDP multicast address and port
	ssdpAddr = "239.255.255.250:1900"

	// SoundTouch device URN for UPnP discovery
	soundTouchURN = "urn:schemas-upnp-org:device:MediaRenderer:1"

	// mDNS service type for SoundTouch devices (matches Bose's actual service name).
	// Retained as the canonical / primary service type for log lines and tests;
	// the full set of accepted variants lives in soundTouchServiceTypes below.
	soundTouchServiceType = "_soundtouch._tcp"
	soundTouchDomain      = "local."

	// Default discovery timeout
	defaultTimeout = 5 * time.Second

	// Default cache TTL
	defaultCacheTTL = 30 * time.Second
)

// soundTouchServiceTypes lists every mDNS service-type variant we consider
// part of the SoundTouch family. mDNS doesn't support wildcard service-type
// queries at the protocol level, so the discovery code issues one parallel
// query per entry below and merges the results. Add new variants here as
// they're observed in the wild — Bose has historically advertised at
// least three:
//
//   - _soundtouch._tcp        : classic SoundTouch speakers (ST10/20/30, …)
//   - _bose-soundtouch._tcp   : seen on some newer firmware variants
//   - _soundtouchstick._tcp   : SoundTouch Wireless Adapter / dongle
var soundTouchServiceTypes = []string{
	"_soundtouch._tcp",
	"_bose-soundtouch._tcp",
	"_soundtouchstick._tcp",
}

// isSoundTouchServiceName reports whether the mDNS service entry name
// belongs to any registered SoundTouch service type. Case-insensitive
// substring match — Bose's mDNS entries embed the service type after a
// dot (e.g. "Speaker._soundtouch._tcp.local.").
func isSoundTouchServiceName(name string) bool {
	lower := strings.ToLower(name)
	for _, t := range soundTouchServiceTypes {
		if strings.Contains(lower, strings.ToLower(t)) {
			return true
		}
	}

	return false
}
