package client

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
	if deviceInfo.DeviceID != "A81B6A536A98" {
		t.Errorf("Expected DeviceID 'A81B6A536A98', got '%s'", deviceInfo.DeviceID)
	}

	if deviceInfo.Type != "SoundTouch 10" {
		t.Errorf("Expected Type 'SoundTouch 10', got '%s'", deviceInfo.Type)
	}

	if deviceInfo.Name != "Sound Machinechen" {
		t.Errorf("Expected Name 'Sound Machinechen', got '%s'", deviceInfo.Name)
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
		if net.IPAddress != "192.168.178.35" {
			t.Errorf("Expected IP address '192.168.178.35', got '%s'", net.IPAddress)
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
