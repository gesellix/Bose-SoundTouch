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
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// SubnetScanMethod is the DiscoveryMethod reported for devices found by ScanSubnets.
const SubnetScanMethod = "subnet-scan"

const maxInfoBodyBytes = 1 << 20

// SubnetScanOptions bounds a ScanSubnets run. Zero values select defaults.
type SubnetScanOptions struct {
	Port        int           // SoundTouch API port (default 8090)
	DialTimeout time.Duration // TCP connect timeout (default 400ms)
	InfoTimeout time.Duration // whole GET /info deadline (default 2s)
	Concurrency int           // parallel probes (default 64)
	MaxHosts    int           // cap on unique hosts probed (default 1024)
	HTTPClient  *http.Client  // injectable; redirects are always refused
}

func (o SubnetScanOptions) withDefaults() SubnetScanOptions {
	if o.Port <= 0 {
		o.Port = 8090
	}

	if o.DialTimeout <= 0 {
		o.DialTimeout = 400 * time.Millisecond
	}

	if o.InfoTimeout <= 0 {
		o.InfoTimeout = 2 * time.Second
	}

	if o.Concurrency <= 0 {
		o.Concurrency = 64
	}

	if o.MaxHosts <= 0 {
		o.MaxHosts = 1024
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

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	hosts, truncated := scanHosts(prefixes, opts.MaxHosts)
	client := scanClient(opts)

	var (
		mu    sync.Mutex
		found []*models.DiscoveredDevice
		wg    sync.WaitGroup
	)

	jobs := make(chan netip.Addr)

	for i := 0; i < opts.Concurrency && i < len(hosts); i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for h := range jobs {
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

	slog.Info("subnet scan finished", "prefixes", len(prefixes), "hosts", len(hosts),
		"truncated", truncated, "found", len(found))

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
	defer resp.Body.Close()

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
func parseInfo(body []byte) (*models.DeviceInfo, error) {
	dec := xml.NewDecoder(bytes.NewReader(body))

	var info models.DeviceInfo
	if err := dec.Decode(&info); err != nil {
		return nil, err
	}

	for {
		_, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return nil, err
		}
	}

	info.DeviceID = strings.TrimSpace(info.DeviceID)
	if info.DeviceID == "" {
		return nil, errors.New("missing deviceID")
	}

	return &info, nil
}
