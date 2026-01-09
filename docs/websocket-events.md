# WebSocket Events - Real-time SoundTouch Monitoring

This document describes the WebSocket event functionality for real-time monitoring of Bose SoundTouch devices.

## Overview

The WebSocket client provides real-time event notifications for various device state changes including:

- **Now Playing Updates**: Track changes, playback status, shuffle/repeat settings
- **Volume Changes**: Volume level and mute status changes
- **Connection Status**: Network connectivity and signal strength
- **Preset Updates**: Preset configuration changes (read-only - creation not supported by API)
- **Multiroom Zone Changes**: Zone membership and master device changes
- **Bass Level Changes**: Bass equalizer adjustments
- **Clock/Display Updates**: Clock time and display setting changes
- **Device Name Changes**: Device name updates
- **Error States**: Device error notifications

## Quick Start

### Basic Usage

```go
package main

import (
    "log"
    "time"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    // Create SoundTouch client
    soundTouchClient := client.NewClientFromHost("192.168.1.10")
    
    // Create WebSocket client with default configuration
    wsClient := soundTouchClient.NewWebSocketClient(nil)
    
    // Set up event handlers
    wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
        np := &event.NowPlaying
        log.Printf("Now Playing: %s by %s", np.Track, np.Artist)
        log.Printf("Status: %s", np.PlayStatus.String())
    })
    
    wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
        vol := &event.Volume
        if vol.IsMuted() {
            log.Println("Volume: Muted")
        } else {
            log.Printf("Volume: %d", vol.ActualVolume)
        }
    })
    
    // Connect to WebSocket
    if err := wsClient.Connect(); err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    
    // Keep running
    wsClient.Wait()
}
```

### Using the CLI Demo

```bash
# Auto-discover device and monitor all events
go run ./cmd/websocket-demo -discover

# Connect to specific device
go run ./cmd/websocket-demo -host 192.168.1.10

# Monitor only volume changes for 5 minutes
go run ./cmd/websocket-demo -host 192.168.1.10 -filter volume -duration 5m

# Monitor multiple event types with verbose logging
go run ./cmd/websocket-demo -host 192.168.1.10 -filter nowPlaying,volume -verbose
```

## Configuration

### WebSocket Configuration Options

```go
config := &client.WebSocketConfig{
    // Reconnection settings
    ReconnectInterval:    5 * time.Second,  // Time between reconnection attempts
    MaxReconnectAttempts: 0,                // 0 = unlimited attempts
    
    // Keep-alive settings
    PingInterval:         30 * time.Second, // Ping frequency
    PongTimeout:          10 * time.Second, // Pong response timeout
    
    // Buffer sizes
    ReadBufferSize:       2048,             // WebSocket read buffer
    WriteBufferSize:      2048,             // WebSocket write buffer
    
    // Logging
    Logger:               customLogger,      // Custom logger implementation
}

wsClient := soundTouchClient.NewWebSocketClient(config)
```

### Default Configuration

If you pass `nil` to `NewWebSocketClient()`, these defaults are used:

- **ReconnectInterval**: 5 seconds
- **MaxReconnectAttempts**: 0 (unlimited)
- **PingInterval**: 30 seconds
- **PongTimeout**: 10 seconds
- **ReadBufferSize**: 1024 bytes
- **WriteBufferSize**: 1024 bytes
- **Logger**: Default logger using standard `log` package

## Event Types and Handlers

### 1. Now Playing Events

Triggered when playback state, track, or playback settings change.

```go
wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
    np := &event.NowPlaying
    
    // Basic info
    fmt.Printf("Track: %s\n", np.Track)
    fmt.Printf("Artist: %s\n", np.Artist)
    fmt.Printf("Album: %s\n", np.Album)
    fmt.Printf("Source: %s\n", np.Source)
    
    // Playback status
    fmt.Printf("Status: %s\n", np.PlayStatus.String())
    fmt.Printf("Is Playing: %t\n", np.PlayStatus.IsPlaying())
    
    // Settings
    fmt.Printf("Shuffle: %s\n", np.ShuffleSetting.String())
    fmt.Printf("Repeat: %s\n", np.RepeatSetting.String())
    
    // Time info (if available)
    if np.HasTimeInfo() {
        fmt.Printf("Duration: %s\n", np.FormatDuration())
        fmt.Printf("Position: %s\n", np.FormatPosition())
    }
    
    // Capabilities
    fmt.Printf("Can Skip: %t\n", np.CanSkip())
    fmt.Printf("Can Seek: %t\n", np.IsSeekSupported())
    fmt.Printf("Can Favorite: %t\n", np.CanFavorite())
})
```

### 2. Volume Events

Triggered when volume level or mute status changes.

