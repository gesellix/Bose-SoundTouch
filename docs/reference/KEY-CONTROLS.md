# Key Control Implementation

This document describes the implementation of the POST `/key` endpoint for media control commands in the Bose SoundTouch API client.

## Overview

The key control functionality allows sending media control commands to SoundTouch devices, including play/pause, volume adjustment, track navigation, and preset selection.

## Implementation Files

- `pkg/models/key.go` - XML model and constants for key commands
- `pkg/models/key_test.go` - Comprehensive tests for key functionality
- `pkg/client/client.go` - Client methods for sending key commands
- `cmd/soundtouch-cli/main.go` - CLI commands for key controls

## API Specification

### POST /key

Sends a key command to the SoundTouch device.

**Request Format:**
According to the API documentation, proper key simulation requires sending both press and release states:
```xml
<key state="press" sender="Gabbo">KEY_NAME</key>
<key state="release" sender="Gabbo">KEY_NAME</key>
```

**Response Format:**
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<status>/key</status>
```

### Important Discovery: Sender Field

During implementation, we discovered that the `sender` attribute is critical for successful key commands. Only specific sender values are accepted:

- ✅ **"Gabbo"** - Works (canonical example from official documentation)
- ❌ "GoClient" - Rejected with CLIENT_XML_ERROR (1019)
- ❌ "SoundTouch app" - Rejected with CLIENT_XML_ERROR (1019)
- ❌ "" (empty) - Rejected with CLIENT_XML_ERROR (1019)

Our implementation uses **"Gabbo"** as the default sender, which is the standard value used in official SoundTouch examples.

## Available Key Commands

### Playback Controls
- `PLAY` - Start playback
- `PAUSE` - Pause current playback
- `STOP` - Stop current playback
- `PREV_TRACK` - Go to previous track
- `NEXT_TRACK` - Go to next track

### Rating and Bookmark Controls
- `THUMBS_UP` - Rate current content positively (Pandora, etc.)
- `THUMBS_DOWN` - Rate current content negatively
- `BOOKMARK` - Bookmark current content

### Power and System Controls
- `POWER` - Toggle device power state
- `MUTE` - Toggle mute state

### Volume Controls
- `VOLUME_UP` - Increase volume
- `VOLUME_DOWN` - Decrease volume

### Preset Controls
- `PRESET_1` through `PRESET_6` - Select preset 1-6

### Input Controls
- `AUX_INPUT` - Switch to auxiliary input

### Shuffle Controls
- `SHUFFLE_OFF` - Turn shuffle mode off
- `SHUFFLE_ON` - Turn shuffle mode on

### Repeat Controls
- `REPEAT_OFF` - Turn repeat mode off
- `REPEAT_ONE` - Repeat current track
- `REPEAT_ALL` - Repeat all tracks in playlist

## Client API

### Basic Methods

```go
// Send complete key command (press + release - recommended)
err := client.SendKey(models.KeyPlay)

// Send key press and release (alias for SendKey)
err := client.SendKeyPress(models.KeyPlay)

// Send only key press state (advanced usage)
err := client.SendKeyPressOnly(models.KeyPlay)

// Send only key release state (advanced usage)
err := client.SendKeyReleaseOnly(models.KeyPlay)
```

### Convenience Methods

```go
// Media controls
err := client.Play()
err := client.Pause()
err := client.Stop()
err := client.NextTrack()
err := client.PrevTrack()

// Volume controls
err := client.VolumeUp()
err := client.VolumeDown()

// Preset selection (1-6)
err := client.SelectPreset(1)
```

### Key Validation

```go
// Check if a key value is valid
isValid := models.IsValidKey("PLAY") // true
isValid := models.IsValidKey("INVALID") // false

// Get all valid key values
allKeys := models.GetAllValidKeys()
```

## CLI Usage

### Individual Key Commands

```bash
# Media controls
soundtouch-cli -host 192.168.1.100 -play
soundtouch-cli -host 192.168.1.100 -pause
soundtouch-cli -host 192.168.1.100 -stop
soundtouch-cli -host 192.168.1.100 -next
soundtouch-cli -host 192.168.1.100 -prev

