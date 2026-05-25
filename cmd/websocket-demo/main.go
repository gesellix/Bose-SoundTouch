// Package main provides a demonstration of WebSocket event handling for Bose SoundTouch devices.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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

func parseFilters(eventFilter string) map[string]bool {
	validFilters := map[string]bool{
		"nowPlaying": true, "volume": true, "connection": true,
		"preset": true, "zone": true, "bass": true,
		"sdkInfo": true, "userActivity": true,
	}

	if eventFilter == "" {
		return nil
	}

	filters := make(map[string]bool)
	filterList := strings.Split(eventFilter, ",")

	for _, f := range filterList {
		f = strings.TrimSpace(f)
		if !validFilters[f] {
			fmt.Printf("Invalid filter '%s'. Valid filters: nowPlaying, volume, connection, preset, zone, bass, sdkInfo, userActivity\n", f)
			os.Exit(1)
		}

		filters[f] = true
	}

	return filters
}

func discoverDevice(discoverFlag bool, hostPort string, defaultPort int) (string, int, error) {
	if hostPort != "" && !discoverFlag {
		deviceHost, devicePort := parseHostPort(hostPort, defaultPort)
		fmt.Printf("Connecting to: %s:%d\n", deviceHost, devicePort)

		return deviceHost, devicePort, nil
	}

	fmt.Println("Discovering SoundTouch devices...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cfg := &config.Config{
		DiscoveryTimeout: 10 * time.Second,
		CacheEnabled:     false,
	}
	discoveryService := discovery.NewUnifiedDiscoveryService(cfg)

	devices, err := discoveryService.DiscoverDevices(ctx)
	if err != nil || len(devices) == 0 {
		if err != nil {
			return "", 0, fmt.Errorf("discovery failed: %w", err)
		}

		return "", 0, fmt.Errorf("no SoundTouch devices found")
	}

	device := devices[0]
	fmt.Printf("Found %d device(s), connecting to: %s (%s:%d)\n",
		len(devices), device.Name, device.Host, device.Port)

	return device.Host, device.Port, nil
}

func setupWebSocket(soundTouchClient *client.Client, reconnect, verbose bool) *client.WebSocketClient {
	wsConfig := &client.WebSocketConfig{
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 0, // Unlimited if reconnect enabled
		PingInterval:         30 * time.Second,
		PongTimeout:          10 * time.Second,
		ReadBufferSize:       2048,
		WriteBufferSize:      2048,
	}

	if verbose {
		wsConfig.Logger = &VerboseLogger{}
	} else {
		// Use a silent logger when not verbose
		wsConfig.Logger = &SilentLogger{}
	}

	if !reconnect {
		wsConfig.MaxReconnectAttempts = 1
	}

	return soundTouchClient.NewWebSocketClient(wsConfig)
}

