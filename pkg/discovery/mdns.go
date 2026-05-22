package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/hashicorp/mdns"
)

// MDNSDiscoveryService handles mDNS/Bonjour discovery of SoundTouch devices
type MDNSDiscoveryService struct {
	timeout   time.Duration
	ifaceName string
}

// NewMDNSDiscoveryService creates a new mDNS discovery service
func NewMDNSDiscoveryService(timeout time.Duration) *MDNSDiscoveryService {
	return NewMDNSDiscoveryServiceWithInterface(timeout, "")
}

// NewMDNSDiscoveryServiceWithInterface creates a new mDNS discovery service
// pinned to the given network interface (e.g. "eth0"). An empty ifaceName
// falls back to the historical auto-pick behaviour.
func NewMDNSDiscoveryServiceWithInterface(timeout time.Duration, ifaceName string) *MDNSDiscoveryService {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &MDNSDiscoveryService{
		timeout:   timeout,
		ifaceName: ifaceName,
	}
}

// DiscoverDevices discovers SoundTouch devices using mDNS
func (m *MDNSDiscoveryService) DiscoverDevices(ctx context.Context) ([]*models.DiscoveredDevice, error) {
	// Initialize devices slice to ensure it's never nil
	devices := make([]*models.DiscoveredDevice, 0)

	// Create a channel to collect service entries
	entries := make(chan *mdns.ServiceEntry, 100)

	// Create a timeout context
	timeoutCtx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	// Fan out one query per SoundTouch service-type variant; mDNS has no
	// wildcard service-type query, so we issue them in parallel and merge
	// into a single entries channel. close(entries) only once all queries
	// are done (or the timeout fires).
	go func() {
		defer close(entries)

		log.Printf("mDNS: Starting discovery for %d service-type variant(s) with timeout %v",
			len(soundTouchServiceTypes), m.timeout)

		var wg sync.WaitGroup

		for _, service := range soundTouchServiceTypes {
			wg.Add(1)

			go func(service string) {
				defer wg.Done()

				m.queryService(service, entries)
			}(service)
		}

		wg.Wait()
		logVerbose("mDNS: All %d service-type queries finished", len(soundTouchServiceTypes))
	}()

	// Collect discovered devices, deduplicating by host:port since a single
	// speaker may answer multiple service types (older firmware advertises
	// both `_soundtouch._tcp` and `_bose-soundtouch._tcp` simultaneously).
	seen := make(map[string]bool)

	for {
		select {
		case <-timeoutCtx.Done():
			// Timeout reached, return what we have
			return devices, nil
		case entry, ok := <-entries:
			if !ok {
				// Channel closed, return collected devices
				log.Printf("mDNS discovery finished. Found %d devices total.", len(devices))
				return devices, nil
			}

			logVerbose("mDNS: Received service entry: Name='%s', Host='%s', Port=%d, AddrV4=%v, AddrV6=%v",
				entry.Name, entry.Host, entry.Port, entry.AddrV4, entry.AddrV6)

			// Only process SoundTouch-family services.
			if !isSoundTouchServiceName(entry.Name) {
				logVerbose("mDNS: Skipping non-SoundTouch service: %s", entry.Name)
				continue
			}

			device := m.serviceEntryToDevice(entry)
			if device == nil {
				log.Printf("mDNS: Failed to convert service entry to device (no valid IP address)")
				continue
			}

			key := fmt.Sprintf("%s:%d", device.Host, device.Port)
			if seen[key] {
				logVerbose("mDNS: Skipping duplicate device %s (already seen via another service-type query)", key)
				continue
			}

			seen[key] = true

			logVerbose("mDNS: Successfully converted to device: %s at %s:%d", device.Name, device.Host, device.Port)
			devices = append(devices, device)
		}
	}
}

// queryService issues a single mDNS Query for the given service type
// against the IPv4 interface first, with a graceful fallback to the
// library's default (IPv4+IPv6) behaviour if the IPv4-only path fails.
// All results stream into the shared entries channel; the caller is
// responsible for fan-in deduplication.
func (m *MDNSDiscoveryService) queryService(service string, entries chan<- *mdns.ServiceEntry) {
	logVerbose("mDNS: Query '%s.%s' starting", service, soundTouchDomain)

	err := mdns.Query(&mdns.QueryParam{
		Service:     service,
		Domain:      "local.",
		Timeout:     m.timeout,
		Entries:     entries,
		DisableIPv6: true,
		Interface:   m.getIPv4Interface(),
	})
	if err == nil {
		logVerbose("mDNS: Query '%s' (IPv4) completed successfully", service)
		return
	}

	log.Printf("mDNS: Query '%s' (IPv4) failed: %v — falling back to dual-stack", service, err)

	err = mdns.Query(&mdns.QueryParam{
		Service: service,
		Domain:  "local.",
		Timeout: m.timeout,
		Entries: entries,
	})
	if err != nil {
		log.Printf("mDNS: Query '%s' (dual-stack) failed: %v", service, err)
	} else {
		logVerbose("mDNS: Query '%s' (dual-stack) completed successfully", service)
	}
}

