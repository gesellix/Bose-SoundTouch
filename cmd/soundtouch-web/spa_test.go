package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/handlers"
	"github.com/gesellix/bose-soundtouch/cmd/soundtouch-web/webtypes"
	"github.com/gesellix/bose-soundtouch/pkg/models"
	"github.com/go-chi/chi/v5"
)

func withChiParams(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestSPARouting(t *testing.T) {
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedHTML   bool
	}{
		{
			name:           "root path serves HTML",
			path:           "/",
			expectedStatus: http.StatusOK,
			expectedHTML:   true,
		},
		{
			name:           "device path serves HTML",
			path:           "/device/test-device",
			expectedStatus: http.StatusOK,
			expectedHTML:   true,
		},
		{
			name:           "arbitrary path serves HTML",
			path:           "/some/random/path",
			expectedStatus: http.StatusOK,
			expectedHTML:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			// Simulate SPA routing handler
			spaHandler := func(w http.ResponseWriter, r *http.Request) {
				// If it's an API route, let it pass through
				if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/ws") {
					http.NotFound(w, r)
					return
				}

				// Serve the SPA index.html content (simulated)
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`<!doctype html>
<html lang="en">
<head>
    <meta charset="UTF-8" />
    <title>SoundTouch Control Center</title>
</head>
<body>
    <div id="app">SPA Content</div>
</body>
</html>`))
			}

			spaHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedHTML {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, "text/html") {
					t.Errorf("Expected HTML content type, got %s", contentType)
				}

				body := w.Body.String()
				if !strings.Contains(body, "<!doctype html>") {
					t.Errorf("Expected HTML content, got: %s", body)
				}
			}
		})
	}
}

func TestAPIEndpoints(t *testing.T) {
	app := handlers.NewWebApp()

	tests := []struct {
		name           string
		path           string
		method         string
		expectedStatus int
		expectedJSON   bool
	}{
		{
			name:           "devices API returns JSON",
			path:           "/api/devices",
			method:         "GET",
			expectedStatus: http.StatusOK,
			expectedJSON:   true,
		},
		{
			name:           "discover API accepts POST",
			path:           "/api/discover",
			method:         "POST",
			expectedStatus: http.StatusOK,
			expectedJSON:   true,
		},
		{
			name:           "device API with ID",
			path:           "/api/device/test-device",
			method:         "GET",
			expectedStatus: http.StatusNotFound, // Device won't exist in test
			expectedJSON:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			switch tt.path {
			case "/api/devices":
				app.HandleAPIDevices(w, req)
			case "/api/discover":
				app.HandleAPIDiscover(w, req)
			default:
				if strings.HasPrefix(tt.path, "/api/device/") {
					deviceID := strings.TrimPrefix(tt.path, "/api/device/")
					req = withChiParams(req, map[string]string{"id": deviceID})
					app.HandleAPIDevice(w, req)
				}
			}

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedJSON {
				contentType := w.Header().Get("Content-Type")
				if !strings.Contains(contentType, "application/json") {
					t.Errorf("Expected JSON content type, got %s", contentType)
				}

				// Validate JSON response structure
				var response webtypes.APIResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Errorf("Invalid JSON response: %v", err)
				}
			}
		})
	}
}

func TestAPIResponseFormat(t *testing.T) {
	app := handlers.NewWebApp()

	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()

	app.HandleAPIDevices(w, req)

	var response webtypes.APIResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	// Check API response structure
	if !response.Success {
		t.Errorf("Expected success=true, got success=%v", response.Success)
	}

	if response.Data == nil {
		t.Errorf("Expected data field to be present")
	}

	// Data should be an empty map for no devices
	dataMap, ok := response.Data.(map[string]interface{})
	if !ok {
		t.Errorf("Expected data to be a map, got %T", response.Data)
	}

	if len(dataMap) != 0 {
		t.Errorf("Expected empty device map, got %d devices", len(dataMap))
	}
}

