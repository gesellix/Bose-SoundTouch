package client

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

const (
	testTimeout   = 10 * time.Second
	testUserAgent = "Bose-SoundTouch-Go-Client-Test/1.0"
)

func TestClient_SelectSource(t *testing.T) {
	tests := []struct {
		name          string
		source        string
		sourceAccount string
		wantError     bool
		errorMessage  string
	}{
		{
			name:          "Valid Spotify source",
			source:        "SPOTIFY",
			sourceAccount: "user@example.com",
			wantError:     false,
		},
		{
			name:          "Valid Bluetooth source",
			source:        "BLUETOOTH",
			sourceAccount: "",
			wantError:     false,
		},
		{
			name:          "Valid AUX source",
			source:        "AUX",
			sourceAccount: "",
			wantError:     false,
		},
		{
			name:          "Valid TuneIn source",
			source:        "TUNEIN",
			sourceAccount: "tunein_account",
			wantError:     false,
		},
		{
			name:          "Valid Pandora source",
			source:        "PANDORA",
			sourceAccount: "pandora_user",
			wantError:     false,
		},
		{
			name:          "Valid Amazon Music source",
			source:        "AMAZON",
			sourceAccount: "amazon_account",
			wantError:     false,
		},
		{
			name:          "Valid iHeartRadio source",
			source:        "IHEARTRADIO",
			sourceAccount: "",
			wantError:     false,
		},
		{
			name:          "Valid Stored Music source",
			source:        "STORED_MUSIC",
			sourceAccount: "",
			wantError:     false,
		},
		{
			name:          "Empty source",
			source:        "",
			sourceAccount: "",
			wantError:     true,
			errorMessage:  "source cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					t.Errorf("Expected POST request, got %s", r.Method)
					return
				}

				if r.URL.Path != "/select" {
					t.Errorf("Expected path /select, got %s", r.URL.Path)
					return
				}

				// Verify Content-Type
				if contentType := r.Header.Get("Content-Type"); contentType != "application/xml" {
					t.Errorf("Expected Content-Type application/xml, got %s", contentType)
					return
				}

				// Parse and validate request body
				var contentItem models.ContentItem
				err := xml.NewDecoder(r.Body).Decode(&contentItem)
				if err != nil {
					t.Errorf("Failed to decode request XML: %v", err)
					return
				}

				// Validate source
				if contentItem.Source != tt.source {
					t.Errorf("Expected source %s, got %s", tt.source, contentItem.Source)
					return
				}

				// Validate source account
				if contentItem.SourceAccount != tt.sourceAccount {
					t.Errorf("Expected sourceAccount %s, got %s", tt.sourceAccount, contentItem.SourceAccount)
					return
				}

				// Validate item name is set correctly
				expectedItemName := getExpectedItemName(tt.source)
				if contentItem.ItemName != expectedItemName {
					t.Errorf("Expected itemName %s, got %s", expectedItemName, contentItem.ItemName)
					return
				}

				// Return success response
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Create client
			config := ClientConfig{
				Host:      server.URL[7:], // Remove "http://"
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			// Override the base URL to use the test server
			client := NewClient(config)
			client.baseURL = server.URL

			// Call SelectSource
			err := client.SelectSource(tt.source, tt.sourceAccount)

			// Validate result
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if err.Error() != tt.errorMessage {
					t.Errorf("Expected error message '%s', got '%s'", tt.errorMessage, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SelectSourceFromItem(t *testing.T) {
	tests := []struct {
		name        string
		sourceItem  *models.SourceItem
		wantError   bool
		wantSource  string
		wantAccount string
	}{
		{
			name: "Valid Spotify source item",
			sourceItem: &models.SourceItem{
				Source:        "SPOTIFY",
				SourceAccount: "spotify_user",
				Status:        models.SourceStatusReady,
				DisplayName:   "Spotify",
			},
			wantError:   false,
			wantSource:  "SPOTIFY",
			wantAccount: "spotify_user",
		},
		{
			name: "Valid Bluetooth source item",
			sourceItem: &models.SourceItem{
				Source:      "BLUETOOTH",
				Status:      models.SourceStatusReady,
				DisplayName: "Bluetooth",
			},
			wantError:   false,
			wantSource:  "BLUETOOTH",
			wantAccount: "",
		},
		{
			name:       "Nil source item",
			sourceItem: nil,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.sourceItem == nil {
				// Test nil source item without server
				config := DefaultConfig()
				client := NewClient(config)

				err := client.SelectSourceFromItem(tt.sourceItem)
				if !tt.wantError {
					t.Errorf("Expected no error, got: %v", err)
				} else if err == nil {
					t.Errorf("Expected error for nil source item")
				}
				return
			}

			// Create mock server for valid source items
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Parse request body
				var contentItem models.ContentItem
				err := xml.NewDecoder(r.Body).Decode(&contentItem)
				if err != nil {
					t.Errorf("Failed to decode request XML: %v", err)
					return
				}

				// Validate source and account
				if contentItem.Source != tt.wantSource {
					t.Errorf("Expected source %s, got %s", tt.wantSource, contentItem.Source)
					return
				}

				if contentItem.SourceAccount != tt.wantAccount {
					t.Errorf("Expected sourceAccount %s, got %s", tt.wantAccount, contentItem.SourceAccount)
					return
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Create client
			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			// Call SelectSourceFromItem
			err := client.SelectSourceFromItem(tt.sourceItem)

			// Validate result
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_ConvenienceSourceMethods(t *testing.T) {
	tests := []struct {
		name            string
		method          string
		sourceAccount   string
		expectedSource  string
		expectedAccount string
	}{
		{
			name:            "SelectSpotify with account",
			method:          "spotify",
			sourceAccount:   "spotify_user",
			expectedSource:  "SPOTIFY",
			expectedAccount: "spotify_user",
		},
		{
			name:            "SelectSpotify without account",
			method:          "spotify",
			sourceAccount:   "",
			expectedSource:  "SPOTIFY",
			expectedAccount: "",
		},
		{
			name:            "SelectBluetooth",
			method:          "bluetooth",
			sourceAccount:   "",
			expectedSource:  "BLUETOOTH",
			expectedAccount: "",
		},
		{
			name:            "SelectAux",
			method:          "aux",
			sourceAccount:   "",
			expectedSource:  "AUX",
			expectedAccount: "",
		},
		{
			name:            "SelectTuneIn",
			method:          "tunein",
			sourceAccount:   "tunein_account",
			expectedSource:  "TUNEIN",
			expectedAccount: "tunein_account",
		},
		{
			name:            "SelectPandora",
			method:          "pandora",
			sourceAccount:   "pandora_user",
			expectedSource:  "PANDORA",
			expectedAccount: "pandora_user",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Parse request body
				var contentItem models.ContentItem
				err := xml.NewDecoder(r.Body).Decode(&contentItem)
				if err != nil {
					t.Errorf("Failed to decode request XML: %v", err)
					return
				}

				// Validate source and account
				if contentItem.Source != tt.expectedSource {
					t.Errorf("Expected source %s, got %s", tt.expectedSource, contentItem.Source)
					return
				}

				if contentItem.SourceAccount != tt.expectedAccount {
					t.Errorf("Expected sourceAccount %s, got %s", tt.expectedAccount, contentItem.SourceAccount)
					return
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			// Create client
			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			// Call the appropriate convenience method
			var err error
			switch tt.method {
			case "spotify":
				err = client.SelectSpotify(tt.sourceAccount)
			case "bluetooth":
				err = client.SelectBluetooth()
			case "aux":
				err = client.SelectAux()
			case "tunein":
				err = client.SelectTuneIn(tt.sourceAccount)
			case "pandora":
				err = client.SelectPandora(tt.sourceAccount)
			default:
				t.Fatalf("Unknown method: %s", tt.method)
			}

			// Validate result
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_SelectSource_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		wantError      bool
		errorContains  string
	}{
		{
			name: "Server returns 404",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Not Found"))
			},
			wantError:     true,
			errorContains: "API request failed with status 404",
		},
		{
			name: "Server returns 500",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			wantError:     true,
			errorContains: "API request failed with status 500",
		},
		{
			name: "Server returns API error",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				apiError := models.APIError{
					Message: "Invalid source selection",
					Code:    400,
				}
				xml.NewEncoder(w).Encode(apiError)
			},
			wantError:     true,
			errorContains: "Invalid source selection",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			// Create client
			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			// Call SelectSource
			err := client.SelectSource("SPOTIFY", "test_account")

			// Validate result
			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if tt.errorContains != "" && !containsSubstring(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SelectSource_RequestFormat(t *testing.T) {
	// Test that the request XML format is correct
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read and parse the raw request body
		var contentItem models.ContentItem
		err := xml.NewDecoder(r.Body).Decode(&contentItem)
		if err != nil {
			t.Errorf("Failed to decode request XML: %v", err)
			return
		}

		// Validate XML structure
		expectedXML := `<ContentItem source="SPOTIFY" sourceAccount="test_user"><itemName>Spotify</itemName></ContentItem>`

		// Re-encode to compare
		actualXML, err := xml.Marshal(contentItem)
		if err != nil {
			t.Errorf("Failed to marshal ContentItem: %v", err)
			return
		}

		// Basic validation of XML content (not exact string match due to formatting)
		if contentItem.Source != "SPOTIFY" {
			t.Errorf("Expected source SPOTIFY, got %s", contentItem.Source)
		}
		if contentItem.SourceAccount != "test_user" {
			t.Errorf("Expected sourceAccount test_user, got %s", contentItem.SourceAccount)
		}
		if contentItem.ItemName != "Spotify" {
			t.Errorf("Expected itemName Spotify, got %s", contentItem.ItemName)
		}

		t.Logf("Expected XML format: %s", expectedXML)
		t.Logf("Actual XML: %s", string(actualXML))

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// Create client
	config := ClientConfig{
		Host:      server.URL[7:],
		Port:      80,
		Timeout:   testTimeout,
		UserAgent: testUserAgent,
	}
	client := NewClient(config)
	client.baseURL = server.URL

	// Call SelectSource
	err := client.SelectSource("SPOTIFY", "test_user")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

// Helper function to get expected item name for each source
func getExpectedItemName(source string) string {
	switch source {
	case "SPOTIFY":
		return "Spotify"
	case "BLUETOOTH":
		return "Bluetooth"
	case "AUX":
		return "AUX Input"
	case "TUNEIN":
		return "TuneIn"
	case "PANDORA":
		return "Pandora"
	case "AMAZON":
		return "Amazon Music"
	case "IHEARTRADIO":
		return "iHeartRadio"
	case "STORED_MUSIC":
		return "Stored Music"
	default:
		return source // Default to source name
	}
}

// Helper function to check if a string contains a substring
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				(s[:len(substr)] == substr ||
					s[len(s)-len(substr):] == substr ||
					containsMiddleSubstring(s, substr))))
}

func containsMiddleSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
