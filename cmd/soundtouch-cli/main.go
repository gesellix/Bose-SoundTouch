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
		host           = flag.String("host", "", "SoundTouch device host/IP address (can include port like host:8090)")
		port           = flag.Int("port", 8090, "SoundTouch device port")
		timeout        = flag.Duration("timeout", 10*time.Second, "Request timeout")
		discover       = flag.Bool("discover", false, "Discover SoundTouch devices via UPnP")
		discoverAll    = flag.Bool("discover-all", false, "Discover all SoundTouch devices and show info")
		info           = flag.Bool("info", false, "Get device information")
		nowPlaying     = flag.Bool("nowplaying", false, "Get current playback status")
		sources        = flag.Bool("sources", false, "Get available audio sources")
		name           = flag.Bool("name", false, "Get device name")
		capabilities   = flag.Bool("capabilities", false, "Get device capabilities")
		presets        = flag.Bool("presets", false, "Get configured presets (requires -host)")
		key            = flag.String("key", "", "Send key command (PLAY, PAUSE, STOP, PREV_TRACK, NEXT_TRACK, THUMBS_UP, THUMBS_DOWN, BOOKMARK, POWER, MUTE, VOLUME_UP, VOLUME_DOWN, PRESET_1-6, AUX_INPUT, SHUFFLE_OFF, SHUFFLE_ON, REPEAT_OFF, REPEAT_ONE, REPEAT_ALL)")
		play           = flag.Bool("play", false, "Send PLAY key command")
		pause          = flag.Bool("pause", false, "Send PAUSE key command")
		stop           = flag.Bool("stop", false, "Send STOP key command")
		next           = flag.Bool("next", false, "Send NEXT_TRACK key command")
		prev           = flag.Bool("prev", false, "Send PREV_TRACK key command")
		volumeUp       = flag.Bool("volume-up", false, "Send VOLUME_UP key command")
		volumeDown     = flag.Bool("volume-down", false, "Send VOLUME_DOWN key command")
		power          = flag.Bool("power", false, "Send POWER key command")
		mute           = flag.Bool("mute", false, "Send MUTE key command")
		thumbsUp       = flag.Bool("thumbs-up", false, "Send THUMBS_UP key command")
		thumbsDown     = flag.Bool("thumbs-down", false, "Send THUMBS_DOWN key command")
		preset         = flag.Int("preset", 0, "Select preset (1-6)")
		volume         = flag.Bool("volume", false, "Get current volume level")
		setVolume      = flag.Int("set-volume", -1, "Set volume level (0-100)")
		incVolume      = flag.Int("inc-volume", 0, "Increase volume by amount (1-10, default: 2)")
		decVolume      = flag.Int("dec-volume", 0, "Decrease volume by amount (1-10, default: 2)")
		bass           = flag.Bool("bass", false, "Get current bass level")
		setBass        = flag.Int("set-bass", -99, "Set bass level (-9 to +9)")
		incBass        = flag.Int("inc-bass", 0, "Increase bass by amount (1-3, default: 1)")
		decBass        = flag.Int("dec-bass", 0, "Decrease bass by amount (1-3, default: 1)")
		balance        = flag.Bool("balance", false, "Get current balance level")
		setBalance     = flag.Int("set-balance", -99, "Set balance level (-50 to +50)")
		incBalance     = flag.Int("inc-balance", 0, "Increase balance by amount (1-10, default: 5)")
		decBalance     = flag.Int("dec-balance", 0, "Decrease balance by amount (1-10, default: 5)")
		selectSource   = flag.String("select-source", "", "Select audio source (SPOTIFY, BLUETOOTH, AUX, TUNEIN, PANDORA, AMAZON, IHEARTRADIO, STORED_MUSIC)")
		sourceAccount  = flag.String("source-account", "", "Source account for streaming services (optional)")
		spotify        = flag.Bool("spotify", false, "Select Spotify source")
		bluetooth      = flag.Bool("bluetooth", false, "Select Bluetooth source")
		aux            = flag.Bool("aux", false, "Select AUX input source")
		clockTime      = flag.Bool("clock-time", false, "Get device clock time")
		setClockTime   = flag.String("set-clock-time", "", "Set device clock time (format: 'now' or Unix timestamp)")
		clockDisplay   = flag.Bool("clock-display", false, "Get clock display settings")
		enableClock    = flag.Bool("enable-clock", false, "Enable clock display")
		disableClock   = flag.Bool("disable-clock", false, "Disable clock display")
		clockFormat    = flag.String("clock-format", "", "Set clock display format (12, 24, auto)")
		clockBright    = flag.Int("clock-brightness", -1, "Set clock display brightness (0-100)")
		networkInfo    = flag.Bool("network-info", false, "Get network information")
		zone           = flag.Bool("zone", false, "Get current zone configuration")
		zoneStatus     = flag.Bool("zone-status", false, "Get zone status for this device")
		zoneMembers    = flag.Bool("zone-members", false, "List all devices in current zone")
		createZone     = flag.String("create-zone", "", "Create zone with device IDs (comma-separated)")
		addToZone      = flag.String("add-to-zone", "", "Add device to zone (format: deviceID@ip or deviceID)")
		removeFromZone = flag.String("remove-from-zone", "", "Remove device from zone (device ID)")
		dissolveZone   = flag.Bool("dissolve-zone", false, "Dissolve current zone (make standalone)")
		help           = flag.Bool("help", false, "Show help")
	)

	flag.Parse()

	if *help {
		printHelp()
		return
	}

	// If no specific action is requested, show help
	if !*discover && !*discoverAll && !*info && !*nowPlaying && !*sources && !*name && !*capabilities && !*presets && *key == "" && !*play && !*pause && !*stop && !*next && !*prev && !*volumeUp && !*volumeDown && !*power && !*mute && !*thumbsUp && !*thumbsDown && *preset == 0 && !*volume && *setVolume == -1 && *incVolume == 0 && *decVolume == 0 && !*bass && *setBass == -99 && *incBass == 0 && *decBass == 0 && !*balance && *setBalance == -99 && *incBalance == 0 && *decBalance == 0 && *selectSource == "" && !*spotify && !*bluetooth && !*aux && !*clockTime && *setClockTime == "" && !*clockDisplay && !*enableClock && !*disableClock && *clockFormat == "" && *clockBright == -1 && !*networkInfo && !*zone && !*zoneStatus && !*zoneMembers && *createZone == "" && *addToZone == "" && *removeFromZone == "" && !*dissolveZone && *host == "" {
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
	if *key != "" || *play || *pause || *stop || *next || *prev || *volumeUp || *volumeDown || *power || *mute || *thumbsUp || *thumbsDown || *preset > 0 {
		if *host == "" {
			log.Fatal("Host is required for key commands. Use -host flag or -discover to find devices.")
		}
		if err := handleKeyCommands(finalHost, finalPort, *timeout, *key, *play, *pause, *stop, *next, *prev, *volumeUp, *volumeDown, *power, *mute, *thumbsUp, *thumbsDown, *preset); err != nil {
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

	// Handle bass commands
	if *bass || *setBass != -99 || *incBass > 0 || *decBass > 0 {
		if *host == "" {
			log.Fatal("Host is required for bass commands. Use -host flag or -discover to find devices.")
		}
		if err := handleBassCommands(finalHost, finalPort, *timeout, *bass, *setBass, *incBass, *decBass); err != nil {
			log.Fatalf("Failed to execute bass command: %v", err)
		}
		return
	}

	// Handle balance commands
	if *balance || *setBalance != -99 || *incBalance > 0 || *decBalance > 0 {
		if *host == "" {
			log.Fatal("Host is required for balance commands. Use -host flag or -discover to find devices.")
		}
		if err := handleBalanceCommands(finalHost, finalPort, *timeout, *balance, *setBalance, *incBalance, *decBalance); err != nil {
			log.Fatalf("Failed to execute balance command: %v", err)
		}
		return
	}

	// Handle source selection commands
	if *selectSource != "" || *spotify || *bluetooth || *aux {
		if *host == "" {
			log.Fatal("Host is required for source selection. Use -host flag or -discover to find devices.")
		}
		if err := handleSourceCommands(finalHost, finalPort, *timeout, *selectSource, *sourceAccount, *spotify, *bluetooth, *aux); err != nil {
			log.Fatalf("Failed to select source: %v", err)
		}
		return
	}

	// Handle clock/time commands
	if *clockTime || *setClockTime != "" || *clockDisplay || *enableClock || *disableClock || *clockFormat != "" || *clockBright != -1 {
		if *host == "" {
			log.Fatal("Host is required for clock/time commands. Use -host flag or -discover to find devices.")
		}
		if err := handleClockCommands(finalHost, finalPort, *timeout, *clockTime, *setClockTime, *clockDisplay, *enableClock, *disableClock, *clockFormat, *clockBright); err != nil {
			log.Fatalf("Failed to execute clock command: %v", err)
		}
		return
	}

	// Handle network info command
	if *networkInfo {
		if *host == "" {
			log.Fatal("Host is required for network info command. Use -host flag or -discover to find devices.")
		}
		if err := handleNetworkInfo(finalHost, finalPort, *timeout); err != nil {
			log.Fatalf("Failed to get network info: %v", err)
		}
		return
	}

	// Handle zone commands
	if *zone || *zoneStatus || *zoneMembers || *createZone != "" || *addToZone != "" || *removeFromZone != "" || *dissolveZone {
		if *host == "" {
			log.Fatal("Host is required for zone commands. Use -host flag or -discover to find devices.")
		}
		if err := handleZoneCommands(finalHost, finalPort, *timeout, *zone, *zoneStatus, *zoneMembers, *createZone, *addToZone, *removeFromZone, *dissolveZone); err != nil {
			log.Fatalf("Failed to execute zone command: %v", err)
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
	fmt.Println("                    Available keys: PLAY, PAUSE, STOP, PREV_TRACK, NEXT_TRACK")
	fmt.Println("                    THUMBS_UP, THUMBS_DOWN, BOOKMARK, POWER, MUTE")
	fmt.Println("                    VOLUME_UP, VOLUME_DOWN, PRESET_1-6, AUX_INPUT")
	fmt.Println("                    SHUFFLE_OFF, SHUFFLE_ON, REPEAT_OFF, REPEAT_ONE, REPEAT_ALL")
	fmt.Println("  -play             Send PLAY key command (requires -host)")
	fmt.Println("  -pause            Send PAUSE key command (requires -host)")
	fmt.Println("  -stop             Send STOP key command (requires -host)")
	fmt.Println("  -next             Send NEXT_TRACK key command (requires -host)")
	fmt.Println("  -prev             Send PREV_TRACK key command (requires -host)")
	fmt.Println("  -volume-up        Send VOLUME_UP key command (requires -host)")
	fmt.Println("  -volume-down      Send VOLUME_DOWN key command (requires -host)")
	fmt.Println("  -power            Send POWER key command (requires -host)")
	fmt.Println("  -mute             Send MUTE key command (requires -host)")
	fmt.Println("  -thumbs-up        Send THUMBS_UP key command (requires -host)")
	fmt.Println("  -thumbs-down      Send THUMBS_DOWN key command (requires -host)")
	fmt.Println("  -preset <1-6>     Select preset (requires -host)")
	fmt.Println("  -volume           Get current volume level (requires -host)")
	fmt.Println("  -set-volume <0-100> Set volume level (requires -host)")
	fmt.Println("  -inc-volume <n>   Increase volume by amount (1-10, default: 2)")
	fmt.Println("  -dec-volume <n>   Decrease volume by amount (1-10, default: 2)")
	fmt.Println()
	fmt.Println("Bass Control:")
	fmt.Println("  -bass             Get current bass level (requires -host)")
	fmt.Println("  -set-bass <-9-+9> Set bass level (requires -host)")
	fmt.Println("  -inc-bass <n>     Increase bass by amount (1-3, default: 1)")
	fmt.Println("  -dec-bass <n>     Decrease bass by amount (1-3, default: 1)")
	fmt.Println()
	fmt.Println("Balance Control:")
	fmt.Println("  -balance          Get current balance level (requires -host)")
	fmt.Println("  -set-balance <-50-+50> Set balance level (requires -host)")
	fmt.Println("  -inc-balance <n>  Increase balance by amount (1-10, default: 5)")
	fmt.Println("  -dec-balance <n>  Decrease balance by amount (1-10, default: 5)")
	fmt.Println()
	fmt.Println("Source Selection:")
	fmt.Println("  -select-source <source>  Select audio source (requires -host)")
	fmt.Println("                          Available: SPOTIFY, BLUETOOTH, AUX, TUNEIN, PANDORA, AMAZON, IHEARTRADIO, STORED_MUSIC")
	fmt.Println("  -source-account <account> Source account for streaming services (optional)")
	fmt.Println("  -spotify          Select Spotify source (requires -host)")
	fmt.Println("  -bluetooth        Select Bluetooth source (requires -host)")
	fmt.Println("  -aux              Select AUX input source (requires -host)")
	fmt.Println()
	fmt.Println("System Information:")
	fmt.Println("  -clock-time       Get device clock time (requires -host)")
	fmt.Println("  -set-clock-time <time> Set device clock time (requires -host)")
	fmt.Println("                    Use 'now' for current time or Unix timestamp")
	fmt.Println("  -clock-display    Get clock display settings (requires -host)")
	fmt.Println("  -enable-clock     Enable clock display (requires -host)")
	fmt.Println("  -disable-clock    Disable clock display (requires -host)")
	fmt.Println("  -clock-format <fmt> Set clock format: 12, 24, auto (requires -host)")
	fmt.Println("  -clock-brightness <0-100> Set clock brightness (requires -host)")
	fmt.Println("  -network-info     Get network information (requires -host)")
	fmt.Println()
	fmt.Println("Zone Management:")
	fmt.Println("  -zone             Get current zone configuration (requires -host)")
	fmt.Println("  -zone-status      Get zone status for this device (requires -host)")
	fmt.Println("  -zone-members     List all devices in current zone (requires -host)")
	fmt.Println("  -create-zone <devices> Create zone with device IDs (comma-separated)")
	fmt.Println("                    Format: masterID,memberID1,memberID2,...")
	fmt.Println("  -add-to-zone <device> Add device to zone (requires -host)")
	fmt.Println("                    Format: deviceID@ip or deviceID")
	fmt.Println("  -remove-from-zone <deviceID> Remove device from zone (requires -host)")
	fmt.Println("  -dissolve-zone    Dissolve current zone, make device standalone (requires -host)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  soundtouch-cli -discover")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -info")
	fmt.Println("  soundtouch-cli -host 192.168.1.10:8090 -nowplaying")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -play")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -set-volume 50")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -bass")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -set-bass 3")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -balance")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -set-balance 10")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -key NEXT_TRACK")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -preset 1")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -select-source SPOTIFY")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -bluetooth")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -aux")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -capabilities")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -presets")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -clock-time")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -set-clock-time now")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -clock-display")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -enable-clock")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -clock-format 24")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -clock-brightness 75")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -network-info")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -zone")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -zone-status")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -zone-members")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -create-zone MASTER123,DEVICE456,DEVICE789")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -add-to-zone DEVICE456@192.168.1.11")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -remove-from-zone DEVICE456")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -dissolve-zone")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -play")
	fmt.Println("  soundtouch-cli -host 192.168.1.10:8090 -pause")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -volume-up")
	fmt.Println("  soundtouch-cli -host 192.168.1.10:8090 -preset 1")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -key STOP")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -power")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -mute")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -thumbs-up")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -key SHUFFLE_ON")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -key REPEAT_ALL")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -volume")
	fmt.Println("  soundtouch-cli -host 192.168.1.10:8090 -set-volume 25")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -inc-volume 2")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -dec-volume 3")
	fmt.Println("  soundtouch-cli -host 192.168.1.10 -port 8090 -info")
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

