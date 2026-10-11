package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"sync"
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

func hostsOf(t *testing.T, p []netip.Prefix, limit int) []netip.Addr {
	t.Helper()

	h, _ := scanHosts(p, limit)

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

func TestCandidatePrefixes(t *testing.T) {
	p := netip.MustParsePrefix
	a := netip.MustParseAddr

	t.Run("explicit wins even when public or loopback", func(t *testing.T) {
		got := CandidatePrefixes([]netip.Prefix{p("203.0.113.0/24"), p("127.0.0.0/30"), p("203.0.113.9/24")},
			[]netip.Addr{a("10.0.0.5")})
		want := []netip.Prefix{p("203.0.113.0/24"), p("127.0.0.0/30")}

		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("got %v want %v", got, want)
		}
	})

	t.Run("hints become deduped /24 in order", func(t *testing.T) {
		got := CandidatePrefixes(nil, []netip.Addr{a("10.0.0.5"), a("10.0.1.5"), a("10.0.0.77")})
		want := []netip.Prefix{p("10.0.0.0/24"), p("10.0.1.0/24")}

		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("got %v want %v", got, want)
		}
	})

	t.Run("non-private hints filtered", func(t *testing.T) {
		got := CandidatePrefixes(nil, []netip.Addr{
			a("8.8.8.8"), a("127.0.0.1"), a("169.254.1.1"), a("224.0.0.1"), a("0.0.0.0"),
			a("::1"), a("fe80::1"), a("fd00::1"), {}, a("10.0.0.5"), a("::ffff:10.0.1.5"),
		})
		want := []netip.Prefix{p("10.0.0.0/24"), p("10.0.1.0/24")}

		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("got %v want %v", got, want)
		}
	})

	t.Run("invalid and ipv6 explicit ignored", func(t *testing.T) {
		got := CandidatePrefixes([]netip.Prefix{{}, p("2001:db8::/64")}, []netip.Addr{a("10.0.0.5")})
		if fmt.Sprint(got) != fmt.Sprint([]netip.Prefix{p("10.0.0.0/24")}) {
			t.Errorf("got %v", got)
		}
	})

	if got := CandidatePrefixes(nil, nil); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestParseSubnets(t *testing.T) {
	got, invalid := ParseSubnets(" 203.0.113.0/24, bogus,10.0.0.0/8,,2001:db8::/32, 203.0.113.9 ,203.0.113.0/24")

	if fmt.Sprint(got) != "[203.0.113.0/24 10.0.0.0/8]" {
		t.Errorf("got %v", got)
	}

	if fmt.Sprint(invalid) != "[bogus 2001:db8::/32 203.0.113.9]" {
		t.Errorf("invalid %v", invalid)
	}

	if g, i := ParseSubnets(""); len(g) != 0 || len(i) != 0 {
		t.Errorf("empty: %v %v", g, i)
	}
}

func TestScanOptions_HardCaps(t *testing.T) {
	o := SubnetScanOptions{Concurrency: 1 << 20, MaxHosts: 1 << 30, Port: 70000}.withDefaults()
	if o.Concurrency != 64 || o.MaxHosts != 1024 || o.Port != 8090 {
		t.Errorf("%+v", o)
	}

	// Values below the caps are respected.
	o = SubnetScanOptions{Concurrency: 8, MaxHosts: 10, Port: 65535}.withDefaults()
	if o.Concurrency != 8 || o.MaxHosts != 10 || o.Port != 65535 {
		t.Errorf("%+v", o)
	}
}

func TestScanSubnets_ExcessiveMaxHostsClamped(t *testing.T) {
	ct := &countingTransport{}

	_, err := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")},
		SubnetScanOptions{Concurrency: 100000, MaxHosts: 1 << 30, HTTPClient: &http.Client{Transport: ct}})
	if err != nil {
		t.Fatal(err)
	}

	if got := ct.calls.Load(); got != 1024 {
		t.Errorf("calls=%d want 1024", got)
	}

	if m := ct.max.Load(); m > 64 {
		t.Errorf("max concurrency %d", m)
	}
}

