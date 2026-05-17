// Package handlers contains tests for HTTP handlers.
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/go-chi/chi/v5"
)

func createTestApp() *WebApp {
	app := NewWebApp()

	// Add test device with minimal data
	deviceInfo := &models.DeviceInfo{
		Name: "Test Speaker",
		Type: "SoundTouch 30",
		NetworkInfo: []models.NetworkInfo{
			{MacAddress: "TEST123", IPAddress: "192.168.1.100"},
		},
	}

	device := &webtypes.DeviceConnection{
		Client:     nil, // No real client for unit tests
		DeviceInfo: deviceInfo,
		LastSeen:   time.Now(),
		Status: webtypes.DeviceStatus{
			Volume:       &models.Volume{ActualVolume: 50, MuteEnabled: false},
			Bass:         &models.Bass{ActualBass: 0},
			IsConnected:  true,
			LastActivity: time.Now(),
		},
	}

	app.AddDevice("test-device", device)
	return app
}

func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestNewWebApp(t *testing.T) {
	app := NewWebApp()

	// Use require-style checks that satisfy static analyzer
	if app == nil {
		t.Fatal("NewWebApp returned nil")
	}

	if count := app.DeviceCount(); count != 0 {
		t.Errorf("Expected empty device registry, got %d devices", count)
	}
}

func TestHandleAPIDevices(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()

	app.HandleAPIDevices(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		t.Errorf("Expected JSON content type, got %s", contentType)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !response.Success {
		t.Errorf("Expected success=true, got false")
	}

	// Check that devices data is present
	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map[string]interface{}")
	}

	if _, exists := data["test-device"]; !exists {
		t.Errorf("Expected 'test-device' in response data")
	}
}

func TestHandleAPIDevice(t *testing.T) {
	app := createTestApp()

	tests := []struct {
		name           string
		path           string
		chiID          string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "valid device",
			path:           "/api/device/test-device",
			chiID:          "test-device",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "missing device ID",
			path:           "/api/device/",
			chiID:          "",
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:           "unknown device",
			path:           "/api/device/unknown",
			chiID:          "unknown",
			expectedStatus: http.StatusNotFound,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			if tt.chiID != "" {
				req = withChiParams(req, map[string]string{"id": tt.chiID})
			}
			w := httptest.NewRecorder()

			app.HandleAPIDevice(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("Expected JSON content type, got %s", contentType)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success != tt.expectSuccess {
				t.Errorf("Expected success=%v, got %v", tt.expectSuccess, response.Success)
			}
		})
	}
}

func TestHandleAPIControl_InvalidDevice(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/control/unknown-device/play", nil)
	req = withChiParams(req, map[string]string{"id": "unknown-device", "action": "play"})
	w := httptest.NewRecorder()

	app.HandleAPIControl(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Errorf("Expected success=false, got true")
	}

	if response.Error != "Device not found" {
		t.Errorf("Expected 'Device not found' error, got '%s'", response.Error)
	}
}

func TestHandleAPIControl_InvalidPath(t *testing.T) {
	app := createTestApp()

	tests := []struct {
		name string
		path string
	}{
		{"missing action", "/api/control/test-device"},
		{"missing device and action", "/api/control/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			app.HandleAPIControl(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status 400, got %d", w.Code)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success {
				t.Errorf("Expected success=false, got true")
			}
		})
	}
}

