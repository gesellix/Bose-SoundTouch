# Volume Control Implementation

This document describes the implementation of the GET/POST `/volume` endpoints for volume management in the Bose SoundTouch API client.

## Overview

The volume control functionality allows getting current volume levels and setting new volume levels on SoundTouch devices. It provides both direct volume setting and incremental adjustments with safety features.

## Implementation Files

- `pkg/models/volume.go` - XML model and validation for volume endpoints
- `pkg/models/volume_test.go` - Comprehensive tests for volume functionality
- `pkg/client/client.go` - Client methods for volume control
- `cmd/soundtouch-cli/main.go` - CLI commands for volume management

## API Specification

### GET /volume

Retrieves the current volume level and mute status from the device.

**Response Format:**
```xml
<volume deviceID="ABCD1234EFGH">
  <targetvolume>50</targetvolume>
  <actualvolume>50</actualvolume>
  <muteenabled>false</muteenabled>
</volume>
```

### POST /volume

Sets the volume level on the device.

**Request Format:**
```xml
<volume>50</volume>
```

**Response Format:**
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<status>/volume</status>
```

## Volume Levels

### Valid Range
- **Minimum**: 0 (mute/silent)
- **Maximum**: 100 (loudest)
- **Validation**: All values must be within 0-100 range

### Volume Categories
- **Mute**: 0
- **Very Quiet**: 1-10
- **Quiet**: 11-25
- **Medium**: 26-50
- **High**: 51-75
- **Loud**: 76-100

## Client API

### Basic Volume Operations

```go
// Get current volume information
volume, err := client.GetVolume()
if err != nil {
    log.Printf("Failed to get volume: %v", err)
}

fmt.Printf("Current volume: %d (%s)\n", volume.GetLevel(), volume.GetVolumeString())
fmt.Printf("Target volume: %d\n", volume.GetTargetLevel())
fmt.Printf("Muted: %v\n", volume.IsMuted())
```

### Set Volume

```go
// Set specific volume level (0-100)
err := client.SetVolume(50)

// Set volume with automatic clamping for invalid values
err := client.SetVolumeSafe(150) // Will be clamped to 100
```

### Incremental Volume Control

```go
// Increase volume by specified amount
newVolume, err := client.IncreaseVolume(5)
if err != nil {
    log.Printf("Failed to increase volume: %v", err)
} else {
    fmt.Printf("Volume increased to: %d\n", newVolume.GetLevel())
}

// Decrease volume by specified amount
newVolume, err := client.DecreaseVolume(3)
if err != nil {
    log.Printf("Failed to decrease volume: %v", err)
} else {
    fmt.Printf("Volume decreased to: %d\n", newVolume.GetLevel())
}
```

### Volume Information Methods

```go
volume, _ := client.GetVolume()

// Get current actual volume level
level := volume.GetLevel()

// Get target volume level
targetLevel := volume.GetTargetLevel()

// Check if device is muted
isMuted := volume.IsMuted()

// Check if volume is synchronized (target == actual)
isSync := volume.IsVolumeSync()

// Get formatted volume string
volumeStr := volume.GetVolumeString() // "Muted" or "50"
```

### Volume Validation

```go
// Validate volume level
if models.ValidateVolumeLevel(75) {
    fmt.Println("Volume level is valid")
}

// Clamp volume to valid range
safeLevel := models.ClampVolumeLevel(150) // Returns 100

// Get descriptive name for volume level
name := models.GetVolumeLevelName(25) // Returns "Quiet"
```

## CLI Usage

### Get Current Volume

```bash
# Get current volume information
soundtouch-cli -host 192.168.1.100:8090 -volume
```

**Output:**
```
Current Volume:
  Device ID: ABCD1234EFGH
  Current Level: 50 (Medium)
  Target Level: 50
  Muted: false
```

### Set Specific Volume Level

```bash
# Set volume to specific level (0-100)
soundtouch-cli -host 192.168.1.100:8090 -set-volume 25
soundtouch-cli -host 192.168.1.100:8090 -set-volume 0   # Mute
```

**Safety Features:**
- Volumes above 30 show a warning and 2-second delay
- Invalid volumes are rejected with error message

### Incremental Volume Control

```bash
# Increase volume by amount (1-10, default: 2)
soundtouch-cli -host 192.168.1.100:8090 -inc-volume 3
soundtouch-cli -host 192.168.1.100:8090 -inc-volume     # Uses default: 2

# Decrease volume by amount (1-20, default: 2)
soundtouch-cli -host 192.168.1.100:8090 -dec-volume 5
soundtouch-cli -host 192.168.1.100:8090 -dec-volume     # Uses default: 2
```

### CLI Safety Features

1. **Volume Warnings**: High volume settings (>30) show warnings
2. **Increment Limits**: Volume increases limited to 10 per command
3. **Decrement Limits**: Volume decreases limited to 20 per command
4. **Automatic Clamping**: All values automatically clamped to 0-100 range
5. **Error Handling**: Clear error messages for invalid operations

## Testing

### Unit Tests

The implementation includes comprehensive unit tests in `pkg/models/volume_test.go`:

- XML marshaling/unmarshaling for requests and responses
- Volume validation and clamping functions
- Volume level categorization
- Helper method functionality
- Constants validation
- Benchmark tests for performance

Run tests:
```bash
go test ./pkg/models/volume*
```

### Integration Testing

Tested with real SoundTouch devices:
- **SoundTouch 10** (192.168.1.10:8090) ✅
- **SoundTouch 20** (192.168.1.11:8090) ✅

All volume operations successfully tested on both devices.

## Code Examples

### Basic Volume Control

```go
package main

