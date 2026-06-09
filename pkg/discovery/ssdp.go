// Package discovery provides device discovery functionality for Bose SoundTouch
// devices using mDNS and UPnP protocols.
package discovery

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// SearchOptions configures a generic SSDP M-SEARCH sweep.
// Targets lists the ST values to search for (e.g.
// "urn:schemas-upnp-org:device:MediaServer:1" and "ssdp:all").
// Timeout is how long to listen for responses.
// Interface, when non-empty, pins multicast to that NIC by name;
// when empty, all non-loopback IPv4 interfaces are used.
type SearchOptions struct {
	Targets   []string
	Timeout   time.Duration
	Interface string
}

// SSDPResponse holds the raw fields from one SSDP HTTP/1.1 200 OK response.
// Responses are deduped by Location before being returned by SearchSSDP.
type SSDPResponse struct {
	Location string
	USN      string
	ST       string
	Server   string
}

// Description is the parsed content of a UPnP device description XML document.
type Description struct {
	// URLBase is the base URL declared in the document (may be empty).
	URLBase string
	Root    Device
}

// Device represents one UPnP device node (root or sub-device).
type Device struct {
	DeviceType   string
	FriendlyName string
	Manufacturer string
	ModelName    string
	SerialNumber string
	UDN          string
	Icons        []Icon
	Services     []UPnPService
	Devices      []Device // embedded sub-devices (e.g. FRITZ!Box nests MediaServer)
}

// UPnPService is a single UPnP service advertisement inside a Device.
type UPnPService struct {
	ServiceType string
	ControlURL  string
	EventSubURL string
	SCPDURL     string
}

// Icon is one entry from a UPnP iconList.
type Icon struct {
	MimeType string
	Width    int
	Height   int
	URL      string
}

// FindService walks the description tree (root device and all sub-devices) and
// returns the first UPnPService whose ServiceType equals serviceType.
func (d *Description) FindService(serviceType string) (UPnPService, bool) {
	return findServiceInDevice(&d.Root, serviceType)
}

func findServiceInDevice(dev *Device, serviceType string) (UPnPService, bool) {
	for _, svc := range dev.Services {
		if svc.ServiceType == serviceType {
			return svc, true
		}
	}

	for i := range dev.Devices {
		if svc, ok := findServiceInDevice(&dev.Devices[i], serviceType); ok {
			return svc, true
		}
	}

	return UPnPService{}, false
}

// FirstIcon walks the device tree depth-first and returns the first icon it
// finds (which is the icon advertised in the root device, or its first
// sub-device if the root has none).
func (d *Description) FirstIcon() (Icon, bool) {
	return firstIconInDevice(&d.Root)
}

func firstIconInDevice(dev *Device) (Icon, bool) {
	if len(dev.Icons) > 0 {
		return dev.Icons[0], true
	}

	for i := range dev.Devices {
		if ic, ok := firstIconInDevice(&dev.Devices[i]); ok {
			return ic, true
		}
	}

	return Icon{}, false
}

// ssdpDefaultMXSecs is the M-SEARCH MX header value (seconds the device may
// wait before answering). Keep it generous so slower NAS boxes are not missed.
const ssdpDefaultMXSecs = 3

// SearchSSDP sends SSDP M-SEARCH requests for each target in opts.Targets,
// collects responses until opts.Timeout expires, and returns the unique
// responses deduped by LOCATION. When opts.Interface is empty, the search is
// sent from every non-loopback IPv4 interface; when set, only that interface
// is used.
func SearchSSDP(ctx context.Context, opts SearchOptions) ([]SSDPResponse, error) {
	if opts.Timeout <= 0 {
		opts.Timeout = defaultTimeout
	}

	sctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	mcAddr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, fmt.Errorf("ssdp: resolve multicast addr: %w", err)
	}

	// Build M-SEARCH packets for each target.
	var msgs [][]byte

	for _, st := range opts.Targets {
		msgs = append(msgs, buildMSearchPacket(st))
	}

	// Determine which source IPs to send from.
	var srcIPs []net.IP

	if opts.Interface != "" {
		ip, err := interfaceIPv4(opts.Interface)
		if err != nil {
			return nil, err
		}

		srcIPs = []net.IP{ip}
	} else {
		srcIPs = candidateIPv4Addrs()

		if len(srcIPs) == 0 {
			slog.Warn("ssdp: no usable IPv4 interfaces, falling back to wildcard")

			srcIPs = []net.IP{net.IPv4zero}
		}
	}

	slog.Info("ssdp: M-SEARCH starting",
		"targets", opts.Targets,
		"interfaces", len(srcIPs),
		"timeout", opts.Timeout.String(),
	)

	// Collect unique locations across all goroutines.
	mu := sync.Mutex{}
	byLocation := map[string]SSDPResponse{}

	var wg sync.WaitGroup

	for _, srcIP := range srcIPs {
		wg.Add(1)

		go func(ip net.IP) {
			defer wg.Done()

			ssdpSendRecv(sctx, ip, mcAddr, msgs, func(resp SSDPResponse) {
				mu.Lock()
				defer mu.Unlock()

				if _, exists := byLocation[resp.Location]; !exists {
					byLocation[resp.Location] = resp
					slog.Info("ssdp: new location", "location", resp.Location, "st", resp.ST)
				}
			})
		}(srcIP)
	}

	wg.Wait()

	out := make([]SSDPResponse, 0, len(byLocation))

	for _, r := range byLocation {
		out = append(out, r)
	}

	slog.Info("ssdp: M-SEARCH done", "locations", len(out))

	return out, nil
}

