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
	timeout time.Duration
}

// NewMDNSDiscoveryService creates a new mDNS discovery service
func NewMDNSDiscoveryService(timeout time.Duration) *MDNSDiscoveryService {
	if timeout == 0 {
		timeout = defaultTimeout
	}

	return &MDNSDiscoveryService{
		timeout: timeout,
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

		// Query for SoundTouch devices
		// Note: hashicorp/mdns expects service and domain separately
		err := mdns.Query(&mdns.QueryParam{
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
	var host string
	var ipSource string

	if entry.AddrV4 != nil {
		host = entry.AddrV4.String()
		ipSource = "IPv4"
		log.Printf("mDNS: Using IPv4 address: %s", host)
	} else if entry.AddrV6 != nil {
		host = entry.AddrV6.String()
		ipSource = "IPv6"
		log.Printf("mDNS: Using IPv6 address: %s", host)
	} else {
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

	device := &models.DiscoveredDevice{
		Host:     host,
		Port:     port,
		Name:     name,
		Location: fmt.Sprintf("http://%s:%d/info", host, port),
		LastSeen: time.Now(),
	}

	log.Printf("mDNS: Created device '%s' at %s:%d (IP source: %s)", name, host, port, ipSource)
	return device
}
