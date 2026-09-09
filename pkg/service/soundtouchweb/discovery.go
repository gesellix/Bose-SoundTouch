package soundtouchweb

import (
	"context"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/speaker"
)

// NewDiscoveryService loads config and returns a unified discovery service
// preconfigured for the web UI's use (10 s discovery timeout, cache on).
// When discoveryInterface is non-empty, mDNS/UPnP are pinned to that NIC.
// configuredHosts (e.g. from --devices) are folded into cfg.PreferredDevices
// alongside any already loaded from PREFERRED_DEVICES (deduplicated by
// host), so they're retried on every subsequent DiscoverDevices pass, not
// just once at startup -- a host that's offline now still gets picked up
// once it comes online.
func NewDiscoveryService(discoveryInterface string, configuredHosts ...string) *discovery.UnifiedDiscoveryService {
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

	existingHosts := make(map[string]bool, len(cfg.PreferredDevices))
	for _, d := range cfg.PreferredDevices {
		existingHosts[d.Host] = true
	}

	for _, host := range configuredHosts {
		if host == "" || existingHosts[host] {
			continue
		}

		cfg.PreferredDevices = append(cfg.PreferredDevices, config.DeviceConfig{
			Host: host,
			Port: speaker.HTTPPort,
		})
		existingHosts[host] = true
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
	app.addDeviceByHost(context.Background(), host, port, source)
}

func (app *WebApp) addDeviceByHost(
	ctx context.Context,
	host string,
	port int,
	source string,
) *webtypes.DeviceConnection {
	// Fast path: skip the network call if we already know this host.
	if app.TouchDevice(host) {
		return nil
	}

	c := client.NewClient(&client.Config{
		Host:    host,
		Port:    port,
		Timeout: 10 * time.Second,
	})

	info, err := c.GetDeviceInfo()
	if err != nil {
		log.Printf("Failed to fetch device info from %s (%s): %v", sanitizeLog(host), sanitizeLog(source), err)
		return nil
	}

	// Keep the registry key stable for controls, but expose a canonical numeric
	// address separately for presentation and sorting.
	info.IPAddress = resolvedDeviceIPAddress(ctx, host, info)

	conn := webtypes.NewDeviceConnection(c, info)
	conn.MarkHTTPSuccess(time.Now())

	if !app.AddDevice(host, conn) {
		// Lost a race — another goroutine inserted the same host
		// between TouchDevice and AddDevice. AddDevice bumped LastSeen
		// on the existing entry; discard our conn.
		return nil
	}

	go app.UpdateDeviceStatus(host, conn)

	// Establish the stereo-pair balance reading. It lives here, not on the
	// status poll, because /balance blocks rather than refusing on a sleeping
	// speaker and must never hold up the other fields; and not on the
	// WebSocket, because that socket is created lazily and a paired speaker
	// would then show no balance control until something was pressed.
	go app.watchBalance(host, conn)

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

	return conn
}

func resolvedDeviceIPAddress(ctx context.Context, host string, info *models.DeviceInfo) string {
	if info != nil {
		if address := numericIPAddress(info.IPAddress); address != "" {
			return address
		}

		for _, network := range info.NetworkInfo {
			if address := numericIPAddress(network.IPAddress); address != "" {
				return address
			}
		}
	}

	bareHost := hostOnly(host)
	if address := numericIPAddress(bareHost); address != "" {
		return address
	}

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	addresses, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", bareHost)
	if err == nil && len(addresses) > 0 {
		return addresses[0].String()
	}

	return ""
}

func numericIPAddress(address string) string {
	ip := net.ParseIP(strings.TrimSpace(address))
	if ip == nil {
		return ""
	}

	return ip.String()
}

// SeedExtraDevices registers any devices reported by the ExtraDeviceHosts hook
// (if set) via AddDeviceByHost, and prunes any previously-seeded host that no
// longer appears in that set. Idempotent: already-known hosts are skipped.
// Used by the embedded build to surface the service datastore's devices even
// when network discovery is disabled; a no-op for standalone soundtouch-player.
//
// Hosts are probed concurrently: AddDeviceByHost makes a blocking /info call
// (up to its 10 s timeout) for each unknown host, so an offline speaker in the
// datastore would otherwise stall the whole seed for 10 s, serially. Fanning
// out bounds the cost to roughly a single timeout regardless of how many
// devices are offline. AddDeviceByHost is registry-safe under concurrency.
//
// A hook read failure is logged and otherwise swallowed here; callers that
// need to distinguish "read failed" from "converged" (the bounded startup
// retry) should call seedExtraDevices directly instead.
func (app *WebApp) SeedExtraDevices() {
	if _, _, _, err := app.seedExtraDevices(context.Background()); err != nil {
		log.Printf("SeedExtraDevices: failed to read extra device hosts: %v", err)
	}
}

type seededExtraDevice struct {
	host string
	conn *webtypes.DeviceConnection
}

// seedExtraDevices probes any ExtraDeviceHosts hosts that aren't already
// registered, and prunes any registered host that's no longer in the current
// desired set. Pruning is safe to apply to the whole registry (not just hosts
// this call inserted) because ExtraDeviceHosts is the only inserter into this
// registry for the embedded build: discoveryService is nil there, so the
// mDNS/UPnP insertion path in DiscoverDevices is never reached.
//
// Runs are serialized via seedMu so the bounded startup retry loop
// (SeedExtraDevicesUntilReady) and a devices-changed-hook-triggered
// SeedExtraDevices call never issue concurrent probes to the same offline
// host.
//
// A non-nil error means the hook itself failed (e.g. a datastore glitch);
// callers must treat that as "unknown state, don't prune, don't declare
// ready" rather than as an empty desired set.
func (app *WebApp) seedExtraDevices(
	ctx context.Context,
) (inserted []seededExtraDevice, removed int, desired map[string]struct{}, err error) {
	if app.ExtraDeviceHosts == nil {
		return nil, 0, nil, nil
	}

	app.seedMu.Lock()
	defer app.seedMu.Unlock()

	desired, err = app.extraDeviceHostSet()
	if err != nil {
		return nil, 0, nil, err
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)

	for host := range desired {
		if _, ok := app.GetDevice(host); ok {
			continue
		}

		wg.Add(1)

		go func(h string) {
			defer wg.Done()

			conn := app.addDeviceByHost(ctx, h, 8090, "service-store")
			if conn == nil {
				return
			}

			mu.Lock()

			inserted = append(inserted, seededExtraDevice{host: h, conn: conn})

			mu.Unlock()
		}(host)
	}

	wg.Wait()

	for _, entry := range app.DeviceSnapshot() {
		if _, ok := desired[entry.ID]; ok {
			continue
		}

		if app.removeDeviceIfMatch(entry.ID, entry.Device) {
			removed++
		}
	}

	return inserted, removed, desired, nil
}

// SeedExtraDevicesUntilReady retries only the hosts returned by
// ExtraDeviceHosts until all of them have been registered or ctx expires. It
// does not run mDNS or UPnP discovery. This gives embedded deployments a
// bounded way to recover when their persisted speakers are not yet routable
// while the service is starting.
func (app *WebApp) SeedExtraDevicesUntilReady(ctx context.Context, retryInterval time.Duration) {
	retryUntilReady(ctx, retryInterval, func() bool {
		inserted, removed, desired, err := app.seedExtraDevices(ctx)
		if err != nil {
			log.Printf("SeedExtraDevicesUntilReady: failed to read extra device hosts, will retry: %v", err)
			return false
		}

		if len(inserted) > 0 || removed > 0 {
			app.BroadcastDeviceList()
		}

		return app.extraDeviceHostsPresent(desired)
	})
}

func (app *WebApp) extraDeviceHosts() ([]string, error) {
	if app.ExtraDeviceHosts == nil {
		return nil, nil
	}

	rawHosts, err := app.ExtraDeviceHosts()
	if err != nil {
		return nil, err
	}

	hosts := make([]string, 0, len(rawHosts))
	seen := make(map[string]struct{}, len(rawHosts))

	for _, host := range rawHosts {
		if host == "" {
			continue
		}

		if _, ok := seen[host]; ok {
			continue
		}

		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}

	return hosts, nil
}

func (app *WebApp) extraDeviceHostSet() (map[string]struct{}, error) {
	hosts, err := app.extraDeviceHosts()
	if err != nil {
		return nil, err
	}

	desired := make(map[string]struct{}, len(hosts))

	for _, host := range hosts {
		desired[host] = struct{}{}
	}

	return desired, nil
}

func (app *WebApp) extraDeviceHostsPresent(desired map[string]struct{}) bool {
	for host := range desired {
		if _, ok := app.GetDevice(host); !ok {
			return false
		}
	}

	return true
}

func retryUntilReady(ctx context.Context, retryInterval time.Duration, attempt func() bool) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if attempt() {
			return
		}

		timer := time.NewTimer(retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

// DiscoverDevices refreshes the device registry. When TriggerDiscovery is set
// (embedded build), it runs the host service's discovery so the shared store is
// refreshed, then re-syncs from ExtraDeviceHosts — it does NOT run its own
// mDNS/UPnP, so the embedded build never duplicates the service's discovery.
// When discoveryService is non-nil (standalone soundtouch-player), it runs an
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
	if _, _, _, err := app.seedExtraDevices(ctx); err != nil {
		log.Printf("DiscoverDevices: failed to read extra device hosts: %v", err)
	}

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
		app.addDeviceByHost(ctx, device.Host, device.Port, classifySource(device.DiscoveryMethod))
	}
}

// classifySource labels a discovered device "manual" if it came from (at
// least in part) a configured host, "discovered" otherwise. discoveryMethod
// can be a "+"-joined composite (e.g. "Configuration+mDNS/Bonjour") when
// mergeDeviceData combines a configured host with the same device found via
// mDNS/UPnP in the same sweep -- match by substring, not exact equality, so
// a manually configured host that's also independently discoverable still
// gets labeled "manual".
func classifySource(discoveryMethod string) string {
	if strings.Contains(discoveryMethod, "Configuration") {
		return "manual"
	}

	return "discovered"
}
