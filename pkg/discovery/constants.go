package discovery

import "time"

const (
	// SSDP multicast address and port
	ssdpAddr = "239.255.255.250:1900"

	// SoundTouch device URN for UPnP discovery
	soundTouchURN = "urn:schemas-upnp-org:device:MediaRenderer:1"

	// mDNS service type for SoundTouch devices (matches Bose's actual service name)
	soundTouchServiceType = "_soundtouch._tcp"
	soundTouchDomain      = "local."

	// Default discovery timeout
	defaultTimeout = 5 * time.Second

	// Default cache TTL
	defaultCacheTTL = 30 * time.Second
)
