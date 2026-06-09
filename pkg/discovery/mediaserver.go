package discovery

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	mediaServerDeviceType = "urn:schemas-upnp-org:device:MediaServer:1"
	cdsServiceType        = "urn:schemas-upnp-org:service:ContentDirectory:1"
	// descFetchTimeout is a separate budget for description fetches so the
	// overall SSDP sweep timing does not cut them off.
	descFetchTimeout = 8 * time.Second
)

// MediaServer is a discovered DLNA UPnP MediaServer that exposes a
// ContentDirectory service.
type MediaServer struct {
	// UDN is the stable unique device name (uuid:...) from the UPnP description.
	UDN string
	// FriendlyName is the human-readable device name, e.g. "FRITZ!Box 7590".
	FriendlyName string
	// Manufacturer and ModelName let callers show a useful device subtitle.
	Manufacturer string
	ModelName    string
	// Address is the "host:port" of the device description endpoint.
	Address string
	// CDSControlURL is the fully resolved URL for ContentDirectory SOAP actions.
	// Empty string means the device does not expose ContentDirectory.
	CDSControlURL string
	// IconURL is the first icon the device advertised, resolved to absolute form.
	IconURL string
}

// DiscoverMediaServers sends SSDP M-SEARCH requests for MediaServer devices,
// fetches each device description concurrently, and returns only the servers
// that expose a ContentDirectory service. Deduplicated by UDN.
func DiscoverMediaServers(ctx context.Context, timeout time.Duration) ([]MediaServer, error) {
	if timeout <= 0 {
		timeout = defaultTimeout
	}

	opts := SearchOptions{
		Targets: []string{
			mediaServerDeviceType,
			"ssdp:all",
		},
		Timeout: timeout,
	}

	responses, err := SearchSSDP(ctx, opts)
	if err != nil {
		return nil, err
	}

	if len(responses) == 0 {
		return nil, nil
	}

	// Fetch descriptions concurrently. Use a fresh context so that the
	// description fetches are not cut off by the already-elapsed SSDP timeout.
	fctx, fcancel := context.WithTimeout(ctx, descFetchTimeout)
	defer fcancel()

	type fetchResult struct {
		srv MediaServer
		ok  bool
	}

	results := make(chan fetchResult, len(responses))

	var wg sync.WaitGroup

	for _, resp := range responses {
		wg.Add(1)

		go func(loc string) {
			defer wg.Done()

			desc, err := FetchDescription(fctx, loc)
			if err != nil {
				slog.Warn("mediaserver: description fetch failed", "location", loc, "err", err.Error())

				results <- fetchResult{}

				return
			}

			srv, ok := mediaServerFromDescription(desc)
			results <- fetchResult{srv: srv, ok: ok}
		}(resp.Location)
	}

	wg.Wait()
	close(results)

	seen := map[string]struct{}{}

	var out []MediaServer

	for r := range results {
		if !r.ok || r.srv.CDSControlURL == "" || r.srv.UDN == "" {
			continue
		}

		if _, dup := seen[r.srv.UDN]; dup {
			continue
		}

		seen[r.srv.UDN] = struct{}{}
		out = append(out, r.srv)
	}

	return out, nil
}

// mediaServerFromDescription maps a parsed Description to a MediaServer.
// Returns ok=false when the description does not expose a ContentDirectory
// service (i.e. the device is not a usable DLNA media server).
//
// It walks the device tree so that nested MediaServer sub-devices (e.g.
// FRITZ!Box root device nesting the NAS MediaServer) are found correctly.
func mediaServerFromDescription(desc *Description) (MediaServer, bool) {
	if desc == nil {
		return MediaServer{}, false
	}

	svc, ok := desc.FindService(cdsServiceType)
	if !ok || svc.ControlURL == "" {
		return MediaServer{}, false
	}

	srv := MediaServer{
		UDN:           desc.Root.UDN,
		FriendlyName:  desc.Root.FriendlyName,
		Manufacturer:  desc.Root.Manufacturer,
		ModelName:     desc.Root.ModelName,
		CDSControlURL: svc.ControlURL,
	}

	// Walk sub-devices to fill in UDN / FriendlyName if the root is sparse
	// (some devices put it all in the sub-device, e.g. FRITZ!Box).
	fillFromTree(desc, &srv)

	if ic, ok := desc.FirstIcon(); ok {
		srv.IconURL = ic.URL
	}

	return srv, true
}

// fillFromTree walks the description tree to fill in missing fields on srv
// from sub-devices. Only fills in fields that are still empty.
func fillFromTree(desc *Description, srv *MediaServer) {
	walkDevice(&desc.Root, srv)
}

func walkDevice(dev *Device, srv *MediaServer) {
	if srv.FriendlyName == "" && dev.FriendlyName != "" {
		srv.FriendlyName = dev.FriendlyName
	}

	if srv.UDN == "" && dev.UDN != "" {
		srv.UDN = dev.UDN
	}

	if srv.Manufacturer == "" && dev.Manufacturer != "" {
		srv.Manufacturer = dev.Manufacturer
	}

	if srv.ModelName == "" && dev.ModelName != "" {
		srv.ModelName = dev.ModelName
	}

	for i := range dev.Devices {
		walkDevice(&dev.Devices[i], srv)
	}
}
