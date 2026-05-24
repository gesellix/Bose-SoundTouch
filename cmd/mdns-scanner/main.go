// Package main provides a simple mDNS scanner to discover SoundTouch devices on the network.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/mdns"
)

func displayResults(services []ServiceInfo) {
	if len(services) == 0 {
		fmt.Println("No services found.")
		fmt.Println()
		fmt.Println("This could mean:")
		fmt.Println("- No mDNS services on network")
		fmt.Println("- Network blocks multicast traffic")
		fmt.Println("- Firewall blocks mDNS port 5353")
		fmt.Println("- Try different service types or increase timeout")

		return
	}

	// Group services by type for better display
	serviceGroups := make(map[string][]ServiceInfo)
	for _, s := range services {
		serviceGroups[s.ServiceType] = append(serviceGroups[s.ServiceType], s)
	}

	// Display grouped services
	for serviceType, serviceList := range serviceGroups {
		fmt.Printf("Service Type: %s\n", serviceType)
		fmt.Printf("  Found %d instance(s):\n", len(serviceList))

		for i, s := range serviceList {
			fmt.Printf("  %d. %s\n", i+1, s.Name)

			if s.Host != "" {
				fmt.Printf("     Host: %s\n", s.Host)
			}

			if s.IPv4 != "" {
				fmt.Printf("     IPv4: %s\n", s.IPv4)
			}

			if s.IPv6 != "" {
				fmt.Printf("     IPv6: %s\n", s.IPv6)
			}

			if s.Port > 0 {
				fmt.Printf("     Port: %d\n", s.Port)
			}

			if len(s.TxtRecords) > 0 {
				fmt.Printf("     TXT Records: %v\n", s.TxtRecords)
			}
		}

		fmt.Println()
	}
}

func showSuggestions(service string) {
	if service == "_services._dns-sd._udp" {
		fmt.Println("Common services to look for SoundTouch devices:")
		fmt.Println("- _soundtouch._tcp.local.")
		fmt.Println("- _http._tcp.local.")
		fmt.Println("- _upnp._tcp.local.")
		fmt.Println("- _device-info._tcp.local.")
		fmt.Println()
		fmt.Println("Try scanning specific services:")
		fmt.Println("  ./mdns-scanner -service _soundtouch._tcp -v")
		fmt.Println("  ./mdns-scanner -service _http._tcp -v")
	}
}

func main() {
	verbose := flag.Bool("verbose", false, "Enable verbose logging")
	timeout := flag.Duration("timeout", 10*time.Second, "Discovery timeout")
	service := flag.String("service", "_services._dns-sd._udp", "Service type to scan for (use _services._dns-sd._udp to find all)")

	flag.Parse()

	// Configure logging
	if *verbose {
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	} else {
		log.SetOutput(os.Stderr)
	}

	fmt.Println("mDNS Service Scanner")
	fmt.Println("===================")
	fmt.Printf("Service: %s\n", *service)
	fmt.Printf("Timeout: %v\n", *timeout)
	fmt.Printf("Verbose: %v\n", *verbose)
	fmt.Println()

	// Create a channel to collect service entries
	entries := make(chan *mdns.ServiceEntry, 1000)

	var services []ServiceInfo

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Start mDNS query in a goroutine
	go func() {
		defer close(entries)

		if *verbose {
			log.Printf("mDNS: Starting scan for service '%s' with timeout %v", sanitizeLog(*service), *timeout)
		}

		// Query for services
		err := mdns.Query(&mdns.QueryParam{
			Service: *service,
			Domain:  "local.",
			Timeout: *timeout,
			Entries: entries,
		})
		if err != nil {
			if *verbose {
				log.Printf("mDNS query completed with error: %v", err)
			}
		} else {
			if *verbose {
				log.Printf("mDNS query completed successfully")
			}
		}
	}()

	// Collect discovered services
	start := time.Now()

	for {
		select {
		case <-ctx.Done():
			// Timeout reached
			goto done
		case entry, ok := <-entries:
			if !ok {
				// Channel closed
				goto done
			}

			if entry != nil {
				service := parseServiceEntry(entry, *verbose)
				if service != nil {
					services = append(services, *service)
				}
			}
		}
	}

done:
	duration := time.Since(start)

	fmt.Printf("Scan completed in %v\n", duration)
	fmt.Printf("Found %d services:\n", len(services))
	fmt.Println()

	// Sort services by name for better display
	sort.Slice(services, func(i, j int) bool {
		return services[i].Name < services[j].Name
	})

	// Display results
	displayResults(services)

	// Show suggestions for common SoundTouch-related services
	showSuggestions(*service)
}

type ServiceInfo struct {
	Name        string
	ServiceType string
	Host        string
	IPv4        string
	IPv6        string
	Port        int
	TxtRecords  []string
}

func parseServiceEntry(entry *mdns.ServiceEntry, verbose bool) *ServiceInfo {
	if entry == nil {
		return nil
	}

	if verbose {
		log.Printf("mDNS: Received service entry: Name='%s', Host='%s', Port=%d, AddrV4=%v, AddrV6=%v",
			sanitizeLog(entry.Name), sanitizeLog(entry.Host), entry.Port, entry.AddrV4, entry.AddrV6)
	}

	service := &ServiceInfo{
		Name: entry.Name,
		Host: entry.Host,
		Port: entry.Port,
	}

	// Extract service type from name (e.g. "MyDevice._http._tcp.local." -> "_http._tcp")
	if entry.Name != "" {
		parts := strings.Split(entry.Name, ".")
		if len(parts) >= 3 {
			// Look for service type pattern: _service._protocol
			for i := 0; i < len(parts)-2; i++ {
				if strings.HasPrefix(parts[i], "_") && strings.HasPrefix(parts[i+1], "_") {
					service.ServiceType = parts[i] + "." + parts[i+1]
					break
				}
			}
		}
	}

	// Get IP addresses
	if entry.AddrV4 != nil {
		service.IPv4 = entry.AddrV4.String()
	}

	if entry.AddrV6 != nil {
		service.IPv6 = entry.AddrV6.String()
	}

	// Parse TXT records if available
	if len(entry.InfoFields) > 0 {
		service.TxtRecords = entry.InfoFields
	}

	return service
}