```go
wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
    vol := &event.Volume
    
    if vol.IsMuted() {
        fmt.Println("Device is muted")
    } else {
        fmt.Printf("Volume: %d\n", vol.ActualVolume)
        fmt.Printf("Level: %s\n", models.GetVolumeLevelName(vol.ActualVolume))
    }
    
    // Check if volume is still transitioning
    if !vol.IsVolumeSync() {
        fmt.Printf("Target volume: %d\n", vol.TargetVolume)
    }
})
```

### 3. Connection State Events

Triggered when network connectivity changes.

```go
wsClient.OnConnectionState(func(event *models.ConnectionStateUpdatedEvent) {
    cs := &event.ConnectionState
    
    if cs.IsConnected() {
        fmt.Println("Device connected to network")
        fmt.Printf("Signal strength: %s\n", cs.GetSignalStrength())
    } else {
        fmt.Printf("Connection state: %s\n", cs.State)
    }
})
```

### 4. Preset Events

Triggered when presets are updated or selected. Note: Preset creation via API is officially not supported by SoundTouch - presets can only be created through the official app or device controls.

```go
wsClient.OnPresetUpdated(func(event *models.PresetUpdatedEvent) {
    preset := &event.Preset
    
    fmt.Printf("Preset %s updated\n", preset.ID)
    if preset.ContentItem != nil {
        fmt.Printf("Name: %s\n", preset.ContentItem.ItemName)
        fmt.Printf("Source: %s\n", preset.ContentItem.Source)
    }
})
```

### 5. Multiroom Zone Events

Triggered when multiroom configuration changes.

```go
wsClient.OnZoneUpdated(func(event *models.ZoneUpdatedEvent) {
    zone := &event.Zone
    
    fmt.Printf("Zone master: %s\n", zone.Master)
    fmt.Printf("Zone members: %d\n", len(zone.Members))
    
    for i, member := range zone.Members {
        fmt.Printf("  %d. %s (%s)\n", i+1, member.DeviceID, member.IP)
    }
})
```

### 6. Bass Level Events

Triggered when bass equalizer settings change.

```go
wsClient.OnBassUpdated(func(event *models.BassUpdatedEvent) {
    bass := &event.Bass
    
    fmt.Printf("Bass level: %d\n", bass.ActualBass)
    
    if bass.ActualBass > 0 {
        fmt.Println("Bass boosted")
    } else if bass.ActualBass < 0 {
        fmt.Println("Bass reduced")
    } else {
        fmt.Println("Bass neutral")
    }
})
```

### 7. Unknown Events Handler

Handle any events not explicitly supported:

```go
wsClient.OnUnknownEvent(func(event *models.WebSocketEvent) {
    fmt.Printf("Unknown event from device %s\n", event.DeviceID)
    for _, eventType := range event.GetEventTypes() {
        fmt.Printf("  Event type: %s\n", eventType)
    }
})
```

## Connection Management

### Connecting and Disconnecting

```go
// Connect with default configuration
err := wsClient.Connect()

// Connect with custom configuration
config := &client.WebSocketConfig{
    ReconnectInterval: 3 * time.Second,
    PingInterval:      15 * time.Second,
}
err := wsClient.ConnectWithConfig(config)

// Check connection status
if wsClient.IsConnected() {
    fmt.Println("WebSocket connected")
}

// Disconnect
err := wsClient.Disconnect()
```

### Automatic Reconnection

The WebSocket client automatically attempts to reconnect when the connection is lost:

```go
config := &client.WebSocketConfig{
    ReconnectInterval:    5 * time.Second,  // Wait 5 seconds between attempts
    MaxReconnectAttempts: 10,               // Try up to 10 times (0 = unlimited)
}

wsClient := soundTouchClient.NewWebSocketClient(config)
```

### Graceful Shutdown

```go
// Set up signal handling for graceful shutdown
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

go func() {
    <-sigChan
    fmt.Println("Shutting down...")
    wsClient.Disconnect()
}()

// Wait for shutdown
wsClient.Wait()
```

## Error Handling and Logging

### Custom Logger

Implement the `Logger` interface for custom logging:

```go
type CustomLogger struct{}

func (c *CustomLogger) Printf(format string, v ...interface{}) {
    // Custom logging implementation
    log.Printf("[WS] "+format, v...)
}

config := &client.WebSocketConfig{
    Logger: &CustomLogger{},
}
```

### Silent Logging

To disable logging completely:

```go
type SilentLogger struct{}

func (s *SilentLogger) Printf(format string, v ...interface{}) {
    // Do nothing
}

config := &client.WebSocketConfig{
    Logger: &SilentLogger{},
}
```

## Advanced Usage

### Event Filtering

Process only specific event types:

```go
wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
    // Only handle now playing events
})

// Don't set other handlers - they'll be ignored
```

### Multiple Event Handlers

You can set multiple handlers, but only the last one set will be used:

```go
// First handler - will be replaced
wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
    fmt.Println("Handler 1")
})

// Second handler - this one will be used
wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
    fmt.Println("Handler 2")
})
```

### Composite Event Handling

Handle multiple event types in a unified way:

