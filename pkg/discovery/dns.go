// Package discovery provides DNS-based discovery and interception for Bose SoundTouch devices.
package discovery

import (
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSDiscovery handles DNS queries and records discovered hosts.
type DNSDiscovery struct {
	// Configuration
	upstreamDNS []string
	serviceIP   string

	// derivedHosts is the auto-derived list of additional hostnames the
	// interceptor should hijack alongside the Bose cloud list. Populated
	// from the operator's configured serverURL at construction time —
	// today this means `<first-label>oauth.<rest>`, the hostname the
	// speaker firmware constructs for the Spotify / Amazon Music OAuth
	// flow. Empty when serverURL is IP-based, missing, or has no domain
	// part to derive from.
	derivedHosts []string

	// State
	discovered       map[string]*DiscoveredHost
	interceptClients map[string]time.Time
	mu               sync.RWMutex

	// Callbacks
	onNewDiscovery func(hostname string)

	// Servers for Shutdown
	udpServer *dns.Server
	tcpServer *dns.Server

	// Address for loop prevention
	bindAddr string

	// Forward timeout
	timeout time.Duration

	// Log throttling
	lastLog   map[string]time.Time
	lastLogMu sync.Mutex
}

// DiscoveredHost represents a host discovered via DNS queries.
type DiscoveredHost struct {
	Hostname      string    `json:"hostname"`
	FirstSeen     time.Time `json:"first_seen"`
	LastSeen      time.Time `json:"last_seen"`
	QueryCount    int       `json:"query_count"`
	IsBoseService bool      `json:"is_bose_service"`
	IsIntercepted bool      `json:"is_intercepted"`
	RemoteAddr    string    `json:"remote_addr,omitempty"`
}

// NewDNSDiscovery creates a new DNSDiscovery instance. serverURL is the
// operator's configured streaming endpoint; its hostname is used to
// derive the OAuth-subdomain alias the speaker constructs (see
// DeriveOAuthHostnames). Pass an empty string when no serverURL is
// available (the derivation is a no-op in that case).
func NewDNSDiscovery(upstreamDNS []string, serviceIP, serverURL string) *DNSDiscovery {
	derived := DeriveOAuthHostnames(serverURL)
	if len(derived) > 0 {
		log.Printf("[DNS] Auto-hijacking OAuth subdomains derived from serverURL %q: %s", sanitizeLog(serverURL), sanitizeLog(strings.Join(derived, ", ")))
	}

	return &DNSDiscovery{
		upstreamDNS:      upstreamDNS,
		serviceIP:        serviceIP,
		derivedHosts:     derived,
		discovered:       make(map[string]*DiscoveredHost),
		interceptClients: make(map[string]time.Time),
		timeout:          2 * time.Second,
		lastLog:          make(map[string]time.Time),
	}
}

// DeriveOAuthHostnames returns the list of additional hostnames the DNS
// interceptor should hijack to support Spotify / Amazon Music OAuth on a
// non-Bose target. SoundTouch firmware constructs the OAuth endpoint by
// appending `oauth` to the first label of the configured streaming
// hostname (e.g. `aftertouch.lan` → `aftertouchoauth.lan`). When the
// target is an IP address the derivation produces a malformed hostname
// no resolver will answer for, so we deliberately return an empty
// slice — the caller's behaviour stays unchanged, but the operator
// (and the health-tab check) can detect the misconfiguration via the
// missing entry.
//
// Returned hostnames are lower-cased. An empty serverURL, a URL that
// fails to parse, or a hostname without a domain part (single-label
// "aftertouch") all yield an empty slice.
func DeriveOAuthHostnames(serverURL string) []string {
	if serverURL == "" {
		return nil
	}

	u, err := url.Parse(serverURL)
	if err != nil {
		return nil
	}

	host := strings.ToLower(u.Hostname())
	if host == "" {
		return nil
	}

	if net.ParseIP(host) != nil {
		// IP-based deployment — the speaker's `<first-label>oauth.<rest>`
		// construction is meaningless (e.g. `192oauth.168.0.30`) and no
		// DNS server can resolve it. Operators in this situation need to
		// switch to a real LAN hostname; see docs/concepts/amazon-music-oauth.md.
		return nil
	}

	idx := strings.IndexByte(host, '.')
	if idx <= 0 {
		// Single-label hostname (e.g. "aftertouch") — no domain part to
		// append after the inserted "oauth". The speaker firmware does
		// the same: it appends "oauth" inside the first label, so a
		// single-label name would produce "aftertouchoauth", which most
		// DNS resolvers won't answer for either.
		return nil
	}

	return []string{host[:idx] + "oauth" + host[idx:]}
}

// ServeDNS implements the dns.Handler interface.
func (d *DNSDiscovery) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		return
	}

	q := r.Question[0]
	hostname := strings.TrimSuffix(q.Name, ".")

	remoteAddr := ""
	if w.RemoteAddr() != nil {
		remoteAddr = w.RemoteAddr().String()
	}

	// Decide how to respond
	isIntercepted := d.shouldIntercept(hostname) || hostname == "aftertouch.test"

	// Record discovery
	d.recordQuery(hostname, isIntercepted, remoteAddr)

	if isIntercepted {
		// Return your service IP
		d.respondWithIP(w, r, d.serviceIP)
		d.throttledLog(fmt.Sprintf("[DNS] Intercepting %s (type %d) -> %s", hostname, q.Qtype, d.serviceIP))
	} else {
		// Forward to real DNS
		if len(d.upstreamDNS) == 0 {
			d.throttledLog("[DNS ERROR] No upstream DNS configured, cannot forward")

			m := new(dns.Msg)
			m.SetReply(r)
			m.Rcode = dns.RcodeServerFailure
			_ = w.WriteMsg(m)

			return
		}

		d.throttledLog(fmt.Sprintf("[DNS] Forwarding %s (type %d) to %v", hostname, q.Qtype, d.upstreamDNS))
		d.forward(w, r)
	}
}

