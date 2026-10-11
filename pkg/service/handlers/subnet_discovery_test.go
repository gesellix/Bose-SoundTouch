package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
)

type discoveryFake struct {
	mu          sync.Mutex
	ssdpDevs    []*models.DiscoveredDevice
	ssdpErr     error
	scanDevs    []*models.DiscoveredDevice
	scanCalls   int
	scanPrefix  []netip.Prefix
	handled     []string
	ssdpCalls   int
	ssdpBlock   chan struct{}
	ifaceAddrs  []netip.Addr
	hostLookups map[string][]netip.Addr
}

func newDiscoveryFake(t *testing.T, serverURL string) (*Server, *discoveryFake) {
	t.Helper()

	f := &discoveryFake{hostLookups: map[string][]netip.Addr{}}
	s := NewServer(datastore.NewDataStore(t.TempDir()), nil, serverURL, false, false, false)
	s.discoveryDeps = discoveryDeps{
		ssdp: func(context.Context) ([]*models.DiscoveredDevice, error) {
			f.mu.Lock()
			f.ssdpCalls++
			block := f.ssdpBlock
			f.mu.Unlock()

			if block != nil {
				<-block
			}

			return f.ssdpDevs, f.ssdpErr
		},
		scan: func(_ context.Context, p []netip.Prefix, _ discovery.SubnetScanOptions) ([]*models.DiscoveredDevice, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.scanCalls++
			f.scanPrefix = p

			return f.scanDevs, nil
		},
		lookupHost: func(_ context.Context, h string) ([]netip.Addr, error) {
			if a, ok := f.hostLookups[h]; ok {
				return a, nil
			}

			return nil, errors.New("no such host")
		},
		interfaceAddrs: func() []netip.Addr { return f.ifaceAddrs },
		handle: func(d models.DiscoveredDevice) {
			f.mu.Lock()
			f.handled = append(f.handled, d.Host)
			f.mu.Unlock()
		},
	}

	return s, f
}

func dev(host string) *models.DiscoveredDevice {
	return &models.DiscoveredDevice{Host: host, Port: 8090}
}

func TestDiscover_SSDPFindsDevices_NoScan(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://192.168.1.2:8000")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("192.168.1.50")}
	f.scanDevs = []*models.DiscoveredDevice{dev("192.168.1.99")}

	s.DiscoverDevices(context.Background())

	if f.scanCalls != 0 || fmt.Sprint(f.handled) != "[192.168.1.50]" {
		t.Errorf("scan=%d handled=%v", f.scanCalls, f.handled)
	}
}

func TestDiscover_SSDPEmptyOrError_FallsBackToScan(t *testing.T) {
	for name, err := range map[string]error{"empty": nil, "error": errors.New("multicast unavailable")} {
		t.Run(name, func(t *testing.T) {
			s, f := newDiscoveryFake(t, "http://192.168.1.2:8000")
			f.ssdpErr = err
			f.scanDevs = []*models.DiscoveredDevice{dev("192.168.1.99")}

			notified := 0
			s.SetDevicesChangedHook(func() { notified++ })

			s.DiscoverDevices(context.Background())

			if f.scanCalls != 1 || fmt.Sprint(f.handled) != "[192.168.1.99]" || notified != 1 {
				t.Errorf("scan=%d handled=%v notified=%d", f.scanCalls, f.handled, notified)
			}

			if fmt.Sprint(f.scanPrefix) != "[192.168.1.0/24]" {
				t.Errorf("prefixes %v", f.scanPrefix)
			}
		})
	}
}

func TestDiscover_ExplicitSubnetsTakePrecedence(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://192.168.1.2:8000")
	s.SetDiscoverySubnets("203.0.113.0/24,junk")

	s.DiscoverDevices(context.Background())

	if fmt.Sprint(f.scanPrefix) != "[203.0.113.0/24]" {
		t.Errorf("prefixes %v", f.scanPrefix)
	}
}

func TestDiscoveryHints_Order(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://speakers.example:8000")
	f.hostLookups["speakers.example"] = []netip.Addr{netip.MustParseAddr("10.1.1.1")}
	f.ifaceAddrs = []netip.Addr{netip.MustParseAddr("172.17.0.2")}

	if err := s.ds.SaveDeviceInfo("acct", "DEV1", &models.ServiceDeviceInfo{DeviceID: "DEV1", IPAddress: "192.168.5.20"}); err != nil {
		t.Fatal(err)
	}

	got := s.discoveryHints(context.Background(), netip.MustParseAddr("192.168.9.9"))
	if fmt.Sprint(got) != "[10.1.1.1 192.168.5.20 192.168.9.9 172.17.0.2]" {
		t.Errorf("hints %v", got)
	}

	// literal IP serverURL, unresolvable hostname ignored, no remote.
	s.serverURL = "http://192.168.1.2:8000"
	if got := s.discoveryHints(context.Background(), netip.Addr{}); fmt.Sprint(got) != "[192.168.1.2 192.168.5.20 172.17.0.2]" {
		t.Errorf("hints %v", got)
	}

	s.serverURL = "http://nowhere.example"
	if got := s.discoveryHints(context.Background(), netip.Addr{}); fmt.Sprint(got) != "[192.168.5.20 172.17.0.2]" {
		t.Errorf("hints %v", got)
	}
}

func TestHandleTriggerDiscovery_ExplicitScanUsesRemoteAddrOnly(t *testing.T) {
	s, f := newDiscoveryFake(t, "")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("192.168.1.50")}
	f.scanDevs = []*models.DiscoveredDevice{dev("192.168.7.8")}

	req := httptest.NewRequest(http.MethodPost, "/setup/discover?scan=subnet", nil)
	req.RemoteAddr = "192.168.7.200:5555"
	req.Header.Set("X-Forwarded-For", "10.99.99.99")

	rec := httptest.NewRecorder()
	s.HandleTriggerDiscovery(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("code %d", rec.Code)
	}

	waitIdle(t, s)

	if f.scanCalls != 1 || fmt.Sprint(f.scanPrefix) != "[192.168.7.0/24]" {
		t.Errorf("scan=%d prefixes=%v", f.scanCalls, f.scanPrefix)
	}

	if fmt.Sprint(f.handled) != "[192.168.1.50 192.168.7.8]" {
		t.Errorf("handled %v", f.handled)
	}
}

func TestHandleTriggerDiscovery_PlainRequestSkipsScanWhenSSDPFinds(t *testing.T) {
	s, f := newDiscoveryFake(t, "")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("192.168.1.50")}

	req := httptest.NewRequest(http.MethodPost, "/setup/discover", nil)
	req.RemoteAddr = "192.168.7.200:5555"
	s.HandleTriggerDiscovery(httptest.NewRecorder(), req)
	waitIdle(t, s)

	if f.scanCalls != 0 {
		t.Errorf("scan=%d", f.scanCalls)
	}
}

func TestDiscover_NoOverlap(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://192.168.1.2:8000")
	f.ssdpBlock = make(chan struct{})

	done := make(chan struct{})

	go func() { s.DiscoverDevices(context.Background()); close(done) }()

	deadline := time.Now().Add(2 * time.Second)
	for !s.discoveryRunning.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	s.DiscoverDevices(context.Background()) // must return immediately

	close(f.ssdpBlock)
	<-done

	if f.ssdpCalls != 1 {
		t.Errorf("ssdp calls %d", f.ssdpCalls)
	}
}

func waitIdle(t *testing.T, s *Server) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !s.discoveryRunning.Load() {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatal("discovery did not finish")
}
