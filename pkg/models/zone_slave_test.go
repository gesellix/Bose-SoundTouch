package models

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestZoneSlaveRequest_Creation(t *testing.T) {
	t.Run("NewZoneSlaveRequest", func(t *testing.T) {
		masterID := "MASTER123"
		request := NewZoneSlaveRequest(masterID)

		if request.Master != masterID {
			t.Errorf("Expected master ID '%s', got '%s'", masterID, request.Master)
		}

		if len(request.Members) != 0 {
			t.Errorf("Expected empty members slice, got %d members", len(request.Members))
		}
	})

	t.Run("AddSlave", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		if len(request.Members) != 1 {
			t.Errorf("Expected 1 member, got %d", len(request.Members))
			return
		}

		member := request.Members[0]
		if member.DeviceID != "SLAVE456" {
			t.Errorf("Expected device ID 'SLAVE456', got '%s'", member.DeviceID)
		}

		if member.IP != "192.0.2.101" {
			t.Errorf("Expected IP '192.0.2.101', got '%s'", member.IP)
		}
	})
}

func TestZoneSlaveRequest_Validation(t *testing.T) {
	tests := []struct {
		name        string
		masterID    string
		members     []ZoneSlaveEntry
		expectError bool
		errorMsg    string
	}{
		{
			name:     "valid request with IP",
			masterID: "MASTER123",
			members: []ZoneSlaveEntry{
				{DeviceID: "SLAVE456", IP: "192.0.2.101"},
			},
			expectError: false,
		},
		{
			name:     "valid request without IP",
			masterID: "MASTER123",
			members: []ZoneSlaveEntry{
				{DeviceID: "SLAVE456", IP: ""},
			},
			expectError: false,
		},
		{
			name:        "empty master ID",
			masterID:    "",
			members:     []ZoneSlaveEntry{{DeviceID: "SLAVE456", IP: "192.0.2.101"}},
			expectError: true,
			errorMsg:    "master device ID is required",
		},
		{
			name:        "no members",
			masterID:    "MASTER123",
			members:     []ZoneSlaveEntry{},
			expectError: true,
			errorMsg:    "zone slave operations require exactly one member",
		},
		{
			name:     "multiple members",
			masterID: "MASTER123",
			members: []ZoneSlaveEntry{
				{DeviceID: "SLAVE456", IP: "192.0.2.101"},
				{DeviceID: "SLAVE789", IP: "192.0.2.102"},
			},
			expectError: true,
			errorMsg:    "zone slave operations require exactly one member",
		},
		{
			name:        "empty slave device ID",
			masterID:    "MASTER123",
			members:     []ZoneSlaveEntry{{DeviceID: "", IP: "192.0.2.101"}},
			expectError: true,
			errorMsg:    "slave device ID cannot be empty",
		},
		{
			name:        "same master and slave ID",
			masterID:    "MASTER123",
			members:     []ZoneSlaveEntry{{DeviceID: "MASTER123", IP: "192.0.2.101"}},
			expectError: true,
			errorMsg:    "slave device ID cannot be the same as master",
		},
		{
			name:        "invalid IP address",
			masterID:    "MASTER123",
			members:     []ZoneSlaveEntry{{DeviceID: "SLAVE456", IP: "invalid-ip"}},
			expectError: true,
			errorMsg:    "invalid IP address",
		},
		{
			name:        "malformed IP address",
			masterID:    "MASTER123",
			members:     []ZoneSlaveEntry{{DeviceID: "SLAVE456", IP: "300.300.300.300"}},
			expectError: true,
			errorMsg:    "invalid IP address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := &ZoneSlaveRequest{
				Master:  tt.masterID,
				Members: tt.members,
			}

			err := request.Validate()

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
	t.Run("GetSlaveDeviceID with member", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		deviceID := request.GetSlaveDeviceID()

		expected := "SLAVE456"
		if deviceID != expected {
			t.Errorf("Expected device ID '%s', got '%s'", expected, deviceID)
		}
	})

	t.Run("GetSlaveDeviceID with no members", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")

		deviceID := request.GetSlaveDeviceID()
		if deviceID != "" {
			t.Errorf("Expected empty device ID, got '%s'", deviceID)
		}
	})

	t.Run("GetSlaveIP with member", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		ip := request.GetSlaveIP()

		expected := "192.0.2.101"
		if ip != expected {
			t.Errorf("Expected IP '%s', got '%s'", expected, ip)
		}
	})

	t.Run("GetSlaveIP with no members", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")

		ip := request.GetSlaveIP()
		if ip != "" {
			t.Errorf("Expected empty IP, got '%s'", ip)
		}
	})

	t.Run("GetSlaveIP with empty IP", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "")

		ip := request.GetSlaveIP()
		if ip != "" {
			t.Errorf("Expected empty IP, got '%s'", ip)
		}
	})
}