func TestHandleAPIControl_VolumeValidation(t *testing.T) {
	app := createTestApp()

	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "invalid method",
			method:         "GET",
			body:           "",
			expectedStatus: http.StatusMethodNotAllowed,
			expectSuccess:  false,
		},
		{
			name:           "invalid JSON",
			method:         "POST",
			body:           `invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:           "volume too low",
			method:         "POST",
			body:           `{"level": -1}`,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:           "volume too high",
			method:         "POST",
			body:           `{"level": 101}`,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, "/api/control/test-device/volume", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/api/control/test-device/volume", nil)
			}
			req = withChiParams(req, map[string]string{"id": "test-device", "action": "volume"})
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			app.HandleAPIControl(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success != tt.expectSuccess {
				t.Errorf("Expected success=%v, got %v", tt.expectSuccess, response.Success)
			}
		})
	}
}

func TestHandleAPIControl_BassValidation(t *testing.T) {
	app := createTestApp()

	tests := []struct {
		name           string
		method         string
		body           string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "bass too low",
			method:         "POST",
			body:           `{"level": -10}`,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:           "bass too high",
			method:         "POST",
			body:           `{"level": 10}`,
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/control/test-device/bass", strings.NewReader(tt.body))
			req = withChiParams(req, map[string]string{"id": "test-device", "action": "bass"})
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			app.HandleAPIControl(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success != tt.expectSuccess {
				t.Errorf("Expected success=%v, got %v", tt.expectSuccess, response.Success)
			}
		})
	}
}

func TestHandleAPIControl_PresetValidation(t *testing.T) {
	app := createTestApp()

	tests := []struct {
		name           string
		query          string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "missing preset ID",
			query:          "",
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:           "invalid preset ID",
			query:          "?id=abc",
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/control/test-device/preset"+tt.query, nil)
			req = withChiParams(req, map[string]string{"id": "test-device", "action": "preset"})
			w := httptest.NewRecorder()

			app.HandleAPIControl(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success != tt.expectSuccess {
				t.Errorf("Expected success=%v, got %v", tt.expectSuccess, response.Success)
			}
		})
	}
}

func TestHandleAPIControl_SourceValidation(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/control/test-device/source", nil)
	req = withChiParams(req, map[string]string{"id": "test-device", "action": "source"})
	w := httptest.NewRecorder()

	app.HandleAPIControl(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Errorf("Expected success=false, got true")
	}

	if response.Error != "Source name required" {
		t.Errorf("Expected 'Source name required' error, got '%s'", response.Error)
	}
}

func TestHandleAPIDiscover(t *testing.T) {
	app := createTestApp()

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		expectSuccess  bool
	}{
		{
			name:           "valid POST request",
			method:         "POST",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "invalid GET request",
			method:         "GET",
			expectedStatus: http.StatusMethodNotAllowed,
			expectSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/discover", nil)
			w := httptest.NewRecorder()

			app.HandleAPIDiscover(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if response.Success != tt.expectSuccess {
				t.Errorf("Expected success=%v, got %v", tt.expectSuccess, response.Success)
			}
		})
	}
}

func TestSendError(t *testing.T) {
	app := createTestApp()

	w := httptest.NewRecorder()
	app.sendError(w, "Test error", http.StatusBadRequest)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Errorf("Expected success=false, got true")
	}

	if response.Error != "Test error" {
		t.Errorf("Expected 'Test error', got '%s'", response.Error)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}
}

func TestHandleWebSocket_InvalidUpgrade(t *testing.T) {
	app := createTestApp()

	// Test without proper WebSocket headers (should fail gracefully)
	req := httptest.NewRequest("GET", "/ws", nil)
	w := httptest.NewRecorder()

	// This will fail because it's not a real WebSocket upgrade, but should not panic
	app.HandleWebSocket(w, req)

	// We're just checking that the handler doesn't panic
	// The actual upgrade will fail in test environment without proper headers
}

func TestHandleAPIControl_UnsupportedAction(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/control/test-device/unsupported", nil)
	req = withChiParams(req, map[string]string{"id": "test-device", "action": "unsupported"})
	w := httptest.NewRecorder()

	app.HandleAPIControl(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Errorf("Expected success=false, got true")
	}

	if response.Error != "Unknown action" {
		t.Errorf("Expected 'Unknown action' error, got '%s'", response.Error)
	}
}

// Benchmark tests
func BenchmarkHandleAPIDevices(b *testing.B) {
	app := createTestApp()

	// Add more devices for realistic benchmarking
	for i := 0; i < 10; i++ {
		deviceID := "device-" + string(rune('0'+i))
		app.AddDevice(deviceID, &webtypes.DeviceConnection{
			Client:     &client.Client{},
			DeviceInfo: &models.DeviceInfo{Name: "Test Device " + deviceID},
			Status:     webtypes.DeviceStatus{IsConnected: true},
		})
	}

	req := httptest.NewRequest("GET", "/api/devices", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.HandleAPIDevices(w, req)
	}
}

func BenchmarkHandleAPIDevice(b *testing.B) {
	app := createTestApp()
	req := httptest.NewRequest("GET", "/api/device/test-device", nil)
	req = withChiParams(req, map[string]string{"id": "test-device"})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.HandleAPIDevice(w, req)
	}
}

func BenchmarkSendError(b *testing.B) {
	app := createTestApp()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.sendError(w, "Test error", http.StatusBadRequest)
	}
}
