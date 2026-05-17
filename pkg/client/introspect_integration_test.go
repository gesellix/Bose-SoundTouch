package client

import (
	"os"
	"testing"
	"time"
)

func TestClient_Introspect_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host := os.Getenv("SOUNDTOUCH_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_HOST not set, skipping integration test")
	}

	config := &Config{
		Host:    host,
		Timeout: 10 * time.Second,
	}
	client := NewClient(config)

	// Test getting Spotify introspect data
	t.Run("spotify introspect", func(t *testing.T) {
		// First check if Spotify is available
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("failed to get service availability: %v", err)
		}

		if !serviceAvailability.HasSpotify() {
			t.Skip("Spotify not available on this device")
		}

		// Test introspect with empty source account (should still work)
		response, err := client.Introspect("SPOTIFY", "")
		if err != nil {
			t.Fatalf("failed to get Spotify introspect data: %v", err)
		}

		if response == nil {
			t.Fatal("expected response, got nil")
		}

		t.Logf("Spotify introspect state: %s", response.State)
		t.Logf("Spotify user: %s", response.User)
		t.Logf("Spotify is playing: %t", response.IsPlaying)
		t.Logf("Spotify shuffle mode: %s", response.ShuffleMode)
		t.Logf("Spotify current URI: %s", response.CurrentURI)
		t.Logf("Spotify subscription type: %s", response.SubscriptionType)

		// Test state methods
		if response.IsActive() {
			t.Log("Spotify service is active")
		} else if response.IsInactive() {
			t.Log("Spotify service is inactive")
		}

		// Test capabilities
		if response.SupportsSkipPrevious() {
			t.Log("Spotify supports skip previous")
		}

		if response.SupportsSeek() {
			t.Log("Spotify supports seek")
		}

		if response.SupportsResume() {
			t.Log("Spotify supports resume")
		}

		// Test history
		historySize := response.GetMaxHistorySize()
		if historySize > 0 {
			t.Logf("Spotify content history max size: %d", historySize)
		}
	})

	// Test the convenience method
	t.Run("spotify introspect convenience method", func(t *testing.T) {
		// First check if Spotify is available
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("failed to get service availability: %v", err)
		}

		if !serviceAvailability.HasSpotify() {
			t.Skip("Spotify not available on this device")
		}

		response, err := client.IntrospectSpotify("")
		if err != nil {
			t.Fatalf("failed to get Spotify introspect data using convenience method: %v", err)
		}

		if response == nil {
			t.Fatal("expected response from convenience method, got nil")
		}

		t.Logf("Convenience method - Spotify state: %s", response.State)
	})

	// Test introspect with other services if available
	t.Run("other services introspect", func(t *testing.T) {
		serviceAvailability, err := client.GetServiceAvailability()
		if err != nil {
			t.Fatalf("failed to get service availability: %v", err)
		}

		// Test Pandora if available
		if serviceAvailability.HasPandora() {
			t.Log("Testing Pandora introspect...")

			response, err := client.Introspect("PANDORA", "")
			if err != nil {
				t.Logf("Pandora introspect failed (expected for some configurations): %v", err)
			} else {
				t.Logf("Pandora introspect state: %s", response.State)
			}
		}

		// Test TuneIn if available
		if serviceAvailability.HasTuneIn() {
			t.Log("Testing TuneIn introspect...")

			response, err := client.Introspect("TUNEIN", "")
			if err != nil {
				t.Logf("TuneIn introspect failed (expected for some configurations): %v", err)
			} else {
				t.Logf("TuneIn introspect state: %s", response.State)
			}
		}
	})
}

func TestClient_Introspect_ErrorCases_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	host := os.Getenv("SOUNDTOUCH_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_HOST not set, skipping integration test")
	}

	config := &Config{
		Host:    host,
		Timeout: 5 * time.Second,
	}
	client := NewClient(config)

	// Test with invalid source
	t.Run("invalid source", func(t *testing.T) {
		response, err := client.Introspect("INVALID_SOURCE", "")
		if err == nil {
			t.Error("expected error for invalid source, got nil")
		}

		if response != nil {
			t.Error("expected nil response for invalid source, got non-nil")
		}

		t.Logf("Expected error for invalid source: %v", err)
	})

	// Test with empty source
	t.Run("empty source", func(t *testing.T) {
		response, err := client.Introspect("", "")
		if err == nil {
			t.Error("expected error for empty source, got nil")
		}

		if response != nil {
			t.Error("expected nil response for empty source, got non-nil")
		}
	})
}

// ExampleClient_Introspect demonstrates how to use the Introspect method
func ExampleClient_Introspect() {
	config := &Config{
		Host: "192.0.2.100",
		Port: 8090,
	}
	client := NewClient(config)

	// Get introspect data for Spotify
	response, err := client.Introspect("SPOTIFY", "")
	if err != nil {
		panic(err)
	}

	// Check service state
	if response.IsActive() {
		println("Spotify service is active")

		if response.IsPlaying {
			println("Currently playing:", response.CurrentURI)
		}
	} else {
		println("Spotify service is inactive")
	}

	// Check capabilities
	if response.SupportsSeek() {
		println("Seek is supported")
	}

	if response.SupportsSkipPrevious() {
		println("Skip previous is supported")
	}
}

// ExampleClient_IntrospectSpotify demonstrates the Spotify convenience method
func ExampleClient_IntrospectSpotify() {
	config := &Config{
		Host: "192.0.2.100",
		Port: 8090,
	}
	client := NewClient(config)

	// Get Spotify introspect data using convenience method
	response, err := client.IntrospectSpotify("")
	if err != nil {
		panic(err)
	}

	// Display user and subscription info
	if response.HasUser() {
		println("Spotify user:", response.User)
	}

	if response.HasSubscription() {
		println("Subscription type:", response.SubscriptionType)
	}

	// Check shuffle state
	if response.IsShuffleEnabled() {
		println("Shuffle is enabled")
	} else {
		println("Shuffle is disabled")
	}
}
