package models

import (
	"encoding/xml"
	"fmt"
	"testing"
)

func TestNewZoneRequest(t *testing.T) {
	masterID := "ABCD1234"
	zr := NewZoneRequest(masterID)

	if zr.Master != masterID {
		t.Errorf("Expected master %s, got %s", masterID, zr.Master)
	}

	if len(zr.Members) != 0 {
		t.Errorf("Expected 0 members, got %d", len(zr.Members))
	}
}

func TestZoneRequest_AddMember(t *testing.T) {
	zr := NewZoneRequest("MASTER123")

	zr.AddMember("DEVICE456", "192.168.1.10")
	zr.AddMember("DEVICE789", "192.168.1.11")

	if len(zr.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(zr.Members))
	}

	if zr.Members[0].DeviceID != "DEVICE456" {
		t.Errorf("Expected first member DEVICE456, got %s", zr.Members[0].DeviceID)
	}

	if zr.Members[0].IP != "192.168.1.10" {
		t.Errorf("Expected first member IP 192.168.1.10, got %s", zr.Members[0].IP)
	}
}

func TestZoneRequest_AddMemberByDeviceID(t *testing.T) {
	zr := NewZoneRequest("MASTER123")

	zr.AddMemberByDeviceID("DEVICE456")

	if len(zr.Members) != 1 {
		t.Errorf("Expected 1 member, got %d", len(zr.Members))
	}

	if zr.Members[0].DeviceID != "DEVICE456" {
		t.Errorf("Expected member DEVICE456, got %s", zr.Members[0].DeviceID)
	}

	if zr.Members[0].IP != "" {
		t.Errorf("Expected empty IP, got %s", zr.Members[0].IP)
	}
}

func TestZoneRequest_RemoveMember(t *testing.T) {
	zr := NewZoneRequest("MASTER123")
	zr.AddMember("DEVICE456", "192.168.1.10")
	zr.AddMember("DEVICE789", "192.168.1.11")
	zr.AddMember("DEVICEABC", "192.168.1.12")

	// Remove middle member
	zr.RemoveMember("DEVICE789")

	if len(zr.Members) != 2 {
		t.Errorf("Expected 2 members after removal, got %d", len(zr.Members))
	}

	// Check that the correct member was removed
	for _, member := range zr.Members {
		if member.DeviceID == "DEVICE789" {
			t.Error("DEVICE789 should have been removed")
		}
	}

	// Remove non-existent member (should not change anything)
	zr.RemoveMember("NONEXISTENT")
	if len(zr.Members) != 2 {
		t.Errorf("Expected 2 members after removing non-existent, got %d", len(zr.Members))
	}
}

func TestZoneRequest_ClearMembers(t *testing.T) {
	zr := NewZoneRequest("MASTER123")
	zr.AddMember("DEVICE456", "192.168.1.10")
	zr.AddMember("DEVICE789", "192.168.1.11")

	zr.ClearMembers()

	if len(zr.Members) != 0 {
		t.Errorf("Expected 0 members after clear, got %d", len(zr.Members))
	}
}

func TestZoneRequest_HasMember(t *testing.T) {
	zr := NewZoneRequest("MASTER123")
	zr.AddMember("DEVICE456", "192.168.1.10")

	if !zr.HasMember("DEVICE456") {
		t.Error("Expected HasMember to return true for DEVICE456")
	}

	if zr.HasMember("NONEXISTENT") {
		t.Error("Expected HasMember to return false for non-existent device")
	}

	if zr.HasMember("MASTER123") {
		t.Error("Expected HasMember to return false for master device")
	}
}

func TestZoneRequest_GetMemberCount(t *testing.T) {
	zr := NewZoneRequest("MASTER123")

	if zr.GetMemberCount() != 0 {
		t.Errorf("Expected 0 members initially, got %d", zr.GetMemberCount())
	}

	zr.AddMember("DEVICE456", "192.168.1.10")
	if zr.GetMemberCount() != 1 {
		t.Errorf("Expected 1 member, got %d", zr.GetMemberCount())
	}

	zr.AddMember("DEVICE789", "192.168.1.11")
	if zr.GetMemberCount() != 2 {
		t.Errorf("Expected 2 members, got %d", zr.GetMemberCount())
	}
}

