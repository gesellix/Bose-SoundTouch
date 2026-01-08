package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
	var (
		host        = flag.String("host", "", "SoundTouch device host/IP address")
		port        = flag.Int("port", 8090, "SoundTouch device port")
		timeout     = flag.Duration("timeout", 10*time.Second, "Request timeout")
		discover    = flag.Bool("discover", false, "Discover SoundTouch devices via UPnP")
		discoverAll = flag.Bool("discover-all", false, "Discover all SoundTouch devices and show info")
		info        = flag.Bool("info", false, "Get device information")
		help        = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// If no specific action is requested, show help
	if !*discover && !*discoverAll && !*info && *host == "" {
		printHelp()
		return
	}

	// Handle discovery
	if *discover || *discoverAll {
		if err := handleDiscovery(*discoverAll, *timeout); err != nil {
			log.Fatalf("Discovery failed: %v", err)
		}
		return
	}

	// Handle device info
	if *info {
		if *host == "" {
			log.Fatal("Host is required for info command. Use -host flag or -discover to find devices.")
		}
		if err := handleDeviceInfo(*host, *port, *timeout); err != nil {
			log.Fatalf("Failed to get device info: %v", err)
		}
		return
	}
}

func printHelp() {
	fmt.Println("SoundTouch CLI - Test tool for Bose SoundTouch API")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  soundtouch-cli [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -host <ip>        SoundTouch device IP address")
	fmt.Println("  -port <port>      SoundTouch device port (default: 8090)")
	fmt.Println("  -timeout <dur>    Request timeout (default: 10s)")
	fmt.Println("  -discover         Discover SoundTouch devices via UPnP")
	fmt.Println("  -discover-all     Discover devices and show detailed info")
	fmt.Println("  -info             Get device information (requires -host)")
	fmt.Println("  -help             Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  soundtouch-cli -discover")
	fmt.Println("  soundtouch-cli -discover-all")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -info")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -port 8090 -info")
}

func handleDiscovery(showInfo bool, timeout time.Duration) error {
	fmt.Println("Discovering SoundTouch devices...")

	// Load configuration from environment and .env file
	cfg, err := config.LoadFromEnv()
	if err != nil {
		fmt.Printf("Warning: Failed to load configuration: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Override timeout if provided via command line
	if timeout > 0 {
		cfg.DiscoveryTimeout = timeout
	}

	discoveryService := discovery.NewDiscoveryServiceWithConfig(cfg)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DiscoveryTimeout+5*time.Second)
	defer cancel()

	devices, err := discoveryService.DiscoverDevices(ctx)
	if err != nil {
		return fmt.Errorf("discovery failed: %w", err)
	}

	if len(devices) == 0 {
		fmt.Println("No SoundTouch devices found")
		return nil
	}

	fmt.Printf("Found %d SoundTouch device(s):\n", len(devices))
	for i, device := range devices {
		fmt.Printf("  %d. %s\n", i+1, device.Name)
		fmt.Printf("     Host: %s:%d\n", device.Host, device.Port)
		fmt.Printf("     Location: %s\n", device.Location)
		fmt.Printf("     Last seen: %s\n", device.LastSeen.Format("2006-01-02 15:04:05"))

		// Indicate source of discovery
		if strings.Contains(device.Location, "/info") && len(cfg.PreferredDevices) > 0 {
			for _, prefDevice := range cfg.PreferredDevices {
				if prefDevice.Host == device.Host && prefDevice.Port == device.Port {
					fmt.Printf("     Source: Configuration (.env)\n")
					break
				}
			}
		} else {
			fmt.Printf("     Source: UPnP Discovery\n")
		}

		if showInfo {
			fmt.Printf("     Getting device info...\n")
			if err := showDeviceInfoWithConfig(device.Host, device.Port, cfg); err != nil {
				fmt.Printf("     Error getting info: %v\n", err)
			}
		}
		fmt.Println()
	}

	return nil
}

func handleDeviceInfo(host string, port int, timeout time.Duration) error {
	// Load configuration for HTTP settings
	cfg, err := config.LoadFromEnv()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	// Override timeout if provided via command line
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	return showDeviceInfoWithConfig(host, port, cfg)
}

func showDeviceInfoWithConfig(host string, port int, cfg *config.Config) error {
	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Connecting to SoundTouch device at %s:%d...\n", host, port)

	// Test connectivity first
	if err := soundtouchClient.Ping(); err != nil {
		return fmt.Errorf("failed to connect to device: %w", err)
	}

	// Get device info
	deviceInfo, err := soundtouchClient.GetDeviceInfo()
	if err != nil {
		return fmt.Errorf("failed to get device info: %w", err)
	}

	// Display device information
	fmt.Printf("Device Information:\n")
	fmt.Printf("  Name: %s\n", deviceInfo.Name)
	fmt.Printf("  Device ID: %s\n", deviceInfo.DeviceID)
	fmt.Printf("  Type: %s\n", deviceInfo.Type)
	fmt.Printf("  Module Type: %s\n", deviceInfo.ModuleType)
	fmt.Printf("  Variant: %s (%s)\n", deviceInfo.Variant, deviceInfo.VariantMode)
	fmt.Printf("  Country: %s\n", deviceInfo.CountryCode)

	if deviceInfo.MargeAccountUUID != "" {
		fmt.Printf("  Marge Account UUID: %s\n", deviceInfo.MargeAccountUUID)
	}

	if deviceInfo.MargeURL != "" {
		fmt.Printf("  Marge URL: %s\n", deviceInfo.MargeURL)
	}

	if len(deviceInfo.NetworkInfo) > 0 {
		fmt.Printf("  Network Info:\n")
		for _, net := range deviceInfo.NetworkInfo {
			fmt.Printf("    - Type: %s\n", net.Type)
			fmt.Printf("      MAC Address: %s\n", net.MacAddress)
			fmt.Printf("      IP Address: %s\n", net.IPAddress)
		}
	}

	if len(deviceInfo.Components) > 0 {
		fmt.Printf("  Components:\n")
		for _, component := range deviceInfo.Components {
			fmt.Printf("    - Category: %s\n", component.ComponentCategory)
			if component.SoftwareVersion != "" {
				fmt.Printf("      Software Version: %s\n", component.SoftwareVersion)
			}
			if component.SerialNumber != "" {
				fmt.Printf("      Serial Number: %s\n", component.SerialNumber)
			}
		}
	}

	fmt.Printf("  Base URL: %s\n", soundtouchClient.BaseURL())

	return nil
}