func (d *DNSDiscovery) throttledLog(msg string) {
	d.lastLogMu.Lock()
	defer d.lastLogMu.Unlock()

	now := time.Now()
	if last, ok := d.lastLog[msg]; ok && now.Sub(last) < 10*time.Second {
		return
	}

	d.lastLog[msg] = now
	log.Print(msg)
}

// recordQuery logs a DNS query and updates the internal state.
func (d *DNSDiscovery) recordQuery(hostname string, isIntercepted bool, remoteAddr string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	host, exists := d.discovered[hostname]
	if !exists {
		// New discovery!
		host = &DiscoveredHost{
			Hostname:      hostname,
			FirstSeen:     time.Now(),
			LastSeen:      time.Now(),
			QueryCount:    1,
			IsBoseService: d.isBoseRelated(hostname),
			IsIntercepted: isIntercepted,
			RemoteAddr:    remoteAddr,
		}
		d.discovered[hostname] = host

		log.Printf("[NEW DISCOVERY] %s (Bose: %v, Intercepted: %v)",
			sanitizeLog(hostname), host.IsBoseService, host.IsIntercepted)

		if d.onNewDiscovery != nil {
			go d.onNewDiscovery(hostname)
		}
	} else {
		host.LastSeen = time.Now()
		host.QueryCount++

		host.IsIntercepted = isIntercepted
		if remoteAddr != "" {
			host.RemoteAddr = remoteAddr
		}
	}

	// Track distinct non-loopback clients that queried an intercepted hostname.
	// Used by the dns_speaker_usage health check to detect whether any speaker
	// actually resolves Bose hostnames through AfterTouch's DNS interceptor.
	if isIntercepted && remoteAddr != "" {
		clientHost, _, err := net.SplitHostPort(remoteAddr)
		if err != nil {
			// remoteAddr doesn't parse as host:port; use as-is.
			clientHost = remoteAddr
		}

		if ip := net.ParseIP(clientHost); ip != nil && !ip.IsLoopback() {
			d.interceptClients[clientHost] = time.Now()
		}
	}
}

// InterceptedBoseHosts is the canonical list of Bose cloud service
// hostnames the DNS server hijacks. Exposed so other packages
// (e.g. the Health tab's DNS sanity check) can iterate the list
// without duplicating it.
var InterceptedBoseHosts = []string{
	"api.bose.com",
	"marge.bose.com",
	"bmx.bose.com",
	"streaming.bose.com",
	"streamingoauth.bose.com",
	"updates.bose.com",
	"stats.bose.com",
	"content.api.bose.io",
	"events.api.bosecm.com",
	"bose-prod.apigee.net",
	"bose-test.apigee.net",
	"worldwide.bose.com",
	"music.api.bose.com",
	"bosecm.com",
	"bose.io",
	"downloads.bose.com",
}

func (d *DNSDiscovery) shouldIntercept(hostname string) bool {
	for _, service := range InterceptedBoseHosts {
		if strings.Contains(hostname, service) {
			return true
		}
	}

	lower := strings.ToLower(hostname)
	for _, h := range d.derivedHosts {
		if lower == h {
			return true
		}
	}

	return false
}

func (d *DNSDiscovery) isBoseRelated(hostname string) bool {
	return strings.Contains(hostname, "bose") ||
		strings.Contains(hostname, "soundtouch")
}

