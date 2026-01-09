# Bose SoundTouch API Client - Golang Implementation Plan

## Overview

This document describes the planning for a Golang-based API client for the Bose SoundTouch Web API. The client follows modern Go patterns and supports both native Go library and WASM integration with embedded web UI.

**New insights from pattern analysis:**
- Single binary deployment with embedded assets
- Multi-target build system (Native + WASM)
- CORS proxy pattern for browser integration
- Robust XML API client patterns
- Production-ready configuration management

## API Fundamentals

### Basic Information
- **Protocol**: HTTP REST-like
- **Data format**: XML Request/Response
- **Port**: 8090 (default)
- **Authentication**: No complex authentication required
- **Real-time updates**: WebSocket connection available
- **Device discovery**: UPnP discovery possible

### Core API Endpoints
- `GET /info` - Device information
- `GET /now_playing` - Currently playing music
- `POST /key` - Send key commands (PLAY, PAUSE, etc.)
- `GET/POST /volume` - Control volume
- `GET/POST /bass` - Bass settings
- `GET/POST /sources` - Available sources
- `POST /select` - Select source
- `GET /presets` - Read presets (1-6) - POST officially not supported
- `WebSocket /` - Live updates for events

## Architecture Based on Modern Go Patterns

### Final Project Structure
```
github.com/gesellix/bose-soundtouch/
├── cmd/
│   ├── cli/                    # CLI Tool (Main Application)
│   │   └── main.go
│   ├── webapp/                 # Web Application with embedded Assets
│   │   ├── main.go
│   │   └── web/               # Embedded HTML/CSS/JS
│   │       ├── index.html
│   │       ├── app.js
│   │       └── style.css
│   └── wasm/                   # WASM Entry Point
│       └── main.go
├── pkg/                        # Public API (external usage)
│   ├── client/                 # HTTP Client with XML Support
│   ├── discovery/              # UPnP Device Discovery
│   ├── models/                 # Type-safe XML Data Models
│   ├── websocket/              # Event Streaming Client
│   ├── wasm/                   # WASM JavaScript Bridge
│   └── config/                 # Configuration Management
├── internal/                   # Private Implementation Details
│   ├── xml/                    # XML Parsing Utilities
│   ├── http/                   # HTTP Utilities & Middleware
│   └── testing/                # Mock Client & Test Utilities
├── web/                        # Frontend Development Assets
│   ├── src/                    # Source files
│   └── dist/                   # Build output → cmd/webapp/web/
├── examples/                   # Usage Examples & Demos
├── test/                       # Integration Tests & Docker
├── Makefile                    # Comprehensive Build System
├── .env.example               # Configuration Template
├── .air-webapp.toml           # Hot Reload Config
├── .air-wasm.toml             # WASM Development Config
├── docker-compose.yml         # Development Environment
├── PROJECT-PATTERNS.md        # Pattern Documentation
├── API-Endpoints-Overview.md  # API Reference
├── go.mod
└── README.md
```

### Core Components (Updated)

#### 1. HTTP Client with XML Support (`pkg/client`)
```go
type Client struct {
    baseURL    string
    httpClient *http.Client
    timeout    time.Duration
    userAgent  string
}

type ClientConfig struct {
    Host      string
    Port      int
    Timeout   time.Duration
    UserAgent string
}

// Core API methods
func NewClient(config ClientConfig) *Client
func (c *Client) GetDeviceInfo() (*models.DeviceInfo, error)
func (c *Client) GetNowPlaying() (*models.NowPlaying, error)
func (c *Client) SetVolume(volume int) error
func (c *Client) GetVolume() (*models.Volume, error)
func (c *Client) SendKey(key models.Key) error
func (c *Client) GetSources() (*models.Sources, error)
func (c *Client) SelectSource(source models.ContentItem) error
func (c *Client) GetPresets() (*models.Presets, error)
func (c *Client) SetPreset(id int, content models.ContentItem) error

// HTTP utilities with XML handling
func (c *Client) get(endpoint string, result interface{}) error
func (c *Client) post(endpoint string, data interface{}, result interface{}) error
```

