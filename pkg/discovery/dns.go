// Package discovery provides DNS-based discovery and interception for Bose SoundTouch devices.
package discovery

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// DNSDiscovery handles DNS queries and records discovered hosts.
type DNSDiscovery struct {
	// Configuration
	upstreamDNS string
	serviceIP   string

	// State
	discovered map[string]*DiscoveredHost
	mu         sync.RWMutex

	// Callbacks
	onNewDiscovery func(hostname string)

	// Servers for Shutdown
	udpServer *dns.Server
	tcpServer *dns.Server

	// Address for loop prevention
	bindAddr string

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

// NewDNSDiscovery creates a new DNSDiscovery instance.
func NewDNSDiscovery(upstreamDNS, serviceIP string) *DNSDiscovery {
	return &DNSDiscovery{
		upstreamDNS: upstreamDNS,
		serviceIP:   serviceIP,
		discovered:  make(map[string]*DiscoveredHost),
		lastLog:     make(map[string]time.Time),
	}
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
		if d.upstreamDNS == "" {
			d.throttledLog("[DNS ERROR] No upstream DNS configured, cannot forward")

			m := new(dns.Msg)
			m.SetReply(r)
			m.Rcode = dns.RcodeServerFailure
			_ = w.WriteMsg(m)

			return
		}

		d.throttledLog(fmt.Sprintf("[DNS] Forwarding %s (type %d) to %s", hostname, q.Qtype, d.upstreamDNS))
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
			hostname, host.IsBoseService, host.IsIntercepted)

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
}

func (d *DNSDiscovery) shouldIntercept(hostname string) bool {
	// Intercept known Bose cloud services
	interceptList := []string{
		"api.bose.com",
		"marge.bose.com",
		"bmx.bose.com",
		"streaming.bose.com",
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
	}

	for _, service := range interceptList {
		if strings.Contains(hostname, service) {
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
	log.Printf("[DNS] Intercepted query for %s (type %d) from %s", q.Name, q.Qtype, w.RemoteAddr())

	switch q.Qtype {
	case dns.TypeA, dns.TypeANY:
		rr, err := dns.NewRR(fmt.Sprintf("%s 60 IN A %s", q.Name, ip))
		if err == nil {
			m.Answer = append(m.Answer, rr)

			log.Printf("[DNS] Returning A record %s -> %s", q.Name, ip)
		} else {
			log.Printf("[DNS] Error creating A record: %v", err)
		}
	case dns.TypeAAAA:
		// Explicitly return SUCCESS with no data for AAAA to prevent fallback issues
		log.Printf("[DNS] Returning empty AAAA success (NODATA) for %s", q.Name)
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

	// Add port 53 if not present
	upstream := d.upstreamDNS
	if !strings.Contains(upstream, ":") {
		upstream += ":53"
	}

	// Loop prevention: don't forward to ourselves
	if upstream == d.bindAddr || (strings.HasPrefix(upstream, "127.0.0.1:") && strings.HasSuffix(d.bindAddr, upstream[9:])) {
		d.throttledLog(fmt.Sprintf("[DNS ERROR] Refusing to forward %s to ourselves (%s)", q.Name, upstream))

		m := new(dns.Msg)
		m.SetReply(r)
		m.Rcode = dns.RcodeServerFailure
		_ = w.WriteMsg(m)

		return
	}

	c := new(dns.Client)
	c.Timeout = 2 * time.Second

	in, _, err := c.Exchange(r, upstream)
	if err != nil {
		d.throttledLog(fmt.Sprintf("[DNS ERROR] Forward failed for %s (type %d): %v", q.Name, q.Qtype, err))
		// Return a failure response instead of just dropping
		m := new(dns.Msg)
		m.SetReply(r)

		m.Rcode = dns.RcodeServerFailure
		if err := w.WriteMsg(m); err != nil {
			log.Printf("[DNS ERROR] Failed to write failure response: %v", err)
		}

		return
	}

	if err := w.WriteMsg(in); err != nil {
		log.Printf("[DNS ERROR] Failed to write forwarded response: %v", err)
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
		log.Printf("[DNS] UDP Discovery server starting on %s", addr)

		if err := udpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("UDP server failed: %w", err)
		}
	}()

	go func() {
		log.Printf("[DNS] TCP Discovery server starting on %s", addr)

		if err := tcpServer.ListenAndServe(); err != nil {
			errChan <- fmt.Errorf("TCP server failed: %w", err)
		}
	}()

	log.Printf("[DNS] Discovery servers starting on %s (upstream: %s, intercept IP: %s)", addr, d.upstreamDNS, d.serviceIP)

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
