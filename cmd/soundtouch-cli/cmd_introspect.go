package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/urfave/cli/v2"
)

// introspectService handles getting introspect data for a specific service
func introspectService(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	source := strings.ToUpper(c.String("source"))
	sourceAccount := c.String("account")

	// Check service availability first
	checker := NewServiceAvailabilityChecker(client)
	if !checker.CheckSourceAvailable(source, fmt.Sprintf("get introspect data for %s", strings.ToLower(source))) {
		PrintWarning(fmt.Sprintf("Service %s may not be available, but continuing with introspect request...", source))
	}

	PrintDeviceHeader(fmt.Sprintf("Getting introspect data for %s", source), clientConfig.Host, clientConfig.Port)

	if sourceAccount != "" {
		fmt.Printf("Source Account: %s\n", sourceAccount)
	}

	fmt.Println()

	response, err := client.Introspect(source, sourceAccount)
	if err != nil {
		return fmt.Errorf("failed to get introspect data: %w", err)
	}

	// Print basic information
	fmt.Printf("=== %s Service Introspect Data ===\n", source)
	printIntrospectBasicInfo(response)

	// Print service state
	fmt.Printf("\n=== Service State ===\n")
	printIntrospectServiceState(response)

	// Print capabilities
	fmt.Printf("\n=== Service Capabilities ===\n")
	printIntrospectCapabilities(response)

	// Print history information
	if response.GetMaxHistorySize() > 0 {
		fmt.Printf("\n=== Content History ===\n")
		printIntrospectHistory(response)
	}

	// Print technical details
	if response.TokenLastChangedTimeSeconds > 0 || response.PlayStatusState != "" {
		fmt.Printf("\n=== Technical Details ===\n")
		printIntrospectTechnicalDetails(response)
	}

	return nil
}

// introspectSpotify handles getting Spotify introspect data using convenience method
func introspectSpotify(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	sourceAccount := c.String("account")

	// Check Spotify availability
	checker := NewServiceAvailabilityChecker(client)
	if !checker.ValidateSpotifyAvailable("get Spotify introspect data") {
		PrintWarning("Spotify may not be available, but continuing with introspect request...")
	}

	PrintDeviceHeader("Getting Spotify introspect data", clientConfig.Host, clientConfig.Port)

	if sourceAccount != "" {
		fmt.Printf("Spotify Account: %s\n", sourceAccount)
	}

	fmt.Println()

	response, err := client.IntrospectSpotify(sourceAccount)
	if err != nil {
		return fmt.Errorf("failed to get Spotify introspect data: %w", err)
	}

	// Print Spotify-specific information
	fmt.Printf("=== Spotify Service Introspect Data ===\n")
	printIntrospectBasicInfo(response)

	// Print service state with Spotify context
	fmt.Printf("\n=== Spotify Service State ===\n")
	printIntrospectServiceState(response)

	// Print Spotify capabilities
	fmt.Printf("\n=== Spotify Service Capabilities ===\n")
	printIntrospectCapabilities(response)

	// Show Spotify-specific recommendations
	if response.IsInactive() {
		fmt.Printf("\n💡 Spotify Setup Recommendations:\n")

		if !response.HasUser() {
			fmt.Printf("   • Sign in to your Spotify account on the device\n")
		}

		fmt.Printf("   • Use 'soundtouch-cli source select --source SPOTIFY' to activate Spotify\n")
		fmt.Printf("   • Ensure you have Spotify Premium for full functionality\n")
	}

	// Print history information
	if response.GetMaxHistorySize() > 0 {
		fmt.Printf("\n=== Spotify Content History ===\n")
		printIntrospectHistory(response)
	}

	// Print technical details
	if response.TokenLastChangedTimeSeconds > 0 || response.PlayStatusState != "" {
		fmt.Printf("\n=== Technical Details ===\n")
		printIntrospectTechnicalDetails(response)
	}

	return nil
}

// introspectAllServices handles getting introspect data for all available services
func introspectAllServices(c *cli.Context) error {
	clientConfig := GetClientConfig(c)

	client, err := CreateSoundTouchClient(clientConfig)
	if err != nil {
		return err
	}

	PrintDeviceHeader("Getting introspect data for all services", clientConfig.Host, clientConfig.Port)

	// Get service availability to know which services to check
	serviceAvailability, err := client.GetServiceAvailability()
	if err != nil {
		return fmt.Errorf("failed to get service availability: %w", err)
	}

	// Services to introspect (only streaming services that support introspect)
	servicesToCheck := []string{"SPOTIFY", "PANDORA", "TUNEIN", "AMAZON", "DEEZER"}

	successCount := 0
	failCount := 0

	for i, source := range servicesToCheck {
		if i > 0 {
			fmt.Println("\n" + strings.Repeat("─", 50))
		}

		// Check if service is available
		serviceType := sourceToServiceType(source)
		if serviceType != "" && !serviceAvailability.IsServiceAvailable(serviceType) {
			fmt.Printf("\n❌ %s: Service not available on this device\n", source)
			continue
		}

		fmt.Printf("\n🔍 Getting introspect data for %s...\n", source)

		response, err := client.Introspect(source, "")
		if err != nil {
			fmt.Printf("❌ %s: Failed to get introspect data - %v\n", source, err)

			failCount++

			continue
		}

		fmt.Printf("✅ %s: Successfully retrieved introspect data\n", source)
		printIntrospectSummary(source, response)

		successCount++
	}

	// Print summary
	fmt.Print("\n" + strings.Repeat("═", 50) + "\n")
	fmt.Printf("📊 Introspect Summary:\n")
	fmt.Printf("   ✅ Successful: %d services\n", successCount)
	fmt.Printf("   ❌ Failed: %d services\n", failCount)
	fmt.Printf("   📡 Total checked: %d services\n", len(servicesToCheck))

	if successCount > 0 {
		PrintSuccess(fmt.Sprintf("Successfully retrieved introspect data for %d services", successCount))
	}

	return nil
}

