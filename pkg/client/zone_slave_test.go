package client

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func TestClient_AddZoneSlave(t *testing.T) {
	tests := []struct {
		name           string
		masterID       string
		slaveID        string
		slaveIP        string
		responseStatus int
		responseBody   string
		expectError    bool
		expectedPath   string
	}{
		{
			name:           "successful add zone slave with IP",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    false,
			expectedPath:   "/addZoneSlave",
		},
		{
			name:           "successful add zone slave without IP",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    false,
			expectedPath:   "/addZoneSlave",
		},
		{
			name:           "server error response",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusInternalServerError,
			responseBody:   `<error>Internal Server Error</error>`,
			expectError:    true,
			expectedPath:   "/addZoneSlave",
		},
		{
			name:           "empty master device ID",
			masterID:       "",
			slaveID:        "SLAVE456",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    true,
			expectedPath:   "/addZoneSlave",
		},
		{
			name:           "empty slave device ID",
			masterID:       "MASTER123",
			slaveID:        "",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    true,
			expectedPath:   "/addZoneSlave",
		},
		{
			name:           "invalid slave IP address",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "invalid-ip",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    true,
			expectedPath:   "/addZoneSlave",
		},
		{
			name:           "same master and slave device ID",
			masterID:       "MASTER123",
			slaveID:        "MASTER123",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    true,
			expectedPath:   "/addZoneSlave",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				receivedMethod string
				receivedPath   string
				receivedBody   string
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				receivedPath = r.URL.Path

				if r.Method == "POST" {
					body := make([]byte, r.ContentLength)
					_, _ = r.Body.Read(body)
					receivedBody = string(body)
				}

				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := createTestClient(server.URL)

			err := client.AddZoneSlave(tt.masterID, tt.slaveID, tt.slaveIP)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error but got: %v", err)
				return
			}

			// Verify request details for successful cases
			if receivedMethod != "POST" {
				t.Errorf("Expected POST request, got %s", receivedMethod)
			}

			if receivedPath != tt.expectedPath {
				t.Errorf("Expected path %s, got %s", tt.expectedPath, receivedPath)
			}

			// Verify the XML contains the expected elements
			if !strings.Contains(receivedBody, `<zone master="`) {
				t.Error("Expected XML to contain zone with master attribute")
			}

			if !strings.Contains(receivedBody, tt.masterID) {
				t.Errorf("Expected XML to contain master ID %s", tt.masterID)
			}

			if !strings.Contains(receivedBody, tt.slaveID) {
				t.Errorf("Expected XML to contain slave ID %s", tt.slaveID)
			}

			if tt.slaveIP != "" && !strings.Contains(receivedBody, tt.slaveIP) {
				t.Errorf("Expected XML to contain slave IP %s", tt.slaveIP)
			}
		})
	}
}

func TestClient_AddZoneSlaveByDeviceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.URL.Path != "/addZoneSlave" {
			t.Errorf("Expected path /addZoneSlave, got %s", r.URL.Path)
		}

		// Read and verify body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, `MASTER123`) {
			t.Error("Expected XML to contain master ID MASTER123")
		}

		if !strings.Contains(bodyStr, `SLAVE456`) {
			t.Error("Expected XML to contain slave ID SLAVE456")
		}

		// Should not contain IP address attribute when not provided
		if strings.Contains(bodyStr, `ipaddress=""`) {
			t.Error("Expected XML to not contain empty ipaddress attribute")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<status>OK</status>`))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.AddZoneSlaveByDeviceID("MASTER123", "SLAVE456")
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
}

func TestClient_RemoveZoneSlave(t *testing.T) {
	tests := []struct {
		name           string
		masterID       string
		slaveID        string
		slaveIP        string
		responseStatus int
		responseBody   string
		expectError    bool
		expectedPath   string
	}{
		{
			name:           "successful remove zone slave with IP",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    false,
			expectedPath:   "/removeZoneSlave",
		},
		{
			name:           "successful remove zone slave without IP",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "",
			responseStatus: http.StatusOK,
			responseBody:   `<status>OK</status>`,
			expectError:    false,
			expectedPath:   "/removeZoneSlave",
		},
		{
			name:           "server error response",
			masterID:       "MASTER123",
			slaveID:        "SLAVE456",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusBadRequest,
			responseBody:   `<error>Bad Request</error>`,
			expectError:    true,
			expectedPath:   "/removeZoneSlave",
		},
		{
			name:           "device not found",
			masterID:       "MASTER123",
			slaveID:        "NONEXISTENT",
			slaveIP:        "192.0.2.101",
			responseStatus: http.StatusNotFound,
			responseBody:   `<error>Device not found</error>`,
			expectError:    true,
			expectedPath:   "/removeZoneSlave",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				receivedMethod string
				receivedPath   string
				receivedBody   string
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				receivedPath = r.URL.Path

				if r.Method == "POST" {
					body := make([]byte, r.ContentLength)
					_, _ = r.Body.Read(body)
					receivedBody = string(body)
				}

				w.WriteHeader(tt.responseStatus)
				_, _ = w.Write([]byte(tt.responseBody))
			}))
			defer server.Close()

			client := createTestClient(server.URL)

			err := client.RemoveZoneSlave(tt.masterID, tt.slaveID, tt.slaveIP)

			// Check error expectation
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}

				return
			}

			if err != nil {
				t.Errorf("Expected no error but got: %v", err)
				return
			}

			// Verify request details for successful cases
			if receivedMethod != "POST" {
				t.Errorf("Expected POST request, got %s", receivedMethod)
			}

			if receivedPath != tt.expectedPath {
				t.Errorf("Expected path %s, got %s", tt.expectedPath, receivedPath)
			}

			// Verify the XML contains the expected elements
			if !strings.Contains(receivedBody, `<zone master="`) {
				t.Error("Expected XML to contain zone with master attribute")
			}

			if !strings.Contains(receivedBody, tt.masterID) {
				t.Errorf("Expected XML to contain master ID %s", tt.masterID)
			}

			if !strings.Contains(receivedBody, tt.slaveID) {
				t.Errorf("Expected XML to contain slave ID %s", tt.slaveID)
			}

			if tt.slaveIP != "" && !strings.Contains(receivedBody, tt.slaveIP) {
				t.Errorf("Expected XML to contain slave IP %s", tt.slaveIP)
			}
		})
	}
}

func TestClient_RemoveZoneSlaveByDeviceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		if r.URL.Path != "/removeZoneSlave" {
			t.Errorf("Expected path /removeZoneSlave, got %s", r.URL.Path)
		}

		// Read and verify body
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, `MASTER123`) {
			t.Error("Expected XML to contain master ID MASTER123")
		}

		if !strings.Contains(bodyStr, `SLAVE456`) {
			t.Error("Expected XML to contain slave ID SLAVE456")
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`<status>OK</status>`))
	}))
	defer server.Close()

	client := createTestClient(server.URL)

	err := client.RemoveZoneSlaveByDeviceID("MASTER123", "SLAVE456")
	if err != nil {
		t.Errorf("Expected no error but got: %v", err)
	}
}