func TestZoneRequest_Validate(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func() *ZoneRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "Valid zone request",
			setupFunc: func() *ZoneRequest {
				zr := NewZoneRequest("MASTER123")
				zr.AddMember("DEVICE456", "192.168.1.10")
				return zr
			},
			expectError: false,
		},
		{
			name: "Valid zone request without IP",
			setupFunc: func() *ZoneRequest {
				zr := NewZoneRequest("MASTER123")
				zr.AddMemberByDeviceID("DEVICE456")
				return zr
			},
			expectError: false,
		},
		{
			name: "Empty master device ID",
			setupFunc: func() *ZoneRequest {
				zr := &ZoneRequest{}
				return zr
			},
			expectError: true,
			errorMsg:    "master device ID is required",
		},
		{
			name: "Empty member device ID",
			setupFunc: func() *ZoneRequest {
				zr := NewZoneRequest("MASTER123")
				zr.Members = append(zr.Members, MemberEntry{DeviceID: ""})
				return zr
			},
			expectError: true,
			errorMsg:    "member device ID cannot be empty",
		},
		{
			name: "Duplicate member device ID",
			setupFunc: func() *ZoneRequest {
				zr := NewZoneRequest("MASTER123")
				zr.AddMember("DEVICE456", "192.168.1.10")
				zr.AddMember("DEVICE456", "192.168.1.11")
				return zr
			},
			expectError: true,
			errorMsg:    "duplicate device ID found: DEVICE456",
		},
		{
			name: "Master device ID as member",
			setupFunc: func() *ZoneRequest {
				zr := NewZoneRequest("MASTER123")
				zr.AddMember("MASTER123", "192.168.1.10")
				return zr
			},
			expectError: true,
			errorMsg:    "duplicate device ID found: MASTER123",
		},
		{
			name: "Invalid IP address",
			setupFunc: func() *ZoneRequest {
				zr := NewZoneRequest("MASTER123")
				zr.AddMember("DEVICE456", "invalid.ip.address")
				return zr
			},
			expectError: true,
			errorMsg:    "invalid IP address for device DEVICE456: invalid.ip.address",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zr := tt.setupFunc()
			err := zr.Validate()

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("Expected error '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, but got: %v", err)
				}
			}
		})
	}
}

func TestZoneInfo_IsStandalone(t *testing.T) {
	zi := &ZoneInfo{
		Master:  "MASTER123",
		Members: []Member{},
	}

	if !zi.IsStandalone() {
		t.Error("Expected standalone zone to return true")
	}

	zi.Members = append(zi.Members, Member{DeviceID: "DEVICE456"})
	if zi.IsStandalone() {
		t.Error("Expected zone with members to return false")
	}
}

func TestZoneInfo_IsMaster(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
		},
	}

	if !zi.IsMaster("MASTER123") {
		t.Error("Expected IsMaster to return true for master device")
	}

	if zi.IsMaster("DEVICE456") {
		t.Error("Expected IsMaster to return false for member device")
	}

	if zi.IsMaster("NONEXISTENT") {
		t.Error("Expected IsMaster to return false for non-existent device")
	}
}

func TestZoneInfo_IsMember(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
			{DeviceID: "DEVICE789"},
		},
	}

	if !zi.IsMember("DEVICE456") {
		t.Error("Expected IsMember to return true for member device")
	}

	if zi.IsMember("MASTER123") {
		t.Error("Expected IsMember to return false for master device")
	}

	if zi.IsMember("NONEXISTENT") {
		t.Error("Expected IsMember to return false for non-existent device")
	}
}

func TestZoneInfo_IsInZone(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
		},
	}

	if !zi.IsInZone("MASTER123") {
		t.Error("Expected IsInZone to return true for master device")
	}

	if !zi.IsInZone("DEVICE456") {
		t.Error("Expected IsInZone to return true for member device")
	}

	if zi.IsInZone("NONEXISTENT") {
		t.Error("Expected IsInZone to return false for non-existent device")
	}
}

func TestZoneInfo_GetMemberByDeviceID(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456", IP: "192.168.1.10"},
			{DeviceID: "DEVICE789", IP: "192.168.1.11"},
		},
	}

	member, found := zi.GetMemberByDeviceID("DEVICE456")
	if !found {
		t.Error("Expected to find member DEVICE456")
	}
	if member.IP != "192.168.1.10" {
		t.Errorf("Expected IP 192.168.1.10, got %s", member.IP)
	}

	_, found = zi.GetMemberByDeviceID("NONEXISTENT")
	if found {
		t.Error("Expected not to find non-existent member")
	}
}

func TestZoneInfo_GetMemberByIP(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456", IP: "192.168.1.10"},
			{DeviceID: "DEVICE789", IP: "192.168.1.11"},
		},
	}

	member, found := zi.GetMemberByIP("192.168.1.10")
	if !found {
		t.Error("Expected to find member by IP 192.168.1.10")
	}
	if member.DeviceID != "DEVICE456" {
		t.Errorf("Expected device ID DEVICE456, got %s", member.DeviceID)
	}

	_, found = zi.GetMemberByIP("192.168.1.99")
	if found {
		t.Error("Expected not to find member with non-existent IP")
	}
}

