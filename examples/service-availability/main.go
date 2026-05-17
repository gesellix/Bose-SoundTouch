// Package main demonstrates service availability checking for SoundTouch devices
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
	// Get SoundTouch device host from command line argument or environment variable
	host := getSoundTouchHost()
	if host == "" {
		fmt.Println("Usage: go run main.go <soundtouch-host>")
		fmt.Println("   or: SOUNDTOUCH_TEST_HOST=192.0.2.100 go run main.go")
		os.Exit(1)
	}

	// Create client
	soundtouchClient := client.NewClientFromHost(host)

	// Get service availability
	serviceAvailability, err := soundtouchClient.GetServiceAvailability()
	if err != nil {
		log.Fatalf("Failed to get service availability: %v", err)
	}

	// Display comprehensive service availability report
	displayServiceReport(serviceAvailability)

	// Show practical usage examples
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("PRACTICAL USAGE EXAMPLES")
	fmt.Println(strings.Repeat("=", 60))

	demonstrateUserFeedback(serviceAvailability, soundtouchClient)
}

func getSoundTouchHost() string {
	// Check command line arguments first
	if len(os.Args) > 1 {
		return os.Args[1]
	}

	// Fall back to environment variable
	return os.Getenv("SOUNDTOUCH_TEST_HOST")
}

func displayServiceReport(sa *models.ServiceAvailability) {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("SOUNDTOUCH SERVICE AVAILABILITY REPORT")
	fmt.Println(strings.Repeat("=", 60))

	if sa.Services == nil {
		fmt.Println("No service information available")
		return
	}

	// Summary statistics
	fmt.Printf("Total Services: %d\n", sa.GetServiceCount())
	fmt.Printf("Available Services: %d\n", sa.GetAvailableServiceCount())
	fmt.Printf("Unavailable Services: %d\n", sa.GetUnavailableServiceCount())

	// Available services
	fmt.Println("\n📱 AVAILABLE SERVICES:")

	availableServices := sa.GetAvailableServices()
	if len(availableServices) == 0 {
		fmt.Println("  None")
	} else {
		for _, service := range availableServices {
			fmt.Printf("  ✅ %s\n", formatServiceName(service.Type))
		}
	}

	// Unavailable services
	fmt.Println("\n❌ UNAVAILABLE SERVICES:")

	unavailableServices := sa.GetUnavailableServices()
	if len(unavailableServices) == 0 {
		fmt.Println("  None")
	} else {
		for _, service := range unavailableServices {
			reason := ""
			if service.Reason != "" {
				reason = fmt.Sprintf(" (%s)", service.Reason)
			}

			fmt.Printf("  ❌ %s%s\n", formatServiceName(service.Type), reason)
		}
	}

	// Category breakdowns
	displayServiceCategories(sa)

	// Quick status checks
	displayQuickStatusChecks(sa)
}

func displayServiceCategories(sa *models.ServiceAvailability) {
	fmt.Println("\n🎵 STREAMING SERVICES:")

	streamingServices := sa.GetStreamingServices()
	availableCount := 0

	for _, service := range streamingServices {
		status := "❌"
		if service.IsAvailable {
			status = "✅"
			availableCount++
		}

		fmt.Printf("  %s %s\n", status, formatServiceName(service.Type))
	}

	fmt.Printf("  Summary: %d/%d streaming services available\n", availableCount, len(streamingServices))

	fmt.Println("\n🔗 LOCAL INPUT SERVICES:")

	localServices := sa.GetLocalServices()
	localAvailableCount := 0

	for _, service := range localServices {
		status := "❌"
		if service.IsAvailable {
			status = "✅"
			localAvailableCount++
		}

		fmt.Printf("  %s %s\n", status, formatServiceName(service.Type))
	}

	fmt.Printf("  Summary: %d/%d local services available\n", localAvailableCount, len(localServices))
}

func displayQuickStatusChecks(sa *models.ServiceAvailability) {
	fmt.Println("\n⚡ QUICK STATUS CHECKS:")

	checks := []struct {
		name  string
		check func() bool
		icon  string
	}{
		{"Spotify Ready", sa.HasSpotify, "🎵"},
		{"Bluetooth Ready", sa.HasBluetooth, "🔵"},
		{"AirPlay Ready", sa.HasAirPlay, "📡"},
		{"Alexa Ready", sa.HasAlexa, "🗣️"},
		{"TuneIn Ready", sa.HasTuneIn, "📻"},
		{"Pandora Ready", sa.HasPandora, "🎼"},
		{"Local Music Ready", sa.HasLocalMusic, "💾"},
	}

	for _, check := range checks {
		status := "❌ Not Available"
		if check.check() {
			status = "✅ Available"
		}

		fmt.Printf("  %s %s: %s\n", check.icon, check.name, status)
	}
}

func demonstrateUserFeedback(sa *models.ServiceAvailability, soundtouchClient *client.Client) {
	fmt.Println("\n1. SMART MUSIC SOURCE RECOMMENDATIONS:")
	recommendMusicSources(sa)

	fmt.Println("\n2. TROUBLESHOOTING UNAVAILABLE SERVICES:")
	provideTroubleshootingInfo(sa)

	fmt.Println("\n3. COMPARISON WITH CONFIGURED SOURCES:")
	compareWithConfiguredSources(sa, soundtouchClient)
}