#### 2. UPnP Device Discovery (`pkg/discovery`)
```go
type DiscoveryService struct {
    timeout    time.Duration
    cache      map[string]*Device
    cacheTTL   time.Duration
    mutex      sync.RWMutex
}

type Device struct {
    Name        string    `json:"name"`
    Host        string    `json:"host"`
    Port        int       `json:"port"`
    ModelID     string    `json:"modelId"`
    SerialNo    string    `json:"serialNo"`
    Location    string    `json:"location"`
    LastSeen    time.Time `json:"lastSeen"`
}

func NewDiscoveryService(timeout time.Duration) *DiscoveryService
func (d *DiscoveryService) DiscoverDevices() ([]Device, error)
func (d *DiscoveryService) DiscoverDevice(name string) (*Device, error)
func (d *DiscoveryService) GetCachedDevices() []Device
func (d *DiscoveryService) ClearCache()

// SSDP/UPnP implementation
func (d *DiscoveryService) sendMSearch() error
func (d *DiscoveryService) parseResponse(response string) (*Device, error)
```

#### 3. Typsichere XML Models (`pkg/models`)
```go
// Base XML response with error handling
type XMLResponse struct {
    XMLName xml.Name  `xml:",innerxml"`
    Error   *APIError `xml:"error,omitempty"`
}

type APIError struct {
    Code    string `xml:"code,attr"`
    Message string `xml:",innerxml"`
}

// Device Info
type DeviceInfo struct {
    XMLResponse
    XMLName     xml.Name `xml:"info"`
    DeviceID    string   `xml:"deviceID,attr"`
    Name        string   `xml:"name"`
    Type        string   `xml:"type"`
    Components  []string `xml:"components>component"`
    // ... additional fields
}

// Now Playing with complete structure
type NowPlaying struct {
    XMLResponse
    XMLName    xml.Name    `xml:"nowPlaying"`
    DeviceID   string      `xml:"deviceID,attr"`
    Source     string      `xml:"source,attr"`
    Content    ContentItem `xml:"ContentItem"`
    Track      string      `xml:"track"`
    Artist     string      `xml:"artist"`
    Album      string      `xml:"album"`
    Art        Art         `xml:"art"`
    PlayStatus PlayStatus  `xml:"playStatus"`
    Position   Position    `xml:"position,omitempty"`
}

// Enum types with validation
type PlayStatus string
const (
    PlayStatusPlaying PlayStatus = "PLAY_STATE"
    PlayStatusPaused  PlayStatus = "PAUSE_STATE" 
    PlayStatusStopped PlayStatus = "STOP_STATE"
)

type Key string
const (
    KeyPlay        Key = "PLAY"
    KeyPause       Key = "PAUSE"
    KeyStop        Key = "STOP"
    KeyPrevTrack   Key = "PREV_TRACK"
    KeyNextTrack   Key = "NEXT_TRACK"
    KeyVolumeUp    Key = "VOLUME_UP"
    KeyVolumeDown  Key = "VOLUME_DOWN"
    KeyMute        Key = "MUTE"
    KeyPower       Key = "POWER"
    KeyPreset1     Key = "PRESET_1"
    KeyPreset2     Key = "PRESET_2"
    KeyPreset3     Key = "PRESET_3"
    KeyPreset4     Key = "PRESET_4"
    KeyPreset5     Key = "PRESET_5"
    KeyPreset6     Key = "PRESET_6"
)
```

#### 4. WebSocket Event Client (`pkg/websocket`)
```go
type EventClient struct {
    client      *client.Client
    conn        *websocket.Conn
    handlers    map[string]EventHandler
    stopChan    chan bool
    reconnect   bool
    backoff     time.Duration
    maxBackoff  time.Duration
}

type EventHandler func(event Event)

type Event struct {
    Type      string      `xml:"type,attr"`
    DeviceID  string      `xml:"deviceID,attr"`
    Data      interface{} `xml:",innerxml"`
    Timestamp time.Time   `json:"timestamp"`
}

func NewEventClient(client *client.Client) *EventClient
func (e *EventClient) Subscribe(eventType string, handler EventHandler)
func (e *EventClient) Unsubscribe(eventType string)
func (e *EventClient) Start() error
func (e *EventClient) Stop() error
func (e *EventClient) IsConnected() bool

// Event types
const (
    EventNowPlayingUpdated    = "nowPlayingUpdated"
    EventVolumeUpdated        = "volumeUpdated"
    EventConnectionState      = "connectionStateUpdated"
    EventPresetUpdated        = "presetUpdated"
)
```

