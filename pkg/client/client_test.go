package client

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	config := ClientConfig{
		Host:    "192.168.1.100",
		Port:    8090,
		Timeout: 15 * time.Second,
	}

	client := NewClient(config)

	if client.baseURL != "http://192.168.1.100:8090" {
		t.Errorf("Expected baseURL 'http://192.168.1.100:8090', got '%s'", client.baseURL)
	}

	if client.timeout != 15*time.Second {
		t.Errorf("Expected timeout 15s, got %v", client.timeout)
	}

	if client.httpClient.Timeout != 15*time.Second {
		t.Errorf("Expected HTTP client timeout 15s, got %v", client.httpClient.Timeout)
	}
}

func TestNewClientWithDefaults(t *testing.T) {
	config := ClientConfig{
		Host: "192.168.1.100",
	}

	client := NewClient(config)

	if client.baseURL != "http://192.168.1.100:8090" {
		t.Errorf("Expected default port 8090 in baseURL, got '%s'", client.baseURL)
	}

	if client.timeout != 10*time.Second {
		t.Errorf("Expected default timeout 10s, got %v", client.timeout)
	}

	if client.userAgent != "Bose-SoundTouch-Go-Client/1.0" {
		t.Errorf("Expected default user agent, got '%s'", client.userAgent)
	}
}

func TestNewClientFromHost(t *testing.T) {
	client := NewClientFromHost("192.168.1.200")

	expected := "http://192.168.1.200:8090"
	if client.baseURL != expected {
		t.Errorf("Expected baseURL '%s', got '%s'", expected, client.baseURL)
	}
}

func TestGetDeviceInfo_Success(t *testing.T) {
	// Load test data
	testData := loadTestData(t, "info_response.xml")

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.URL.Path != "/info" {
			t.Errorf("Expected path '/info', got '%s'", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("Expected method GET, got %s", r.Method)
		}

		// Check headers
		if r.Header.Get("Accept") != "application/xml" {
			t.Errorf("Expected Accept header 'application/xml', got '%s'", r.Header.Get("Accept"))
		}
		if r.Header.Get("User-Agent") != "Bose-SoundTouch-Go-Client/1.0" {
			t.Errorf("Expected User-Agent header, got '%s'", r.Header.Get("User-Agent"))
		}

		// Send response
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testData))
	}))
	defer server.Close()

	// Create client pointing to mock server
	client := createTestClient(server.URL)

	// Test GetDeviceInfo
	deviceInfo, err := client.GetDeviceInfo()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify response parsing
	if deviceInfo.DeviceID != "ABCD1234EFGH" {
		t.Errorf("Expected DeviceID 'ABCD1234EFGH', got '%s'", deviceInfo.DeviceID)
	}

	if deviceInfo.Type != "SoundTouch 10" {
		t.Errorf("Expected Type 'SoundTouch 10', got '%s'", deviceInfo.Type)
	}

	if deviceInfo.Name != "My SoundTouch Device" {
		t.Errorf("Expected Name 'My SoundTouch Device', got '%s'", deviceInfo.Name)
	}

	if deviceInfo.MargeAccountUUID != "3230304" {
		t.Errorf("Expected MargeAccountUUID '3230304', got '%s'", deviceInfo.MargeAccountUUID)
	}

	if deviceInfo.ModuleType != "sm2" {
		t.Errorf("Expected ModuleType 'sm2', got '%s'", deviceInfo.ModuleType)
	}

	if len(deviceInfo.Components) != 2 {
		t.Errorf("Expected 2 components, got %d", len(deviceInfo.Components))
	}

	// Check first component
	if len(deviceInfo.Components) > 0 {
		comp := deviceInfo.Components[0]
		if comp.ComponentCategory != "SCM" {
			t.Errorf("Expected first component category 'SCM', got '%s'", comp.ComponentCategory)
		}
		if comp.SerialNumber != "I6332527703739342000020" {
			t.Errorf("Expected first component serial 'I6332527703739342000020', got '%s'", comp.SerialNumber)
		}
	}

	// Check network info
	if len(deviceInfo.NetworkInfo) != 2 {
		t.Errorf("Expected 2 network info entries, got %d", len(deviceInfo.NetworkInfo))
	}

	if len(deviceInfo.NetworkInfo) > 0 {
		net := deviceInfo.NetworkInfo[0]
		if net.Type != "SCM" {
			t.Errorf("Expected first network type 'SCM', got '%s'", net.Type)
		}
		if net.IPAddress != "192.168.1.10" {
			t.Errorf("Expected IP address '192.168.1.10', got '%s'", net.IPAddress)
		}
	}
}