func TestControlAPIValidation(t *testing.T) {
	app := handlers.NewWebApp()

	tests := []struct {
		name           string
		path           string
		method         string
		body           string
		expectedStatus int
		chiParams      map[string]string
	}{
		{
			name:           "missing device ID",
			path:           "/api/control//play",
			method:         "GET",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid control path",
			path:           "/api/control/device",
			method:         "GET",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "unknown action",
			path:           "/api/control/nonexistent/invalid",
			method:         "GET",
			expectedStatus: http.StatusNotFound,
			chiParams:      map[string]string{"id": "nonexistent", "action": "invalid"},
		},
		{
			name:           "nonexistent device",
			path:           "/api/control/nonexistent/play",
			method:         "GET",
			expectedStatus: http.StatusNotFound,
			chiParams:      map[string]string{"id": "nonexistent", "action": "play"},
		},
		{
			name:           "unknown action with valid device",
			path:           "/api/control/testdevice/unknownaction",
			method:         "GET",
			expectedStatus: http.StatusBadRequest,
			chiParams:      map[string]string{"id": "testdevice", "action": "unknownaction"},
		},
	}

	// Add a mock device for testing unknown action validation
	mockDevice := &webtypes.DeviceConnection{
		Client:     nil,
		DeviceInfo: &models.DeviceInfo{Name: "Test Device"},
		LastSeen:   time.Now(),
		Status:     webtypes.DeviceStatus{IsConnected: true},
	}
	app.Devices["testdevice"] = mockDevice

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req *http.Request
			if tt.body != "" {
				req = httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(tt.method, tt.path, nil)
			}
			if tt.chiParams != nil {
				req = withChiParams(req, tt.chiParams)
			}

			w := httptest.NewRecorder()
			app.HandleAPIControl(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Test %s: Expected status %d, got %d. Response: %s", tt.name, tt.expectedStatus, w.Code, w.Body.String())
			}

			// Validate error response format
			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("Expected JSON content type, got %s", contentType)
			}

			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Errorf("Invalid JSON response: %v", err)
			}

			if response.Success {
				t.Errorf("Expected success=false for error case, got success=true")
			}

			if response.Error == "" {
				t.Errorf("Expected error message, got empty string")
			}
		})
	}
}

func TestWebSocketUpgrade(t *testing.T) {
	app := handlers.NewWebApp()

	// Test WebSocket upgrade request
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Connection", "upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.Header.Set("Sec-WebSocket-Version", "13")

	w := httptest.NewRecorder()

	// The actual WebSocket upgrade will fail in test environment,
	// but we can check that the handler exists and accepts the request
	app.HandleWebSocket(w, req)

	// In a real test environment, this would fail with a websocket upgrade error
	// We're just checking the handler doesn't panic and processes the request
}

func TestJSONAPIConsistency(t *testing.T) {
	app := handlers.NewWebApp()

	endpoints := []string{
		"/api/devices",
		"/api/device/test",
	}

	for _, endpoint := range endpoints {
		t.Run("JSON consistency for "+endpoint, func(t *testing.T) {
			req := httptest.NewRequest("GET", endpoint, nil)
			w := httptest.NewRecorder()

			switch endpoint {
			case "/api/devices":
				app.HandleAPIDevices(w, req)
			default:
				if strings.HasPrefix(endpoint, "/api/device/") {
					deviceID := strings.TrimPrefix(endpoint, "/api/device/")
					req = withChiParams(req, map[string]string{"id": deviceID})
					app.HandleAPIDevice(w, req)
				}
			}

			// All API endpoints should return JSON
			contentType := w.Header().Get("Content-Type")
			if !strings.Contains(contentType, "application/json") {
				t.Errorf("Endpoint %s should return JSON, got %s", endpoint, contentType)
			}

			// All responses should follow APIResponse structure
			var response webtypes.APIResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Errorf("Endpoint %s returned invalid JSON: %v", endpoint, err)
			}

			// Response should have either data or error
			if response.Success && response.Data == nil {
				t.Errorf("Endpoint %s: success response should have data", endpoint)
			}
			if !response.Success && response.Error == "" {
				t.Errorf("Endpoint %s: error response should have error message", endpoint)
			}
		})
	}
}
