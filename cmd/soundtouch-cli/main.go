package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/config"
	"github.com/gesellix/bose-soundtouch/pkg/discovery"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// parseHostPort splits a host:port string into separate host and port components
// If no port is specified, returns the original host and the provided default port
func parseHostPort(hostPort string, defaultPort int) (string, int) {
	// Check if host contains a port (has a colon)
	if strings.Contains(hostPort, ":") {
		host, portStr, err := net.SplitHostPort(hostPort)
		if err != nil {
			// If parsing fails, return original host and default port
			return hostPort, defaultPort
		}

		port, err := strconv.Atoi(portStr)
		if err != nil || port < 1 || port > 65535 {
			// If port parsing fails or is invalid, return host and default port
			return host, defaultPort
		}

		return host, port
	}

	// No port specified, return original host and default port
	return hostPort, defaultPort
}

func main() {
	var (
		host         = flag.String("host", "", "SoundTouch device host/IP address (can include port like host:8090)")
		port         = flag.Int("port", 8090, "SoundTouch device port")
		timeout      = flag.Duration("timeout", 10*time.Second, "Request timeout")
		discover     = flag.Bool("discover", false, "Discover SoundTouch devices via UPnP")
		discoverAll  = flag.Bool("discover-all", false, "Discover all SoundTouch devices and show info")
		info         = flag.Bool("info", false, "Get device information")
		nowPlaying   = flag.Bool("nowplaying", false, "Get current playback status")
		sources      = flag.Bool("sources", false, "Get available audio sources")
		name         = flag.Bool("name", false, "Get device name")
		capabilities = flag.Bool("capabilities", false, "Get device capabilities")
		presets      = flag.Bool("presets", false, "Get configured presets")
		key          = flag.String("key", "", "Send key command (PLAY, PAUSE, STOP, PREV_TRACK, NEXT_TRACK, VOLUME_UP, VOLUME_DOWN, PRESET_1-6)")
		play         = flag.Bool("play", false, "Send PLAY key command")
		pause        = flag.Bool("pause", false, "Send PAUSE key command")
		stop         = flag.Bool("stop", false, "Send STOP key command")
		next         = flag.Bool("next", false, "Send NEXT_TRACK key command")
		prev         = flag.Bool("prev", false, "Send PREV_TRACK key command")
		volumeUp     = flag.Bool("volume-up", false, "Send VOLUME_UP key command")
		volumeDown   = flag.Bool("volume-down", false, "Send VOLUME_DOWN key command")
		preset       = flag.Int("preset", 0, "Select preset (1-6)")
		volume       = flag.Bool("volume", false, "Get current volume level")
		setVolume    = flag.Int("set-volume", -1, "Set volume level (0-100)")
		incVolume    = flag.Int("inc-volume", 0, "Increase volume by amount (1-10, default: 2)")
		decVolume    = flag.Int("dec-volume", 0, "Decrease volume by amount (1-10, default: 2)")
		help         = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// If no specific action is requested, show help
	if !*discover && !*discoverAll && !*info && !*nowPlaying && !*sources && !*name && !*capabilities && !*presets && *key == "" && !*play && !*pause && !*stop && !*next && !*prev && !*volumeUp && !*volumeDown && *preset == 0 && !*volume && *setVolume == -1 && *incVolume == 0 && *decVolume == 0 && *host == "" {
		printHelp()
		return
	}

	// Parse host:port if provided
	var finalHost string
	var finalPort int
	if *host != "" {
		finalHost, finalPort = parseHostPort(*host, *port)
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
		if err := handleDeviceInfo(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get device info: %v", err)
		}
		return
	}

	// Handle now playing
	if *nowPlaying {
		if *host == "" {
			log.Fatal("Host is required for nowplaying command. Use -host flag or -discover to find devices.")
		}
		if err := handleNowPlaying(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get now playing: %v", err)
		}
		return
	}

	// Handle sources
	if *sources {
		if *host == "" {
			log.Fatal("Host is required for sources command. Use -host flag or -discover to find devices.")
		}
		if err := handleSources(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get sources: %v", err)
		}
		return
	}

	// Handle name
	if *name {
		if *host == "" {
			log.Fatal("Host is required for name command. Use -host flag or -discover to find devices.")
		}
		if err := handleName(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get device name: %v", err)
		}
		return
	}

	// Handle capabilities
	if *capabilities {
		if *host == "" {
			log.Fatal("Host is required for capabilities command. Use -host flag or -discover to find devices.")
		}
		if err := handleCapabilities(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get device capabilities: %v", err)
		}
		return
	}

	// Handle presets
	if *presets {
		if *host == "" {
			log.Fatal("Host is required for presets command. Use -host flag or -discover to find devices.")
		}
		if err := handlePresets(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get presets: %v", err)
		}
		return
	}

	// Handle key commands
	if *key != "" || *play || *pause || *stop || *next || *prev || *volumeUp || *volumeDown || *preset > 0 {
		if *host == "" {
			log.Fatal("Host is required for key commands. Use -host flag or -discover to find devices.")
		}
		if err := handleKeyCommands(finalHost, finalPort, *timeout, *key, *play, *pause, *stop, *next, *prev, *volumeUp, *volumeDown, *preset); err != nil {
			log.Fatalf("Failed to send key command: %v", err)
		}
		return
	}

	// Handle volume commands
	if *volume || *setVolume != -1 || *incVolume > 0 || *decVolume > 0 {
		if *host == "" {
			log.Fatal("Host is required for volume commands. Use -host flag or -discover to find devices.")
		}
		if err := handleVolumeCommands(finalHost, finalPort, *timeout, *volume, *setVolume, *incVolume, *decVolume); err != nil {
			log.Fatalf("Failed to execute volume command: %v", err)
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
	fmt.Println("  -host <ip>        SoundTouch device IP address (or host:port)")
	fmt.Println("  -port <port>      SoundTouch device port (default: 8090)")
	fmt.Println("  -timeout <dur>    Request timeout (default: 10s)")
	fmt.Println("  -discover         Discover SoundTouch devices via UPnP")
	fmt.Println("  -discover-all     Discover devices and show detailed info")
	fmt.Println("  -info             Get device information (requires -host)")
	fmt.Println("  -nowplaying       Get current playback status (requires -host)")
	fmt.Println("  -sources          Get available audio sources (requires -host)")
	fmt.Println("  -name             Get device name (requires -host)")
	fmt.Println("  -capabilities     Get device capabilities (requires -host)")
	fmt.Println("  -presets          Get configured presets (requires -host)")
	fmt.Println("  -key <key>        Send key command (requires -host)")
	fmt.Println("  -play             Send PLAY key command (requires -host)")
	fmt.Println("  -pause            Send PAUSE key command (requires -host)")
	fmt.Println("  -stop             Send STOP key command (requires -host)")
	fmt.Println("  -next             Send NEXT_TRACK key command (requires -host)")
	fmt.Println("  -prev             Send PREV_TRACK key command (requires -host)")
	fmt.Println("  -volume-up        Send VOLUME_UP key command (requires -host)")
	fmt.Println("  -volume-down      Send VOLUME_DOWN key command (requires -host)")
	fmt.Println("  -preset <1-6>     Select preset (requires -host)")
	fmt.Println("  -volume           Get current volume level (requires -host)")
	fmt.Println("  -set-volume <0-100> Set volume level (requires -host)")
	fmt.Println("  -inc-volume <1-10>  Increase volume by amount (requires -host, default: 2)")
	fmt.Println("  -dec-volume <1-10>  Decrease volume by amount (requires -host, default: 2)")
	fmt.Println("  -help             Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  soundtouch-cli -discover")
	fmt.Println("  soundtouch-cli -discover-all")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -info")
	fmt.Println("  soundtouch-cli -host 192.168.1.100:8090 -info")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -nowplaying")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -sources")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -name")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -capabilities")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -presets")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -play")
	fmt.Println("  soundtouch-cli -host 192.168.1.100:8090 -pause")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -volume-up")
	fmt.Println("  soundtouch-cli -host 192.168.1.100:8090 -preset 1")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -key STOP")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -volume")
	fmt.Println("  soundtouch-cli -host 192.168.1.100:8090 -set-volume 25")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -inc-volume 2")
	fmt.Println("  soundtouch-cli -host 192.168.1.100 -dec-volume 3")
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

	discoveryService := discovery.NewUnifiedDiscoveryService(cfg)
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
			fmt.Printf("     Source: Network Discovery (UPnP/mDNS)\n")
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