func TestGetDeviceInfo_HTTPError(t *testing.T) {
	// Create mock server that returns 404
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	// Test GetDeviceInfo with 404 response
	_, err := client.GetDeviceInfo()
	if err == nil {
		t.Fatal("Expected error for 404 response, got nil")
	}

	expectedError := "API request failed with status 404"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestGetDeviceInfo_InvalidXML(t *testing.T) {
	// Create mock server that returns invalid XML
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("invalid xml content"))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	// Test GetDeviceInfo with invalid XML
	_, err := client.GetDeviceInfo()
	if err == nil {
		t.Fatal("Expected error for invalid XML, got nil")
	}

	expectedError := "failed to unmarshal XML response"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestGetDeviceInfo_APIError(t *testing.T) {
	// Create mock server that returns API error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><error code="404">Device not found</error>`))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	// Test GetDeviceInfo with API error
	_, err := client.GetDeviceInfo()
	if err == nil {
		t.Fatal("Expected API error, got nil")
	}

	// The error gets wrapped by GetDeviceInfo, so check the error message content
	if !contains(err.Error(), "Device not found") {
		t.Errorf("Expected error to contain 'Device not found', got '%s'", err.Error())
	}
}

func TestPing_Success(t *testing.T) {
	testData := loadTestData(t, "info_response.xml")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testData))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.Ping()
	if err != nil {
		t.Errorf("Expected successful ping, got error: %v", err)
	}
}

func TestPing_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.Ping()
	if err == nil {
		t.Error("Expected ping to fail, but got no error")
	}
}

func TestBaseURL(t *testing.T) {
	client := NewClientFromHost("192.168.1.100")
	expected := "http://192.168.1.100:8090"

	if client.BaseURL() != expected {
		t.Errorf("Expected BaseURL '%s', got '%s'", expected, client.BaseURL())
	}
}

