# Bose SoundTouch API Client

A modern Go library and CLI tool for interacting with Bose SoundTouch devices via their Web API.

## Features

### ✅ Implemented
- **HTTP Client with XML Support**: Complete client for SoundTouch Web API
- **Device Information**: Get detailed device info via `/info` endpoint
- **Device Name**: Get device name via `/name` endpoint
- **Device Capabilities**: Get device capabilities via `/capabilities` endpoint
- **Configured Presets**: Get preset configurations via `/presets` endpoint
- **Now Playing Status**: Get current playback information via `/now_playing` endpoint
- **Audio Sources**: Get available sources via `/sources` endpoint
- **Media Controls**: Play, pause, stop, track navigation via `/key` endpoint
- **Volume Management**: Get/set volume, incremental control via `/volume` endpoint
- **Host:Port Parsing**: Enhanced CLI with automatic host:port parsing
- **UPnP/SSDP Discovery**: Automatic device discovery using Universal Plug and Play
- **mDNS/Bonjour Discovery**: Multicast DNS device discovery support
- **Cross-Platform**: Works on Windows, macOS, Linux, and WASM
- **CLI Tool**: Command-line interface for testing and control operations
- **Flexible Configuration**: Support for .env files and environment variables
- **Unified Discovery**: Combines UPnP, mDNS, and configured device lists
- **Safety Features**: Volume warnings, increment limits, error validation
- **System Management**: Clock/time settings, network information, device diagnostics

### 🔄 Planned
- Real-time WebSocket events
- Preset management (create/update presets)
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
MDNS_ENABLED=true

# Preferred Devices (alternative to UPnP)
# Format: name@host:port;name@host:port;...
PREFERRED_DEVICES="Living Room@192.168.1.10;Kitchen@192.168.1.11;192.168.1.12:8091"

# HTTP Client Settings
HTTP_TIMEOUT=10s
USER_AGENT="Bose-SoundTouch-Go-Client/1.0"
```

### CLI Usage

#### Device Discovery

The library supports multiple discovery methods automatically:
- **Configuration**: Manually specified devices in `.env` file (fastest, most reliable)
- **UPnP/SSDP**: Universal Plug and Play discovery (widely supported)
- **mDNS/Bonjour**: Multicast DNS discovery (Apple ecosystem friendly)

See [docs/DISCOVERY.md](docs/DISCOVERY.md) for detailed information.

```bash
# Discover SoundTouch devices (combines UPnP, mDNS + configured devices)
soundtouch-cli -discover

# Discover and show detailed info for all devices
soundtouch-cli -discover-all

# Discover with custom timeout
soundtouch-cli -discover -timeout 10s
```

#### Device Information
```bash
# Get device information by IP address
soundtouch-cli -host 192.168.1.10 -info

# With custom port and timeout
soundtouch-cli -host 192.168.1.10 -port 8090 -timeout 15s -info
```

#### Now Playing Status
```bash
# Get current playback information
soundtouch-cli -host 192.168.1.10 -nowplaying

# Example output:
# Now Playing:
#   Device ID: ABCD1234EFGH
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
soundtouch-cli -host 192.168.1.10:8090 -sources

# Example output:
# Audio Sources:
#   Device ID: ABCD1234EFGH
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

#### Media Controls
```bash
# Basic playback controls
soundtouch-cli -host 192.168.1.10:8090 -play
soundtouch-cli -host 192.168.1.10:8090 -pause
soundtouch-cli -host 192.168.1.10:8090 -stop

# Track navigation
soundtouch-cli -host 192.168.1.10:8090 -next
soundtouch-cli -host 192.168.1.10:8090 -prev

# Volume controls (key-based)
soundtouch-cli -host 192.168.1.10:8090 -volume-up
soundtouch-cli -host 192.168.1.10:8090 -volume-down

# Preset selection
soundtouch-cli -host 192.168.1.10:8090 -preset 1
soundtouch-cli -host 192.168.1.10:8090 -preset 6

# Generic key command
soundtouch-cli -host 192.168.1.10:8090 -key STOP
```

#### Volume Management
```bash
# Get current volume
soundtouch-cli -host 192.168.1.10:8090 -volume

# Example output:
# Current Volume:
#   Device ID: ABCD1234EFGH
#   Current Level: 50 (Medium)
#   Target Level: 50
#   Muted: false

# Set specific volume (0-100, shows warning for >30)
soundtouch-cli -host 192.168.1.10:8090 -set-volume 25
soundtouch-cli -host 192.168.1.10:8090 -set-volume 0  # Mute

# Incremental volume control
soundtouch-cli -host 192.168.1.10:8090 -inc-volume 3
soundtouch-cli -host 192.168.1.10:8090 -dec-volume 5
```