func recommendMusicSources(sa *models.ServiceAvailability) {
	if sa.HasSpotify() {
		fmt.Println("  🎵 Spotify is available - you can stream from your Spotify account")
	}

	if sa.HasBluetooth() {
		fmt.Println("  🔵 Bluetooth is available - you can pair your phone or device")
	} else {
		fmt.Println("  🔵 Bluetooth is not available - check if Bluetooth is enabled on your device")
	}

	if sa.HasAirPlay() {
		fmt.Println("  📡 AirPlay is available - you can stream from Apple devices")
	}

	if sa.HasTuneIn() {
		fmt.Println("  📻 TuneIn Radio is available - you can listen to internet radio stations")
	}

	if sa.HasLocalMusic() {
		fmt.Println("  💾 Local Music is available - you can access music from network storage")
	}

	// Suggest alternatives if main services are unavailable
	if !sa.HasSpotify() && !sa.HasBluetooth() && sa.HasTuneIn() {
		fmt.Println("  💡 Consider using TuneIn Radio as an alternative music source")
	}
}

func provideTroubleshootingInfo(sa *models.ServiceAvailability) {
	unavailableServices := sa.GetUnavailableServices()

	for _, service := range unavailableServices {
		switch service.Type {
		case "BLUETOOTH":
			fmt.Printf("  🔵 Bluetooth: %s\n", getTroubleshootingTip("BLUETOOTH", service.Reason))
		case "SPOTIFY":
			fmt.Printf("  🎵 Spotify: %s\n", getTroubleshootingTip("SPOTIFY", service.Reason))
		case "ALEXA":
			fmt.Printf("  🗣️ Alexa: %s\n", getTroubleshootingTip("ALEXA", service.Reason))
		case "AIRPLAY":
			fmt.Printf("  📡 AirPlay: %s\n", getTroubleshootingTip("AIRPLAY", service.Reason))
		}
	}
}

func getTroubleshootingTip(serviceType, reason string) string {
	switch serviceType {
	case "BLUETOOTH":
		if reason == "INVALID_SOURCE_TYPE" {
			return "This device may not support Bluetooth audio input"
		}

		return "Check if Bluetooth is enabled and try restarting the device"
	case "SPOTIFY":
		return "Ensure you have a Spotify Premium account and are logged in"
	case "ALEXA":
		return "Check if Amazon Alexa is properly set up and connected"
	case "AIRPLAY":
		return "Ensure your Apple device and SoundTouch are on the same network"
	default:
		if reason != "" {
			return fmt.Sprintf("Reason: %s", reason)
		}

		return "Service is currently unavailable"
	}
}

func compareWithConfiguredSources(sa *models.ServiceAvailability, soundtouchClient *client.Client) {
	sources, err := soundtouchClient.GetSources()
	if err != nil {
		fmt.Printf("  ❌ Could not retrieve configured sources: %v\n", err)
		return
	}

	fmt.Println("  Comparing service availability with configured sources:")

	// Check Spotify
	spotifyAvailable := sa.HasSpotify()
	spotifyConfigured := sources.HasSpotify()
	fmt.Printf("  🎵 Spotify - Available: %v, Configured: %v\n", spotifyAvailable, spotifyConfigured)

	if spotifyAvailable && !spotifyConfigured {
		fmt.Println("      💡 Spotify is available but not configured - you may need to sign in")
	}

	// Check Bluetooth
	bluetoothAvailable := sa.HasBluetooth()
	bluetoothConfigured := sources.HasBluetooth()
	fmt.Printf("  🔵 Bluetooth - Available: %v, Configured: %v\n", bluetoothAvailable, bluetoothConfigured)

	if bluetoothAvailable && !bluetoothConfigured {
		fmt.Println("      💡 Bluetooth is available but not configured - try pairing a device")
	}

	fmt.Printf("\n  📊 Total configured sources: %d\n", sources.GetSourceCount())
	fmt.Printf("  📊 Ready configured sources: %d\n", sources.GetReadySourceCount())
}

func formatServiceName(serviceType string) string {
	switch serviceType {
	case "SPOTIFY":
		return "Spotify"
	case "BLUETOOTH":
		return "Bluetooth"
	case "AIRPLAY":
		return "AirPlay"
	case "ALEXA":
		return "Amazon Alexa"
	case "AMAZON":
		return "Amazon Music"
	case "PANDORA":
		return "Pandora"
	case "TUNEIN":
		return "TuneIn Radio"
	case "DEEZER":
		return "Deezer"
	case "IHEART":
		return "iHeartRadio"
	case "LOCAL_INTERNET_RADIO":
		return "Internet Radio"
	case "LOCAL_MUSIC":
		return "Local Music Library"
	case "BMX":
		return "BMX"
	case "NOTIFICATION":
		return "Notifications"
	default:
		return serviceType
	}
}
