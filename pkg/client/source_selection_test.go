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
			sourceAccount: "AUX",
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
			config := &Config{
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
			config := &Config{
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
			expectedAccount: "AUX",
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
			config := &Config{
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
			serverResponse: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("Not Found"))
			},
			wantError:     true,
			errorContains: "API request failed with status 404",
		},
		{
			name: "Server returns 500",
			serverResponse: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("Internal Server Error"))
			},
			wantError:     true,
			errorContains: "API request failed with status 500",
		},
		{
			name: "Server returns API error",
			serverResponse: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)

				apiError := models.APIError{
					Message: "Invalid source selection",
					Code:    400,
				}
				_ = xml.NewEncoder(w).Encode(apiError)
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
			config := &Config{
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
	config := &Config{
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
		return "AUX IN"
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

func TestClient_SelectContentItem(t *testing.T) {
	tests := []struct {
		name        string
		contentItem *models.ContentItem
		wantError   bool
		errorMsg    string
	}{
		{
			name: "Valid LOCAL_INTERNET_RADIO with streamUrl format",
			contentItem: &models.ContentItem{
				Source:       "LOCAL_INTERNET_RADIO",
				Type:         "stationurl",
				Location:     "http://contentapi.gmuth.de/station.php?name=Antenne%20Chillout&streamUrl=https://stream.antenne.de/chillout/stream/aacp",
				IsPresetable: false,
				ItemName:     "Antenne Chillout",
				ContainerArt: "https://www.radio.net/300/antennechillout.png",
			},
			wantError: false,
		},
		{
			name: "Valid LOCAL_MUSIC content",
			contentItem: &models.ContentItem{
				Source:        "LOCAL_MUSIC",
				Type:          "album",
				Location:      "album:983",
				SourceAccount: "3f205110-4a57-4e91-810a-123456789012",
				IsPresetable:  true,
				ItemName:      "Welcome to the New",
				ContainerArt:  "http://192.168.1.14:8085/v1/albums/983/image",
			},
			wantError: false,
		},
		{
			name: "Valid STORED_MUSIC content",
			contentItem: &models.ContentItem{
				Source:        "STORED_MUSIC",
				Location:      "6_a2874b5d_4f83d999",
				SourceAccount: "d09708a1-5953-44bc-a413-123456789012/0",
				IsPresetable:  true,
				ItemName:      "Christmas Album",
			},
			wantError: false,
		},
		{
			name:        "Nil ContentItem",
			contentItem: nil,
			wantError:   true,
			errorMsg:    "contentItem cannot be nil",
		},
		{
			name: "Empty source",
			contentItem: &models.ContentItem{
				Source:   "",
				Location: "test",
			},
			wantError: true,
			errorMsg:  "contentItem source cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select" {
					t.Errorf("Expected path /select, got %s", r.URL.Path)
				}

				if r.Method != "POST" {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := &Config{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			err := client.SelectContentItem(tt.contentItem)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SelectLocalInternetRadio(t *testing.T) {
	tests := []struct {
		name          string
		location      string
		sourceAccount string
		itemName      string
		containerArt  string
		wantError     bool
		errorMsg      string
	}{
		{
			name:          "Direct stream URL",
			location:      "https://stream.example.com/radio",
			sourceAccount: "",
			itemName:      "My Radio",
			containerArt:  "",
			wantError:     false,
		},
		{
			name:          "StreamUrl format with proxy",
			location:      "http://contentapi.gmuth.de/station.php?name=MyStation&streamUrl=https://stream.example.com/radio",
			sourceAccount: "",
			itemName:      "My Station",
			containerArt:  "https://example.com/art.png",
			wantError:     false,
		},
		{
			name:          "Empty itemName gets default",
			location:      "https://stream.example.com/radio",
			sourceAccount: "",
			itemName:      "",
			containerArt:  "",
			wantError:     false,
		},
		{
			name:      "Empty location",
			location:  "",
			wantError: true,
			errorMsg:  "location cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select" {
					t.Errorf("Expected path /select, got %s", r.URL.Path)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := &Config{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			err := client.SelectLocalInternetRadio(tt.location, tt.sourceAccount, tt.itemName, tt.containerArt)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SelectLocalMusic(t *testing.T) {
	tests := []struct {
		name          string
		location      string
		sourceAccount string
		itemName      string
		containerArt  string
		wantError     bool
		errorMsg      string
	}{
		{
			name:          "Valid album selection",
			location:      "album:983",
			sourceAccount: "3f205110-4a57-4e91-810a-123456789012",
			itemName:      "Welcome to the New",
			containerArt:  "http://192.168.1.14:8085/v1/albums/983/image",
			wantError:     false,
		},
		{
			name:          "Valid track selection",
			location:      "track:2579",
			sourceAccount: "3f205110-4a57-4e91-810a-123456789012",
			itemName:      "Finish What He Started",
			containerArt:  "",
			wantError:     false,
		},
		{
			name:          "Empty location",
			location:      "",
			sourceAccount: "test",
			wantError:     true,
			errorMsg:      "location cannot be empty",
		},
		{
			name:          "Empty sourceAccount",
			location:      "album:983",
			sourceAccount: "",
			wantError:     true,
			errorMsg:      "sourceAccount cannot be empty for LOCAL_MUSIC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select" {
					t.Errorf("Expected path /select, got %s", r.URL.Path)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := &Config{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			err := client.SelectLocalMusic(tt.location, tt.sourceAccount, tt.itemName, tt.containerArt)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SelectStoredMusic(t *testing.T) {
	tests := []struct {
		name          string
		location      string
		sourceAccount string
		itemName      string
		containerArt  string
		wantError     bool
		errorMsg      string
	}{
		{
			name:          "Valid NAS album selection",
			location:      "6_a2874b5d_4f83d999",
			sourceAccount: "d09708a1-5953-44bc-a413-123456789012/0",
			itemName:      "Christmas Album",
			containerArt:  "",
			wantError:     false,
		},
		{
			name:          "Valid track selection",
			location:      "7_114e8de9-8115 TRACK",
			sourceAccount: "d09708a1-5953-44bc-a413-123456789012/0",
			itemName:      "Burn Baby Burn",
			containerArt:  "",
			wantError:     false,
		},
		{
			name:          "Empty location",
			location:      "",
			sourceAccount: "test",
			wantError:     true,
			errorMsg:      "location cannot be empty",
		},
		{
			name:          "Empty sourceAccount",
			location:      "6_a2874b5d_4f83d999",
			sourceAccount: "",
			wantError:     true,
			errorMsg:      "sourceAccount cannot be empty for STORED_MUSIC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/select" {
					t.Errorf("Expected path /select, got %s", r.URL.Path)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := &Config{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			err := client.SelectStoredMusic(tt.location, tt.sourceAccount, tt.itemName, tt.containerArt)

			if tt.wantError {
				if err == nil {
					t.Error("Expected error but got none")
				} else if tt.errorMsg != "" && err.Error() != tt.errorMsg {
					t.Errorf("Expected error message %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}