#### 5. WASM JavaScript Bridge (`pkg/wasm`)
```go
//go:build wasm
// +build wasm

import "syscall/js"

// Global WASM API registration
func RegisterWASMFunctions()

// Device Discovery (via proxy)
func wasmDiscoverDevices(this js.Value, args []js.Value) interface{}

// Client Management
func wasmCreateClient(this js.Value, args []js.Value) interface{}
func wasmGetNowPlaying(this js.Value, args []js.Value) interface{}
func wasmSendKey(this js.Value, args []js.Value) interface{}
func wasmSetVolume(this js.Value, args []js.Value) interface{}
func wasmGetSources(this js.Value, args []js.Value) interface{}

// Event Streaming
func wasmStartEventStream(this js.Value, args []js.Value) interface{}
func wasmStopEventStream(this js.Value, args []js.Value) interface{}
```

#### 6. Configuration Management (`pkg/config`)
```go
type Config struct {
    // Server configuration
    WebPort      int           `env:"WEB_PORT" default:"8080"`
    APITimeout   time.Duration `env:"API_TIMEOUT" default:"10s"`
    
    // Discovery configuration  
    DiscoveryTimeout time.Duration `env:"DISCOVERY_TIMEOUT" default:"5s"`
    CacheDevices     bool          `env:"CACHE_DEVICES" default:"true"`
    CacheTTL         time.Duration `env:"CACHE_TTL" default:"5m"`
    
    // CORS configuration (for web proxy)
    CORSOrigins      []string `env:"CORS_ORIGINS" default:"*"`
    
    // Logging
    LogLevel    string `env:"LOG_LEVEL" default:"info"`
    LogFormat   string `env:"LOG_FORMAT" default:"json"`
    
    // Development
    DevMode     bool `env:"DEV_MODE" default:"false"`
}

func Load() Config
func LoadFromFile(filename string) (Config, error)
func (c Config) Validate() error
```

## Implementation Roadmap (Updated)

### Phase 1: Foundation & Core API ⭐ ✅ COMPLETE
- [x] Go module setup with modern dependencies
- [x] **HTTP Client with XML Support** ✅ DONE
  - [x] Basic client structure
  - [x] GET/POST methods with XML marshaling
  - [x] Error handling for HTTP + XML
  - [x] Timeout and retry logic
- [x] **Core XML models** ✅ DONE
  - [x] DeviceInfo - Device information endpoint
  - [x] NowPlaying - Current playback status endpoint
  - [x] Sources - Available audio sources endpoint
  - [x] Name - Device name endpoint
  - [x] Capabilities - Device capabilities endpoint
  - [x] Presets - Configured presets endpoint
  - [x] Key - Media control commands endpoint
  - [x] Volume - Volume control endpoints
  - [x] Custom XML unmarshaling for enums
  - [x] Validation and defaults
- [x] **Media Controls** ✅ DONE
  - [x] POST /key endpoint implementation
  - [x] Press + Release key pattern (API compliant)
  - [x] Play, pause, stop, track navigation
  - [x] Volume up/down via keys
  - [x] Preset selection (1-6)
  - [x] Key validation and error handling
- [x] **Volume Management** ✅ DONE
  - [x] GET /volume endpoint
  - [x] POST /volume endpoint
  - [x] Incremental volume control
  - [x] Volume level validation and clamping
  - [x] Safety features and warnings
- [x] **Enhanced CLI tool** ✅ DONE
  - [x] Host:port parsing (host:8090 format)
  - [x] Device discovery via UPnP
  - [x] All informational endpoints
  - [x] Media control commands
  - [x] Volume management with safety
  - [x] Comprehensive help and examples
- [x] **Unit tests with mocks** ✅ DONE
  - [x] HTTP client tests
  - [x] XML parsing tests
  - [x] Key control tests
  - [x] Volume control tests
  - [x] Host:port parsing tests
  - [x] Mock responses with real device data

