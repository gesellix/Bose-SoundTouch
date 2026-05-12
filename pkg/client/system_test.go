package client

import (
	"net/http"
	"net/http/httptest"

	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_GetClockTime(t *testing.T) {
	tests := []struct {
		name           string
		serverResponse string
		statusCode     int
		expectError    bool
		expectedUTC    int64
		expectedZone   string
	}{
		{
			name:           "Successful clock time retrieval",
			serverResponse: `<clockTime zone="UTC" utc="1609459200">2021-01-01 00:00:00</clockTime>`,
			statusCode:     http.StatusOK,
			expectError:    false,
			expectedUTC:    1609459200,
			expectedZone:   "UTC",
		},
		{
			name:           "Clock time with timezone",
			serverResponse: `<clockTime zone="America/New_York" utc="1609459200">2021-01-01 00:00:00</clockTime>`,
			statusCode:     http.StatusOK,
			expectError:    false,
			expectedUTC:    1609459200,
			expectedZone:   "America/New_York",
		},
		{
			name:           "Server error",
			serverResponse: `<error>Internal server error</error>`,
			statusCode:     http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/clockTime" {
					t.Errorf("Expected path '/clockTime', got '%s'", r.URL.Path)
				}

				if r.Method != "GET" {
					t.Errorf("Expected GET method, got '%s'", r.Method)
				}

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			clockTime, err := client.GetClockTime()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if clockTime.GetUTC() != tt.expectedUTC {
				t.Errorf("Expected UTC %d, got %d", tt.expectedUTC, clockTime.GetUTC())
			}

			if clockTime.GetZone() != tt.expectedZone {
				t.Errorf("Expected Zone %q, got %q", tt.expectedZone, clockTime.GetZone())
			}
		})
	}
}

