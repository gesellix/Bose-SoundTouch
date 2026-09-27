package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/handlers"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb"
)

// fakeSpeaker starts a real loopback listener answering /info, the only
// endpoint AddDeviceByHost needs to register a device. Mirrors the fixture
// shape soundtouchweb's own discovery tests use.
func fakeSpeaker(t *testing.T) string {
	t.Helper()

	server := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<info deviceID="TESTDEVICE"><name>Test speaker</name><type>SoundTouch 10</type></info>`))
	}))
	server.Start()

	return strings.TrimPrefix(server.URL, "http://")
}

// TestPlayerDevicesChangedHook covers the no-seeding-when-disabled
// requirement of issue 762 at the level main.go actually wires it: the hook
// registered via server.SetDevicesChangedHook must reseed the player's
// device registry while enabled, and must not touch it at all while
// disabled.
func TestPlayerDevicesChangedHook(t *testing.T) {
	host := fakeSpeaker(t)

	t.Run("enabled: reseeds from ExtraDeviceHosts", func(t *testing.T) {
		ds := datastore.NewDataStore(t.TempDir())
		_ = ds.Initialize()

		server := handlers.NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)
		webApp := soundtouchweb.NewWebApp()
		webApp.ExtraDeviceHosts = func() ([]string, error) { return []string{host}, nil }

		playerDevicesChangedHook(server, webApp)()

		if got := webApp.DeviceCount(); got != 1 {
			t.Fatalf("device count = %d, want 1", got)
		}
	})

	t.Run("disabled: no seeding at all", func(t *testing.T) {
		ds := datastore.NewDataStore(t.TempDir())
		_ = ds.Initialize()

		server := handlers.NewServer(ds, nil, "http://127.0.0.1:8000", false, false, false)
		server.SetPlayerEnabled(false)

		webApp := soundtouchweb.NewWebApp()
		webApp.ExtraDeviceHosts = func() ([]string, error) { return []string{host}, nil }

		playerDevicesChangedHook(server, webApp)()

		if got := webApp.DeviceCount(); got != 0 {
			t.Fatalf("device count = %d, want 0 (no seeding while disabled)", got)
		}
	})
}

// TestPlayerEnabledChangedHook covers the live-toggle side effect: switching
// the player off must stop the per-device background polling by removing
// every registered device (RemoveDevice tears down its goroutines), and
// switching it back on must reseed.
func TestPlayerEnabledChangedHook(t *testing.T) {
	host := fakeSpeaker(t)

	webApp := soundtouchweb.NewWebApp()
	webApp.ExtraDeviceHosts = func() ([]string, error) { return []string{host}, nil }

	hook := playerEnabledChangedHook(webApp)

	hook(true)

	if got := webApp.DeviceCount(); got != 1 {
		t.Fatalf("after enabling: device count = %d, want 1", got)
	}

	hook(false)

	if got := webApp.DeviceCount(); got != 0 {
		t.Fatalf("after disabling: device count = %d, want 0 (devices removed, polling stopped)", got)
	}

	hook(true)

	if got := webApp.DeviceCount(); got != 1 {
		t.Fatalf("after re-enabling: device count = %d, want 1 (reseeded)", got)
	}
}
