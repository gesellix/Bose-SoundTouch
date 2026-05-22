package main

import (
	"context"
	"fmt"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// discoverDevices handles device discovery command
func discoverDevices(c *cli.Context) error {
	fmt.Printf("Discovering SoundTouch devices...\n")

	// CLI discovery is interactive — flip on verbose protocol logging
	// so operators can see per-packet / per-header detail. The service
	// binary leaves this off so its log stays terse.
	discovery.SetVerbose(c.Bool("verbose"))

	// Load configuration
	cfg, err := config.LoadFromEnv()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Update config with CLI flags
	updateConfigFromCLI(c, cfg)

	if c.Bool("all") {
		printDiscoveryContext(cfg)
	}

	fmt.Println()

	// Create discovery service
	discoveryService := discovery.NewUnifiedDiscoveryService(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), cfg.DiscoveryTimeout+5*time.Second)
	defer cancel()

	// Perform discovery
	devices, err := discoveryService.DiscoverDevices(ctx)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	if len(devices) == 0 {
		printNoDevicesMessage()
		return nil
	}

	// Display results
	printDiscoveryResults(devices, c.Bool("all"))

	return nil
}

func updateConfigFromCLI(c *cli.Context, cfg *config.Config) {
	if c.IsSet("timeout") {
		httpTimeout := c.Duration("timeout")
		cfg.HTTPTimeout = httpTimeout
		// Set discovery timeout to be 2x HTTP timeout (min 5s, max 30s)
		discoveryTimeout := httpTimeout * 2
		if discoveryTimeout < 5*time.Second {
			discoveryTimeout = 5 * time.Second
		}

		if discoveryTimeout > 30*time.Second {
			discoveryTimeout = 30 * time.Second
		}

		cfg.DiscoveryTimeout = discoveryTimeout
	}
}

func printDiscoveryContext(cfg *config.Config) {
	fmt.Printf("HTTP Timeout: %v\n", cfg.HTTPTimeout)
	fmt.Printf("Discovery Timeout: %v\n", cfg.DiscoveryTimeout)
	fmt.Printf("Mode: Detailed information\n")
}

func printNoDevicesMessage() {
	fmt.Println("No SoundTouch devices found on the network.")
	fmt.Println()
	fmt.Println("This could mean:")
	fmt.Println("- No SoundTouch devices are powered on")
	fmt.Println("- Devices are on a different network segment")
	fmt.Println("- Network blocks multicast traffic")
	fmt.Println("- Firewall is blocking discovery ports")
}

func printDiscoveryResults(devices []*models.DiscoveredDevice, showAll bool) {
	fmt.Printf("Found %d SoundTouch device(s):\n\n", len(devices))

	for i, device := range devices {
		fmt.Printf("%d. %s\n", i+1, device.Name)
		fmt.Printf("   Host: %s:%d\n", device.Host, device.Port)
		fmt.Printf("   Model: %s\n", device.ModelID)

		if device.SerialNo != "" {
			fmt.Printf("   Serial: %s\n", device.SerialNo)
		}

		if device.APIBaseURL != "" {
			fmt.Printf("   API Base URL: %s\n", device.APIBaseURL)
		}

		if device.InfoURL != "" {
			fmt.Printf("   Info URL: %s\n", device.InfoURL)
		}

		if device.DiscoveryMethod != "" {
			fmt.Printf("   Discovery Method: %s\n", device.DiscoveryMethod)
		}

		if showAll {
			// Show protocol-specific details in verbose mode
			if device.UPnPLocation != "" {
				fmt.Printf("   UPnP Location: %s\n", device.UPnPLocation)
			}

			if device.UPnPUSN != "" {
				fmt.Printf("   UPnP USN: %s\n", device.UPnPUSN)
			}

			if device.MDNSHostname != "" {
				fmt.Printf("   mDNS Hostname: %s\n", device.MDNSHostname)
			}

			if device.MDNSService != "" {
				fmt.Printf("   mDNS Service: %s\n", device.MDNSService)
			}

			if device.ConfigName != "" {
				fmt.Printf("   Config Name: %s\n", device.ConfigName)
			}

			fmt.Printf("   Last Seen: %s\n", device.LastSeen.Format("2006-01-02 15:04:05"))
		}

		// Add spacing between devices
		if i < len(devices)-1 {
			fmt.Println()
		}
	}

	fmt.Println()
	fmt.Printf("Use any of these hosts with other commands:\n")
	fmt.Printf("Example: soundtouch-cli info --host %s\n", devices[0].Host)
}