func TestClient_SetClockTime(t *testing.T) {
	tests := []struct {
		name        string
		request     *models.ClockTimeRequest
		statusCode  int
		expectError bool
	}{
		{
			name: "Successful clock time set",
			request: &models.ClockTimeRequest{
				UTCTime: 1609459200,
			},
			statusCode:  http.StatusOK,
			expectError: false,
		},
		{
			name:    "Invalid request",
			request: &models.ClockTimeRequest{
				// Empty request - should fail validation
			},
			expectError: true,
		},
		{
			name: "Server error",
			request: &models.ClockTimeRequest{
				UTCTime: 1609459200,
			},
			statusCode:  http.StatusInternalServerError,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/clockTime" {
					t.Errorf("Expected path '/clockTime', got '%s'", r.URL.Path)
				}

				if r.Method != "POST" {
					t.Errorf("Expected POST method, got '%s'", r.Method)
				}

				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			err := client.SetClockTime(tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_SetClockTimeNow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clockTime" {
			t.Errorf("Expected path '/clockTime', got '%s'", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.SetClockTimeNow()
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestClient_GetClockDisplay(t *testing.T) {
	tests := []struct {
		name               string
		serverResponse     string
		statusCode         int
		expectError        bool
		expectedEnabled    bool
		expectedFormat     string
		expectedBrightness int
	}{
		{
			name:               "Successful clock display retrieval",
			serverResponse:     `<clockDisplay deviceID="ABCD1234EFGH" enabled="true" format="24" brightness="75" autoDim="true"></clockDisplay>`,
			statusCode:         http.StatusOK,
			expectError:        false,
			expectedEnabled:    true,
			expectedFormat:     "24",
			expectedBrightness: 75,
		},
		{
			name:               "Disabled clock display",
			serverResponse:     `<clockDisplay enabled="false" format="12" brightness="0"></clockDisplay>`,
			statusCode:         http.StatusOK,
			expectError:        false,
			expectedEnabled:    false,
			expectedFormat:     "12",
			expectedBrightness: 0,
		},
		{
			name:           "Server error",
			serverResponse: `<error>Internal server error</error>`,
			statusCode:     http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/clockDisplay" {
					t.Errorf("Expected path '/clockDisplay', got '%s'", r.URL.Path)
				}

				if r.Method != "GET" {
					t.Errorf("Expected GET method, got '%s'", r.Method)
				}

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			clockDisplay, err := client.GetClockDisplay()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if clockDisplay.IsEnabled() != tt.expectedEnabled {
				t.Errorf("Expected Enabled %v, got %v", tt.expectedEnabled, clockDisplay.IsEnabled())
			}

			if clockDisplay.GetFormat() != tt.expectedFormat {
				t.Errorf("Expected Format %q, got %q", tt.expectedFormat, clockDisplay.GetFormat())
			}

			if clockDisplay.GetBrightness() != tt.expectedBrightness {
				t.Errorf("Expected Brightness %d, got %d", tt.expectedBrightness, clockDisplay.GetBrightness())
			}
		})
	}
}

func TestClient_SetClockDisplay(t *testing.T) {
	tests := []struct {
		name        string
		request     *models.ClockDisplayRequest
		statusCode  int
		expectError bool
	}{
		{
			name: "Successful clock display set",
			request: models.NewClockDisplayRequest().
				SetEnabled(true).
				SetFormat(models.ClockFormat24Hour).
				SetBrightness(75),
			statusCode:  http.StatusOK,
			expectError: false,
		},
		{
			name: "Invalid brightness",
			request: models.NewClockDisplayRequest().
				SetBrightness(150), // Invalid brightness > 100
			expectError: true,
		},
		{
			name:        "No changes",
			request:     models.NewClockDisplayRequest(),
			expectError: true,
		},
		{
			name: "Server error",
			request: models.NewClockDisplayRequest().
				SetEnabled(true),
			statusCode:  http.StatusInternalServerError,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.expectError && tt.statusCode == 0 {
				// For client-side validation errors, we don't need a server
				client := createTestClient("http://localhost:8080")

				err := client.SetClockDisplay(tt.request)
				if err == nil {
					t.Error("Expected error, got none")
				}

				return
			}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/clockDisplay" {
					t.Errorf("Expected path '/clockDisplay', got '%s'", r.URL.Path)
				}

				if r.Method != "POST" {
					t.Errorf("Expected POST method, got '%s'", r.Method)
				}

				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				} else {
					w.WriteHeader(http.StatusOK)
				}
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			err := client.SetClockDisplay(tt.request)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_EnableDisableClockDisplay(t *testing.T) {
	tests := []struct {
		name        string
		method      func(*Client) error
		expectError bool
	}{
		{
			name: "Enable clock display",
			method: func(c *Client) error {
				return c.EnableClockDisplay()
			},
			expectError: false,
		},
		{
			name: "Disable clock display",
			method: func(c *Client) error {
				return c.DisableClockDisplay()
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			err := tt.method(client)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestClient_SetClockDisplayBrightness(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clockDisplay" {
			t.Errorf("Expected path '/clockDisplay', got '%s'", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("Expected POST method, got '%s'", r.Method)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.SetClockDisplayBrightness(75)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestClient_SetClockDisplayFormat(t *testing.T) {
	tests := []struct {
		name   string
		format models.ClockFormat
	}{
		{
			name:   "12-hour format",
			format: models.ClockFormat12Hour,
		},
		{
			name:   "24-hour format",
			format: models.ClockFormat24Hour,
		},
		{
			name:   "Auto format",
			format: models.ClockFormatAuto,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/clockDisplay" {
					t.Errorf("Expected path '/clockDisplay', got '%s'", r.URL.Path)
				}

				if r.Method != "POST" {
					t.Errorf("Expected POST method, got '%s'", r.Method)
				}

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			client := createTestClient(server.URL)

			err := client.SetClockDisplayFormat(tt.format)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestClient_GetNetworkInfo(t *testing.T) {
	tests := []struct {
		name                   string
		serverResponse         string
		statusCode             int
		expectError            bool
		expectedDeviceID       string
		expectedInterfaceCount int
	}{
		{
			name: "Successful network info retrieval - WiFi device",
			serverResponse: `<networkInfo wifiProfileCount="2">
<interfaces>
<interface type="WIFI_INTERFACE" name="wlan0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" ssid="MyHomeNetwork" frequencyKHz="5500000" state="NETWORK_WIFI_CONNECTED" signal="EXCELLENT_SIGNAL" mode="STATION"/>
<interface type="WIFI_INTERFACE" name="wlan1" macAddress="AA:BB:CC:DD:EE:01" state="NETWORK_WIFI_DISCONNECTED"/>
</interfaces>
</networkInfo>`,
			statusCode:             http.StatusOK,
			expectError:            false,
			expectedDeviceID:       "",
			expectedInterfaceCount: 2,
		},
		{
			name: "Ethernet device",
			serverResponse: `<networkInfo wifiProfileCount="3">
<interfaces>
<interface type="ETHERNET_INTERFACE" name="eth0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" state="NETWORK_ETHERNET_CONNECTED"/>
</interfaces>
</networkInfo>`,
			statusCode:             http.StatusOK,
			expectError:            false,
			expectedDeviceID:       "",
			expectedInterfaceCount: 1,
		},
		{
			name:           "Server error",
			serverResponse: `<error>Internal server error</error>`,
			statusCode:     http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/networkInfo" {
					t.Errorf("Expected path '/networkInfo', got '%s'", r.URL.Path)
				}

				if r.Method != "GET" {
					t.Errorf("Expected GET method, got '%s'", r.Method)
				}

				w.WriteHeader(tt.statusCode)
				_, _ = w.Write([]byte(tt.serverResponse))
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			networkInfo, err := client.GetNetworkInfo()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectedDeviceID != "" && networkInfo.GetWifiProfileCount() == 0 {
				t.Errorf("Expected non-zero wifi profile count for test %q", tt.name)
			}

			if len(networkInfo.GetInterfaces()) != tt.expectedInterfaceCount {
				t.Errorf("Expected %d interfaces, got %d", tt.expectedInterfaceCount, len(networkInfo.GetInterfaces()))
			}
		})
	}
}

func TestClient_SystemEndpoints_Integration(t *testing.T) {
	// This test ensures all system endpoints work together
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/clockTime":
			switch r.Method {
			case "GET":
				_, _ = w.Write([]byte(`<clockTime zone="UTC" utc="1609459200">2021-01-01 00:00:00</clockTime>`))
			case "POST":
				w.WriteHeader(http.StatusOK)
			}
		case "/clockDisplay":
			switch r.Method {
			case "GET":
				_, _ = w.Write([]byte(`<clockDisplay enabled="true" format="24" brightness="75"></clockDisplay>`))
			case "POST":
				w.WriteHeader(http.StatusOK)
			}
		case "/networkInfo":
			_, _ = w.Write([]byte(`<networkInfo wifiProfileCount="2">
<interfaces>
<interface type="WIFI_INTERFACE" name="wlan0" macAddress="AA:BB:CC:DD:EE:FF" ipAddress="192.168.1.10" ssid="TestNetwork" frequencyKHz="5500000" state="NETWORK_WIFI_CONNECTED" signal="EXCELLENT_SIGNAL" mode="STATION"/>
</interfaces>
</networkInfo>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	// Test clock time operations
	clockTime, err := client.GetClockTime()
	if err != nil {
		t.Errorf("GetClockTime failed: %v", err)
	} else if clockTime.GetUTC() != 1609459200 {
		t.Errorf("Expected UTC 1609459200, got %d", clockTime.GetUTC())
	}

	// Test setting clock time to now
	err = client.SetClockTimeNow()
	if err != nil {
		t.Errorf("SetClockTimeNow failed: %v", err)
	}

	// Test clock display operations
	clockDisplay, err := client.GetClockDisplay()
	if err != nil {
		t.Errorf("GetClockDisplay failed: %v", err)
	} else if !clockDisplay.IsEnabled() {
		t.Error("Expected clock display to be enabled")
	}

	// Test enabling/disabling clock display
	err = client.EnableClockDisplay()
	if err != nil {
		t.Errorf("EnableClockDisplay failed: %v", err)
	}

	err = client.DisableClockDisplay()
	if err != nil {
		t.Errorf("DisableClockDisplay failed: %v", err)
	}

	// Test network info
	networkInfo, err := client.GetNetworkInfo()
	if err != nil {
		t.Errorf("GetNetworkInfo failed: %v", err)
	} else if networkInfo.GetWifiProfileCount() != 2 {
		t.Errorf("Expected wifi profile count 2, got %d", networkInfo.GetWifiProfileCount())
	}
}