### Phase 2: Device Discovery & Management ✅ COMPLETE
- [x] **UPnP SSDP Discovery** ✅ DONE
  - [x] M-SEARCH implementation
  - [x] Response parsing
  - [x] Device caching with TTL
- [x] **CLI Device Selection** ✅ DONE
  - [x] Automatic discovery
  - [x] Host:port parsing enhancement
  - [x] Configuration-based device lists
- [x] **Integration Tests** ✅ DONE
  - [x] Tests against real SoundTouch devices (SoundTouch 10 & 20)
  - [x] Comprehensive real device validation
- [x] **Error Handling & Logging** ✅ DONE
  - [x] Structured error messages
  - [x] Graceful error handling
  - [x] Network timeout management

### Phase 3: Additional Control Endpoints 🎛️ (Next Priority)
- [ ] **Source Management**
  - POST /select - Switch audio sources
  - Source validation and error handling
- [ ] **Bass Control**
  - GET /bass - Get bass settings
  - POST /bass - Set bass level (-9 to +9)
- [x] **Preset Management (Read-Only)**
  - ~~POST /presets - Create/update presets~~ - **Officially not supported by SoundTouch API**
- [ ] **Advanced Features**
  - GET/POST /balance - Stereo balance (stereo devices)
  - GET/POST /clockTime - Device time management
  - GET/POST /clockDisplay - Clock display settings
  - GET /networkInfo - Network diagnostics

### Phase 4: WebSocket Real-time Events 📡
- [ ] **Implement WebSocket Client**
  - Connection Management
  - Event parsing and routing
  - Reconnection with exponential backoff
- [ ] **Event Handler System**
  - Typed event structs
  - Handler Registration
  - Event Filtering
- [ ] **CLI Real-time Monitoring**
  - Live Now-Playing Updates
  - Volume Change Monitoring
  - Connection Status Display
- [ ] **Event Storage & History**
  - Event logging for debugging
  - Historical Event Queries

### Phase 5: Web Application & CORS Proxy 🌐
- [ ] **Create Embedded Web UI**
  - HTML/CSS/JS for SoundTouch control
  - Responsive design for mobile
  - Real-time Updates via WebSocket
- [ ] **CORS-Proxy Server**
  - HTTP proxy to local SoundTouch devices
  - WebSocket proxy for events
  - CORS Header Management
- [ ] **Single Binary with Embedded Assets**
  - go:embed for web assets
  - Static File Serving
  - SPA Routing Support
- [ ] **Web-UI Features**
  - Device Discovery & Selection
  - Now playing display with album art
  - Volume & Bass Controls
  - Source Selection
  - Preset Management

### Phase 5: WASM Browser Integration 🧩
- [ ] **WASM Build Configuration**
  - Build tags and conditional compilation
  - WASM-specific HTTP client (via proxy)
  - JavaScript Promise Integration
- [ ] **WASM JavaScript Bridge**
  - Go function export to JavaScript
  - Asynchronous API calls
  - Error handling via promise rejection
- [ ] **Browser Demo Application**
  - Pure Frontend SoundTouch Control
  - Local Network Device Discovery (via Proxy)
  - Real-time Event Updates
- [ ] **Cross-Origin Solutions**
  - Local proxy server for development
  - Browser Extension Support
  - Documentation for CORS issues

### Phase 6: Production Features & Polish 🚀
- [ ] **Advanced Configuration**
  - Environment-based Config
  - Configuration File Support
  - Runtime Configuration Updates
- [ ] **Multi-Device Support**
  - Multiple Device Connections
  - Device Groups/Zones
  - Synchronized Operations
- [ ] **Preset & Source Management**
  - Preset Backup/Restore
  - Custom Source Integration
  - Playlist Management
- [ ] **Performance Optimizations**
  - Connection Pooling
  - Request Caching
  - Lazy Loading
- [ ] **Documentation & Examples**
  - Comprehensive API Documentation
  - Usage examples for all use cases
  - Best Practices Guide

## Build System Based on Modern Patterns

