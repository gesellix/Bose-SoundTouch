package client

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_SetName(t *testing.T) {
	tests := []struct {
		name           string
		deviceName     string
		responseStatus int
		responseBody   string
		expectError    bool
		errorMessage   string
	}{
		{
			name:           "successful name set",
			deviceName:     "Living Room Speaker",
			responseStatus: http.StatusOK,
			responseBody:   "",
			expectError:    false,
		},
		{
			name:           "empty name",
			deviceName:     "",
			responseStatus: http.StatusOK,
			responseBody:   "",
			expectError:    false,
		},
		{
			name:           "special characters in name",
			deviceName:     "Kitchen Speaker (Main) & More",
			responseStatus: http.StatusOK,
			responseBody:   "",
			expectError:    false,
		},
		{
			name:           "server error",
			deviceName:     "Test Speaker",
			responseStatus: http.StatusInternalServerError,
			responseBody:   "Internal Server Error",
			expectError:    true,
			errorMessage:   "500",
		},
		{
			name:           "bad request",
			deviceName:     "Test Speaker",
			responseStatus: http.StatusBadRequest,
			responseBody:   "Bad Request",
			expectError:    true,
			errorMessage:   "400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
				}

				if r.URL.Path != "/name" {
					t.Errorf("Expected path /name, got %s", r.URL.Path)
				}

				// Verify content type
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/xml" {
					t.Errorf("Expected Content-Type application/xml, got %s", contentType)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client pointing to test server
			config := &Config{
				Host: server.URL[7:], // Remove "http://" prefix
				Port: 0,              // Will be ignored due to full URL in Host
			}
			client := NewClient(config)

			// Override the base URL to point to our test server
			client.baseURL = server.URL

			// Execute set name
			err := client.SetName(tt.deviceName)

			// Check results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMessage != "" && !containsString(err.Error(), tt.errorMessage) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMessage, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestClient_GetBassCapabilities(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
		expectedCaps   *models.BassCapabilities
	}{
		{
			name:           "bass supported device",
			responseStatus: http.StatusOK,
			responseBody: `<bassCapabilities deviceID="AABBCCDDEEFF">
				<bassAvailable>true</bassAvailable>
				<bassMin>-9</bassMin>
				<bassMax>9</bassMax>
				<bassDefault>0</bassDefault>
			</bassCapabilities>`,
			expectError: false,
			expectedCaps: &models.BassCapabilities{
				DeviceID:      "AABBCCDDEEFF",
				BassAvailable: true,
				BassMin:       -9,
				BassMax:       9,
				BassDefault:   0,
			},
		},
		{
			name:           "bass not supported device",
			responseStatus: http.StatusOK,
			responseBody: `<bassCapabilities deviceID="AABBCCDDEEFF">
				<bassAvailable>false</bassAvailable>
				<bassMin>0</bassMin>
				<bassMax>0</bassMax>
				<bassDefault>0</bassDefault>
			</bassCapabilities>`,
			expectError: false,
			expectedCaps: &models.BassCapabilities{
				DeviceID:      "AABBCCDDEEFF",
				BassAvailable: false,
				BassMin:       0,
				BassMax:       0,
				BassDefault:   0,
			},
		},
		{
			name:           "server error",
			responseStatus: http.StatusInternalServerError,
			responseBody:   "Internal Server Error",
			expectError:    true,
		},
		{
			name:           "not found (unsupported)",
			responseStatus: http.StatusNotFound,
			responseBody:   "Not Found",
			expectError:    true,
		},
		{
			name:           "invalid XML response",
			responseStatus: http.StatusOK,
			responseBody:   "invalid xml content",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/bassCapabilities" {
					t.Errorf("Expected path /bassCapabilities, got %s", r.URL.Path)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client pointing to test server
			config := &Config{
				Host: server.URL[7:], // Remove "http://" prefix
				Port: 0,              // Will be ignored due to full URL in Host
			}
			client := NewClient(config)

			// Override the base URL to point to our test server
			client.baseURL = server.URL

			// Execute get bass capabilities
			capabilities, err := client.GetBassCapabilities()

			// Check results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}

				if capabilities == nil {
					t.Errorf("Expected capabilities but got nil")
					return
				}

				// Check specific fields
				if capabilities.DeviceID != tt.expectedCaps.DeviceID {
					t.Errorf("Expected DeviceID %s, got %s", tt.expectedCaps.DeviceID, capabilities.DeviceID)
				}

				if capabilities.BassAvailable != tt.expectedCaps.BassAvailable {
					t.Errorf("Expected BassAvailable %t, got %t", tt.expectedCaps.BassAvailable, capabilities.BassAvailable)
				}

				if capabilities.BassMin != tt.expectedCaps.BassMin {
					t.Errorf("Expected BassMin %d, got %d", tt.expectedCaps.BassMin, capabilities.BassMin)
				}

				if capabilities.BassMax != tt.expectedCaps.BassMax {
					t.Errorf("Expected BassMax %d, got %d", tt.expectedCaps.BassMax, capabilities.BassMax)
				}

				if capabilities.BassDefault != tt.expectedCaps.BassDefault {
					t.Errorf("Expected BassDefault %d, got %d", tt.expectedCaps.BassDefault, capabilities.BassDefault)
				}

				// Test helper methods
				if capabilities.IsBassSupported() != tt.expectedCaps.BassAvailable {
					t.Errorf("IsBassSupported() mismatch")
				}

				if tt.expectedCaps.BassAvailable {
					if capabilities.GetMinLevel() != tt.expectedCaps.BassMin {
						t.Errorf("GetMinLevel() returned %d, expected %d", capabilities.GetMinLevel(), tt.expectedCaps.BassMin)
					}

					if capabilities.GetMaxLevel() != tt.expectedCaps.BassMax {
						t.Errorf("GetMaxLevel() returned %d, expected %d", capabilities.GetMaxLevel(), tt.expectedCaps.BassMax)
					}

					if capabilities.GetDefaultLevel() != tt.expectedCaps.BassDefault {
						t.Errorf("GetDefaultLevel() returned %d, expected %d", capabilities.GetDefaultLevel(), tt.expectedCaps.BassDefault)
					}

					// Test validation
					if !capabilities.ValidateLevel(0) {
						t.Errorf("ValidateLevel(0) should return true for supported device")
					}

					if !capabilities.ValidateLevel(tt.expectedCaps.BassMin) {
						t.Errorf("ValidateLevel(min) should return true")
					}

					if !capabilities.ValidateLevel(tt.expectedCaps.BassMax) {
						t.Errorf("ValidateLevel(max) should return true")
					}

					if capabilities.ValidateLevel(tt.expectedCaps.BassMin - 1) {
						t.Errorf("ValidateLevel(below min) should return false")
					}

					if capabilities.ValidateLevel(tt.expectedCaps.BassMax + 1) {
						t.Errorf("ValidateLevel(above max) should return false")
					}

					// Test clamping
					if capabilities.ClampLevel(tt.expectedCaps.BassMin-5) != tt.expectedCaps.BassMin {
						t.Errorf("ClampLevel(below min) should return min level")
					}

					if capabilities.ClampLevel(tt.expectedCaps.BassMax+5) != tt.expectedCaps.BassMax {
						t.Errorf("ClampLevel(above max) should return max level")
					}
				}
			}
		})
	}
}

