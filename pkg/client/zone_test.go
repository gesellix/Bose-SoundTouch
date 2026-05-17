package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_GetZone(t *testing.T) {
	tests := []struct {
		name            string
		responseXML     string
		responseStatus  int
		expectError     bool
		expectedMaster  string
		expectedMembers int
	}{
		{
			name: "Standalone device",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH"></zone>`,
			responseStatus:  http.StatusOK,
			expectError:     false,
			expectedMaster:  "ABCD1234EFGH",
			expectedMembers: 0,
		},
		{
			name: "Zone with members",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
	<member ipaddress="192.0.2.12">IJKL9012MNOP</member>
</zone>`,
			responseStatus:  http.StatusOK,
			expectError:     false,
			expectedMaster:  "ABCD1234EFGH",
			expectedMembers: 2,
		},
		{
			name:            "Server error",
			responseXML:     `<error>Server Error</error>`,
			responseStatus:  http.StatusInternalServerError,
			expectError:     true,
			expectedMaster:  "",
			expectedMembers: 0,
		},
		{
			name: "Device not found",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<errors deviceID="ABCD1234EFGH">
	<error value="7" name="DEVICE_NOT_FOUND_ERROR">Device not found</error>
</errors>`,
			responseStatus:  http.StatusNotFound,
			expectError:     true,
			expectedMaster:  "",
			expectedMembers: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/getZone" {
					t.Errorf("Expected path /getZone, got %s", r.URL.Path)
				}

				if r.Method != http.MethodGet {
					t.Errorf("Expected GET method, got %s", r.Method)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseXML))
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			zone, err := client.GetZone()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if zone.Master != tt.expectedMaster {
				t.Errorf("Expected master %s, got %s", tt.expectedMaster, zone.Master)
			}

			if len(zone.Members) != tt.expectedMembers {
				t.Errorf("Expected %d members, got %d", tt.expectedMembers, len(zone.Members))
			}
		})
	}
}

