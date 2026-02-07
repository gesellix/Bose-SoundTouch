# Bose SoundTouch API Client

A comprehensive Go library and CLI tool for controlling Bose SoundTouch devices via their Web API.

[![Go Reference](https://pkg.go.dev/badge/github.com/gesellix/bose-soundtouch.svg)](https://pkg.go.dev/github.com/gesellix/bose-soundtouch)
[![Go Report Card](https://goreportcard.com/badge/github.com/gesellix/bose-soundtouch)](https://goreportcard.com/report/github.com/gesellix/bose-soundtouch)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

> **Note**: This is an independent project based on the [official Bose SoundTouch Web API documentation](https://assets.bosecreative.com/m/496577402d128874/original/SoundTouch-Web-API.pdf). Not affiliated with or endorsed by Bose Corporation.

## Features

- ✅ **Complete API Coverage**: All available SoundTouch Web API endpoints implemented
- 🎵 **Media Control**: Play, pause, stop, volume, bass, balance, source selection
- 🔔 **Smart Notifications**: TTS messages, URL audio content, notification beeps (ST-10)
- 🏠 **Multiroom Support**: Create and manage zones across multiple speakers
- ⚡ **Real-time Events**: WebSocket connection for live device state monitoring
- 🔍 **Device Discovery**: Automatic discovery via UPnP/SSDP and mDNS
- 📻 **Content Navigation**: Browse and search TuneIn, Pandora, Spotify, local music
- 🎙️ **Station Management**: Add and play radio stations without presets
- 🖥️ **CLI Tool**: Comprehensive command-line interface
- 🌐 **SoundTouch Service**: Emulate Bose services and proxy device traffic (offline support)
- 🔒 **Production Ready**: Extensive testing with real SoundTouch hardware
- 🌐 **Cross-Platform**: Windows, macOS, Linux support

## Quick Start

### Installation

#### Install CLI and Service Tools
```bash
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-cli@latest
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-service@latest
```

#### Add Library to Your Project
```bash
go get github.com/gesellix/bose-soundtouch
```

### CLI Usage

#### Discover Devices
```bash
# Find SoundTouch devices on your network
soundtouch-cli discover devices
```

# Control a Device
```bash
# Basic device information
soundtouch-cli --host 192.168.1.100 info get

# Media controls
soundtouch-cli --host 192.168.1.100 play start
soundtouch-cli --host 192.168.1.100 volume set --level 50
soundtouch-cli --host 192.168.1.100 source select --source SPOTIFY

# Preset management
soundtouch-cli --host 192.168.1.100 preset list
soundtouch-cli --host 192.168.1.100 preset store-current --slot 1
soundtouch-cli --host 192.168.1.100 preset select --slot 1

# Browse and discover content
soundtouch-cli --host 192.168.1.100 browse tunein
soundtouch-cli --host 192.168.1.100 station search-tunein --query "jazz"
soundtouch-cli --host 192.168.1.100 station add --source TUNEIN --token <token> --name "Jazz Radio"

# Speaker notifications (ST-10 only)
soundtouch-cli --host 192.168.1.100 speaker tts --text "Welcome home" --app-key YOUR_KEY
soundtouch-cli --host 192.168.1.100 speaker url --url "https://example.com/doorbell.mp3" --app-key YOUR_KEY
soundtouch-cli --host 192.168.1.100 speaker beep

# Real-time monitoring
soundtouch-cli --host 192.168.1.100 events subscribe
```

### Service Usage

The `soundtouch-service` provides a REST API and can emulate Bose backend services (BMX/Marge), which is useful for offline device usage or custom service integration.

#### Start the Service
```bash
# Start with default settings (port 8000)
soundtouch-service
```

#### Key Service Features
- **Device Discovery**: Automatically scans and lists Bose devices.
- **Service Emulation**: Emulates Bose BMX and Marge services.
- **Logging Proxy**: Intercept and log traffic between your device and the service.
- **Embedded Web UI**: Management interface available at `http://localhost:8000/`.

See [docs/SOUNDTOUCH-SERVICE.md](docs/SOUNDTOUCH-SERVICE.md) for detailed configuration and API usage.

### Library Usage

#### Basic Control
```go
package main

import (
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
)

func main() {
    // Connect to your SoundTouch device
    c := client.NewClient(&client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    })
    
    // Get device information
    info, err := c.GetDeviceInfo()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Device: %s\n", info.Name)
    
    // Control playback
    err = c.Play()
    if err != nil {
        log.Fatal(err)
    }
    
    // Set volume
    err = c.SetVolume(50)
    if err != nil {
        log.Fatal(err)
    }
}
```

#### Device Discovery
```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
    // Discover SoundTouch devices
    service := discovery.NewService(5 * time.Second)
    devices, err := service.DiscoverDevices(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    
    for _, device := range devices {
        fmt.Printf("Found: %s at %s:%d\n", 
            device.Name, device.Host, device.Port)
    }
}
```

#### Real-time Events
```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    c := client.NewClient(&client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    })
    
    // Subscribe to device events
    events, err := c.SubscribeToEvents(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    
    for event := range events {
        switch e := event.(type) {
        case *models.NowPlayingUpdated:
            fmt.Printf("Now playing: %s by %s\n", e.Track, e.Artist)
        case *models.VolumeUpdated:
            fmt.Printf("Volume changed to: %d\n", e.ActualVolume)
        case *models.ConnectionStateUpdated:
            fmt.Printf("Connection state: %s\n", e.State)
        }
    }
}
```

#### Preset Management
```go
package main

import (
    "fmt"
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    c := client.NewClient(&client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    })
    
    // Get current presets
    presets, err := c.GetPresets()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Found %d presets\n", len(presets.Preset))
    
    // Store currently playing content as preset 1
    err = c.StoreCurrentAsPreset(1)
    if err != nil {
        log.Fatal(err)
    }
    
    // Store Spotify playlist as preset 2
    spotifyContent := &models.ContentItem{
        Source:        "SPOTIFY",
        Type:          "uri",
        Location:      "spotify:playlist:37i9dQZF1DXcBWIGoYBM5M",
        SourceAccount: "your_username",
        IsPresetable:  true,
        ItemName:      "Today's Top Hits",
    }
    err = c.StorePreset(2, spotifyContent)
    if err != nil {
        log.Fatal(err)
    }
    
    // Store radio station as preset 3
    radioContent := &models.ContentItem{
        Source:       "TUNEIN",
        Type:         "stationurl",
        Location:     "/v1/playbook/station/s33828",
        IsPresetable: true,
        ItemName:     "K-LOVE Radio",
    }
    err = c.StorePreset(3, radioContent)
    if err != nil {
        log.Fatal(err)
    }
    
    // Select preset 1
    err = c.SelectPreset(1)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Preset management complete!")
}
```

#### Multiroom Zones
```go
package main

import (
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    master := client.NewClient(&client.Config{
        Host: "192.168.1.100", // Master speaker
        Port: 8090,
    })
    
    // Create a multiroom zone
    zone := &models.Zone{
        Master: "192.168.1.100",
        Members: []models.ZoneMember{
            {IPAddress: "192.168.1.101"}, // Living room
            {IPAddress: "192.168.1.102"}, // Kitchen
        },
    }
    
    err := master.SetZone(zone)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Multiroom zone created!")
}
```

#### Speaker Notifications (ST-10 only)
```go
package main

import (
    "log"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
)

func main() {
    c := client.NewClient(&client.Config{
        Host: "192.168.1.100",
        Port: 8090,
    })
    
    // Play Text-to-Speech message
    err := c.PlayTTS("Welcome home!", "your-app-key", 70)
    if err != nil {
        log.Fatal(err)
    }
    
    // Play audio content from URL
    err = c.PlayURL(
        "https://example.com/doorbell.mp3",
        "your-app-key",
        "Doorbell",
        "Front Door",
        "Visitor Alert",
        80,
    )
    if err != nil {
        log.Fatal(err)
    }
    
    // Play notification beep
    err = c.PlayNotificationBeep()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println("Notifications sent!")
}
```

## Supported Devices

This library supports all Bose SoundTouch-compatible devices, including:

- SoundTouch 10, 20, 30 series
- SoundTouch Portable  
- Wave SoundTouch music system
- SoundTouch-enabled Bose speakers

**Tested Hardware**:
- ✅ SoundTouch 10
- ✅ SoundTouch 20

## API Coverage

| Feature | Status | Description |
|---------|--------|-------------|
| Device Info | ✅ Complete | Device details, name, capabilities |
| Media Control | ✅ Complete | Play/pause/stop, track navigation |
| Volume & Audio | ✅ Complete | Volume, bass, balance control |
| Source Selection | ✅ Complete | Spotify, Bluetooth, AUX, etc. |
| Content Navigation | ✅ Complete | Browse music libraries, radio stations |
| Station Management | ✅ Complete | Search, add, remove stations |
| Preset Management | ✅ Complete | Store, select, remove presets |
| Real-time Events | ✅ Complete | WebSocket event streaming |
| Multiroom Zones | ✅ Complete | Zone creation and management |
| Speaker Notifications | ✅ Complete | TTS, URL audio, beep alerts (ST-10) |
| System Settings | ✅ Complete | Clock, display, network info |
| Advanced Audio | ✅ Complete | DSP controls, tone controls |

**API Limitations**: None - all documented SoundTouch Web API functionality is implemented, including endpoints discovered via the comprehensive [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API).

## Documentation

- 📖 [Contributing Guide](CONTRIBUTING.md) - How to contribute to the project
- 📚 [API Reference](docs/API-Endpoints-Overview.md) - Complete endpoint documentation
- 🔧 [CLI Reference](docs/CLI-REFERENCE.md) - Command-line tool guide
- 🎯 [Getting Started](docs/GETTING-STARTED.md) - Detailed setup and usage
- 📻 [Preset Quick Start](docs/PRESET-QUICKSTART.md) - Favorite content management
- 🧭 [Navigation Guide](docs/NAVIGATION-GUIDE.md) - Content browsing and station management
- 📋 [Navigation API Reference](docs/API-NAVIGATION-REFERENCE.md) - Navigation API documentation
- ⚙️ [Advanced Features](docs/SYSTEM-ENDPOINTS.md) - Advanced functionality
- 🏠 [Multiroom Setup](docs/zone-management.md) - Zone configuration guide
- ⚡ [WebSocket Events](docs/websocket-events.md) - Real-time event handling
- 🔔 [Speaker Notifications](docs/SPEAKER_ENDPOINT.md) - TTS and audio notifications guide
- 🔍 [Device Discovery](docs/DISCOVERY.md) - Discovery configuration
- 🛠️ [Troubleshooting](docs/TROUBLESHOOTING.md) - Common issues and solutions

## Development

### Prerequisites
- Go 1.25.6 or later
- Optional: SoundTouch device for testing

### Building from Source
```bash
# Clone the repository
git clone https://github.com/gesellix/bose-soundtouch.git
cd Bose-SoundTouch

# Install dependencies
go mod download

# Build CLI tool
make build

# Run tests
make test

# Install CLI locally
go install ./cmd/soundtouch-cli
```

### Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details on:

- Setting up your development environment
- Coding guidelines and best practices
- Testing with real devices
- Submitting pull requests

## Examples

Check out the [examples/](examples/) directory for more usage patterns:

- **Basic HTTP Client**: Simple device control
- **Preset Management**: Store and manage favorite content
- **Navigation & Stations**: Browse content and manage radio stations
- **WebSocket Events**: Real-time monitoring
- **Device Discovery**: Finding devices on your network
- **Multiroom Management**: Zone operations
- **Advanced Audio**: DSP and tone controls

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Disclaimer

This is an independent project based on the official Bose SoundTouch Web API documentation provided by Bose Corporation. It is not affiliated with, endorsed by, or supported by Bose Corporation. Use at your own risk.

SoundTouch is a trademark of Bose Corporation.

## SoundTouch End of Life Notice

**Important:** Bose has announced that [SoundTouch cloud support will end on May 6, 2026](https://www.bose.com/soundtouch-end-of-life).

**What will continue to work:**
- ✅ Local API control (this library's primary functionality)
- ✅ Bluetooth, AirPlay, Spotify Connect, and AUX streaming
- ✅ Remote control features (Play, Pause, Skip, Volume)
- ✅ Multiroom grouping

**What will stop working:**
- ❌ Cloud-based preset sync between devices and SoundTouch app
- ❌ Browsing music services directly from the SoundTouch app
- ❌ Cloud-based features and updates

**What continues to work:**
- ✅ Local preset management via this API client (store, select, remove)
- ✅ Direct content playback (stations, playlists, etc.)

This Go library will continue to work as it uses the local Web API for direct device control, which is unaffected by the cloud service discontinuation. The local preset management functionality implemented in this library (discovered through the [SoundTouch Plus Wiki](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)) provides an alternative to the cloud-based preset features that will be discontinued.

**Community Alternatives**: See the [Related Projects](#related-projects) section below for additional tools like SoundCork that provide cloud service alternatives and the SoundTouch Plus project that offers comprehensive Home Assistant integration.

## Related Projects

### SoundTouch Plus
- **Project**: [SoundTouch Plus Home Assistant Component](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus)
- **Wiki**: [SoundTouch WebServices API Documentation](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- **Description**: Comprehensive Home Assistant integration with extensive API documentation
- **Contribution**: The SoundTouch Plus Wiki provided invaluable documentation of working endpoints beyond the official API, enabling the preset management and content navigation features in this library

### SoundCork
- **Project**: [SoundCork - SoundTouch API Intercept](https://github.com/deborahgu/soundcork)
- **Description**: Intercept API for Bose SoundTouch devices after cloud service discontinuation
- **Purpose**: Provides a local alternative to cloud-based SoundTouch services post-sunset
- **Compatibility**: Complements this Go library by extending functionality beyond the local device API

These projects form a comprehensive ecosystem for SoundTouch device management and provide alternatives to Bose's discontinued cloud services.

## Support

- 🐛 **Bug Reports**: [Create an issue](https://github.com/gesellix/bose-soundtouch/issues/new)
- 💡 **Feature Requests**: [Start a discussion](https://github.com/gesellix/bose-soundtouch/discussions)
- ❓ **Questions**: Check [existing discussions](https://github.com/gesellix/bose-soundtouch/discussions)
- 📖 **Documentation**: Browse the [docs/](docs/) directory

---

**Star this project** ⭐ if you find it useful!
