// Package handlers contains tests for HTTP handlers.
package soundtouchweb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/client"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/gesellix/bose-soundtouch/pkg/service/soundtouchweb/webtypes"
	"github.com/go-chi/chi/v5"
)

func createTestApp() *WebApp {
	app := NewWebApp()

	// Add test device with minimal data
	deviceInfo := &models.DeviceInfo{
		Name: "Test Speaker",
		Type: "SoundTouch 30",
		NetworkInfo: []models.NetworkInfo{
			{MacAddress: "TEST123", IPAddress: "192.0.2.100"},
		},
	}

	device := webtypes.NewDeviceConnection(nil, deviceInfo)
	device.SetStatus(&webtypes.DeviceStatus{
		Volume:       &models.Volume{ActualVolume: 50, MuteEnabled: false},
		Bass:         &models.Bass{ActualBass: 0},
		IsConnected:  true,
		LastActivity: time.Now(),
	})

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

func TestStereoPairPersistenceClientHasBoundedLifecycleTimeout(t *testing.T) {
	app := NewWebApp()
	transport := &http.Transport{}
	app.ServiceClient = &http.Client{Transport: transport, Timeout: 10 * time.Second}

	configured := app.stereoPairPersistenceClient()
	if configured.Timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", configured.Timeout)
	}
	if configured.Transport != transport {
		t.Fatal("custom service transport was not preserved")
	}
	if app.ServiceClient.Timeout != 10*time.Second {
		t.Fatalf("source client timeout was mutated to %s", app.ServiceClient.Timeout)
	}
}

func TestHandleAPIDevices(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/control/devices", nil)
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

func TestHandleDeleteDevicePreservesConcurrentReplacement(t *testing.T) {
	app := NewWebApp()
	host := "speaker.local"
	original := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "ORIGINAL"})
	replacement := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "REPLACEMENT"})
	app.AddDevice(host, original)
	app.RemoveDeviceHook = func(deviceID string) error {
		if deviceID != "ORIGINAL" {
			t.Fatalf("RemoveDeviceHook deviceID = %q, want ORIGINAL", deviceID)
		}
		app.RemoveDevice(host)
		app.AddDevice(host, replacement)

		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/control/devices/"+host, nil)
	req = withChiParams(req, map[string]string{"id": host})
	w := httptest.NewRecorder()

	app.HandleDeleteDevice(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusConflict)
	}
	got, ok := app.GetDevice(host)
	if !ok || got != replacement {
		t.Fatal("concurrent replacement was removed")
	}

	app.RemoveDevice(host)
}

