package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestIntrospectCommands(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedOutput []string
		expectError    bool
	}{
		{
			name: "introspect service with source flag",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "source", "introspect", "--source", "SPOTIFY"},
			expectedOutput: []string{
				"Getting introspect data for SPOTIFY",
				"=== SPOTIFY Service Introspect Data ===",
				"State: Active",
				"User: test_user",
				"Currently Playing: ✅ Yes",
				"Current Content: spotify://track/123",
				"Shuffle Mode: ON",
				"Subscription Type: Premium",
				"=== Service State ===",
				"✅ Service is ACTIVE",
				"🎵 Currently playing content",
				"🔀 Shuffle mode is ON",
				"=== Service Capabilities ===",
				"✅ ⏮️ Skip Previous",
				"✅ 🎯 Seek within tracks",
				"✅ ▶️ Resume playback",
				"🚫 Data collection: DISABLED",
				"=== Spotify Content History ===",
				"Max History Size: 15 items",
				"=== Technical Details ===",
				"Token Last Changed:",
				"Token Timestamp: 1702566495",
				"Play Status State: 2",
				"Received Playback Request: ❌ No",
			},
		},
		{
			name: "introspect spotify convenience command",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "source", "introspect-spotify"},
			expectedOutput: []string{
				"Getting Spotify introspect data",
				"=== Spotify Service Introspect Data ===",
				"State: Active",
				"User: test_user",
				"=== Spotify Service State ===",
				"✅ Service is ACTIVE",
				"=== Spotify Service Capabilities ===",
			},
		},
		{
			name: "introspect with account parameter",
			args: []string{"soundtouch-cli", "--host", "192.0.2.100", "source", "introspect", "--source", "SPOTIFY", "--account", "my_spotify_account"},
			expectedOutput: []string{
				"Getting introspect data for SPOTIFY",
				"Source Account: my_spotify_account",
			},
		},
		{
			name:        "introspect missing source flag",
			args:        []string{"soundtouch-cli", "--host", "192.0.2.100", "source", "introspect"},
			expectError: true,
		},
		{
			name:        "introspect missing host",
			args:        []string{"soundtouch-cli", "source", "introspect", "--source", "SPOTIFY"},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip actual execution for now - these would need mock HTTP servers
			// This test structure shows how the CLI commands would be tested
			t.Skip("Integration test - requires mock HTTP server setup")

			// Example of how you would set up the test:
			// app := createTestApp()
			//
			// var buf bytes.Buffer
			// app.Writer = &buf
			// app.ErrWriter = &buf
			//
			// err := app.Run(tt.args)
			//
			// if tt.expectError {
			//     if err == nil {
			//         t.Error("expected error, got nil")
			//     }
			//     return
			// }
			//
			// if err != nil {
			//     t.Fatalf("unexpected error: %v", err)
			// }
			//
			// output := buf.String()
			// for _, expected := range tt.expectedOutput {
			//     if !strings.Contains(output, expected) {
			//         t.Errorf("expected output to contain %q, got:\n%s", expected, output)
			//     }
			// }
		})
	}
}

func TestPrintIntrospectBasicInfo(t *testing.T) {
	tests := []struct {
		name     string
		response *models.IntrospectResponse
		expected []string
	}{
		{
			name: "active spotify response",
			response: &models.IntrospectResponse{
				State:            "Active",
				User:             "test_user",
				IsPlaying:        true,
				ShuffleMode:      "ON",
				CurrentURI:       "spotify://track/123",
				SubscriptionType: "Premium",
			},
			expected: []string{
				"State: Active",
				"User: test_user",
				"Currently Playing: ✅ Yes",
				"Current Content: spotify://track/123",
				"Shuffle Mode: ON",
				"Subscription Type: Premium",
			},
		},
		{
			name: "inactive response",
			response: &models.IntrospectResponse{
				State:       "InactiveUnselected",
				User:        "",
				IsPlaying:   false,
				ShuffleMode: "OFF",
				CurrentURI:  "",
			},
			expected: []string{
				"State: InactiveUnselected",
				"Currently Playing: ❌ No",
				"Shuffle Mode: OFF",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printIntrospectBasicInfo(tt.response)

			// Restore stdout and read output
			w.Close()

			os.Stdout = oldStdout

			var buf bytes.Buffer

			_, err := buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("failed to read output: %v", err)
			}

			output := buf.String()

			// Check expected strings are present
			for _, expected := range tt.expected {
				if !containsSubstring(output, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}

			// Check unwanted strings are not present
			if tt.response.User == "" && containsSubstring(output, "User:") {
				t.Error("expected no user information when user is empty")
			}

			if tt.response.CurrentURI == "" && containsSubstring(output, "Current Content:") {
				t.Error("expected no current content when URI is empty")
			}

			if tt.response.SubscriptionType == "" && containsSubstring(output, "Subscription Type:") {
				t.Error("expected no subscription information when type is empty")
			}
		})
	}
}