// printIntrospectBasicInfo prints basic introspect information
func printIntrospectBasicInfo(response *models.IntrospectResponse) {
	fmt.Printf("State: %s\n", response.State)

	if response.HasUser() {
		fmt.Printf("User: %s\n", response.User)
	}

	fmt.Printf("Currently Playing: %s\n", formatBooleanStatus(response.IsPlaying))

	if response.HasCurrentContent() {
		fmt.Printf("Current Content: %s\n", response.CurrentURI)
	}

	fmt.Printf("Shuffle Mode: %s\n", response.ShuffleMode)

	if response.HasSubscription() {
		fmt.Printf("Subscription Type: %s\n", response.SubscriptionType)
	}
}

// printIntrospectServiceState prints service state information
func printIntrospectServiceState(response *models.IntrospectResponse) {
	if response.IsActive() {
		fmt.Printf("✅ Service is ACTIVE\n")
	} else if response.IsInactive() {
		fmt.Printf("❌ Service is INACTIVE")

		if response.GetState() == models.IntrospectStateInactiveUnselected {
			fmt.Printf(" (Never been used)")
		}

		fmt.Println()
	}

	// Additional state information
	if response.IsPlaying {
		fmt.Printf("🎵 Currently playing content\n")
	} else {
		fmt.Printf("⏸️  Not currently playing\n")
	}

	if response.IsShuffleEnabled() {
		fmt.Printf("🔀 Shuffle mode is ON\n")
	} else {
		fmt.Printf("➡️  Shuffle mode is OFF\n")
	}
}

// printIntrospectCapabilities prints service capabilities
func printIntrospectCapabilities(response *models.IntrospectResponse) {
	capabilities := []struct {
		supported bool
		feature   string
		icon      string
	}{
		{response.SupportsSkipPrevious(), "Skip Previous", "⏮️"},
		{response.SupportsSeek(), "Seek within tracks", "🎯"},
		{response.SupportsResume(), "Resume playback", "▶️"},
	}

	for _, feature := range capabilities {
		status := "❌"
		if feature.supported {
			status = "✅"
		}

		fmt.Printf("%s %s %s\n", status, feature.icon, feature.feature)
	}

	// Data collection status
	if response.CollectsData() {
		fmt.Printf("📊 Data collection: ENABLED\n")
	} else {
		fmt.Printf("🚫 Data collection: DISABLED\n")
	}
}

// printIntrospectHistory prints content history information
func printIntrospectHistory(response *models.IntrospectResponse) {
	fmt.Printf("Max History Size: %d items\n", response.GetMaxHistorySize())
}

// printIntrospectTechnicalDetails prints technical details
func printIntrospectTechnicalDetails(response *models.IntrospectResponse) {
	if response.TokenLastChangedTimeSeconds > 0 {
		// Convert timestamp to readable format
		tokenTime := time.Unix(response.TokenLastChangedTimeSeconds, 0)
		fmt.Printf("Token Last Changed: %s\n", tokenTime.Format("2006-01-02 15:04:05 MST"))
		fmt.Printf("Token Timestamp: %d seconds since Unix epoch\n", response.TokenLastChangedTimeSeconds)

		if response.TokenLastChangedTimeMicroseconds > 0 {
			fmt.Printf("Token Microseconds: %d\n", response.TokenLastChangedTimeMicroseconds)
		}
	}

	if response.PlayStatusState != "" {
		fmt.Printf("Play Status State: %s\n", response.PlayStatusState)
	}

	fmt.Printf("Received Playback Request: %s\n", formatBooleanStatus(response.ReceivedPlaybackRequest))
}

// printIntrospectSummary prints a brief summary for the "all" command
func printIntrospectSummary(_ string, response *models.IntrospectResponse) {
	fmt.Printf("   State: %s", response.State)

	if response.HasUser() {
		fmt.Printf(" (User: %s)", response.User)
	}

	fmt.Println()

	fmt.Printf("   Playing: %s", formatBooleanStatus(response.IsPlaying))

	if response.HasCurrentContent() {
		fmt.Printf(" | Content: %.50s", response.CurrentURI)

		if len(response.CurrentURI) > 50 {
			fmt.Printf("...")
		}
	}

	fmt.Println()

	var capabilities []string
	if response.SupportsSkipPrevious() {
		capabilities = append(capabilities, "Skip")
	}

	if response.SupportsSeek() {
		capabilities = append(capabilities, "Seek")
	}

	if response.SupportsResume() {
		capabilities = append(capabilities, "Resume")
	}

	if len(capabilities) > 0 {
		fmt.Printf("   Capabilities: %s\n", strings.Join(capabilities, ", "))
	} else {
		fmt.Printf("   Capabilities: None\n")
	}
}

// formatBooleanStatus formats boolean values for display
func formatBooleanStatus(value bool) string {
	if value {
		return "✅ Yes"
	}

	return "❌ No"
}