func handleKeyCommands(host string, port int, timeout time.Duration, key string, play, pause, stop, next, prev, volumeUp, volumeDown, power, mute, thumbsUp, thumbsDown bool, preset int) error {
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
	if power {
		commandCount++
		commandName = "POWER"
	}
	if mute {
		commandCount++
		commandName = "MUTE"
	}
	if thumbsUp {
		commandCount++
		commandName = "THUMBS_UP"
	}
	if thumbsDown {
		commandCount++
		commandName = "THUMBS_DOWN"
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
	} else if power {
		err = soundtouchClient.SendKey(models.KeyPower)
	} else if mute {
		err = soundtouchClient.SendKey(models.KeyMute)
	} else if thumbsUp {
		err = soundtouchClient.SendKey(models.KeyThumbsUp)
	} else if thumbsDown {
		err = soundtouchClient.SendKey(models.KeyThumbsDown)
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

// handleSourceCommands handles source selection commands
func handleSourceCommands(host string, port int, timeout time.Duration, selectSource, sourceAccount string, spotify, bluetooth, aux bool) error {
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

	c := client.NewClient(clientConfig)

	// Handle convenience flags first
	if spotify {
		fmt.Printf("Selecting Spotify source...\n")
		err := c.SelectSpotify(sourceAccount)
		if err != nil {
			return fmt.Errorf("failed to select Spotify: %w", err)
		}
		fmt.Println("✓ Spotify source selected successfully")
		return nil
	}

	if bluetooth {
		fmt.Printf("Selecting Bluetooth source...\n")
		err := c.SelectBluetooth()
		if err != nil {
			return fmt.Errorf("failed to select Bluetooth: %w", err)
		}
		fmt.Println("✓ Bluetooth source selected successfully")
		return nil
	}

	if aux {
		fmt.Printf("Selecting AUX input source...\n")
		err := c.SelectAux()
		if err != nil {
			return fmt.Errorf("failed to select AUX: %w", err)
		}
		fmt.Println("✓ AUX input source selected successfully")
		return nil
	}

	// Handle generic source selection
	if selectSource != "" {
		fmt.Printf("Selecting source: %s", selectSource)
		if sourceAccount != "" {
			fmt.Printf(" (account: %s)", sourceAccount)
		}
		fmt.Printf("...\n")

		err := c.SelectSource(selectSource, sourceAccount)
		if err != nil {
			return fmt.Errorf("failed to select source %s: %w", selectSource, err)
		}
		fmt.Printf("✓ Source %s selected successfully\n", selectSource)
	}

	return nil
}

// handleBassCommands handles bass control commands
func handleBassCommands(host string, port int, timeout time.Duration, getBass bool, setBass, incBass, decBass int) error {
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

	// Handle get bass
	if getBass {
		fmt.Printf("Getting current bass level from %s:%d...\n", host, port)
		bass, err := soundtouchClient.GetBass()
		if err != nil {
			return fmt.Errorf("failed to get bass: %w", err)
		}

		fmt.Printf("Bass Level: %d (%s)\n", bass.GetLevel(), models.GetBassLevelName(bass.GetLevel()))
		fmt.Printf("Category: %s\n", models.GetBassLevelCategory(bass.GetLevel()))
		if !bass.IsAtTarget() {
			fmt.Printf("Target: %d, Actual: %d (adjusting...)\n", bass.TargetBass, bass.ActualBass)
		}
		return nil
	}

	// Handle set bass
	if setBass != -99 {
		if !models.ValidateBassLevel(setBass) {
			return fmt.Errorf("invalid bass level: %d (must be between %d and %d)", setBass, models.BassLevelMin, models.BassLevelMax)
		}

		fmt.Printf("Setting bass to %d on %s:%d...\n", setBass, host, port)
		err := soundtouchClient.SetBass(setBass)
		if err != nil {
			return fmt.Errorf("failed to set bass: %w", err)
		}

		// Get updated bass level to confirm
		bass, err := soundtouchClient.GetBass()
		if err != nil {
			fmt.Printf("✓ Bass set successfully\n")
		} else {
			fmt.Printf("✓ Bass set to %d (%s)\n", bass.GetLevel(), models.GetBassLevelName(bass.GetLevel()))
		}
		return nil
	}

	// Handle bass increase (with safety limits)
	if incBass > 0 {
		if incBass > 3 {
			incBass = 3 // Safety limit
		}
		if incBass == 0 {
			incBass = 1 // Default increment
		}

		fmt.Printf("Increasing bass by %d on %s:%d...\n", incBass, host, port)
		bass, err := soundtouchClient.IncreaseBass(incBass)
		if err != nil {
			return fmt.Errorf("failed to increase bass: %w", err)
		}

		fmt.Printf("✓ Bass increased to %d (%s)\n", bass.GetLevel(), models.GetBassLevelName(bass.GetLevel()))
		return nil
	}

	// Handle bass decrease
	if decBass > 0 {
		if decBass > 3 {
			decBass = 3 // Safety limit for decrease
		}
		if decBass == 0 {
			decBass = 1 // Default decrement
		}

		fmt.Printf("Decreasing bass by %d on %s:%d...\n", decBass, host, port)
		bass, err := soundtouchClient.DecreaseBass(decBass)
		if err != nil {
			return fmt.Errorf("failed to decrease bass: %w", err)
		}

		fmt.Printf("✓ Bass decreased to %d (%s)\n", bass.GetLevel(), models.GetBassLevelName(bass.GetLevel()))
		return nil
	}

	return fmt.Errorf("no bass command specified")
}

// handleBalanceCommands handles balance control commands
func handleBalanceCommands(host string, port int, timeout time.Duration, getBalance bool, setBalance, incBalance, decBalance int) error {
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

	// Handle get balance
	if getBalance {
		fmt.Printf("Getting current balance level from %s:%d...\n", host, port)
		balance, err := soundtouchClient.GetBalance()
		if err != nil {
			return fmt.Errorf("failed to get balance: %w", err)
		}

		fmt.Printf("Balance Level: %d (%s)\n", balance.GetLevel(), models.GetBalanceLevelName(balance.GetLevel()))
		fmt.Printf("Category: %s\n", models.GetBalanceLevelCategory(balance.GetLevel()))
		left, right := balance.GetLeftRightPercentage()
		fmt.Printf("Left/Right: %d%%/%d%%\n", left, right)
		if !balance.IsAtTarget() {
			fmt.Printf("Target: %d, Actual: %d (adjusting...)\n", balance.TargetBalance, balance.ActualBalance)
		}
		return nil
	}

	// Handle set balance
	if setBalance != -99 {
		if !models.ValidateBalanceLevel(setBalance) {
			return fmt.Errorf("invalid balance level: %d (must be between %d and %d)", setBalance, models.BalanceLevelMin, models.BalanceLevelMax)
		}

		fmt.Printf("Setting balance to %d on %s:%d...\n", setBalance, host, port)
		err := soundtouchClient.SetBalance(setBalance)
		if err != nil {
			return fmt.Errorf("failed to set balance: %w", err)
		}

		// Get updated balance level to confirm
		balance, err := soundtouchClient.GetBalance()
		if err != nil {
			fmt.Printf("✓ Balance set successfully\n")
		} else {
			fmt.Printf("✓ Balance set to %d (%s)\n", balance.GetLevel(), models.GetBalanceLevelName(balance.GetLevel()))
		}
		return nil
	}

	// Handle balance increase (with safety limits)
	if incBalance > 0 {
		if incBalance > 10 {
			incBalance = 10 // Safety limit
		}
		if incBalance == 0 {
			incBalance = 5 // Default increment
		}

		fmt.Printf("Increasing balance by %d on %s:%d...\n", incBalance, host, port)
		balance, err := soundtouchClient.IncreaseBalance(incBalance)
		if err != nil {
			return fmt.Errorf("failed to increase balance: %w", err)
		}

		fmt.Printf("✓ Balance increased to %d (%s)\n", balance.GetLevel(), models.GetBalanceLevelName(balance.GetLevel()))
		return nil
	}

	// Handle balance decrease
	if decBalance > 0 {
		if decBalance > 10 {
			decBalance = 10 // Safety limit for decrease
		}
		if decBalance == 0 {
			decBalance = 5 // Default decrement
		}

		fmt.Printf("Decreasing balance by %d on %s:%d...\n", decBalance, host, port)
		balance, err := soundtouchClient.DecreaseBalance(decBalance)
		if err != nil {
			return fmt.Errorf("failed to decrease balance: %w", err)
		}

		fmt.Printf("✓ Balance decreased to %d (%s)\n", balance.GetLevel(), models.GetBalanceLevelName(balance.GetLevel()))
		return nil
	}

	return fmt.Errorf("no balance command specified")
}

// handleClockCommands handles all clock/time related commands
func handleClockCommands(host string, port int, timeout time.Duration, getClockTime bool, setClockTime string, getClockDisplay, enableClock, disableClock bool, clockFormat string, clockBright int) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	// Configure client with custom timeout
	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	// Get clock time
	if getClockTime {
		fmt.Printf("Getting clock time from %s:%d...\n", host, port)
		clockTime, err := soundtouchClient.GetClockTime()
		if err != nil {
			return fmt.Errorf("failed to get clock time: %w", err)
		}

		fmt.Printf("Device Clock Time:\n")
		if !clockTime.IsEmpty() {
			fmt.Printf("  Time: %s\n", clockTime.GetTimeString())
			if clockTime.GetUTC() > 0 {
				fmt.Printf("  UTC Timestamp: %d\n", clockTime.GetUTC())
			}
			if clockTime.GetZone() != "" {
				fmt.Printf("  Timezone: %s\n", clockTime.GetZone())
			}
		} else {
			fmt.Printf("  No time data available\n")
		}
		return nil
	}

	// Set clock time
	if setClockTime != "" {
		fmt.Printf("Setting clock time on %s:%d...\n", host, port)
		if setClockTime == "now" {
			err = soundtouchClient.SetClockTimeNow()
		} else {
			// Try to parse as Unix timestamp
			if timestamp, parseErr := strconv.ParseInt(setClockTime, 10, 64); parseErr == nil {
				request := models.NewClockTimeRequestUTC(timestamp)
				err = soundtouchClient.SetClockTime(request)
			} else {
				return fmt.Errorf("invalid time format: use 'now' or Unix timestamp")
			}
		}

		if err != nil {
			return fmt.Errorf("failed to set clock time: %w", err)
		}

		fmt.Printf("✓ Clock time set successfully\n")
		return nil
	}

	// Get clock display settings
	if getClockDisplay {
		fmt.Printf("Getting clock display settings from %s:%d...\n", host, port)
		clockDisplay, err := soundtouchClient.GetClockDisplay()
		if err != nil {
			return fmt.Errorf("failed to get clock display settings: %w", err)
		}

		fmt.Printf("Clock Display Settings:\n")
		if clockDisplay.GetDeviceID() != "" {
			fmt.Printf("  Device ID: %s\n", clockDisplay.GetDeviceID())
		}
		fmt.Printf("  Enabled: %v\n", clockDisplay.IsEnabled())
		fmt.Printf("  Format: %s\n", clockDisplay.GetFormatDescription())
		fmt.Printf("  Brightness: %d%% (%s)\n", clockDisplay.GetBrightness(), clockDisplay.GetBrightnessLevel())
		fmt.Printf("  Auto-Dim: %v\n", clockDisplay.IsAutoDimEnabled())
		if clockDisplay.GetTimeZone() != "" {
			fmt.Printf("  Timezone: %s\n", clockDisplay.GetTimeZone())
		}
		return nil
	}

	// Enable clock display
	if enableClock {
		fmt.Printf("Enabling clock display on %s:%d...\n", host, port)
		err = soundtouchClient.EnableClockDisplay()
		if err != nil {
			return fmt.Errorf("failed to enable clock display: %w", err)
		}
		fmt.Printf("✓ Clock display enabled\n")
		return nil
	}

	// Disable clock display
	if disableClock {
		fmt.Printf("Disabling clock display on %s:%d...\n", host, port)
		err = soundtouchClient.DisableClockDisplay()
		if err != nil {
			return fmt.Errorf("failed to disable clock display: %w", err)
		}
		fmt.Printf("✓ Clock display disabled\n")
		return nil
	}

	// Set clock format
	if clockFormat != "" {
		fmt.Printf("Setting clock format to %s on %s:%d...\n", clockFormat, host, port)

		var format models.ClockFormat
		switch clockFormat {
		case "12":
			format = models.ClockFormat12Hour
		case "24":
			format = models.ClockFormat24Hour
		case "auto":
			format = models.ClockFormatAuto
		default:
			return fmt.Errorf("invalid clock format: use '12', '24', or 'auto'")
		}

		err = soundtouchClient.SetClockDisplayFormat(format)
		if err != nil {
			return fmt.Errorf("failed to set clock format: %w", err)
		}
		fmt.Printf("✓ Clock format set to %s\n", clockFormat)
		return nil
	}

	// Set clock brightness
	if clockBright != -1 {
		if clockBright < 0 || clockBright > 100 {
			return fmt.Errorf("brightness must be between 0 and 100")
		}

		fmt.Printf("Setting clock brightness to %d%% on %s:%d...\n", clockBright, host, port)
		err = soundtouchClient.SetClockDisplayBrightness(clockBright)
		if err != nil {
			return fmt.Errorf("failed to set clock brightness: %w", err)
		}
		fmt.Printf("✓ Clock brightness set to %d%%\n", clockBright)
		return nil
	}

	return fmt.Errorf("no clock command specified")
}

// handleNetworkInfo handles network information retrieval
func handleNetworkInfo(host string, port int, timeout time.Duration) error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override config with command line arguments if provided
	if timeout > 0 {
		cfg.HTTPTimeout = timeout
	}

	// Configure client with custom timeout
	clientConfig := client.ClientConfig{
		Host:      host,
		Port:      port,
		Timeout:   cfg.HTTPTimeout,
		UserAgent: cfg.UserAgent,
	}

	soundtouchClient := client.NewClient(clientConfig)

	fmt.Printf("Getting network information from %s:%d...\n", host, port)
	networkInfo, err := soundtouchClient.GetNetworkInfo()
	if err != nil {
		return fmt.Errorf("failed to get network info: %w", err)
	}

	fmt.Printf("Network Information:\n")

	if networkInfo.GetWifiProfileCount() > 0 {
		fmt.Printf("  WiFi Profiles: %d\n", networkInfo.GetWifiProfileCount())
	}

	interfaces := networkInfo.GetInterfaces()
	if len(interfaces) > 0 {
		fmt.Printf("  Total Interfaces: %d\n", len(interfaces))

		activeInterfaces := networkInfo.GetActiveInterfaces()
		fmt.Printf("  Active Interfaces: %d\n\n", len(activeInterfaces))

		for i, iface := range interfaces {
			fmt.Printf("Interface %d:\n", i+1)
			fmt.Printf("  • Type: %s", iface.GetType())
			if iface.GetName() != "" {
				fmt.Printf(" (%s)", iface.GetName())
			}
			fmt.Printf("\n")

			if iface.GetMacAddress() != "" {
				fmt.Printf("  • MAC Address: %s", iface.GetMacAddress())
				if !iface.ValidateMAC() {
					fmt.Printf(" (invalid)")
				}
				fmt.Printf("\n")
			}

			if iface.GetIPAddress() != "" {
				fmt.Printf("  • IP Address: %s", iface.GetIPAddress())
				if !iface.ValidateIP() {
					fmt.Printf(" (invalid)")
				}
				fmt.Printf("\n")
			}

			fmt.Printf("  • State: %s", iface.GetStateDescription())
			if iface.IsConnected() {
				fmt.Printf(" ✓")
			}
			fmt.Printf("\n")

			// WiFi-specific information
			if iface.IsWiFi() {
				if iface.GetSSID() != "" {
					fmt.Printf("  • SSID: %s\n", iface.GetSSID())
				}

				if iface.GetSignal() != "" {
					fmt.Printf("  • Signal: %s (%d%%)\n",
						iface.GetSignalDescription(),
						iface.GetSignalQuality())
				}

				if iface.GetFrequencyKHz() > 0 {
					fmt.Printf("  • Frequency: %s (%s)\n",
						iface.FormatFrequency(),
						iface.GetFrequencyBand())
				}

				if iface.GetMode() != "" {
					fmt.Printf("  • Mode: %s\n", iface.GetModeDescription())
				}
			}

			// Network summary
			summary := iface.GetNetworkSummary()
			if summary != "" {
				fmt.Printf("  • Summary: %s\n", summary)
			}

			fmt.Printf("\n")
		}
	} else {
		fmt.Printf("  No network interfaces found\n")
	}

	// Connected interface details
	if connectedWifi := networkInfo.GetConnectedWiFiInterface(); connectedWifi != nil {
		fmt.Printf("Active WiFi Connection:\n")
		fmt.Printf("  • Network: %s\n", connectedWifi.GetNetworkSummary())
		fmt.Printf("  • Interface: %s\n", connectedWifi.GetName())
		fmt.Printf("  • IP Address: %s\n", connectedWifi.GetIPAddress())
		fmt.Printf("\n")
	}

	if connectedEthernet := networkInfo.GetConnectedEthernetInterface(); connectedEthernet != nil {
		fmt.Printf("Active Ethernet Connection:\n")
		fmt.Printf("  • Interface: %s\n", connectedEthernet.GetName())
		fmt.Printf("  • IP Address: %s\n", connectedEthernet.GetIPAddress())
		fmt.Printf("\n")
	}

	// Connectivity Summary
	fmt.Printf("Connectivity Summary:\n")
	if networkInfo.HasWiFi() {
		fmt.Printf("  ✓ WiFi Available")
		if networkInfo.GetConnectedWiFiInterface() != nil {
			fmt.Printf(" (Connected)")
		}
		fmt.Printf("\n")
	} else {
		fmt.Printf("  ✗ No WiFi\n")
	}

	if networkInfo.HasEthernet() {
		fmt.Printf("  ✓ Ethernet Available")
		if networkInfo.GetConnectedEthernetInterface() != nil {
			fmt.Printf(" (Connected)")
		}
		fmt.Printf("\n")
	} else {
		fmt.Printf("  ✗ No Ethernet\n")
	}

	return nil
}

