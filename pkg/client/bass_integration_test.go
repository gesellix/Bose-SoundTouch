package client

import (
	"os"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/speaker"
)

// Integration tests for bass control functionality
// These tests require a real SoundTouch device for validation
// Set SOUNDTOUCH_TEST_HOST environment variable to run these tests

func TestClient_Bass_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseBassHostPort(host)

	config := &Config{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Bass-Integration-Test/1.0",
	}

	client := NewClient(config)

	t.Run("GetBass", func(t *testing.T) {
		t.Logf("Testing GetBass on %s:%d", finalHost, finalPort)

		bass, err := client.GetBass()
		if err != nil {
			t.Errorf("Failed to get bass: %v", err)
			return
		}

		t.Logf("✓ Current bass level: %d (%s)", bass.GetLevel(), bass.String())
		t.Logf("✓ Bass category: %s", models.GetBassLevelCategory(bass.GetLevel()))
		t.Logf("✓ Target vs Actual: %d vs %d", bass.TargetBass, bass.ActualBass)

		// Validate bass level is within expected range
		if bass.GetLevel() < -9 || bass.GetLevel() > 9 {
			t.Errorf("Bass level %d is outside valid range [-9, 9]", bass.GetLevel())
		}

		// Device ID should be present
		if bass.DeviceID == "" {
			t.Error("Device ID should not be empty")
		}
	})

	t.Run("SetBass", func(t *testing.T) {
		t.Logf("Testing SetBass on %s:%d", finalHost, finalPort)

		// Get original bass level first
		originalBass, err := client.GetBass()
		if err != nil {
			t.Errorf("Failed to get original bass level: %v", err)
			return
		}

		t.Logf("Original bass level: %d", originalBass.GetLevel())

		// Try setting to 0 (neutral)
		err = client.SetBass(0)
		if err != nil {
			t.Errorf("Failed to set bass to 0: %v", err)
			return
		}

		t.Log("✓ SetBass(0) completed successfully")

		// Give device time to process
		time.Sleep(500 * time.Millisecond)

		// Verify the change (note: some devices may override this)
		newBass, err := client.GetBass()
		if err != nil {
			t.Errorf("Failed to get bass after setting: %v", err)
			return
		}

		t.Logf("Bass after setting to 0: %d", newBass.GetLevel())

		// Restore original bass level
		err = client.SetBass(originalBass.GetLevel())
		if err != nil {
			t.Logf("Warning: Failed to restore original bass level %d: %v", originalBass.GetLevel(), err)
		} else {
			t.Logf("✓ Restored original bass level: %d", originalBass.GetLevel())
		}
	})

	t.Run("SetBassWithValidation", func(t *testing.T) {
		t.Logf("Testing bass validation on %s:%d", finalHost, finalPort)

		// Test valid range boundaries
		validLevels := []int{-9, -5, 0, 5, 9}
		for _, level := range validLevels {
			err := client.SetBass(level)
			if err != nil {
				t.Errorf("SetBass(%d) should succeed, got error: %v", level, err)
			} else {
				t.Logf("✓ SetBass(%d) accepted", level)
			}

			time.Sleep(200 * time.Millisecond) // Brief pause between commands
		}

		// Test invalid levels (should fail validation before hitting device)
		invalidLevels := []int{-10, -100, 10, 100}
		for _, level := range invalidLevels {
			err := client.SetBass(level)
			if err == nil {
				t.Errorf("SetBass(%d) should fail validation, got nil error", level)
			} else {
				t.Logf("✓ SetBass(%d) correctly rejected: %v", level, err)
			}
		}
	})

	t.Run("SetBassSafe", func(t *testing.T) {
		t.Logf("Testing SetBassSafe (clamping) on %s:%d", finalHost, finalPort)

		// Test clamping behavior
		tests := []struct {
			input    int
			expected int
		}{
			{input: 15, expected: 9},   // Clamp high
			{input: -15, expected: -9}, // Clamp low
			{input: 5, expected: 5},    // No clamp needed
		}

		for _, test := range tests {
			err := client.SetBassSafe(test.input)
			if err != nil {
				t.Errorf("SetBassSafe(%d) failed: %v", test.input, err)
			} else {
				t.Logf("✓ SetBassSafe(%d) completed (should clamp to %d)", test.input, test.expected)
			}

			time.Sleep(200 * time.Millisecond)
		}
	})
}