// buildMSearchPacket returns an SSDP M-SEARCH request for the given ST value.
func buildMSearchPacket(st string) []byte {
	return []byte(strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"HOST: " + ssdpAddr,
		"MAN: \"ssdp:discover\"",
		fmt.Sprintf("MX: %d", ssdpDefaultMXSecs),
		"ST: " + st,
		"USER-AGENT: AfterTouch/1 UPnP/1.0",
		"", "",
	}, "\r\n"))
}

// ssdpSendRecv opens a UDP socket bound to srcIP, sends all msgs to mcAddr,
// reads responses until sctx is done or a UDP timeout, and calls notify for
// each response that carries a non-empty LOCATION header.
func ssdpSendRecv(sctx context.Context, srcIP net.IP, mcAddr *net.UDPAddr, msgs [][]byte, notify func(SSDPResponse)) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: srcIP, Port: 0})
	if err != nil {
		slog.Warn("ssdp: ListenUDP failed", "src", srcIP.String(), "err", err.Error())
		return
	}
	defer func() { _ = conn.Close() }()

	// Send all messages in 2 rounds with an 80 ms gap between rounds.
	// The spacing lets slower NAS/router boxes that drop back-to-back bursts
	// still answer, rather than sending the whole batch as one burst.
	for range 2 {
		for _, msg := range msgs {
			if _, err := conn.WriteToUDP(msg, mcAddr); err != nil {
				slog.Warn("ssdp: WriteToUDP failed", "src", srcIP.String(), "err", err.Error())
			}
		}

		time.Sleep(80 * time.Millisecond)
	}

	deadline, ok := sctx.Deadline()
	if ok {
		_ = conn.SetReadDeadline(deadline)
	}

	buf := make([]byte, 4096)

	for {
		select {
		case <-sctx.Done():
			return
		default:
		}

		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Timeout or context done.
			return
		}

		loc := ssdpHeaderValue(buf[:n], "LOCATION")
		if loc == "" {
			continue
		}

		notify(SSDPResponse{
			Location: loc,
			USN:      ssdpHeaderValue(buf[:n], "USN"),
			ST:       ssdpHeaderValue(buf[:n], "ST"),
			Server:   ssdpHeaderValue(buf[:n], "SERVER"),
		})
	}
}

// ssdpHeaderValue finds the value of header in a raw SSDP UDP packet.
// Header matching is case-insensitive.
func ssdpHeaderValue(packet []byte, header string) string {
	lines := bytes.Split(packet, []byte("\r\n"))
	prefix := strings.ToLower(header) + ":"

	for _, line := range lines {
		if len(line) <= len(prefix) {
			continue
		}

		if strings.EqualFold(string(line[:len(prefix)]), prefix) {
			return strings.TrimSpace(string(line[len(prefix):]))
		}
	}

	return ""
}

// candidateIPv4Addrs returns the routable IPv4 source addresses to send SSDP
// M-SEARCH from. Excludes loopback, link-local, and interfaces that are down.
// Rationale: a host with two Wi-Fi adapters on different networks needs to
// probe both.
func candidateIPv4Addrs() []net.IP {
	var out []net.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}

			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}

			if ip4.IsLoopback() || ip4.IsLinkLocalUnicast() || ip4.IsLinkLocalMulticast() {
				continue
			}

			out = append(out, ip4)
		}
	}

	return out
}