func handleNowPlaying(host string, port int, timeout time.Duration) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Getting current playback status from %s:%d...\n", host, port)

	// Get now playing info
	nowPlaying, err := soundtouchClient.GetNowPlaying()
	if err != nil {
		return fmt.Errorf("failed to get now playing: %w", err)
	}

	// Display playback information
	fmt.Printf("Now Playing:\n")
	fmt.Printf("  Device ID: %s\n", nowPlaying.DeviceID)
	fmt.Printf("  Source: %s\n", nowPlaying.Source)
	fmt.Printf("  Status: %s\n", nowPlaying.PlayStatus.String())

	if nowPlaying.IsEmpty() {
		fmt.Printf("  No content currently playing\n")
	} else {
		// Track information
		title := nowPlaying.GetDisplayTitle()
		artist := nowPlaying.GetDisplayArtist()

		fmt.Printf("  Title: %s\n", title)
		if artist != "" {
			fmt.Printf("  Artist: %s\n", artist)
		}
		if nowPlaying.Album != "" {
			fmt.Printf("  Album: %s\n", nowPlaying.Album)
		}

		// Radio/streaming info
		if nowPlaying.IsRadio() && nowPlaying.StationName != "" {
			fmt.Printf("  Station: %s\n", nowPlaying.StationName)
		}

		// Duration/Position info
		if nowPlaying.HasTimeInfo() {
			if duration := nowPlaying.FormatDuration(); duration != "" {
				fmt.Printf("  Duration: %s\n", duration)
			} else if position := nowPlaying.FormatPosition(); position != "" {
				fmt.Printf("  Position: %s\n", position)
			}
		}

		// Playback settings
		if nowPlaying.ShuffleSetting != "" {
			fmt.Printf("  Shuffle: %s\n", nowPlaying.ShuffleSetting.String())
		}
		if nowPlaying.RepeatSetting != "" {
			fmt.Printf("  Repeat: %s\n", nowPlaying.RepeatSetting.String())
		}

		// Artwork
		if artURL := nowPlaying.GetArtworkURL(); artURL != "" {
			fmt.Printf("  Artwork: %s\n", artURL)
		}

		// Additional metadata
		if nowPlaying.Description != "" {
			fmt.Printf("  Description: %s\n", nowPlaying.Description)
		}
		if nowPlaying.StationLocation != "" {
			fmt.Printf("  Station Location: %s\n", nowPlaying.StationLocation)
		}

		// Capabilities
		var capabilities []string
		if nowPlaying.CanSkip() {
			capabilities = append(capabilities, "Skip")
		}
		if nowPlaying.CanSkipPrevious() {
			capabilities = append(capabilities, "Skip Previous")
		}
		if nowPlaying.IsSeekSupported() {
			capabilities = append(capabilities, "Seek")
		}
		if nowPlaying.CanFavorite() {
			capabilities = append(capabilities, "Favorite")
		}
		if len(capabilities) > 0 {
			fmt.Printf("  Capabilities: %s\n", strings.Join(capabilities, ", "))
		}
	}

	return nil
}

