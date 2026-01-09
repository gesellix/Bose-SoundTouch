# Zone Management - Multiroom SoundTouch Control

This document describes the comprehensive zone management functionality for controlling multiroom setups with Bose SoundTouch devices.

## Overview

Zone management allows you to:

- **Create multiroom zones** with multiple SoundTouch devices
- **Query zone status** and membership information
- **Add and remove devices** from existing zones
- **Dissolve zones** to make devices standalone
- **Monitor zone changes** via WebSocket events

All SoundTouch devices that support multiroom functionality can participate in zones, with one device acting as the master and others as members.

## Quick Start

### Basic Zone Operations

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    // Connect to SoundTouch device
    soundTouchClient := client.NewClientFromHost("192.168.1.10")
    
    // Get current zone information
    zone, err := soundTouchClient.GetZone()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Zone Master: %s\n", zone.Master)
    fmt.Printf("Zone Members: %d\n", len(zone.Members))
    
    if zone.IsStandalone() {
        fmt.Println("Device is standalone (not in a zone)")
    } else {
        fmt.Printf("Total devices in zone: %d\n", zone.GetTotalDeviceCount())
    }
}
```

### CLI Zone Operations

```bash
# Get zone information
go run ./cmd/soundtouch-cli -host 192.168.1.10 -zone

# Check zone status for this device
go run ./cmd/soundtouch-cli -host 192.168.1.10 -zone-status

# List all zone members
go run ./cmd/soundtouch-cli -host 192.168.1.10 -zone-members

# Create a new zone
go run ./cmd/soundtouch-cli -host 192.168.1.10 -create-zone MASTER123,MEMBER456,MEMBER789

# Add device to existing zone
go run ./cmd/soundtouch-cli -host 192.168.1.10 -add-to-zone DEVICE456@192.168.1.11

# Remove device from zone
go run ./cmd/soundtouch-cli -host 192.168.1.10 -remove-from-zone DEVICE456

# Dissolve current zone
go run ./cmd/soundtouch-cli -host 192.168.1.10 -dissolve-zone
```

## Zone Concepts

### Zone Hierarchy

- **Master Device**: Controls zone playback, receives commands
- **Member Devices**: Follow master's playback, synchronized audio
- **Standalone**: Single device not part of any zone

### Zone States

| State | Description |
|-------|-------------|
| `STANDALONE` | Device operates independently |
| `MASTER` | Device controls a multiroom zone |
| `SLAVE` | Device follows zone master |

## API Reference

### Zone Information

#### Get Zone Configuration

```go
zone, err := client.GetZone()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Master: %s\n", zone.Master)
for i, member := range zone.Members {
    fmt.Printf("Member %d: %s (%s)\n", i+1, member.DeviceID, member.IP)
}
```

**Response Structure:**
```xml
<zone master="ABCD1234EFGH">
    <member ipaddress="192.168.1.11">EFGH5678IJKL</member>
    <member ipaddress="192.168.1.12">IJKL9012MNOP</member>
</zone>
```

#### Check Zone Status

```go
status, err := client.GetZoneStatus()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("This device is: %s\n", status.String())
```

#### Get Zone Members

```go
members, err := client.GetZoneMembers()
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Zone has %d devices:\n", len(members))
for _, deviceID := range members {
    fmt.Printf("  - %s\n", deviceID)
}
```

### Zone Creation and Management

#### Create New Zone

```go
// Simple zone creation
masterID := "ABCD1234EFGH"
memberIDs := []string{"EFGH5678IJKL", "IJKL9012MNOP"}

err := client.CreateZone(masterID, memberIDs)
if err != nil {
    log.Fatal(err)
}
```

#### Create Zone with IP Addresses

```go
masterID := "ABCD1234EFGH"
members := map[string]string{
    "EFGH5678IJKL": "192.168.1.11",
    "IJKL9012MNOP": "192.168.1.12",
}

err := client.CreateZoneWithIPs(masterID, members)
if err != nil {
    log.Fatal(err)
}
```

#### Add Device to Zone

```go
// Add device with IP address
err := client.AddToZone("DEVICE789", "192.168.1.13")
if err != nil {
    log.Fatal(err)
}
```

#### Remove Device from Zone

```go
err := client.RemoveFromZone("DEVICE789")
if err != nil {
    log.Fatal(err)
}
```

#### Dissolve Zone

```go
// Make all devices standalone
err := client.DissolveZone()
if err != nil {
    log.Fatal(err)
}
```

### Advanced Zone Operations

#### Using Zone Builder

```go
zoneRequest, err := models.NewZoneBuilder("MASTER123").
    WithMember("DEVICE456", "192.168.1.11").
    WithMemberByDeviceID("DEVICE789").
    Build()

if err != nil {
    log.Fatal(err)
}

err = client.SetZone(zoneRequest)
if err != nil {
    log.Fatal(err)
}
```

#### Custom Zone Configuration

```go
// Create zone request manually
zoneRequest := models.NewZoneRequest("MASTER123")
zoneRequest.AddMember("DEVICE456", "192.168.1.11")
zoneRequest.AddMember("DEVICE789", "192.168.1.12")

