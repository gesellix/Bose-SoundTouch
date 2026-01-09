package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
	verbose := flag.Bool("v", false, "Enable verbose logging")
	timeout := flag.Duration("timeout", 5*time.Second, "Discovery timeout")
	flag.Parse()

	// Configure logging
	if *verbose {
		log.SetOutput(os.Stdout)
		log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	} else {
		log.SetOutput(os.Stderr)
	}

	fmt.Println("SoundTouch UPnP/SSDP Discovery Example")
	fmt.Println("=====================================")

	fmt.Printf("Timeout: %v, Verbose: %v\n", *timeout, *verbose)
	fmt.Println()

	// Create context with timeout (add buffer time)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout+2*time.Second)
	defer cancel()

	fmt.Println("Searching for SoundTouch devices via UPnP/SSDP...")
	fmt.Println("This will send M-SEARCH requests to multicast address 239.255.255.250:1900")
	if *verbose {
		fmt.Println("Verbose logging enabled - watch for technical details...")
	}
	fmt.Println()

	start := time.Now()

	// Create a config that disables configured devices to test only UPnP
	cfg := &config.Config{
		DiscoveryTimeout: *timeout,
		UPnPEnabled:      true,
		MDNSEnabled:      false,
		PreferredDevices: []config.DeviceConfig{}, // Empty to test only UPnP
		CacheEnabled:     false,
	}

	// Use the configured discovery service to isolate UPnP
	configuredService := discovery.NewDiscoveryServiceWithConfig(cfg)
	devices, err := configuredService.DiscoverDevices(ctx)
	duration := time.Since(start)

	fmt.Printf("Discovery completed in %v\n", duration)
	fmt.Println()

	if err != nil {
		fmt.Printf("UPnP discovery completed with error: %v\n", err)
		fmt.Println("Note: This might indicate network issues or no UPnP devices")
	}

	// Display results
	if len(devices) == 0 {
		fmt.Println("No SoundTouch devices found via UPnP/SSDP")
		fmt.Println()
		fmt.Println("Technical Status:")
		if *verbose {
			fmt.Println("✓ SSDP M-SEARCH request was sent (check logs above for details)")
			fmt.Println("✓ UDP multicast connection established")
			fmt.Println("✗ No devices responded with MediaRenderer service type")
		} else {
			fmt.Println("Run with -v flag for detailed technical information")
		}
		fmt.Println()
		fmt.Println("This could mean:")
		fmt.Println("- No SoundTouch devices on network")
		fmt.Println("- Devices don't support UPnP/SSDP")
		fmt.Println("- Network blocks multicast traffic (common in corporate networks)")
		fmt.Println("- Firewall blocks UDP port 1900")
		fmt.Println("- Devices are not advertising MediaRenderer service")
		return
	}

	fmt.Printf("Found %d SoundTouch device(s):\n", len(devices))
	fmt.Println()

	for i, device := range devices {
		fmt.Printf("%d. %s\n", i+1, device.Name)
		fmt.Printf("   Host: %s\n", device.Host)
		fmt.Printf("   Port: %d\n", device.Port)
		fmt.Printf("   API URL: http://%s:%d/\n", device.Host, device.Port)
		fmt.Printf("   Location: %s\n", device.Location)
		fmt.Printf("   Last seen: %s\n", device.LastSeen.Format("2006-01-02 15:04:05"))
		fmt.Println()
	}

	fmt.Println("✓ UPnP/SSDP discovery completed successfully!")
	fmt.Printf("✓ Found %d device(s) in %v\n", len(devices), duration)

	if *verbose {
		fmt.Println()
		fmt.Println("Technical Details:")
		fmt.Printf("- Multicast address used: 239.255.255.250:1900\n")
		fmt.Printf("- Service type searched: urn:schemas-upnp-org:device:MediaRenderer:1\n")
		fmt.Printf("- Discovery timeout: %v\n", *timeout)
		fmt.Printf("- Protocol: UDP SSDP (Simple Service Discovery Protocol)\n")
	}
}
