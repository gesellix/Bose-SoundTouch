package models

import (
	"encoding/xml"
	"fmt"
	"net"
	"strings"
)

// ZoneInfo represents the response from GET /getZone endpoint
type ZoneInfo struct {
	XMLName xml.Name `xml:"zone"`
	Master  string   `xml:"master,attr"`
	Members []Member `xml:"member"`
}

// Member represents a device member in a multiroom zone
type Member struct {
	XMLName  xml.Name `xml:"member"`
	DeviceID string   `xml:",chardata"`
	IP       string   `xml:"ipaddress,attr"`
}

// ZoneRequest represents the request for POST /setZone endpoint
type ZoneRequest struct {
	XMLName xml.Name      `xml:"zone"`
	Master  string        `xml:"master,attr"`
	Members []MemberEntry `xml:"member"`
}

// MemberEntry represents a member entry in zone configuration requests
type MemberEntry struct {
	XMLName  xml.Name `xml:"member"`
	DeviceID string   `xml:",chardata"`
	IP       string   `xml:"ipaddress,attr,omitempty"`
}

// ZoneStatus represents possible zone states
type ZoneStatus string

const (
	// ZoneStatusStandalone indicates the device is operating independently
	ZoneStatusStandalone ZoneStatus = "STANDALONE"
	// ZoneStatusMaster indicates the device is the master in a zone
	ZoneStatusMaster ZoneStatus = "MASTER"
	// ZoneStatusSlave indicates the device is a slave in a zone
	ZoneStatusSlave ZoneStatus = "SLAVE"
)

// String returns a human-readable string representation
func (zs ZoneStatus) String() string {
	switch zs {
	case ZoneStatusStandalone:
		return "Standalone"
	case ZoneStatusMaster:
		return "Zone Master"
	case ZoneStatusSlave:
		return "Zone Member"
	default:
		return "Unknown"
	}
}

// NewZoneRequest creates a new zone configuration request
func NewZoneRequest(masterDeviceID string) *ZoneRequest {
	return &ZoneRequest{
		Master:  masterDeviceID,
		Members: []MemberEntry{},
	}
}

// AddMember adds a device to the zone configuration
func (zr *ZoneRequest) AddMember(deviceID, ipAddress string) {
	member := MemberEntry{
		DeviceID: deviceID,
		IP:       ipAddress,
	}
	zr.Members = append(zr.Members, member)
}

// AddMemberByDeviceID adds a device to the zone by device ID only
func (zr *ZoneRequest) AddMemberByDeviceID(deviceID string) {
	member := MemberEntry{
		DeviceID: deviceID,
	}
	zr.Members = append(zr.Members, member)
}

// GetMemberCount returns the number of members in the zone
func (zr *ZoneRequest) GetMemberCount() int {
	return len(zr.Members)
}

// Validate validates the zone request
func (zr *ZoneRequest) Validate() error {
	if zr.Master == "" {
		return fmt.Errorf("master device ID is required")
	}

	// Check for duplicate device IDs
	seen := make(map[string]bool)
	seen[zr.Master] = true

	for _, member := range zr.Members {
		if member.DeviceID == "" {
			return fmt.Errorf("member device ID cannot be empty")
		}

		if seen[member.DeviceID] {
			return fmt.Errorf("duplicate device ID found: %s", member.DeviceID)
		}

		seen[member.DeviceID] = true

		// Validate IP address if provided
		if member.IP != "" {
			if net.ParseIP(member.IP) == nil {
				return fmt.Errorf("invalid IP address for device %s: %s", member.DeviceID, member.IP)
			}
		}
	}

	return nil
}

// IsStandalone returns true if this is a standalone (single device) configuration
func (zi *ZoneInfo) IsStandalone() bool {
	return len(zi.Members) == 0
}

// IsMaster returns true if the given device ID is the zone master
func (zi *ZoneInfo) IsMaster(deviceID string) bool {
	return zi.Master == deviceID
}

// IsMember returns true if the given device ID is a zone member (not master)
func (zi *ZoneInfo) IsMember(deviceID string) bool {
	for _, member := range zi.Members {
		if member.DeviceID == deviceID {
			return true
		}
	}

	return false
}

// IsInZone returns true if the given device ID is in the zone (master or member)
func (zi *ZoneInfo) IsInZone(deviceID string) bool {
	return zi.IsMaster(deviceID) || zi.IsMember(deviceID)
}

// GetMemberByDeviceID returns the member with the given device ID
func (zi *ZoneInfo) GetMemberByDeviceID(deviceID string) (*Member, bool) {
	for _, member := range zi.Members {
		if member.DeviceID == deviceID {
			return &member, true
		}
	}

	return nil, false
}

