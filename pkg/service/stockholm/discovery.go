package stockholm

import (
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ssdpAddr            = "239.255.255.250:1900"
	rendererST          = "urn:schemas-upnp-org:device:MediaRenderer:1"
	serverST            = "urn:schemas-upnp-org:device:MediaServer:1"
	ssdpProbes          = 3
	ssdpProbeIntervalMS = 350
	ssdpGraceMS         = 1250
	ssdpReceiveSliceMS  = 250
	ssdpMX              = 1
)

// RendererDevice is the payload pushed to the browser for a discovered speaker.
type RendererDevice struct {
	UID string `json:"uID"`
	IP  string `json:"ip"`
}

// ServerDevice is the payload pushed for a discovered HRMS media server.
type ServerDevice struct {
	UID  string `json:"uID"`
	IP   string `json:"ip"`
	Port string `json:"port"`
}

// infoXML is used to unmarshal /info responses from speakers.
type infoXML struct {
	DeviceID         string `xml:"deviceID,attr"`
	MargeAccountUUID string `xml:"margeAccountUUID"`
}

// DiscoverRenderers performs SSDP MediaRenderer:1 discovery, fetches /info from
// each speaker, optionally filters by expectedAccountID, and calls onDevice for
// each accepted speaker incrementally.
func DiscoverRenderers(expectedAccountID string, onDevice func(RendererDevice)) []RendererDevice {
	responses := ssdpSearch(rendererST)
	seen := make(map[string]bool)

	var results []RendererDevice

	for _, resp := range responses {
		host := hostFromSSDPResponse(resp)
		if host == "" || seen[host] {
			continue
		}

		seen[host] = true

		info, err := fetchSpeakerInfo(host)
		if err != nil {
			log.Printf("[Stockholm SSDP] Failed to fetch /info from %s: %v", sanitizeLog(host), err)
			continue
		}

		if expectedAccountID != "" && info.MargeAccountUUID != expectedAccountID {
			log.Printf("[Stockholm SSDP] Skipping %s: account %q != %q", sanitizeLog(host), sanitizeLog(info.MargeAccountUUID), sanitizeLog(expectedAccountID))
			continue
		}

		uid := strings.ToUpper(info.DeviceID)
		if uid == "" {
			continue
		}

		d := RendererDevice{UID: uid, IP: host}
		results = append(results, d)

		if onDevice != nil {
			onDevice(d)
		}
	}

	return results
}

// DiscoverServers performs SSDP MediaServer:1 discovery and returns all found servers.
func DiscoverServers() []ServerDevice {
	responses := ssdpSearch(serverST)
	seen := make(map[string]bool)

	var results []ServerDevice

	for _, resp := range responses {
		location := resp["location"]
		if location == "" {
			continue
		}

		u, err := url.Parse(location)
		if err != nil || u.Host == "" {
			continue
		}

		host := u.Hostname()

		portStr := u.Port()
		if portStr == "" {
			switch u.Scheme {
			case "https":
				portStr = "443"
			default:
				portStr = "80"
			}
		}

		key := host + ":" + portStr
		if seen[key] {
			continue
		}

		seen[key] = true

		uid := normalizeUSN(resp["usn"], key)
		results = append(results, ServerDevice{UID: uid, IP: host, Port: portStr})
	}

	return results
}

// ssdpSearch sends SSDP M-SEARCH requests and returns raw response header maps.
func ssdpSearch(searchTarget string) []map[string]string {
	ifaces := discoveryInterfaces()

	if len(ifaces) == 0 {
		return searchOnInterface(searchTarget, nil)
	}

	for _, iface := range ifaces {
		results := searchOnInterface(searchTarget, &iface)
		if len(results) > 0 {
			return results
		}
	}

	return nil
}

func searchOnInterface(searchTarget string, iface *net.Interface) []map[string]string {
	mcastAddr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil
	}

	var conn *net.UDPConn
	if iface == nil {
		conn, err = net.ListenUDP("udp4", &net.UDPAddr{})
	} else {
		bindAddr := primaryIPv4(iface)
		if bindAddr == nil {
			return nil
		}

		conn, err = net.ListenUDP("udp4", &net.UDPAddr{IP: bindAddr})
	}

	if err != nil {
		return nil
	}

	defer func() { _ = conn.Close() }()

	payload := []byte(buildMSearch(searchTarget))
	seen := make(map[string]map[string]string)

	// 3 probes
	deadline := time.Now().Add(time.Duration(ssdpProbes*ssdpProbeIntervalMS+ssdpGraceMS) * time.Millisecond)
	_ = conn.SetReadDeadline(deadline)

	for probe := 0; probe < ssdpProbes; probe++ {
		if _, err := conn.WriteToUDP(payload, mcastAddr); err != nil {
			log.Printf("[Stockholm SSDP] Send error: %v", err)
			break
		}

		collectUntil(conn, searchTarget, seen, time.Now().Add(time.Duration(ssdpProbeIntervalMS)*time.Millisecond))
	}

	collectUntil(conn, searchTarget, seen, time.Now().Add(time.Duration(ssdpGraceMS)*time.Millisecond))

	result := make([]map[string]string, 0, len(seen))
	for _, v := range seen {
		result = append(result, v)
	}

	return result
}

