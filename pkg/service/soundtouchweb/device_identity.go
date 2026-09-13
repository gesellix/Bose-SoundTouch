package soundtouchweb

import (
	"log"
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
// alias (a .local name next to its IP, say) and is kept; one that fails, or
// answers with a different DeviceID because its address now belongs to another
// speaker, is retired.
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

	for i, entry := range candidates {
		if !stale[i] {
			continue
		}

		// removeDeviceIfMatch leaves a replacement registered under the same
		// key in the meantime untouched.
		if app.removeDeviceIfMatch(entry.ID, entry.Device) {
			log.Printf("Retired %s: device %s now answers at %s",
				sanitizeLog(entry.ID), sanitizeLog(deviceID), sanitizeLog(host))

			retired = true
		}
	}

	if retired {
		app.BroadcastDeviceList()
	}
}