// handleZoneCommands handles all zone management commands
func handleZoneCommands(host string, port int, timeout time.Duration, getZone, getZoneStatus, getZoneMembers bool, createZone, addToZone, removeFromZone string, dissolveZone bool) error {
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

	// Handle get zone info
	if getZone {
		return handleGetZone(soundtouchClient)
	}

	// Handle get zone status
	if getZoneStatus {
		return handleGetZoneStatus(soundtouchClient)
	}

	// Handle get zone members
	if getZoneMembers {
		return handleGetZoneMembers(soundtouchClient)
	}

	// Handle create zone
	if createZone != "" {
		return handleCreateZone(soundtouchClient, createZone)
	}

	// Handle add to zone
	if addToZone != "" {
		return handleAddToZone(soundtouchClient, addToZone)
	}

	// Handle remove from zone
	if removeFromZone != "" {
		return handleRemoveFromZone(soundtouchClient, removeFromZone)
	}

	// Handle dissolve zone
	if dissolveZone {
		return handleDissolveZone(soundtouchClient)
	}

	return nil
}

// handleGetZone displays current zone configuration
func handleGetZone(client *client.Client) error {
	zone, err := client.GetZone()
	if err != nil {
		return err
	}

	fmt.Printf("Zone Configuration:\n")
	fmt.Printf("  Master Device: %s\n", zone.Master)

	if zone.IsStandalone() {
		fmt.Printf("  Status: Standalone (no multiroom zone)\n")
		fmt.Printf("  Total Devices: 1\n")
	} else {
		fmt.Printf("  Status: Active multiroom zone\n")
		fmt.Printf("  Total Devices: %d\n", zone.GetTotalDeviceCount())
		fmt.Printf("  Zone Members:\n")
		for i, member := range zone.Members {
			fmt.Printf("    %d. %s", i+1, member.DeviceID)
			if member.IP != "" {
				fmt.Printf(" (%s)", member.IP)
			}
			fmt.Printf("\n")
		}
	}

	return nil
}