func (d *DNSDiscovery) respondWithIP(w dns.ResponseWriter, r *dns.Msg, ip string) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false // Embedded clients sometimes don't like compression
	m.Authoritative = true
	m.RecursionAvailable = true

	q := r.Question[0]
	log.Printf("[DNS] Intercepted query for %s (type %d) from %s", sanitizeLog(q.Name), q.Qtype, sanitizeLog(remoteAddrString(w)))

	resolvedIP := ip
	if net.ParseIP(ip) == nil {
		// Attempt resolution if it's not a numeric IP
		ips, err := net.LookupIP(ip)
		if err == nil && len(ips) > 0 {
			for _, rIP := range ips {
				if rIP.To4() != nil {
					resolvedIP = rIP.String()
					break
				}
			}

			if resolvedIP == ip && len(ips) > 0 {
				resolvedIP = ips[0].String()
			}
		}
	}

	switch q.Qtype {
	case dns.TypeA, dns.TypeANY:
		if net.ParseIP(resolvedIP) == nil || strings.Contains(resolvedIP, ":") {
			// If it's still not a valid IPv4 address, we can't create an A record.
			// Try CNAME as a fallback if it looks like a hostname.
			if !strings.Contains(resolvedIP, ":") {
				// Normalize hostname for CNAME
				target := resolvedIP
				if !strings.HasSuffix(target, ".") {
					target += "."
				}

				rr, err := dns.NewRR(fmt.Sprintf("%s 60 IN CNAME %s", q.Name, target))
				if err == nil {
					m.Answer = append(m.Answer, rr)

					log.Printf("[DNS] Returning CNAME record %s -> %s", sanitizeLog(q.Name), sanitizeLog(target))
				} else {
					log.Printf("[DNS] Error creating CNAME fallback for %s: %v", sanitizeLog(target), err)

					m.Rcode = dns.RcodeServerFailure
				}
			} else {
				m.Rcode = dns.RcodeServerFailure
			}
		} else {
			rr, err := dns.NewRR(fmt.Sprintf("%s 60 IN A %s", q.Name, resolvedIP))
			if err == nil {
				m.Answer = append(m.Answer, rr)

				log.Printf("[DNS] Returning A record %s -> %s", sanitizeLog(q.Name), sanitizeLog(resolvedIP))
			} else {
				log.Printf("[DNS] Error creating A record for %s: %v", sanitizeLog(resolvedIP), err)

				m.Rcode = dns.RcodeServerFailure
			}
		}
	case dns.TypeAAAA:
		// Check if we have an IPv6 address
		if net.ParseIP(resolvedIP) != nil && strings.Contains(resolvedIP, ":") {
			rr, err := dns.NewRR(fmt.Sprintf("%s 60 IN AAAA %s", q.Name, resolvedIP))
			if err == nil {
				m.Answer = append(m.Answer, rr)

				log.Printf("[DNS] Returning AAAA record %s -> %s", sanitizeLog(q.Name), sanitizeLog(resolvedIP))
			} else {
				log.Printf("[DNS] Error creating AAAA record for %s: %v", sanitizeLog(resolvedIP), err)

				m.Rcode = dns.RcodeServerFailure
			}
		} else {
			// Explicitly return SUCCESS with no data for AAAA to prevent fallback issues if no IPv6
			log.Printf("[DNS] Returning empty AAAA success (NODATA) for %s", sanitizeLog(q.Name))
		}
	default:
		log.Printf("[DNS] Returning empty success for type %d", q.Qtype)
	}

	if err := w.WriteMsg(m); err != nil {
		log.Printf("[DNS ERROR] Failed to write response: %v", err)
	}
}

