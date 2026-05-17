package client

import (
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestGetServiceAvailability_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a real SoundTouch device
	// Set the SOUNDTOUCH_HOST environment variable to run this test
	// Example: SOUNDTOUCH_HOST=192.0.2.100 go test -v -run TestGetServiceAvailability_Integration
	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	client := NewClientFromHost(host)

	t.Run("get service availability", func(t *testing.T) {
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("Failed to get service availability: %v", err)
		}

		if serviceAvailability == nil {
			t.Fatal("Service availability response is nil")
		}

		if serviceAvailability.Services == nil {
			t.Fatal("Services list is nil")
		}

		t.Logf("Total services: %d", serviceAvailability.GetServiceCount())
		t.Logf("Available services: %d", serviceAvailability.GetAvailableServiceCount())
		t.Logf("Unavailable services: %d", serviceAvailability.GetUnavailableServiceCount())

		// Log all services and their availability
		if serviceAvailability.Services != nil {
			for _, service := range serviceAvailability.Services.Service {
				status := "available"
				if !service.IsAvailable {
					status = "unavailable"
					if service.Reason != "" {
						status += " (" + service.Reason + ")"
					}
				}

				t.Logf("Service %s: %s", service.Type, status)
			}
		}

		// Test convenience methods
		t.Logf("Has Spotify: %v", serviceAvailability.HasSpotify())
		t.Logf("Has Bluetooth: %v", serviceAvailability.HasBluetooth())
		t.Logf("Has AirPlay: %v", serviceAvailability.HasAirPlay())
		t.Logf("Has Alexa: %v", serviceAvailability.HasAlexa())
		t.Logf("Has TuneIn: %v", serviceAvailability.HasTuneIn())
		t.Logf("Has Pandora: %v", serviceAvailability.HasPandora())
		t.Logf("Has Local Music: %v", serviceAvailability.HasLocalMusic())

		// Test service categorization
		streamingServices := serviceAvailability.GetStreamingServices()
		t.Logf("Streaming services count: %d", len(streamingServices))

		for _, service := range streamingServices {
			t.Logf("  - Streaming: %s (%v)", service.Type, service.IsAvailable)
		}

		localServices := serviceAvailability.GetLocalServices()
		t.Logf("Local services count: %d", len(localServices))

		for _, service := range localServices {
			t.Logf("  - Local: %s (%v)", service.Type, service.IsAvailable)
		}

		// Validate that we have at least some services
		if serviceAvailability.GetServiceCount() == 0 {
			t.Error("Expected at least one service in the response")
		}
	})

	t.Run("compare with sources endpoint", func(t *testing.T) {
		// Get service availability
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("Failed to get service availability: %v", err)
		}

		// Get sources for comparison
		sources, err := client.GetSources()
		if err != nil {
			t.Fatalf("Failed to get sources: %v", err)
		}

		t.Logf("Comparing service availability with sources endpoint...")

		// Compare Spotify availability
		spotifyAvailable := serviceAvailability.HasSpotify()
		spotifyInSources := sources.HasSpotify()
		t.Logf("Spotify - ServiceAvailability: %v, Sources: %v", spotifyAvailable, spotifyInSources)

		// Compare Bluetooth availability
		bluetoothAvailable := serviceAvailability.HasBluetooth()
		bluetoothInSources := sources.HasBluetooth()
		t.Logf("Bluetooth - ServiceAvailability: %v, Sources: %v", bluetoothAvailable, bluetoothInSources)

		// Compare AUX availability (not directly comparable but useful info)
		auxInSources := sources.HasAux()
		t.Logf("AUX in Sources: %v (no direct equivalent in ServiceAvailability)", auxInSources)

		// Note: ServiceAvailability and Sources may not always match perfectly
		// ServiceAvailability shows what services are theoretically available
		// Sources shows what sources are currently configured and ready
		t.Logf("Note: ServiceAvailability shows theoretical availability, Sources shows current configuration")
	})

	t.Run("validate specific service details", func(t *testing.T) {
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("Failed to get service availability: %v", err)
		}

		// Test getting specific services
		spotifyService := serviceAvailability.GetServiceByType(models.ServiceTypeSpotify)
		if spotifyService != nil {
			t.Logf("Spotify service details: Available=%v, Reason=%s",
				spotifyService.IsAvailable, spotifyService.Reason)

			if !spotifyService.IsType(models.ServiceTypeSpotify) {
				t.Error("Spotify service type check failed")
			}
		} else {
			t.Log("Spotify service not found in response")
		}

		bluetoothService := serviceAvailability.GetServiceByType(models.ServiceTypeBluetooth)
		if bluetoothService != nil {
			t.Logf("Bluetooth service details: Available=%v, Reason=%s",
				bluetoothService.IsAvailable, bluetoothService.Reason)
		} else {
			t.Log("Bluetooth service not found in response")
		}

		// Check for services that commonly have reasons when unavailable
		unavailableServices := serviceAvailability.GetUnavailableServices()
		for _, service := range unavailableServices {
			if service.Reason != "" {
				t.Logf("Service %s is unavailable: %s", service.Type, service.Reason)
			} else {
				t.Logf("Service %s is unavailable (no reason provided)", service.Type)
			}
		}
	})
}

func TestGetServiceAvailability_UserFeedback(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	client := NewClientFromHost(host)

	t.Run("generate user feedback about supported services", func(t *testing.T) {
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("Failed to get service availability: %v", err)
		}

		// Example of how this could be used for user feedback
		t.Log("\n=== SERVICE AVAILABILITY REPORT ===")

		availableServices := serviceAvailability.GetAvailableServices()
		if len(availableServices) > 0 {
			t.Log("\nAvailable Services:")

			for _, service := range availableServices {
				t.Logf("  ✅ %s", formatServiceName(service.Type))
			}
		}

		unavailableServices := serviceAvailability.GetUnavailableServices()
		if len(unavailableServices) > 0 {
			t.Log("\nUnavailable Services:")

			for _, service := range unavailableServices {
				reason := ""
				if service.Reason != "" {
					reason = " - " + service.Reason
				}

				t.Logf("  ❌ %s%s", formatServiceName(service.Type), reason)
			}
		}

		// Streaming services summary
		streamingServices := serviceAvailability.GetStreamingServices()
		availableStreaming := 0

		for _, service := range streamingServices {
			if service.IsAvailable {
				availableStreaming++
			}
		}

		t.Logf("\nStreaming Services: %d/%d available", availableStreaming, len(streamingServices))

		// Local services summary
		localServices := serviceAvailability.GetLocalServices()
		availableLocal := 0

		for _, service := range localServices {
			if service.IsAvailable {
				availableLocal++
			}
		}

		t.Logf("Local Input Services: %d/%d available", availableLocal, len(localServices))

		t.Log("\n=== END REPORT ===")
	})
}

// formatServiceName converts service type constants to user-friendly names
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
