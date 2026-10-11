package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/datastore"
	"github.com/gesellix/bose-soundtouch/pkg/service/setup"
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
	s, f := newDiscoveryFake(t, "http://10.0.0.2:8000")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("198.51.100.50")}
	f.scanDevs = []*models.DiscoveredDevice{dev("198.51.100.99")}

	s.DiscoverDevices(context.Background())

	if f.scanCalls != 0 || fmt.Sprint(f.handled) != "[198.51.100.50]" {
		t.Errorf("scan=%d handled=%v", f.scanCalls, f.handled)
	}
}

func TestDiscover_SSDPEmptyOrError_FallsBackToScan(t *testing.T) {
	for name, err := range map[string]error{"empty": nil, "error": errors.New("multicast unavailable")} {
		t.Run(name, func(t *testing.T) {
			s, f := newDiscoveryFake(t, "http://10.0.0.2:8000")
			f.ssdpErr = err
			f.scanDevs = []*models.DiscoveredDevice{dev("198.51.100.99")}

			notified := 0
			s.SetDevicesChangedHook(func() { notified++ })

			s.DiscoverDevices(context.Background())

			if f.scanCalls != 1 || fmt.Sprint(f.handled) != "[198.51.100.99]" || notified != 1 {
				t.Errorf("scan=%d handled=%v notified=%d", f.scanCalls, f.handled, notified)
			}

			if fmt.Sprint(f.scanPrefix) != "[10.0.0.0/24]" {
				t.Errorf("prefixes %v", f.scanPrefix)
			}
		})
	}
}

func TestDiscover_ExplicitSubnetsTakePrecedence(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://10.0.0.2:8000")
	s.SetDiscoverySubnets("203.0.113.0/24,junk")

	s.DiscoverDevices(context.Background())

	if fmt.Sprint(f.scanPrefix) != "[203.0.113.0/24]" {
		t.Errorf("prefixes %v", f.scanPrefix)
	}
}

func TestDiscoveryHints_Order(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://speakers.example:8000")
	f.hostLookups["speakers.example"] = []netip.Addr{netip.MustParseAddr("10.0.1.1")}
	f.ifaceAddrs = []netip.Addr{netip.MustParseAddr("10.0.2.2")}

	if err := s.ds.SaveDeviceInfo("acct", "DEV1", &models.ServiceDeviceInfo{DeviceID: "DEV1", IPAddress: "10.0.3.20"}); err != nil {
		t.Fatal(err)
	}

	got := s.discoveryHints(context.Background(), netip.MustParseAddr("10.0.4.9"))
	if fmt.Sprint(got) != "[10.0.1.1 10.0.3.20 10.0.4.9 10.0.2.2]" {
		t.Errorf("hints %v", got)
	}

	// literal IP serverURL, unresolvable hostname ignored, no remote.
	s.serverURL = "http://10.0.0.2:8000"
	if got := s.discoveryHints(context.Background(), netip.Addr{}); fmt.Sprint(got) != "[10.0.0.2 10.0.3.20 10.0.2.2]" {
		t.Errorf("hints %v", got)
	}

	s.serverURL = "http://nowhere.example"
	if got := s.discoveryHints(context.Background(), netip.Addr{}); fmt.Sprint(got) != "[10.0.3.20 10.0.2.2]" {
		t.Errorf("hints %v", got)
	}
}

func TestHandleTriggerDiscovery_UsesRemoteAddrOnly(t *testing.T) {
	s, f := newDiscoveryFake(t, "")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("198.51.100.50")}
	f.scanDevs = []*models.DiscoveredDevice{dev("10.0.5.8")}

	req := httptest.NewRequest(http.MethodPost, "/setup/discover", nil)
	req.RemoteAddr = "10.0.5.200:5555"
	req.Header.Set("X-Forwarded-For", "10.99.99.99")

	rec := httptest.NewRecorder()
	s.HandleTriggerDiscovery(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("code %d", rec.Code)
	}

	waitIdle(t, s)

	if f.scanCalls != 1 || fmt.Sprint(f.scanPrefix) != "[10.0.5.0/24]" {
		t.Errorf("scan=%d prefixes=%v", f.scanCalls, f.scanPrefix)
	}

	if fmt.Sprint(f.handled) != "[198.51.100.50 10.0.5.8]" {
		t.Errorf("handled %v", f.handled)
	}
}