func main() {
	var (
		host = flag.String("host", "", "SoundTouch device host/IP address (can include port like host:8090)")
		port = flag.Int("port", 8090, "SoundTouch device port")

		discover    = flag.Bool("discover", false, "Discover SoundTouch devices and connect to first found")
		duration    = flag.Duration("duration", 0, "How long to listen for events (0 = infinite)")
		reconnect   = flag.Bool("reconnect", true, "Enable automatic reconnection")
		verbose     = flag.Bool("verbose", false, "Enable verbose logging")
		eventFilter = flag.String("filter", "", "Filter events by type (nowPlaying,volume,connection,preset,zone,bass)")
		help        = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// Validate filter if provided
	filters := parseFilters(*eventFilter)

	// Discover devices if no host specified or discover flag used
	deviceHost, devicePort, err := discoverDevice(*discover, *host, *port)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Create client
	clientConfig := &client.Config{
		Host:    deviceHost,
		Port:    devicePort,
		Timeout: 10 * time.Second,
	}

	soundTouchClient := client.NewClient(clientConfig)

	// Test basic connectivity
	fmt.Println("Testing device connectivity...")

	deviceInfo, err := soundTouchClient.GetDeviceInfo()
	if err != nil {
		fmt.Printf("Failed to connect to device: %v\n", err)
		return
	}

	macAddress := ""
	if len(deviceInfo.NetworkInfo) > 0 {
		macAddress = deviceInfo.NetworkInfo[0].MacAddress
	}

	fmt.Printf("Connected to: %s (Type: %s, MAC: %s)\n",
		deviceInfo.Name, deviceInfo.Type, macAddress)

	// Create WebSocket client
	wsClient := setupWebSocket(soundTouchClient, *reconnect, *verbose)

	// Set up event handlers
	setupEventHandlers(wsClient, filters, *verbose)

	// Set up special message handler
	wsClient.OnSpecialMessage(func(message *models.SpecialMessage) {
		handleSpecialMessage(message, filters, *verbose)
	})

	// Connect to WebSocket
	fmt.Println("Connecting to WebSocket...")

	err = wsClient.Connect()
	if err != nil {
		fmt.Printf("Failed to connect to WebSocket: %v\n", err)

		return
	}

	fmt.Println("Connected! Listening for events...")

	if len(filters) > 0 {
		fmt.Printf("Filtering events: %v\n", getFilterKeys(filters))
	}

	if *duration > 0 {
		fmt.Printf("Will listen for %v\n", *duration)
	} else {
		fmt.Println("Press Ctrl+C to stop")
	}

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle duration limit
	if *duration > 0 {
		go func() {
			select {
			case <-time.After(*duration):
				fmt.Println("\nDuration limit reached, shutting down...")
				cancel()
			case <-ctx.Done():
				return
			}
		}()
	}

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		select {
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
			cancel()
		case <-ctx.Done():
			return
		}
	}()

	// Wait for shutdown
	<-ctx.Done()

	// Disconnect WebSocket
	fmt.Println("Disconnecting...")

	if err := wsClient.Disconnect(); err != nil {
		fmt.Printf("Error during disconnect: %v\n", err)
	}

	fmt.Println("Disconnected successfully")
}

func handleNowPlaying(event *models.NowPlayingUpdatedEvent, verbose bool) {
	fmt.Printf("\n🎵 Now Playing Update [%s]:\n", event.DeviceID)
	np := &event.NowPlaying

	if np.IsEmpty() {
		fmt.Println("  ⏹️  Nothing playing")
		return
	}

	fmt.Printf("  🎵 %s\n", np.GetDisplayTitle())

	if artist := np.GetDisplayArtist(); artist != "" {
		fmt.Printf("  👤 %s\n", artist)
	}

	if np.Album != "" {
		fmt.Printf("  💿 %s\n", np.Album)
	}

	fmt.Printf("  📻 Source: %s\n", np.Source)
	fmt.Printf("  ▶️  Status: %s\n", np.PlayStatus.String())

	if np.HasTimeInfo() {
		fmt.Printf("  ⏱️  Duration: %s\n", np.FormatDuration())
	}

	if np.ShuffleSetting != "" {
		fmt.Printf("  🔀 Shuffle: %s\n", np.ShuffleSetting.String())
	}

	if np.RepeatSetting != "" {
		fmt.Printf("  🔁 Repeat: %s\n", np.RepeatSetting.String())
	}

	if verbose {
		fmt.Printf("  📱 Raw Source: %s, Account: %s\n", np.Source, np.SourceAccount)

		if np.Art != nil && np.Art.URL != "" {
			fmt.Printf("  🖼️  Artwork: %s\n", np.Art.URL)
		}
	}
}

func handleVolume(event *models.VolumeUpdatedEvent, verbose bool) {
	vol := &event.Volume
	fmt.Printf("\n🔊 Volume Update [%s]:\n", event.DeviceID)

	if vol.IsMuted() {
		fmt.Println("  🔇 Muted")
	} else {
		fmt.Printf("  🔊 Level: %d\n", vol.ActualVolume)

		if vol.TargetVolume != vol.ActualVolume {
			fmt.Printf("  🎯 Target: %d\n", vol.TargetVolume)
		}

		fmt.Printf("  📊 %s\n", models.GetVolumeLevelName(vol.ActualVolume))
	}

	if verbose {
		fmt.Printf("  📱 Sync: %v\n", vol.IsVolumeSync())
	}
}

func handleConnection(event *models.ConnectionStateUpdatedEvent) {
	cs := &event.ConnectionState
	fmt.Printf("\n🌐 Connection Update [%s]:\n", event.DeviceID)

	if cs.IsConnected() {
		fmt.Println("  ✅ Connected")
	} else {
		fmt.Printf("  ❌ State: %s\n", cs.State)
	}

	if cs.Signal != "" {
		fmt.Printf("  📶 Signal: %s\n", cs.GetSignalStrength())
	}
}