// GetMemberByIP returns the member with the given IP address
func (zi *ZoneInfo) GetMemberByIP(ipAddress string) (*Member, bool) {
	for _, member := range zi.Members {
		if member.IP == ipAddress {
			return &member, true
		}
	}

	return nil, false
}

// GetAllDeviceIDs returns all device IDs in the zone (master + members)
func (zi *ZoneInfo) GetAllDeviceIDs() []string {
	devices := []string{zi.Master}
	for _, member := range zi.Members {
		devices = append(devices, member.DeviceID)
	}

	return devices
}

// GetTotalDeviceCount returns the total number of devices in the zone
func (zi *ZoneInfo) GetTotalDeviceCount() int {
	return 1 + len(zi.Members) // Master + members
}

// GetZoneStatus returns the zone status for a given device ID
func (zi *ZoneInfo) GetZoneStatus(deviceID string) ZoneStatus {
	if zi.IsMaster(deviceID) {
		if zi.IsStandalone() {
			return ZoneStatusStandalone
		}

		return ZoneStatusMaster
	}

	if zi.IsMember(deviceID) {
		return ZoneStatusSlave
	}

	return ZoneStatusStandalone // Device not in zone
}

// String returns a human-readable string representation of the zone
func (zi *ZoneInfo) String() string {
	if zi.IsStandalone() {
		return fmt.Sprintf("Standalone device: %s", zi.Master)
	}

	var memberIDs []string
	for _, member := range zi.Members {
		memberIDs = append(memberIDs, member.DeviceID)
	}

	return fmt.Sprintf("Zone Master: %s, Members: [%s] (%d total devices)",
		zi.Master, strings.Join(memberIDs, ", "), zi.GetTotalDeviceCount())
}

// ToZoneRequest converts ZoneInfo to a ZoneRequest for modification
func (zi *ZoneInfo) ToZoneRequest() *ZoneRequest {
	request := NewZoneRequest(zi.Master)
	for _, member := range zi.Members {
		request.AddMember(member.DeviceID, member.IP)
	}

	return request
}

// ZoneOperation represents different types of zone operations
type ZoneOperation string

const (
	// ZoneOpCreate indicates creating a new zone
	ZoneOpCreate ZoneOperation = "CREATE"
	// ZoneOpModify indicates modifying an existing zone
	ZoneOpModify ZoneOperation = "MODIFY"
	// ZoneOpAddMember indicates adding a member to a zone
	ZoneOpAddMember ZoneOperation = "ADD_MEMBER"
	// ZoneOpRemove indicates removing a member from a zone
	ZoneOpRemove ZoneOperation = "REMOVE_MEMBER"
	// ZoneOpDissolve indicates dissolving a zone
	ZoneOpDissolve ZoneOperation = "DISSOLVE"
)

// String returns a human-readable string representation
func (zo ZoneOperation) String() string {
	switch zo {
	case ZoneOpCreate:
		return "Create Zone"
	case ZoneOpModify:
		return "Modify Zone"
	case ZoneOpAddMember:
		return "Add Member"
	case ZoneOpRemove:
		return "Remove Member"
	case ZoneOpDissolve:
		return "Dissolve Zone"
	default:
		return "Unknown Operation"
	}
}

// ZoneBuilder provides a fluent interface for building zone configurations
type ZoneBuilder struct {
	request *ZoneRequest
}

// NewZoneBuilder creates a new zone builder with the specified master device
func NewZoneBuilder(masterDeviceID string) *ZoneBuilder {
	return &ZoneBuilder{
		request: NewZoneRequest(masterDeviceID),
	}
}

// WithMember adds a member to the zone configuration
func (zb *ZoneBuilder) WithMember(deviceID, ipAddress string) *ZoneBuilder {
	zb.request.AddMember(deviceID, ipAddress)
	return zb
}

// WithMemberByDeviceID adds a member by device ID only
func (zb *ZoneBuilder) WithMemberByDeviceID(deviceID string) *ZoneBuilder {
	zb.request.AddMemberByDeviceID(deviceID)
	return zb
}

// Build returns the constructed zone request
func (zb *ZoneBuilder) Build() (*ZoneRequest, error) {
	if err := zb.request.Validate(); err != nil {
		return nil, err
	}

	return zb.request, nil
}

// ZoneError represents zone-specific errors
type ZoneError struct {
	Operation ZoneOperation
	DeviceID  string
	Reason    string
}

// Error implements the error interface
func (ze *ZoneError) Error() string {
	if ze.DeviceID != "" {
		return fmt.Sprintf("zone %s failed for device %s: %s",
			ze.Operation.String(), ze.DeviceID, ze.Reason)
	}

	return fmt.Sprintf("zone %s failed: %s", ze.Operation.String(), ze.Reason)
}

// NewZoneError creates a new zone error
func NewZoneError(op ZoneOperation, deviceID, reason string) *ZoneError {
	return &ZoneError{
		Operation: op,
		DeviceID:  deviceID,
		Reason:    reason,
	}
}