func handleSources(host string, port int, timeout time.Duration) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Getting available audio sources from %s:%d...\n", host, port)

	// Get sources info
	sources, err := soundtouchClient.GetSources()
	if err != nil {
		return fmt.Errorf("failed to get sources: %w", err)
	}

	// Display sources information
	fmt.Printf("Audio Sources:\n")
	fmt.Printf("  Device ID: %s\n", sources.DeviceID)
	fmt.Printf("  Total Sources: %d\n", sources.GetSourceCount())
	fmt.Printf("  Ready Sources: %d\n", sources.GetReadySourceCount())
	fmt.Println()

	// Display available sources
	availableSources := sources.GetAvailableSources()
	if len(availableSources) > 0 {
		fmt.Printf("Ready Sources:\n")
		for _, source := range availableSources {
			fmt.Printf("  • %s", source.GetDisplayName())
			if source.SourceAccount != "" && source.SourceAccount != source.Source {
				fmt.Printf(" (%s)", source.SourceAccount)
			}

			var attributes []string
			if source.IsLocalSource() {
				attributes = append(attributes, "Local")
			} else {
				attributes = append(attributes, "Remote")
			}
			if source.SupportsMultiroom() {
				attributes = append(attributes, "Multiroom")
			}
			if source.IsStreamingService() {
				attributes = append(attributes, "Streaming")
			}

			if len(attributes) > 0 {
				fmt.Printf(" [%s]", strings.Join(attributes, ", "))
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// Display unavailable sources
	var unavailableSources []models.SourceItem
	for _, source := range sources.SourceItem {
		if source.Status.IsUnavailable() {
			unavailableSources = append(unavailableSources, source)
		}
	}

	if len(unavailableSources) > 0 {
		fmt.Printf("Unavailable Sources:\n")
		for _, source := range unavailableSources {
			fmt.Printf("  • %s", source.GetDisplayName())
			if source.SourceAccount != "" && source.SourceAccount != source.Source {
				fmt.Printf(" (%s)", source.SourceAccount)
			}
			fmt.Printf(" [%s]", source.Status.String())
			fmt.Println()
		}
		fmt.Println()
	}

	// Summary by category
	fmt.Printf("Categories:\n")
	if sources.HasSpotify() {
		spotifySources := sources.GetReadySpotifySources()
		fmt.Printf("  Spotify: %d account(s) ready\n", len(spotifySources))
	}
	if sources.HasBluetooth() {
		fmt.Printf("  Bluetooth: Ready\n")
	}
	if sources.HasAux() {
		fmt.Printf("  AUX Input: Ready\n")
	}

	streamingSources := sources.GetStreamingSources()
	readyStreaming := 0
	for _, source := range streamingSources {
		if source.Status.IsReady() {
			readyStreaming++
		}
	}
	if readyStreaming > 0 {
		fmt.Printf("  Streaming Services: %d ready\n", readyStreaming)
	}

	return nil
}

func handleName(host string, port int, timeout time.Duration) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Getting device name from %s:%d...\n", host, port)

	// Get device name
	name, err := soundtouchClient.GetName()
	if err != nil {
		return fmt.Errorf("failed to get device name: %w", err)
	}

	// Display name information
	fmt.Printf("Device Name: %s\n", name.GetName())

	if name.IsEmpty() {
		fmt.Printf("Warning: Device name is empty\n")
	}

	return nil
}

func handleCapabilities(host string, port int, timeout time.Duration) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Getting device capabilities from %s:%d...\n", host, port)

	// Get device capabilities
	capabilities, err := soundtouchClient.GetCapabilities()
	if err != nil {
		return fmt.Errorf("failed to get device capabilities: %w", err)
	}

	// Display capabilities information
	fmt.Printf("Device Capabilities:\n")
	fmt.Printf("  Device ID: %s\n", capabilities.DeviceID)
	fmt.Println()

	// System capabilities
	systemCaps := capabilities.GetSystemCapabilities()
	if len(systemCaps) > 0 {
		fmt.Printf("System Features:\n")
		for _, cap := range systemCaps {
			fmt.Printf("  • %s\n", cap)
		}
		fmt.Println()
	}

	// Audio capabilities
	audioCaps := capabilities.GetAudioCapabilities()
	if len(audioCaps) > 0 {
		fmt.Printf("Audio Features:\n")
		for _, cap := range audioCaps {
			fmt.Printf("  • %s\n", cap)
		}
		fmt.Println()
	}

	// Network capabilities
	networkCaps := capabilities.GetNetworkCapabilities()
	if len(networkCaps) > 0 {
		fmt.Printf("Network Features:\n")
		for _, cap := range networkCaps {
			fmt.Printf("  • %s\n", cap)
		}

		// Show hosted wifi details if available
		if capabilities.HasHostedWifiConfig() {
			fmt.Printf("  Hosted WiFi Config:\n")
			fmt.Printf("    • Port: %s\n", capabilities.GetHostedWifiPort())
			fmt.Printf("    • Hosted by: %s\n", capabilities.GetHostedWifiHostedBy())
		}
		fmt.Println()
	}

	// Extended capabilities
	capNames := capabilities.GetCapabilityNames()
	if len(capNames) > 0 {
		fmt.Printf("Extended Capabilities:\n")
		for _, capName := range capNames {
			cap := capabilities.GetCapabilityByName(capName)
			fmt.Printf("  • %s", capName)
			if cap.URL != "" {
				fmt.Printf(" (%s)", cap.URL)
			}
			fmt.Println()
		}
	}

	return nil
}