func TestHandleTriggerDiscovery_AlwaysScansEvenWhenSSDPFinds(t *testing.T) {
	s, f := newDiscoveryFake(t, "")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("198.51.100.50")}
	f.scanDevs = []*models.DiscoveredDevice{dev("10.0.5.8")}

	req := httptest.NewRequest(http.MethodPost, "/api/setup/discover", nil)
	req.RemoteAddr = "10.0.5.200:5555"
	s.HandleTriggerDiscovery(httptest.NewRecorder(), req)
	waitIdle(t, s)

	if f.scanCalls != 1 {
		t.Errorf("scan=%d, admin trigger must always scan", f.scanCalls)
	}
}

func TestMergeDiscovered_DropsDuplicateHosts(t *testing.T) {
	base := []*models.DiscoveredDevice{dev("198.51.100.50"), dev("198.51.100.51")}
	extra := []*models.DiscoveredDevice{dev("198.51.100.51"), dev("10.0.5.8"), dev("10.0.5.8")}

	got := mergeDiscovered(base, extra)

	var hosts []string
	for _, d := range got {
		hosts = append(hosts, d.Host)
	}

	if fmt.Sprint(hosts) != "[198.51.100.50 198.51.100.51 10.0.5.8]" {
		t.Errorf("hosts %v", hosts)
	}
}

func TestDiscover_SSDPAndScanOverlapHandledOnce(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://10.0.0.2:8000")
	f.ssdpDevs = []*models.DiscoveredDevice{dev("10.0.0.50")}
	f.scanDevs = []*models.DiscoveredDevice{dev("10.0.0.50"), dev("10.0.0.51")}

	s.runDiscovery(context.Background(), netip.Addr{}, true)

	if fmt.Sprint(f.handled) != "[10.0.0.50 10.0.0.51]" {
		t.Errorf("handled %v", f.handled)
	}
}

func TestSubnetScan_LogsPrefixesWithoutResponseContent(t *testing.T) {
	var buf bytes.Buffer

	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	s, f := newDiscoveryFake(t, "")
	s.SetDiscoverySubnets("203.0.113.0/24")
	f.scanDevs = []*models.DiscoveredDevice{{Host: "203.0.113.7", Name: "SECRET-NAME"}}

	s.DiscoverDevices(context.Background())

	out := buf.String()
	if !strings.Contains(out, "203.0.113.0/24") {
		t.Errorf("log lacks scanned prefix: %s", out)
	}

	if strings.Contains(out, "SECRET-NAME") {
		t.Errorf("log leaks device content: %s", out)
	}
}

// TestDiscover_ScanResultIsPersisted drives the real handleDiscoveredDevice
// against a local fake speaker, so the datastore write is exercised.
func TestDiscover_ScanResultIsPersisted(t *testing.T) {
	const infoXML = `<?xml version="1.0" encoding="UTF-8" ?><info deviceID="DEVICEID01"><name>Fake Speaker</name><type>SoundTouch 10</type></info>`

	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(infoXML))
	}))
	t.Cleanup(speaker.Close)

	hostPort := strings.TrimPrefix(speaker.URL, "http://")

	ds := datastore.NewDataStore(t.TempDir())
	s := NewServer(ds, setup.NewManager("http://localhost:8000", ds, nil), "", false, false, false)
	s.discoveryDeps = discoveryDeps{
		ssdp:           func(context.Context) ([]*models.DiscoveredDevice, error) { return nil, nil },
		lookupHost:     func(context.Context, string) ([]netip.Addr, error) { return nil, errors.New("offline") },
		interfaceAddrs: func() []netip.Addr { return nil },
		scan: func(context.Context, []netip.Prefix, discovery.SubnetScanOptions) ([]*models.DiscoveredDevice, error) {
			return []*models.DiscoveredDevice{{
				Name: "Fake Speaker", Host: hostPort, Port: 8090, SerialNo: "DEVICEID01",
				DiscoveryMethod: discovery.SubnetScanMethod,
			}}, nil
		},
		handle: s.handleDiscoveredDevice,
	}
	s.SetDiscoverySubnets("203.0.113.0/24")

	s.DiscoverDevices(context.Background())

	devices, err := ds.ListAllDevices()
	if err != nil {
		t.Fatal(err)
	}

	if len(devices) != 1 || devices[0].DeviceID != "DEVICEID01" || devices[0].IPAddress != hostPort {
		t.Fatalf("persisted devices: %+v", devices)
	}
}

func TestDiscover_NoOverlap(t *testing.T) {
	s, f := newDiscoveryFake(t, "http://10.0.0.2:8000")
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