// Common zone error reasons
const (
	ZoneErrorDeviceNotFound    = "device not found"
	ZoneErrorDeviceOffline     = "device offline"
	ZoneErrorIncompatible      = "incompatible device type"
	ZoneErrorAlreadyInZone     = "device already in zone"
	ZoneErrorNotInZone         = "device not in zone"
	ZoneErrorMasterRequired    = "master device required"
	ZoneErrorNetworkError      = "network communication error"
	ZoneErrorUnsupported       = "operation not supported by device"
	ZoneErrorMaxMembersReached = "maximum zone members reached"
)

// ZoneCapabilities represents zone-related capabilities of a device
type ZoneCapabilities struct {
	CanBeMaster       bool `json:"canBeMaster"`
	CanBeMember       bool `json:"canBeMember"`
	MaxZoneMembers    int  `json:"maxZoneMembers"`
	SupportsMultiroom bool `json:"supportsMultiroom"`
}

// DefaultZoneCapabilities returns default zone capabilities
func DefaultZoneCapabilities() ZoneCapabilities {
	return ZoneCapabilities{
		CanBeMaster:       true,
		CanBeMember:       true,
		MaxZoneMembers:    6, // Common SoundTouch limit
		SupportsMultiroom: true,
	}
}

// CanCreateZone returns true if the device can create zones
func (zc *ZoneCapabilities) CanCreateZone() bool {
	return zc.SupportsMultiroom && zc.CanBeMaster
}

// CanJoinZone returns true if the device can join zones
func (zc *ZoneCapabilities) CanJoinZone() bool {
	return zc.SupportsMultiroom && zc.CanBeMember
}

// ZoneSlaveRequest represents the request for /addZoneSlave and /removeZoneSlave endpoints
type ZoneSlaveRequest struct {
	XMLName xml.Name         `xml:"zone"`
	Master  string           `xml:"master,attr"`
	Members []ZoneSlaveEntry `xml:"member"`
}

// ZoneSlaveEntry represents a single member entry in zone slave operations
type ZoneSlaveEntry struct {
	XMLName  xml.Name `xml:"member"`
	DeviceID string   `xml:",chardata"`
	IP       string   `xml:"ipaddress,attr,omitempty"`
}

// NewZoneSlaveRequest creates a new zone slave operation request
func NewZoneSlaveRequest(masterDeviceID string) *ZoneSlaveRequest {
	return &ZoneSlaveRequest{
		Master:  masterDeviceID,
		Members: []ZoneSlaveEntry{},
	}
}

// AddSlave adds a single slave to the request
func (zsr *ZoneSlaveRequest) AddSlave(deviceID, ipAddress string) {
	slave := ZoneSlaveEntry{
		DeviceID: deviceID,
		IP:       ipAddress,
	}
	zsr.Members = append(zsr.Members, slave)
}

// Validate validates the zone slave request
func (zsr *ZoneSlaveRequest) Validate() error {
	if zsr.Master == "" {
		return fmt.Errorf("master device ID is required")
	}

	if len(zsr.Members) != 1 {
		return fmt.Errorf("zone slave operations require exactly one member, got %d", len(zsr.Members))
	}

	member := zsr.Members[0]
	if member.DeviceID == "" {
		return fmt.Errorf("slave device ID cannot be empty")
	}

	if member.DeviceID == zsr.Master {
		return fmt.Errorf("slave device ID cannot be the same as master: %s", member.DeviceID)
	}

	if member.IP != "" {
		if net.ParseIP(member.IP) == nil {
			return fmt.Errorf("invalid IP address for device %s: %s", member.DeviceID, member.IP)
		}
	}

	return nil
}

// GetSlaveDeviceID returns the device ID of the slave being added/removed
func (zsr *ZoneSlaveRequest) GetSlaveDeviceID() string {
	if len(zsr.Members) > 0 {
		return zsr.Members[0].DeviceID
	}

	return ""
}

// GetSlaveIP returns the IP address of the slave being added/removed
func (zsr *ZoneSlaveRequest) GetSlaveIP() string {
	if len(zsr.Members) > 0 {
		return zsr.Members[0].IP
	}

	return ""
}

// String returns a human-readable string representation
func (zsr *ZoneSlaveRequest) String() string {
	if len(zsr.Members) == 0 {
		return fmt.Sprintf("Zone slave operation on master %s (no slave specified)", zsr.Master)
	}

	slave := zsr.Members[0]
	if slave.IP != "" {
		return fmt.Sprintf("Zone slave operation: master=%s, slave=%s (%s)",
			zsr.Master, slave.DeviceID, slave.IP)
	}

	return fmt.Sprintf("Zone slave operation: master=%s, slave=%s",
		zsr.Master, slave.DeviceID)
}
