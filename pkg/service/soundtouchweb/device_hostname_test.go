package soundtouchweb

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
)

func speakerHostname(t *testing.T, speaker *fakeSpeaker) string {
	t.Helper()
	_, port, err := net.SplitHostPort(speaker.host)
	if err != nil {
		t.Fatal(err)
	}
	return net.JoinHostPort("localhost", port)
}

func TestConfiguredHostnameSurvivesDiscovery(t *testing.T) {
	for _, hostnameFirst := range []bool{true, false} {
		name := "IP first"
		if hostnameFirst {
			name = "hostname first"
		}
		t.Run(name, func(t *testing.T) {
			speaker := newFakeSpeaker(t, "DEVICEID01")
			hostname := speakerHostname(t, speaker)
			app := NewWebApp()
			cleanupRegistry(t, app)
			app.RemoveDeviceHook = func(string) error {
				t.Error("alias reconciliation must not delete the physical speaker from the datastore")
				return nil
			}
			var old *webtypes.DeviceConnection
			if !hostnameFirst {
				old = app.addDeviceByHost(context.Background(), speaker.host, 8090, "discovered")
			}
			preferred := app.addDeviceByHost(context.Background(), hostname, 8090, "manual")
			if preferred == nil {
				t.Fatal("manual hostname was not registered")
			}
			// Repeated discovery must not restore the numeric alias or replace
			// the connection used by controls and event subscriptions.
			for range 3 {
				app.AddDeviceByHost(speaker.host, 8090, "discovered")
				if got := app.DeviceCount(); got != 1 {
					t.Errorf("device count = %d, want one speaker", got)
				}
				if got, _ := app.GetDevice(hostname); got != preferred {
					t.Error("manual hostname connection was replaced")
				}
				if _, ok := app.GetDevice(speaker.host); ok {
					t.Error("discovered numeric alias is still registered")
				}
			}
			if old != nil {
				select {
				case <-old.Done():
				default:
					t.Error("retired numeric alias still has background work")
				}
			}
			views := app.deviceViewSnapshot()
			if len(views) != 1 || views[hostname].Info == nil {
				t.Errorf("player inventory must contain only %q: %v", hostname, views)
			}
		})
	}
}

func TestConfiguredHostnameDoesNotOverrideExplicitOrUnrelatedAddresses(t *testing.T) {
	for _, tc := range []struct {
		name, hostnameSource, ipSource, hostnameID, ipID string
	}{
		{"discovered hostname", "discovered", "discovered", "DEVICEID01", "DEVICEID01"},
		{"manual IP", "manual", "manual", "DEVICEID01", "DEVICEID01"},
		{"service store IP", "manual", "service-store", "DEVICEID01", "DEVICEID01"},
		{"different speakers", "manual", "discovered", "DEVICEID01", "DEVICEID02"},
		{"blank identity", "manual", "discovered", " ", " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			named := newFakeSpeaker(t, tc.hostnameID)
			numeric := newFakeSpeaker(t, tc.ipID)
			app := NewWebApp()
			cleanupRegistry(t, app)
			app.AddDeviceByHost(speakerHostname(t, named), 8090, tc.hostnameSource)
			app.AddDeviceByHost(numeric.host, 8090, tc.ipSource)
			if got := app.DeviceCount(); got != 2 {
				t.Fatalf("device count = %d, want both live addresses", got)
			}
		})
	}
}

func TestConfiguredHostnameCanBePromotedAfterDiscovery(t *testing.T) {
	speaker := newFakeSpeaker(t, "DEVICEID01")
	hostname := speakerHostname(t, speaker)
	app := NewWebApp()
	cleanupRegistry(t, app)
	preferred := app.addDeviceByHost(context.Background(), hostname, 8090, "discovered")
	app.AddDeviceByHost(speaker.host, 8090, "discovered")
	app.AddDeviceByHost(hostname, 8090, "manual")
	app.AddDeviceByHost(hostname, 8090, "discovered")
	app.AddDeviceByHost(speaker.host, 8090, "discovered")
	if got := app.DeviceCount(); got != 1 {
		t.Errorf("device count = %d after manual promotion, want 1", got)
	}
	if got, _ := app.GetDevice(hostname); got != preferred {
		t.Error("promotion must retain the live connection")
	}
}

func TestConfiguredHostnamePromotionDoesNotTrustCachedIdentity(t *testing.T) {
	for _, identity := range []string{"", "DEVICEID02"} {
		t.Run("new identity "+identity, func(t *testing.T) {
			named := newFakeSpeaker(t, "DEVICEID01")
			numeric := newFakeSpeaker(t, "DEVICEID01")
			hostname := speakerHostname(t, named)
			app := NewWebApp()
			cleanupRegistry(t, app)
			app.AddDeviceByHost(hostname, 8090, "discovered")
			working := app.addDeviceByHost(context.Background(), numeric.host, 8090, "discovered")
			named.deviceID.Store(identity)
			app.AddDeviceByHost(hostname, 8090, "manual")
			if got, _ := app.GetDevice(numeric.host); got != working {
				t.Fatal("stale hostname promotion removed the working connection")
			}
		})
	}
}

func TestConfiguredHostnameDoesNotRemovePromotedNumericAddress(t *testing.T) {
	speaker := newFakeSpeaker(t, "DEVICEID01")
	hostname := speakerHostname(t, speaker)
	app := NewWebApp()
	cleanupRegistry(t, app)
	app.AddDeviceByHost(speaker.host, 8090, "discovered")
	app.AddDeviceByHost(speaker.host, 8090, "manual")
	app.AddDeviceByHost(speaker.host, 8090, "discovered")
	app.AddDeviceByHost(hostname, 8090, "manual")
	if got := app.DeviceCount(); got != 2 {
		t.Fatalf("device count = %d, want both explicitly configured addresses", got)
	}
}

func TestConfiguredHostnameFallsBackWhenItStopsAnsweringAsSpeaker(t *testing.T) {
	for _, identity := range []string{"", "DEVICEID02"} {
		t.Run("new identity "+identity, func(t *testing.T) {
			named := newFakeSpeaker(t, "DEVICEID01")
			numeric := newFakeSpeaker(t, "DEVICEID01")
			hostname := speakerHostname(t, named)
			app := NewWebApp()
			cleanupRegistry(t, app)
			old := app.addDeviceByHost(context.Background(), hostname, 8090, "manual")
			named.deviceID.Store(identity)
			app.AddDeviceByHost(numeric.host, 8090, "discovered")
			if got := app.DeviceCount(); got != 1 {
				t.Errorf("device count = %d, want 1", got)
			}
			if _, ok := app.GetDevice(numeric.host); !ok {
				t.Error("working numeric address was suppressed by stale hostname")
			}
			select {
			case <-old.Done():
			default:
				t.Error("stale hostname was not retired")
			}
		})
	}
}

func TestConfiguredHostnameWinsConcurrentRegistration(t *testing.T) {
	speaker := newFakeSpeaker(t, "DEVICEID01")
	hostname := speakerHostname(t, speaker)
	app := NewWebApp()
	cleanupRegistry(t, app)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			app.AddDeviceByHost(hostname, 8090, "manual")
		}()
		go func() {
			defer wg.Done()
			<-start
			app.AddDeviceByHost(speaker.host, 8090, "discovered")
		}()
	}
	close(start)
	wg.Wait()
	if got := app.DeviceCount(); got != 1 {
		t.Errorf("device count = %d, want one preferred address", got)
	}
	if _, ok := app.GetDevice(hostname); !ok {
		t.Error("manual hostname lost concurrent reconciliation")
	}
}