func handlePreset(event *models.PresetUpdatedEvent, verbose bool) {
	presets := &event.Presets

	deviceHeader := "\n📻 Presets Update"
	if event.DeviceID != "" {
		deviceHeader += fmt.Sprintf(" [%s]", event.DeviceID)
	}

	fmt.Printf("%s:\n", deviceHeader)
	fmt.Printf("  📻 Total presets: %d\n", len(presets.Preset))

	for _, preset := range presets.Preset {
		fmt.Printf("  📻 Preset %d:", preset.ID)

		// IsEmpty catches both <preset/> and INVALID_SOURCE
		// placeholders; using the nil-safe helpers below means the
		// inner Printf never dereferences a nil ContentItem.
		if !preset.IsEmpty() {
			fmt.Printf(" %s (%s)", preset.GetDisplayName(), preset.GetSource())
		}

		fmt.Println()
	}

	if verbose {
		fmt.Printf("  📱 Raw presets data: %d total presets\n", len(presets.Preset))
	}
}

func handleZone(event *models.ZoneUpdatedEvent) {
	zone := &event.Zone
	fmt.Printf("\n🏠 Zone Update [%s]:\n", event.DeviceID)
	fmt.Printf("  👑 Master: %s\n", zone.Master)

	if len(zone.Members) > 0 {
		fmt.Printf("  👥 Members (%d):\n", len(zone.Members))

		for i, member := range zone.Members {
			fmt.Printf("    %d. %s (%s)\n", i+1, member.DeviceID, member.IP)
		}
	} else {
		fmt.Println("  👤 Single device (no zone)")
	}
}

func handleBass(event *models.BassUpdatedEvent) {
	bass := &event.Bass
	fmt.Printf("\n🎵 Bass Update [%s]:\n", event.DeviceID)
	fmt.Printf("  🎚️  Level: %d\n", bass.ActualBass)

	if bass.TargetBass != bass.ActualBass {
		fmt.Printf("  🎯 Target: %d\n", bass.TargetBass)
	}

	levelDesc := "Neutral"
	if bass.ActualBass > 0 {
		levelDesc = "Boosted"
	} else if bass.ActualBass < 0 {
		levelDesc = "Reduced"
	}

	fmt.Printf("  📊 %s\n", levelDesc)
}

func handleSpecialMessage(message *models.SpecialMessage, filters map[string]bool, verbose bool) {
	// Check if we should filter this message type
	if filters != nil {
		switch message.Type {
		case models.MessageTypeSdkInfo:
			if !filters["sdkInfo"] {
				return
			}
		case models.MessageTypeUserActivity:
			if !filters["userActivity"] {
				return
			}
		}
	}

	switch message.Type {
	case models.MessageTypeSdkInfo:
		if sdkInfo := message.GetSdkInfo(); sdkInfo != nil {
			fmt.Printf("\n📡 SDK Info:\n")
			fmt.Printf("  📋 Server Version: %s\n", sdkInfo.ServerVersion)
			fmt.Printf("  🔧 Server Build: %s\n", sdkInfo.ServerBuild)
		}
	case models.MessageTypeUserActivity:
		fmt.Printf("\n👤 User Activity [%s]\n", message.DeviceID)

		if verbose {
			fmt.Printf("  ⏰ Timestamp: %s\n", message.Timestamp.Format("15:04:05"))
		}
	default:
		fmt.Printf("\n❓ Unknown Special Message: %s\n", message.String())

		if verbose {
			fmt.Printf("  📱 Raw data: %s\n", string(message.RawData))
		}
	}
}

