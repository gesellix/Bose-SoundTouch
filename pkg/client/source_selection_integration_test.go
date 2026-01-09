package client

import (
	"os"
	"testing"
	"time"
)

// Integration tests for source selection functionality
// These tests require a real SoundTouch device for validation
// Set SOUNDTOUCH_TEST_HOST environment variable to run these tests

func TestClient_SelectSource_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseHostPort(host, 8090)

	config := ClientConfig{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Integration-Test/1.0",
	}

	client := NewClient(config)

	// First, get available sources to know what we can test
	sources, err := client.GetSources()
	if err != nil {
		t.Fatalf("Failed to get sources: %v", err)
	}

	t.Logf("Found %d total sources, %d ready", sources.GetSourceCount(), sources.GetReadySourceCount())

	// Test source selection based on what's available
	tests := []struct {
		name          string
		method        func() error
		checkSource   func() bool
		description   string
		skipIfMissing bool
	}{
		{
			name: "Select Spotify",
			method: func() error {
				spotifySources := sources.GetReadySpotifySources()
				if len(spotifySources) == 0 {
					return nil // Skip if no Spotify available
				}
				// Use first available Spotify account
				return client.SelectSpotify(spotifySources[0].SourceAccount)
			},
			checkSource: func() bool {
				return sources.HasSpotify()
			},
			description:   "Spotify source selection",
			skipIfMissing: true,
		},
		{
			name: "Select TuneIn",
			method: func() error {
				tuneInSources := sources.GetSourcesByType("TUNEIN")
				for _, src := range tuneInSources {
					if src.Status.IsReady() {
						return client.SelectTuneIn(src.SourceAccount)
					}
				}
				return nil // Skip if no TuneIn available
			},
			checkSource: func() bool {
				return sources.HasSource("TUNEIN")
			},
			description:   "TuneIn source selection",
			skipIfMissing: true,
		},
		{
			name: "Select via generic method",
			method: func() error {
				// Find any ready streaming source
				for _, source := range sources.GetAvailableSources() {
					if source.IsStreamingService() {
						return client.SelectSource(source.Source, source.SourceAccount)
					}
				}
				return nil // Skip if no streaming sources available
			},
			checkSource: func() bool {
				streaming := sources.GetStreamingSources()
				for _, src := range streaming {
					if src.Status.IsReady() {
						return true
					}
				}
				return false
			},
			description:   "Generic source selection",
			skipIfMissing: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipIfMissing && !tt.checkSource() {
				t.Skipf("Skipping %s - source not available on test device", tt.description)
			}

			t.Logf("Testing %s on %s:%d", tt.description, finalHost, finalPort)

			err := tt.method()
			if err != nil {
				t.Errorf("Failed to execute %s: %v", tt.description, err)
				return
			}

			t.Logf("✓ %s completed successfully", tt.description)

			// Give the device a moment to process the change
			time.Sleep(500 * time.Millisecond)
		})
	}
}

func TestClient_SelectSourceFromItem_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseHostPort(host, 8090)

	config := ClientConfig{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Integration-Test/1.0",
	}

	client := NewClient(config)

	// Get available sources
	sources, err := client.GetSources()
	if err != nil {
		t.Fatalf("Failed to get sources: %v", err)
	}

	// Test selecting from available source items
	availableSources := sources.GetAvailableSources()
	if len(availableSources) == 0 {
		t.Skip("No available sources to test with")
	}

	// Test with the first available source
	testSource := availableSources[0]
	t.Logf("Testing SelectSourceFromItem with source: %s (account: %s)",
		testSource.Source, testSource.SourceAccount)

	err = client.SelectSourceFromItem(&testSource)
	if err != nil {
		t.Errorf("Failed to select source from item: %v", err)
		return
	}

	t.Logf("✓ SelectSourceFromItem completed successfully")
}

func TestClient_SelectSource_ErrorHandling_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseHostPort(host, 8090)

	config := ClientConfig{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Integration-Test/1.0",
	}

	client := NewClient(config)

	// Test with invalid source
	t.Run("Invalid source", func(t *testing.T) {
		err := client.SelectSource("INVALID_SOURCE", "")
		if err == nil {
			t.Error("Expected error for invalid source, got nil")
		} else {
			t.Logf("✓ Got expected error for invalid source: %v", err)
		}
	})

	// Test with empty source (should fail validation)
	t.Run("Empty source", func(t *testing.T) {
		err := client.SelectSource("", "")
		if err == nil {
			t.Error("Expected error for empty source, got nil")
		} else if err.Error() != "source cannot be empty" {
			t.Errorf("Expected 'source cannot be empty' error, got: %v", err)
		} else {
			t.Logf("✓ Got expected validation error: %v", err)
		}
	})

	// Test with nil source item
	t.Run("Nil source item", func(t *testing.T) {
		err := client.SelectSourceFromItem(nil)
		if err == nil {
			t.Error("Expected error for nil source item, got nil")
		} else if err.Error() != "sourceItem cannot be nil" {
			t.Errorf("Expected 'sourceItem cannot be nil' error, got: %v", err)
		} else {
			t.Logf("✓ Got expected validation error: %v", err)
		}
	})
}

