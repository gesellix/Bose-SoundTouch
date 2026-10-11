package discovery

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const validInfoXML = `<?xml version="1.0" encoding="UTF-8" ?><info deviceID="AABBCCDDEEFF"><name>Test Speaker</name><type>SoundTouch 10</type><components><component><componentCategory>SCM</componentCategory><serialNumber>SN123</serialNumber></component></components></info>`

func loopbackServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, int) {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}

	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)

	return srv, ln.Addr().(*net.TCPAddr).Port
}

func infoHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/info" {
			http.NotFound(w, r)
			return
		}

		_, _ = w.Write([]byte(body))
	}
}

func TestScanSubnets_FindsSoundTouch(t *testing.T) {
	_, port := loopbackServer(t, infoHandler(validInfoXML))

	found, err := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		SubnetScanOptions{Port: port, MaxHosts: 4})
	if err != nil {
		t.Fatalf("ScanSubnets: %v", err)
	}

	if len(found) != 1 {
		t.Fatalf("want 1 device, got %d", len(found))
	}

	d := found[0]
	if d.DiscoveryMethod != "subnet-scan" || d.Host != "127.0.0.1" || d.Port != port ||
		d.Name != "Test Speaker" || d.ModelID != "SoundTouch 10" || d.SerialNo != "AABBCCDDEEFF" {
		t.Errorf("unexpected device: %+v", d)
	}

	wantBase := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if d.APIBaseURL != wantBase || d.InfoURL != wantBase+"info" {
		t.Errorf("urls: %q %q", d.APIBaseURL, d.InfoURL)
	}
}

func TestScanSubnets_RejectsNonSoundTouch(t *testing.T) {
	bodies := map[string]string{
		"html":          `<html>hi</html>`,
		"wrong root":    `<device deviceID="X"/>`,
		"missing id":    `<info><name>x</name></info>`,
		"blank id":      `<info deviceID="  "><name>x</name></info>`,
		"not xml":       `nope`,
		"trailing junk": validInfoXML + `<<<`,
	}

	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			_, port := loopbackServer(t, infoHandler(body))

			found, err := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
				SubnetScanOptions{Port: port})
			if err != nil || len(found) != 0 {
				t.Errorf("want none, got %v err=%v", found, err)
			}
		})
	}
}

func TestScanSubnets_RejectsNon200AndRedirects(t *testing.T) {
	_, other := loopbackServer(t, infoHandler(validInfoXML))

	_, redirPort := loopbackServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, fmt.Sprintf("http://127.0.0.1:%d/info", other), http.StatusFound)
	})
	_, errPort := loopbackServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(validInfoXML))
	})

	for _, p := range []int{redirPort, errPort} {
		found, _ := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}, SubnetScanOptions{Port: p})
		if len(found) != 0 {
			t.Errorf("port %d: want none, got %v", p, found)
		}
	}
}

func TestScanSubnets_BoundedBody(t *testing.T) {
	huge := `<info deviceID="AA"><name>` + strings.Repeat("a", 2<<20) + `</name></info>`

	_, port := loopbackServer(t, infoHandler(huge))

	found, _ := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}, SubnetScanOptions{Port: port})
	if len(found) != 0 {
		t.Errorf("oversized body must be rejected")
	}
}

func TestScanSubnets_SkipsNetworkAndBroadcast(t *testing.T) {
	var dialed []string

	hosts := hostsOf(t, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/30")}, 100)
	for _, h := range hosts {
		dialed = append(dialed, h.String())
	}

	if len(dialed) != 2 || dialed[0] != "192.0.2.1" || dialed[1] != "192.0.2.2" {
		t.Errorf("got %v", dialed)
	}

	// /31 and /32 keep all addresses.
	if n := len(hostsOf(t, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/31")}, 100)); n != 2 {
		t.Errorf("/31: %d", n)
	}

	if n := len(hostsOf(t, []netip.Prefix{netip.MustParsePrefix("192.0.2.7/32")}, 100)); n != 1 {
		t.Errorf("/32: %d", n)
	}
}

func hostsOf(t *testing.T, p []netip.Prefix, max int) []netip.Addr {
	t.Helper()

	h, _ := scanHosts(p, max)

	return h
}

func TestScanHosts_CapsAndDedups(t *testing.T) {
	hosts, truncated := scanHosts([]netip.Prefix{
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("192.0.2.0/25"), // overlap
		netip.MustParsePrefix("198.51.100.0/24"),
	}, 300)

	if len(hosts) != 300 || !truncated {
		t.Errorf("len=%d truncated=%v", len(hosts), truncated)
	}

	seen := map[netip.Addr]bool{}
	for _, h := range hosts {
		if seen[h] {
			t.Fatalf("duplicate %v", h)
		}

		seen[h] = true
	}

	if h, tr := scanHosts([]netip.Prefix{netip.MustParsePrefix("2001:db8::/64")}, 10); len(h) != 0 || tr {
		t.Errorf("ipv6 must be ignored")
	}
}

func TestScanSubnets_ContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := ScanSubnets(ctx, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")}, SubnetScanOptions{})

	if err == nil {
		t.Error("want ctx error")
	}

	if time.Since(start) > time.Second {
		t.Error("cancel not honoured")
	}
}

type countingTransport struct {
	cur, max atomic.Int32
	calls    atomic.Int32
}

func (c *countingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	n := c.cur.Add(1)
	defer c.cur.Add(-1)

	for {
		m := c.max.Load()
		if n <= m || c.max.CompareAndSwap(m, n) {
			break
		}
	}

	c.calls.Add(1)
	time.Sleep(5 * time.Millisecond)

	return nil, fmt.Errorf("refused")
}

func TestScanSubnets_ConcurrencyAndCapAndSpeed(t *testing.T) {
	ct := &countingTransport{}
	client := &http.Client{Transport: ct}

	start := time.Now()
	found, err := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("10.0.0.0/16")},
		SubnetScanOptions{Concurrency: 8, MaxHosts: 254, HTTPClient: client})

	if err != nil || len(found) != 0 {
		t.Fatalf("found=%v err=%v", found, err)
	}

	if got := ct.calls.Load(); got != 254 {
		t.Errorf("calls=%d want 254 (cap)", got)
	}

	if m := ct.max.Load(); m > 8 || m < 2 {
		t.Errorf("max concurrency %d", m)
	}

	if time.Since(start) > 10*time.Second {
		t.Errorf("too slow: %v", time.Since(start))
	}
}

func TestScanSubnets_Defaults(t *testing.T) {
	o := SubnetScanOptions{}.withDefaults()
	if o.Port != 8090 || o.DialTimeout != 400*time.Millisecond || o.InfoTimeout != 2*time.Second ||
		o.Concurrency != 64 || o.MaxHosts != 1024 || o.HTTPClient != nil {
		t.Errorf("%+v", o)
	}

	_ = strconv.Itoa
}
