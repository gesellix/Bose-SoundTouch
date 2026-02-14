# Getting Started with SoundTouch Go Client

**A complete guide to controlling your Bose SoundTouch devices with Go**

This guide will get you up and running with the SoundTouch Go client in under 10 minutes. By the end, you'll be able to discover devices, control playback, manage volume, and monitor real-time events.

## 📋 **Prerequisites**

- **Go 1.25.6 or later** installed on your system
- **Bose SoundTouch device** on your network (SoundTouch 10, 20, 30, etc.)
- **Same network** - Your computer and SoundTouch device must be on the same network

## 🚀 **Quick Start**

### Step 1: Create a New Go Project

```bash
mkdir soundtouch-example
cd soundtouch-example
go mod init soundtouch-example
```

### Step 2: Add the SoundTouch Client

```bash
go get github.com/gesellix/bose-soundtouch
```

### Step 3: Find Your Device

Create `main.go`:

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
    // Discover devices on your network
    discoverer := discovery.NewDiscoverer(discovery.Config{
        Timeout: 10 * time.Second,
    })

    fmt.Println("🔍 Discovering SoundTouch devices...")
    devices, err := discoverer.DiscoverDevices()
    if err != nil {
        log.Fatalf("Discovery failed: %v", err)
    }

    if len(devices) == 0 {
        fmt.Println("❌ No devices found. Make sure your SoundTouch is on and connected.")
        return
    }

    fmt.Printf("✅ Found %d device(s):\n", len(devices))
    for i, device := range devices {
        fmt.Printf("%d. %s at %s:%d\n", i+1, device.Name, device.Host, device.Port)
    }
}
```

Run it:
```bash
go run main.go
```

You should see your SoundTouch device(s) listed!

### Step 4: Control Your Device

Now let's add basic control functionality:

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
    // Discover and connect to first device
    discoverer := discovery.NewDiscoverer(discovery.Config{
        Timeout: 5 * time.Second,
    })

    devices, err := discoverer.DiscoverDevices()
    if err != nil || len(devices) == 0 {
        log.Fatal("No devices found")
    }

    // Connect to the first device
    soundtouch := client.NewClient(client.ClientConfig{
        Host:    devices[0].Host,
        Port:    devices[0].Port,
        Timeout: 10 * time.Second,
    })

    // Get device information
    info, err := soundtouch.GetDeviceInfo()
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }

    fmt.Printf("🎵 Connected to: %s\n", info.Name)
    fmt.Printf("   Type: %s\n", info.Type)
    fmt.Printf("   ID: %s\n", info.DeviceID)

    // Basic controls
    fmt.Println("\n🎮 Testing basic controls...")

    // Set volume to 30
    fmt.Println("Setting volume to 30...")
    if err := soundtouch.SetVolume(30); err != nil {
        fmt.Printf("Volume control failed: %v\n", err)
    }

    // Play
    fmt.Println("Sending PLAY command...")
    if err := soundtouch.Play(); err != nil {
        fmt.Printf("Play failed: %v\n", err)
    }

    // Wait a moment
    time.Sleep(2 * time.Second)

    // Pause
    fmt.Println("Sending PAUSE command...")
    if err := soundtouch.Pause(); err != nil {
        fmt.Printf("Pause failed: %v\n", err)
    }

    // Get current status
    nowPlaying, err := soundtouch.GetNowPlaying()
    if err == nil {
        fmt.Printf("\n📊 Current Status:\n")
        fmt.Printf("   Source: %s\n", nowPlaying.Source)
        if nowPlaying.Track != "" {
            fmt.Printf("   Track: %s\n", nowPlaying.Track)
        }
        if nowPlaying.Artist != "" {
            fmt.Printf("   Artist: %s\n", nowPlaying.Artist)
        }
        fmt.Printf("   Status: %s\n", nowPlaying.PlayStatus)
    }

    fmt.Println("\n✅ Basic setup complete!")
}
```

## 📖 **Core Concepts**

### Device Discovery

The SoundTouch library supports multiple discovery methods:

```go
// UPnP discovery (default)
discoverer := discovery.NewDiscoverer(discovery.Config{
    Timeout: 10 * time.Second,
})
devices, err := discoverer.DiscoverDevices()

// Or connect directly if you know the IP
soundtouch := client.NewClientFromHost("192.168.1.100")
```

### Client Configuration

```go
config := client.ClientConfig{
    Host:      "192.168.1.100",
    Port:      8090,  // Default SoundTouch port
    Timeout:   10 * time.Second,
    UserAgent: "MyApp/1.0",  // Optional
}
soundtouch := client.NewClient(config)
```

### Error Handling

Always check for errors, especially with network operations:

```go
volume, err := soundtouch.GetVolume()
if err != nil {
    log.Printf("Failed to get volume: %v", err)
    return
}

fmt.Printf("Current volume: %d\n", volume.TargetVolume)
if volume.Muted {
    fmt.Println("Device is muted")
}
```

## 🎵 **Common Operations**

### Playback Control

```go
// Basic playback
soundtouch.Play()
soundtouch.Pause()
soundtouch.Stop()

// Navigation
soundtouch.NextTrack()
soundtouch.PrevTrack()

// Power and mute
soundtouch.SendKey("POWER")
soundtouch.SendKey("MUTE")
```

### Volume Management

```go
// Get current volume
volume, err := soundtouch.GetVolume()
if err == nil {
    fmt.Printf("Volume: %d, Muted: %t\n", volume.TargetVolume, volume.Muted)
}

// Set volume (0-100)
soundtouch.SetVolume(50)

// Incremental control
soundtouch.VolumeUp()
soundtouch.VolumeDown()

// Safe volume setting (clamps to valid range)
soundtouch.SetVolumeSafe(150)  // Will be set to 100
```

### Source Selection

```go
// Get available sources
sources, err := soundtouch.GetSources()
if err == nil {
    for _, source := range sources.Sources {
        fmt.Printf("Source: %s (%s)\n", source.Source, source.Status)
    }
}

// Select sources
soundtouch.SelectSpotify()
soundtouch.SelectBluetooth()
soundtouch.SelectAux()

// Or select by name
soundtouch.SelectSource("SPOTIFY", "")
```

### Preset Management

```go
// Get presets
presets, err := soundtouch.GetPresets()
if err == nil {
    for _, preset := range presets.Presets {
        fmt.Printf("Preset %d: %s\n", preset.ID, preset.ContentItem.ItemName)
    }
}

// Select preset (1-6)
soundtouch.SelectPreset(1)

// Or use key command
soundtouch.SendKey("PRESET_3")
```

### Device Information

```go
// Basic device info
info, _ := soundtouch.GetDeviceInfo()
fmt.Printf("Device: %s (%s)\n", info.Name, info.Type)

// Capabilities
caps, _ := soundtouch.GetCapabilities()
fmt.Printf("Bass control: %t\n", caps.BassCapable)

// Network information
network, _ := soundtouch.GetNetworkInfo()
for _, iface := range network.GetInterfaces() {
    fmt.Printf("Interface: %s - %s\n", iface.Type, iface.IPAddress)
}
```

## 🌐 **Real-time Monitoring**

One of the most powerful features is real-time event monitoring:

```go
package main

import (
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/gesellix/bose-soundtouch/pkg/client"
)

func main() {
    // Connect to device
    soundtouch := client.NewClientFromHost("192.168.1.100")

    // Create WebSocket client
    wsClient := soundtouch.NewWebSocketClient(nil)

    // Set up event handlers
    wsClient.OnNowPlaying(func(event *models.NowPlayingUpdatedEvent) {
        fmt.Printf("🎵 Now Playing: %s - %s\n", 
            event.NowPlaying.Artist, event.NowPlaying.Track)
    })

    wsClient.OnVolumeUpdated(func(event *models.VolumeUpdatedEvent) {
        fmt.Printf("🔊 Volume: %d\n", event.Volume.TargetVolume)
    })

    // Connect to WebSocket
    if err := wsClient.Connect(); err != nil {
        log.Fatalf("WebSocket connection failed: %v", err)
    }

    fmt.Println("✅ Monitoring events... Press Ctrl+C to exit")

    // Wait for interrupt
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan

    // Cleanup
    wsClient.Disconnect()
    fmt.Println("Disconnected.")
}
```