func TestZoneInfo_GetAllDeviceIDs(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
			{DeviceID: "DEVICE789"},
		},
	}

	deviceIDs := zi.GetAllDeviceIDs()
	expected := []string{"MASTER123", "DEVICE456", "DEVICE789"}

	if len(deviceIDs) != len(expected) {
		t.Errorf("Expected %d device IDs, got %d", len(expected), len(deviceIDs))
	}

	for i, expectedID := range expected {
		if deviceIDs[i] != expectedID {
			t.Errorf("Expected device ID %s at index %d, got %s", expectedID, i, deviceIDs[i])
		}
	}
}

func TestZoneInfo_GetTotalDeviceCount(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
			{DeviceID: "DEVICE789"},
		},
	}

	count := zi.GetTotalDeviceCount()
	if count != 3 {
		t.Errorf("Expected 3 devices, got %d", count)
	}

	// Test standalone
	zi.Members = []Member{}
	count = zi.GetTotalDeviceCount()
	if count != 1 {
		t.Errorf("Expected 1 device for standalone, got %d", count)
	}
}

func TestZoneInfo_GetZoneStatus(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
		},
	}

	// Test master device in zone
	status := zi.GetZoneStatus("MASTER123")
	if status != ZoneStatusMaster {
		t.Errorf("Expected ZoneStatusMaster, got %v", status)
	}

	// Test member device
	status = zi.GetZoneStatus("DEVICE456")
	if status != ZoneStatusSlave {
		t.Errorf("Expected ZoneStatusSlave, got %v", status)
	}

	// Test device not in zone
	status = zi.GetZoneStatus("NONEXISTENT")
	if status != ZoneStatusStandalone {
		t.Errorf("Expected ZoneStatusStandalone, got %v", status)
	}

	// Test standalone master
	zi.Members = []Member{}
	status = zi.GetZoneStatus("MASTER123")
	if status != ZoneStatusStandalone {
		t.Errorf("Expected ZoneStatusStandalone for standalone master, got %v", status)
	}
}

func TestZoneStatus_String(t *testing.T) {
	tests := []struct {
		status   ZoneStatus
		expected string
	}{
		{ZoneStatusStandalone, "Standalone"},
		{ZoneStatusMaster, "Zone Master"},
		{ZoneStatusSlave, "Zone Member"},
		{ZoneStatus("UNKNOWN"), "Unknown"},
	}

	for _, tt := range tests {
		result := tt.status.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestZoneInfo_String(t *testing.T) {
	// Test standalone
	zi := &ZoneInfo{
		Master:  "MASTER123",
		Members: []Member{},
	}

	result := zi.String()
	expected := "Standalone device: MASTER123"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}

	// Test zone with members
	zi.Members = []Member{
		{DeviceID: "DEVICE456"},
		{DeviceID: "DEVICE789"},
	}

	result = zi.String()
	expected = "Zone Master: MASTER123, Members: [DEVICE456, DEVICE789] (3 total devices)"
	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}

func TestZoneInfo_ToZoneRequest(t *testing.T) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456", IP: "192.168.1.10"},
			{DeviceID: "DEVICE789", IP: "192.168.1.11"},
		},
	}

	zr := zi.ToZoneRequest()

	if zr.Master != zi.Master {
		t.Errorf("Expected master %s, got %s", zi.Master, zr.Master)
	}

	if len(zr.Members) != len(zi.Members) {
		t.Errorf("Expected %d members, got %d", len(zi.Members), len(zr.Members))
	}

	for i, member := range zi.Members {
		if zr.Members[i].DeviceID != member.DeviceID {
			t.Errorf("Expected member %s, got %s", member.DeviceID, zr.Members[i].DeviceID)
		}
		if zr.Members[i].IP != member.IP {
			t.Errorf("Expected IP %s, got %s", member.IP, zr.Members[i].IP)
		}
	}
}