func setupEventHandlers(wsClient *client.WebSocketClient, filters map[string]bool, verbose bool) {
	// Now Playing events
	if filters == nil || filters["nowPlaying"] {
		wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
			handleNowPlaying(event, verbose)
		})
	}

	// Volume events
	if filters == nil || filters["volume"] {
		wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
			handleVolume(event, verbose)
		})
	}

	// Connection state events
	if filters == nil || filters["connection"] {
		wsClient.OnConnectionState(func(event *models.ConnectionStateUpdatedEvent) {
			handleConnection(event)
		})
	}

	// Preset events
	if filters == nil || filters["preset"] {
		wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
			handlePreset(event, verbose)
		})
	}

	// Zone/Multiroom events
	if filters == nil || filters["zone"] {
		wsClient.OnZoneUpdated(func(event *models.ZoneUpdatedEvent) {
			handleZone(event)
		})
	}

	// Bass events
	if filters == nil || filters["bass"] {
		wsClient.OnBassUpdated(func(event *models.BassUpdatedEvent) {
			handleBass(event)
		})
	}

	// Unknown events (always enabled for debugging)
	wsClient.OnUnknownEvent(func(event *models.WebSocketEvent) {
		fmt.Printf("\n❓ Unknown Event [%s]:\n", event.DeviceID)
		types := event.GetEventTypes()

		for _, eventType := range types {
			fmt.Printf("  📝 Type: %s\n", eventType)
		}

		if verbose {
			events := event.GetEvents()
			fmt.Printf("  📱 Event count: %d\n", len(events))
			fmt.Printf("  ⏰ Timestamp: %s\n", event.Timestamp.Format(time.RFC3339))
		}
	})
}

func getFilterKeys(filters map[string]bool) []string {
	var keys []string
	for k := range filters {
		keys = append(keys, k)
	}

	return keys
}

func printHelp() {
	fmt.Println("SoundTouch WebSocket Event Monitor")
	fmt.Println("==================================")
	fmt.Println()
	fmt.Println("This tool connects to a Bose SoundTouch device via WebSocket to monitor real-time events.")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Printf("  %s [options]\n", os.Args[0])
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  -host string")
	fmt.Println("        SoundTouch device host/IP address (can include port like host:8090)")
	fmt.Println("  -port int")
	fmt.Println("        SoundTouch device port (default: 8090)")
	fmt.Println("  -timeout duration")
	fmt.Println("        Request timeout (default: 10s)")
	fmt.Println("  -discover")
	fmt.Println("        Discover SoundTouch devices and connect to first found")
	fmt.Println("  -duration duration")
	fmt.Println("        How long to listen for events (0 = infinite)")
	fmt.Println("  -reconnect")
	fmt.Println("        Enable automatic reconnection (default: true)")
	fmt.Println("  -verbose")
	fmt.Println("        Enable verbose logging")
	fmt.Println("  -filter string")
	fmt.Println("        Filter events by type (comma-separated):")
	fmt.Println("        nowPlaying, volume, connection, preset, zone, bass, sdkInfo, userActivity")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  # Auto-discover and monitor all events")
	fmt.Printf("  %s -discover\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Connect to specific device and monitor volume events only")
	fmt.Printf("  %s -host 192.0.2.10 -filter volume\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Monitor for 5 minutes with verbose output")
	fmt.Printf("  %s -host 192.0.2.10 -duration 5m -verbose\n", os.Args[0])
	fmt.Println()
	fmt.Println("  # Monitor now playing and volume events")
	fmt.Printf("  %s -host 192.0.2.10 -filter nowPlaying,volume\n", os.Args[0])
	fmt.Println()
	fmt.Println("Event Types:")
	fmt.Println("  🎵 nowPlaying  - Track changes, playback status")
	fmt.Println("  🔊 volume      - Volume and mute changes")
	fmt.Println("  🌐 connection  - Network connectivity status")
	fmt.Println("  📻 preset      - Preset configuration changes")
	fmt.Println("  🏠 zone        - Multiroom zone changes")
	fmt.Println("  🎚️  bass        - Bass level changes")
	fmt.Println("  📡 sdkInfo     - SDK version information")
	fmt.Println("  👤 userActivity - User interaction notifications")
	fmt.Println()
	fmt.Println("The tool will automatically reconnect if the connection is lost.")
	fmt.Println("Press Ctrl+C to stop monitoring.")
}

// VerboseLogger provides detailed WebSocket logging
type VerboseLogger struct{}

func (v *VerboseLogger) Printf(format string, args ...interface{}) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] [WebSocket] %s\n", timestamp, sanitizeLog(fmt.Sprintf(format, args...)))
}

// SilentLogger provides no-op WebSocket logging
type SilentLogger struct{}

func (s *SilentLogger) Printf(_ string, _ ...interface{}) {
	// Do nothing - silent logging
}