func TestZoneSlaveRequest_Validation(t *testing.T) {
	tests := []struct {
		name        string
		request     *models.ZoneSlaveRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid request with IP",
			request: &models.ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: "192.0.2.101"},
				},
			},
			expectError: false,
		},
		{
			name: "valid request without IP",
			request: &models.ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: ""},
				},
			},
			expectError: false,
		},
		{
			name: "empty master ID",
			request: &models.ZoneSlaveRequest{
				Master: "",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: "192.0.2.101"},
				},
			},
			expectError: true,
			errorMsg:    "master device ID is required",
		},
		{
			name: "no members",
			request: &models.ZoneSlaveRequest{
				Master:  "MASTER123",
				Members: []models.ZoneSlaveEntry{},
			},
			expectError: true,
			errorMsg:    "zone slave operations require exactly one member",
		},
		{
			name: "multiple members",
			request: &models.ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: "192.0.2.101"},
					{DeviceID: "SLAVE789", IP: "192.0.2.102"},
				},
			},
			expectError: true,
			errorMsg:    "zone slave operations require exactly one member",
		},
		{
			name: "empty slave device ID",
			request: &models.ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "", IP: "192.0.2.101"},
				},
			},
			expectError: true,
			errorMsg:    "slave device ID cannot be empty",
		},
		{
			name: "same master and slave ID",
			request: &models.ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "MASTER123", IP: "192.0.2.101"},
				},
			},
			expectError: true,
			errorMsg:    "slave device ID cannot be the same as master",
		},
		{
			name: "invalid IP address",
			request: &models.ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []models.ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: "invalid-ip"},
				},
			},
			expectError: true,
			errorMsg:    "invalid IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
					return
				}

				if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestZoneSlaveRequest_HelperMethods(t *testing.T) {
	t.Run("GetSlaveDeviceID", func(t *testing.T) {
		request := models.NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		deviceID := request.GetSlaveDeviceID()
		if deviceID != "SLAVE456" {
			t.Errorf("Expected device ID 'SLAVE456', got '%s'", deviceID)
		}
	})

	t.Run("GetSlaveIP", func(t *testing.T) {
		request := models.NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		ip := request.GetSlaveIP()
		if ip != "192.0.2.101" {
			t.Errorf("Expected IP '192.0.2.101', got '%s'", ip)
		}
	})

	t.Run("GetSlaveDeviceID with no members", func(t *testing.T) {
		request := models.NewZoneSlaveRequest("MASTER123")

		deviceID := request.GetSlaveDeviceID()
		if deviceID != "" {
			t.Errorf("Expected empty device ID, got '%s'", deviceID)
		}
	})

	t.Run("String representation", func(t *testing.T) {
		request := models.NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		str := request.String()

		expected := "Zone slave operation: master=MASTER123, slave=SLAVE456 (192.0.2.101)"
		if str != expected {
			t.Errorf("Expected string '%s', got '%s'", expected, str)
		}
	})

	t.Run("String representation without IP", func(t *testing.T) {
		request := models.NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "")

		str := request.String()

		expected := "Zone slave operation: master=MASTER123, slave=SLAVE456"
		if str != expected {
			t.Errorf("Expected string '%s', got '%s'", expected, str)
		}
	})
}

func TestClient_ZoneSlaveOperations_NetworkError(t *testing.T) {
	// Create client with invalid host to trigger network error
	config := DefaultConfig()
	config.Host = "invalid-host-that-does-not-exist"
	config.Port = 9999
	client := NewClient(config)

	// Test AddZoneSlave with network error
	err := client.AddZoneSlave("MASTER123", "SLAVE456", "192.0.2.101")
	if err == nil {
		t.Errorf("Expected network error for AddZoneSlave but got none")
	}

	// Test RemoveZoneSlave with network error
	err = client.RemoveZoneSlave("MASTER123", "SLAVE456", "192.0.2.101")
	if err == nil {
		t.Errorf("Expected network error for RemoveZoneSlave but got none")
	}

	// Test AddZoneSlaveByDeviceID with network error
	err = client.AddZoneSlaveByDeviceID("MASTER123", "SLAVE456")
	if err == nil {
		t.Errorf("Expected network error for AddZoneSlaveByDeviceID but got none")
	}

	// Test RemoveZoneSlaveByDeviceID with network error
	err = client.RemoveZoneSlaveByDeviceID("MASTER123", "SLAVE456")
	if err == nil {
		t.Errorf("Expected network error for RemoveZoneSlaveByDeviceID but got none")
	}
}