```go
handlers := &models.WebSocketEventHandlers{
    OnNowPlaying: func(event *models.NowPlayingUpdatedEvent) {
        logEvent("NowPlaying", event.DeviceID)
    },
    OnVolumeUpdated: func(event *models.VolumeUpdatedEvent) {
        logEvent("Volume", event.DeviceID)
    },
    OnConnectionState: func(event *models.ConnectionStateUpdatedEvent) {
        logEvent("Connection", event.DeviceID)
    },
}

wsClient.SetHandlers(handlers)
```

## WebSocket Protocol Details

### Connection Endpoint

The WebSocket connects to:
- **Protocol**: `ws://`
- **Port**: `8080` (different from HTTP API port 8090)
- **Path**: `/`

Example: `ws://192.168.1.10:8080/`

### Message Format

Events are received as XML messages in this format:

```xml
<?xml version="1.0" encoding="UTF-8" ?>
<updates deviceID="689E19B8BB8A">
    <nowPlayingUpdated deviceID="689E19B8BB8A">
        <nowPlaying deviceID="689E19B8BB8A" source="SPOTIFY">
            <track>Song Title</track>
            <artist>Artist Name</artist>
            <album>Album Name</album>
            <playStatus>PLAY_STATE</playStatus>
        </nowPlaying>
    </nowPlayingUpdated>
</updates>
```

### Keep-Alive

The client automatically sends WebSocket ping frames to keep the connection alive. The server responds with pong frames.

## Troubleshooting

### Common Issues

1. **Connection Refused**
   - Ensure the device is on the network and reachable
   - Check that WebSocket port 8080 is not blocked by firewall
   - Verify the device supports WebSocket connections

2. **Frequent Disconnections**
   - Check network stability
   - Increase ping interval if network is slow
   - Enable verbose logging to see connection details

3. **Events Not Received**
   - Verify event handlers are set before connecting
   - Check if the device actually generates the expected events
   - Enable unknown event handler to see all incoming events

### Debugging

Enable verbose logging:

```go
config := &client.WebSocketConfig{
    Logger: &VerboseLogger{},
}

type VerboseLogger struct{}

func (v *VerboseLogger) Printf(format string, args ...interface{}) {
    timestamp := time.Now().Format("15:04:05.000")
    fmt.Printf("[%s] [WebSocket] %s\n", timestamp, fmt.Sprintf(format, args...))
}
```

### Testing

Use the CLI demo to test WebSocket functionality:

```bash
# Test with verbose output
go run ./cmd/websocket-demo -host 192.168.1.10 -verbose

# Test reconnection by temporarily disconnecting device
go run ./cmd/websocket-demo -host 192.168.1.10 -verbose -duration 5m
```

## Performance Considerations

- **Buffer Sizes**: Increase buffer sizes for high-frequency events
- **Handler Efficiency**: Keep event handlers lightweight to avoid blocking
- **Memory Usage**: The client maintains minimal state and shouldn't leak memory
- **CPU Usage**: XML parsing adds some CPU overhead but should be minimal

## Security Notes

- WebSocket connections are unencrypted (ws://, not wss://)
- Authentication is not required for WebSocket connections
- Only devices on the same network can connect
- No sensitive data is transmitted over WebSocket

## API Limitations

### Preset Management
The SoundTouch API officially does not support preset creation or modification via WebSocket or HTTP endpoints. Preset events are read-only notifications when presets are updated through:
- Official SoundTouch mobile app
- Physical device controls
- Voice assistants (Alexa integration)

This is an intentional API design decision to maintain user control over personal preset configurations.

## Integration Examples

### Home Automation

```go
wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
    if event.NowPlaying.PlayStatus.IsPlaying() {
        // Dim lights when music starts playing
        homeAutomation.DimLights()
    }
})

wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
    if event.Volume.ActualVolume > 80 {
        // Send notification for loud volume
        notification.Send("Volume is very loud!")
    }
})
```

### Music Dashboard

```go
type MusicDashboard struct {
    currentTrack string
    volume      int
    isPlaying   bool
}

dashboard := &MusicDashboard{}

wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
    dashboard.currentTrack = event.NowPlaying.GetDisplayTitle()
    dashboard.isPlaying = event.NowPlaying.PlayStatus.IsPlaying()
    dashboard.updateUI()
})

wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
    dashboard.volume = event.Volume.ActualVolume
    dashboard.updateUI()
})
```

## API Reference

See the generated Go documentation for complete API details:

```bash
go doc github.com/gesellix/bose-soundtouch/pkg/client.WebSocketClient
go doc github.com/gesellix/bose-soundtouch/pkg/models.WebSocketEvent
```

## Testing

Run the WebSocket tests:

```bash
# Run unit tests
go test ./pkg/client -v -run TestWebSocket

# Run model tests
go test ./pkg/models -v -run TestWebSocket

# Run benchmarks
go test ./pkg/client -bench=BenchmarkWebSocket
```