### Makefile with Multi-Target Support
```makefile
BINARY_NAME=soundtouch
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION=$(shell go version | cut -d ' ' -f 3)

LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GoVersion=$(GO_VERSION)"
BUILD_FLAGS=-trimpath $(LDFLAGS)

# Development builds
build:
	go build $(BUILD_FLAGS) -o $(BINARY_NAME) ./cmd/cli

build-webapp:
	go build $(BUILD_FLAGS) -o $(BINARY_NAME)-webapp ./cmd/webapp

# WASM build
build-wasm:
	GOOS=js GOARCH=wasm go build $(BUILD_FLAGS) -o web/soundtouch.wasm ./cmd/wasm
	cp "$(shell go env GOROOT)/misc/wasm/wasm_exec.js" web/

# Cross-platform builds
build-all: build-linux build-darwin build-windows

# Development with hot reload
dev-cli:
	air -c .air-cli.toml

dev-webapp: 
	air -c .air-webapp.toml

dev-wasm:
	air -c .air-wasm.toml

# Testing
test:
	go test -v -race ./...

test-coverage:
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Quality checks
check: fmt vet lint test

# Docker development environment
docker-dev:
	docker-compose up --build

# Release packaging
release: build-all
	mkdir -p dist
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz $(BINARY_NAME)-linux-amd64
	tar -czf dist/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz $(BINARY_NAME)-darwin-amd64
	zip dist/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip $(BINARY_NAME)-windows-amd64.exe
```

## Technical Solution Approaches (Updated)

### WASM Browser Integration
1. **CORS Proxy Pattern**: Go web app as proxy between browser and SoundTouch devices
2. **Local Development Server**: CORS headers for local development
3. **WebSocket Proxy**: Real-time events via secure WebSocket connection
4. **Graceful Degradation**: Functionality depending on browser environment

### XML API Robustness
1. **Type-Safe Models**: Strict Go structs with validation
2. **Custom Unmarshaling**: Enum validation and error recovery
3. **Timeout Handling**: Robust network calls with retry logic
4. **Connection Pooling**: Efficient HTTP client reuse

### Multi-Platform Deployment
1. **Single Binary**: Embedded assets eliminate external dependencies
2. **Cross-Compilation**: Native binaries for all platforms
3. **Docker Support**: Containerized development and deployment
4. **Progressive Enhancement**: CLI → WebApp → WASM depending on requirements

## Example Usage (Updated)

### Native Go Library
```go
package main

import (
    "fmt"
    "log"
    "time"
    
    "github.com/gesellix/bose-soundtouch/pkg/client"
    "github.com/gesellix/bose-soundtouch/pkg/discovery"
    "github.com/gesellix/bose-soundtouch/pkg/models"
)

func main() {
    // Discover devices
    discoveryService := discovery.NewDiscoveryService(5 * time.Second)
    devices, err := discoveryService.DiscoverDevices()
    if err != nil {
        log.Fatal(err)
    }
    
    if len(devices) == 0 {
        log.Fatal("No SoundTouch devices found")
    }
    
    // Create client for first device
    client := client.NewClient(client.ClientConfig{
        Host:    devices[0].Host,
        Port:    8090,
        Timeout: 10 * time.Second,
    })
    
    // Get device info
    info, err := client.GetDeviceInfo()
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Connected to: %s\n", info.Name)
    
    // Get current playback
    nowPlaying, err := client.GetNowPlaying()
    if err != nil {
        log.Fatal(err)
    }
    
    if nowPlaying.PlayStatus == models.PlayStatusPlaying {
        fmt.Printf("Playing: %s - %s (%s)\n", 
            nowPlaying.Artist, nowPlaying.Track, nowPlaying.Album)
    }
    
    // Control playback
    if nowPlaying.PlayStatus == models.PlayStatusPlaying {
        client.SendKey(models.KeyPause)
        fmt.Println("Paused playback")
    } else {
        client.SendKey(models.KeyPlay)
        fmt.Println("Started playback")
    }
}
```