func TestPrintIntrospectServiceState(t *testing.T) {
	tests := []struct {
		name     string
		response *models.IntrospectResponse
		expected []string
	}{
		{
			name: "active playing with shuffle",
			response: &models.IntrospectResponse{
				State:       "Active",
				IsPlaying:   true,
				ShuffleMode: "ON",
			},
			expected: []string{
				"✅ Service is ACTIVE",
				"🎵 Currently playing content",
				"🔀 Shuffle mode is ON",
			},
		},
		{
			name: "inactive unselected",
			response: &models.IntrospectResponse{
				State:       "InactiveUnselected",
				IsPlaying:   false,
				ShuffleMode: "OFF",
			},
			expected: []string{
				"❌ Service is INACTIVE (Never been used)",
				"⏸️  Not currently playing",
				"➡️  Shuffle mode is OFF",
			},
		},
		{
			name: "inactive but configured",
			response: &models.IntrospectResponse{
				State:       "Inactive",
				IsPlaying:   false,
				ShuffleMode: "OFF",
			},
			expected: []string{
				"❌ Service is INACTIVE",
				"⏸️  Not currently playing",
				"➡️  Shuffle mode is OFF",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printIntrospectServiceState(tt.response)

			// Restore stdout and read output
			w.Close()

			os.Stdout = oldStdout

			var buf bytes.Buffer

			_, err := buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("failed to read output: %v", err)
			}

			output := buf.String()

			// Check expected strings are present
			for _, expected := range tt.expected {
				if !containsSubstring(output, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

func TestPrintIntrospectCapabilities(t *testing.T) {
	tests := []struct {
		name     string
		response *models.IntrospectResponse
		expected []string
	}{
		{
			name: "full capabilities enabled",
			response: &models.IntrospectResponse{
				NowPlaying: &models.IntrospectNowPlaying{
					SkipPreviousSupported: true,
					SeekSupported:         true,
					ResumeSupported:       true,
					CollectData:           true,
				},
			},
			expected: []string{
				"✅ ⏮️ Skip Previous",
				"✅ 🎯 Seek within tracks",
				"✅ ▶️ Resume playback",
				"📊 Data collection: ENABLED",
			},
		},
		{
			name: "limited capabilities",
			response: &models.IntrospectResponse{
				NowPlaying: &models.IntrospectNowPlaying{
					SkipPreviousSupported: false,
					SeekSupported:         false,
					ResumeSupported:       true,
					CollectData:           false,
				},
			},
			expected: []string{
				"❌ ⏮️ Skip Previous",
				"❌ 🎯 Seek within tracks",
				"✅ ▶️ Resume playback",
				"🚫 Data collection: DISABLED",
			},
		},
		{
			name: "no capabilities info",
			response: &models.IntrospectResponse{
				NowPlaying: nil,
			},
			expected: []string{
				"❌ ⏮️ Skip Previous",
				"❌ 🎯 Seek within tracks",
				"❌ ▶️ Resume playback",
				"🚫 Data collection: DISABLED",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printIntrospectCapabilities(tt.response)

			// Restore stdout and read output
			w.Close()

			os.Stdout = oldStdout

			var buf bytes.Buffer

			_, err := buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("failed to read output: %v", err)
			}

			output := buf.String()

			// Check expected strings are present
			for _, expected := range tt.expected {
				if !containsSubstring(output, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

func TestPrintIntrospectSummary(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		response *models.IntrospectResponse
		expected []string
	}{
		{
			name:   "full spotify summary",
			source: "SPOTIFY",
			response: &models.IntrospectResponse{
				State:      "Active",
				User:       "spotify_user",
				IsPlaying:  true,
				CurrentURI: "spotify://track/very_long_track_uri_that_should_be_truncated_because_its_too_long_for_display",
				NowPlaying: &models.IntrospectNowPlaying{
					SkipPreviousSupported: true,
					SeekSupported:         true,
					ResumeSupported:       true,
				},
			},
			expected: []string{
				"State: Active (User: spotify_user)",
				"Playing: ✅ Yes | Content: spotify://track/very_long_track_uri_that_should_be...",
				"Capabilities: Skip, Seek, Resume",
			},
		},
		{
			name:   "minimal summary",
			source: "PANDORA",
			response: &models.IntrospectResponse{
				State:     "Inactive",
				IsPlaying: false,
			},
			expected: []string{
				"State: Inactive",
				"Playing: ❌ No",
				"Capabilities: None",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Call the function
			printIntrospectSummary(tt.source, tt.response)

			// Restore stdout and read output
			w.Close()

			os.Stdout = oldStdout

			var buf bytes.Buffer

			_, err := buf.ReadFrom(r)
			if err != nil {
				t.Fatalf("failed to read output: %v", err)
			}

			output := buf.String()

			// Check expected strings are present
			for _, expected := range tt.expected {
				if !containsSubstring(output, expected) {
					t.Errorf("expected output to contain %q, got:\n%s", expected, output)
				}
			}
		})
	}
}

func TestFormatBooleanStatus(t *testing.T) {
	tests := []struct {
		name     string
		value    bool
		expected string
	}{
		{
			name:     "true value",
			value:    true,
			expected: "✅ Yes",
		},
		{
			name:     "false value",
			value:    false,
			expected: "❌ No",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatBooleanStatus(tt.value)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Helper function to check if output contains a substring
func containsSubstring(output, substring string) bool {
	return bytes.Contains([]byte(output), []byte(substring))
}