import (
    "fmt"
    "log"
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    // Create client
    soundtouchClient := client.NewClientFromHost("192.168.1.100")
    
    // Get current volume
    volume, err := soundtouchClient.GetVolume()
    if err != nil {
        log.Fatalf("Failed to get volume: %v", err)
    }
    
    fmt.Printf("Current volume: %d (%s)\n", 
        volume.GetLevel(), 
        models.GetVolumeLevelName(volume.GetLevel()))
    
    // Set to comfortable listening level
    if err := soundtouchClient.SetVolume(35); err != nil {
        log.Fatalf("Failed to set volume: %v", err)
    }
    
    fmt.Println("Volume set to comfortable level")
}
```

### Volume Monitoring

```go
func monitorVolume(client *client.Client) {
    for {
        volume, err := client.GetVolume()
        if err != nil {
            log.Printf("Error getting volume: %v", err)
            continue
        }
        
        fmt.Printf("Volume: %d", volume.GetLevel())
        
        if volume.IsMuted() {
            fmt.Print(" (MUTED)")
        }
        
        if !volume.IsVolumeSync() {
            fmt.Printf(" -> %d (adjusting)", volume.GetTargetLevel())
        }
        
        fmt.Println()
        time.Sleep(2 * time.Second)
    }
}
```

### Safe Volume Management

```go
func safeVolumeControl(client *client.Client, newLevel int) error {
    // Get current volume first
    currentVolume, err := client.GetVolume()
    if err != nil {
        return fmt.Errorf("failed to get current volume: %w", err)
    }
    
    // Don't allow large jumps in volume
    currentLevel := currentVolume.GetLevel()
    if abs(newLevel - currentLevel) > 20 {
        return fmt.Errorf("volume change too large: %d -> %d", currentLevel, newLevel)
    }
    
    // Validate and clamp
    if !models.ValidateVolumeLevel(newLevel) {
        newLevel = models.ClampVolumeLevel(newLevel)
        fmt.Printf("Volume clamped to safe range: %d\n", newLevel)
    }
    
    // Set volume
    return client.SetVolume(newLevel)
}

func abs(x int) int {
    if x < 0 {
        return -x
    }
    return x
}
```

## Implementation Notes

1. **Target vs Actual Volume**: The API returns both target and actual volume levels. During volume changes, these may differ temporarily.

2. **Volume Synchronization**: Use `IsVolumeSync()` to check if the device has finished adjusting to the target volume.

3. **Mute Handling**: Mute is a separate boolean field, not just volume level 0.

4. **Safety Features**: The CLI implementation includes several safety features to prevent accidental loud volume settings.

5. **Incremental Control**: The `IncreaseVolume()` and `DecreaseVolume()` methods return the updated volume level for immediate feedback.

6. **Error Handling**: All volume operations include comprehensive error handling with descriptive messages.

## Relationship to Key Controls

Volume can be controlled in two ways:

### Via Volume API (Precise)
```go
client.SetVolume(50)              // Set exact level
client.IncreaseVolume(5)         // Increase by exact amount
```

### Via Key Commands (Step-based)
```go
client.VolumeUp()                // Single step up
client.VolumeDown()              // Single step down
client.SendKey(models.KeyVolumeUp)   // Same as VolumeUp()
```

**Note**: Key commands now properly send both press and release states as per API documentation.

## Known Issues and Workarounds

1. **Stuck Volume Keys**: If volume appears to be continuously adjusting, there may be a stuck key press. Send the same key command to ensure proper press+release cycle.

2. **External Volume Control**: Some sources (like Spotify apps) may override volume settings. Check the source and consider switching sources if needed.

3. **Volume Jumps**: If volume jumps unexpectedly during increment/decrement operations, check for external volume control interference.

## Future Enhancements

Potential areas for future development:

1. **Volume Profiles**: Predefined volume profiles (quiet, normal, party)
2. **Time-based Volume**: Automatic volume adjustment based on time of day
3. **Source-specific Volume**: Remember volume levels per audio source
4. **Volume Limits**: Configurable maximum volume limits for safety
5. **Volume Fade**: Gradual volume transitions for smooth experience
6. **Volume Monitoring**: Real-time volume change notifications via WebSocket

## Reference

- **Official API**: Based on Bose SoundTouch Web API documentation
- **Test Devices**: Validated with SoundTouch 10 and SoundTouch 20
- **Standards**: Follows existing project patterns and conventions
- **Safety**: Implements multiple safety features for user protection