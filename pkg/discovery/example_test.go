package discovery_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
)

// Example demonstrates basic device discovery.
func Example() {
	service := discovery.NewService(5 * time.Second)
	ctx := context.Background()

	// Discover all SoundTouch devices on the network
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d devices:\n", len(devices))

	for _, device := range devices {
		fmt.Printf("- %s at %s:%d\n", device.Name, device.Host, device.Port)
	}

	// Example output:
	// Found 2 devices:
	// - Living Room at 192.0.2.100:8090
	// - Kitchen at 192.0.2.101:8090
}

// ExampleService_DiscoverDevices demonstrates discovering devices with timeout.
func ExampleService_DiscoverDevices() {
	service := discovery.NewService(3 * time.Second)
	ctx := context.Background()

	// Quick discovery with 3 second timeout
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		log.Fatal(err)
	}

	if len(devices) == 0 {
		fmt.Println("No SoundTouch devices found")
		return
	}

	// Print detailed device information
	for _, device := range devices {
		fmt.Printf("Device: %s\n", device.Name)
		fmt.Printf("  Address: %s:%d\n", device.Host, device.Port)
		fmt.Printf("  Serial: %s\n", device.SerialNo)
		fmt.Printf("  Info URL: %s\n", device.InfoURL)
		fmt.Printf("  Host: %s:%d\n", device.Host, device.Port)
		fmt.Println()
	}

	// Example output:
	// Device: Living Room
	//   Address: 192.0.2.100:8090
	//   Serial: AA123456789
	//   Location: /device.xml
	//   Host: 192.0.2.100:8090
	//
	// Device: Kitchen
	//   Address: 192.0.2.101:8090
	//   Serial: BB123456789
	//   Location: /device.xml
	//   Host: 192.0.2.101:8090
}

// ExampleUnifiedDiscoveryService_DiscoverDevices demonstrates caching functionality.
func ExampleUnifiedDiscoveryService_DiscoverDevices() {
	cfg := &config.Config{
		DiscoveryTimeout: 5 * time.Second,
		CacheEnabled:     true,
		CacheTTL:         5 * time.Minute,
	}
	service := discovery.NewUnifiedDiscoveryService(cfg)

	ctx := context.Background()

	// First discovery scan
	fmt.Println("First scan:")

	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d devices\n", len(devices))

	// Second scan (should use cache)
	fmt.Println("Second scan (cached):")

	devices, err = service.DiscoverDevices(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Found %d devices (from cache)\n", len(devices))

	// Example output:
	// First scan:
	// Found 2 devices
	// Second scan (cached):
	// Found 2 devices (from cache)
}

// Example_upnpOnlyDiscovery demonstrates UPnP-only discovery.
func Example_upnpOnlyDiscovery() {
	service := discovery.NewService(3 * time.Second)
	ctx := context.Background()

	// Use UPnP/SSDP discovery
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("UPnP discovered %d devices:\n", len(devices))

	for _, device := range devices {
		fmt.Printf("- %s at %s:%d\n", device.Name, device.Host, device.Port)
	}

	// Example output:
	// UPnP discovered 1 devices:
	// - Living Room at 192.0.2.100:8090
}

// ExampleMDNSDiscoveryService_DiscoverDevices demonstrates mDNS-only discovery.
func ExampleMDNSDiscoveryService_DiscoverDevices() {
	service := discovery.NewMDNSDiscoveryService(3 * time.Second)
	ctx := context.Background()

	// Use only mDNS discovery
	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("mDNS discovered %d devices:\n", len(devices))

	for _, device := range devices {
		fmt.Printf("- %s at %s:%d\n", device.Name, device.Host, device.Port)
	}

	// Example output:
	// mDNS discovered 1 devices:
	// - Kitchen at 192.0.2.101:8090
}

// Example_errorHandling demonstrates proper error handling in discovery.
func Example_errorHandling() {
	// Very short timeout to demonstrate timeout handling
	service := discovery.NewService(100 * time.Millisecond)
	ctx := context.Background()

	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		fmt.Printf("Discovery error: %v\n", err)
		return
	}

	if len(devices) == 0 {
		fmt.Println("No devices found - check network connectivity")
		return
	}

	fmt.Printf("Found %d devices despite short timeout\n", len(devices))

	// Example output:
	// No devices found - check network connectivity
}

// Example_contextCancellation demonstrates context cancellation.
func Example_contextCancellation() {
	// Create a context that cancels after 2 seconds
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	service := discovery.NewService(10 * time.Second)

	devices, err := service.DiscoverDevices(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			fmt.Println("Discovery cancelled due to context timeout")
		} else {
			fmt.Printf("Discovery error: %v\n", err)
		}

		return
	}

	fmt.Printf("Found %d devices before context cancellation\n", len(devices))

	// Example output:
	// Discovery cancelled due to context timeout
}