#### Device Name
```bash
# Get device name
soundtouch-cli -host 192.168.1.10 -name

# Example output:
# Device Name: My SoundTouch
```

#### Device Capabilities
```bash
# Get device capabilities
soundtouch-cli -host 192.168.1.10 -capabilities

# Example output:
# Device Capabilities:
#   Device ID: ABCD1234EFGH
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
soundtouch-cli -host 192.168.1.10 -presets

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

#### System Information
```bash
# Get device clock time
soundtouch-cli -host 192.168.1.10 -clock-time

# Set device time to current system time
soundtouch-cli -host 192.168.1.10 -set-clock-time now

# Get clock display settings
soundtouch-cli -host 192.168.1.10 -clock-display

# Configure clock display
soundtouch-cli -host 192.168.1.10 -enable-clock
soundtouch-cli -host 192.168.1.10 -clock-format 24
soundtouch-cli -host 192.168.1.10 -clock-brightness 75

# Get network information
soundtouch-cli -host 192.168.1.10 -network-info
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
    soundtouchClient := client.NewClientFromHost("192.168.1.10")
    
    deviceInfo, err := soundtouchClient.GetDeviceInfo()
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Printf("Device: %s (%s)\n", deviceInfo.Name, deviceInfo.Type)

    // Option 2: Discover devices automatically (unified: UPnP + mDNS + config)
    cfg, _ := config.LoadFromEnv()
    discoveryService := discovery.NewUnifiedDiscoveryService(cfg)
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
    
    // Media controls
    fmt.Println("Playing music...")
    if err := soundtouchClient.Play(); err != nil {
        log.Printf("Failed to play: %v", err)
    }
    
    // Volume control
    volume, err := soundtouchClient.GetVolume()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Current Volume: %d (%s)\n", 
        volume.GetLevel(), 
        volume.GetVolumeString())
    
    // Set comfortable volume
    if err := soundtouchClient.SetVolume(25); err != nil {
        log.Printf("Failed to set volume: %v", err)
    }
}
```

## Project Structure

```
├── cmd/
│   └── soundtouch-cli/     # CLI application
├── pkg/
│   ├── client/             # HTTP client with XML support
│   ├── discovery/          # Device discovery (UPnP/SSDP + mDNS/Bonjour)
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
make dev-info HOST=192.168.1.10

# Test device discovery
make dev-discover

# Test mDNS discovery example
make dev-mdns
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
- `POST /key` - Send key commands (play, pause, etc.) ✅ Implemented
- `GET/POST /volume` - Volume control ✅ Implemented
- `GET/POST /bass` - Bass control ✅ Implemented
- `GET/POST /balance` - Stereo balance control ✅ Implemented
- `POST /select` - Source selection ✅ Implemented
- `GET/POST /clockTime` - Device time management ✅ Implemented
- `GET/POST /clockDisplay` - Clock display settings ✅ Implemented
- `GET /networkInfo` - Network information ✅ Implemented
- WebSocket `/` - Real-time event stream

For complete API documentation, see:
- [API Endpoints Overview](docs/API-Endpoints-Overview.md)
- [Key Controls Documentation](docs/KEY-CONTROLS.md)
- [Volume Controls Documentation](docs/VOLUME-CONTROLS.md)
- [Host:Port Parsing Feature](docs/HOST-PORT-PARSING.md)

## Configuration Options

The application supports configuration through `.env` files and environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `DISCOVERY_TIMEOUT` | `5s` | Timeout for device discovery |
| `UPNP_ENABLED` | `true` | Enable/disable UPnP/SSDP discovery |
| `MDNS_ENABLED` | `true` | Enable/disable mDNS/Bonjour discovery |
| `PREFERRED_DEVICES` | (empty) | Semicolon-separated list of devices |
| `HTTP_TIMEOUT` | `10s` | HTTP client timeout |
| `CACHE_ENABLED` | `true` | Enable device caching |
| `CACHE_TTL` | `30s` | Cache time-to-live |

### Device Configuration Format

The `PREFERRED_DEVICES` environment variable supports multiple formats:

```bash
# Host only (uses default port 8090)
PREFERRED_DEVICES="192.168.1.10"

# Host with port
PREFERRED_DEVICES="192.168.1.10:8091"

# Named device
PREFERRED_DEVICES="Living Room@192.168.1.10"

# Multiple devices
PREFERRED_DEVICES="Living Room@192.168.1.10;Kitchen@192.168.1.11:8091"
```

## Supported Devices

Tested with:
- Bose SoundTouch 10
- Bose SoundTouch 20

Should work with all SoundTouch series devices that support the Web API.

## Real Device Examples

### SoundTouch 10 Response
```xml
<info deviceID="ABCD1234EFGH">
    <name>My SoundTouch Device</name>
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
<info deviceID="ABCD1234EFGH">
    <name>My SoundTouch Device</name>
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