func TestClientTimeout(t *testing.T) {
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<info deviceID="test"></info>`))
	}))
	defer server.Close()

	// Create client with short timeout
	config := DefaultConfig()
	config.Timeout = 100 * time.Millisecond
	client := NewClient(config)
	client.baseURL = server.URL

	// Test that request times out
	_, err := client.GetDeviceInfo()
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}

	expectedError := "deadline exceeded"
	if !contains(err.Error(), expectedError) {
		t.Errorf("Expected error to contain '%s', got '%s'", expectedError, err.Error())
	}
}

func TestClient_GetNowPlaying(t *testing.T) {
	tests := []struct {
		name           string
		responseFile   string
		expectedError  bool
		expectedTrack  string
		expectedArtist string
		expectedSource string
		expectedStatus string
	}{
		{
			name:           "spotify track playing",
			responseFile:   "nowplaying_response.xml",
			expectedError:  false,
			expectedTrack:  "In Between Breaths - Paris Unplugged",
			expectedArtist: "SYML",
			expectedSource: "SPOTIFY",
			expectedStatus: "Playing",
		},
		{
			name:           "radio station playing",
			responseFile:   "nowplaying_radio.xml",
			expectedError:  false,
			expectedTrack:  "",
			expectedArtist: "",
			expectedSource: "TUNEIN",
			expectedStatus: "Playing",
		},
		{
			name:           "standby state",
			responseFile:   "nowplaying_empty.xml",
			expectedError:  false,
			expectedTrack:  "",
			expectedArtist: "",
			expectedSource: "STANDBY",
			expectedStatus: "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/now_playing" {
					t.Errorf("Expected path /now_playing, got %s", r.URL.Path)
				}

				// Check headers
				if userAgent := r.Header.Get("User-Agent"); userAgent == "" {
					t.Error("Expected User-Agent header to be set")
				}

				if accept := r.Header.Get("Accept"); accept != "application/xml" {
					t.Errorf("Expected Accept header 'application/xml', got '%s'", accept)
				}

				// Read test data
				data, err := os.ReadFile(filepath.Join("testdata", tt.responseFile))
				if err != nil {
					t.Fatalf("Failed to read test data: %v", err)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			}))
			defer server.Close()

			// Parse server URL to get host and port
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			host := serverURL.Hostname()
			port, _ := strconv.Atoi(serverURL.Port())

			client := NewClient(ClientConfig{
				Host:      host,
				Port:      port,
				Timeout:   5 * time.Second,
				UserAgent: "test-client",
			})

			nowPlaying, err := client.GetNowPlaying()

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if nowPlaying == nil {
				t.Fatal("Expected NowPlaying response but got nil")
			}

			// Verify basic fields
			if nowPlaying.Source != tt.expectedSource {
				t.Errorf("Expected Source '%s', got '%s'", tt.expectedSource, nowPlaying.Source)
			}

			if nowPlaying.Track != tt.expectedTrack {
				t.Errorf("Expected Track '%s', got '%s'", tt.expectedTrack, nowPlaying.Track)
			}

			if nowPlaying.Artist != tt.expectedArtist {
				t.Errorf("Expected Artist '%s', got '%s'", tt.expectedArtist, nowPlaying.Artist)
			}

			if nowPlaying.PlayStatus.String() != tt.expectedStatus {
				t.Errorf("Expected PlayStatus '%s', got '%s'", tt.expectedStatus, nowPlaying.PlayStatus.String())
			}
		})
	}
}

func TestClient_GetNowPlaying_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetNowPlaying()

	if err == nil {
		t.Error("Expected error for server error response")
	}

	expectedErrorMsg := "failed to get now playing"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetNowPlaying_NetworkError(t *testing.T) {
	client := NewClient(ClientConfig{
		Host:    "non-existent-host.invalid",
		Port:    8090,
		Timeout: 1 * time.Second,
	})

	_, err := client.GetNowPlaying()

	if err == nil {
		t.Error("Expected error for network error")
	}

	expectedErrorMsg := "failed to get now playing"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetNowPlaying_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<invalid-xml>"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetNowPlaying()

	if err == nil {
		t.Error("Expected error for invalid XML response")
	}

	expectedErrorMsg := "failed to get now playing"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetSources(t *testing.T) {
	tests := []struct {
		name          string
		responseFile  string
		expectedError bool
		expectedCount int
		expectedReady int
		hasSpotify    bool
		hasAux        bool
		hasBluetooth  bool
	}{
		{
			name:          "sources with mixed availability",
			responseFile:  "sources_response.xml",
			expectedError: false,
			expectedCount: 14,
			expectedReady: 5,
			hasSpotify:    true,
			hasAux:        true,
			hasBluetooth:  false, // Bluetooth is unavailable in test data
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/sources" {
					t.Errorf("Expected path /sources, got %s", r.URL.Path)
				}

				// Check headers
				if userAgent := r.Header.Get("User-Agent"); userAgent == "" {
					t.Error("Expected User-Agent header to be set")
				}

				if accept := r.Header.Get("Accept"); accept != "application/xml" {
					t.Errorf("Expected Accept header 'application/xml', got '%s'", accept)
				}

				// Read test data
				data, err := os.ReadFile(filepath.Join("testdata", tt.responseFile))
				if err != nil {
					t.Fatalf("Failed to read test data: %v", err)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			}))
			defer server.Close()

			// Parse server URL to get host and port
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			host := serverURL.Hostname()
			port, _ := strconv.Atoi(serverURL.Port())

			client := NewClient(ClientConfig{
				Host:      host,
				Port:      port,
				Timeout:   5 * time.Second,
				UserAgent: "test-client",
			})

			sources, err := client.GetSources()

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if sources == nil {
				t.Fatal("Expected Sources response but got nil")
			}

			// Verify counts
			if sources.GetSourceCount() != tt.expectedCount {
				t.Errorf("Expected source count %d, got %d", tt.expectedCount, sources.GetSourceCount())
			}

			if sources.GetReadySourceCount() != tt.expectedReady {
				t.Errorf("Expected ready source count %d, got %d", tt.expectedReady, sources.GetReadySourceCount())
			}

			// Verify specific sources
			if sources.HasSpotify() != tt.hasSpotify {
				t.Errorf("Expected HasSpotify() %v, got %v", tt.hasSpotify, sources.HasSpotify())
			}

			if sources.HasAux() != tt.hasAux {
				t.Errorf("Expected HasAux() %v, got %v", tt.hasAux, sources.HasAux())
			}

			if sources.HasBluetooth() != tt.hasBluetooth {
				t.Errorf("Expected HasBluetooth() %v, got %v", tt.hasBluetooth, sources.HasBluetooth())
			}
		})
	}
}

func TestClient_GetSources_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetSources()

	if err == nil {
		t.Error("Expected error for server error response")
	}

	expectedErrorMsg := "failed to get sources"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetSources_NetworkError(t *testing.T) {
	client := NewClient(ClientConfig{
		Host:    "non-existent-host.invalid",
		Port:    8090,
		Timeout: 1 * time.Second,
	})

	_, err := client.GetSources()

	if err == nil {
		t.Error("Expected error for network error")
	}

	expectedErrorMsg := "failed to get sources"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetSources_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<invalid-xml>"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetSources()

	if err == nil {
		t.Error("Expected error for invalid XML response")
	}

	expectedErrorMsg := "failed to get sources"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetName(t *testing.T) {
	tests := []struct {
		name          string
		responseFile  string
		expectedError bool
		expectedName  string
	}{
		{
			name:          "valid device name",
			responseFile:  "name_response.xml",
			expectedError: false,
			expectedName:  "My SoundTouch Device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/name" {
					t.Errorf("Expected path /name, got %s", r.URL.Path)
				}

				// Check headers
				if userAgent := r.Header.Get("User-Agent"); userAgent == "" {
					t.Error("Expected User-Agent header to be set")
				}

				if accept := r.Header.Get("Accept"); accept != "application/xml" {
					t.Errorf("Expected Accept header 'application/xml', got '%s'", accept)
				}

				// Read test data
				data, err := os.ReadFile(filepath.Join("testdata", tt.responseFile))
				if err != nil {
					t.Fatalf("Failed to read test data: %v", err)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			}))
			defer server.Close()

			// Parse server URL to get host and port
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			host := serverURL.Hostname()
			port, _ := strconv.Atoi(serverURL.Port())

			client := NewClient(ClientConfig{
				Host:      host,
				Port:      port,
				Timeout:   5 * time.Second,
				UserAgent: "test-client",
			})

			name, err := client.GetName()

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if name == nil {
				t.Fatal("Expected Name response but got nil")
			}

			if name.GetName() != tt.expectedName {
				t.Errorf("Expected name '%s', got '%s'", tt.expectedName, name.GetName())
			}
		})
	}
}

func TestClient_GetCapabilities(t *testing.T) {
	tests := []struct {
		name           string
		responseFile   string
		expectedError  bool
		expectedDevice string
		hasLRStereo    bool
		hasDualMode    bool
		hasWSAPIProxy  bool
	}{
		{
			name:           "valid capabilities",
			responseFile:   "capabilities_response.xml",
			expectedError:  false,
			expectedDevice: "ABCD1234EFGH",
			hasLRStereo:    true,
			hasDualMode:    true,
			hasWSAPIProxy:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/capabilities" {
					t.Errorf("Expected path /capabilities, got %s", r.URL.Path)
				}

				// Read test data
				data, err := os.ReadFile(filepath.Join("testdata", tt.responseFile))
				if err != nil {
					t.Fatalf("Failed to read test data: %v", err)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			}))
			defer server.Close()

			// Parse server URL to get host and port
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			host := serverURL.Hostname()
			port, _ := strconv.Atoi(serverURL.Port())

			client := NewClient(ClientConfig{
				Host:      host,
				Port:      port,
				Timeout:   5 * time.Second,
				UserAgent: "test-client",
			})

			capabilities, err := client.GetCapabilities()

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if capabilities == nil {
				t.Fatal("Expected Capabilities response but got nil")
			}

			if capabilities.DeviceID != tt.expectedDevice {
				t.Errorf("Expected device ID '%s', got '%s'", tt.expectedDevice, capabilities.DeviceID)
			}

			if capabilities.HasLRStereoCapability() != tt.hasLRStereo {
				t.Errorf("Expected HasLRStereoCapability() %v, got %v", tt.hasLRStereo, capabilities.HasLRStereoCapability())
			}

			if capabilities.HasDualModeNetwork() != tt.hasDualMode {
				t.Errorf("Expected HasDualModeNetwork() %v, got %v", tt.hasDualMode, capabilities.HasDualModeNetwork())
			}

			if capabilities.HasWSAPIProxy() != tt.hasWSAPIProxy {
				t.Errorf("Expected HasWSAPIProxy() %v, got %v", tt.hasWSAPIProxy, capabilities.HasWSAPIProxy())
			}
		})
	}
}

func TestClient_GetPresets(t *testing.T) {
	tests := []struct {
		name            string
		responseFile    string
		expectedError   bool
		expectedCount   int
		expectedUsed    int
		expectedSpotify int
	}{
		{
			name:            "valid presets",
			responseFile:    "presets_response.xml",
			expectedError:   false,
			expectedCount:   6,
			expectedUsed:    6,
			expectedSpotify: 6,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Errorf("Expected GET request, got %s", r.Method)
				}

				if r.URL.Path != "/presets" {
					t.Errorf("Expected path /presets, got %s", r.URL.Path)
				}

				// Read test data
				data, err := os.ReadFile(filepath.Join("testdata", tt.responseFile))
				if err != nil {
					t.Fatalf("Failed to read test data: %v", err)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				w.Write(data)
			}))
			defer server.Close()

			// Parse server URL to get host and port
			serverURL, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			host := serverURL.Hostname()
			port, _ := strconv.Atoi(serverURL.Port())

			client := NewClient(ClientConfig{
				Host:      host,
				Port:      port,
				Timeout:   5 * time.Second,
				UserAgent: "test-client",
			})

			presets, err := client.GetPresets()

			if tt.expectedError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if presets == nil {
				t.Fatal("Expected Presets response but got nil")
			}

			if presets.GetPresetCount() != tt.expectedCount {
				t.Errorf("Expected preset count %d, got %d", tt.expectedCount, presets.GetPresetCount())
			}

			if len(presets.GetUsedPresetSlots()) != tt.expectedUsed {
				t.Errorf("Expected used count %d, got %d", tt.expectedUsed, len(presets.GetUsedPresetSlots()))
			}

			if len(presets.GetSpotifyPresets()) != tt.expectedSpotify {
				t.Errorf("Expected Spotify count %d, got %d", tt.expectedSpotify, len(presets.GetSpotifyPresets()))
			}
		})
	}
}

func TestClient_GetName_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetName()

	if err == nil {
		t.Error("Expected error for server error response")
	}

	expectedErrorMsg := "failed to get device name"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetCapabilities_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetCapabilities()

	if err == nil {
		t.Error("Expected error for server error response")
	}

	expectedErrorMsg := "failed to get device capabilities"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestClient_GetPresets_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)
	host := serverURL.Hostname()
	port, _ := strconv.Atoi(serverURL.Port())

	client := NewClient(ClientConfig{
		Host:    host,
		Port:    port,
		Timeout: 5 * time.Second,
	})

	_, err := client.GetPresets()

	if err == nil {
		t.Error("Expected error for server error response")
	}

	expectedErrorMsg := "failed to get presets"
	if !strings.Contains(err.Error(), expectedErrorMsg) {
		t.Errorf("Expected error message to contain '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

// Helper functions

func loadTestData(t *testing.T, filename string) string {
	t.Helper()

	path := filepath.Join("testdata", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to load test data %s: %v", filename, err)
	}

	return string(data)
}

func createTestClient(serverURL string) *Client {
	config := DefaultConfig()
	config.Host = "localhost" // Will be overridden by baseURL
	client := NewClient(config)
	client.baseURL = serverURL
	return client
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