func TestZoneBuilder(t *testing.T) {
	zb := NewZoneBuilder("MASTER123")

	zr, err := zb.
		WithMember("DEVICE456", "192.168.1.10").
		WithMemberByDeviceID("DEVICE789").
		Build()

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	if zr.Master != "MASTER123" {
		t.Errorf("Expected master MASTER123, got %s", zr.Master)
	}

	if len(zr.Members) != 2 {
		t.Errorf("Expected 2 members, got %d", len(zr.Members))
	}

	if zr.Members[0].DeviceID != "DEVICE456" {
		t.Errorf("Expected first member DEVICE456, got %s", zr.Members[0].DeviceID)
	}

	if zr.Members[0].IP != "192.168.1.10" {
		t.Errorf("Expected first member IP 192.168.1.10, got %s", zr.Members[0].IP)
	}

	if zr.Members[1].DeviceID != "DEVICE789" {
		t.Errorf("Expected second member DEVICE789, got %s", zr.Members[1].DeviceID)
	}

	if zr.Members[1].IP != "" {
		t.Errorf("Expected second member empty IP, got %s", zr.Members[1].IP)
	}
}

func TestZoneBuilder_ValidationError(t *testing.T) {
	zb := NewZoneBuilder("")

	_, err := zb.Build()
	if err == nil {
		t.Error("Expected validation error for empty master ID")
	}
}

func TestZoneOperation_String(t *testing.T) {
	tests := []struct {
		op       ZoneOperation
		expected string
	}{
		{ZoneOpCreate, "Create Zone"},
		{ZoneOpModify, "Modify Zone"},
		{ZoneOpAddMember, "Add Member"},
		{ZoneOpRemove, "Remove Member"},
		{ZoneOpDissolve, "Dissolve Zone"},
		{ZoneOperation("UNKNOWN"), "Unknown Operation"},
	}

	for _, tt := range tests {
		result := tt.op.String()
		if result != tt.expected {
			t.Errorf("Expected %s, got %s", tt.expected, result)
		}
	}
}

func TestZoneError(t *testing.T) {
	tests := []struct {
		name        string
		op          ZoneOperation
		deviceID    string
		reason      string
		expectedMsg string
	}{
		{
			name:        "Error with device ID",
			op:          ZoneOpAddMember,
			deviceID:    "DEVICE123",
			reason:      "device offline",
			expectedMsg: "zone Add Member failed for device DEVICE123: device offline",
		},
		{
			name:        "Error without device ID",
			op:          ZoneOpCreate,
			deviceID:    "",
			reason:      "network error",
			expectedMsg: "zone Create Zone failed: network error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewZoneError(tt.op, tt.deviceID, tt.reason)

			if err.Error() != tt.expectedMsg {
				t.Errorf("Expected error message '%s', got '%s'", tt.expectedMsg, err.Error())
			}

			if err.Operation != tt.op {
				t.Errorf("Expected operation %v, got %v", tt.op, err.Operation)
			}

			if err.DeviceID != tt.deviceID {
				t.Errorf("Expected device ID '%s', got '%s'", tt.deviceID, err.DeviceID)
			}

			if err.Reason != tt.reason {
				t.Errorf("Expected reason '%s', got '%s'", tt.reason, err.Reason)
			}
		})
	}
}

func TestZoneCapabilities(t *testing.T) {
	caps := DefaultZoneCapabilities()

	if !caps.CanCreateZone() {
		t.Error("Expected default capabilities to allow zone creation")
	}

	if !caps.CanJoinZone() {
		t.Error("Expected default capabilities to allow joining zones")
	}

	// Test incapable device
	caps.SupportsMultiroom = false
	if caps.CanCreateZone() {
		t.Error("Expected device without multiroom support to not create zones")
	}

	if caps.CanJoinZone() {
		t.Error("Expected device without multiroom support to not join zones")
	}

	// Test master-only device
	caps.SupportsMultiroom = true
	caps.CanBeMember = false
	if caps.CanJoinZone() {
		t.Error("Expected device that can't be member to not join zones")
	}

	if !caps.CanCreateZone() {
		t.Error("Expected device that can be master to create zones")
	}
}