func handlePresets(host string, port int, timeout time.Duration) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Getting configured presets from %s:%d...\n", host, port)

	// Get presets
	presets, err := soundtouchClient.GetPresets()
	if err != nil {
		return fmt.Errorf("failed to get presets: %w", err)
	}

	// Display presets information
	fmt.Printf("Configured Presets:\n")

	if !presets.HasPresets() {
		fmt.Printf("  No presets configured\n")
		return nil
	}

	summary := presets.GetPresetsSummary()
	fmt.Printf("  Used Slots: %d/6\n", summary["used"])
	fmt.Printf("  Spotify Presets: %d\n", summary["spotify"])
	fmt.Println()

	// Show each configured preset
	for _, preset := range presets.Preset {
		if preset.IsEmpty() {
			continue
		}

		fmt.Printf("Preset %d: %s\n", preset.ID, preset.GetDisplayName())
		fmt.Printf("  Source: %s", preset.GetSource())
		if preset.GetSourceAccount() != "" {
			fmt.Printf(" (%s)", preset.GetSourceAccount())
		}
		fmt.Println()

		if preset.GetContentType() != "" {
			fmt.Printf("  Type: %s\n", preset.GetContentType())
		}

		if preset.HasTimestamps() {
			if !preset.GetCreatedTime().IsZero() {
				fmt.Printf("  Created: %s\n", preset.GetCreatedTime().Format("2006-01-02 15:04:05"))
			}
			if !preset.GetUpdatedTime().IsZero() {
				fmt.Printf("  Updated: %s\n", preset.GetUpdatedTime().Format("2006-01-02 15:04:05"))
			}
		}

		if preset.GetArtworkURL() != "" {
			fmt.Printf("  Artwork: %s\n", preset.GetArtworkURL())
		}

		fmt.Println()
	}

	// Show empty slots
	emptySlots := presets.GetEmptyPresetSlots()
	if len(emptySlots) > 0 {
		fmt.Printf("Available Slots: %v\n", emptySlots)
	}

	// Show most recent preset
	if recent := presets.GetMostRecentPreset(); recent != nil {
		fmt.Printf("Most Recent: Preset %d (%s)\n", recent.ID, recent.GetDisplayName())
	}

	return nil
}

