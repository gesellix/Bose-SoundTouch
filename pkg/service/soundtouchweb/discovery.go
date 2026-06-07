package soundtouchweb

import (
	"context"
	"log"
	"sync"
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
		log.Printf("Failed to fetch device info from %s (%s): %v", sanitizeLog(host), sanitizeLog(source), err)
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

	// Poll via HTTP every 30 s as a fallback for WebSocket events that the
	// speaker does not emit (e.g. Spotify Connect track changes) and for the
	// window between a WS disconnect and its reconnect.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				app.UpdateDeviceStatus(host, conn)
			case <-conn.Done():
				return
			}
		}
	}()

	log.Printf("Added %s device %s (%s) at %s:%d", sanitizeLog(source), sanitizeLog(info.Name), sanitizeLog(info.Type), sanitizeLog(host), port)
}

// SeedExtraDevices registers any devices reported by the ExtraDeviceHosts hook
// (if set) via AddDeviceByHost. Idempotent: already-known hosts are skipped.
// Used by the embedded build to surface the service datastore's devices even
// when network discovery is disabled; a no-op for standalone soundtouch-web.
//
// Hosts are probed concurrently: AddDeviceByHost makes a blocking /info call
// (up to its 10 s timeout) for each unknown host, so an offline speaker in the
// datastore would otherwise stall the whole seed for 10 s, serially. Fanning
// out bounds the cost to roughly a single timeout regardless of how many
// devices are offline. AddDeviceByHost is registry-safe under concurrency.
func (app *WebApp) SeedExtraDevices() {
	if app.ExtraDeviceHosts == nil {
		return
	}

	var wg sync.WaitGroup

	for _, host := range app.ExtraDeviceHosts() {
		if host == "" {
			continue
		}

		wg.Add(1)

		go func(h string) {
			defer wg.Done()

			app.AddDeviceByHost(h, 8090, "service-store")
		}(host)
	}

	wg.Wait()
}

// DiscoverDevices refreshes the device registry. When TriggerDiscovery is set
// (embedded build), it runs the host service's discovery so the shared store is
// refreshed, then re-syncs from ExtraDeviceHosts — it does NOT run its own
// mDNS/UPnP, so the embedded build never duplicates the service's discovery.
// When discoveryService is non-nil (standalone soundtouch-web), it runs an
// mDNS/UPnP sweep and registers any found devices via AddDeviceByHost.
// Used by the startup goroutine in main and by the /api/control/discover route
// inside MountWeb.
func (app *WebApp) DiscoverDevices(ctx context.Context, discoveryService *discovery.UnifiedDiscoveryService) {
	// External discovery (embedded build): run the host service's sweep so the
	// shared datastore is refreshed.
	if app.TriggerDiscovery != nil {
		app.TriggerDiscovery(ctx)
	}

	// Re-sync from the external device source (embedded: the service datastore).
	// No-op when ExtraDeviceHosts is unset.
	app.SeedExtraDevices()

	// Own mDNS/UPnP sweep — standalone only. The embedded build passes a nil
	// discovery service and relies entirely on the host service's discovery.
	if discoveryService == nil {
		return
	}

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