// Validate before sending
if err := zoneRequest.Validate(); err != nil {
    log.Fatal(err)
}

err := client.SetZone(zoneRequest)
if err != nil {
    log.Fatal(err)
}
```

### Zone Utility Methods

#### Zone Information Analysis

```go
zone, _ := client.GetZone()

// Check zone status
if zone.IsStandalone() {
    fmt.Println("No multiroom zone active")
}

// Check device membership
if zone.IsMaster("DEVICE123") {
    fmt.Println("DEVICE123 is the zone master")
}

if zone.IsMember("DEVICE456") {
    fmt.Println("DEVICE456 is a zone member")
}

// Find device by IP
member, found := zone.GetMemberByIP("192.168.1.11")
if found {
    fmt.Printf("Device at 192.168.1.11 is %s\n", member.DeviceID)
}

// Get all device IDs
allDevices := zone.GetAllDeviceIDs()
fmt.Printf("Zone contains: %v\n", allDevices)
```

## Real-time Zone Monitoring

### WebSocket Zone Events

```go
wsClient := soundTouchClient.NewWebSocketClient(nil)

// Monitor zone changes
wsClient.OnZoneUpdated(func(event *models.ZoneUpdatedEvent) {
    zone := &event.Zone
    
    if zone.Master != "" {
        fmt.Printf("Zone updated - Master: %s\n", zone.Master)
        fmt.Printf("Members: %d\n", len(zone.Members))
        
        for _, member := range zone.Members {
            fmt.Printf("  - %s (%s)\n", member.DeviceID, member.IP)
        }
    } else {
        fmt.Println("Zone dissolved - device is now standalone")
    }
})

// Connect and start monitoring
wsClient.Connect()
defer wsClient.Disconnect()
```

## Error Handling

### Zone-Specific Errors

```go
err := client.AddToZone("INVALID_DEVICE", "192.168.1.99")
if err != nil {
    // Handle specific zone errors
    if zoneErr, ok := err.(*models.ZoneError); ok {
        fmt.Printf("Zone operation %s failed: %s\n", 
                   zoneErr.Operation, zoneErr.Reason)
        
        switch zoneErr.Reason {
        case models.ZoneErrorDeviceNotFound:
            fmt.Println("Device not found on network")
        case models.ZoneErrorDeviceOffline:
            fmt.Println("Device is offline")
        case models.ZoneErrorAlreadyInZone:
            fmt.Println("Device already in a zone")
        case models.ZoneErrorMaxMembersReached:
            fmt.Println("Maximum zone size reached")
        }
    }
}
```

### Validation Errors

```go
zoneRequest := models.NewZoneRequest("") // Invalid: empty master
if err := zoneRequest.Validate(); err != nil {
    fmt.Printf("Zone validation failed: %v\n", err)
    // Error: "master device ID is required"
}
```

## Best Practices

### Zone Design Guidelines

1. **Master Selection**: Choose the most reliable device as master
2. **Network Quality**: Ensure all devices have stable network connections
3. **Device Compatibility**: Verify all devices support multiroom functionality
4. **Zone Size**: Keep zones reasonably sized (typically 2-6 devices)

### Error Recovery

```go
// Robust zone creation with retry
func createZoneWithRetry(client *client.Client, master string, members []string) error {
    maxRetries := 3
    
    for i := 0; i < maxRetries; i++ {
        err := client.CreateZone(master, members)
        if err == nil {
            return nil
        }
        
        // Handle specific errors
        if zoneErr, ok := err.(*models.ZoneError); ok {
            switch zoneErr.Reason {
            case models.ZoneErrorNetworkError:
                // Retry on network errors
                time.Sleep(time.Second * 2)
                continue
            case models.ZoneErrorDeviceNotFound:
                // Don't retry on device not found
                return err
            }
        }
        
        time.Sleep(time.Second * 2)
    }
    
    return fmt.Errorf("failed to create zone after %d retries", maxRetries)
}
```

### Performance Considerations

```go
// Check if device is in zone efficiently
func isDeviceInZone(client *client.Client) (bool, error) {
    // More efficient than getting full zone info
    return client.IsInZone()
}

// Get zone member count without full member list
func getZoneMemberCount(client *client.Client) (int, error) {
    zone, err := client.GetZone()
    if err != nil {
        return 0, err
    }
    return zone.GetTotalDeviceCount(), nil
}
```

## Integration Examples

### Home Automation

```go
// Automatically create zones based on room groupings
func createRoomZones() {
    livingRoomMaster := "LIVING_ROOM_MAIN"
    livingRoomMembers := []string{"LIVING_ROOM_LEFT", "LIVING_ROOM_RIGHT"}
    
    kitchenMaster := "KITCHEN_MAIN"
    kitchenMembers := []string{"KITCHEN_COUNTER"}
    
    // Create living room zone
    livingRoomClient := client.NewClientFromHost("192.168.1.10")
    livingRoomClient.CreateZone(livingRoomMaster, livingRoomMembers)
    
    // Create kitchen zone
    kitchenClient := client.NewClientFromHost("192.168.1.20")
    kitchenClient.CreateZone(kitchenMaster, kitchenMembers)
}
```

### Party Mode

```go
// Create house-wide party zone
func enablePartyMode() {
    allDevices := []string{
        "LIVING_ROOM", "KITCHEN", "BEDROOM", 
        "BATHROOM", "OFFICE", "BASEMENT",
    }
    
    if len(allDevices) > 0 {
        master := allDevices[0]
        members := allDevices[1:]
        
        masterClient := client.NewClientFromHost("192.168.1.10")
        err := masterClient.CreateZone(master, members)
        if err != nil {
            log.Printf("Failed to create party zone: %v", err)
        }
    }
}

