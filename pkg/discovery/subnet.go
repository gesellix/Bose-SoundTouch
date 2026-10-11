package discovery

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// SubnetScanMethod is the DiscoveryMethod reported for devices found by ScanSubnets.
const SubnetScanMethod = "subnet-scan"

const (
	maxInfoBodyBytes = 1 << 20
	maxConcurrency   = 64
	maxScanHosts     = 1024
	maxPort          = 65535
)

// SubnetScanOptions bounds a ScanSubnets run. Zero values select defaults.
type SubnetScanOptions struct {
	Port        int           // SoundTouch API port (default 8090; out of range selects the default)
	DialTimeout time.Duration // TCP connect timeout (default 400ms)
	InfoTimeout time.Duration // whole GET /info deadline (default 2s)
	Concurrency int           // parallel probes (default and hard cap 64)
	MaxHosts    int           // cap on unique hosts probed (default and hard cap 1024)
	HTTPClient  *http.Client  // injectable; redirects are always refused
}

func (o SubnetScanOptions) withDefaults() SubnetScanOptions {
	if o.Port <= 0 || o.Port > maxPort {
		o.Port = 8090
	}

	if o.DialTimeout <= 0 {
		o.DialTimeout = 400 * time.Millisecond
	}

	if o.InfoTimeout <= 0 {
		o.InfoTimeout = 2 * time.Second
	}

	if o.Concurrency <= 0 || o.Concurrency > maxConcurrency {
		o.Concurrency = maxConcurrency
	}

	if o.MaxHosts <= 0 || o.MaxHosts > maxScanHosts {
		o.MaxHosts = maxScanHosts
	}

	return o
}

// scanHosts expands IPv4 prefixes into unique usable host addresses (network
// and broadcast excluded for prefixes shorter than /31), stopping at max.
// truncated reports that the cap cut the list short.
func scanHosts(prefixes []netip.Prefix, maxHosts int) (hosts []netip.Addr, truncated bool) {
	seen := make(map[netip.Addr]struct{})

	for _, p := range prefixes {
		if !p.IsValid() || !p.Addr().Is4() {
			continue
		}

		p = p.Masked()
		last := lastAddr(p)

		for a := p.Addr(); a.IsValid() && a.Compare(last) <= 0; a = a.Next() {
			if p.Bits() < 31 && (a == p.Addr() || a == last) {
				continue
			}

			if _, dup := seen[a]; dup {
				continue
			}

			if len(hosts) >= maxHosts {
				return hosts, true
			}

			seen[a] = struct{}{}

			hosts = append(hosts, a)
		}
	}

	return hosts, false
}

func lastAddr(p netip.Prefix) netip.Addr {
	b := p.Addr().As4()
	host := 32 - p.Bits()

	for i := 0; i < host; i++ {
		b[3-i/8] |= 1 << (i % 8)
	}

	return netip.AddrFrom4(b)
}

// ScanSubnets probes every usable IPv4 host in prefixes with GET /info and
// returns the ones answering like a SoundTouch speaker. It does not rely on
// multicast, so it works from bridge-networked containers.
func ScanSubnets(ctx context.Context, prefixes []netip.Prefix, opts SubnetScanOptions) ([]*models.DiscoveredDevice, error) {
	opts = opts.withDefaults()
	started := time.Now()

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	hosts, truncated := scanHosts(prefixes, opts.MaxHosts)
	client := scanClient(opts)

	var (
		mu     sync.Mutex
		found  []*models.DiscoveredDevice
		probed atomic.Int64
		wg     sync.WaitGroup
	)

	jobs := make(chan netip.Addr)

	for i := 0; i < opts.Concurrency && i < len(hosts); i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for h := range jobs {
				if ctx.Err() != nil {
					continue
				}

				probed.Add(1)

				if d := probeInfo(ctx, client, h, opts); d != nil {
					mu.Lock()

					found = append(found, d)

					mu.Unlock()
				}
			}
		}()
	}

feed:
	for _, h := range hosts {
		select {
		case jobs <- h:
		case <-ctx.Done():
			break feed
		}
	}

	close(jobs)
	wg.Wait()

	sort.Slice(found, func(i, j int) bool { return found[i].Host < found[j].Host })

	slog.Info("subnet scan finished", "duration", time.Since(started).Round(time.Millisecond),
		"prefixes", prefixes, "hosts", probed.Load(), "candidates", len(hosts), "truncated", truncated, "found", len(found))

	return found, ctx.Err()
}

func scanClient(opts SubnetScanOptions) *http.Client {
	var c http.Client

	if opts.HTTPClient != nil {
		c = *opts.HTTPClient
	} else {
		c.Transport = &http.Transport{
			Proxy:             nil,
			DialContext:       (&net.Dialer{Timeout: opts.DialTimeout}).DialContext,
			DisableKeepAlives: true,
		}
	}

	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	c.Timeout = opts.InfoTimeout

	return &c
}