func handleKeyCommands(host string, port int, timeout time.Duration, key string, play, pause, stop, next, prev, volumeUp, volumeDown bool, preset int) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	// Count how many commands are requested
	commandCount := 0
	var commandName string

	if key != "" {
		commandCount++
		commandName = fmt.Sprintf("key %s", key)
	}
	if play {
		commandCount++
		commandName = "PLAY"
	}
	if pause {
		commandCount++
		commandName = "PAUSE"
	}
	if stop {
		commandCount++
		commandName = "STOP"
	}
	if next {
		commandCount++
		commandName = "NEXT_TRACK"
	}
	if prev {
		commandCount++
		commandName = "PREV_TRACK"
	}
	if volumeUp {
		commandCount++
		commandName = "VOLUME_UP"
	}
	if volumeDown {
		commandCount++
		commandName = "VOLUME_DOWN"
	}
	if preset > 0 {
		commandCount++
		commandName = fmt.Sprintf("PRESET_%d", preset)
	}

	// Only allow one command at a time
	if commandCount > 1 {
		return fmt.Errorf("only one key command can be sent at a time")
	}
	if commandCount == 0 {
		return fmt.Errorf("no key command specified")
	}

	fmt.Printf("Sending %s command to %s:%d...\n", commandName, host, port)

	// Execute the appropriate command
	if key != "" {
		err = soundtouchClient.SendKey(strings.ToUpper(key))
	} else if play {
		err = soundtouchClient.Play()
	} else if pause {
		err = soundtouchClient.Pause()
	} else if stop {
		err = soundtouchClient.Stop()
	} else if next {
		err = soundtouchClient.NextTrack()
	} else if prev {
		err = soundtouchClient.PrevTrack()
	} else if volumeUp {
		err = soundtouchClient.VolumeUp()
	} else if volumeDown {
		err = soundtouchClient.VolumeDown()
	} else if preset > 0 {
		err = soundtouchClient.SelectPreset(preset)
	}

	if err != nil {
		return fmt.Errorf("failed to send key command: %w", err)
	}

	fmt.Printf("✓ %s command sent successfully\n", commandName)
	return nil
}

