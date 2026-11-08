package soundtouchweb

import (
	"log"
	"net/netip"
	"strings"
	"sync"

	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

// retireStaleAliases removes registry entries that claim the same DeviceID as
// conn, which was just registered under host, but no longer answer as that
// speaker. The registry is keyed by the address a speaker was reached at, so a
// DHCP lease change otherwise leaves the old address behind as a dead entry
// next to the new one (issue 726).
//
// Each other claimant is asked for /info again rather than judged by its
// cached connectivity: a speaker that moved a moment ago can still look Stale
// rather than Offline. An entry that answers with the same DeviceID is a live
// alias and is kept unless it is a discovered numeric address shadowing a
// manually configured hostname. A failed alias, or one answering as a different
// speaker, is retired even when it was manually configured.
//
// Retiring only drops the registry entry and stops its goroutines. It must not
// go through RemoveDeviceHook: the service datastore is keyed by DeviceID, so
// the hook would delete the very speaker that just reappeared.
func (app *WebApp) retireStaleAliases(host string, conn *webtypes.DeviceConnection) {
	info := conn.Info()
	if info == nil {
		return
	}

	deviceID := strings.TrimSpace(info.DeviceID)
	if deviceID == "" {
		return
	}

	var candidates []DeviceEntry

	for _, entry := range app.DeviceSnapshot() {
		if entry.ID == host || entry.Device == nil || entry.Device.Client == nil {
			continue
		}

		if other := entry.Device.Info(); other != nil && strings.TrimSpace(other.DeviceID) == deviceID {
			candidates = append(candidates, entry)
		}
	}

	if len(candidates) == 0 {
		return
	}

	// Probe concurrently: a dead address costs up to the client timeout.
	stale := make([]bool, len(candidates))

	var wg sync.WaitGroup

	for i, entry := range candidates {
		wg.Add(1)

		go func() {
			defer wg.Done()

			live, err := entry.Device.Client.GetDeviceInfo()
			stale[i] = err != nil || live == nil || strings.TrimSpace(live.DeviceID) != deviceID
		}()
	}

	wg.Wait()

	retired := false
	current := DeviceEntry{ID: host, Device: conn}

	for i, entry := range candidates {
		alias, survivor := entry, current
		if !stale[i] && prefersDeviceHostname(entry, current) {
			alias, survivor = current, entry
		}

		if app.retireDeviceAlias(alias, survivor, stale[i]) {
			log.Printf("Retired %s: device %s is available at %s",
				sanitizeLog(alias.ID), sanitizeLog(deviceID), sanitizeLog(survivor.ID))

			retired = true
		}
	}

	if retired {
		app.BroadcastDeviceList()
	}
}

func (app *WebApp) promoteManualDevice(host, source string) {
	if source != "manual" {
		return
	}

	app.devicesMu.Lock()
	conn := app.devices[host]

	promoted := false
	if conn != nil {
		promoted = conn.RecordDiscoverySource(source)
	}
	app.devicesMu.Unlock()

	if !promoted || conn.Client == nil || conn.Info() == nil {
		return
	}

	// The fast path reused a cached connection. Confirm that the hostname
	// still answers as this speaker before retiring a working IP alias.
	live, err := conn.Client.GetDeviceInfo()
	if err != nil || live == nil || strings.TrimSpace(live.DeviceID) != strings.TrimSpace(conn.Info().DeviceID) {
		return
	}

	app.retireStaleAliases(host, conn)
}

func prefersDeviceHostname(preferred, other DeviceEntry) bool {
	return preferred.Device.DiscoverySource() == "manual" &&
		other.Device.DiscoverySource() == "discovered" &&
		!isNumericDeviceHost(preferred.ID) && isNumericDeviceHost(other.ID)
}

func isNumericDeviceHost(host string) bool {
	_, err := netip.ParseAddr(strings.Trim(hostOnly(host), "[]"))

	return err == nil
}

// Require both connections to still be registered: concurrent reconciliation
// must neither remove a replacement nor retire both addresses of one speaker.
// Recheck preference while holding devicesMu so a concurrent manual promotion
// cannot lose an explicitly configured numeric address.
func (app *WebApp) retireDeviceAlias(alias, survivor DeviceEntry, stale bool) bool {
	release := app.beginDeviceSettingsOperation(alias.Device)
	defer release()

	app.devicesMu.Lock()

	remove := app.devices[alias.ID] == alias.Device && app.devices[survivor.ID] == survivor.Device &&
		(stale || prefersDeviceHostname(survivor, alias))
	if remove {
		delete(app.devices, alias.ID)
	}
	app.devicesMu.Unlock()

	if remove {
		alias.Device.Close()
	}

	return remove
}