func TestClient_GetTrackInfo(t *testing.T) {
	tests := []struct {
		name           string
		responseStatus int
		responseBody   string
		expectError    bool
		expectedInfo   *models.NowPlaying
	}{
		{
			name:           "track info with full details",
			responseStatus: http.StatusOK,
			responseBody: `<nowPlaying deviceID="AABBCCDDEEFF" source="SPOTIFY">
				<ContentItem source="SPOTIFY" location="/12345" sourceAccount="user@example.com" isPresetable="true">
					<itemName>Test Track</itemName>
				</ContentItem>
				<track>Test Track</track>
				<artist>Test Artist</artist>
				<album>Test Album</album>
				<playStatus>PLAY_STATE</playStatus>
			</nowPlaying>`,
			expectError: false,
			expectedInfo: &models.NowPlaying{
				DeviceID:   "AABBCCDDEEFF",
				Source:     "SPOTIFY",
				Track:      "Test Track",
				Artist:     "Test Artist",
				Album:      "Test Album",
				PlayStatus: "PLAY_STATE",
			},
		},
		{
			name:           "radio station info",
			responseStatus: http.StatusOK,
			responseBody: `<nowPlaying deviceID="AABBCCDDEEFF" source="TUNEIN">
				<ContentItem source="TUNEIN" location="/station123" isPresetable="true">
					<itemName>Jazz FM</itemName>
				</ContentItem>
				<stationName>Jazz FM</stationName>
				<playStatus>PLAY_STATE</playStatus>
			</nowPlaying>`,
			expectError: false,
			expectedInfo: &models.NowPlaying{
				DeviceID:    "AABBCCDDEEFF",
				Source:      "TUNEIN",
				StationName: "Jazz FM",
				PlayStatus:  "PLAY_STATE",
			},
		},
		{
			name:           "standby state",
			responseStatus: http.StatusOK,
			responseBody: `<nowPlaying deviceID="AABBCCDDEEFF" source="STANDBY">
				<ContentItem source="STANDBY">
					<itemName>STANDBY</itemName>
				</ContentItem>
				<playStatus>STOP_STATE</playStatus>
			</nowPlaying>`,
			expectError: false,
			expectedInfo: &models.NowPlaying{
				DeviceID:   "AABBCCDDEEFF",
				Source:     "STANDBY",
				PlayStatus: "STOP_STATE",
			},
		},
		{
			name:           "server error",
			responseStatus: http.StatusInternalServerError,
			responseBody:   "Internal Server Error",
			expectError:    true,
		},
		{
			name:           "not found",
			responseStatus: http.StatusNotFound,
			responseBody:   "Not Found",
			expectError:    true,
		},
		{
			name:           "invalid XML response",
			responseStatus: http.StatusOK,
			responseBody:   "invalid xml content",
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Verify the request
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/trackInfo" {
					t.Errorf("Expected path /trackInfo, got %s", r.URL.Path)
				}

				// Send response
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			// Create client pointing to test server
			config := &Config{
				Host: server.URL[7:], // Remove "http://" prefix
				Port: 0,              // Will be ignored due to full URL in Host
			}
			client := NewClient(config)

			// Override the base URL to point to our test server
			client.baseURL = server.URL

			// Execute get track info
			trackInfo, err := client.GetTrackInfo()

			// Check results
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}

				if trackInfo == nil {
					t.Errorf("Expected track info but got nil")
					return
				}

				// Check specific fields
				if trackInfo.DeviceID != tt.expectedInfo.DeviceID {
					t.Errorf("Expected DeviceID %s, got %s", tt.expectedInfo.DeviceID, trackInfo.DeviceID)
				}

				if trackInfo.Source != tt.expectedInfo.Source {
					t.Errorf("Expected Source %s, got %s", tt.expectedInfo.Source, trackInfo.Source)
				}

				if trackInfo.Track != tt.expectedInfo.Track {
					t.Errorf("Expected Track %s, got %s", tt.expectedInfo.Track, trackInfo.Track)
				}

				if trackInfo.Artist != tt.expectedInfo.Artist {
					t.Errorf("Expected Artist %s, got %s", tt.expectedInfo.Artist, trackInfo.Artist)
				}

				if trackInfo.Album != tt.expectedInfo.Album {
					t.Errorf("Expected Album %s, got %s", tt.expectedInfo.Album, trackInfo.Album)
				}

				if trackInfo.StationName != tt.expectedInfo.StationName {
					t.Errorf("Expected StationName %s, got %s", tt.expectedInfo.StationName, trackInfo.StationName)
				}

				if trackInfo.PlayStatus != tt.expectedInfo.PlayStatus {
					t.Errorf("Expected PlayStatus %s, got %s", tt.expectedInfo.PlayStatus, trackInfo.PlayStatus)
				}
			}
		})
	}
}

func TestClient_NewEndpoints_NetworkError(t *testing.T) {
	// Create client pointing to non-existent server
	config := &Config{
		Host: "192.0.2.999", // Invalid IP
		Port: 8090,
	}
	client := NewClient(config)

	// Test SetName
	err := client.SetName("Test")
	if err == nil {
		t.Errorf("Expected network error for SetName but got none")
	}

	// Test GetBassCapabilities
	_, err = client.GetBassCapabilities()
	if err == nil {
		t.Errorf("Expected network error for GetBassCapabilities but got none")
	}

	// Test GetTrackInfo
	_, err = client.GetTrackInfo()
	if err == nil {
		t.Errorf("Expected network error for GetTrackInfo but got none")
	}
}

// containsString checks if a string contains a substring (helper function)
func containsString(str, substr string) bool {
	return len(str) >= len(substr) &&
		(str == substr ||
			(len(str) > len(substr) &&
				(str[:len(substr)] == substr ||
					str[len(str)-len(substr):] == substr ||
					findSubstring(str, substr))))
}

func findSubstring(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}

	return false
}
