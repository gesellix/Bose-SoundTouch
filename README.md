# Bose SoundTouch API Client

A modern Go library and CLI tool for interacting with Bose SoundTouch devices via their Web API.

## Features

### ✅ Implemented (Phase 1)
- **HTTP Client with XML Support**: Complete client for SoundTouch Web API
- **Device Information**: Get detailed device info via `/info` endpoint
- **Device Name**: Get device name via `/name` endpoint
- **Device Capabilities**: Get device capabilities via `/capabilities` endpoint
- **Configured Presets**: Get preset configurations via `/presets` endpoint
- **Now Playing Status**: Get current playback information via `/now_playing` endpoint
- **Audio Sources**: Get available sources via `/sources` endpoint
- **UPnP Discovery**: Automatic device discovery on local network
- **Cross-Platform**: Works on Windows, macOS, Linux, and WASM
- **CLI Tool**: Command-line interface for testing and basic operations
- **Flexible Configuration**: Support for .env files and environment variables
- **Hybrid Discovery**: Combines UPnP discovery with configured device lists

### 🔄 Planned
- Real-time WebSocket events
- Playback control (play, pause, volume, etc.)
- Source management (Spotify, Bluetooth, etc.)
- Preset management
- Web application interface
- Multi-room zone support

## Installation

### Using Go
```bash
go install github.com/gesellix/bose-soundtouch/cmd/soundtouch-cli@latest
```

### From Source
```bash
git clone https://github.com/gesellix/bose-soundtouch.git
cd bose-soundtouch
make build
```

## Quick Start

### Configuration

Create a `.env` file in your working directory to configure preferred devices:

```bash
# Copy the example file
cp .env.example .env
```

Example `.env` configuration:
```bash
# Discovery Settings
DISCOVERY_TIMEOUT=5s
UPNP_ENABLED=true

# Preferred Devices (alternative to UPnP)
# Format: name@host:port;name@host:port;...
PREFERRED_DEVICES="Living Room@192.168.1.100;Kitchen@192.168.1.101;192.168.1.102:8091"

# HTTP Client Settings
HTTP_TIMEOUT=10s
USER_AGENT="Bose-SoundTouch-Go-Client/1.0"
```

### CLI Usage

#### Device Discovery
```bash
# Discover SoundTouch devices (combines UPnP + configured devices)
soundtouch-cli -discover

# Discover and show detailed info for all devices
soundtouch-cli -discover-all
```

#### Device Information
```bash
# Get device information by IP address
soundtouch-cli -host 192.168.1.100 -info

# With custom port and timeout
soundtouch-cli -host 192.168.1.100 -port 8090 -timeout 15s -info
```

#### Now Playing Status
```bash
# Get current playback information
soundtouch-cli -host 192.168.1.100 -nowplaying

# Example output:
# Now Playing:
#   Device ID: A81B6A536A98
#   Source: SPOTIFY
#   Status: Playing
#   Title: In Between Breaths - Paris Unplugged
#   Artist: SYML
#   Album: Paris Unplugged
#   Duration: 2:32 / 3:30
#   Shuffle: Off
#   Repeat: Off
#   Artwork: https://i.scdn.co/image/...
#   Capabilities: Skip, Skip Previous, Seek, Favorite

#### Audio Sources
```bash
# Get available audio sources
soundtouch-cli -host 192.168.1.100 -sources

# Example output:
# Audio Sources:
#   Device ID: A81B6A536A98
#   Total Sources: 14
#   Ready Sources: 5
#
# Ready Sources:
#   • AUX IN [Local, Multiroom]
#   • user+spotify@example.com (user) [Remote, Multiroom, Streaming]
#   • Alexa [Remote, Multiroom]
#   • Tunein [Remote, Multiroom, Streaming]
#   • Local_internet_radio [Remote, Multiroom, Streaming]
#
# Categories:
#   Spotify: 1 account(s) ready
#   AUX Input: Ready
#   Streaming Services: 3 ready
```

#### Device Name
```bash
# Get device name
soundtouch-cli -host 192.168.1.100 -name