func TestClient_SetZone(t *testing.T) {
	tests := []struct {
		name           string
		zoneRequest    *models.ZoneRequest
		responseStatus int
		expectError    bool
		errorMessage   string
	}{
		{
			name: "Valid zone request",
			zoneRequest: func() *models.ZoneRequest {
				zr := models.NewZoneRequest("ABCD1234EFGH")
				zr.AddMember("EFGH5678IJKL", "192.0.2.11")

				return zr
			}(),
			responseStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:           "Standalone zone request",
			zoneRequest:    models.NewZoneRequest("ABCD1234EFGH"),
			responseStatus: http.StatusOK,
			expectError:    false,
		},
		{
			name:         "Invalid zone request - empty master",
			zoneRequest:  &models.ZoneRequest{},
			expectError:  true,
			errorMessage: "invalid zone request: master device ID is required",
		},
		{
			name: "Invalid zone request - duplicate device",
			zoneRequest: func() *models.ZoneRequest {
				zr := models.NewZoneRequest("ABCD1234EFGH")
				zr.AddMember("ABCD1234EFGH", "192.0.2.10") // Same as master

				return zr
			}(),
			expectError:  true,
			errorMessage: "invalid zone request: duplicate device ID found: ABCD1234EFGH",
		},
		{
			name:           "Server error response",
			zoneRequest:    models.NewZoneRequest("ABCD1234EFGH"),
			responseStatus: http.StatusInternalServerError,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/setZone" {
					t.Errorf("Expected path /setZone, got %s", r.URL.Path)
				}

				if r.Method != http.MethodPost {
					t.Errorf("Expected POST method, got %s", r.Method)
				}

				// Check Content-Type
				contentType := r.Header.Get("Content-Type")
				if contentType != "application/xml" {
					t.Errorf("Expected Content-Type application/xml, got %s", contentType)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tt.responseStatus)

				if tt.responseStatus != http.StatusOK {
					_, _ = w.Write([]byte(`<error>Server Error</error>`))
				}
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			err := client.SetZone(tt.zoneRequest)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				} else if tt.errorMessage != "" && err.Error() != tt.errorMessage {
					t.Errorf("Expected error message '%s', got '%s'", tt.errorMessage, err.Error())
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestClient_CreateZone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setZone" {
			t.Errorf("Expected path /setZone, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	masterDeviceID := "ABCD1234EFGH"
	memberDeviceIDs := []string{"EFGH5678IJKL", "IJKL9012MNOP"}

	err := client.CreateZone(masterDeviceID, memberDeviceIDs)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
}

func TestClient_CreateZoneWithIPs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/setZone" {
			t.Errorf("Expected path /setZone, got %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	masterDeviceID := "ABCD1234EFGH"
	members := map[string]string{
		"EFGH5678IJKL": "192.0.2.11",
		"IJKL9012MNOP": "192.0.2.12",
	}

	err := client.CreateZoneWithIPs(masterDeviceID, members)
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}
}

func TestClient_AddToZone(t *testing.T) {
	getZoneCalled := false
	setZoneCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		if r.URL.Path == "/getZone" && r.Method == http.MethodGet {
			getZoneCalled = true
			// Return existing zone
			response := `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
</zone>`

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else if r.URL.Path == "/setZone" && r.Method == http.MethodPost {
			setZoneCalled = true

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.AddToZone("IJKL9012MNOP", "192.0.2.12")
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	if !getZoneCalled {
		t.Error("Expected GetZone to be called")
	}

	if !setZoneCalled {
		t.Error("Expected SetZone to be called")
	}
}

func TestClient_RemoveFromZone(t *testing.T) {
	getZoneCalled := false
	setZoneCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		if r.URL.Path == "/getZone" && r.Method == http.MethodGet {
			getZoneCalled = true
			// Return existing zone with members
			response := `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
	<member ipaddress="192.0.2.12">IJKL9012MNOP</member>
</zone>`

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else if r.URL.Path == "/setZone" && r.Method == http.MethodPost {
			setZoneCalled = true

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.RemoveFromZone("EFGH5678IJKL")
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	if !getZoneCalled {
		t.Error("Expected GetZone to be called")
	}

	if !setZoneCalled {
		t.Error("Expected SetZone to be called")
	}
}

func TestClient_DissolveZone(t *testing.T) {
	getZoneCalled := false
	setZoneCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")

		if r.URL.Path == "/getZone" && r.Method == http.MethodGet {
			getZoneCalled = true
			// Return existing zone with members
			response := `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
	<member ipaddress="192.0.2.12">IJKL9012MNOP</member>
</zone>`

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(response))
		} else if r.URL.Path == "/setZone" && r.Method == http.MethodPost {
			setZoneCalled = true

			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.DissolveZone()
	if err != nil {
		t.Errorf("Expected no error, but got: %v", err)
	}

	if !getZoneCalled {
		t.Error("Expected GetZone to be called")
	}

	if !setZoneCalled {
		t.Error("Expected SetZone to be called")
	}
}

func TestClient_IsInZone(t *testing.T) {
	tests := []struct {
		name           string
		responseXML    string
		expectedResult bool
		expectError    bool
	}{
		{
			name: "Standalone device",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH"></zone>`,
			expectedResult: false,
			expectError:    false,
		},
		{
			name: "Device in zone",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
</zone>`,
			expectedResult: true,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/getZone" {
					t.Errorf("Expected path /getZone, got %s", r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseXML))
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			result, err := client.IsInZone()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if result != tt.expectedResult {
				t.Errorf("Expected result %t, got %t", tt.expectedResult, result)
			}
		})
	}
}

func TestClient_GetZoneStatus(t *testing.T) {
	tests := []struct {
		name           string
		deviceInfoXML  string
		zoneXML        string
		expectedStatus models.ZoneStatus
		expectError    bool
	}{
		{
			name: "Standalone device",
			deviceInfoXML: `<?xml version="1.0" encoding="UTF-8" ?>
<info deviceID="ABCD1234EFGH">
	<name>Living Room</name>
	<type>SoundTouch 20</type>
</info>`,
			zoneXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH"></zone>`,
			expectedStatus: models.ZoneStatusStandalone,
			expectError:    false,
		},
		{
			name: "Zone master",
			deviceInfoXML: `<?xml version="1.0" encoding="UTF-8" ?>
<info deviceID="ABCD1234EFGH">
	<name>Living Room</name>
	<type>SoundTouch 20</type>
</info>`,
			zoneXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
</zone>`,
			expectedStatus: models.ZoneStatusMaster,
			expectError:    false,
		},
		{
			name: "Zone member",
			deviceInfoXML: `<?xml version="1.0" encoding="UTF-8" ?>
<info deviceID="EFGH5678IJKL">
	<name>Kitchen</name>
	<type>SoundTouch 10</type>
</info>`,
			zoneXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
</zone>`,
			expectedStatus: models.ZoneStatusSlave,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)

				switch r.URL.Path {
				case "/getZone":
					_, _ = w.Write([]byte(tt.zoneXML))
				case "/info":
					_, _ = w.Write([]byte(tt.deviceInfoXML))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			status, err := client.GetZoneStatus()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if status != tt.expectedStatus {
				t.Errorf("Expected status %v, got %v", tt.expectedStatus, status)
			}
		})
	}
}

func TestClient_GetZoneMembers(t *testing.T) {
	tests := []struct {
		name            string
		responseXML     string
		expectedMembers []string
		expectError     bool
	}{
		{
			name: "Standalone device",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH"></zone>`,
			expectedMembers: []string{"ABCD1234EFGH"},
			expectError:     false,
		},
		{
			name: "Zone with members",
			responseXML: `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
	<member ipaddress="192.0.2.12">IJKL9012MNOP</member>
</zone>`,
			expectedMembers: []string{"ABCD1234EFGH", "EFGH5678IJKL", "IJKL9012MNOP"},
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/getZone" {
					t.Errorf("Expected path /getZone, got %s", r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.responseXML))
			}))
			defer server.Close()

			client := createTestClient(server.URL)
			members, err := client.GetZoneMembers()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error, but got: %v", err)
				return
			}

			if len(members) != len(tt.expectedMembers) {
				t.Errorf("Expected %d members, got %d", len(tt.expectedMembers), len(members))
				return
			}

			for i, expectedMember := range tt.expectedMembers {
				if members[i] != expectedMember {
					t.Errorf("Expected member %s at index %d, got %s", expectedMember, i, members[i])
				}
			}
		})
	}
}

