// Package handlers contains tests for HTTP handlers.
package soundtouchweb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

// TestHandleSourceControl_ForwardsAccount verifies that the account query
// parameter is forwarded as sourceAccount in the /select XML. Devices like
// the ST-5 expose multiple AUX jacks that share source="AUX" and are only
// disambiguated by distinct sourceAccount values (AUX, AUX1, …). Regression
// test for issue #444, where the handler dropped the account parameter.
func TestHandleSourceControl_ForwardsAccount(t *testing.T) {
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

			if want := `source="AUX"`; !strings.Contains(capturedBody, want) {
				t.Errorf("XML should contain %q, got: %s", want, capturedBody)
			}
			if want := `sourceAccount="` + tt.wantSourceAccount + `"`; !strings.Contains(capturedBody, want) {
				t.Errorf("XML should contain %q, got: %s", want, capturedBody)
			}
		})
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
