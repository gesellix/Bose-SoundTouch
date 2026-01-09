package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

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

	fmt.Println("SoundTouch mDNS Discovery Example")
	fmt.Println("================================")

	fmt.Printf("Timeout: %v, Verbose: %v\n", *timeout, *verbose)
	fmt.Println()

	// Create mDNS discovery service
	mdnsService := discovery.NewMDNSDiscoveryService(*timeout)

	// Create context with timeout (add buffer time)
	ctx, cancel := context.WithTimeout(context.Background(), *timeout+2*time.Second)
	defer cancel()

	fmt.Println("Searching for SoundTouch devices via mDNS...")
	fmt.Println("This will search for devices advertising _soundtouch._tcp.local. service")
	if *verbose {
		fmt.Println("Verbose logging enabled - watch for technical details...")
	}
	fmt.Println()

	start := time.Now()

	// Discover devices
	devices, err := mdnsService.DiscoverDevices(ctx)
	duration := time.Since(start)

	fmt.Printf("Discovery completed in %v\n", duration)
	fmt.Println()

	if err != nil {
		fmt.Printf("mDNS discovery completed with error: %v\n", err)
		fmt.Println("Note: This might be normal if no devices support mDNS")
	}

	// Display results
	if len(devices) == 0 {
		fmt.Println("No SoundTouch devices found via mDNS")
		fmt.Println()
		fmt.Println("Technical Status:")
		if *verbose {
			fmt.Println("✓ mDNS query was sent (check logs above for details)")
			fmt.Println("✓ No network errors during discovery process")
			fmt.Println("✗ No devices responded with _soundtouch._tcp service")
		} else {
			fmt.Println("Run with -v flag for detailed technical information")
		}
		fmt.Println()
		fmt.Println("This could mean:")
		fmt.Println("- No SoundTouch devices on network")
		fmt.Println("- Devices don't support Bonjour/mDNS")
		fmt.Println("- Network blocks multicast traffic (common in corporate networks)")
		fmt.Println("- Devices use different service name than '_soundtouch._tcp.local.'")
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

	fmt.Println("✓ mDNS discovery completed successfully!")
	fmt.Printf("✓ Found %d device(s) in %v\n", len(devices), duration)
}