// serviceEntryToDevice converts an mdns ServiceEntry to a DiscoveredDevice
func (m *MDNSDiscoveryService) serviceEntryToDevice(entry *mdns.ServiceEntry) *models.DiscoveredDevice {
	if entry == nil {
		logVerbose("mDNS: Received nil service entry")
		return nil
	}

	// Get the IP address - prefer IPv4
	var (
		host     string
		ipSource string
	)

	switch {
	case entry.AddrV4 != nil:
		host = entry.AddrV4.String()
		ipSource = "IPv4"

		logVerbose("mDNS: Using IPv4 address: %s", host)
	case entry.AddrV6 != nil:
		host = entry.AddrV6.String()
		ipSource = "IPv6"

		logVerbose("mDNS: Using IPv6 address: %s", host)
	default:
		// Try to resolve from hostname
		logVerbose("mDNS: No direct IP address, trying to resolve hostname: %s", entry.Host)

		ips, err := net.LookupIP(entry.Host)
		if err != nil || len(ips) == 0 {
			log.Printf("mDNS: Failed to resolve hostname '%s': %v", entry.Host, err)
			return nil
		}

		// Prefer IPv4
		for _, ip := range ips {
			if ip.To4() != nil {
				host = ip.String()
				ipSource = "resolved IPv4"

				logVerbose("mDNS: Resolved to IPv4 address: %s", host)

				break
			}
		}

		// If no IPv4 found, use first available
		if host == "" {
			host = ips[0].String()
			if ips[0].To4() != nil {
				ipSource = "resolved IPv4 (fallback)"
			} else {
				ipSource = "resolved IPv6 (fallback)"
			}

			logVerbose("mDNS: Using fallback address (%s): %s", ipSource, host)
		}
	}

	if host == "" {
		log.Printf("mDNS: No usable IP address found for entry")
		return nil
	}

	port := entry.Port
	// Default to port 8090 if port is 0 or invalid
	if port == 0 {
		port = 8090
	}

	// Extract device name from instance name or use a default
	name := entry.Name
	if name == "" {
		name = fmt.Sprintf("SoundTouch-%s", host)
	}

	// Clean up the name by removing the service type suffix
	if strings.HasSuffix(name, "."+soundTouchServiceType+"."+soundTouchDomain) {
		name = strings.TrimSuffix(name, "."+soundTouchServiceType+"."+soundTouchDomain)
	}

	// Unescape any escaped characters in the name (common in mDNS)
	name = strings.ReplaceAll(name, `\ `, " ")
	name = strings.ReplaceAll(name, `\.`, ".")
	name = strings.ReplaceAll(name, `\\`, `\`)

	device := &models.DiscoveredDevice{
		Host:            host,
		Port:            port,
		Name:            name,
		LastSeen:        time.Now(),
		DiscoveryMethod: "mDNS/Bonjour",
		APIBaseURL:      fmt.Sprintf("http://%s:%d/", host, port),
		InfoURL:         fmt.Sprintf("http://%s:%d/info", host, port),
		MDNSHostname:    entry.Host,
		MDNSService:     entry.Name,
	}

	logVerbose("mDNS: Created device '%s' at %s:%d (IP source: %s)", name, host, port, ipSource)

	return device
}

// getIPv4Interface returns the network interface to use for mDNS queries.
// If an explicit name was configured, it is resolved and validated; otherwise
// the first suitable, up, non-loopback IPv4 interface is returned.
func (m *MDNSDiscoveryService) getIPv4Interface() *net.Interface {
	if m.ifaceName != "" {
		iface, err := net.InterfaceByName(m.ifaceName)
		if err != nil {
			log.Printf("mDNS: Configured interface %q not found: %v", m.ifaceName, err)
			return nil
		}

		if !interfaceHasIPv4(iface) {
			log.Printf("mDNS: Configured interface %q has no usable IPv4 address", m.ifaceName)
			return nil
		}

		logVerbose("mDNS: Using configured IPv4 interface: %s", iface.Name)

		return iface
	}

	interfaces, err := net.Interfaces()
	if err != nil {
		log.Printf("mDNS: Failed to get network interfaces: %v", err)
		return nil
	}

	for _, iface := range interfaces {
		// Skip loopback, down interfaces, and point-to-point interfaces
		if iface.Flags&net.FlagLoopback != 0 ||
			iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagPointToPoint != 0 {
			continue
		}

		if !interfaceHasIPv4(&iface) {
			continue
		}

		logVerbose("mDNS: Using IPv4 interface: %s", iface.Name)

		return &iface
	}

	logVerbose("mDNS: No suitable IPv4 interface found")

	return nil
}

// interfaceHasIPv4 reports whether iface has at least one non-loopback IPv4
// address assigned and is administratively up.
func interfaceHasIPv4(iface *net.Interface) bool {
	if iface.Flags&net.FlagUp == 0 {
		return false
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		if ipNet.IP.To4() != nil && !ipNet.IP.IsLoopback() {
			return true
		}
	}

	return false
}