func collectUntil(conn *net.UDPConn, searchTarget string, seen map[string]map[string]string, until time.Time) {
	buf := make([]byte, 8192)

	for {
		remaining := time.Until(until)
		if remaining <= 0 {
			return
		}

		slice := time.Duration(ssdpReceiveSliceMS) * time.Millisecond
		if slice > remaining {
			slice = remaining
		}

		_ = conn.SetReadDeadline(time.Now().Add(slice))

		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return
			}

			return
		}

		headers := parseSSDPHeaders(buf[:n], remoteAddr.IP.String())

		if !matchesST(headers, searchTarget) {
			continue
		}

		key := responseKey(headers)
		if _, exists := seen[key]; !exists {
			seen[key] = headers
		}
	}
}

func parseSSDPHeaders(data []byte, remoteIP string) map[string]string {
	out := map[string]string{"remote-ip": remoteIP}

	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")

		idx := strings.IndexByte(line, ':')
		if idx <= 0 {
			continue
		}

		k := strings.ToLower(strings.TrimSpace(line[:idx]))
		v := strings.TrimSpace(line[idx+1:])

		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}

	return out
}

func matchesST(resp map[string]string, target string) bool {
	if st := resp["st"]; strings.EqualFold(st, target) {
		return true
	}

	if usn := resp["usn"]; strings.Contains(strings.ToLower(usn), strings.ToLower(target)) {
		return true
	}

	return false
}

func responseKey(resp map[string]string) string {
	return resp["usn"] + "|" + resp["location"] + "|" + resp["remote-ip"]
}

func hostFromSSDPResponse(resp map[string]string) string {
	if loc := resp["location"]; loc != "" {
		if u, err := url.Parse(loc); err == nil && u.Host != "" {
			return u.Hostname()
		}
	}

	return resp["remote-ip"]
}

func fetchSpeakerInfo(host string) (*infoXML, error) {
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(fmt.Sprintf("http://%s:8090/info", host))
	if err != nil {
		return nil, err
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var info infoXML
	if err := xml.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	return &info, nil
}

func normalizeUSN(usn, fallback string) string {
	if usn == "" {
		return fallback
	}

	// Strip "::urn:..." suffix
	if i := strings.Index(usn, "::"); i >= 0 {
		usn = usn[:i]
	}

	// Strip "uuid:" prefix
	if strings.HasPrefix(strings.ToLower(usn), "uuid:") {
		usn = usn[5:]
	}

	if usn == "" {
		return fallback
	}

	return usn
}

func buildMSearch(st string) string {
	return strings.Join([]string{
		"M-SEARCH * HTTP/1.1",
		"Host:239.255.255.250:1900",
		`Man:"ssdp:discover"`,
		fmt.Sprintf("MX:%d", ssdpMX),
		"ST:" + st,
		"",
		"",
	}, "\r\n")
}

// discoveryInterfaces returns suitable network interfaces sorted by priority:
// ethernet/en* first, then wifi/wl*, then others. Loopback/virtual/docker etc. are excluded.
func discoveryInterfaces() []net.Interface {
	all, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var ifaces []net.Interface

	for _, iface := range all {
		if !isDiscoveryInterface(iface) {
			continue
		}

		ifaces = append(ifaces, iface)
	}

	// Sort: ethernet first (priority 0), wifi (1), others (2)
	for i := 0; i < len(ifaces); i++ {
		for j := i + 1; j < len(ifaces); j++ {
			if interfacePriority(ifaces[i]) > interfacePriority(ifaces[j]) {
				ifaces[i], ifaces[j] = ifaces[j], ifaces[i]
			}
		}
	}

	return ifaces
}

func isDiscoveryInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return false
	}

	if iface.Flags&net.FlagLoopback != 0 {
		return false
	}

	if iface.Flags&net.FlagMulticast == 0 {
		return false
	}

	desc := strings.ToLower(iface.Name + " " + iface.Name)
	for _, banned := range []string{"docker", "vbox", "vmware", "hyper-v", "loopback", "bluetooth", "teredo", "tunnel"} {
		if strings.Contains(desc, banned) {
			return false
		}
	}

	return primaryIPv4(&iface) != nil
}

func interfacePriority(iface net.Interface) int {
	name := strings.ToLower(iface.Name)
	if strings.HasPrefix(name, "eth") || strings.HasPrefix(name, "en") {
		return 0
	}

	if strings.HasPrefix(name, "wl") || strings.Contains(name, "wifi") || strings.Contains(name, "wlan") {
		return 1
	}

	return 2
}

func primaryIPv4(iface *net.Interface) net.IP {
	addrs, err := iface.Addrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		var ip net.IP

		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
			return ip4
		}
	}

	return nil
}