func TestZoneXMLMarshaling(t *testing.T) {
	t.Run("ZoneInfo XML Unmarshaling", func(t *testing.T) {
		xmlData := `<zone master="MASTER123">
			<member ipaddress="192.168.1.10">DEVICE456</member>
			<member ipaddress="192.168.1.11">DEVICE789</member>
		</zone>`

		var zi ZoneInfo
		err := xml.Unmarshal([]byte(xmlData), &zi)
		if err != nil {
			t.Fatalf("Failed to unmarshal XML: %v", err)
		}

		if zi.Master != "MASTER123" {
			t.Errorf("Expected master MASTER123, got %s", zi.Master)
		}

		if len(zi.Members) != 2 {
			t.Errorf("Expected 2 members, got %d", len(zi.Members))
		}

		if zi.Members[0].DeviceID != "DEVICE456" {
			t.Errorf("Expected first member DEVICE456, got %s", zi.Members[0].DeviceID)
		}

		if zi.Members[0].IP != "192.168.1.10" {
			t.Errorf("Expected first member IP 192.168.1.10, got %s", zi.Members[0].IP)
		}
	})

	t.Run("ZoneRequest XML Marshaling", func(t *testing.T) {
		zr := NewZoneRequest("MASTER123")
		zr.AddMember("DEVICE456", "192.168.1.10")
		zr.AddMember("DEVICE789", "192.168.1.11")

		data, err := xml.MarshalIndent(zr, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		expected := `<zone master="MASTER123">
  <member ipaddress="192.168.1.10">DEVICE456</member>
  <member ipaddress="192.168.1.11">DEVICE789</member>
</zone>`

		if string(data) != expected {
			t.Errorf("Expected XML:\n%s\n\nGot:\n%s", expected, string(data))
		}
	})

	t.Run("ZoneRequest XML Marshaling Without IP", func(t *testing.T) {
		zr := NewZoneRequest("MASTER123")
		zr.AddMemberByDeviceID("DEVICE456")

		data, err := xml.MarshalIndent(zr, "", "  ")
		if err != nil {
			t.Fatalf("Failed to marshal XML: %v", err)
		}

		expected := `<zone master="MASTER123">
  <member>DEVICE456</member>
</zone>`

		if string(data) != expected {
			t.Errorf("Expected XML:\n%s\n\nGot:\n%s", expected, string(data))
		}
	})

	t.Run("Empty Zone XML", func(t *testing.T) {
		xmlData := `<zone master="MASTER123"></zone>`

		var zi ZoneInfo
		err := xml.Unmarshal([]byte(xmlData), &zi)
		if err != nil {
			t.Fatalf("Failed to unmarshal XML: %v", err)
		}

		if zi.Master != "MASTER123" {
			t.Errorf("Expected master MASTER123, got %s", zi.Master)
		}

		if len(zi.Members) != 0 {
			t.Errorf("Expected 0 members, got %d", len(zi.Members))
		}

		if !zi.IsStandalone() {
			t.Error("Expected zone to be standalone")
		}
	})
}

func TestZoneEdgeCases(t *testing.T) {
	t.Run("Zone with many members", func(t *testing.T) {
		zr := NewZoneRequest("MASTER")
		for i := 1; i <= 10; i++ {
			deviceID := fmt.Sprintf("DEVICE%03d", i)
			ip := fmt.Sprintf("192.168.1.%d", i+10)
			zr.AddMember(deviceID, ip)
		}

		if err := zr.Validate(); err != nil {
			t.Errorf("Expected valid zone request, got error: %v", err)
		}

		if zr.GetMemberCount() != 10 {
			t.Errorf("Expected 10 members, got %d", zr.GetMemberCount())
		}
	})

	t.Run("Zone status for various scenarios", func(t *testing.T) {
		zi := &ZoneInfo{
			Master: "MASTER123",
			Members: []Member{
				{DeviceID: "DEVICE456"},
			},
		}

		// Test all status types
		if zi.GetZoneStatus("MASTER123") != ZoneStatusMaster {
			t.Error("Master should have ZoneStatusMaster")
		}

		if zi.GetZoneStatus("DEVICE456") != ZoneStatusSlave {
			t.Error("Member should have ZoneStatusSlave")
		}

		if zi.GetZoneStatus("OTHER") != ZoneStatusStandalone {
			t.Error("Unknown device should have ZoneStatusStandalone")
		}

		// Test standalone scenario
		zi.Members = []Member{}
		if zi.GetZoneStatus("MASTER123") != ZoneStatusStandalone {
			t.Error("Standalone master should have ZoneStatusStandalone")
		}
	})
}

func BenchmarkZoneInfo_GetAllDeviceIDs(b *testing.B) {
	zi := &ZoneInfo{
		Master: "MASTER123",
		Members: []Member{
			{DeviceID: "DEVICE456"},
			{DeviceID: "DEVICE789"},
			{DeviceID: "DEVICEABC"},
			{DeviceID: "DEVICEDEF"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = zi.GetAllDeviceIDs()
	}
}

func BenchmarkZoneRequest_Validate(b *testing.B) {
	zr := NewZoneRequest("MASTER123")
	for i := 0; i < 5; i++ {
		deviceID := fmt.Sprintf("DEVICE%d", i)
		ip := fmt.Sprintf("192.168.1.%d", i+10)
		zr.AddMember(deviceID, ip)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = zr.Validate()
	}
}