func probeInfo(ctx context.Context, client *http.Client, host netip.Addr, opts SubnetScanOptions) *models.DiscoveredDevice {
	reqCtx, cancel := context.WithTimeout(ctx, opts.InfoTimeout)
	defer cancel()

	base := fmt.Sprintf("http://%s/", net.JoinHostPort(host.String(), strconv.Itoa(opts.Port)))

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, base+"info", nil)
	if err != nil {
		return nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxInfoBodyBytes+1))
	if err != nil || len(body) > maxInfoBodyBytes {
		return nil
	}

	info, err := parseInfo(body)
	if err != nil {
		return nil
	}

	return &models.DiscoveredDevice{
		Name:            strings.TrimSpace(info.Name),
		Host:            host.String(),
		Port:            opts.Port,
		ModelID:         strings.TrimSpace(info.Type),
		SerialNo:        info.DeviceID,
		LastSeen:        time.Now(),
		DiscoveryMethod: SubnetScanMethod,
		APIBaseURL:      base,
		InfoURL:         base + "info",
	}
}

// parseInfo accepts only a single well-formed <info deviceID="..."> document.
// Before the root only whitespace, comments and one XML declaration at the very
// start are allowed; after it only whitespace and comments.
func parseInfo(body []byte) (*models.DeviceInfo, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))

	var (
		info   models.DeviceInfo
		offset int64
	)

	for first := true; ; first = false {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}

		next := dec.InputOffset()

		if start, ok := tok.(xml.StartElement); ok {
			if err := dec.DecodeElement(&info, &start); err != nil {
				return nil, err
			}

			offset = dec.InputOffset()

			break
		}

		if err := checkOutsideRoot(tok, body[offset:next], first); err != nil {
			return nil, err
		}

		offset = next
	}

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}

		next := dec.InputOffset()

		if err := checkOutsideRoot(tok, body[offset:next], false); err != nil {
			return nil, err
		}

		offset = next
	}

	info.DeviceID = strings.TrimSpace(info.DeviceID)
	if info.DeviceID == "" {
		return nil, errors.New("missing deviceID")
	}

	return &info, nil
}

// ParseSubnets parses a comma-separated list of IPv4 CIDRs (the
// DISCOVERY_SUBNETS format). Entries that are not IPv4 CIDRs are returned in
// invalid so the caller can warn; blanks are skipped silently.
func ParseSubnets(s string) (prefixes []netip.Prefix, invalid []string) {
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		p, err := netip.ParsePrefix(entry)
		if err != nil || !p.Addr().Is4() {
			invalid = append(invalid, entry)
			continue
		}

		prefixes = append(prefixes, p.Masked())
	}

	return dedupePrefixes(prefixes), invalid
}

// CandidatePrefixes picks what to scan. Explicit IPv4 prefixes win outright,
// even public or loopback ones. Without them, only RFC1918 hint addresses
// qualify, each widened to its /24. Order is preserved and duplicates dropped.
func CandidatePrefixes(explicit []netip.Prefix, hints []netip.Addr) []netip.Prefix {
	var out []netip.Prefix

	for _, p := range explicit {
		if p.IsValid() && p.Addr().Is4() {
			out = append(out, p.Masked())
		}
	}

	if len(out) > 0 {
		return dedupePrefixes(out)
	}

	for _, h := range hints {
		h = h.Unmap()
		if !h.IsValid() || !h.Is4() || !h.IsPrivate() {
			continue
		}

		out = append(out, netip.PrefixFrom(h, 24).Masked())
	}

	return dedupePrefixes(out)
}

func dedupePrefixes(in []netip.Prefix) []netip.Prefix {
	seen := make(map[netip.Prefix]struct{}, len(in))

	var out []netip.Prefix

	for _, p := range in {
		if _, ok := seen[p]; ok {
			continue
		}

		seen[p] = struct{}{}

		out = append(out, p)
	}

	return out
}

// checkOutsideRoot rejects anything but whitespace, comments and, as the very
// first token, the XML declaration.
func checkOutsideRoot(tok xml.Token, raw []byte, first bool) error {
	switch t := tok.(type) {
	case xml.Comment:
		return nil
	case xml.ProcInst:
		if first && t.Target == "xml" {
			return nil
		}

		return errors.New("unexpected processing instruction")
	case xml.CharData:
		_ = t // Decoder presents CDATA as CharData; validate the raw bytes instead.

		for _, c := range raw {
			if c != ' ' && c != '	' && c != '\r' && c != '\n' {
				return errors.New("unexpected text outside root element")
			}
		}

		return nil
	default:
		return errors.New("unexpected content outside root element")
	}
}