### CLI Usage
```bash
# Discover devices
soundtouch discover

# Device operations
soundtouch --device 192.168.1.100 info
soundtouch --device 192.168.1.100 play
soundtouch --device 192.168.1.100 volume 50
soundtouch --device 192.168.1.100 preset 1

# Interactive mode
soundtouch interactive

# Web interface
soundtouch-webapp --port 8080
```

### JavaScript/WASM Usage
```javascript
// Load WASM module
await loadWASM('/soundtouch.wasm');

// Discover devices (via proxy)
const devices = await boseAPI.discoverDevices();
console.log('Found devices:', devices);

// Create client
const client = boseAPI.createClient(devices[0].host, 8090);

// Get now playing
const nowPlaying = await client.getNowPlaying();
console.log(`Playing: ${nowPlaying.artist} - ${nowPlaying.track}`);

// Control playback
await client.sendKey('PAUSE');

// Volume control
await client.setVolume(75);

// Real-time events
client.startEventStream((event) => {
    if (event.type === 'nowPlayingUpdated') {
        updateUI(event.data);
    }
});
```

## Testing Strategy (Enhanced)

### Unit Tests
- **Mock HTTP Client**: Simulierte SoundTouch-Responses
- **XML Parsing Tests**: Robustness für verschiedene Response-Formate
- **Model Validation**: Enum-Validation und Edge-Cases
- **Error Handling**: Network Failures und API Errors

### Integration Tests
- **Real Device Tests**: Gegen echte SoundTouch-Hardware
- **Docker Mock Server**: Simulierte SoundTouch-API für CI/CD
- **Discovery Tests**: UPnP SSDP in verschiedenen Netzwerk-Szenarien
- **WebSocket Tests**: Event-Streaming und Reconnection

### E2E Tests
- **CLI Tests**: Command-Line Interface Validation
- **Web Interface Tests**: Browser-basierte Tests mit Headless Chrome
- **WASM Tests**: Browser WASM Module Loading und Execution
- **Cross-Platform Tests**: Builds auf Linux/macOS/Windows

## Deployment Strategies

### Single Binary Distribution
```bash
# CLI Tool
./soundtouch-linux-amd64 discover
./soundtouch-linux-amd64 --device IP play

# Web Application (embedded assets)
./soundtouch-webapp-linux-amd64 --port 8080

# Docker
docker run -p 8080:8080 soundtouch-webapp
```

### Development Environment
```bash
# Local development with hot reload
make dev-webapp    # Web app development
make dev-wasm      # WASM development  
make dev-cli       # CLI development

# Full development environment
docker-compose up  # Mock devices + web app
```

## Success Criteria

### Phase 1-2 (Foundation)
- ✅ Stable HTTP API connection to SoundTouch devices
- ✅ XML model coverage for implemented APIs (DeviceInfo, NowPlaying, Sources, Name, Capabilities, Presets)
- ✅ Automatic device discovery via UPnP
- ✅ Functional CLI tool with discovery, info, now playing, sources, name, capabilities, and presets commands
- ✅ Now Playing endpoint with comprehensive status information
- ✅ Sources endpoint with filtering and categorization features
- ✅ Device identification endpoints (name, capabilities)
- ✅ Preset management with comprehensive analysis and filtering

### Phase 3-4 (Real-time & Web)
- ✅ WebSocket event streaming with reconnection
- ✅ Web UI with responsive design
- ✅ Single binary deployment with embedded assets
- ✅ CORS proxy for browser integration

### Phase 5-6 (Advanced)
- ✅ WASM integration with JavaScript bridge
- ✅ Multi-Device Support
- ✅ Production-ready Configuration Management
- ✅ Comprehensive documentation and examples

## Resources & References

- [Bose SoundTouch Web API Documentation](https://assets.bosecreative.com/m/496577402d128874/original/SoundTouch-Web-API.pdf)
- [Go WebAssembly](https://github.com/golang/go/wiki/WebAssembly)
- [UPnP Device Architecture](http://upnp.org/specs/arch/UPnP-arch-DeviceArchitecture-v1.0.pdf) 
- [Go Embed Directive](https://pkg.go.dev/embed)
- [Gorilla WebSocket](https://github.com/gorilla/websocket)
- [PROJECT-PATTERNS.md](./PROJECT-PATTERNS.md) - Detailed pattern documentation