func TestClient_ConvenienceSourceMethods_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseHostPort(host, 8090)

	config := ClientConfig{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Integration-Test/1.0",
	}

	client := NewClient(config)

	// Get available sources to determine what we can test
	sources, err := client.GetSources()
	if err != nil {
		t.Fatalf("Failed to get sources: %v", err)
	}

	// Test convenience methods based on availability
	if sources.HasSpotify() {
		t.Run("SelectSpotify", func(t *testing.T) {
			spotifySources := sources.GetReadySpotifySources()
			if len(spotifySources) > 0 {
				err := client.SelectSpotify(spotifySources[0].SourceAccount)
				if err != nil {
					t.Errorf("SelectSpotify failed: %v", err)
				} else {
					t.Log("✓ SelectSpotify succeeded")
				}
			}
		})
	} else {
		t.Log("Spotify not available - skipping SelectSpotify test")
	}

	if sources.HasBluetooth() {
		t.Run("SelectBluetooth", func(t *testing.T) {
			err := client.SelectBluetooth()
			if err != nil {
				t.Errorf("SelectBluetooth failed: %v", err)
			} else {
				t.Log("✓ SelectBluetooth succeeded")
			}
		})
	} else {
		t.Log("Bluetooth not available - skipping SelectBluetooth test")
	}

	if sources.HasSource("TUNEIN") {
		t.Run("SelectTuneIn", func(t *testing.T) {
			tuneInSources := sources.GetSourcesByType("TUNEIN")
			for _, src := range tuneInSources {
				if src.Status.IsReady() {
					err := client.SelectTuneIn(src.SourceAccount)
					if err != nil {
						t.Errorf("SelectTuneIn failed: %v", err)
					} else {
						t.Log("✓ SelectTuneIn succeeded")
					}
					break
				}
			}
		})
	} else {
		t.Log("TuneIn not available - skipping SelectTuneIn test")
	}

	if sources.HasSource("PANDORA") {
		t.Run("SelectPandora", func(t *testing.T) {
			pandoraSources := sources.GetSourcesByType("PANDORA")
			for _, src := range pandoraSources {
				if src.Status.IsReady() {
					err := client.SelectPandora(src.SourceAccount)
					if err != nil {
						t.Errorf("SelectPandora failed: %v", err)
					} else {
						t.Log("✓ SelectPandora succeeded")
					}
					break
				}
			}
		})
	} else {
		t.Log("Pandora not available - skipping SelectPandora test")
	}
}

// Benchmark source selection performance
func BenchmarkClient_SelectSource_Integration(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping integration benchmarks in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		b.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration benchmarks")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseHostPort(host, 8090)

	config := ClientConfig{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Benchmark-Test/1.0",
	}

	client := NewClient(config)

	// Get available sources
	sources, err := client.GetSources()
	if err != nil {
		b.Fatalf("Failed to get sources: %v", err)
	}

	availableSources := sources.GetAvailableSources()
	if len(availableSources) == 0 {
		b.Skip("No available sources to benchmark with")
	}

	// Use first available source for benchmarking
	testSource := availableSources[0]

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := client.SelectSource(testSource.Source, testSource.SourceAccount)
		if err != nil {
			b.Fatalf("SelectSource failed: %v", err)
		}
	}
}

// parseHostPort is a helper function for integration tests
// This is a simple version for test use
func parseHostPort(hostPort string, defaultPort int) (string, int) {
	if !containsSubstring(hostPort, ":") {
		return hostPort, defaultPort
	}

	// Simple parsing - in real use, we'd use net.SplitHostPort
	parts := make([]string, 0, 2)
	current := ""
	for _, char := range hostPort {
		if char == ':' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}

	if len(parts) == 2 {
		// Try to parse port
		port := defaultPort
		portStr := parts[1]
		portInt := 0
		for _, char := range portStr {
			if char >= '0' && char <= '9' {
				portInt = portInt*10 + int(char-'0')
			} else {
				portInt = -1
				break
			}
		}
		if portInt > 0 && portInt <= 65535 {
			port = portInt
		}
		return parts[0], port
	}

	return hostPort, defaultPort
}
