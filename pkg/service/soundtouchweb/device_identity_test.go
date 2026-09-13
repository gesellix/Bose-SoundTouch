package soundtouchweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSpeaker answers /info with whatever deviceID currently holds, or 503
// while deviceID is empty. It stands in for one address a speaker was reached
// at.
type fakeSpeaker struct {
	host     string
	deviceID atomic.Value
}

func newFakeSpeaker(t *testing.T, deviceID string) *fakeSpeaker {
	t.Helper()

	speaker := &fakeSpeaker{}
	speaker.deviceID.Store(deviceID)

	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}

		id, _ := speaker.deviceID.Load().(string)
		if id == "" {
			http.Error(w, "offline", http.StatusServiceUnavailable)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<info deviceID="` + id + `"><name>Kitchen</name><type>SoundTouch 10</type></info>`))
	}))
	server.Start()

	speaker.host = strings.TrimPrefix(server.URL, "http://")

	return speaker
}

// addFake registers speaker by host the way discovery does. The host already
// carries the httptest port, which the client keeps over the port argument.
func addFake(t *testing.T, app *WebApp, speaker *fakeSpeaker) {
	t.Helper()

	if app.addDeviceByHost(context.Background(), speaker.host, 8090, "manual") == nil {
		t.Fatalf("registering %s failed", speaker.host)
	}
}

func cleanupRegistry(t *testing.T, app *WebApp) {
	t.Helper()

	t.Cleanup(func() {
		for _, entry := range app.DeviceSnapshot() {
			app.RemoveDevice(entry.ID)
		}

		// See TestDiscoverDevicesRetriesConfiguredHosts: let the one-shot
		// status goroutines finish before the test servers close.
		time.Sleep(50 * time.Millisecond)
	})
}

func TestAddDeviceRetiresOldAddressOfMovedSpeaker(t *testing.T) {
	oldAddress := newFakeSpeaker(t, "DEVICEID01")
	newAddress := newFakeSpeaker(t, "DEVICEID01")

	app := NewWebApp()
	cleanupRegistry(t, app)

	addFake(t, app, oldAddress)

	// The lease changes: nobody answers at the old address any more.
	oldAddress.deviceID.Store("")

	addFake(t, app, newAddress)

	if _, ok := app.GetDevice(oldAddress.host); ok {
		t.Errorf("entry for the old address %s is still registered", oldAddress.host)
	}

	if _, ok := app.GetDevice(newAddress.host); !ok {
		t.Errorf("entry for the new address %s is missing", newAddress.host)
	}

	if got := app.DeviceCount(); got != 1 {
		t.Errorf("device count = %d, want 1", got)
	}
}

func TestAddDeviceRetiresAddressNowHeldByAnotherSpeaker(t *testing.T) {
	oldAddress := newFakeSpeaker(t, "DEVICEID01")
	newAddress := newFakeSpeaker(t, "DEVICEID01")

	app := NewWebApp()
	cleanupRegistry(t, app)

	addFake(t, app, oldAddress)

	// DHCP handed the old address to a different speaker.
	oldAddress.deviceID.Store("DEVICEID02")

	addFake(t, app, newAddress)

	if _, ok := app.GetDevice(oldAddress.host); ok {
		t.Errorf("entry for %s still claims DEVICEID01", oldAddress.host)
	}

	if got := app.DeviceCount(); got != 1 {
		t.Errorf("device count = %d, want 1", got)
	}
}

func TestAddDeviceKeepsLiveAliasOfSameSpeaker(t *testing.T) {
	first := newFakeSpeaker(t, "DEVICEID01")
	second := newFakeSpeaker(t, "DEVICEID01")

	app := NewWebApp()
	cleanupRegistry(t, app)

	addFake(t, app, first)
	addFake(t, app, second)

	// Both addresses still answer as the same speaker (a hostname next to its
	// IP): neither is a ghost, so neither is dropped.
	if got := app.DeviceCount(); got != 2 {
		t.Errorf("device count = %d, want 2", got)
	}
}

func TestAddDeviceLeavesOtherSpeakersAlone(t *testing.T) {
	kitchen := newFakeSpeaker(t, "DEVICEID01")
	lounge := newFakeSpeaker(t, "DEVICEID02")

	app := NewWebApp()
	cleanupRegistry(t, app)

	addFake(t, app, kitchen)
	kitchen.deviceID.Store("")
	addFake(t, app, lounge)

	// An unreachable speaker with a different DeviceID is not a duplicate of
	// the one being added and must stay listed.
	if _, ok := app.GetDevice(kitchen.host); !ok {
		t.Errorf("unrelated offline speaker %s was removed", kitchen.host)
	}
}
