package discovery

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
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

	// Start mDNS query in a goroutine
	go func() {
		defer close(entries)

		log.Printf("mDNS: Starting discovery for service '%s.%s' with timeout %v",
			soundTouchServiceType, soundTouchDomain, m.timeout)

		// IPv4-only query to fix "no route to host" errors on IPv6
		// This addresses the issue where hashicorp/mdns fails with:
		// "write udp6 [::]:port->[ff02::fb]:5353: sendto: no route to host"
		// The trailing dot in service names is handled correctly by separating
		// service and domain parameters as expected by the library.
		err := mdns.Query(&mdns.QueryParam{
			Service:     "_soundtouch._tcp",
			Domain:      "local.",
			Timeout:     m.timeout,
			Entries:     entries,
			DisableIPv6: true,                 // Force IPv4 only to avoid routing issues
			Interface:   m.getIPv4Interface(), // Use specific interface if available
		})
		if err != nil {
			log.Printf("mDNS IPv4 query failed: %v", err)

			// Fallback to standard query (both IPv4 and IPv6)
			log.Printf("mDNS: Falling back to standard query...")

			err = mdns.Query(&mdns.QueryParam{
				Service: "_soundtouch._tcp",
				Domain:  "local.",
				Timeout: m.timeout,
				Entries: entries,
			})
			if err != nil {
				log.Printf("mDNS query completed with error: %v", err)
			} else {
				log.Printf("mDNS query completed successfully")
			}
		} else {
			log.Printf("mDNS IPv4 query completed successfully")
		}
	}()

	// Collect discovered devices
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

			log.Printf("mDNS: Received service entry: Name='%s', Host='%s', Port=%d, AddrV4=%v, AddrV6=%v",
				entry.Name, entry.Host, entry.Port, entry.AddrV4, entry.AddrV6)

			// Only process SoundTouch devices
			if !strings.Contains(entry.Name, "_soundtouch._tcp") {
				log.Printf("mDNS: Skipping non-SoundTouch service: %s", entry.Name)
				continue
			}

			device := m.serviceEntryToDevice(entry)
			if device != nil {
				log.Printf("mDNS: Successfully converted to device: %s at %s:%d", device.Name, device.Host, device.Port)
				devices = append(devices, device)
			} else {
				log.Printf("mDNS: Failed to convert service entry to device (no valid IP address)")
			}
		}
	}
}

// serviceEntryToDevice converts an mdns ServiceEntry to a DiscoveredDevice
func (m *MDNSDiscoveryService) serviceEntryToDevice(entry *mdns.ServiceEntry) *models.DiscoveredDevice {
	if entry == nil {
		log.Printf("mDNS: Received nil service entry")
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

		log.Printf("mDNS: Using IPv4 address: %s", host)
	case entry.AddrV6 != nil:
		host = entry.AddrV6.String()
		ipSource = "IPv6"

		log.Printf("mDNS: Using IPv6 address: %s", host)
	default:
		// Try to resolve from hostname
		log.Printf("mDNS: No direct IP address, trying to resolve hostname: %s", entry.Host)

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

				log.Printf("mDNS: Resolved to IPv4 address: %s", host)

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

			log.Printf("mDNS: Using fallback address (%s): %s", ipSource, host)
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

	log.Printf("mDNS: Created device '%s' at %s:%d (IP source: %s)", name, host, port, ipSource)

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

		log.Printf("mDNS: Using configured IPv4 interface: %s", iface.Name)

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

		log.Printf("mDNS: Using IPv4 interface: %s", iface.Name)

		return &iface
	}

	log.Printf("mDNS: No suitable IPv4 interface found")

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