func (d *DNSDiscovery) forward(w dns.ResponseWriter, r *dns.Msg) {
	if len(r.Question) == 0 {
		return
	}

	q := r.Question[0]

	// Don't forward PTR queries for our own service IP to avoid loops or slow timeouts
	if q.Qtype == dns.TypePTR {
		m := new(dns.Msg)
		m.SetReply(r)

		m.Rcode = dns.RcodeNameError
		if err := w.WriteMsg(m); err != nil {
			log.Printf("[DNS ERROR] Failed to write NXDOMAIN: %v", err)
		}

		return
	}

	c := new(dns.Client)
	c.Timeout = d.timeout

	for _, upstream := range d.upstreamDNS {
		// Add port 53 if not present
		if !strings.Contains(upstream, ":") {
			upstream += ":53"
		}

		// Loop prevention: don't forward to ourselves
		if upstream == d.bindAddr || (strings.HasPrefix(upstream, "127.0.0.1:") && strings.HasSuffix(d.bindAddr, upstream[9:])) {
			d.throttledLog(fmt.Sprintf("[DNS ERROR] Refusing to forward %s to ourselves (%s)", q.Name, upstream))
			continue
		}

		in, _, err := c.Exchange(r, upstream)
		if err == nil {
			if in.Rcode == dns.RcodeSuccess {
				if writeErr := w.WriteMsg(in); writeErr != nil {
					log.Printf("[DNS ERROR] Failed to write forwarded response from %s: %v", sanitizeLog(upstream), writeErr)
				}

				return
			}

			d.throttledLog(fmt.Sprintf("[DNS] Upstream %s returned %s for %s, trying next", upstream, dns.RcodeToString[in.Rcode], q.Name))
		} else {
			d.throttledLog(fmt.Sprintf("[DNS ERROR] Forward failed for %s (type %d) via %s: %v", q.Name, q.Qtype, upstream, err))
		}
	}

	// If we reach here, all upstreams failed
	m := new(dns.Msg)
	m.SetReply(r)
	m.Rcode = dns.RcodeServerFailure

	if err := w.WriteMsg(m); err != nil {
		log.Printf("[DNS ERROR] Failed to write failure response: %v", err)
	}
}

// GetDiscovered returns a map of all discovered hosts.
func (d *DNSDiscovery) GetDiscovered() map[string]*DiscoveredHost {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Return copy
	result := make(map[string]*DiscoveredHost)
	for k, v := range d.discovered {
		result[k] = v
	}

	return result
}

// InterceptClientIPs returns a copy of the set of non-loopback client IPs
// that have queried an intercepted Bose hostname. The value for each key is
// the timestamp of the most recent intercepted query from that client.
// Empty map means no non-loopback speaker has ever used AfterTouch's DNS
// interceptor. Callers receive a snapshot; the map is safe to read after
// this method returns.
func (d *DNSDiscovery) InterceptClientIPs() map[string]time.Time {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make(map[string]time.Time, len(d.interceptClients))
	for k, v := range d.interceptClients {
		result[k] = v
	}

	return result
}

// GetBoseHosts returns a slice of all discovered Bose-related hosts.
func (d *DNSDiscovery) GetBoseHosts() []*DiscoveredHost {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var result []*DiscoveredHost

	for _, host := range d.discovered {
		if host.IsBoseService {
			result = append(result, host)
		}
	}

	return result
}

// SetDiscovered sets the map of discovered hosts.
func (d *DNSDiscovery) SetDiscovered(discovered map[string]*DiscoveredHost) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.discovered = discovered
}

// Start DNS server starts both UDP and TCP listeners
func (d *DNSDiscovery) Start(addr string) error {
	mux := dns.NewServeMux()
	mux.HandleFunc(".", d.ServeDNS)

	d.mu.Lock()
	d.bindAddr = addr
	d.udpServer = &dns.Server{
		Addr:    addr,
		Net:     "udp",
		Handler: mux,
	}

	d.tcpServer = &dns.Server{
		Addr:    addr,
		Net:     "tcp",
		Handler: mux,
	}

	// Capture server references before releasing mutex to avoid race condition
	udpServer := d.udpServer
	tcpServer := d.tcpServer
	d.mu.Unlock()

	errChan := make(chan error, 2)

	go func() {
		log.Printf("[DNS] UDP Discovery server starting on %s", sanitizeLog(addr))

		if err := udpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("UDP server failed: %w", err)
		}
	}()

	go func() {
		log.Printf("[DNS] TCP Discovery server starting on %s", sanitizeLog(addr))

		if err := tcpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("TCP server failed: %w", err)
		}
	}()

	log.Printf("[DNS] Discovery servers starting on %s (upstream: %s, intercept IP: %s)", sanitizeLog(addr), sanitizeLog(fmt.Sprint(d.upstreamDNS)), sanitizeLog(d.serviceIP))

	// Wait for first error
	return <-errChan
}

// IsRunning returns true if the DNS server is active and bound to the specified address.
func (d *DNSDiscovery) IsRunning(addr string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if d.udpServer == nil || d.tcpServer == nil {
		return false
	}

	// We check if the address matches what we expect
	return d.udpServer.Addr == addr && d.tcpServer.Addr == addr
}

// Shutdown stops the DNS server listeners
func (d *DNSDiscovery) Shutdown() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.udpServer != nil {
		if err := d.udpServer.Shutdown(); err != nil {
			log.Printf("[DNS] Error shutting down UDP server: %v", err)
		}

		d.udpServer = nil
	}

	if d.tcpServer != nil {
		if err := d.tcpServer.Shutdown(); err != nil {
			log.Printf("[DNS] Error shutting down TCP server: %v", err)
		}

		d.tcpServer = nil
	}

	return nil
}
