# Bose SoundTouch Toolkit

A comprehensive solution for controlling and preserving Bose SoundTouch devices, including a Go library, CLI tool, and a local service for cloud emulation.

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
- 🌐 **SoundTouch Service**: Emulate Bose cloud services for offline device operation
- 🔧 **Service Migration**: Migrate devices to use local services instead of Bose cloud
- 📊 **Traffic Analysis**: Proxy and log device communications
- 📝 **HTTP Recording**: Persist interactions as re-playable `.http` files
- 🧹 **Session Management**: Manage and cleanup recorded interaction sessions
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

Find SoundTouch devices on your network:
```bash
soundtouch-cli discover devices
```

Control a device (replace `192.168.1.100` with your speaker's IP):
```bash
# Basic information
soundtouch-cli --host 192.168.1.100 info

# Media controls
soundtouch-cli --host 192.168.1.100 play start
soundtouch-cli --host 192.168.1.100 volume set --level 50

# Preset management
soundtouch-cli --host 192.168.1.100 preset list
```

For full CLI documentation, see the [CLI Reference](https://gesellix.github.io/Bose-SoundTouch/guides/CLI-REFERENCE.html).

### SoundTouch Service (Cloud Shutdown Protection)

The `soundtouch-service` is a local server that emulates Bose's cloud services. This is critical for keeping your speakers functional after the **Bose Cloud Shutdown in May 2026**.

#### Key Features:
- **🏠 Local Emulation**: BMX and Marge service implementation
- **🔌 Easy Setup**: Activate SSH via USB stick (`remote_services` file)
- **🔧 Device Migration**: Seamlessly transition devices to local control
- **🌐 Web Management UI**: Easy browser-based setup and management
- **💾 Persistent Data**: Store presets, recents, and sources locally
- **📝 HTTP Recording**: Persist all interactions as re-playable `.http` files
- **🧹 Session Management**: Manage and cleanup recorded interaction sessions

#### Quick Start:
```bash
# Start the service
soundtouch-service
```
Open `http://localhost:8000` in your browser to manage your devices. Documentation is also available directly through the web interface.

For a comprehensive guide on transitioning your system, see the [Bose Cloud Shutdown: Survival Guide](https://gesellix.github.io/Bose-SoundTouch/guides/SURVIVAL-GUIDE.html).

Detailed service configuration and Docker instructions can be found in [SoundTouch Service Guide](https://gesellix.github.io/Bose-SoundTouch/guides/SOUNDTOUCH-SERVICE.html).

For professional migration tips and safety measures, see the [Migration & Safety Guide](https://gesellix.github.io/Bose-SoundTouch/guides/MIGRATION-SAFETY.html).

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
 - 📚 [API Reference](https://gesellix.github.io/Bose-SoundTouch/reference/API-ENDPOINTS.html) - Complete endpoint documentation
- 🔧 [CLI Reference](https://gesellix.github.io/Bose-SoundTouch/guides/CLI-REFERENCE.html) - Command-line tool guide
- 🌐 [SoundTouch Service Guide](https://gesellix.github.io/Bose-SoundTouch/guides/SOUNDTOUCH-SERVICE.html) - Local service setup and migration
- 🎯 [Getting Started](https://gesellix.github.io/Bose-SoundTouch/guides/GETTING-STARTED.html) - Detailed setup and usage
- 📻 [Preset Quick Start](https://gesellix.github.io/Bose-SoundTouch/PRESET-QUICKSTART.md) - Favorite content management
- 🧭 [Navigation Guide](https://gesellix.github.io/Bose-SoundTouch/NAVIGATION-GUIDE.md) - Content browsing and station management
- 📋 [Navigation API Reference](https://gesellix.github.io/Bose-SoundTouch/API-NAVIGATION-REFERENCE.md) - Navigation API documentation
- ⚙️ [Advanced Features](https://gesellix.github.io/Bose-SoundTouch/reference/SYSTEM-ENDPOINTS.html) - Advanced functionality
- 🏠 [Multiroom Setup](https://gesellix.github.io/Bose-SoundTouch/reference/ZONE-MANAGEMENT.html) - Zone configuration guide
- ⚡ [WebSocket Events](https://gesellix.github.io/Bose-SoundTouch/reference/WEBSOCKET-EVENTS.html) - Real-time event handling
- 🔔 [Speaker Notifications](https://gesellix.github.io/Bose-SoundTouch/reference/SPEAKER-ENDPOINT.html) - TTS and audio notifications guide
- 🔍 [Device Discovery](https://gesellix.github.io/Bose-SoundTouch/reference/DISCOVERY.html) - Discovery configuration
- 🛠️ [Troubleshooting](https://gesellix.github.io/Bose-SoundTouch/guides/TROUBLESHOOTING.html) - Common issues and solutions

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

## Related Projects & Credits

This project builds upon the excellent work of several community projects:

### SoundCork 🍾
- **Project**: [SoundCork - SoundTouch API Intercept](https://github.com/deborahgu/soundcork)
- **Authors**: Deborah Kaplan and contributors
- **Our Implementation**: The `soundtouch-service` in this project is heavily inspired by SoundCork's Python implementation. SoundCork pioneered the approach of intercepting and emulating Bose's cloud services, providing the foundation for offline SoundTouch operation.
- **Key Contributions**: Service emulation architecture, BMX/Marge endpoint discovery, device migration strategies
- **License**: MIT License

### ÜberBöse API 🎵
- **Project**: [ÜberBöse API](https://github.com/julius-d/ueberboese-api)
- **Author**: Julius
- **Our Implementation**: This project provided valuable insights into advanced SoundTouch API endpoints and helped make our implementation more complete, particularly for content navigation and advanced device features.
- **Key Contributions**: Extended API endpoint documentation, advanced feature discovery
- **License**: MIT License

### SoundTouch Plus 🏠
- **Project**: [SoundTouch Plus Home Assistant Component](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus)
- **Wiki**: [SoundTouch WebServices API Documentation](https://github.com/thlucas1/homeassistantcomponent_soundtouchplus/wiki/SoundTouch-WebServices-API)
- **Author**: Todd Lucas
- **Our Implementation**: The comprehensive API documentation in the SoundTouch Plus Wiki provided invaluable insights into undocumented endpoints beyond the official API, enabling our preset management and content navigation features.
- **Key Contributions**: Extensive API endpoint documentation, real-world usage patterns
- **License**: MIT License

### SoundTouch Hook 🪝
- **Project**: [Bose SoundTouch Hook](https://github.com/CodeFinder2/bose-soundtouch-hook)
- **Author**: Adrian Böckenkamp
- **Our Implementation**: This project provides a powerful framework for intercepting and hooking into internal device processes using `LD_PRELOAD`. It was instrumental in verifying internal function calls and understanding how the device validates cloud domains.
- **Key Contributions**: Reverse engineering framework, process hooking, cross-compilation toolchain
- **License**: GPL-3.0 License

### Community Ecosystem

These projects together form a comprehensive ecosystem for SoundTouch device management:

- **This Project**: Go library + CLI + service for programmatic control and offline operation
- **SoundCork**: Python-based service interception and cloud replacement  
- **SoundTouch Plus**: Home Assistant integration with extensive device support
- **ÜberBöse**: API research and advanced endpoint discovery
- **SoundTouch Hook**: Advanced reverse engineering and process instrumentation

We are grateful to these projects and their maintainers for paving the way and providing the foundation that made this comprehensive Go implementation possible. The SoundTouch community's collaborative approach to reverse engineering and documentation has been invaluable.

### Contributing Back

If you discover new endpoints, features, or improvements through this library, please consider contributing back to these projects as well. The stronger our community ecosystem becomes, the better we can support SoundTouch devices beyond Bose's official support timeline.

## Support

- 🐛 **Bug Reports**: [Create an issue](https://github.com/gesellix/bose-soundtouch/issues/new)
- 💡 **Feature Requests**: [Start a discussion](https://github.com/gesellix/bose-soundtouch/discussions)
- ❓ **Questions**: Check [existing discussions](https://github.com/gesellix/bose-soundtouch/discussions)
- 📖 **Documentation**: [Online Documentation](https://gesellix.github.io/Bose-SoundTouch/)
- 🔍 **New Discoveries**: [Undocumented Community Features](https://gesellix.github.io/Bose-SoundTouch/UNDOCUMENTED-COMMUNITY-FEATURES.md)
- 🌐 **Upstream Analysis**: [Upstream URLs & Domains](https://gesellix.github.io/Bose-SoundTouch/analysis/UPSTREAM-URLS.html)
- 🔧 **Redirection Guide**: [Device Redirect Methods](https://gesellix.github.io/Bose-SoundTouch/analysis/DEVICE-REDIRECT-METHODS.html)
- 🐣 **Initial Setup**: [Device Initial Setup Variants](https://gesellix.github.io/Bose-SoundTouch/guides/DEVICE-INITIAL-SETUP.html)
- 📜 **Logging & Debugging**: [Device Logging Guide](https://gesellix.github.io/Bose-SoundTouch/DEVICE-LOGGING.md)
- 🔒 **HTTPS & CA Setup**: [HTTPS & Custom CA Guide](https://gesellix.github.io/Bose-SoundTouch/guides/HTTPS-SETUP.html)

---

**Star this project** ⭐ if you find it useful!