func disablePartyMode() {
    // Dissolve all zones
    for _, ip := range []string{"192.168.1.10", "192.168.1.11", "192.168.1.12"} {
        client := client.NewClientFromHost(ip)
        client.DissolveZone()
    }
}
```

### Music Following

```go
// Move zone to follow user between rooms
func moveZoneToRoom(currentClient, targetClient *client.Client, targetDeviceID string) error {
    // Get current zone
    zone, err := currentClient.GetZone()
    if err != nil {
        return err
    }
    
    // Add target device to zone
    err = currentClient.AddToZone(targetDeviceID, "")
    if err != nil {
        return err
    }
    
    // Wait for synchronization
    time.Sleep(time.Second * 2)
    
    // Make target device the new master
    newZoneRequest := models.NewZoneRequest(targetDeviceID)
    for _, member := range zone.Members {
        if member.DeviceID != targetDeviceID {
            newZoneRequest.AddMember(member.DeviceID, member.IP)
        }
    }
    
    return targetClient.SetZone(newZoneRequest)
}
```

## Troubleshooting

### Common Issues

1. **Zone Creation Fails**
   - Verify all devices are online and reachable
   - Check network connectivity between devices
   - Ensure devices support multiroom functionality

2. **Devices Not Synchronizing**
   - Check network quality and bandwidth
   - Verify all devices are on the same network segment
   - Try recreating the zone

3. **Zone Commands Timeout**
   - Increase client timeout
   - Check device responsiveness
   - Verify API endpoint availability

### Debug Zone Status

```go
func debugZoneStatus(client *client.Client) {
    // Get comprehensive zone information
    zone, err := client.GetZone()
    if err != nil {
        fmt.Printf("Error getting zone: %v\n", err)
        return
    }
    
    fmt.Printf("Zone Debug Information:\n")
    fmt.Printf("  Master: %s\n", zone.Master)
    fmt.Printf("  Members: %d\n", len(zone.Members))
    fmt.Printf("  Total Devices: %d\n", zone.GetTotalDeviceCount())
    fmt.Printf("  Is Standalone: %t\n", zone.IsStandalone())
    
    for i, member := range zone.Members {
        fmt.Printf("  Member %d: %s (IP: %s)\n", 
                   i+1, member.DeviceID, member.IP)
    }
    
    // Check device status
    status, err := client.GetZoneStatus()
    if err == nil {
        fmt.Printf("  This Device Status: %s\n", status.String())
    }
}
```

### Validation and Testing

```go
// Test zone functionality
func testZoneOperations(client *client.Client) {
    fmt.Println("Testing zone operations...")
    
    // Test 1: Get initial status
    initialZone, err := client.GetZone()
    if err != nil {
        fmt.Printf("❌ Failed to get initial zone: %v\n", err)
        return
    }
    fmt.Printf("✅ Initial zone status: %s\n", initialZone.String())
    
    // Test 2: Check capabilities
    inZone, err := client.IsInZone()
    if err != nil {
        fmt.Printf("❌ Failed to check zone membership: %v\n", err)
        return
    }
    fmt.Printf("✅ Zone membership check: %t\n", inZone)
    
    // Test 3: Get zone status
    status, err := client.GetZoneStatus()
    if err != nil {
        fmt.Printf("❌ Failed to get zone status: %v\n", err)
        return
    }
    fmt.Printf("✅ Zone status: %s\n", status.String())
    
    fmt.Println("All zone tests passed!")
}
```

## API Limitations

### SoundTouch API Constraints

1. **Zone Size**: Typically limited to 6 devices per zone
2. **Master Role**: Only certain device types can be zone masters
3. **Network Requirements**: All devices must be on same network
4. **Synchronization**: Audio sync depends on network quality

### Implementation Notes

- Zone operations may take several seconds to complete
- WebSocket events provide real-time zone change notifications
- IP addresses in zone configurations are optional but recommended
- Device IDs must be valid and reachable for zone operations

## Security Considerations

- Zone management requires network access to all devices
- No authentication required for zone operations
- Consider network segmentation for security
- Monitor zone changes via WebSocket events for unauthorized modifications

## Performance Guidelines

- **Batch Operations**: Group multiple zone changes when possible
- **Error Handling**: Always implement retry logic for network operations
- **Monitoring**: Use WebSocket events for real-time zone state tracking
- **Validation**: Validate zone configurations before applying

The zone management implementation provides comprehensive multiroom control with robust error handling, validation, and real-time monitoring capabilities for production-ready applications.