package soundtouchweb

import (
	"context"
	"log"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// NewDiscoveryService loads config and returns a unified discovery service
// preconfigured for the web UI's use (10 s discovery timeout, cache on).
// When discoveryInterface is non-empty, mDNS/UPnP are pinned to that NIC.
func NewDiscoveryService(discoveryInterface string) *discovery.UnifiedDiscoveryService {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Printf("Failed to load config: %v, using defaults", err)

		cfg = config.DefaultConfig()
	}

	cfg.DiscoveryTimeout = 10 * time.Second
	cfg.CacheEnabled = true

	if discoveryInterface != "" {
		cfg.DiscoveryInterface = discoveryInterface
	}

	return discovery.NewUnifiedDiscoveryService(cfg)
}

// AddDeviceByHost registers a SoundTouch device with the WebApp by fetching
// its /info and creating a DeviceConnection. The source label
// ("manual" or "discovered") appears in log lines so the operator can
// tell apart entries that came from --devices from those found via
// mDNS/UPnP. If the host is already known, the existing entry's
// LastSeen is bumped and the function returns without re-fetching.
func (app *WebApp) AddDeviceByHost(host string, port int, source string) {
	// Fast path: skip the network call if we already know this host.
	if app.TouchDevice(host) {
		return
	}

	c := client.NewClient(&client.Config{
		Host:    host,
		Port:    port,
		Timeout: 10 * time.Second,
	})

	info, err := c.GetDeviceInfo()
	if err != nil {
		log.Printf("Failed to fetch device info from %s (%s): %v", host, source, err)
		return
	}

	// Ensure IPAddress is set for the web UI
	if info.IPAddress == "" {
		info.IPAddress = host
	}

	conn := webtypes.NewDeviceConnection(c, info)
	if !app.AddDevice(host, conn) {
		// Lost a race — another goroutine inserted the same host
		// between TouchDevice and AddDevice. AddDevice bumped LastSeen
		// on the existing entry; discard our conn.
		return
	}

	go app.UpdateDeviceStatus(host, conn)

	log.Printf("Added %s device %s (%s) at %s:%d", source, info.Name, info.Type, host, port)
}

// DiscoverDevices runs an mDNS/UPnP sweep and registers any found
// devices via AddDeviceByHost. Used by the startup goroutine in main
// and by the /api/discover route inside Mount.
func (app *WebApp) DiscoverDevices(ctx context.Context, discoveryService *discovery.UnifiedDiscoveryService) {
	log.Println("Starting device discovery...")

	devices, err := discoveryService.DiscoverDevices(ctx)
	if err != nil {
		log.Printf("Discovery failed: %v", err)
		app.BroadcastDiscoveryStatus("failed", app.DeviceCount())

		return
	}

	log.Printf("Found %d devices", len(devices))

	for _, device := range devices {
		app.AddDeviceByHost(device.Host, device.Port, "discovered")
	}
}