// handleGetZoneStatus displays zone status for this device
func handleGetZoneStatus(client *client.Client) error {
	status, err := client.GetZoneStatus()
	if err != nil {
		return err
	}

	// Get device info for context
	deviceInfo, err := client.GetDeviceInfo()
	if err != nil {
		return err
	}

	fmt.Printf("Zone Status for %s (%s):\n", deviceInfo.Name, deviceInfo.DeviceID)
	fmt.Printf("  Status: %s\n", status.String())

	// Show additional zone info if in a zone
	if status != models.ZoneStatusStandalone {
		zone, err := client.GetZone()
		if err == nil {
			if status == models.ZoneStatusMaster {
				fmt.Printf("  Zone Members: %d\n", len(zone.Members))
			} else {
				fmt.Printf("  Zone Master: %s\n", zone.Master)
			}
		}
	}

	return nil
}

// handleGetZoneMembers lists all devices in the current zone
func handleGetZoneMembers(client *client.Client) error {
	members, err := client.GetZoneMembers()
	if err != nil {
		return err
	}

	zone, err := client.GetZone()
	if err != nil {
		return err
	}

	fmt.Printf("Zone Members:\n")
	if len(members) == 1 {
		fmt.Printf("  Device is standalone (not in a zone)\n")
		fmt.Printf("  Device: %s\n", members[0])
	} else {
		fmt.Printf("  Total Devices: %d\n", len(members))
		fmt.Printf("  Master: %s\n", zone.Master)
		fmt.Printf("  Members:\n")
		for i, memberID := range members {
			if memberID == zone.Master {
				fmt.Printf("    %d. %s (Master)\n", i+1, memberID)
			} else {
				// Find IP address if available
				var ip string
				if member, found := zone.GetMemberByDeviceID(memberID); found {
					ip = member.IP
				}
				fmt.Printf("    %d. %s", i+1, memberID)
				if ip != "" {
					fmt.Printf(" (%s)", ip)
				}
				fmt.Printf("\n")
			}
		}
	}

	return nil
}