func TestZoneSlaveRequest_String(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *ZoneSlaveRequest
		expected string
	}{
		{
			name: "with IP address",
			setup: func() *ZoneSlaveRequest {
				req := NewZoneSlaveRequest("MASTER123")
				req.AddSlave("SLAVE456", "192.0.2.101")

				return req
			},
			expected: "Zone slave operation: master=MASTER123, slave=SLAVE456 (192.0.2.101)",
		},
		{
			name: "without IP address",
			setup: func() *ZoneSlaveRequest {
				req := NewZoneSlaveRequest("MASTER123")
				req.AddSlave("SLAVE456", "")

				return req
			},
			expected: "Zone slave operation: master=MASTER123, slave=SLAVE456",
		},
		{
			name: "no members",
			setup: func() *ZoneSlaveRequest {
				return NewZoneSlaveRequest("MASTER123")
			},
			expected: "Zone slave operation on master MASTER123 (no slave specified)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := tt.setup()
			result := request.String()

			if result != tt.expected {
				t.Errorf("Expected string '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestZoneSlaveRequest_XMLMarshaling(t *testing.T) {
	t.Run("marshal with IP", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")

		xmlData, err := xml.Marshal(request)
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		xmlStr := string(xmlData)

		// Check for expected XML elements
		if !strings.Contains(xmlStr, `<zone master="MASTER123">`) {
			t.Error("Expected XML to contain zone element with master attribute")
		}

		if !strings.Contains(xmlStr, `<member ipaddress="192.0.2.101">SLAVE456</member>`) {
			t.Error("Expected XML to contain member with IP address")
		}
	})

	t.Run("marshal without IP", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "")

		xmlData, err := xml.Marshal(request)
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		xmlStr := string(xmlData)

		// Check for expected XML elements
		if !strings.Contains(xmlStr, `<zone master="MASTER123">`) {
			t.Error("Expected XML to contain zone element with master attribute")
		}

		if !strings.Contains(xmlStr, `<member>SLAVE456</member>`) {
			t.Error("Expected XML to contain member without IP address")
		}

		// Should not contain empty ipaddress attribute
		if strings.Contains(xmlStr, `ipaddress=""`) {
			t.Error("Expected XML to not contain empty ipaddress attribute")
		}
	})
}

func TestZoneSlaveRequest_XMLUnmarshaling(t *testing.T) {
	tests := []struct {
		name        string
		xmlData     string
		expectedReq *ZoneSlaveRequest
		expectError bool
	}{
		{
			name:    "valid XML with IP",
			xmlData: `<zone master="MASTER123"><member ipaddress="192.0.2.101">SLAVE456</member></zone>`,
			expectedReq: &ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: "192.0.2.101"},
				},
			},
			expectError: false,
		},
		{
			name:    "valid XML without IP",
			xmlData: `<zone master="MASTER123"><member>SLAVE456</member></zone>`,
			expectedReq: &ZoneSlaveRequest{
				Master: "MASTER123",
				Members: []ZoneSlaveEntry{
					{DeviceID: "SLAVE456", IP: ""},
				},
			},
			expectError: false,
		},
		{
			name:        "invalid XML",
			xmlData:     `<zone master="MASTER123"><member>SLAVE456</member>`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var request ZoneSlaveRequest

			err := xml.Unmarshal([]byte(tt.xmlData), &request)

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

			// Compare the unmarshaled request with expected
			if request.Master != tt.expectedReq.Master {
				t.Errorf("Expected master '%s', got '%s'", tt.expectedReq.Master, request.Master)
			}

			if len(request.Members) != len(tt.expectedReq.Members) {
				t.Errorf("Expected %d members, got %d", len(tt.expectedReq.Members), len(request.Members))
				return
			}

			for i, expectedMember := range tt.expectedReq.Members {
				member := request.Members[i]
				if member.DeviceID != expectedMember.DeviceID {
					t.Errorf("Expected member %d device ID '%s', got '%s'", i, expectedMember.DeviceID, member.DeviceID)
				}

				if member.IP != expectedMember.IP {
					t.Errorf("Expected member %d IP '%s', got '%s'", i, expectedMember.IP, member.IP)
				}
			}
		})
	}
}

func TestZoneSlaveEntry_XMLMarshaling(t *testing.T) {
	t.Run("entry with IP", func(t *testing.T) {
		entry := ZoneSlaveEntry{
			DeviceID: "SLAVE456",
			IP:       "192.0.2.101",
		}

		xmlData, err := xml.Marshal(entry)
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		xmlStr := string(xmlData)

		expected := `<member ipaddress="192.0.2.101">SLAVE456</member>`
		if xmlStr != expected {
			t.Errorf("Expected XML '%s', got '%s'", expected, xmlStr)
		}
	})

	t.Run("entry without IP", func(t *testing.T) {
		entry := ZoneSlaveEntry{
			DeviceID: "SLAVE456",
			IP:       "",
		}

		xmlData, err := xml.Marshal(entry)
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		xmlStr := string(xmlData)

		expected := `<member>SLAVE456</member>`
		if xmlStr != expected {
			t.Errorf("Expected XML '%s', got '%s'", expected, xmlStr)
		}
	})
}

func TestZoneSlaveRequest_EdgeCases(t *testing.T) {
	t.Run("multiple AddSlave calls", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "192.0.2.101")
		request.AddSlave("SLAVE789", "192.0.2.102")

		if len(request.Members) != 2 {
			t.Errorf("Expected 2 members, got %d", len(request.Members))
		}

		// Should fail validation due to multiple members
		err := request.Validate()
		if err == nil {
			t.Error("Expected validation error for multiple members but got none")
		}
	})

	t.Run("IPv6 address", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "2001:db8::1")

		err := request.Validate()
		if err != nil {
			t.Errorf("Expected no error for IPv6 address but got: %v", err)
		}
	})

	t.Run("localhost IP", func(t *testing.T) {
		request := NewZoneSlaveRequest("MASTER123")
		request.AddSlave("SLAVE456", "127.0.0.1")

		err := request.Validate()
		if err != nil {
			t.Errorf("Expected no error for localhost IP but got: %v", err)
		}
	})
}