func TestParseInfo_SingleDocument(t *testing.T) {
	const root = `<info deviceID="DEVICEID01"/>`

	bad := map[string]string{
		"second root":          root + `<other/>`,
		"second info":          root + root,
		"trailing text":        root + "garbage",
		"trailing text ws":     root + "\n garbage \n",
		"trailing cdata":       root + "<![CDATA[x]]>",
		"trailing close tag":   root + "</info>",
		"trailing pi":          root + "<?pi x?>",
		"leading text":         "garbage" + root,
		"leading text ws":      " garbage \n" + root,
		"leading cdata":        "<![CDATA[x]]>" + root,
		"leading blank cdata":  "<![CDATA[ ]]>" + root,
		"trailing blank cdata": root + "<![CDATA[ ]]>",
		"non-XML whitespace":   root + "\u00a0",
		"leading close tag":    "</info>" + root,
		"decl after comment":   "<!-- c -->" + `<?xml version="1.0"?>` + root,
		"decl after ws":        "\n" + `<?xml version="1.0"?>` + root,
		"decl after root":      root + `<?xml version="1.0"?>`,
		"second decl":          `<?xml version="1.0"?><?xml version="1.0"?>` + root,
		"other pi before":      `<?pi x?>` + root,
	}
	for name, body := range bad {
		if _, err := parseInfo([]byte(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	good := map[string]string{
		"plain":            root,
		"whitespace":       "\n  " + root + " \r\n\t",
		"comment after":    root + "<!-- ok -->\n",
		"comment before":   "<!-- hi -->" + root,
		"decl and comment": `<?xml version="1.0"?>` + "\n<!-- c -->" + root + "\n<!-- d -->\n",
		"decl then ws":     `<?xml version="1.0" encoding="UTF-8"?>` + "\n" + root,
	}
	for name, body := range good {
		if _, err := parseInfo([]byte(body)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestScanSubnets_SlowInfoHitsDeadline(t *testing.T) {
	release := make(chan struct{})

	_, port := loopbackServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	})
	t.Cleanup(func() { close(release) })

	start := time.Now()
	found, err := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		SubnetScanOptions{Port: port, InfoTimeout: 150 * time.Millisecond})

	if err != nil || len(found) != 0 {
		t.Errorf("found=%v err=%v", found, err)
	}

	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("deadline not enforced: %v", d)
	}
}

func TestScanSubnets_CancelDuringProbe(t *testing.T) {
	arrived := make(chan struct{}, 1)
	release := make(chan struct{})

	_, port := loopbackServer(t, func(w http.ResponseWriter, r *http.Request) {
		select {
		case arrived <- struct{}{}:
		default:
		}

		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
	})
	t.Cleanup(func() { close(release) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		found int
		err   error
	}

	done := make(chan result, 1)

	go func() {
		found, err := ScanSubnets(ctx, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
			SubnetScanOptions{Port: port, InfoTimeout: 30 * time.Second})
		done <- result{len(found), err}
	}()

	select {
	case <-arrived:
	case <-time.After(5 * time.Second):
		t.Fatal("probe never reached server")
	}

	cancel()

	select {
	case r := <-done:
		if r.err == nil || r.found != 0 {
			t.Errorf("found=%d err=%v", r.found, r.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("scan did not stop after cancel")
	}
}

type recordingHandler struct {
	mu   sync.Mutex
	logs []string
}

func (h *recordingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *recordingHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *recordingHandler) WithGroup(string) slog.Handler            { return h }
func (h *recordingHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})

	h.mu.Lock()
	h.logs = append(h.logs, b.String())
	h.mu.Unlock()

	return nil
}

func TestScanSubnets_LogsDurationHostsAndPrefixes(t *testing.T) {
	rec := &recordingHandler{}
	old := slog.Default()

	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(old) })

	_, port := loopbackServer(t, infoHandler(validInfoXML))

	_, err := ScanSubnets(context.Background(), []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")},
		SubnetScanOptions{Port: port})
	if err != nil {
		t.Fatal(err)
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.logs) != 1 {
		t.Fatalf("logs %v", rec.logs)
	}

	line := rec.logs[0]
	for _, want := range []string{"duration=", "hosts=1", "prefixes=[127.0.0.1/32]", "found=1"} {
		if !strings.Contains(line, want) {
			t.Errorf("log %q lacks %q", line, want)
		}
	}

	if strings.Contains(line, "Test Speaker") || strings.Contains(line, "AABBCCDDEEFF") {
		t.Errorf("log leaks response content: %q", line)
	}
}

func TestScanSubnets_CancelLogsAttemptedProbes(t *testing.T) {
	rec := &recordingHandler{}
	old := slog.Default()

	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(old) })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var calls atomic.Int32

	rt := roundTripFunc(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 3 {
			cancel()
		}

		return nil, fmt.Errorf("refused")
	})

	_, err := ScanSubnets(ctx, []netip.Prefix{netip.MustParsePrefix("192.0.2.0/24")},
		SubnetScanOptions{Concurrency: 1, HTTPClient: &http.Client{Transport: rt}})
	if err == nil {
		t.Fatal("want ctx error")
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	if len(rec.logs) != 1 {
		t.Fatalf("logs %v", rec.logs)
	}

	line := rec.logs[0]
	if strings.Contains(line, "hosts=254") {
		t.Errorf("log counts unprobed hosts: %q", line)
	}

	m := regexp.MustCompile(`hosts=(\d+)`).FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("no hosts in %q", line)
	}

	n, err := strconv.Atoi(m[1])
	if err != nil || n != int(calls.Load()) || !strings.Contains(line, "candidates=254") {
		t.Errorf("logged hosts=%d, requests=%d, want equal; log=%q", n, calls.Load(), line)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