## 👥 **Multiroom Setup**

Control multiple speakers together:

```go
// Get current zone configuration
zone, err := soundtouch.GetZone()
if err == nil {
    fmt.Printf("Zone master: %s\n", zone.Master)
    fmt.Printf("Members: %d\n", len(zone.Members))
}

// Create a zone (master + members)
masterID := "DEVICE123"
memberIDs := []string{"DEVICE456", "DEVICE789"}
soundtouch.CreateZone(masterID, memberIDs)

// Add device to existing zone
soundtouch.AddToZone("DEVICE999", "192.168.1.15")

// Remove device from zone
soundtouch.RemoveFromZone("DEVICE456")

// Dissolve zone (make all devices standalone)
soundtouch.DissolveZone()
```

## ⚠️ **Common Issues & Solutions**

### Device Not Found
```
❌ No devices found
```
**Solutions:**
- Ensure SoundTouch is powered on
- Check both devices are on same network
- Try specifying IP directly: `client.NewClientFromHost("192.168.1.100")`
- Check firewall settings

### Connection Timeouts
```
❌ Failed to connect: context deadline exceeded
```
**Solutions:**
- Increase timeout: `Timeout: 30 * time.Second`
- Verify IP address and port (default 8090)
- Check network connectivity with `ping 192.168.1.100`

### Volume/Control Issues
```
❌ Volume control failed
```
**Solutions:**
- Check device isn't in a zone (members can't control volume directly)
- Ensure device isn't in setup mode
- Try basic commands first (play, pause)

### WebSocket Connection Issues
```
❌ WebSocket connection failed
```
**Solutions:**
- WebSocket uses port 8080 (not 8090)
- Ensure no other apps are connected
- Try disconnecting and reconnecting

## 🔧 **Development Tips**

### Enable Debug Logging

```go
// For HTTP requests
import "net/http/httputil"

// Custom transport for debugging
transport := &http.Transport{}
httpClient := &http.Client{
    Transport: transport,
    Timeout:   10 * time.Second,
}

// Use with client...
```

### Testing with CLI Tool

Use the included CLI for quick testing:

```bash
# Discovery
go run ./cmd/soundtouch-cli discover devices

# Device info
go run ./cmd/soundtouch-cli --host 192.168.1.100 info

# Basic controls
go run ./cmd/soundtouch-cli --host 192.168.1.100 play start
go run ./cmd/soundtouch-cli --host 192.168.1.100 volume set --level 50

# WebSocket monitoring (use websocket-demo)
go run ./cmd/websocket-demo --host 192.168.1.100
```

### Configuration Management

```go
// Use environment variables
import "os"

host := os.Getenv("SOUNDTOUCH_HOST")
if host == "" {
    host = "192.168.1.100"  // fallback
}

soundtouch := client.NewClientFromHost(host)
```

## 📚 **Next Steps**

Now that you have the basics working:

1. **Explore Examples**: Check out `/examples` directory for more advanced usage
2. **API Reference**: Read `/docs/API-Endpoints-Overview.md` for complete API details
3. **WebSocket Events**: See `/docs/websocket-events.md` for real-time monitoring
4. **Multiroom Guide**: Check `/docs/zone-management.md` for multiroom setup
5. **Production Guide**: Read `/docs/DEPLOYMENT.md` for production considerations

## 🛟 **Getting Help**

- **Issues**: Create an issue on GitHub with device model and Go version
- **API Reference**: See comprehensive documentation in `/docs`
- **Examples**: Check `/examples` directory for working code samples
- **CLI Tool**: Use the included CLI for testing and debugging

## 🎉 **You're Ready!**

You now have everything needed to build amazing SoundTouch integrations! The library handles all the complexity of device communication, XML parsing, WebSocket management, and error handling - you can focus on building great user experiences.

**Happy coding!** 🎵