# Example output:
# Device Name: Sound Machinechen
```

#### Device Capabilities
```bash
# Get device capabilities
soundtouch-cli -host 192.168.1.100 -capabilities

# Example output:
# Device Capabilities:
#   Device ID: A81B6A536A98
#
# System Features:
#   • Power Saving Disabled
#
# Audio Features:
#   • L/R Stereo
#
# Network Features:
#   • Dual Mode
#   • WSAPI Proxy
#
# Extended Capabilities:
#   • systemtimeout (/systemtimeout)
#   • rebroadcastlatencymode (/rebroadcastlatencymode)
```

#### Configured Presets
```bash
# Get configured presets
soundtouch-cli -host 192.168.1.100 -presets

# Example output:
# Configured Presets:
#   Used Slots: 6/6
#   Spotify Presets: 6
#
# Preset 1: My Playlist
#   Source: SPOTIFY (user@example.com)
#   Type: tracklisturl
#   Created: 2024-06-23 09:40:36
#   Updated: 2024-10-12 15:39:42
#   Artwork: https://i.scdn.co/image/...
#
# Most Recent: Preset 4 (Movie Soundtrack)
```

### Go Library Usage

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"

    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/discovery"
)

func main() {
    // Option 1: Connect to known device
    soundtouchClient := client.NewClientFromHost("192.168.1.100")
    
    deviceInfo, err := soundtouchClient.GetDeviceInfo()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Device: %s (%s)\n", deviceInfo.Name, deviceInfo.Type)

    // Option 2: Discover devices automatically
    discoveryService := discovery.NewDiscoveryService(5 * time.Second)
    ctx := context.Background()
    
    devices, err := discoveryService.DiscoverDevices(ctx)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, device := range devices {
        fmt.Printf("Found: %s at %s:%d\n", device.Name, device.Host, device.Port)
    }
    
    // Get current playback status
    nowPlaying, err := soundtouchClient.GetNowPlaying()
    if err != nil {
        log.Fatal(err)
    }
    
    if !nowPlaying.IsEmpty() {
        fmt.Printf("Now Playing: %s by %s\n", 
            nowPlaying.GetDisplayTitle(), 
            nowPlaying.GetDisplayArtist())
    }
    
    // Get available audio sources
    sources, err := soundtouchClient.GetSources()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Ready Sources: %d/%d\n", 
        sources.GetReadySourceCount(), 
        sources.GetSourceCount())
    
    if sources.HasSpotify() {
        fmt.Println("Spotify is available")
    }
    
    // Get device name
    name, err := soundtouchClient.GetName()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Device: %s\n", name.GetName())
    
    // Get device capabilities
    capabilities, err := soundtouchClient.GetCapabilities()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("L/R Stereo Support: %v\n", capabilities.HasLRStereoCapability())
    
    // Get presets
    presets, err := soundtouchClient.GetPresets()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Presets Used: %d/6\n", len(presets.GetUsedPresetSlots()))
}
```

## Project Structure

```
├── cmd/
│   └── soundtouch-cli/     # CLI application
├── pkg/
│   ├── client/             # HTTP client with XML support
│   ├── discovery/          # UPnP SSDP device discovery
│   └── models/             # XML data models
├── docs/                   # Documentation
└── build/                  # Build artifacts
```

## Development

### Prerequisites
- Go 1.25.5 or later
- Make (optional, for convenience)

### Building
```bash
# Build CLI tool
make build

# Build for all platforms
make build-all

# Build and run tests
make check

# Run tests with coverage
make test-coverage
```

### Testing
```bash
# Run all tests
make test

# Run specific package tests
go test -v ./pkg/client
go test -v ./pkg/discovery

# Test with real devices
make dev-info HOST=192.168.1.100
```