func TestClient_Zone_ErrorHandling(t *testing.T) {
	t.Run("GetZone network error", func(t *testing.T) {
		client := NewClientFromHost("invalid.host:9999")
		client.timeout = 100 * time.Millisecond

		_, err := client.GetZone()
		if err == nil {
			t.Error("Expected network error, but got none")
		}
	})

	t.Run("SetZone network error", func(t *testing.T) {
		client := NewClientFromHost("invalid.host:9999")
		client.timeout = 100 * time.Millisecond

		zr := models.NewZoneRequest("DEVICE123")

		err := client.SetZone(zr)
		if err == nil {
			t.Error("Expected network error, but got none")
		}
	})

	t.Run("AddToZone - GetZone fails", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/getZone" {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`<error>Server Error</error>`))
			}
		}))
		defer server.Close()

		client := createTestClient(server.URL)

		err := client.AddToZone("DEVICE456", "192.0.2.10")
		if err == nil {
			t.Error("Expected error when GetZone fails, but got none")
		}

		expectedErrorPrefix := "failed to get current zone:"
		if !strings.Contains(err.Error(), expectedErrorPrefix) {
			t.Errorf("Expected error to contain '%s', got: %v", expectedErrorPrefix, err)
		}
	})
}

// Benchmark tests
func BenchmarkClient_GetZone(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		response := `<?xml version="1.0" encoding="UTF-8" ?>
<zone master="ABCD1234EFGH">
	<member ipaddress="192.0.2.11">EFGH5678IJKL</member>
	<member ipaddress="192.0.2.12">IJKL9012MNOP</member>
</zone>`

		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := client.GetZone()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClient_SetZone(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := createTestClient(server.URL)
	zoneRequest := models.NewZoneRequest("ABCD1234EFGH")
	zoneRequest.AddMember("EFGH5678IJKL", "192.0.2.11")

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		err := client.SetZone(zoneRequest)
		if err != nil {
			b.Fatal(err)
		}
	}
}