func handleVolumeCommands(host string, port int, timeout time.Duration, getVolume bool, setVolume, incVolume, decVolume int) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	// Handle get volume
	if getVolume {
		fmt.Printf("Getting current volume from %s:%d...\n", host, port)
		volume, err := soundtouchClient.GetVolume()
		if err != nil {
			return fmt.Errorf("failed to get volume: %w", err)
		}

		fmt.Printf("Current Volume:\n")
		fmt.Printf("  Device ID: %s\n", volume.DeviceID)
		fmt.Printf("  Current Level: %d (%s)\n", volume.GetLevel(), models.GetVolumeLevelName(volume.GetLevel()))
		fmt.Printf("  Target Level: %d\n", volume.GetTargetLevel())
		fmt.Printf("  Muted: %v\n", volume.IsMuted())
		if !volume.IsVolumeSync() {
			fmt.Printf("  Note: Volume is adjusting (target: %d, actual: %d)\n", volume.GetTargetLevel(), volume.GetLevel())
		}
		return nil
	}

	// Handle set volume
	if setVolume != -1 {
		if setVolume > 30 {
			fmt.Printf("⚠️  Warning: Setting volume to %d (this is quite loud!)\n", setVolume)
			fmt.Printf("Proceeding in 2 seconds... Press Ctrl+C to cancel\n")
			time.Sleep(2 * time.Second)
		}

		fmt.Printf("Setting volume to %d on %s:%d...\n", setVolume, host, port)
		err := soundtouchClient.SetVolume(setVolume)
		if err != nil {
			return fmt.Errorf("failed to set volume: %w", err)
		}

		// Get updated volume
		volume, err := soundtouchClient.GetVolume()
		if err != nil {
			fmt.Printf("✓ Volume set successfully\n")
		} else {
			fmt.Printf("✓ Volume set to %d (%s)\n", volume.GetLevel(), models.GetVolumeLevelName(volume.GetLevel()))
		}
		return nil
	}

	// Handle volume increase (with safety limits)
	if incVolume > 0 {
		if incVolume > 10 {
			incVolume = 10 // Safety limit
		}
		if incVolume == 0 {
			incVolume = 2 // Default increment
		}

		fmt.Printf("Increasing volume by %d on %s:%d...\n", incVolume, host, port)
		volume, err := soundtouchClient.IncreaseVolume(incVolume)
		if err != nil {
			return fmt.Errorf("failed to increase volume: %w", err)
		}

		fmt.Printf("✓ Volume increased to %d (%s)\n", volume.GetLevel(), models.GetVolumeLevelName(volume.GetLevel()))
		return nil
	}

	// Handle volume decrease
	if decVolume > 0 {
		if decVolume > 20 {
			decVolume = 20 // Safety limit for decrease
		}
		if decVolume == 0 {
			decVolume = 2 // Default decrement
		}

		fmt.Printf("Decreasing volume by %d on %s:%d...\n", decVolume, host, port)
		volume, err := soundtouchClient.DecreaseVolume(decVolume)
		if err != nil {
			return fmt.Errorf("failed to decrease volume: %w", err)
		}

		fmt.Printf("✓ Volume decreased to %d (%s)\n", volume.GetLevel(), models.GetVolumeLevelName(volume.GetLevel()))
		return nil
	}

	return fmt.Errorf("no volume command specified")
}