// interfaceIPv4 returns the first non-loopback IPv4 address of the named
// interface, or an error if not found.
func interfaceIPv4(ifaceName string) (net.IP, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, fmt.Errorf("ssdp: interface %q not found: %w", ifaceName, err)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("ssdp: read addrs for %q: %w", ifaceName, err)
	}

	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}

		ip4 := ipnet.IP.To4()
		if ip4 == nil || ip4.IsLoopback() {
			continue
		}

		return ip4, nil
	}

	return nil, fmt.Errorf("ssdp: interface %q has no usable IPv4 address", ifaceName)
}

// FetchDescription fetches the UPnP device description at location and parses
// it into a Description tree. Relative URLs in the tree (controlURL, icon URL)
// are resolved to absolute form using URLBase or location as the base.
func FetchDescription(ctx context.Context, location string) (*Description, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, location, nil)
	if err != nil {
		return nil, fmt.Errorf("ssdp: build request for %s: %w", location, err)
	}

	client := &http.Client{Timeout: 8 * time.Second}

	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("ssdp: fetch description failed", "location", location, "err", err.Error())
		return nil, fmt.Errorf("ssdp: fetch %s: %w", location, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("ssdp: read body from %s: %w", location, err)
	}

	return parseDescription(body, location)
}

// parseDescription parses raw UPnP device description XML bytes and resolves
// relative URLs against the given location. It is a pure function (no I/O)
// so it can be unit-tested on canned bytes.
func parseDescription(body []byte, location string) (*Description, error) {
	// Raw XML types that mirror the UPnP device description schema.
	type xmlIcon struct {
		MimeType string `xml:"mimetype"`
		Width    int    `xml:"width"`
		Height   int    `xml:"height"`
		URL      string `xml:"url"`
	}

	type xmlService struct {
		ServiceType string `xml:"serviceType"`
		ControlURL  string `xml:"controlURL"`
		EventSubURL string `xml:"eventSubURL"`
		SCPDURL     string `xml:"SCPDURL"`
	}

	// xmlDevice is defined as a named type so it can reference itself.
	type xmlDevice struct {
		DeviceType   string       `xml:"deviceType"`
		FriendlyName string       `xml:"friendlyName"`
		Manufacturer string       `xml:"manufacturer"`
		ModelName    string       `xml:"modelName"`
		SerialNumber string       `xml:"serialNumber"`
		UDN          string       `xml:"UDN"`
		Icons        []xmlIcon    `xml:"iconList>icon"`
		Services     []xmlService `xml:"serviceList>service"`
		SubDevices   []xmlDevice  `xml:"deviceList>device"`
	}

	type xmlRoot struct {
		XMLName xml.Name  `xml:"root"`
		URLBase string    `xml:"URLBase"`
		Device  xmlDevice `xml:"device"`
	}

	var root xmlRoot

	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("ssdp: parse description XML: %w", err)
	}

	// Determine base URL for resolving relative references.
	baseURL, _ := url.Parse(location)

	if root.URLBase != "" {
		if u, err := url.Parse(root.URLBase); err == nil {
			baseURL = u
		}
	}

	// Recursive mapper from xmlDevice to Device.
	var mapDevice func(xd xmlDevice) Device

	mapDevice = func(xd xmlDevice) Device {
		d := Device{
			DeviceType:   xd.DeviceType,
			FriendlyName: xd.FriendlyName,
			Manufacturer: xd.Manufacturer,
			ModelName:    xd.ModelName,
			SerialNumber: xd.SerialNumber,
			UDN:          xd.UDN,
		}

		for _, xi := range xd.Icons {
			d.Icons = append(d.Icons, Icon{
				MimeType: xi.MimeType,
				Width:    xi.Width,
				Height:   xi.Height,
				URL:      absURL(baseURL, xi.URL),
			})
		}

		for _, xs := range xd.Services {
			d.Services = append(d.Services, UPnPService{
				ServiceType: xs.ServiceType,
				ControlURL:  absURL(baseURL, xs.ControlURL),
				EventSubURL: absURL(baseURL, xs.EventSubURL),
				SCPDURL:     absURL(baseURL, xs.SCPDURL),
			})
		}

		for i := range xd.SubDevices {
			d.Devices = append(d.Devices, mapDevice(xd.SubDevices[i]))
		}

		return d
	}

	desc := &Description{
		URLBase: root.URLBase,
		Root:    mapDevice(root.Device),
	}

	return desc, nil
}

// absURL resolves ref relative to base. If ref is already absolute, or if
// parsing fails, ref is returned unchanged.
func absURL(base *url.URL, ref string) string {
	if ref == "" || base == nil {
		return ref
	}

	u, err := url.Parse(ref)
	if err != nil {
		return ref
	}

	return base.ResolveReference(u).String()
}