### Development Commands
```bash
# Format code
make fmt

# Run linter (requires golangci-lint)
make lint

# Clean build artifacts
make clean

# Show help
make help
```

## API Documentation

The SoundTouch Web API uses HTTP with XML payloads. Key endpoints include:

- `GET /info` - Device information ✅ Implemented
- `GET /name` - Device name ✅ Implemented
- `GET /capabilities` - Device capabilities ✅ Implemented
- `GET /presets` - Configured presets ✅ Implemented
- `GET /now_playing` - Current playback status ✅ Implemented
- `GET /sources` - Available audio sources ✅ Implemented
- `POST /key` - Send key commands (play, pause, etc.)
- `GET/POST /volume` - Volume control
- WebSocket `/` - Real-time event stream

For complete API documentation, see [docs/API-Endpoints-Overview.md](docs/API-Endpoints-Overview.md).

## Configuration Options

The application supports configuration through `.env` files and environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCOVERY_TIMEOUT` | `5s` | Timeout for device discovery |
| `UPNP_ENABLED` | `true` | Enable/disable UPnP discovery |
| `PREFERRED_DEVICES` | (empty) | Semicolon-separated list of devices |
| `HTTP_TIMEOUT` | `10s` | HTTP client timeout |
| `CACHE_ENABLED` | `true` | Enable device caching |
| `CACHE_TTL` | `30s` | Cache time-to-live |

### Device Configuration Format

The `PREFERRED_DEVICES` environment variable supports multiple formats:

```bash
# Host only (uses default port 8090)
PREFERRED_DEVICES="192.168.1.100"

# Host with port
PREFERRED_DEVICES="192.168.1.100:8091"

# Named device
PREFERRED_DEVICES="Living Room@192.168.1.100"

# Multiple devices
PREFERRED_DEVICES="Living Room@192.168.1.100;Kitchen@192.168.1.101:8091"
```

## Supported Devices

Tested with:
- Bose SoundTouch 10
- Bose SoundTouch 20

Should work with all SoundTouch series devices that support the Web API.

## Real Device Examples

### SoundTouch 10 Response
```xml
<info deviceID="A81B6A536A98">
    <name>Sound Machinechen</name>
    <type>SoundTouch 10</type>
    <moduleType>sm2</moduleType>
    <variant>rhino</variant>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>27.0.6.46330.5043500</softwareVersion>
        </component>
    </components>
</info>
```

### SoundTouch 20 Response
```xml
<info deviceID="08DF1F0BA325">
    <name>A Sound Machine</name>
    <type>SoundTouch 20</type>
    <moduleType>scm</moduleType>
    <variant>spotty</variant>
    <components>
        <component>
            <componentCategory>SCM</componentCategory>
            <softwareVersion>27.0.6.46330.5043500</softwareVersion>
        </component>
        <component>
            <componentCategory>Lightswitch</componentCategory>
        </component>
    </components>
</info>
```

## Architecture

This project follows modern Go patterns:

- **Clean Architecture**: Separated concerns with pkg structure
- **Interface-Based Design**: Testable and mockable components
- **Cross-Platform**: Supports Windows, macOS, Linux, and WASM
- **Test-Driven**: Comprehensive unit and integration tests
- **Real Device Integration**: Tested with actual SoundTouch hardware

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Ensure all tests pass: `make check`
5. Submit a pull request

### Development Guidelines

- **Tests are mandatory**: Every feature needs corresponding tests
- **KISS principle**: Keep implementations simple and readable
- **Small iterations**: Break large features into testable chunks
- **Real device testing**: Validate against actual SoundTouch devices
- **Cross-platform compatibility**: Test on multiple platforms

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## References

- [Official Bose SoundTouch Web API Documentation](docs/2025.12.18%20SoundTouch%20Web%20API.pdf)
- [Project Development Plan](docs/PLAN.md)
- [Development Guidelines](docs/CLAUDE.md)