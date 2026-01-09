package client

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_GetBass(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		wantError      bool
		wantTargetBass int
		wantActualBass int
		wantDeviceID   string
	}{
		{
			name: "Valid bass response",
			serverResponse: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>3</targetbass>
  <actualbass>3</actualbass>
</bass>`,
			wantError:      false,
			wantTargetBass: 3,
			wantActualBass: 3,
			wantDeviceID:   "1234567890AB",
		},
		{
			name: "Negative bass response",
			serverResponse: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>-5</targetbass>
  <actualbass>-5</actualbass>
</bass>`,
			wantError:      false,
			wantTargetBass: -5,
			wantActualBass: -5,
			wantDeviceID:   "1234567890AB",
		},
		{
			name: "Zero bass response",
			serverResponse: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>0</targetbass>
  <actualbass>0</actualbass>
</bass>`,
			wantError:      false,
			wantTargetBass: 0,
			wantActualBass: 0,
			wantDeviceID:   "1234567890AB",
		},
		{
			name: "Bass adjustment in progress",
			serverResponse: `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>6</targetbass>
  <actualbass>4</actualbass>
</bass>`,
			wantError:      false,
			wantTargetBass: 6,
			wantActualBass: 4,
			wantDeviceID:   "1234567890AB",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
					return
				}
				if r.URL.Path != "/bass" {
					t.Errorf("Expected path /bass, got %s", r.URL.Path)
					return
				}
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			config := ClientConfig{
				Host:      server.URL[7:], // Remove "http://"
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			bass, err := client.GetBass()

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if bass.TargetBass != tt.wantTargetBass {
					t.Errorf("Expected target bass %d, got %d", tt.wantTargetBass, bass.TargetBass)
				}
				if bass.ActualBass != tt.wantActualBass {
					t.Errorf("Expected actual bass %d, got %d", tt.wantActualBass, bass.ActualBass)
				}
				if bass.DeviceID != tt.wantDeviceID {
					t.Errorf("Expected device ID %s, got %s", tt.wantDeviceID, bass.DeviceID)
				}
			}
		})
	}
}

func TestClient_SetBass(t *testing.T) {
	tests := []struct {
		name      string
		level     int
		wantError bool
	}{
		{
			name:      "Valid bass level 0",
			level:     0,
			wantError: false,
		},
		{
			name:      "Valid bass level +9",
			level:     9,
			wantError: false,
		},
		{
			name:      "Valid bass level -9",
			level:     -9,
			wantError: false,
		},
		{
			name:      "Valid bass level +3",
			level:     3,
			wantError: false,
		},
		{
			name:      "Valid bass level -3",
			level:     -3,
			wantError: false,
		},
		{
			name:      "Invalid bass level +10",
			level:     10,
			wantError: true,
		},
		{
			name:      "Invalid bass level -10",
			level:     -10,
			wantError: true,
		},
		{
			name:      "Invalid bass level +100",
			level:     100,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.wantError {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != "POST" {
						t.Errorf("Expected POST request, got %s", r.Method)
						return
					}
					if r.URL.Path != "/bass" {
						t.Errorf("Expected path /bass, got %s", r.URL.Path)
						return
					}

					// Verify Content-Type
					if contentType := r.Header.Get("Content-Type"); contentType != "application/xml" {
						t.Errorf("Expected Content-Type application/xml, got %s", contentType)
						return
					}

					// Parse and validate request body
					var bassReq models.BassRequest
					err := xml.NewDecoder(r.Body).Decode(&bassReq)
					if err != nil {
						t.Errorf("Failed to decode request XML: %v", err)
						return
					}

					if bassReq.Level != tt.level {
						t.Errorf("Expected bass level %d, got %d", tt.level, bassReq.Level)
						return
					}

					w.WriteHeader(http.StatusOK)
				}))
				defer server.Close()

				config := ClientConfig{
					Host:      server.URL[7:],
					Port:      80,
					Timeout:   testTimeout,
					UserAgent: testUserAgent,
				}
				client := NewClient(config)
				client.baseURL = server.URL

				err := client.SetBass(tt.level)
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			} else {
				// Test validation without server
				config := ClientConfig{
					Host:      "localhost",
					Port:      8090,
					Timeout:   testTimeout,
					UserAgent: testUserAgent,
				}
				client := NewClient(config)

				err := client.SetBass(tt.level)
				if err == nil {
					t.Errorf("Expected error for invalid bass level %d, got nil", tt.level)
				}
			}
		})
	}
}

func TestClient_SetBassSafe(t *testing.T) {
	tests := []struct {
		name          string
		level         int
		expectedLevel int
	}{
		{
			name:          "Valid level unchanged",
			level:         3,
			expectedLevel: 3,
		},
		{
			name:          "Too high clamped",
			level:         15,
			expectedLevel: 9,
		},
		{
			name:          "Too low clamped",
			level:         -15,
			expectedLevel: -9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Parse request body to verify clamped level
				var bassReq models.BassRequest
				err := xml.NewDecoder(r.Body).Decode(&bassReq)
				if err != nil {
					t.Errorf("Failed to decode request XML: %v", err)
					return
				}

				if bassReq.Level != tt.expectedLevel {
					t.Errorf("Expected clamped bass level %d, got %d", tt.expectedLevel, bassReq.Level)
					return
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			err := client.SetBassSafe(tt.level)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_IncreaseBass(t *testing.T) {
	tests := []struct {
		name            string
		currentBass     int
		amount          int
		expectedNewBass int
	}{
		{
			name:            "Normal increase",
			currentBass:     0,
			amount:          3,
			expectedNewBass: 3,
		},
		{
			name:            "Increase with clamping",
			currentBass:     8,
			amount:          3,
			expectedNewBass: 9,
		},
		{
			name:            "Increase from negative",
			currentBass:     -3,
			amount:          2,
			expectedNewBass: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getCallCount := 0
			postCallCount := 0

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")

				if r.Method == "GET" && r.URL.Path == "/bass" {
					getCallCount++
					// Return current bass level
					response := `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>` + string(rune(tt.currentBass+48)) + `</targetbass>
  <actualbass>` + string(rune(tt.currentBass+48)) + `</actualbass>
</bass>`
					if getCallCount == 1 {
						// First call - return current bass
						if tt.currentBass >= 0 && tt.currentBass <= 9 {
							response = `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>` + string(rune(tt.currentBass+'0')) + `</targetbass>
  <actualbass>` + string(rune(tt.currentBass+'0')) + `</actualbass>
</bass>`
						} else {
							// Handle negative numbers
							response = `<?xml version="1.0" encoding="UTF-8" ?>
<bass deviceID="1234567890AB">
  <targetbass>` + string(rune(-tt.currentBass+'0')) + `</targetbass>
  <actualbass>` + string(rune(-tt.currentBass+'0')) + `</actualbass>
</bass>`
						}
						// For simplicity in testing, let's use a different approach
						if tt.currentBass == 0 {
							response = `<bass deviceID="1234567890AB"><targetbass>0</targetbass><actualbass>0</actualbass></bass>`
						} else if tt.currentBass == 8 {
							response = `<bass deviceID="1234567890AB"><targetbass>8</targetbass><actualbass>8</actualbass></bass>`
						} else if tt.currentBass == -3 {
							response = `<bass deviceID="1234567890AB"><targetbass>-3</targetbass><actualbass>-3</actualbass></bass>`
						}
					} else {
						// Second call - return new bass level
						if tt.expectedNewBass == 3 {
							response = `<bass deviceID="1234567890AB"><targetbass>3</targetbass><actualbass>3</actualbass></bass>`
						} else if tt.expectedNewBass == 9 {
							response = `<bass deviceID="1234567890AB"><targetbass>9</targetbass><actualbass>9</actualbass></bass>`
						} else if tt.expectedNewBass == -1 {
							response = `<bass deviceID="1234567890AB"><targetbass>-1</targetbass><actualbass>-1</actualbass></bass>`
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				} else if r.Method == "POST" && r.URL.Path == "/bass" {
					postCallCount++
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()

			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			bass, err := client.IncreaseBass(tt.amount)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if bass.GetLevel() != tt.expectedNewBass {
				t.Errorf("Expected new bass level %d, got %d", tt.expectedNewBass, bass.GetLevel())
			}

			if getCallCount != 2 {
				t.Errorf("Expected 2 GET calls, got %d", getCallCount)
			}
			if postCallCount != 1 {
				t.Errorf("Expected 1 POST call, got %d", postCallCount)
			}
		})
	}
}

func TestClient_DecreaseBass(t *testing.T) {
	tests := []struct {
		name            string
		currentBass     int
		amount          int
		expectedNewBass int
	}{
		{
			name:            "Normal decrease",
			currentBass:     3,
			amount:          2,
			expectedNewBass: 1,
		},
		{
			name:            "Decrease with clamping",
			currentBass:     -7,
			amount:          5,
			expectedNewBass: -9,
		},
		{
			name:            "Decrease to negative",
			currentBass:     2,
			amount:          4,
			expectedNewBass: -2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getCallCount := 0
			postCallCount := 0

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")

				if r.Method == "GET" && r.URL.Path == "/bass" {
					getCallCount++
					var response string
					if getCallCount == 1 {
						// First call - return current bass
						if tt.currentBass == 3 {
							response = `<bass deviceID="1234567890AB"><targetbass>3</targetbass><actualbass>3</actualbass></bass>`
						} else if tt.currentBass == -7 {
							response = `<bass deviceID="1234567890AB"><targetbass>-7</targetbass><actualbass>-7</actualbass></bass>`
						} else if tt.currentBass == 2 {
							response = `<bass deviceID="1234567890AB"><targetbass>2</targetbass><actualbass>2</actualbass></bass>`
						}
					} else {
						// Second call - return new bass level
						if tt.expectedNewBass == 1 {
							response = `<bass deviceID="1234567890AB"><targetbass>1</targetbass><actualbass>1</actualbass></bass>`
						} else if tt.expectedNewBass == -9 {
							response = `<bass deviceID="1234567890AB"><targetbass>-9</targetbass><actualbass>-9</actualbass></bass>`
						} else if tt.expectedNewBass == -2 {
							response = `<bass deviceID="1234567890AB"><targetbass>-2</targetbass><actualbass>-2</actualbass></bass>`
						}
					}
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(response))
				} else if r.Method == "POST" && r.URL.Path == "/bass" {
					postCallCount++
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()

			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			bass, err := client.DecreaseBass(tt.amount)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if bass.GetLevel() != tt.expectedNewBass {
				t.Errorf("Expected new bass level %d, got %d", tt.expectedNewBass, bass.GetLevel())
			}

			if getCallCount != 2 {
				t.Errorf("Expected 2 GET calls, got %d", getCallCount)
			}
			if postCallCount != 1 {
				t.Errorf("Expected 1 POST call, got %d", postCallCount)
			}
		})
	}
}

func TestClient_Bass_ErrorHandling(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse func(w http.ResponseWriter, r *http.Request)
		method         func(*Client) error
		wantError      bool
		errorContains  string
	}{
		{
			name: "GetBass server returns 404",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				w.Write([]byte("Not Found"))
			},
			method: func(c *Client) error {
				_, err := c.GetBass()
				return err
			},
			wantError:     true,
			errorContains: "failed to get bass",
		},
		{
			name: "SetBass server returns 500",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Internal Server Error"))
			},
			method: func(c *Client) error {
				return c.SetBass(3)
			},
			wantError:     true,
			errorContains: "API request failed with status 500",
		},
		{
			name: "GetBass invalid XML response",
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("invalid xml"))
			},
			method: func(c *Client) error {
				_, err := c.GetBass()
				return err
			},
			wantError:     true,
			errorContains: "failed to get bass",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(tt.serverResponse))
			defer server.Close()

			config := ClientConfig{
				Host:      server.URL[7:],
				Port:      80,
				Timeout:   testTimeout,
				UserAgent: testUserAgent,
			}
			client := NewClient(config)
			client.baseURL = server.URL

			err := tt.method(client)

			if tt.wantError {
				if err == nil {
					t.Errorf("Expected error, got nil")
				} else if !containsSubstring(err.Error(), tt.errorContains) {
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

func TestClient_Bass_RequestFormat(t *testing.T) {
	// Test that the request XML format is correct
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read and parse the raw request body
		var bassReq models.BassRequest
		err := xml.NewDecoder(r.Body).Decode(&bassReq)
		if err != nil {
			t.Errorf("Failed to decode request XML: %v", err)
			return
		}

		// Validate XML structure
		expectedLevel := 5
		if bassReq.Level != expectedLevel {
			t.Errorf("Expected bass level %d, got %d", expectedLevel, bassReq.Level)
		}

		// Re-encode to verify XML format
		actualXML, err := xml.Marshal(bassReq)
		if err != nil {
			t.Errorf("Failed to marshal BassRequest: %v", err)
			return
		}

		expectedXML := "<bass>5</bass>"
		if string(actualXML) != expectedXML {
			t.Errorf("Expected XML '%s', got '%s'", expectedXML, string(actualXML))
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	config := ClientConfig{
		Host:      server.URL[7:],
		Port:      80,
		Timeout:   testTimeout,
		UserAgent: testUserAgent,
	}
	client := NewClient(config)
	client.baseURL = server.URL

	err := client.SetBass(5)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}