// handleCreateZone creates a new multiroom zone
func handleCreateZone(client *client.Client, deviceList string) error {
	deviceIDs := strings.Split(deviceList, ",")
	if len(deviceIDs) < 2 {
		return fmt.Errorf("at least 2 devices required for zone creation (master + 1 member)")
	}

	// Clean up device IDs
	for i := range deviceIDs {
		deviceIDs[i] = strings.TrimSpace(deviceIDs[i])
	}

	masterID := deviceIDs[0]
	memberIDs := deviceIDs[1:]

	fmt.Printf("Creating zone with master %s and %d member(s)...\n", masterID, len(memberIDs))

	err := client.CreateZone(masterID, memberIDs)
	if err != nil {
		return fmt.Errorf("failed to create zone: %w", err)
	}

	fmt.Printf("✓ Zone created successfully\n")
	fmt.Printf("  Master: %s\n", masterID)
	fmt.Printf("  Members: %s\n", strings.Join(memberIDs, ", "))

	return nil
}

// handleAddToZone adds a device to the current zone
func handleAddToZone(client *client.Client, deviceSpec string) error {
	var deviceID, ipAddress string

	// Parse device specification (deviceID@ip or just deviceID)
	if strings.Contains(deviceSpec, "@") {
		parts := strings.Split(deviceSpec, "@")
		if len(parts) != 2 {
			return fmt.Errorf("invalid device specification. Use format: deviceID@ip or deviceID")
		}
		deviceID = strings.TrimSpace(parts[0])
		ipAddress = strings.TrimSpace(parts[1])
	} else {
		deviceID = strings.TrimSpace(deviceSpec)
	}

	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}

	fmt.Printf("Adding device %s to zone", deviceID)
	if ipAddress != "" {
		fmt.Printf(" (IP: %s)", ipAddress)
	}
	fmt.Printf("...\n")

	err := client.AddToZone(deviceID, ipAddress)
	if err != nil {
		return fmt.Errorf("failed to add device to zone: %w", err)
	}

	fmt.Printf("✓ Device %s added to zone successfully\n", deviceID)

	return nil
}

// handleRemoveFromZone removes a device from the current zone
func handleRemoveFromZone(client *client.Client, deviceID string) error {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return fmt.Errorf("device ID cannot be empty")
	}

	fmt.Printf("Removing device %s from zone...\n", deviceID)

	err := client.RemoveFromZone(deviceID)
	if err != nil {
		return fmt.Errorf("failed to remove device from zone: %w", err)
	}

	fmt.Printf("✓ Device %s removed from zone successfully\n", deviceID)

	return nil
}

// handleDissolveZone dissolves the current zone
func handleDissolveZone(client *client.Client) error {
	fmt.Printf("Dissolving current zone (making device standalone)...\n")

	err := client.DissolveZone()
	if err != nil {
		return fmt.Errorf("failed to dissolve zone: %w", err)
	}

	fmt.Printf("✓ Zone dissolved successfully - device is now standalone\n")

	return nil
}