func TestHandleDeleteDeviceAcceptsHookReseedRemoval(t *testing.T) {
	app := NewWebApp()
	host := "speaker.local"
	original := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "ORIGINAL"})
	app.AddDevice(host, original)
	app.RemoveDeviceHook = func(deviceID string) error {
		if deviceID != "ORIGINAL" {
			t.Fatalf("RemoveDeviceHook deviceID = %q, want ORIGINAL", deviceID)
		}
		if !app.RemoveDevice(host) {
			t.Fatal("synchronous reseed did not remove original connection")
		}

		return nil
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/control/devices/"+host, nil)
	req = withChiParams(req, map[string]string{"id": host})
	w := httptest.NewRecorder()

	app.HandleDeleteDevice(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", w.Code, http.StatusOK, w.Body.String())
	}
	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.Success {
		t.Fatalf("response = %+v, want success", response)
	}
	if _, ok := app.GetDevice(host); ok {
		t.Fatal("removed device remained in registry")
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
			path:           "/api/control/devices/test-device",
			chiID:          "test-device",
			expectedStatus: http.StatusOK,
			expectSuccess:  true,
		},
		{
			name:           "missing device ID",
			path:           "/api/control/devices/",
			chiID:          "",
			expectedStatus: http.StatusBadRequest,
			expectSuccess:  false,
		},
		{
			name:           "unknown device",
			path:           "/api/control/devices/unknown",
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

func TestHandleAPIDevice_MasterIncludesStereoPairProjection(t *testing.T) {
	app := NewWebApp()
	group := testStereoGroup()
	app.AddDevice("192.0.2.10", projectionDevice("192.0.2.10", "left-id", "Living Room", true, group).Device)
	app.AddDevice("192.0.2.11", projectionDevice("192.0.2.11", "right-id", "Living Room", true, group).Device)

	req := httptest.NewRequest("GET", "/api/control/devices/192.0.2.10", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.10"})
	w := httptest.NewRecorder()

	app.HandleAPIDevice(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("response.Data = %#v, want a map", response.Data)
	}

	if _, ok := data["stereoPair"]; !ok {
		t.Fatalf("response.Data = %#v, want a stereoPair projection for the pair master", data)
	}
}

func TestHandleAPIDevice_HiddenPairMemberNotFound(t *testing.T) {
	app := NewWebApp()
	group := testStereoGroup()
	app.AddDevice("192.0.2.10", projectionDevice("192.0.2.10", "left-id", "Living Room", true, group).Device)
	app.AddDevice("192.0.2.11", projectionDevice("192.0.2.11", "right-id", "Living Room", true, group).Device)

	req := httptest.NewRequest("GET", "/api/control/devices/192.0.2.11", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.11"})
	w := httptest.NewRecorder()

	app.HandleAPIDevice(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("Expected status %d for a hidden pair member's own id, got %d", http.StatusNotFound, w.Code)
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Success {
		t.Fatal("Expected success=false for a hidden pair member's own id")
	}
}

func TestHandleAPIControl_InvalidDevice(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/control/devices/unknown-device/action/play", nil)
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
		{"missing action", "/api/control/devices/test-device/action/"},
		{"missing device and action", "/api/control/devices//action/"},
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
				req = httptest.NewRequest(tt.method, "/api/control/devices/test-device/volume/50", strings.NewReader(tt.body))
			} else {
				req = httptest.NewRequest(tt.method, "/api/control/devices/test-device/volume/50", nil)
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
			req := httptest.NewRequest(tt.method, "/api/control/devices/test-device/action/bass", strings.NewReader(tt.body))
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
			req := httptest.NewRequest("GET", "/api/control/devices/test-device/action/preset"+tt.query, nil)
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

	req := httptest.NewRequest("GET", "/api/control/devices/test-device/action/source", nil)
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
			req := httptest.NewRequest(tt.method, "/api/control/discover", nil)
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
	req := httptest.NewRequest("GET", "/api/control/ws", nil)
	w := httptest.NewRecorder()

	// This will fail because it's not a real WebSocket upgrade, but should not panic
	app.HandleWebSocket(w, req)

	// We're just checking that the handler doesn't panic
	// The actual upgrade will fail in test environment without proper headers
}

func TestHandleAPIControl_UnsupportedAction(t *testing.T) {
	app := createTestApp()

	req := httptest.NewRequest("GET", "/api/control/devices/test-device/action/unsupported", nil)
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

func TestHandleAPIVersion(t *testing.T) {
	app := createTestApp()
	app.Version = "1.2.3"
	app.Commit = "abcdef123"
	app.Date = "2023-01-01"
	app.RepoURL = "https://github.com/example/repo"

	req := httptest.NewRequest("GET", "/api/control/version", nil)
	w := httptest.NewRecorder()

	app.HandleAPIVersion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var resp webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected success=true, got false")
	}

	data, ok := resp.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data to be map[string]interface{}, got %T", resp.Data)
	}

	expected := map[string]string{
		"version":     "1.2.3",
		"commit":      "abcdef123",
		"date":        "2023-01-01",
		"repo_url":    "https://github.com/example/repo",
		"release_url": "https://github.com/example/repo/releases/tag/1.2.3",
		"commit_url":  "https://github.com/example/repo/commit/abcdef123",
	}

	for k, v := range expected {
		if data[k] != v {
			t.Errorf("Expected %s=%s, got %v", k, v, data[k])
		}
	}
}

// Benchmark tests
func BenchmarkHandleAPIDevices(b *testing.B) {
	app := createTestApp()

	// Add more devices for realistic benchmarking
	for i := 0; i < 10; i++ {
		deviceID := "device-" + string(rune('0'+i))
		conn := webtypes.NewDeviceConnection(&client.Client{}, &models.DeviceInfo{Name: "Test Device " + deviceID})
		conn.SetStatus(&webtypes.DeviceStatus{IsConnected: true})
		app.AddDevice(deviceID, conn)
	}

	req := httptest.NewRequest("GET", "/api/control/devices", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		app.HandleAPIDevices(w, req)
	}
}

func BenchmarkHandleAPIDevice(b *testing.B) {
	app := createTestApp()
	req := httptest.NewRequest("GET", "/api/control/devices/test-device", nil)
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

// TestHandleDevicePlay_SourceAccountFiltering verifies that a SourceAccount
// equal to Source (the placeholder speakers echo back, e.g. "TUNEIN") is
// stripped before the ContentItem XML is sent to the speaker, while a real
// credential (SourceAccount != Source) is preserved.
func TestHandleDevicePlay_SourceAccountFiltering(t *testing.T) {
	tests := []struct {
		name              string
		source            string
		sourceAccount     string
		wantSourceAccount string // empty means the XML attr must be absent
	}{
		{
			name:              "placeholder echoed back — stripped",
			source:            "TUNEIN",
			sourceAccount:     "TUNEIN",
			wantSourceAccount: "",
		},
		{
			name:              "real credential — preserved",
			source:            "TUNEIN",
			sourceAccount:     "real-account-id",
			wantSourceAccount: "real-account-id",
		},
		{
			name:              "empty account — stays empty",
			source:            "TUNEIN",
			sourceAccount:     "",
			wantSourceAccount: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody string

			// Fake speaker that captures the /select POST body.
			speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/select" {
					b, _ := io.ReadAll(r.Body)
					capturedBody = string(b)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer speaker.Close()

			speakerClient := client.NewClient(&client.Config{Host: speaker.URL})

			app := NewWebApp()
			deviceInfo := &models.DeviceInfo{Name: "Test Speaker"}
			conn := webtypes.NewDeviceConnection(speakerClient, deviceInfo)
			conn.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
			app.AddDevice("play-device", conn)

			body := strings.NewReader(`{
				"source":"` + tt.source + `",
				"type":"stationurl",
				"location":"/v1/playback/station/s6634",
				"sourceAccount":"` + tt.sourceAccount + `",
				"itemName":"Venice Classic Radio"
			}`)
			req := httptest.NewRequest("POST", "/api/control/devices/play-device/play", body)
			req.Header.Set("Content-Type", "application/json")
			req = withChiParams(req, map[string]string{"id": "play-device"})
			w := httptest.NewRecorder()

			app.HandleDevicePlay(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			// SourceAccount XML attribute is always emitted (no omitempty on the struct
			// tag), so check its value rather than its presence/absence.
			want := `sourceAccount="` + tt.wantSourceAccount + `"`
			if !strings.Contains(capturedBody, want) {
				t.Errorf("XML should contain %q, got: %s", want, capturedBody)
			}
		})
	}
}

// TestHandleSourceControl_LegacyGETForwardsAccount verifies that the temporary
// GET compatibility route still forwards sourceAccount while clearly marking
// the response deprecated.
func TestHandleSourceControl_LegacyGETForwardsAccount(t *testing.T) {
	tests := []struct {
		name              string
		query             string
		wantSourceAccount string
	}{
		{
			name:              "AUX with explicit account — forwarded",
			query:             "name=AUX&account=AUX1",
			wantSourceAccount: "AUX1",
		},
		{
			name:              "AUX without account — defaults to AUX",
			query:             "name=AUX",
			wantSourceAccount: "AUX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedBody string

			// Fake speaker that captures the /select POST body.
			speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/select" {
					b, _ := io.ReadAll(r.Body)
					capturedBody = string(b)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer speaker.Close()

			speakerClient := client.NewClient(&client.Config{Host: speaker.URL})

			app := NewWebApp()
			deviceInfo := &models.DeviceInfo{Name: "Test Speaker"}
			conn := webtypes.NewDeviceConnection(speakerClient, deviceInfo)
			conn.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
			app.AddDevice("source-device", conn)

			req := httptest.NewRequest("GET", "/api/control/devices/source-device/action/source?"+tt.query, nil)
			req = withChiParams(req, map[string]string{"id": "source-device", "action": "source"})
			w := httptest.NewRecorder()

			app.HandleAPIControl(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if got := w.Header().Get("Deprecation"); got != "true" {
				t.Errorf("Deprecation header = %q, want true", got)
			}
			if got := w.Header().Get("Warning"); !strings.Contains(got, "use POST") {
				t.Errorf("Warning header = %q, want POST migration guidance", got)
			}

			if want := `source="AUX"`; !strings.Contains(capturedBody, want) {
				t.Errorf("XML should contain %q, got: %s", want, capturedBody)
			}
			if want := `sourceAccount="` + tt.wantSourceAccount + `"`; !strings.Contains(capturedBody, want) {
				t.Errorf("XML should contain %q, got: %s", want, capturedBody)
			}
		})
	}
}

func TestHandleZoneAddRejectsSelf(t *testing.T) {
	app := NewWebApp()
	req := httptest.NewRequest("POST", "/api/control/devices/192.0.2.10/zone/add/192.0.2.10", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.10", "slaveId": "192.0.2.10"})
	w := httptest.NewRecorder()

	app.HandleZoneAdd(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cannot be added to its own zone") {
		t.Fatalf("unexpected response: %s", w.Body.String())
	}
}

func TestHandleZoneAddRejectsSameHardwareUnderDifferentKeys(t *testing.T) {
	app := NewWebApp()
	app.AddDevice("speaker.local", webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: "http://speaker.local"}),
		&models.DeviceInfo{Name: "Speaker", DeviceID: "SAMEHW01"},
	))
	app.AddDevice("192.0.2.10", webtypes.NewDeviceConnection(nil,
		&models.DeviceInfo{Name: "Speaker alias", DeviceID: "SAMEHW01"}))

	req := httptest.NewRequest("POST", "/api/control/devices/speaker.local/zone/add/192.0.2.10", nil)
	req = withChiParams(req, map[string]string{"id": "speaker.local", "slaveId": "192.0.2.10"})
	w := httptest.NewRecorder()
	app.HandleZoneAdd(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCurrentSourceAllowsMultiroom(t *testing.T) {
	sources := &models.Sources{SourceItem: []models.SourceItem{
		{Source: "SPOTIFY", SourceAccount: "first", MultiroomAllowed: true},
		{Source: "BLUETOOTH", MultiroomAllowed: false},
	}}

	for _, test := range []struct {
		name       string
		nowPlaying *models.NowPlaying
		allowed    bool
	}{
		{name: "matching account", nowPlaying: &models.NowPlaying{Source: "SPOTIFY", SourceAccount: "first"}, allowed: true},
		{name: "different account", nowPlaying: &models.NowPlaying{Source: "SPOTIFY", SourceAccount: "second"}},
		{name: "source disallows multiroom", nowPlaying: &models.NowPlaying{Source: "BLUETOOTH"}},
		{name: "standby", nowPlaying: &models.NowPlaying{Source: "STANDBY"}},
		{name: "missing state"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := currentSourceAllowsMultiroom(test.nowPlaying, sources); got != test.allowed {
				t.Fatalf("currentSourceAllowsMultiroom() = %t, want %t", got, test.allowed)
			}
		})
	}
}

func TestHandleZoneAddUsesSetZoneWithoutStartingPlayback(t *testing.T) {
	var paths []string
	var zoneBody string
	masterSpeaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/now_playing":
			_, _ = w.Write([]byte(`<nowPlaying deviceID="MASTERHW01" source="LOCAL_INTERNET_RADIO"><playStatus>PLAY_STATE</playStatus></nowPlaying>`))
		case "/sources":
			_, _ = w.Write([]byte(`<sources deviceID="MASTERHW01"><sourceItem source="LOCAL_INTERNET_RADIO" status="READY" isLocal="false" multiroomallowed="true" /></sources>`))
		case "/getZone":
			_, _ = w.Write([]byte(`<zone master="MASTERHW01"/>`))
		case "/setZone":
			body, _ := io.ReadAll(r.Body)
			zoneBody = string(body)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer masterSpeaker.Close()

	app := NewWebApp()
	master := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: masterSpeaker.URL}),
		&models.DeviceInfo{Name: "Master", DeviceID: "MASTERHW01"},
	)
	master.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
	app.AddDevice("192.0.2.10", master)
	app.AddDevice("192.0.2.20", webtypes.NewDeviceConnection(nil,
		&models.DeviceInfo{Name: "Slave", DeviceID: "SLAVEHW02"}))

	req := httptest.NewRequest("POST", "/api/control/devices/192.0.2.10/zone/add/192.0.2.20", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.10", "slaveId": "192.0.2.20"})
	w := httptest.NewRecorder()
	app.HandleZoneAdd(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	wantPaths := []string{"GET /now_playing", "GET /sources", "GET /getZone", "POST /setZone"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Fatalf("requests = %v, want %v", paths, wantPaths)
	}
	if !strings.Contains(zoneBody, "SLAVEHW02") {
		t.Fatalf("setZone body does not contain slave: %s", zoneBody)
	}
}

func TestHandleZoneAddRejectsStandbyMaster(t *testing.T) {
	var paths []string
	masterSpeaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/now_playing":
			_, _ = w.Write([]byte(`<nowPlaying deviceID="MASTERHW01" source="STANDBY"/>`))
		case "/sources":
			_, _ = w.Write([]byte(`<sources deviceID="MASTERHW01"><sourceItem source="LOCAL_INTERNET_RADIO" status="READY" multiroomallowed="true" /></sources>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer masterSpeaker.Close()

	app := NewWebApp()
	app.AddDevice("192.0.2.10", webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: masterSpeaker.URL}),
		&models.DeviceInfo{Name: "Master", DeviceID: "MASTERHW01"},
	))
	app.AddDevice("192.0.2.20", webtypes.NewDeviceConnection(nil,
		&models.DeviceInfo{Name: "Slave", DeviceID: "SLAVEHW02"}))

	req := httptest.NewRequest("POST", "/api/control/devices/192.0.2.10/zone/add/192.0.2.20", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.10", "slaveId": "192.0.2.20"})
	w := httptest.NewRecorder()
	app.HandleZoneAdd(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
	if want := []string{"GET /now_playing", "GET /sources"}; !reflect.DeepEqual(paths, want) {
		t.Fatalf("requests = %v, want %v", paths, want)
	}
}

func TestHandleSourceControl_CanonicalPOSTForwardsExactBody(t *testing.T) {
	var capturedBody string
	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/select" {
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer speaker.Close()

	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: speaker.URL}),
		&models.DeviceInfo{Name: "Test Speaker"},
	)
	conn.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
	app.AddDevice("source-device", conn)

	body := strings.NewReader(`{"source":"AUX","account":"AUX1"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/control/devices/source-device/action/source", body)
	req.Header.Set("Content-Type", "application/json")
	req = withChiParams(req, map[string]string{"id": "source-device", "action": "source"})
	w := httptest.NewRecorder()

	app.HandleAPIControl(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Deprecation"); got != "" {
		t.Errorf("canonical POST unexpectedly marked deprecated: %q", got)
	}
	for _, want := range []string{`source="AUX"`, `sourceAccount="AUX1"`} {
		if !strings.Contains(capturedBody, want) {
			t.Errorf("XML should contain %q, got: %s", want, capturedBody)
		}
	}
}

func TestHandleSourceControl_CanonicalPOSTRejectsUnknownFields(t *testing.T) {
	app := NewWebApp()
	conn := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{Name: "Test Speaker"})
	app.AddDevice("source-device", conn)

	body := strings.NewReader(`{"source":"AUX","account":"AUX1","name":"legacy"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/control/devices/source-device/action/source", body)
	req = withChiParams(req, map[string]string{"id": "source-device", "action": "source"})
	w := httptest.NewRecorder()

	app.HandleAPIControl(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// TestHandleZoneRemove_UsesRemoveZoneSlave is the #511 regression: removing one
// member from a multi-member zone must target that member via /removeZoneSlave.
// The previous implementation rebuilt the zone with /setZone and the remaining
// members, which the speaker only honoured when the resulting member set was
// empty — so removing one of several members appeared to do nothing, while
// removing the last member (empty set == dissolve) worked.
func TestHandleZoneRemove_UsesRemoveZoneSlave(t *testing.T) {
	var gotPath, gotBody string

	speaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer speaker.Close()

	app := NewWebApp()

	// Registry is IP-keyed; use RFC-5737 documentation addresses.
	master := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: speaker.URL}),
		&models.DeviceInfo{Name: "Master", DeviceID: "MASTERHW01"},
	)
	master.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
	app.AddDevice("192.0.2.10", master)

	slave := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{Name: "Slave", DeviceID: "SLAVEHW02"})
	app.AddDevice("192.0.2.20", slave)

	req := httptest.NewRequest("POST", "/api/control/devices/192.0.2.10/zone/remove/192.0.2.20", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.10", "slaveId": "192.0.2.20"})
	w := httptest.NewRecorder()

	app.HandleZoneRemove(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if gotPath != "/removeZoneSlave" {
		t.Errorf("expected POST to /removeZoneSlave, got %q (a /setZone rebuild does not drop a member from a multi-member zone)", gotPath)
	}

	if !strings.Contains(gotBody, "SLAVEHW02") {
		t.Errorf("removeZoneSlave body should target the slave device ID, got: %s", gotBody)
	}

	if !strings.Contains(gotBody, `master="MASTERHW01"`) {
		t.Errorf("removeZoneSlave body should name the master, got: %s", gotBody)
	}
}

// TestHandleZoneLeave_UsesRemoveZoneSlave is the #511 regression for the slave's
// "Leave zone" path: it must drop the slave via /removeZoneSlave on the master,
// not rebuild the master's zone with /setZone (which leaves a 3+ device zone
// unchanged). The leaving slave's "id" is its IP; it carries the master's hwID
// in its /getZone, which we resolve to the master's registry entry.
func TestHandleZoneLeave_UsesRemoveZoneSlave(t *testing.T) {
	var masterPath, masterBody string

	// Master speaker captures the /removeZoneSlave call.
	masterSpeaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		masterPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		masterBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer masterSpeaker.Close()

	// Slave speaker answers /getZone naming the master by hwID.
	slaveSpeaker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/getZone" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8" ?>
<zone master="MASTERHW01">
	<member ipaddress="192.0.2.20">SLAVEHW02</member>
	<member ipaddress="192.0.2.30">SLAVEHW03</member>
</zone>`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer slaveSpeaker.Close()

	app := NewWebApp()

	master := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: masterSpeaker.URL}),
		&models.DeviceInfo{Name: "Master", DeviceID: "MASTERHW01"},
	)
	master.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
	app.AddDevice("192.0.2.10", master)

	slave := webtypes.NewDeviceConnection(
		client.NewClient(&client.Config{Host: slaveSpeaker.URL}),
		&models.DeviceInfo{Name: "Slave", DeviceID: "SLAVEHW02"},
	)
	slave.SetStatus(&webtypes.DeviceStatus{IsConnected: true, LastActivity: time.Now()})
	app.AddDevice("192.0.2.20", slave)

	req := httptest.NewRequest("POST", "/api/control/devices/192.0.2.20/zone/leave", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.20"})
	w := httptest.NewRecorder()

	app.HandleZoneLeave(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if masterPath != "/removeZoneSlave" {
		t.Errorf("expected POST to master's /removeZoneSlave, got %q", masterPath)
	}

	if !strings.Contains(masterBody, "SLAVEHW02") {
		t.Errorf("removeZoneSlave body should target the leaving slave, got: %s", masterBody)
	}

	if !strings.Contains(masterBody, `master="MASTERHW01"`) {
		t.Errorf("removeZoneSlave body should name the master, got: %s", masterBody)
	}
}

// TestHandleGetZoneCandidates_IncludesHiddenPairMemberAndSelf guards two
// things the stereo-pair projection (device_projection.go) must not affect:
// Zone and Group are separate, unrelated groupings, so (a) a stereo pair's
// hidden non-master member -- absent from the collapsed "devices" list --
// must still be a valid zone-add candidate, matching how HandleZoneAdd
// already treats it (raw registry lookup, unaffected by projection); and
// (b) the endpoint itself does not exclude the requesting {id} device,
// since deciding what to exclude (this page's own device, current zone
// members, ...) is the caller's concern -- Zone.js already does this via
// the existing zoneIps set, which includes the master's own IP even for a
// standalone zone (see models.ZoneInfo.IsStandalone).
func TestHandleGetZoneCandidates_IncludesHiddenPairMemberAndSelf(t *testing.T) {
	app := NewWebApp()

	group := testStereoGroup()
	master := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "left-id", Name: "Living Room", IPAddress: "192.0.2.10"})
	master.SetStatus(&webtypes.DeviceStatus{IsConnected: true, Group: group})
	app.AddDevice("192.0.2.10", master)

	hiddenMember := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "right-id", Name: "Living Room", IPAddress: "192.0.2.11"})
	hiddenMember.SetStatus(&webtypes.DeviceStatus{IsConnected: true, Group: group})
	app.AddDevice("192.0.2.11", hiddenMember)

	standalone := webtypes.NewDeviceConnection(nil, &models.DeviceInfo{DeviceID: "kitchen-id", Name: "Kitchen", IPAddress: "192.0.2.12"})
	standalone.SetStatus(&webtypes.DeviceStatus{IsConnected: true})
	app.AddDevice("192.0.2.12", standalone)

	// Sanity check: the hidden member really is absent from the projected
	// device list this test is guarding against leaking into.
	projected := app.deviceViewSnapshot()
	if _, visible := projected["192.0.2.11"]; visible {
		t.Fatal("test setup: expected 192.0.2.11 to be hidden by the stereo-pair projection")
	}

	req := httptest.NewRequest("GET", "/api/control/devices/192.0.2.10/zone/candidates", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.10"})
	w := httptest.NewRecorder()

	app.HandleGetZoneCandidates(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be a map, got %T", response.Data)
	}

	for _, ip := range []string{"192.0.2.10", "192.0.2.11", "192.0.2.12"} {
		if _, ok := data[ip]; !ok {
			t.Errorf("expected %s in zone candidates, got: %+v", ip, data)
		}
	}
}

func TestHandleGetZoneCandidates_UnknownDeviceNotFound(t *testing.T) {
	app := NewWebApp()

	req := httptest.NewRequest("GET", "/api/control/devices/192.0.2.99/zone/candidates", nil)
	req = withChiParams(req, map[string]string{"id": "192.0.2.99"})
	w := httptest.NewRecorder()

	app.HandleGetZoneCandidates(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for an unknown device, got %d: %s", w.Code, w.Body.String())
	}
}