# Volume controls
soundtouch-cli -host 192.168.1.100 -volume-up
soundtouch-cli -host 192.168.1.100 -volume-down

# Preset selection
soundtouch-cli -host 192.168.1.100 -preset 1
soundtouch-cli -host 192.168.1.100 -preset 6
```

### Generic Key Command

```bash
# Send any valid key using the -key flag
soundtouch-cli -host 192.168.1.100 -key PLAY
soundtouch-cli -host 192.168.1.100 -key STOP
soundtouch-cli -host 192.168.1.100 -key PRESET_3
```

### Error Handling

```bash
# Invalid key validation
$ soundtouch-cli -host 192.168.1.100 -key INVALID
Failed to send key command: invalid key value: INVALID

# Multiple commands rejected
$ soundtouch-cli -host 192.168.1.100 -play -pause
Failed to send key command: only one key command can be sent at a time

# Missing host
$ soundtouch-cli -play
Host is required for key commands. Use -host flag or -discover to find devices.
```

## Testing

### Unit Tests

The implementation includes comprehensive unit tests in `pkg/models/key_test.go`:

- XML marshaling/unmarshaling
- Key validation
- Constructor functions
- Constants validation
- Benchmark tests

Run tests:
```bash
go test ./pkg/models/...
```

### Integration Testing

Tested with real SoundTouch devices:
- **SoundTouch 10** (192.168.1.10:8090) ✅
- **SoundTouch 20** (192.168.1.11:8090) ✅

All key commands successfully sent and executed on both devices.

## Code Examples

### Basic Usage

```go
package main

import (
    "log"
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    // Create client
    soundtouchClient := client.NewClientFromHost("192.168.1.100")
    
    // Play music
    if err := soundtouchClient.Play(); err != nil {
        log.Fatalf("Failed to play: %v", err)
    }
    
    // Adjust volume
    if err := soundtouchClient.VolumeUp(); err != nil {
        log.Fatalf("Failed to increase volume: %v", err)
    }
    
    // Select preset
    if err := soundtouchClient.SelectPreset(1); err != nil {
        log.Fatalf("Failed to select preset: %v", err)
    }
}
```

### Advanced Usage with Validation

```go
func sendKeyCommand(client *client.Client, keyValue string) error {
    // Validate before sending
    if !models.IsValidKey(keyValue) {
        return fmt.Errorf("invalid key: %s", keyValue)
    }
    
    // SendKey automatically sends both press and release states
    return client.SendKey(keyValue)
}

func sendAllValidKeys(client *client.Client) {
    for _, key := range models.GetAllValidKeys() {
        fmt.Printf("Sending key: %s (press+release)\n", key)
        if err := client.SendKey(key); err != nil {
            log.Printf("Failed to send %s: %v", key, err)
        }
        time.Sleep(1 * time.Second) // Avoid overwhelming the device
    }
}
```

## Implementation Notes

1. **Press + Release Pattern**: Following API documentation, `SendKey()` sends both press and release states for proper key simulation
2. **Sender Field Critical**: The `sender` attribute must be "Gabbo" for commands to be accepted
3. **XML Format**: Simple XML structure without namespaces or headers
4. **State Handling**: Both "press" and "release" states are supported, with complete press+release cycle as default
5. **Input Validation**: All key values are validated before sending to the device
6. **Error Handling**: Comprehensive error handling for invalid keys and API errors
7. **CLI Safety**: Only one key command allowed per CLI invocation to prevent conflicts

## Future Enhancements

Potential areas for future development:

1. **Key Sequences**: Support for sending multiple key commands in sequence
2. **Macros**: Predefined key command sequences (e.g., "power on and play preset 1")
3. **Key Hold**: Support for key hold duration for volume changes
4. **Device State**: Check device state before sending commands
5. **Async Commands**: Non-blocking key command execution
6. **Key Mapping**: Custom key mappings for different device types

## Reference

- **Official API**: Based on Bose SoundTouch Web API documentation
- **Test Devices**: Validated with SoundTouch 10 and SoundTouch 20
- **Standards**: Follows existing project patterns and conventions