func TestClient_Bass_IncrementDecrement_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseBassHostPort(host)

	config := &Config{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Bass-Integration-Test/1.0",
	}

	client := NewClient(config)

	// Get and store original bass level
	originalBass, err := client.GetBass()
	if err != nil {
		t.Fatalf("Failed to get original bass level: %v", err)
	}

	t.Logf("Original bass level: %d", originalBass.GetLevel())

	// Ensure we restore original level at the end
	defer func() {
		err := client.SetBass(originalBass.GetLevel())
		if err != nil {
			t.Logf("Warning: Failed to restore original bass level: %v", err)
		}
	}()

	t.Run("IncreaseBass", func(t *testing.T) {
		t.Logf("Testing IncreaseBass on %s:%d", finalHost, finalPort)

		// Set to known starting point
		err := client.SetBass(0)
		if err != nil {
			t.Errorf("Failed to set bass to starting point: %v", err)
			return
		}

		time.Sleep(300 * time.Millisecond)

		// Test increasing by 1
		bass, err := client.IncreaseBass(1)
		if err != nil {
			t.Errorf("IncreaseBass(1) failed: %v", err)
			return
		}

		t.Logf("✓ IncreaseBass(1) completed, result: %d (%s)", bass.GetLevel(), bass.String())

		// Test that result is returned correctly
		if bass == nil {
			t.Error("IncreaseBass should return non-nil bass result")
		}
	})

	t.Run("DecreaseBass", func(t *testing.T) {
		t.Logf("Testing DecreaseBass on %s:%d", finalHost, finalPort)

		// Set to known starting point
		err := client.SetBass(0)
		if err != nil {
			t.Errorf("Failed to set bass to starting point: %v", err)
			return
		}

		time.Sleep(300 * time.Millisecond)

		// Test decreasing by 1
		bass, err := client.DecreaseBass(1)
		if err != nil {
			t.Errorf("DecreaseBass(1) failed: %v", err)
			return
		}

		t.Logf("✓ DecreaseBass(1) completed, result: %d (%s)", bass.GetLevel(), bass.String())

		// Test that result is returned correctly
		if bass == nil {
			t.Error("DecreaseBass should return non-nil bass result")
		}
	})

	t.Run("BassClampingBehavior", func(t *testing.T) {
		t.Logf("Testing bass clamping behavior on %s:%d", finalHost, finalPort)

		// Test increase near maximum
		err := client.SetBass(8)
		if err != nil {
			t.Errorf("Failed to set bass to 8: %v", err)
			return
		}

		time.Sleep(300 * time.Millisecond)

		bass, err := client.IncreaseBass(3) // Should clamp to 9
		if err != nil {
			t.Errorf("IncreaseBass(3) from 8 failed: %v", err)
			return
		}

		t.Logf("✓ IncreaseBass(3) from 8 result: %d (should be clamped)", bass.GetLevel())

		// Test decrease near minimum
		err = client.SetBass(-8)
		if err != nil {
			t.Errorf("Failed to set bass to -8: %v", err)
			return
		}

		time.Sleep(300 * time.Millisecond)

		bass, err = client.DecreaseBass(3) // Should clamp to -9
		if err != nil {
			t.Errorf("DecreaseBass(3) from -8 failed: %v", err)
			return
		}

		t.Logf("✓ DecreaseBass(3) from -8 result: %d (should be clamped)", bass.GetLevel())
	})
}

func TestClient_Bass_ErrorHandling_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		t.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration tests")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseBassHostPort(host)

	config := &Config{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Bass-Integration-Test/1.0",
	}

	client := NewClient(config)

	t.Run("ValidationErrors", func(t *testing.T) {
		t.Logf("Testing bass validation errors on %s:%d", finalHost, finalPort)

		// Test out-of-range values
		invalidLevels := []int{-10, -100, 10, 50, 100}
		for _, level := range invalidLevels {
			err := client.SetBass(level)
			if err == nil {
				t.Errorf("SetBass(%d) should have failed validation", level)
			} else {
				t.Logf("✓ SetBass(%d) correctly failed: %v", level, err)
			}
		}
	})

	t.Run("IncrementDecrementErrors", func(t *testing.T) {
		t.Logf("Testing increment/decrement error conditions on %s:%d", finalHost, finalPort)

		// Test with very large increments (should be clamped, not error)
		_, err := client.IncreaseBass(100)
		if err != nil {
			t.Errorf("IncreaseBass(100) should clamp, not error: %v", err)
		} else {
			t.Log("✓ IncreaseBass(100) handled with clamping")
		}

		_, err = client.DecreaseBass(100)
		if err != nil {
			t.Errorf("DecreaseBass(100) should clamp, not error: %v", err)
		} else {
			t.Log("✓ DecreaseBass(100) handled with clamping")
		}
	})
}

// Benchmark bass control performance
func BenchmarkClient_Bass_Integration(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping integration benchmarks in short mode")
	}

	host := os.Getenv("SOUNDTOUCH_TEST_HOST")
	if host == "" {
		b.Skip("SOUNDTOUCH_TEST_HOST environment variable not set - skipping integration benchmarks")
	}

	// Parse host:port if provided
	finalHost, finalPort := parseBassHostPort(host)

	config := &Config{
		Host:      finalHost,
		Port:      finalPort,
		Timeout:   15 * time.Second,
		UserAgent: "Bose-SoundTouch-Go-Bass-Benchmark/1.0",
	}

	client := NewClient(config)

	// Get original bass level for restoration
	originalBass, err := client.GetBass()
	if err != nil {
		b.Fatalf("Failed to get original bass: %v", err)
	}

	// Restore original bass at the end
	defer func() {
		_ = client.SetBass(originalBass.GetLevel())
	}()

	b.Run("GetBass", func(b *testing.B) {
		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			_, err := client.GetBass()
			if err != nil {
				b.Fatalf("GetBass failed: %v", err)
			}
		}
	})

	b.Run("SetBass", func(b *testing.B) {
		bassLevels := []int{-3, 0, 3, -1, 1} // Cycle through different levels

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			level := bassLevels[i%len(bassLevels)]

			err := client.SetBass(level)
			if err != nil {
				b.Fatalf("SetBass(%d) failed: %v", level, err)
			}
		}
	})

	b.Run("IncreaseBass", func(b *testing.B) {
		// Set to a safe starting point
		_ = client.SetBass(-3)

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			// Keep increments small to avoid hitting limits
			_, err := client.IncreaseBass(1)
			if err != nil {
				b.Fatalf("IncreaseBass failed: %v", err)
			}
			// Reset to safe level periodically
			if i%3 == 0 {
				_ = client.SetBass(-3)
			}
		}
	})
}

// parseBassHostPort is a helper function for integration tests
// This is a simple version for test use
func parseBassHostPort(hostPort string) (string, int) {
	if !containsSubstring(hostPort, ":") {
		return hostPort, speaker.HTTPPort
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
		port := speaker.HTTPPort
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

	return hostPort, speaker.HTTPPort
}
